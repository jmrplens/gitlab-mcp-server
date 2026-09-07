package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/actions"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/enums"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/merge"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/metadata"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/paths"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// Seams for what a test cannot otherwise reach: the process exit behind a
// fatal error, a repository root that cannot be found, the analyzers whose
// failures the real tree never produces, and the JSON encoder that never
// fails on a report of strings and ints. Each is a variable a test restores.
var (
	fatalf         = cmdutil.Fatalf
	repositoryRoot = cmdutil.RepositoryRoot
	structsRun     = structs.Run
	actionsRun     = actions.Run
	enumsRun       = enums.Run
	sdkRun         = sdk.Run
	pathsRun       = paths.Run
	marshalIndent  = json.MarshalIndent
)

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include entries with at least one finding")
	scope := flag.String("scope", "structs,actions,metadata,enums", "one of {structs,actions,metadata,enums,sdk,paths} for a single-scope report, or the first four (default) for the merged backlog; other combinations are not supported")
	validateDocs := flag.Bool("validate-docs", false, "instead of the audit, verify every doc/api citation in the adjudication tables is still fetchable (exits non-zero on a stale citation)")
	checkEndpoints := flag.Bool("check-endpoints", false, "with -scope=paths, also compare every recorded REST endpoint against GitLab's API documentation (needs the network and reads ~250 pages; fails on an endpoint no declaration in cmd/audit_1to1/internal/paths accounts for)")
	refresh := flag.Bool("refresh", false, "with -validate-docs or -check-endpoints, force re-fetch of cited docs even when cached and fresh")
	offline := flag.Bool("offline", false, "with -validate-docs or -check-endpoints, use only cached docs; do not fetch")
	maxAge := flag.Duration("max-age", apidocs.DefaultMaxAge, "with -validate-docs or -check-endpoints, re-download cached docs older than this")
	flag.Parse()

	if *validateDocs {
		runValidateDocsMode(*outputPath, *refresh, *offline, *maxAge)
		return
	}

	// Cancel a documentation sweep on Ctrl+C, the way -validate-docs does: it
	// is 250 fetches and a person who changed their mind should not wait it
	// out.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, options{
		scope:      *scope,
		gapsOnly:   *gapsOnly,
		outputPath: *outputPath,
		docs:       apidocs.Options{Refresh: *refresh, Offline: *offline, MaxAge: *maxAge},
		endpoints:  *checkEndpoints,
	}); err != nil {
		fatalf("%v", err)
	}
}

// options is what the flags decided, carried together because the paths scope
// needs more of them than the scopes that came before it.
type options struct {
	scope      string
	gapsOnly   bool
	outputPath string
	// docs configures the API-doc fetcher the endpoint comparison reads
	// through, and is used only when endpoints is set.
	docs      apidocs.Options
	endpoints bool
}

// run resolves the -scope selection, produces the report it names (the merged
// backlog for the four candidate streams, one analyzer's native shape for a
// single scope) and writes it to outputPath.
func run(ctx context.Context, opts options) error {
	scopes, err := parseScope(opts.scope)
	if err != nil {
		return err
	}

	var content []byte
	clean := true
	switch {
	case isMergedScope(scopes):
		content, err = runMerged(opts.gapsOnly)
	case len(scopes) == 1:
		content, clean, err = runSingle(ctx, scopes[0], opts)
	default:
		return fmt.Errorf("scope must be a single value or the merged set %s (got %d: %s); other combinations are not supported",
			strings.Join(mergedScopes, ","), len(scopes), strings.Join(scopes, ","))
	}
	if err != nil {
		return err
	}
	if writeErr := writeOutput(opts.outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	// The report is written before the gate fails, so whoever reads the failure
	// has the same artifact a passing run would have produced.
	if !clean {
		return errors.New(gateFailure(scopes))
	}
	return nil
}

// gateFailure names which gate refused. Two scopes gate and they answer
// different questions, so a reader of the exit line should not have to guess
// which one produced the report beside it.
func gateFailure(scopes []string) string {
	if slices.Equal(scopes, []string{scopePaths}) {
		return "audit_1to1: request-path findings (see report)"
	}
	return "audit_1to1: SDK parity findings (see report)"
}

// runValidateDocsMode resolves the repo root, builds the shared API-doc fetcher,
// validates the cited docs, writes the report, and exits non-zero when any
// citation is stale so it can gate CI.
func runValidateDocsMode(outputPath string, refresh, offline bool, maxAge time.Duration) {
	root, err := repositoryRoot(".")
	if err != nil {
		fatalf("find repository root: %v", err)
		return
	}
	// Cancel the doc-fetch sweep on Ctrl+C so a slow validation aborts promptly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	// Strict: a download failure (e.g. an upstream 404 for a renamed/removed doc)
	// must surface as a stale citation rather than be masked by a cached copy.
	fetcher := apidocs.New(root, apidocs.Options{Refresh: refresh, Offline: offline, MaxAge: maxAge, Strict: true})
	if validateErr := validateDocs(ctx, root, outputPath, fetcher); validateErr != nil {
		fatalf("%v", validateErr)
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

// runMerged runs all four analyzers and produces the merged backlog JSON via
// the shared merge pipeline. The root is resolved once for the three
// filesystem scanners (structs, actions, enums); metadata uses the in-memory
// catalog alone, and enums reads it beside the tree.
func runMerged(gapsOnly bool) ([]byte, error) {
	root, err := repositoryRoot(".")
	if err != nil {
		return nil, fmt.Errorf("find repository root: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [1/4] analyzing struct field mapping (R-INPUT/R-OUTPUT)...")
	structBytes, err := structsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("struct report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [2/4] analyzing action coverage (R-ACTION)...")
	actionBytes, err := actionsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("action report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [3/4] analyzing discovery metadata (R-META)...")
	metadataBytes := metadata.Run(gapsOnly)
	cmdutil.Progressf("audit_1to1: [4/4] analyzing enum values (R-ENUM)...")
	// The merged backlog is a report, not a gate, so the enum stream's own
	// verdict is not consulted here; -scope=sdk is where it fails the build.
	enumBytes, _, err := enumsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("enum report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: merging backlog...")
	return merge.BuildBacklogFromBytes(structBytes, actionBytes, metadataBytes, enumBytes)
}

// runSingle runs one analyzer and returns its native JSON shape plus whether
// that scope's gate passes. Only sdk, enums and paths gate; the three
// candidate streams always report clean, because a listed candidate is a
// backlog entry rather than a defect.
func runSingle(ctx context.Context, scope string, opts options) (content []byte, clean bool, err error) {
	switch scope {
	case "structs", "actions", "enums", "sdk", scopePaths:
		root, rootErr := repositoryRoot(".")
		if rootErr != nil {
			return nil, false, fmt.Errorf("find repository root: %w", rootErr)
		}
		switch scope {
		case "structs":
			content, err = structsRun(root, opts.gapsOnly)
			return content, true, err
		case "actions":
			content, err = actionsRun(root, opts.gapsOnly)
			return content, true, err
		case "enums":
			return enumsRun(root, opts.gapsOnly)
		case scopePaths:
			return pathsRun(ctx, root, opts.gapsOnly, endpointFetcher(root, opts))
		default:
			return sdkRun(root, opts.gapsOnly)
		}
	case "metadata":
		// The metadata analyzer reads the in-memory catalog, not the tree, so
		// unlike the filesystem scanners it has nothing to fail at.
		return metadata.Run(opts.gapsOnly), true, nil
	default:
		return nil, false, fmt.Errorf("unknown scope %q (valid: %s)", scope, strings.Join(validScopes, ", "))
	}
}

// endpointFetcher returns the API-doc fetcher the endpoint comparison reads
// through, or nil when the comparison was not asked for.
//
// Nil is the default because the comparison needs the network and two minutes
// of it on a cold cache: a run that only wants the two offline checks should
// not pay for it. When it is asked for it gates, on the endpoints no
// declaration in cmd/audit_1to1/internal/paths accounts for.
func endpointFetcher(root string, opts options) *apidocs.Fetcher {
	if !opts.endpoints {
		return nil
	}
	return apidocs.New(root, opts.docs)
}

// scopePaths is the request-path dimension (R-PATH).
const scopePaths = "paths"

// mergedScopes is the set the merged backlog is built from, sorted. The sdk
// and paths scopes are deliberately not among them: both gate rather than
// accumulating candidates, and adding either would change the shape of
// plan/1to1-backlog.json.
var mergedScopes = []string{"actions", "enums", "metadata", "structs"}

// validScopes is every value -scope accepts, in the order a message lists them.
var validScopes = []string{"structs", "actions", "metadata", "enums", "sdk", scopePaths}

// isMergedScope reports whether scopes is exactly the merged set, so a
// selection that merely happens to have as many entries (say
// structs,actions,metadata,sdk) is rejected instead of silently merging.
func isMergedScope(scopes []string) bool {
	return slices.Equal(scopes, mergedScopes)
}

// parseScope validates and normalizes the -scope flag. Returns the deduplicated,
// sorted list of scopes. An empty value or "all" expands to the merged set.
func parseScope(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" {
		return slices.Clone(mergedScopes), nil
	}
	seen := map[string]bool{}
	var scopes []string
	for raw := range strings.SplitSeq(s, ",") {
		v := strings.TrimSpace(raw)
		if !slices.Contains(validScopes, v) {
			return nil, fmt.Errorf("invalid scope %q (valid: %s, all)", v, strings.Join(validScopes, ", "))
		}
		if !seen[v] {
			seen[v] = true
			scopes = append(scopes, v)
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
