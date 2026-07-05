package auditshared

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// IsActionSpecGroupBuilderName reports whether a function name follows the
// buildXxxActionSpecs convention used by the per-domain catalog builders.
func IsActionSpecGroupBuilderName(name string) bool {
	return strings.HasPrefix(name, "build") && strings.HasSuffix(name, "ActionSpecs") && len(name) > len("buildActionSpecs")
}

// DiscoverActionSpecGroupBuilders scans the top-level non-test, non-generated
// .go files of dir for buildXxxActionSpecs builder functions and returns
// their sorted names. It fails on duplicate builder names and when no
// builders are found, so both the manifest generator and the catalog-first
// auditor agree on the same source of truth.
func DiscoverActionSpecGroupBuilders(dir string) ([]string, error) {
	fileSet := token.NewFileSet()
	builders := make(map[string]string)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", path, parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !IsActionSpecGroupBuilderName(function.Name.Name) {
				continue
			}
			if previousPath, exists := builders[function.Name.Name]; exists {
				return fmt.Errorf("duplicate action spec group builder %s in %s and %s", function.Name.Name, previousPath, path)
			}
			builders[function.Name.Name] = path
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(builders) == 0 {
		return nil, errors.New("no action spec group builders found")
	}
	names := make([]string, 0, len(builders))
	for name := range builders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
