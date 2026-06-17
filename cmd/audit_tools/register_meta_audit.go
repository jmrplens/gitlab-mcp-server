package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// registerMetaDefinition records one package-level RegisterMeta function
// discovered while walking the internal/tools tree.
//
// Package is the Go package that defines the function. File is the path
// (relative to repo root) of the source file containing it. ToolNames is the
// sorted list of "gitlab_*" individual tool names registered inside the
// function body. Referenced reports whether the package is called from
// internal/tools/register_meta.go's central dispatch.
type registerMetaDefinition struct {
	Package    string
	File       string
	ToolNames  []string
	Referenced bool
}

// unexpectedRegisterMetaDefinition explains why a RegisterMeta definition
// violates the catalog-first runtime contract.
//
// Reason is a human-readable string consumed verbatim by the markdown report.
type unexpectedRegisterMetaDefinition struct {
	Package string
	File    string
	Reason  string
}

// delegatedRegisterMetaPackages enumerates packages whose package-level
// RegisterMeta is explicitly approved as a delegated alias hub. Empty by
// default; the catalog-first migration must add entries here only after the
// team confirms the package is reachable from internal/tools/register_meta.go.
var delegatedRegisterMetaPackages = map[string]struct{}{}

// auditRegisterMetaDefinitions scans internal/tools for package-level
// RegisterMeta definitions and marks each one as referenced by the central
// hub or not.
//
// The returned slice is sorted by package name then file path so callers can
// produce stable reports.
func auditRegisterMetaDefinitions(root string) ([]registerMetaDefinition, error) {
	toolsDir := filepath.Join(root, "internal", "tools")
	definitions, err := findRegisterMetaDefinitions(root, toolsDir)
	if err != nil {
		return nil, err
	}
	references, err := referencedRegisterMetaPackages(filepath.Join(toolsDir, "register_meta.go"))
	if err != nil {
		return nil, err
	}
	for index := range definitions {
		_, definitions[index].Referenced = references[definitions[index].Package]
	}
	return definitions, nil
}

// auditRegisterMetaDefinitionViolations converts each unexpected definition
// into a violation the markdown reporter can render.
func auditRegisterMetaDefinitionViolations(definitions []registerMetaDefinition) []violation {
	unexpected := unexpectedRegisterMetaDefinitions(definitions)
	violations := make([]violation, 0, len(unexpected))
	for _, definition := range unexpected {
		violations = append(violations, violation{
			tool:     definition.Package,
			category: "register-meta",
			detail:   fmt.Sprintf("%s (%s)", definition.Reason, definition.File),
		})
	}
	return violations
}

// unexpectedRegisterMetaDefinitions flags definitions that are neither in the
// allow-list of delegated packages nor reachable from the central hub.
func unexpectedRegisterMetaDefinitions(definitions []registerMetaDefinition) []unexpectedRegisterMetaDefinition {
	unexpected := make([]unexpectedRegisterMetaDefinition, 0)
	for _, definition := range definitions {
		if _, ok := delegatedRegisterMetaPackages[definition.Package]; !ok {
			unexpected = append(unexpected, unexpectedRegisterMetaDefinition{
				Package: definition.Package,
				File:    definition.File,
				Reason:  "package-level RegisterMeta is not an approved catalog-first runtime pattern",
			})
			continue
		}
		if !definition.Referenced {
			unexpected = append(unexpected, unexpectedRegisterMetaDefinition{
				Package: definition.Package,
				File:    definition.File,
				Reason:  "approved delegated RegisterMeta is not referenced from internal/tools/register_meta.go",
			})
		}
	}
	return unexpected
}

// isDelegatedRegisterMetaDefinition reports whether the definition is both
// allow-listed and reachable from the central RegisterMeta hub.
func isDelegatedRegisterMetaDefinition(definition registerMetaDefinition) bool {
	_, ok := delegatedRegisterMetaPackages[definition.Package]
	return ok && definition.Referenced
}

// findRegisterMetaDefinitions walks toolsDir looking for top-level
// RegisterMeta functions and extracts the individual tool names they declare.
//
// Duplicate builder names across files produce an error so the migration can
// surface accidental re-introductions.
func findRegisterMetaDefinitions(root, toolsDir string) ([]registerMetaDefinition, error) {
	var definitions []registerMetaDefinition
	fileSet := token.NewFileSet()
	err := filepath.WalkDir(toolsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Name.Name != "RegisterMeta" || function.Recv != nil {
				continue
			}
			relative, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			definitions = append(definitions, registerMetaDefinition{
				Package:   file.Name.Name,
				File:      filepath.ToSlash(relative),
				ToolNames: registerMetaToolNames(function),
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(definitions, func(i, j int) bool {
		if definitions[i].Package == definitions[j].Package {
			return definitions[i].File < definitions[j].File
		}
		return definitions[i].Package < definitions[j].Package
	})
	return definitions, nil
}

// registerMetaToolNames returns the sorted, deduplicated list of "gitlab_*"
// tool names declared inside the body of a RegisterMeta function.
//
// The function walks the AST looking for Name:"gitlab_*" string literals,
// which matches the registration pattern used by all delegated packages.
func registerMetaToolNames(function *ast.FuncDecl) []string {
	seen := make(map[string]struct{})
	var names []string
	ast.Inspect(function.Body, func(node ast.Node) bool {
		keyValue, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := keyValue.Key.(*ast.Ident)
		if !ok || key.Name != "Name" {
			return true
		}
		literal, ok := keyValue.Value.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil || !strings.HasPrefix(value, "gitlab_") {
			return true
		}
		if _, found := seen[value]; found {
			return true
		}
		seen[value] = struct{}{}
		names = append(names, value)
		return true
	})
	sort.Strings(names)
	return names
}

// referencedRegisterMetaPackages parses registerMetaPath and returns the set
// of package identifiers referenced by selector expressions ending in
// ".RegisterMeta" — i.e., the central dispatch site that delegates to each
// package-level RegisterMeta function.
func referencedRegisterMetaPackages(registerMetaPath string) (map[string]struct{}, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, registerMetaPath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", registerMetaPath, err)
	}
	references := make(map[string]struct{})
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "RegisterMeta" {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		references[identifier.Name] = struct{}{}
		return true
	})
	return references, nil
}
