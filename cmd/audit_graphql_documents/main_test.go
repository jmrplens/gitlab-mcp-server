package main

import (
	"bytes"
	"cmp"
	"errors"
	"go/token"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// afterThePin is a day later than the committed provenance record, so the age
// line a live run prints is a number rather than a negative one.
func afterThePin() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }

// soundFixture holds documents the pinned schema accepts, written the way the
// repository writes them: a named constant, and one assembled from a shared
// fragment.
const soundFixture = "package sound\n\n" +
	"const vulnFields = `\n      id\n      title\n      severity\n`\n\n" +
	"const getVulnerability = `\nquery($id: VulnerabilityID!) {\n  vulnerability(id: $id) {` + vulnFields + `\n  }\n}\n`\n"

// brokenFixture holds the two shapes GitLab refuses that no test would ever
// catch on its own: a field the type does not have, and an argument the field
// does not accept.
const brokenFixture = "package broken\n\n" +
	"const listVulnerabilities = `\nquery($path: ID!, $severity: [String!]) {\n" +
	"  project(fullPath: $path) {\n    vulnerabilities(severity: $severity) {\n" +
	"      nodes {\n        id\n        hasSolutions\n      }\n    }\n  }\n}\n`\n"

// fixtureModule writes a throwaway module holding one package per entry and
// returns its root. An entry may name a nested directory, such as
// internal/sound, which is where the command itself looks.
//
// A module of its own rather than an overlay over this repository, because
// what this file tests is the command around the audit: the exit status, the
// two streams and the shape of a finding. The reading of the source is
// cmd/internal/graphqldocs's own business and is tested there against the
// harder shapes.
func fixtureModule(t *testing.T, packages map[string]string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}
	for name, source := range packages {
		dir := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, path.Base(name)+".go"), []byte(source), 0o600); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
	}
	return root
}

// runFixture runs the audit over a fixture module and returns the exit status
// with both streams.
func runFixture(t *testing.T, packages map[string]string, verbose bool) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      fixtureModule(t, packages),
		patterns: []string{"./..."},
		verbose:  verbose,
	}, &out, &errOut)
	return status, out.String(), errOut.String()
}

// mainArgsEnv tells a test binary to be the command instead of its tests. When
// it is set, [TestMain] runs main with the arguments it carries, one per line,
// and the process ends with whatever main exits with.
const mainArgsEnv = "AUDIT_GRAPHQL_DOCUMENTS_MAIN_ARGS"

// mainReturnedStatus is what that process ends with when main returns instead
// of exiting. The command's own statuses are 0 and 1, so a run ending with
// this one names the defect rather than passing for either of them.
const mainReturnedStatus = 3

// TestMain runs the tests, or, in a process [runMainProcess] started, the
// command itself. Deciding it here rather than inside a test means the process
// that is the command runs no test machinery first, so nothing the testing
// package prints can reach the two streams the tests read.
func TestMain(m *testing.M) {
	if args, child := os.LookupEnv(mainArgsEnv); child {
		os.Args = append([]string{"audit_graphql_documents"}, strings.Split(args, "\n")...)
		main()
		os.Exit(mainReturnedStatus)
	}
	os.Exit(m.Run())
}

// runMainProcess starts this test binary again as the command, with args as its
// command line and extraEnv over an environment holding neither GITLAB_URL nor
// GITLAB_TOKEN, and returns the status it exited with and both of its streams.
//
// A process rather than a call, because main is the one function here that
// reads the process itself: the flags, the environment, the two real streams
// and the exit status, which is all `make check-graphql-documents` and CI read.
// Each pair it hands over looks alike to the compiler and means the opposite
// (-live and -schema, the token and the reason it was withheld, GITLAB_URL and
// GITLAB_TOKEN, stdout and stderr), so the tests below give the two members of
// each values nothing could mistake for each other.
func runMainProcess(t *testing.T, extraEnv []string, args ...string) (int, string, string) {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+len(extraEnv)+2)
	for _, entry := range os.Environ() {
		switch name, _, _ := strings.Cut(entry, "="); name {
		case "GITLAB_URL", "GITLAB_TOKEN", "GOCOVERDIR", mainArgsEnv:
		default:
			env = append(env, entry)
		}
	}
	// A binary built for coverage writes its counters where GOCOVERDIR says,
	// and warns on stderr when nothing says, which is a stream this test reads.
	env = append(env, mainArgsEnv+"="+strings.Join(args, "\n"), "GOCOVERDIR="+t.TempDir())

	// #nosec G204 G702 -- the program is this test binary, started again as the command it tests
	command := exec.CommandContext(t.Context(), os.Args[0])
	command.Env = append(env, extraEnv...)
	var out, errOut bytes.Buffer
	command.Stdout, command.Stderr = &out, &errOut

	status := 0
	var exited *exec.ExitError
	if err := command.Run(); errors.As(err, &exited) {
		status = exited.ExitCode()
	} else if err != nil {
		t.Fatalf("start the command: %v", err)
	}
	return status, out.String(), errOut.String()
}

// TestMain_ThePinnedRun_ReportsOnItsOwnStreamsAndExitsWithRunsStatus verifies
// the command line `make check-graphql-documents` runs: -dir and -v reach the
// run, only ./internal/... is read, the listing and the findings reach stdout
// and stderr respectively, and a refusal is the process's exit status.
func TestMain_ThePinnedRun_ReportsOnItsOwnStreamsAndExitsWithRunsStatus(t *testing.T) {
	root := fixtureModule(t, map[string]string{
		"internal/sound":  soundFixture,
		"internal/broken": brokenFixture,
		// Outside ./internal/..., so a gate that read more than the tree this
		// server's documents live in would count it and refuse it.
		"stray": strings.Replace(brokenFixture, "package broken", "package stray", 1),
	})

	status, out, errOut := runMainProcess(t, nil, "-dir", root, "-v")

	if status != 1 {
		t.Fatalf("exit status %d, want 1: one document under internal/ is refused.\nstdout:\n%s\nstderr:\n%s", status, out, errOut)
	}
	for _, testCase := range []struct{ name, stream, want string }{
		{name: "the finding", stream: errOut, want: "fixture/internal/broken listVulnerabilities (internal/broken/broken.go:"},
		{name: "the summary, counting internal/ alone", stream: errOut, want: "refuses 1 of 2 document(s)"},
		{name: "the listing", stream: out, want: "    ok  fixture/internal/sound getVulnerability\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if !strings.Contains(testCase.stream, testCase.want) {
				t.Errorf("the stream does not contain %q:\n%s", testCase.want, testCase.stream)
			}
		})
	}
	if strings.Contains(out, "listVulnerabilities") || strings.Contains(errOut, "    ok  ") {
		t.Errorf("a finding and the listing reached each other's stream.\nstdout:\n%s\nstderr:\n%s", out, errOut)
	}
}

// TestMain_SchemaFlag_JudgesByTheFileItNames verifies that -schema reaches the
// field that reads a file and a clean run exits 0. -live is the other flag
// naming a schema, and a path handed to it would be fetched as a URL and fail.
func TestMain_SchemaFlag_JudgesByTheFileItNames(t *testing.T) {
	narrow := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrow, []byte("type Query {\n  ok: Boolean\n}\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	status, out, errOut := runMainProcess(t, nil,
		"-dir", fixtureModule(t, map[string]string{"internal/ok": okFixture}), "-schema", narrow)

	if status != 0 {
		t.Fatalf("exit status %d, want 0.\nstderr:\n%s", status, errOut)
	}
	if want := " types from " + narrow + ", not the pinned schema)\n"; !strings.Contains(out, "1 document(s) accepted (") || !strings.Contains(out, want) {
		t.Errorf("stdout does not accept the document by %q:\n%s", want, out)
	}
}

// TestMain_LiveFlag_HandsGITLABTOKENOnlyToTheInstanceGITLABURLNames verifies
// the credential main resolves before anything runs: sent to the instance
// GITLAB_URL names, withheld from any other with the reason in the report.
// The token, the reason, and the two variables they come from are all
// strings, so each is looked for where only the right one would arrive.
func TestMain_LiveFlag_HandsGITLABTOKENOnlyToTheInstanceGITLABURLNames(t *testing.T) {
	cases := []struct {
		name, instance, wantAuthorization, wantReport string
	}{
		{
			name:              "the instance GITLAB_URL names",
			wantAuthorization: "Bearer secret",
			wantReport:        "(GitLab 19.4.0-ee), fetched now, not the pinned schema)",
		},
		{
			name:       "an instance GITLAB_URL does not name",
			instance:   "https://gitlab.example.com",
			wantReport: "(GitLab unknown), fetched now, not the pinned schema; GITLAB_TOKEN belongs to https://gitlab.example.com and this run asks ",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			endpoint, authorization := answeringInstanceRecordingAuthorization(t)
			instance := cmp.Or(testCase.instance, endpoint)

			status, out, errOut := runMainProcess(t, []string{"GITLAB_URL=" + instance, "GITLAB_TOKEN=secret"},
				"-dir", fixtureModule(t, map[string]string{"internal/ok": okFixture}), "-live", endpoint)

			if status != 0 {
				t.Fatalf("exit status %d, want 0.\nstderr:\n%s", status, errOut)
			}
			if authorization() != testCase.wantAuthorization {
				t.Errorf("the instance was offered %q, want %q", authorization(), testCase.wantAuthorization)
			}
			if !strings.Contains(out, testCase.wantReport) {
				t.Errorf("stdout does not contain %q:\n%s", testCase.wantReport, out)
			}
		})
	}
}

// TestRun_AgainstASchemaTheCallerSupplies_JudgesByThatSchema verifies the entry
// the live re-probe uses.
//
// The pin can only report a document that was already broken when it was taken,
// never one GitLab has narrowed since, which is how every defect this gate was
// built for arose. Handing the audit a schema fetched today closes that, so the
// case that matters is a document the pin accepts and the supplied schema does
// not, and a summary that says which of the two judged it.
//
// The drift report is asserted on the same run because that refusal is the run
// it is for: a reader told a document broke asks next what moved under it, so
// the report has to arrive whether or not a document was refused, naming the
// field the supplied schema no longer has.
func TestRun_AgainstASchemaTheCallerSupplies_JudgesByThatSchema(t *testing.T) {
	narrowed := filepath.Join(t.TempDir(), "narrow.graphql")
	if err := os.WriteFile(narrowed, []byte("type Query {\n  ok: Boolean\n}\n"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:        fixtureModule(t, map[string]string{"sound": soundFixture}),
		patterns:   []string{"./..."},
		schemaPath: narrowed,
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1: the supplied schema has no vulnerability field.\nstdout:\n%s", status, out.String())
	}
	if want := " types from " + narrowed + ", not the pinned schema)\n"; !strings.Contains(errOut.String(), want) {
		t.Errorf("the summary does not say which schema judged the documents, want %q:\n%s", want, errOut.String())
	}
	if want := "    Query.vulnerability: the pin has it, the live schema does not\n"; !strings.Contains(out.String(), want) {
		t.Errorf("a run that refused a document reported no drift, want %q:\n%s", want, out.String())
	}
}

// okFixture holds one document the smallest possible instance accepts, so a run
// against a fetched schema can be judged by that schema rather than by the pin.
const okFixture = "package ok\n\nconst queryOk = `query { ok }`\n"

// TestRun_AgainstAnInstanceItIntrospects_JudgesByWhatThatInstanceServes
// verifies the mode the scheduled job runs.
//
// It is the one check the pin cannot perform. The pin says the documents were
// valid on gitlab.com on the day it was taken; this says they are valid on the
// GitLab an instance is serving now, and it reports where the two disagree
// about something the documents touch, which is how the pin's age becomes a
// number somebody sees rather than an assumption.
func TestRun_AgainstAnInstanceItIntrospects_JudgesByWhatThatInstanceServes(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:      fixtureModule(t, map[string]string{"ok": okFixture}),
		patterns: []string{"./..."},
		live:     answeringInstance(t, introspectionAnswer(queryOnly)),
		now:      afterThePin,
	}, &out, &errOut)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut.String())
	}
	for _, want := range []string{
		"fetched now, not the pinned schema",
		"the pin and the live schema disagree on",
		"Query.ok: the live schema has it, the pin does not",
		"day(s) ago",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out.String(), want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out.String())
			}
		})
	}
}

// answeringInstanceRecordingAuthorization is an instance that answers
// introspection, keeps whatever credential it was offered, and names its
// version only to a caller that offered one, which is what GitLab does.
func answeringInstanceRecordingAuthorization(t *testing.T) (endpoint string, authorization func() string) {
	t.Helper()
	var mutex sync.Mutex
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offered := r.Header.Get("Authorization")
		if offered != "" {
			mutex.Lock()
			seen = offered
			mutex.Unlock()
		}
		// Read to the end rather than once into a ContentLength-sized
		// buffer: a single Read may return fewer bytes than it was given,
		// and a short one that stopped before "metadata" would route the
		// version query to the introspection answer.
		payload, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading the request body: %v", readErr)
			http.Error(w, "unreadable body", http.StatusInternalServerError)

			return
		}
		switch {
		case !strings.Contains(string(payload), "metadata"):
			_, _ = w.Write([]byte(introspectionAnswer(queryOnly)))
		case offered == "":
			_, _ = w.Write([]byte(`{"data":{"metadata":null}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"metadata":{"version":"19.4.0-ee","revision":"abc1234"}}}`))
		}
	}))
	t.Cleanup(server.Close)
	return server.URL, func() string {
		mutex.Lock()
		defer mutex.Unlock()
		return seen
	}
}

// TestRun_ACredentialThisRunHolds_ReachesTheInstanceOrIsExplained verifies the
// two strings a live run carries about its credential. They look alike in the
// configuration and mean opposite things: one is sent to the instance, the
// other is the reason nothing was. A run that confused them would offer the
// explanation as a bearer token and print the token as an explanation, and a
// report that only ever says "GitLab unknown" cannot tell the two apart.
func TestRun_ACredentialThisRunHolds_ReachesTheInstanceOrIsExplained(t *testing.T) {
	const withheld = "GITLAB_TOKEN belongs to https://gitlab.com and this run asks elsewhere"

	t.Run("a token this run may send", func(t *testing.T) {
		endpoint, authorization := answeringInstanceRecordingAuthorization(t)

		var out, errOut bytes.Buffer
		status := run(auditRun{
			dir:      fixtureModule(t, map[string]string{"ok": okFixture}),
			patterns: []string{"./..."},
			live:     endpoint,
			token:    "secret",
			now:      afterThePin,
		}, &out, &errOut)

		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut.String())
		}
		if authorization() != "Bearer secret" {
			t.Errorf("the instance was offered %q, want the credential this run was given", authorization())
		}
		if !strings.Contains(out.String(), "GitLab 19.4.0-ee") {
			t.Errorf("the report does not name the version that credential bought:\n%s", out.String())
		}
	})

	t.Run("a token this run withholds", func(t *testing.T) {
		endpoint, authorization := answeringInstanceRecordingAuthorization(t)

		var out, errOut bytes.Buffer
		status := run(auditRun{
			dir:           fixtureModule(t, map[string]string{"ok": okFixture}),
			patterns:      []string{"./..."},
			live:          endpoint,
			tokenWithheld: withheld,
			now:           afterThePin,
		}, &out, &errOut)

		if status != 0 {
			t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut.String())
		}
		if authorization() != "" {
			t.Errorf("the instance was offered %q by a run that withheld its token", authorization())
		}
		for _, want := range []string{"GitLab unknown", withheld} {
			t.Run(want, func(t *testing.T) {
				if !strings.Contains(out.String(), want) {
					t.Errorf("the report does not contain %q:\n%s", want, out.String())
				}
			})
		}
	})
}

// TestRun_AnInstanceThatCannotBeReached_FailsWithoutFallingBackToThePin
// verifies that a re-probe whose instance never answered stops rather than
// judging by the pin, which would report a pass for a question nobody asked.
func TestRun_AnInstanceThatCannotBeReached_FailsWithoutFallingBackToThePin(t *testing.T) {
	unreachable := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable.Close()

	var out, errOut bytes.Buffer
	status := run(auditRun{
		dir:      fixtureModule(t, map[string]string{"ok": okFixture}),
		patterns: []string{"./..."},
		live:     unreachable.URL,
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "ask ") {
		t.Errorf("stderr does not say the instance could not be asked:\n%s", errOut.String())
	}
	if strings.Contains(out.String(), "document(s) accepted") {
		t.Errorf("the run reported documents accepted after the fetch failed:\n%s", out.String())
	}
}

// TestRun_BothSchemaSourcesAtOnce_IsRefused verifies that a run naming two
// schemas is refused rather than silently preferring one. Which of the two
// judged the documents is the whole meaning of the result, so a run that cannot
// say must not produce one.
func TestRun_BothSchemaSourcesAtOnce_IsRefused(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{
		dir:        fixtureModule(t, map[string]string{"ok": okFixture}),
		patterns:   []string{"./..."},
		live:       "https://gitlab.example.com/api/graphql",
		schemaPath: filepath.Join(t.TempDir(), "unused.graphql"),
	}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "pass one") {
		t.Errorf("stderr does not explain the refusal:\n%s", errOut.String())
	}
}

// TestRun_ASuppliedSchemaThatCannotBeUsed_Fails verifies that a live re-probe
// whose schema never arrived stops rather than falling back to the pin, which
// would report a pass for a question nobody asked.
func TestRun_ASuppliedSchemaThatCannotBeUsed_Fails(t *testing.T) {
	unparseable := filepath.Join(t.TempDir(), "prose.graphql")
	if err := os.WriteFile(unparseable, []byte("this is prose, not a schema"), 0o600); err != nil {
		t.Fatalf("prepare the fixture: %v", err)
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "no such file", path: filepath.Join(t.TempDir(), "absent.graphql"), want: "read the schema to judge against"},
		{name: "not a schema", path: unparseable, want: unparseable + ": parse the schema"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var out, errOut bytes.Buffer

			status := run(auditRun{
				dir:        fixtureModule(t, map[string]string{"sound": soundFixture}),
				patterns:   []string{"./..."},
				schemaPath: testCase.path,
			}, &out, &errOut)

			if status != 1 {
				t.Fatalf("exit status %d, want 1", status)
			}
			if !strings.Contains(errOut.String(), testCase.want) {
				t.Errorf("stderr does not explain the failure %q:\n%s", testCase.want, errOut.String())
			}
		})
	}
}

// TestRun_DocumentsThePinnedSchemaAccepts_Succeeds verifies the passing path,
// including that the summary names the pin so a reader is told how old the
// judgement is.
func TestRun_DocumentsThePinnedSchemaAccepts_Succeeds(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"sound": soundFixture}, false)

	if status != 0 {
		t.Fatalf("exit status %d, want 0. stderr:\n%s", status, errOut)
	}
	for _, want := range []string{"document(s) accepted", "gitlab.com", "retrieved"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(out, want) {
				t.Errorf("stdout does not contain %q:\n%s", want, out)
			}
		})
	}
	if errOut != "" {
		t.Errorf("a clean run wrote to stderr:\n%s", errOut)
	}
}

// TestRun_Verbose_ListsWhatItAccepted verifies that the set a reviewer has to
// care about is reviewable rather than a count, and that the listing names only
// what passed: a document printed as accepted and refused in the same run would
// be worse than either line alone.
// The accepted line is compared whole, because its two halves are both names of
// the same document and a listing that printed them the other way round would
// still contain each of them.
func TestRun_Verbose_ListsWhatItAccepted(t *testing.T) {
	status, out, errOut := runFixture(t, map[string]string{"sound": soundFixture, "broken": brokenFixture}, true)

	if status != 1 {
		t.Fatalf("exit status %d, want 1: the broken fixture is refused", status)
	}
	if !strings.Contains(out, "    ok  fixture/sound getVulnerability\n") {
		t.Errorf("the verbose run does not name the package and the document it checked:\n%s", out)
	}
	if strings.Contains(out, "listVulnerabilities") {
		t.Errorf("the verbose run listed a refused document as accepted:\n%s", out)
	}
	// The two counts are the refused documents and every document, in that
	// order. A fixture with one of each cannot tell them apart, so the summary
	// is asserted here, where the run carries two documents and one refusal.
	if !strings.Contains(errOut, "refuses 1 of 2 document(s)") {
		t.Errorf("the summary does not count the refusals against every document checked:\n%s", errOut)
	}
}

// TestRun_ARelativeDir_StillTrimsTheFindingsToIt verifies the one thing the
// audit does with `-dir` besides handing it to the loader: findings come out of
// the loader positioned absolutely and with every symlink resolved, so the root
// they are trimmed against is made both first, whatever the flag was written
// as.
//
// Both halves of that are asserted because absolute alone passes here on Linux
// and fails on macOS, where a temp directory is reached through /var and read
// back through /private/var. The absence check names both spellings for the
// same reason: against the unresolved one alone it would pass on macOS while
// the finding still carried the resolved one.
//
// It is also what pins the branch beside it. `filepath.Abs` fails only when the
// working directory cannot be resolved, and a relative `-dir` is the only shape
// that asks it: with an absolute one it returns before it ever looks. So the
// failure arm is unreachable from here (a run whose working directory had gone
// could not have loaded the package this finding names) and this is the arm
// that does run.
func TestRun_ARelativeDir_StillTrimsTheFindingsToIt(t *testing.T) {
	root := fixtureModule(t, map[string]string{"broken": brokenFixture})
	t.Chdir(filepath.Dir(root))

	var out, errOut bytes.Buffer
	status := run(auditRun{dir: filepath.Base(root), patterns: []string{"./..."}}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "(broken/broken.go:") {
		t.Errorf("the finding is not trimmed to the relative root:\n%s", errOut.String())
	}
	for _, absolute := range rootSpellings(t, root) {
		if strings.Contains(errOut.String(), absolute) {
			t.Errorf("the finding still carries the absolute path %q:\n%s", absolute, errOut.String())
		}
	}
}

// rootSpellings returns every absolute path the fixture root can be named by:
// the one [testing.T.TempDir] handed out and, where they differ, the one every
// symlink resolves to. macOS is where they differ, /var being a link to
// /private/var, and asserting against one spelling there says nothing about
// the other.
func rootSpellings(t *testing.T, root string) []string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved == root {
		return []string{root}
	}

	return []string{root, resolved}
}

// TestRun_ADocumentGitLabWouldRefuse_Fails verifies the finding: a non-zero
// exit, the constant named, the file it is declared in, and every reason
// underneath.
func TestRun_ADocumentGitLabWouldRefuse_Fails(t *testing.T) {
	status, _, errOut := runFixture(t, map[string]string{"broken": brokenFixture}, false)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	for _, want := range []string{
		"listVulnerabilities",
		"broken/broken.go:",
		`Cannot query field "hasSolutions"`,
		`used in position expecting type "[VulnerabilitySeverity!]"`,
		"refuses 1 of 1 document(s)",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not contain %q:\n%s", want, errOut)
			}
		})
	}
}

// TestRun_NothingToCheck_IsAFailure verifies the guard against an audit
// pointed at the wrong tree: finding no documents at all means the audit is
// looking somewhere the documents are not, which must not read as a pass.
func TestRun_NothingToCheck_IsAFailure(t *testing.T) {
	const noDocuments = "package empty\n\nconst notADocument = \"there is no GraphQL here\"\n"

	status, _, errOut := runFixture(t, map[string]string{"empty": noDocuments}, false)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut, "no GraphQL documents were found") {
		t.Errorf("stderr does not report the empty result:\n%s", errOut)
	}
}

// TestRun_SourceThatCannotBeLoaded_Fails verifies that the audit stops rather
// than reporting a clean run over source it could not read.
func TestRun_SourceThatCannotBeLoaded_Fails(t *testing.T) {
	var out, errOut bytes.Buffer

	status := run(auditRun{dir: t.TempDir(), patterns: []string{"./..."}}, &out, &errOut)

	if status != 1 {
		t.Fatalf("exit status %d, want 1", status)
	}
	if !strings.Contains(errOut.String(), "audit_graphql_documents:") {
		t.Errorf("stderr does not name the command:\n%s", errOut.String())
	}
}

// TestFinding_EveryReason_IsListedUnderTheDocument verifies that a finding
// names the document once and every objection under it, including the case
// where the audit could not judge the document at all and the single line it
// has must survive.
func TestFinding_EveryReason_IsListedUnderTheDocument(t *testing.T) {
	cases := []struct {
		name    string
		reasons []string
		want    string
	}{
		{name: "a refusal", reasons: []string{"first", "second"}, want: "    - first\n    - second\n"},
		{name: "anything else", reasons: []string{"the pin is corrupt"}, want: "    - the pin is corrupt\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report := finding(nil, graphqldocs.Refusal{
				Document: graphqldocs.Document{Package: "x/y", Name: "queryThing"},
				Reasons:  testCase.reasons,
			})

			if !strings.HasPrefix(report, "x/y queryThing (") {
				t.Errorf("the report does not name the document:\n%s", report)
			}
			if !strings.HasSuffix(report, testCase.want) {
				t.Errorf("the report does not end with %q:\n%s", testCase.want, report)
			}
		})
	}
}

// TestRelative_PositionsUnderTheRoot_AreTrimmed verifies that a finding reads
// as a path a person can open, and that a position outside the root is left
// whole rather than turned into a walk of parent directories.
func TestRelative_PositionsUnderTheRoot_AreTrimmed(t *testing.T) {
	cases := []struct {
		name     string
		position token.Position
		roots    []string
		want     string
	}{
		{
			name:     "under the root",
			position: token.Position{Filename: filepath.Join("/repo", "internal", "tools", "x.go"), Line: 12},
			roots:    []string{"/repo"},
			want:     "internal/tools/x.go:12",
		},
		{
			name:     "outside the root",
			position: token.Position{Filename: filepath.Join("/elsewhere", "x.go"), Line: 3},
			roots:    []string{"/repo"},
			want:     filepath.Join("/elsewhere", "x.go") + ":3",
		},
		{
			name:     "no root to trim against",
			position: token.Position{Filename: filepath.Join("/repo", "x.go"), Line: 1},
			roots:    nil,
			want:     filepath.Join("/repo", "x.go") + ":1",
		},
		{
			name:     "a filename that is not a path under the root",
			position: token.Position{Filename: "relative.go", Line: 7},
			roots:    []string{"/repo"},
			want:     "relative.go:7",
		},
		{
			// The two spellings of one directory, which is what macOS and
			// Windows each produce in the opposite order. Trimming against
			// the first alone would leave the finding absolute.
			name:     "under the second spelling of the root",
			position: token.Position{Filename: filepath.Join("/private", "repo", "x.go"), Line: 4},
			roots:    []string{"/repo", filepath.Join("/private", "repo")},
			want:     "x.go:4",
		},
		{
			name:     "an empty spelling among the roots is skipped",
			position: token.Position{Filename: filepath.Join("/repo", "x.go"), Line: 5},
			roots:    []string{"", "/repo"},
			want:     "x.go:5",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := relative(testCase.position, testCase.roots); got != testCase.want {
				t.Errorf("relative() = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestTrimRoots_ARelativeDir_OffersEverySpellingOfIt verifies the list a
// finding is trimmed against: absolute always, and the resolved form beside it
// only where the two differ.
//
// The second is not decoration. The loader positions a finding with symlinks
// resolved, and the two platforms that expose it disagree about which spelling
// wins, so offering one and guessing which leaves the finding absolute on the
// other. A duplicate is left out rather than offered twice, since a second
// identical root can only ever repeat the first one's answer.
func TestTrimRoots_ARelativeDir_OffersEverySpellingOfIt(t *testing.T) {
	t.Run("a directory reached through a link offers the resolved spelling first", func(t *testing.T) {
		parent := t.TempDir()
		target := filepath.Join(parent, "target")
		if err := os.Mkdir(target, 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		link := filepath.Join(parent, "link")
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("this platform will not make a symbolic link here: %v", err)
		}
		resolved, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) error = %v", target, err)
		}

		got := trimRoots(link)

		if want := []string{resolved, link}; !slices.Equal(got, want) {
			t.Errorf("trimRoots(%q) = %v, want %v", link, got, want)
		}
	})

	t.Run("a directory that is not a link offers one spelling", func(t *testing.T) {
		dir := t.TempDir()
		resolved, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q) error = %v", dir, err)
		}

		got := trimRoots(resolved)

		if len(got) != 1 || got[0] != resolved {
			t.Errorf("trimRoots(%q) = %v, want just the one absolute spelling", resolved, got)
		}
	})

	t.Run("a relative dir is made absolute", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)

		got := trimRoots(".")

		if len(got) == 0 || !filepath.IsAbs(got[0]) {
			t.Errorf("trimRoots(\".\") = %v, want an absolute root first", got)
		}
	})
}

// TestTrimRoots_ASpellingThatCannotBeHad_FallsBackToOneThatCan verifies the two
// fallbacks, each of which keeps a finding readable when a spelling cannot be
// had: a directory that cannot be resolved is still offered absolute, and a
// working directory that is gone leaves -dir as it was written rather than
// offering nothing to trim against.
func TestTrimRoots_ASpellingThatCannotBeHad_FallsBackToOneThatCan(t *testing.T) {
	t.Run("a directory that cannot be resolved is offered absolute alone", func(t *testing.T) {
		absent := filepath.Join(t.TempDir(), "absent")

		got := trimRoots(absent)

		if want := []string{absent}; !slices.Equal(got, want) {
			t.Errorf("trimRoots(%q) = %v, want %v", absent, got, want)
		}
	})

	t.Run("a working directory that is gone leaves the flag as written", func(t *testing.T) {
		gone := filepath.Join(t.TempDir(), "gone")
		if err := os.Mkdir(gone, 0o750); err != nil {
			t.Fatalf("prepare the fixture: %v", err)
		}
		t.Chdir(gone)
		if err := os.Remove(gone); err != nil {
			t.Skipf("this platform keeps a working directory from being removed: %v", err)
		}
		if _, err := os.Getwd(); err == nil {
			t.Skip("this platform still names a removed working directory, so filepath.Abs cannot fail here")
		}

		got := trimRoots("relative")

		if want := []string{"relative"}; !slices.Equal(got, want) {
			t.Errorf("trimRoots(%q) = %v, want %v", "relative", got, want)
		}
	})
}
