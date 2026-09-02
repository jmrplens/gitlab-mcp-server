// Command audit_1to1 is the consolidated 1:1 SDK↔API parity audit. It combines
// the three gap streams — struct field mapping (R-INPUT/R-OUTPUT), action
// coverage (R-ACTION), and discovery metadata (R-META) — behind a single
// -scope flag, and merges them in-process when all three run together.
//
// Single-scope mode emits that auditor's native JSON shape (matching the legacy
// audit_struct_completeness / audit_action_coverage / audit_metadata_completeness
// binaries byte-for-byte). All-scopes mode produces the merged per-package
// backlog that gen_1to1_backlog previously generated from three separate files,
// via the same merge pipeline (byte-identical by construction).
//
// Usage:
//
//	go run ./cmd/audit_1to1/                                  # merged backlog to stdout
//	go run ./cmd/audit_1to1/ -gaps-only -output plan/1to1-backlog.json
//	go run ./cmd/audit_1to1/ -scope=structs                   # struct report only
//	go run ./cmd/audit_1to1/ -scope=actions -gaps-only
//	go run ./cmd/audit_1to1/ -scope=metadata
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/actions"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/merge"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include entries with at least one finding")
	scope := flag.String("scope", "structs,actions,metadata", "one of {structs,actions,metadata} for a single-scope report, or all three (default) for the merged backlog; two-scope combinations are not supported")
	validateDocs := flag.Bool("validate-docs", false, "instead of the audit, verify every doc/api citation in the adjudication tables is still fetchable (exits non-zero on a stale citation)")
	refresh := flag.Bool("refresh", false, "with -validate-docs, force re-fetch of cited docs even when cached and fresh")
	offline := flag.Bool("offline", false, "with -validate-docs, use only cached docs; do not fetch")
	maxAge := flag.Duration("max-age", apidocs.DefaultMaxAge, "with -validate-docs, re-download cached docs older than this")
	flag.Parse()

	if *validateDocs {
		runValidateDocsMode(*outputPath, *refresh, *offline, *maxAge)
		return
	}

	if err := run(*scope, *gapsOnly, *outputPath); err != nil {
		cmdutil.Fatalf("%v", err)
	}
}

// run resolves the -scope selection, produces the report it names (the merged
// backlog for all three, one analyzer's native shape for a single scope) and
// writes it to outputPath.
func run(scope string, gapsOnly bool, outputPath string) error {
	scopes, err := parseScope(scope)
	if err != nil {
		return err
	}

	var content []byte
	switch {
	case len(scopes) == 3:
		content, err = runMerged(gapsOnly)
	case len(scopes) == 1:
		content, err = runSingle(scopes[0], gapsOnly)
	default:
		return fmt.Errorf("scope must be a single value or all three (got %d: %s); partial two-scope combinations are not supported",
			len(scopes), strings.Join(scopes, ","))
	}
	if err != nil {
		return err
	}
	if writeErr := writeOutput(outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	return nil
}

// runValidateDocsMode resolves the repo root, builds the shared API-doc fetcher,
// validates the cited docs, writes the report, and exits non-zero when any
// citation is stale so it can gate CI.
func runValidateDocsMode(outputPath string, refresh, offline bool, maxAge time.Duration) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}
	// Cancel the doc-fetch sweep on Ctrl+C so a slow validation aborts promptly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// Strict: a download failure (e.g. an upstream 404 for a renamed/removed doc)
	// must surface as a stale citation rather than be masked by a cached copy.
	fetcher := apidocs.New(root, apidocs.Options{Refresh: refresh, Offline: offline, MaxAge: maxAge, Strict: true})
	if validateErr := validateDocs(ctx, root, outputPath, fetcher); validateErr != nil {
		cmdutil.Fatalf("%v", validateErr)
	}
}

// validateDocs runs the citation check against fetcher, writes the report to
// outputPath, and returns an error when the report could not be produced or
// when any citation is stale.
func validateDocs(ctx context.Context, root, outputPath string, fetcher *apidocs.Fetcher) error {
	content, ok, err := runValidateDocs(ctx, root, fetcher)
	if err != nil {
		return err
	}
	if writeErr := writeOutput(outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	if !ok {
		return errors.New("audit_1to1: stale doc/api citations found (see report)")
	}
	return nil
}

// runMerged runs all three analyzers and produces the merged backlog JSON via
// the shared merge pipeline. The root is resolved once for the two filesystem
// scanners (structs, actions); metadata uses the in-memory catalog.
func runMerged(gapsOnly bool) ([]byte, error) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		return nil, fmt.Errorf("find repository root: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [1/3] analyzing struct field mapping (R-INPUT/R-OUTPUT)...")
	structBytes, err := structs.Run(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("struct report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [2/3] analyzing action coverage (R-ACTION)...")
	actionBytes, err := actions.Run(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("action report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [3/3] analyzing discovery metadata (R-META)...")
	metadataBytes, err := metadata.Run(gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("metadata report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: merging backlog...")
	return merge.BuildBacklogFromBytes(structBytes, actionBytes, metadataBytes)
}

// runSingle runs one analyzer and returns its native JSON shape.
func runSingle(scope string, gapsOnly bool) ([]byte, error) {
	switch scope {
	case "structs", "actions":
		root, err := cmdutil.RepositoryRoot(".")
		if err != nil {
			return nil, fmt.Errorf("find repository root: %w", err)
		}
		if scope == "structs" {
			return structs.Run(root, gapsOnly)
		}
		return actions.Run(root, gapsOnly)
	case "metadata":
		return metadata.Run(gapsOnly)
	default:
		return nil, fmt.Errorf("unknown scope %q (valid: structs, actions, metadata)", scope)
	}
}

// parseScope validates and normalizes the -scope flag. Returns the deduplicated,
// sorted list of scopes. An empty value or "all" expands to all three.
func parseScope(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" {
		return []string{"actions", "metadata", "structs"}, nil
	}
	seen := map[string]bool{}
	var scopes []string
	for raw := range strings.SplitSeq(s, ",") {
		v := strings.TrimSpace(raw)
		switch v {
		case "structs", "actions", "metadata":
			if !seen[v] {
				seen[v] = true
				scopes = append(scopes, v)
			}
		default:
			return nil, fmt.Errorf("invalid scope %q (valid: structs, actions, metadata, all)", v)
		}
	}
	sort.Strings(scopes)
	return scopes, nil
}

func writeOutput(outputPath string, content []byte) error {
	if outputPath == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o750); err != nil {
		return err
	}
	return os.WriteFile(outputPath, content, 0o600)
}
