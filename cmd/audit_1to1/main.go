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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/actions"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/merge"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include entries with at least one finding")
	scope := flag.String("scope", "structs,actions,metadata", "comma-separated subset of {structs,actions,metadata}; default all (merged backlog)")
	validateDocs := flag.Bool("validate-docs", false, "instead of the audit, verify every doc/api citation in the adjudication tables is still fetchable (exits non-zero on a stale citation)")
	refresh := flag.Bool("refresh", false, "with -validate-docs, force re-fetch of cited docs even when cached and fresh")
	offline := flag.Bool("offline", false, "with -validate-docs, use only cached docs; do not fetch")
	maxAge := flag.Duration("max-age", apidocs.DefaultMaxAge, "with -validate-docs, re-download cached docs older than this")
	flag.Parse()

	if *validateDocs {
		runValidateDocsMode(*outputPath, *refresh, *offline, *maxAge)
		return
	}

	scopes, err := parseScope(*scope)
	if err != nil {
		cmdutil.Fatalf("%v", err)
	}

	var content []byte
	switch {
	case len(scopes) == 3:
		content, err = runMerged(*gapsOnly)
	case len(scopes) == 1:
		content, err = runSingle(scopes[0], *gapsOnly)
	default:
		cmdutil.Fatalf("scope must be a single value or all three (got %d: %s); partial two-scope combinations are not supported",
			len(scopes), strings.Join(scopes, ","))
	}
	if err != nil {
		cmdutil.Fatalf("%v", err)
	}
	if writeErr := writeOutput(*outputPath, content); writeErr != nil {
		cmdutil.Fatalf("write output: %v", writeErr)
	}
}

// runValidateDocsMode resolves the repo root, builds the shared API-doc fetcher,
// validates the cited docs, writes the report, and exits non-zero when any
// citation is stale so it can gate CI.
func runValidateDocsMode(outputPath string, refresh, offline bool, maxAge time.Duration) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}
	fetcher := apidocs.New(root, apidocs.Options{Refresh: refresh, Offline: offline, MaxAge: maxAge})
	content, ok, err := runValidateDocs(root, fetcher)
	if err != nil {
		cmdutil.Fatalf("%v", err)
	}
	if writeErr := writeOutput(outputPath, content); writeErr != nil {
		cmdutil.Fatalf("write output: %v", writeErr)
	}
	if !ok {
		cmdutil.Fatalf("audit_1to1: stale doc/api citations found (see report)")
	}
}

// runMerged runs all three analyzers and produces the merged backlog JSON via
// the shared merge pipeline. The root is resolved once for the two filesystem
// scanners (structs, actions); metadata uses the in-memory catalog.
func runMerged(gapsOnly bool) ([]byte, error) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		return nil, fmt.Errorf("find repository root: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [1/3] analyzing struct field mapping (R-INPUT/R-OUTPUT)…")
	structBytes, err := structs.Run(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("struct report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [2/3] analyzing action coverage (R-ACTION)…")
	actionBytes, err := actions.Run(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("action report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [3/3] analyzing discovery metadata (R-META)…")
	metadataBytes, err := metadata.Run(gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("metadata report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: merging backlog…")
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
