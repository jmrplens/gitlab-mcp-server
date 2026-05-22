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

type registerMetaDefinition struct {
	Package    string
	File       string
	ToolNames  []string
	Referenced bool
}

type unexpectedRegisterMetaDefinition struct {
	Package string
	File    string
	Reason  string
}

var delegatedRegisterMetaPackages = map[string]struct{}{}

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

func isDelegatedRegisterMetaDefinition(definition registerMetaDefinition) bool {
	_, ok := delegatedRegisterMetaPackages[definition.Package]
	return ok && definition.Referenced
}

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
