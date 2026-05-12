package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
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

func findRegisterMetaDefinitions(root, toolsDir string) ([]registerMetaDefinition, error) {
	var definitions []registerMetaDefinition
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

		fileSet := token.NewFileSet()
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

func repositoryRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
		current = parent
	}
}
