// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/actions"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/baseline"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/enums"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/grants"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/issues"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/merge"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/params"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/paths"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/required"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/routes"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/sdk"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// Seams for what a test cannot otherwise reach: the process exit behind a
// fatal error, a repository root that cannot be found, the analyzers whose
// failures the real tree never produces, the JSON encoder that never fails on
// a report of strings and ints, and GitHub. Each is a variable a test
// restores.
var (
	fatalf         = cmdutil.Fatalf
	repositoryRoot = cmdutil.RepositoryRoot
	structsRun     = structs.Run
	actionsRun     = actions.Run
	enumsRun       = enums.Run
	sdkRun         = sdk.Run
	pathsRun       = paths.Run
	grantsRun      = grants.Run
	requiredRun    = required.Run
	paramsRun      = params.Run
	routesRun      = routes.Run
	marshalIndent  = json.MarshalIndent
	// releaseRef and buildDocEndpoints are the documentation oracle's
	// generator: the ref the live record names, and the sweep of doc/api at
	// it, which a test answers without the network.
	releaseRef        = paths.ReleaseRef
	buildDocEndpoints = paths.BuildDocEndpoints
	// githubAPI and getenv are where -check-issues asks and the token it asks
	// with, which a test points at a stand-in GitHub.
	githubAPI = issues.DefaultAPI
	getenv    = os.Getenv
)

func main() {
	outputPath := flag.String("output", "-", "path to write JSON report, or '-' for stdout")
	gapsOnly := flag.Bool("gaps-only", false, "only include entries with at least one finding")
	scope := flag.String("scope", "structs,actions,enums", "one of {structs,actions,enums,sdk,paths,grants,required,params,routes} for a single-scope report, or the first three (default) for the merged backlog; other combinations are not supported, and metadata (R-META) is retired in favor of cmd/audit_discovery_completeness")
	validateDocs := flag.Bool("validate-docs", false, "instead of the audit, verify every doc/api citation in the adjudication tables is still fetchable (exits non-zero on a stale citation)")
	checkEndpoints := flag.Bool("check-endpoints", false, "with -scope=paths, also compare every recorded REST endpoint with the documentation oracle "+paths.DocEndpointsFile+" (offline; fails on an endpoint neither it nor a declaration in cmd/audit_1to1/internal/paths accounts for, and on an oracle read at another release than the live record's)")
	writeDocEndpoints := flag.Bool("write-doc-endpoints", false, "instead of the audit, read GitLab's API documentation (doc/api of gitlab-org/gitlab) at -doc-ref and write the endpoint oracle -check-endpoints reads to -output (needs the network and reads ~250 pages; make gen-api-doc-endpoints)")
	docRef := flag.String("doc-ref", "", "the gitlab-org/gitlab tag or branch to read doc/api at: with -write-doc-endpoints, by default the tag of the live API record's release, the only ref -check-endpoints accepts; with -validate-docs, by default "+citationsRef)
	e2eCalls := flag.String("e2e-calls", "", "with -scope=paths or -scope=grants, the shard directory an end-to-end run recorded its calls into (dist/e2e-calls after `make test-e2e-ce`), which lets the observation question (paths) and the request count and routes of each action (grants) be asked per action rather than per owning package; reports and never gates, and a directory either scope cannot read fails the run")
	refresh := flag.Bool("refresh", false, "with -validate-docs or -write-doc-endpoints, force re-fetch of the docs even when cached and fresh")
	offline := flag.Bool("offline", false, "with -validate-docs or -write-doc-endpoints, use only cached docs; do not fetch")
	maxAge := flag.Duration("max-age", apidocs.DefaultMaxAge, "with -validate-docs or -write-doc-endpoints, re-download cached docs older than this")
	writeBaseline := flag.Bool("write-baseline", false, "instead of the audit, run the structs, actions, params, paths, required, grants and enums scopes and write every finding they report without gating, as keys by stream, to -output (make gen-audit-1to1-baseline writes "+baseline.File+")")
	checkBaseline := flag.Bool("check-baseline", false, "instead of the audit, hold the findings those scopes report without gating to the committed "+baseline.File+" and write the comparison to -output; exits non-zero on a finding the file does not hold and on a key of it no finding answers")
	checkIssues := flag.Bool("check-issues", false, "instead of the audit, ask GitHub whether every issue a tracked row of the routes and sdk reports names is still open, with GITHUB_TOKEN or GH_TOKEN when set, and write the answers to -output; exits non-zero on an issue that is not open and on one it could not ask about (needs the network)")
	flag.Parse()

	if *validateDocs {
		runValidateDocsMode(*outputPath, *docRef, *refresh, *offline, *maxAge)
		return
	}
	if *writeDocEndpoints {
		// Strict, as -validate-docs is: a page the sweep cannot download must
		// fail it rather than be answered by an older cached copy.
		runWriteDocEndpointsMode(*outputPath, *docRef, apidocs.Options{Refresh: *refresh, Offline: *offline, MaxAge: *maxAge, Strict: true})
		return
	}
	if *writeBaseline {
		failOn(writeBaselineFile(*outputPath))
		return
	}
	if *checkBaseline {
		failOn(checkBaselineFile(*outputPath))
		return
	}
	if *checkIssues {
		runCheckIssuesMode(*outputPath)
		return
	}

	if err := run(options{
		scope:      *scope,
		gapsOnly:   *gapsOnly,
		outputPath: *outputPath,
		endpoints:  *checkEndpoints,
		e2eCalls:   *e2eCalls,
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
	// endpoints asks the paths scope for the endpoint comparison, which reads
	// the committed documentation oracle.
	endpoints bool
	// e2eCalls is the shard directory an end-to-end run recorded, read by the
	// paths and grants scopes. Empty leaves the per-action observation and
	// request counts out, which is every run outside a Docker session.
	e2eCalls string
}

// run resolves the -scope selection, produces the report it names (the merged
// backlog for the three merged streams, one analyzer's native shape for a
// single scope) and writes it to outputPath.
//
// It takes no context, because no scope waits on anything a caller could
// cancel: every input is committed or compiled in. The two modes that read the
// network, -validate-docs and -write-doc-endpoints, take one of their own.
func run(opts options) error {
	scopes, err := parseScope(opts.scope)
	if err != nil {
		return err
	}

	// Ifs rather than a tagless switch, so the mutation tool, which cannot see
	// a case expression, measures each condition.
	merged := isMergedScope(scopes)
	if !merged && len(scopes) != 1 {
		return fmt.Errorf("scope must be a single value or the merged set %s (got %d: %s); other combinations are not supported",
			strings.Join(mergedScopes, ","), len(scopes), strings.Join(scopes, ","))
	}
	var content []byte
	clean := true
	if merged {
		content, err = runMerged(opts.gapsOnly)
	} else {
		content, clean, err = runSingle(scopes[0], opts)
	}
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteReport(opts.outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	// The report is written before the gate fails, so whoever reads the failure
	// has the same artifact a passing run would have produced.
	if !clean {
		return errors.New(gateFailure(scopes))
	}
	return nil
}

// gateFailure names which gate refused. Several scopes gate and they answer
// different questions, so a reader of the exit line should not have to guess
// which one produced the report beside it.
func gateFailure(scopes []string) string {
	switch {
	case slices.Equal(scopes, []string{scopePaths}):
		return "audit_1to1: request-path findings (see report)"
	case slices.Equal(scopes, []string{scopeGrants}):
		return "audit_1to1: fine-grained grant inconsistencies (see report)"
	case slices.Equal(scopes, []string{scopeRequired}):
		return "audit_1to1: requiredness disagreements with GitLab (see report)"
	case slices.Equal(scopes, []string{scopeParams}):
		return "audit_1to1: param declarations that answer nothing (see report)"
	case slices.Equal(scopes, []string{scopeRoutes}):
		return "audit_1to1: live routes nothing answers, or route declarations that no longer describe the tree (see report)"
	default:
		return "audit_1to1: SDK parity findings (see report)"
	}
}

// citationsRef is the gitlab-org/gitlab ref -validate-docs reads the cited
// pages at when -doc-ref names none: master, since a citation is a claim about
// the documentation GitLab publishes now.
//
// The pages are read through the repository files API at that ref, as
// -write-doc-endpoints reads them, and not through the raw web route under
// master the shared fetcher defaults to. Measured on 2026-11-04, the raw route
// answered one of the 37 cited pages with HTTP 429 on all six attempts, as it
// did the oracle's sweep after some thirty pages (see apidocs.DefaultFilesURL),
// so a weekly run reading it reported a citation stale that was not.
const citationsRef = "master"

// runValidateDocsMode resolves the repo root, builds an API-doc fetcher at ref
// (citationsRef when empty), validates the cited docs, writes the report, and
// exits non-zero when any citation is stale so it can gate CI.
func runValidateDocsMode(outputPath, ref string, refresh, offline bool, maxAge time.Duration) {
	root, err := repositoryRoot(".")
	if err != nil {
		fatalf("find repository root: %v", err)
		return
	}
	// Strict: a download failure (e.g. an upstream 404 for a renamed/removed doc)
	// must surface as a stale citation rather than be masked by a cached copy.
	fetcher, err := apidocs.NewAtRef(root, cmp.Or(ref, citationsRef), apidocs.Options{Refresh: refresh, Offline: offline, MaxAge: maxAge, Strict: true})
	if err != nil {
		fatalf("%v", err)
		return
	}
	// Cancel the doc-fetch sweep on Ctrl+C so a slow validation aborts promptly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if validateErr := validateDocs(ctx, root, outputPath, fetcher); validateErr != nil {
		fatalf("%v", validateErr)
	}
}

// runWriteDocEndpointsMode writes the documentation oracle -check-endpoints
// reads, and exits non-zero when it could not, with a context that a Ctrl+C
// cancels: the sweep is 250 downloads, and a person who changed their mind
// should not wait it out.
func runWriteDocEndpointsMode(outputPath, ref string, docs apidocs.Options) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := writeDocEndpointsOracle(ctx, outputPath, ref, docs); err != nil {
		fatalf("%v", err)
	}
}

// writeDocEndpointsOracle reads doc/api at ref, or at the tag of the live
// record's release when ref is empty, and writes the oracle to outputPath.
// Nothing is written when any page could not be read, so a failed sweep leaves
// the committed oracle as it was.
func writeDocEndpointsOracle(ctx context.Context, outputPath, ref string, docs apidocs.Options) error {
	root, err := repositoryRoot(".")
	if err != nil {
		return fmt.Errorf("find repository root: %w", err)
	}
	if ref == "" {
		if ref, err = releaseRef(root); err != nil {
			return fmt.Errorf("read the release to read the documentation at: %w", err)
		}
	}
	fetcher, err := apidocs.NewAtRef(root, ref, docs)
	if err != nil {
		return err
	}
	content, err := buildDocEndpoints(ctx, fetcher, ref)
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteReport(outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	return nil
}

// validateDocs runs the citation check against fetcher, writes the report to
// outputPath, and returns an error when the report could not be produced or
// when any citation is stale.
func validateDocs(ctx context.Context, root, outputPath string, fetcher *apidocs.Fetcher) error {
	content, ok, err := runValidateDocs(ctx, root, fetcher)
	if err != nil {
		return err
	}
	if writeErr := docgen.WriteReport(outputPath, content); writeErr != nil {
		return fmt.Errorf("write output: %w", writeErr)
	}
	if !ok {
		return errors.New("audit_1to1: stale doc/api citations found (see report)")
	}
	return nil
}

// failOn ends the run through fatalf when a mode returned an error.
func failOn(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

// runMerged runs the three analyzers and produces the merged backlog JSON via
// the shared merge pipeline. The root is resolved once for the three
// filesystem scanners; enums reads the in-memory catalog beside the tree.
func runMerged(gapsOnly bool) ([]byte, error) {
	root, err := repositoryRoot(".")
	if err != nil {
		return nil, fmt.Errorf("find repository root: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [1/3] analyzing struct field mapping (R-INPUT/R-OUTPUT)...")
	structBytes, err := structsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("struct report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [2/3] analyzing action coverage (R-ACTION)...")
	actionBytes, err := actionsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("action report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: [3/3] analyzing enum values (R-ENUM)...")
	// The merged backlog is a report, not a gate, so the enum stream's own
	// verdict is not consulted here; -scope=sdk is where it fails the build.
	enumBytes, _, err := enumsRun(root, gapsOnly)
	if err != nil {
		return nil, fmt.Errorf("enum report: %w", err)
	}
	cmdutil.Progressf("audit_1to1: merging backlog...")
	return merge.BuildBacklogFromBytes(structBytes, actionBytes, enumBytes)
}

// runSingle runs one analyzer and returns its native JSON shape plus whether
// that scope's gate passes. The structs and actions streams always report
// clean, because a listed candidate is a backlog entry rather than a defect;
// every other scope gates.
func runSingle(scope string, opts options) (content []byte, clean bool, err error) {
	switch scope {
	case "structs", "actions", "enums", "sdk", scopePaths, scopeGrants, scopeRequired, scopeParams, scopeRoutes:
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
			return pathsRun(root, paths.Options{
				GapsOnly:       opts.gapsOnly,
				CheckEndpoints: opts.endpoints,
				E2ECallsDir:    opts.e2eCalls,
			})
		case scopeGrants:
			return grantsRun(root, grants.Options{GapsOnly: opts.gapsOnly, E2ECallsDir: opts.e2eCalls})
		case scopeRequired:
			return requiredRun(root, required.Options{GapsOnly: opts.gapsOnly})
		case scopeParams:
			return paramsRun(root, params.Options{GapsOnly: opts.gapsOnly})
		case scopeRoutes:
			return routesRun(root, routes.Options{GapsOnly: opts.gapsOnly})
		default:
			return sdkRun(root, opts.gapsOnly)
		}
	default:
		return nil, false, fmt.Errorf("unknown scope %q (valid: %s)", scope, strings.Join(validScopes, ", "))
	}
}

// scopePaths is the request-path dimension (R-PATH).
const scopePaths = "paths"

// scopeGrants is the fine-grained permission dimension (R-GRANT).
const scopeGrants = "grants"

// scopeRequired is R-REQUIRED: each parameter the input schema requires, held
// to what GitLab requires of it.
const scopeRequired = "required"

// scopeParams is R-PARAM: each param a route this server reaches declares,
// held to whether anything here can send it.
const scopeParams = "params"

// scopeRoutes is R-ROUTE: each route the live record mounts, held to an
// action that sends it or a reason none does.
const scopeRoutes = "routes"

// mergedScopes is the set the merged backlog is built from, sorted. The sdk,
// paths and grants scopes are deliberately not among them: they gate rather
// than accumulating candidates, and adding any would change the shape of
// plan/1to1-backlog.json.
var mergedScopes = []string{"actions", "enums", "structs"}

// validScopes is every value -scope accepts, in the order a message lists them.
var validScopes = []string{"structs", "actions", "enums", "sdk", scopePaths, scopeGrants, scopeRequired, scopeParams, scopeRoutes}

// scopeMetadata is R-META, the discovery metadata stream, which is retired.
const scopeMetadata = "metadata"

// retiredMetadataScope is the refusal of -scope=metadata. R-META counted four
// flags of the ActionSpec catalog and gated nothing, while
// cmd/audit_discovery_completeness raises the same four with a severity and
// fails on the errors among them, so a second count of them in this command
// was a report nobody acted on.
const retiredMetadataScope = `scope "metadata" (R-META) is retired: cmd/audit_discovery_completeness raises its four flags (generic_usage, aliases_only_toolname, empty_related, weak_individual_description) with a severity, and make audit-discovery-check fails on the errors among them`

// isMergedScope reports whether scopes is exactly the merged set, so a
// selection that merely happens to have as many entries (say
// structs,actions,sdk) is rejected instead of silently merging.
func isMergedScope(scopes []string) bool {
	return slices.Equal(scopes, mergedScopes)
}

// parseScope validates and normalizes the -scope flag. Returns the deduplicated,
// sorted list of scopes. An empty value or "all" expands to the merged set.
// The retired metadata scope is refused with the message that says where its
// flags went, so the old default, which named it, is refused the same way.
func parseScope(s string) ([]string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" {
		return slices.Clone(mergedScopes), nil
	}
	seen := map[string]bool{}
	var scopes []string
	for raw := range strings.SplitSeq(s, ",") {
		v := strings.TrimSpace(raw)
		if v == scopeMetadata {
			return nil, errors.New(retiredMetadataScope)
		}
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
