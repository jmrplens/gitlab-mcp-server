package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/paths"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// runScope runs one scope the way the command runs it with -gaps-only and no
// documentation comparison, which is what every case below asks for.
func runScope(scope, outputPath string) error {
	return run(context.Background(), options{scope: scope, gapsOnly: true, outputPath: outputPath})
}

// captureFatal replaces fatalf with a recorder and returns the messages it
// received, so a fatal path can be asserted on instead of exiting the test
// binary.
func captureFatal(t *testing.T) *[]string {
	t.Helper()
	original := fatalf
	t.Cleanup(func() { fatalf = original })
	var messages []string
	fatalf = func(message string, args ...any) {
		messages = append(messages, strings.TrimSpace(fmt.Sprintf(message, args...)))
	}
	return &messages
}

// resetFlags gives main a fresh flag set and the given arguments, restoring
// both afterwards, so main can be called more than once in one process.
func resetFlags(t *testing.T, args ...string) {
	t.Helper()
	originalArgs, originalFlags := os.Args, flag.CommandLine
	t.Cleanup(func() {
		os.Args, flag.CommandLine = originalArgs, originalFlags
	})
	os.Args = append([]string{"audit_1to1"}, args...)
	flag.CommandLine = flag.NewFlagSet("audit_1to1", flag.ContinueOnError)
}

// TestMain_Arguments_DispatchToTheModeAndReportFatalErrors runs main itself:
// a single-scope audit writes its report and exits normally, an invalid scope
// reaches the fatal path with the parse error, and -validate-docs in offline
// mode with nothing cached reaches the fatal path with the stale citations.
func TestMain_Arguments_DispatchToTheModeAndReportFatalErrors(t *testing.T) {
	cases := []struct {
		name string
		// args returns what main is given and the file it was told to write,
		// so the case that claims a report can read the report back. Naming
		// the path in the argument list alone left "writes its report"
		// asserted by nothing but the absence of a fatal message.
		args      func(t *testing.T) (args []string, output string)
		wantFatal string
	}{
		{
			name: "metadata_scope_writes_its_report",
			args: func(t *testing.T) ([]string, string) {
				t.Helper()
				output := filepath.Join(t.TempDir(), "metadata.json")
				return []string{"-scope=metadata", "-gaps-only", "-output", output}, output
			},
		},
		{
			name:      "invalid_scope_is_fatal",
			args:      func(*testing.T) ([]string, string) { return []string{"-scope=bogus"}, "" },
			wantFatal: `invalid scope "bogus"`,
		},
		{
			name: "validate_docs_offline_without_a_cache_is_fatal",
			args: func(t *testing.T) ([]string, string) {
				t.Helper()
				output := filepath.Join(t.TempDir(), "docs.json")
				return []string{"-validate-docs", "-offline", "-output", output}, output
			},
			wantFatal: "stale doc/api citations found",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			messages := captureFatal(t)
			args, output := tc.args(t)
			resetFlags(t, args...)
			main()
			switch {
			case tc.wantFatal == "" && len(*messages) != 0:
				t.Errorf("main reported %v, want a clean exit", *messages)
			case tc.wantFatal != "" && (len(*messages) != 1 || !strings.Contains((*messages)[0], tc.wantFatal)):
				t.Errorf("main reported %v, want one fatal message containing %q", *messages, tc.wantFatal)
			}
			if output == "" {
				return
			}
			if doc := readJSONFile(t, output); doc["schema_version"] == nil {
				t.Errorf("report at %s carries no schema_version (keys %v)", output, keysOf(doc))
			}
		})
	}
}

// TestRunValidateDocsMode_MissingRoot_IsFatal verifies the mode stops at a
// repository root it cannot find, before building a fetcher against it.
func TestRunValidateDocsMode_MissingRoot_IsFatal(t *testing.T) {
	messages := captureFatal(t)
	original := repositoryRoot
	t.Cleanup(func() { repositoryRoot = original })
	repositoryRoot = func(string) (string, error) { return "", errors.New("no go.mod above here") }

	runValidateDocsMode(filepath.Join(t.TempDir(), "docs.json"), false, true, 0)
	if len(*messages) != 1 || !strings.Contains((*messages)[0], "find repository root: no go.mod above here") {
		t.Errorf("fatal messages = %v, want the root error", *messages)
	}
}

// TestRunValidateDocsMode_EveryCitationCached_ExitsWithoutAFatal verifies the
// mode's other ending: a validation that finds nothing stale writes its report
// and returns, rather than reaching the fatal path.
//
// Until now only the failing ending was driven, so the branch that decides
// between them was evaluated one way in every run and a mode that always
// exited fatally would have passed. The tree is planted, because the
// repository's own citations are hundreds of pages nothing local can answer
// for; the cache is the one the fetcher reads for that root, so the mode
// builds its own fetcher exactly as it does in production.
func TestRunValidateDocsMode_EveryCitationCached_ExitsWithoutAFatal(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"cmd/audit_1to1/main.go":              "package main // see doc/api/alpha.md",
		".cache/gitlab-api-docs/alpha.md":     "# alpha",
		".cache/gitlab-api-docs/unrelated.md": "# not cited by the planted source",
	})

	original := repositoryRoot
	t.Cleanup(func() { repositoryRoot = original })
	repositoryRoot = func(string) (string, error) { return root, nil }
	messages := captureFatal(t)

	output := filepath.Join(t.TempDir(), "docs.json")
	runValidateDocsMode(output, false, true, 0)

	if len(*messages) != 0 {
		t.Fatalf("runValidateDocsMode reported %v, want a clean exit", *messages)
	}
	doc := readJSONFile(t, output)
	if checked, _ := doc["checked"].(float64); checked != 1 {
		t.Errorf("report checked = %v, want the one cited area", doc["checked"])
	}
	if okCount, _ := doc["ok"].(float64); okCount != 1 {
		t.Errorf("report ok = %v, want the one cited area validated", doc["ok"])
	}
}

// TestMain_PathsScope_CarriesEachFlagToTheFieldItNames verifies the two flags
// the paths scope reads land in their own field of [paths.Options], driven
// through main so the flag set itself is what fills them.
//
// -gaps-only and -check-endpoints are both booleans and were only ever passed
// together with the same value, so the options literal could have crossed them
// and every assertion would have held. Each case therefore sets one and leaves
// the other, and the shard directory beside them is a third value no other
// field could be holding.
func TestMain_PathsScope_CarriesEachFlagToTheFieldItNames(t *testing.T) {
	original := pathsRun
	t.Cleanup(func() { pathsRun = original })
	var got paths.Options
	pathsRun = func(_ context.Context, _ string, opts paths.Options) ([]byte, bool, error) {
		got = opts
		return []byte("{}\n"), true, nil
	}

	cases := []struct {
		name      string
		args      []string
		wantGaps  bool
		wantFetch bool
		wantDir   string
	}{
		{name: "endpoints_without_gaps_only", args: []string{"-check-endpoints"}, wantFetch: true},
		{name: "gaps_only_without_endpoints", args: []string{"-gaps-only"}, wantGaps: true},
		{name: "the_shard_directory", args: []string{"-e2e-calls", "/tmp/e2e-calls-fixture"}, wantDir: "/tmp/e2e-calls-fixture"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got = paths.Options{}
			messages := captureFatal(t)
			args := append([]string{"-scope=paths", "-output", filepath.Join(t.TempDir(), "paths.json")}, testCase.args...)
			resetFlags(t, args...)

			main()

			if len(*messages) != 0 {
				t.Fatalf("main reported %v, want a clean run", *messages)
			}
			if got.GapsOnly != testCase.wantGaps {
				t.Errorf("GapsOnly = %v, want %v", got.GapsOnly, testCase.wantGaps)
			}
			if (got.Fetcher != nil) != testCase.wantFetch {
				t.Errorf("Fetcher present = %v, want %v", got.Fetcher != nil, testCase.wantFetch)
			}
			if got.E2ECallsDir != testCase.wantDir {
				t.Errorf("E2ECallsDir = %q, want %q", got.E2ECallsDir, testCase.wantDir)
			}
		})
	}
}

// TestRun_AnalyzerFailures_AreNamedByStream verifies each seam-reachable
// failure of the merged run and the single-scope run surfaces with the name
// of the stream that failed, and that a gate reporting findings fails the run
// after the report was written.
func TestRun_AnalyzerFailures_AreNamedByStream(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name    string
		scope   string
		arrange func(t *testing.T)
		wantErr string
		// wantReport is set for the two gates, which write their report before
		// refusing so a reader of the failure holds the same artifact a passing
		// run would have produced. The analyzer failures above them produce no
		// report at all, having failed before there was one to write.
		wantReport bool
	}{
		{
			name: "merged_root_missing", scope: "all",
			arrange: func(t *testing.T) {
				t.Helper()
				original := repositoryRoot
				t.Cleanup(func() { repositoryRoot = original })
				repositoryRoot = func(string) (string, error) { return "", boom }
			},
			wantErr: "find repository root: boom",
		},
		{
			name: "single_root_missing", scope: "structs",
			arrange: func(t *testing.T) {
				t.Helper()
				original := repositoryRoot
				t.Cleanup(func() { repositoryRoot = original })
				repositoryRoot = func(string) (string, error) { return "", boom }
			},
			wantErr: "find repository root: boom",
		},
		{
			name: "struct_stream_fails", scope: "all",
			arrange: func(t *testing.T) {
				t.Helper()
				original := structsRun
				t.Cleanup(func() { structsRun = original })
				structsRun = func(string, bool) ([]byte, error) { return nil, boom }
			},
			wantErr: "struct report: boom",
		},
		{
			name: "action_stream_fails", scope: "all",
			arrange: func(t *testing.T) {
				t.Helper()
				originalStructs, originalActions := structsRun, actionsRun
				t.Cleanup(func() { structsRun, actionsRun = originalStructs, originalActions })
				structsRun = func(string, bool) ([]byte, error) { return []byte(`{"packages":[]}`), nil }
				actionsRun = func(string, bool) ([]byte, error) { return nil, boom }
			},
			wantErr: "action report: boom",
		},
		{
			name: "enum_stream_fails", scope: "all",
			arrange: func(t *testing.T) {
				t.Helper()
				originalStructs, originalActions, originalEnums := structsRun, actionsRun, enumsRun
				t.Cleanup(func() { structsRun, actionsRun, enumsRun = originalStructs, originalActions, originalEnums })
				structsRun = func(string, bool) ([]byte, error) { return []byte(`{"packages":[]}`), nil }
				actionsRun = func(string, bool) ([]byte, error) { return []byte(`{"services":[]}`), nil }
				enumsRun = func(string, bool) ([]byte, bool, error) { return nil, false, boom }
			},
			wantErr: "enum report: boom",
		},
		{
			name: "sdk_gate_reports_findings", scope: "sdk",
			arrange: func(t *testing.T) {
				t.Helper()
				original := sdkRun
				t.Cleanup(func() { sdkRun = original })
				sdkRun = func(string, bool) ([]byte, bool, error) { return []byte("{}\n"), false, nil }
			},
			wantErr: "SDK parity findings", wantReport: true,
		},
		{
			name: "paths_gate_reports_findings", scope: scopePaths,
			arrange: func(t *testing.T) {
				t.Helper()
				original := pathsRun
				t.Cleanup(func() { pathsRun = original })
				pathsRun = func(context.Context, string, paths.Options) ([]byte, bool, error) {
					return []byte("{}\n"), false, nil
				}
			},
			wantErr: "request-path findings", wantReport: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.arrange(t)
			output := filepath.Join(t.TempDir(), "out.json")
			err := runScope(tc.scope, output)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run(%q) error = %v, want it to contain %q", tc.scope, err, tc.wantErr)
			}
			if _, statErr := os.Stat(output); (statErr == nil) != tc.wantReport {
				t.Errorf("run(%q) report at %s: %v, want written == %v", tc.scope, output, statErr, tc.wantReport)
			}
		})
	}
}

// TestRunSingle_EnumScope_ReturnsTheGateVerdict verifies the enums scope is
// dispatched to the enum rule and its verdict is what the run reports, through
// the seam so the case does not depend on the tree being clean.
func TestRunSingle_EnumScope_ReturnsTheGateVerdict(t *testing.T) {
	original := enumsRun
	t.Cleanup(func() { enumsRun = original })
	enumsRun = func(_ string, gapsOnly bool) ([]byte, bool, error) {
		if !gapsOnly {
			t.Errorf("enums.Run gapsOnly = false, want the flag passed through")
		}
		return []byte("{}\n"), false, nil
	}
	content, clean, err := runSingle(t.Context(), "enums", options{gapsOnly: true})
	if err != nil || clean || string(content) != "{}\n" {
		t.Fatalf("runSingle(enums) = %q/%v/%v, want the seam's report and verdict", content, clean, err)
	}
}

// TestParseScope covers the happy and error paths of -scope parsing in one
// table: explicit/keyword/empty expansion to all three, single scope,
// deduplication, whitespace trimming, and rejection of unknown/garbage values.
func TestParseScope(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantCount int    // expected scope count on success
		wantFirst string // expected sole scope (single-scope cases); "" to skip
		wantErr   bool
		errSubstr string // required error substring when wantErr; "" to skip
	}{
		{name: "explicit all four", input: "structs,actions,metadata,enums", wantCount: 4},
		{name: "keyword all expands", input: "all", wantCount: 4},
		{name: "empty expands to all", input: "", wantCount: 4},
		{name: "single scope", input: "structs", wantCount: 1, wantFirst: "structs"},
		{name: "sdk scope", input: "sdk", wantCount: 1, wantFirst: "sdk"},
		{name: "enums scope", input: "enums", wantCount: 1, wantFirst: "enums"},
		{name: "deduplicates repeats", input: "structs,structs,actions,actions", wantCount: 2},
		{name: "trims whitespace", input: " structs , actions , metadata , enums ", wantCount: 4},
		{name: "rejects unknown token", input: "structs,unknown", wantErr: true, errSubstr: "unknown"},
		{name: "rejects garbage word", input: "foo", wantErr: true},
		{name: "rejects mixed garbage", input: "structs,bar", wantErr: true},
		{name: "rejects symbols", input: "???", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseScope(c.input)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseScope(%q) expected error, got nil", c.input)
				}
				if c.errSubstr != "" && !strings.Contains(err.Error(), c.errSubstr) {
					t.Errorf("parseScope(%q) error = %q, want substring %q", c.input, err, c.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseScope(%q) unexpected error: %v", c.input, err)
			}
			if len(got) != c.wantCount {
				t.Fatalf("parseScope(%q) = %v (%d scopes), want %d", c.input, got, len(got), c.wantCount)
			}
			if c.wantFirst != "" && (len(got) != 1 || got[0] != c.wantFirst) {
				t.Errorf("parseScope(%q) = %v, want [%s]", c.input, got, c.wantFirst)
			}
		})
	}
}

// TestRunMerged_EachStream_LandsUnderItsOwnKey verifies the merged backlog
// attributes each analyzer's figures to the summary key that names its stream,
// driven through the seams with three reports whose numbers share no value.
//
// The four reports reach the merge as four byte slices of one type, and each
// decodes into a report that ignores every key it does not name, so a crossed
// pair produces a backlog with both streams simply absent rather than a parse
// failure. The only assertion that ran over this path checked that the summary
// carried each key, which it does whatever the counts are.
func TestRunMerged_EachStream_LandsUnderItsOwnKey(t *testing.T) {
	originalStructs, originalActions, originalEnums := structsRun, actionsRun, enumsRun
	t.Cleanup(func() { structsRun, actionsRun, enumsRun = originalStructs, originalActions, originalEnums })
	structsRun = func(string, bool) ([]byte, error) {
		return []byte(`{"packages":[{"package":"alpha","missing_input_count":7,` +
			`"missing_output_count":11,"extra_output_count":13}]}`), nil
	}
	actionsRun = func(string, bool) ([]byte, error) {
		return []byte(`{"services":[{"service":"BetaService","packages":["beta"],` +
			`"api_methods":23,"covered_methods":20,"missing_methods":["One","Two","Three"]}]}`), nil
	}
	enumsRun = func(string, bool) ([]byte, bool, error) {
		return []byte(`{"packages":[{"package":"gamma","missing_values":17,"extra_values":19}]}`), true, nil
	}

	path := filepath.Join(t.TempDir(), "backlog.json")
	if err := runScope("all", path); err != nil {
		t.Fatalf("run(all): %v", err)
	}
	summary, _ := readJSONFile(t, path)["summary"].(map[string]any)

	cases := []struct {
		key  string
		want float64
	}{
		{key: "struct_missing_input", want: 7},
		{key: "struct_missing_output", want: 11},
		{key: "struct_extra_output", want: 13},
		{key: "action_missing_methods", want: 3},
		{key: "enum_missing_values", want: 17},
		{key: "enum_extra_values", want: 19},
	}
	for _, testCase := range cases {
		t.Run(testCase.key, func(t *testing.T) {
			if got, _ := summary[testCase.key].(float64); got != testCase.want {
				t.Errorf("summary[%q] = %v, want %v", testCase.key, summary[testCase.key], testCase.want)
			}
		})
	}
}

// TestMain_EndpointComparison_BuildsTheFetcherFromTheDocumentationFlags
// verifies the fetcher the endpoint comparison is handed was configured from
// the documentation flags, by asking it for an area nothing cached: offline, it
// must say so rather than reach for the network.
//
// -refresh and -offline are two booleans of one options struct that no test
// distinguished: every case asserted the fetcher was present and nothing about
// what it was built to do, so a run told to stay offline could have been
// handed one that downloads. The root is a planted directory, so the cache the
// fetcher derives from it holds nothing whatever this machine has fetched
// before.
func TestMain_EndpointComparison_BuildsTheFetcherFromTheDocumentationFlags(t *testing.T) {
	originalRoot, originalPaths := repositoryRoot, pathsRun
	t.Cleanup(func() { repositoryRoot, pathsRun = originalRoot, originalPaths })
	root := t.TempDir()
	repositoryRoot = func(string) (string, error) { return root, nil }
	var got paths.Options
	pathsRun = func(_ context.Context, _ string, opts paths.Options) ([]byte, bool, error) {
		got = opts
		return []byte("{}\n"), true, nil
	}

	messages := captureFatal(t)
	resetFlags(t, "-scope=paths", "-check-endpoints", "-offline",
		"-output", filepath.Join(t.TempDir(), "paths.json"))
	main()

	if len(*messages) != 0 {
		t.Fatalf("main reported %v, want a clean run", *messages)
	}
	if got.Fetcher == nil {
		t.Fatal("-check-endpoints was passed no fetcher")
	}
	_, err := got.Fetcher.Fetch(t.Context(), "an-area-nothing-cached")
	if err == nil || !strings.Contains(err.Error(), "not cached and offline") {
		t.Errorf("fetching an uncached area = %v, want the offline refusal", err)
	}
}

// TestRunValidateDocsMode_OfflineWithNothingCached_ReportsTheOfflineReason
// verifies the mode's own fetcher is built offline from its parameter: with a
// planted root whose one citation is cached nowhere, the run fails the gate and
// the report says the doc is not cached rather than naming a download failure.
//
// It is the other half of the ending this mode was given: a mode that passed
// refresh where offline belongs would leave that cache miss to the network, and
// the citation would still be reported stale, for a different reason and after
// six attempts at gitlab.com.
func TestRunValidateDocsMode_OfflineWithNothingCached_ReportsTheOfflineReason(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"cmd/audit_1to1/main.go": "package main // see doc/api/delta.md",
	})
	original := repositoryRoot
	t.Cleanup(func() { repositoryRoot = original })
	repositoryRoot = func(string) (string, error) { return root, nil }
	messages := captureFatal(t)

	output := filepath.Join(t.TempDir(), "docs.json")
	runValidateDocsMode(output, false, true, 0)

	if len(*messages) != 1 || !strings.Contains((*messages)[0], "stale doc/api citations found") {
		t.Fatalf("runValidateDocsMode reported %v, want the stale-citation gate", *messages)
	}
	stale, _ := readJSONFile(t, output)["stale"].([]any)
	if len(stale) != 1 {
		t.Fatalf("report stale = %v, want the one citation", stale)
	}
	entry, _ := stale[0].(map[string]any)
	if area, _ := entry["area"].(string); area != "delta" {
		t.Errorf("stale area = %v, want delta", entry["area"])
	}
	if reason, _ := entry["error"].(string); !strings.Contains(reason, "not cached and offline") {
		t.Errorf("stale reason = %v, want the offline refusal", entry["error"])
	}
}

// TestRunSingle_PathsScope_PassesTheEndpointFetcherThrough verifies the wiring
// of the dimension that reads the recorded requests: the flag decides whether
// the documentation comparison gets a fetcher at all, and the scope's own
// verdict is what the run reports.
//
// Nil is the interesting half. The comparison needs the network and 250 pages
// of it, and it gates nothing, so a run that did not ask for it must be handed
// nothing to fetch with rather than a fetcher it might use. The shard
// directory is the same shape of input one layer along: an end-to-end record
// exists only after a Docker session wrote one, so a run that named none must
// be handed the empty string and not a path it might read.
func TestRunSingle_PathsScope_PassesTheEndpointFetcherThrough(t *testing.T) {
	original := pathsRun
	t.Cleanup(func() { pathsRun = original })
	var got paths.Options
	pathsRun = func(_ context.Context, _ string, opts paths.Options) ([]byte, bool, error) {
		got = opts
		return []byte("{}\n"), true, nil
	}

	cases := []struct {
		name      string
		endpoints bool
		e2eCalls  string
		wantNil   bool
	}{
		{name: "without -check-endpoints", endpoints: false, wantNil: true},
		{name: "with -check-endpoints", endpoints: true, wantNil: false},
		{name: "with -e2e-calls", endpoints: false, e2eCalls: "/tmp/e2e-calls", wantNil: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got = paths.Options{}

			content, clean, err := runSingle(t.Context(), scopePaths,
				options{gapsOnly: true, endpoints: testCase.endpoints, e2eCalls: testCase.e2eCalls})

			if err != nil || !clean || string(content) != "{}\n" {
				t.Fatalf("runSingle(paths) = %q/%v/%v, want the seam's report and verdict", content, clean, err)
			}
			if !got.GapsOnly {
				t.Error("runSingle(paths) did not pass -gaps-only through")
			}
			if (got.Fetcher == nil) != testCase.wantNil {
				t.Errorf("runSingle(paths) fetcher = %v, want nil == %v", got.Fetcher, testCase.wantNil)
			}
			if got.E2ECallsDir != testCase.e2eCalls {
				t.Errorf("runSingle(paths) e2e calls = %q, want %q", got.E2ECallsDir, testCase.e2eCalls)
			}
		})
	}
}

// TestRunSingle_UnknownScope verifies the single-scope dispatcher refuses a
// scope name it does not know instead of running nothing silently, and that a
// refusal reports the gate as failed rather than clean.
func TestRunSingle_UnknownScope(t *testing.T) {
	_, clean, err := runSingle(t.Context(), "bogus", options{})
	if err == nil {
		t.Fatal("expected error for unknown scope")
	}
	if !strings.Contains(err.Error(), "unknown scope") {
		t.Errorf("error should mention 'unknown scope', got: %v", err)
	}
	if clean {
		t.Error("an unknown scope reported a clean gate")
	}
}

// TestIsMergedScope_Combinations_MatchOnlyTheMergedSet verifies the merged
// backlog runs for exactly the four merged streams. Adding the sdk scope made
// a count-based test wrong: structs,actions,metadata,sdk also has four
// entries and must not be merged.
func TestIsMergedScope_Combinations_MatchOnlyTheMergedSet(t *testing.T) {
	cases := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{name: "the_merged_set", scopes: []string{"actions", "enums", "metadata", "structs"}, want: true},
		{name: "old_trio_without_enums", scopes: []string{"actions", "metadata", "structs"}, want: false},
		{name: "set_with_sdk_substituted", scopes: []string{"actions", "metadata", "sdk", "structs"}, want: false},
		{name: "all_five", scopes: []string{"actions", "enums", "metadata", "sdk", "structs"}, want: false},
		{name: "single", scopes: []string{"sdk"}, want: false},
		{name: "empty", scopes: nil, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isMergedScope(tc.scopes); got != tc.want {
				t.Errorf("isMergedScope(%v) = %v, want %v", tc.scopes, got, tc.want)
			}
		})
	}
}

// readJSONFile decodes the JSON document at path into a generic map.
func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc map[string]any
	if unmarshalErr := json.Unmarshal(data, &doc); unmarshalErr != nil {
		t.Fatalf("%s is not JSON: %v", path, unmarshalErr)
	}
	return doc
}

// keysOf lists a decoded object's keys for failure messages.
func keysOf(doc map[string]any) []string {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestRun_ScopeSelection_RejectsUnsupportedScopes verifies the two refusals
// before any analyzer runs: an unknown scope name and a two-scope
// combination, which the merged backlog cannot represent.
func TestRun_ScopeSelection_RejectsUnsupportedScopes(t *testing.T) {
	cases := []struct {
		name    string
		scope   string
		wantErr string
	}{
		{name: "unknown_scope", scope: "bogus", wantErr: `invalid scope "bogus"`},
		{name: "two_scopes", scope: "structs,actions", wantErr: "other combinations are not supported"},
		{name: "the_old_trio_without_enums", scope: "structs,actions,metadata", wantErr: "other combinations are not supported"},
		{name: "four_scopes_that_are_not_the_merged_set", scope: "structs,actions,metadata,sdk", wantErr: "other combinations are not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runScope(tc.scope, filepath.Join(t.TempDir(), "out.json"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("run(%q) error = %v, want it to contain %q", tc.scope, err, tc.wantErr)
			}
		})
	}
}

// TestRun_EachScope_WritesItsReportShape runs the single-scope reports and
// the merged backlog through the seam against the real tree and checks each
// lands in its file in the analyzer's native shape: the struct report names
// the client-go path, the action report lists services, the metadata report
// lists packages, the enum report lists packages under the client-go path,
// the sdk report carries the enum fields beside its services, and the merged
// backlog carries the note and the per-stream summary. A write failure
// surfaces as such.
func TestRun_EachScope_WritesItsReportShape(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		scope    string
		wantKeys []string
	}{
		{name: "metadata", scope: "metadata", wantKeys: []string{"schema_version", "summary", "packages"}},
		{name: "structs", scope: "structs", wantKeys: []string{"schema_version", "client_go_path", "summary", "packages"}},
		{name: "actions", scope: "actions", wantKeys: []string{"schema_version", "client_go_path", "summary", "services"}},
		{name: "enums", scope: "enums", wantKeys: []string{"schema_version", "client_go_path", "summary", "packages"}},
		{name: "sdk", scope: "sdk", wantKeys: []string{"schema_version", "client_go_path", "summary", "services", "graphql_operations", "enum_fields"}},
		{name: "merged", scope: "all", wantKeys: []string{"schema_version", "note", "summary", "packages"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.scope+".json")
			if err := runScope(tc.scope, path); err != nil {
				t.Fatalf("run(%q): %v", tc.scope, err)
			}
			doc := readJSONFile(t, path)
			for _, key := range tc.wantKeys {
				if _, ok := doc[key]; !ok {
					t.Errorf("%s report lacks %q (keys %v)", tc.scope, key, keysOf(doc))
				}
			}
			if v, _ := doc["schema_version"].(float64); v != 1 {
				t.Errorf("%s schema_version = %v, want 1", tc.scope, doc["schema_version"])
			}
		})
	}

	t.Run("merged_summary_carries_every_stream", func(t *testing.T) {
		doc := readJSONFile(t, filepath.Join(dir, "all.json"))
		summary, _ := doc["summary"].(map[string]any)
		for _, key := range []string{"struct_missing_input", "action_missing_methods", "meta_generic_usage", "enum_missing_values", "enum_extra_values"} {
			t.Run(key, func(t *testing.T) {
				if _, ok := summary[key]; !ok {
					t.Errorf("merged summary lacks %q (keys %v)", key, keysOf(summary))
				}
			})
		}
		if note, _ := doc["note"].(string); !strings.HasPrefix(note, "Merged 1:1 audit backlog.") {
			t.Errorf("merged note = %q, want the backlog note", note)
		}
	})

	t.Run("write_failure", func(t *testing.T) {
		err := runScope("metadata", dir)
		if err == nil || !strings.HasPrefix(err.Error(), "write output: ") {
			t.Fatalf("run to a directory path = %v, want a write output error", err)
		}
	})
}

// TestValidateDocs_Outcomes_ReportAndGate verifies the -validate-docs seam:
// with every cited doc cached the report is written and the gate passes,
// with nothing cached the report is still written and the gate fails naming
// the stale citations, an unwritable output fails as a write error, and a
// root without the auditor source fails the scan itself.
func TestValidateDocs_Outcomes_ReportAndGate(t *testing.T) {
	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	areas, err := scanDocCitations(root)
	if err != nil {
		t.Fatalf("scanDocCitations: %v", err)
	}
	seededCache := func(t *testing.T) string {
		t.Helper()
		cache := t.TempDir()
		for _, a := range areas {
			if writeErr := os.WriteFile(filepath.Join(cache, a+".md"), []byte("# "+a), 0o600); writeErr != nil {
				t.Fatal(writeErr)
			}
		}
		return cache
	}
	reportFile := func(t *testing.T) string {
		t.Helper()
		return filepath.Join(t.TempDir(), "docs.json")
	}

	cases := []struct {
		name       string
		root       string
		cache      func(t *testing.T) string
		output     func(t *testing.T) string
		wantErr    string
		wantReport bool
	}{
		{name: "all_cached_passes", root: root, cache: seededCache, output: reportFile, wantReport: true},
		{
			name: "nothing_cached_fails_the_gate", root: root,
			cache: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			}, output: reportFile,
			wantErr: "stale doc/api citations found", wantReport: true,
		},
		{
			name: "unwritable_output", root: root, cache: seededCache,
			output: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			},
			wantErr: "write output: ",
		},
		{
			name: "root_without_auditor_source", root: filepath.Join(t.TempDir(), "absent"),
			cache: func(t *testing.T) string {
				t.Helper()
				return t.TempDir()
			}, output: reportFile,
			wantErr: "scan doc citations: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := apidocs.New(tc.root, apidocs.Options{Offline: true, CacheDir: tc.cache(t)})
			output := tc.output(t)
			validateErr := validateDocs(context.Background(), tc.root, output, fetcher)
			if tc.wantErr == "" {
				if validateErr != nil {
					t.Fatalf("validateDocs: %v", validateErr)
				}
			} else if validateErr == nil || !strings.Contains(validateErr.Error(), tc.wantErr) {
				t.Fatalf("validateDocs error = %v, want it to contain %q", validateErr, tc.wantErr)
			}
			if !tc.wantReport {
				return
			}
			doc := readJSONFile(t, output)
			if checked, _ := doc["checked"].(float64); int(checked) != len(areas) {
				t.Errorf("report checked = %v, want %d cited areas", doc["checked"], len(areas))
			}
		})
	}
}
