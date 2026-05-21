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

const (
	defaultRoot          = "."
	statPathErrorFormat  = "stat %s: %w"
	writeStdoutErrorText = "write stdout: %w"
)

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

	rootFS, err := os.OpenRoot(opts.root)
	if err != nil {
		return fmt.Errorf("open root %s: %w", opts.root, err)
	}
	defer rootFS.Close()

	files, err := discoverMarkdownFiles(rootFS, opts.root, opts.paths)
	if err != nil {
		return err
	}

	changed := make([]string, 0)
	for _, file := range files {
		var fileChanged bool
		fileChanged, err = formatMarkdownTableFile(rootFS, file, opts.check)
		if err != nil {
			return err
		}
		if fileChanged {
			changed = append(changed, displayPath(file))
		}
	}

	if opts.check {
		if len(changed) > 0 {
			return fmt.Errorf("markdown tables are out of date in %d file(s): %s", len(changed), strings.Join(changed, ", "))
		}
		return writeStdout(stdout, "Markdown tables are up to date")
	}

	if len(changed) == 0 {
		return writeStdout(stdout, "Markdown tables already formatted")
	}
	if _, writeErr := fmt.Fprintf(stdout, "Formatted Markdown tables in %d file(s):\n", len(changed)); writeErr != nil {
		return fmt.Errorf(writeStdoutErrorText, writeErr)
	}
	for _, file := range changed {
		if _, writeErr := fmt.Fprintf(stdout, "- %s\n", file); writeErr != nil {
			return fmt.Errorf(writeStdoutErrorText, writeErr)
		}
	}
	return nil
}

func writeStdout(stdout io.Writer, message string) error {
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		return fmt.Errorf(writeStdoutErrorText, err)
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

func discoverMarkdownFiles(rootFS *os.Root, root string, paths []string) ([]string, error) {
	files := make([]string, 0)
	for _, item := range paths {
		discovered, err := markdownFilesForInput(rootFS, root, item)
		if err != nil {
			return nil, err
		}
		files = append(files, discovered...)
	}
	sort.Slice(files, func(i, j int) bool {
		return filepath.ToSlash(files[i]) < filepath.ToSlash(files[j])
	})
	return files, nil
}

func markdownFilesForInput(rootFS *os.Root, root, item string) ([]string, error) {
	rel, err := resolveInputPath(root, item)
	if err != nil {
		return nil, err
	}
	info, err := rootFS.Stat(rel)
	if err != nil {
		return nil, fmt.Errorf(statPathErrorFormat, item, err)
	}
	if info.IsDir() {
		return markdownFilesInDir(rootFS, rel, item)
	}
	if !strings.EqualFold(filepath.Ext(rel), ".md") {
		return nil, nil
	}
	return []string{rel}, nil
}

func markdownFilesInDir(rootFS *os.Root, rel, item string) ([]string, error) {
	files := make([]string, 0)
	walkErr := iofs.WalkDir(rootFS.FS(), filepath.ToSlash(rel), func(path string, entry iofs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return err
		}
		relPath := filepath.FromSlash(path)
		fileInfo, statErr := rootFS.Stat(relPath)
		if statErr != nil {
			return fmt.Errorf(statPathErrorFormat, relPath, statErr)
		}
		if !fileInfo.IsDir() {
			files = append(files, relPath)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", item, walkErr)
	}
	return files, nil
}

func resolveInputPath(root, item string) (string, error) {
	path := item
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, item)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", item, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %s escapes root %s", item, root)
	}
	return rel, nil
}

func formatMarkdownTableFile(rootFS *os.Root, path string, check bool) (bool, error) {
	content, err := rootFS.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	formatted, changed := docgen.FormatMarkdownTables(string(content))
	if !changed || check {
		return changed, nil
	}
	info, err := rootFS.Stat(path)
	if err != nil {
		return false, fmt.Errorf(statPathErrorFormat, path, err)
	}
	writeErr := rootFS.WriteFile(path, []byte(formatted), info.Mode().Perm())
	if writeErr != nil {
		return false, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}

func displayPath(path string) string {
	return filepath.ToSlash(path)
}
