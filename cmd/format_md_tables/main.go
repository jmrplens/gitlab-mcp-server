// Command format_md_tables normalizes Markdown pipe tables in README.md and docs/.
//
// Usage:
//
//	go run ./cmd/format_md_tables/
//	go run ./cmd/format_md_tables/ --check
//	go run ./cmd/format_md_tables/ README.md docs
package main

import (
	"flag"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/docgen"
)

const defaultRoot = "."

type options struct {
	root  string
	check bool
	paths []string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(opts.root)
	if err != nil {
		return fmt.Errorf("resolve root %s: %w", opts.root, err)
	}
	opts.root = root

	files, err := discoverMarkdownFiles(opts.root, opts.paths)
	if err != nil {
		return err
	}

	changed := make([]string, 0)
	for _, file := range files {
		var fileChanged bool
		fileChanged, err = formatMarkdownTableFile(file, opts.check)
		if err != nil {
			return err
		}
		if fileChanged {
			changed = append(changed, displayPath(opts.root, file))
		}
	}

	if opts.check {
		if len(changed) > 0 {
			return fmt.Errorf("markdown tables are out of date in %d file(s): %s", len(changed), strings.Join(changed, ", "))
		}
		_, _ = fmt.Fprintln(stdout, "Markdown tables are up to date")
		return nil
	}

	if len(changed) == 0 {
		_, _ = fmt.Fprintln(stdout, "Markdown tables already formatted")
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "Formatted Markdown tables in %d file(s):\n", len(changed))
	for _, file := range changed {
		_, _ = fmt.Fprintf(stdout, "- %s\n", file)
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("format_md_tables", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	opts := options{}
	flags.StringVar(&opts.root, "root", defaultRoot, "repository root containing README.md and docs/")
	flags.BoolVar(&opts.check, "check", false, "fail if any Markdown table needs formatting")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	opts.paths = flags.Args()
	if len(opts.paths) == 0 {
		opts.paths = []string{"README.md", "docs"}
	}
	return opts, nil
}

func discoverMarkdownFiles(root string, paths []string) ([]string, error) {
	files := make([]string, 0)
	for _, item := range paths {
		path, err := resolveInputPath(root, item)
		if err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", path, err)
		}
		if info.IsDir() {
			walkErr := filepath.WalkDir(path, func(path string, entry iofs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
					files = append(files, path)
				}
				return nil
			})
			if walkErr != nil {
				return nil, fmt.Errorf("walk %s: %w", path, walkErr)
			}
			continue
		}
		if strings.EqualFold(filepath.Ext(info.Name()), ".md") {
			files = append(files, path)
		}
	}
	sort.Strings(files)
	return files, nil
}

func resolveInputPath(root, item string) (string, error) {
	path := filepath.Clean(filepath.Join(root, item))
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", item, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes root %s", item, root)
	}
	return path, nil
}

func formatMarkdownTableFile(path string, check bool) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	formatted, changed := docgen.FormatMarkdownTables(string(content))
	if !changed || check {
		return changed, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	// #nosec G703 -- paths are resolved under --root before formatting repository files.
	writeErr := os.WriteFile(path, []byte(formatted), info.Mode().Perm())
	if writeErr != nil {
		return false, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}

func displayPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
