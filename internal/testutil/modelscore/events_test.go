package modelscore

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelcorpus"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/modelrecord"
	gitlabtools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actioncatalog"
	dynamictools "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/dynamic"
)

// Reading a shard, and reading one call.

// lines wraps record payloads into the envelopes a shard holds.
func lines(payloads ...any) []modelrecord.Record {
	records := make([]modelrecord.Record, 0, len(payloads))
	for _, payload := range payloads {
		record := modelrecord.Record{Schema: modelrecord.SchemaVersion}
		switch typed := payload.(type) {
		case modelrecord.Run:
			record.Type, record.Run = modelrecord.TypeRun, &typed
		case modelrecord.Session:
			record.Type, record.Session = modelrecord.TypeSession, &typed
		case modelrecord.Attempt:
			record.Type, record.Attempt = modelrecord.TypeAttempt, &typed
		case modelrecord.Turn:
			record.Type, record.Turn = modelrecord.TypeTurn, &typed
		case modelrecord.Call:
			record.Type, record.Call = modelrecord.TypeCall, &typed
		case modelrecord.Verify:
			record.Type, record.Verify = modelrecord.TypeVerify, &typed
		}
		records = append(records, record)
	}
	return records
}

// TestAttempts_JoinsEveryLineToItsAttemptAndItsRun reads a shard in the order a
// run really writes one.
//
// The run and session lines come last on purpose: a run writes its attempt
// lines as each subtest ends and its run and session lines from an exit hook,
// so a reader that joined in one pass would find neither.
func TestAttempts_JoinsEveryLineToItsAttemptAndItsRun(t *testing.T) {
	records := lines(
		modelrecord.Attempt{ID: "a1", Case: "MT-002", Session: "dynamic", Surface: "dynamic"},
		modelrecord.Call{Attempt: "a1", Index: 1, Tool: "gitlab_execute_action"},
		modelrecord.Turn{Attempt: "a1", Index: 1},
		modelrecord.Attempt{ID: "a2", Case: "MT-004", Session: "dynamic", Surface: "dynamic"},
		modelrecord.Verify{Attempt: "a2", Name: "the project is starred", Passed: true},
		modelrecord.Session{Label: "dynamic", Surface: "dynamic", Mode: ModeDefault},
		modelrecord.Run{RunID: "run-1", Tier: "ultimate"},
	)

	attempts, err := Attempts(records)
	if err != nil {
		t.Fatalf("Attempts: %v", err)
	}

	if len(attempts) != 2 {
		t.Fatalf("Attempts returned %d, want the two attempt lines", len(attempts))
	}
	first := attempts[0]
	if first.Run.RunID != "run-1" || first.Session.Label != "dynamic" {
		t.Errorf("attempt 1 joined run %q and session %q, want run-1 and dynamic",
			first.Run.RunID, first.Session.Label)
	}
	if len(first.Calls) != 1 || len(first.Turns) != 1 {
		t.Errorf("attempt 1 holds %d call(s) and %d turn(s), want one of each",
			len(first.Calls), len(first.Turns))
	}
	if len(attempts[1].Verifies) != 1 {
		t.Errorf("attempt 2 holds %d verify line(s), want one", len(attempts[1].Verifies))
	}
	if first.tier() != "ultimate" {
		t.Errorf("tier() = %q, want the run's own", first.tier())
	}
}

// TestAttempts_ARecordItCannotJoin_IsAnError covers every shape that would
// otherwise be scored with a hole in it.
func TestAttempts_ARecordItCannotJoin_IsAnError(t *testing.T) {
	cases := []struct {
		name    string
		records []modelrecord.Record
		names   string
	}{
		{
			name:    "no run line, so the tier is unknown",
			records: lines(modelrecord.Attempt{ID: "a1"}),
			names:   modelrecord.TypeRun,
		},
		{
			name: "two runs in one shard",
			records: lines(
				modelrecord.Run{RunID: "run-1", Tier: "free"},
				modelrecord.Run{RunID: "run-2", Tier: "ultimate"},
			),
			names: "run-2",
		},
		{
			name: "two attempts with one identifier",
			records: lines(
				modelrecord.Run{RunID: "run-1", Tier: "free"},
				modelrecord.Attempt{ID: "a1"},
				modelrecord.Attempt{ID: "a1"},
			),
			names: "a1",
		},
		{
			name: "a call belonging to an attempt the shard does not hold",
			records: lines(
				modelrecord.Run{RunID: "run-1", Tier: "free"},
				modelrecord.Call{Attempt: "a9", Index: 1},
			),
			names: "a9",
		},
		{
			name: "an attempt naming a session the shard does not hold",
			records: lines(
				modelrecord.Run{RunID: "run-1", Tier: "free"},
				modelrecord.Attempt{ID: "a1", Session: "meta"},
			),
			names: "meta",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := Attempts(testCase.records)
			if err == nil {
				t.Fatal("Attempts returned no error for a shard it cannot join")
			}
			if !strings.Contains(err.Error(), testCase.names) {
				t.Errorf("error %q does not name %q", err, testCase.names)
			}
		})
	}
}

// TestAttempts_ARunWrittenTwice_IsOneRun allows the one repetition a writer can
// legitimately produce, a run line offered again for the same run, and refuses
// only a second run.
func TestAttempts_ARunWrittenTwice_IsOneRun(t *testing.T) {
	records := lines(
		modelrecord.Run{RunID: "run-1", Tier: "premium"},
		modelrecord.Run{RunID: "run-1", Tier: "premium"},
		modelrecord.Attempt{ID: "a1", Surface: "dynamic"},
	)

	attempts, err := Attempts(records)
	if err != nil {
		t.Fatalf("Attempts: %v", err)
	}
	if attempts[0].Run.Tier != "premium" {
		t.Errorf("tier = %q, want premium", attempts[0].Run.Tier)
	}
}

// TestAttempts_TheShardsOnDisk_ScoreAgainstTheRealCorpus is the end this
// package is for, run over a record nobody in this process wrote.
//
// The shards under testdata are what a run really produces, hand-written at the
// same schema: a dynamic session in the default mode with one attempt that did
// the task and one that read the wrong project, and a read-only meta session
// with one attempt that declined in prose and one that named the action and was
// refused the enum. They are scored against the corpus at HEAD, so a case whose
// key this package can no longer read fails here rather than in a paid run, and
// cmd/gen_model_results has a record to be written against before any run
// exists.
func TestAttempts_TheShardsOnDisk_ScoreAgainstTheRealCorpus(t *testing.T) {
	shards, err := modelrecord.ReadShards(filepath.Join("testdata", "shards"))
	if err != nil {
		t.Fatalf("ReadShards: %v", err)
	}
	if len(shards) != 2 {
		t.Fatalf("read %d shard(s), want the two under testdata", len(shards))
	}

	want := map[string]Outcome{
		"MT-002/dynamic/1": OutcomeCompleted,
		"MT-002/dynamic/2": OutcomeFailed,
		"MT-004/meta/1":    OutcomeCompleted,
		"MT-004/meta/2":    OutcomeCompleted,
	}
	scored := map[string]Verdict{}
	for _, shard := range shards {
		attempts, groupErr := Attempts(shard.Records)
		if groupErr != nil {
			t.Fatalf("Attempts(%s): %v", shard.Path, groupErr)
		}
		for _, attempt := range attempts {
			verdict, scoreErr := Score(attempt, keyFor(t, attempt.Line.Case))
			if scoreErr != nil {
				t.Fatalf("Score(%s): %v", attempt.Line.ID, scoreErr)
			}
			scored[attempt.Line.ID] = verdict
		}
	}

	if len(scored) != len(want) {
		t.Fatalf("scored %d attempt(s), want %d", len(scored), len(want))
	}
	for id, expected := range want {
		t.Run(id, func(t *testing.T) {
			verdict, found := scored[id]
			if !found {
				t.Fatalf("the shards hold no attempt %s", id)
			}
			if verdict.Outcome != expected {
				t.Errorf("Outcome = %q (%s), want %q", verdict.Outcome, verdict.Reason, expected)
			}
		})
	}

	t.Run("the declines are told apart", func(t *testing.T) {
		if got := scored["MT-004/meta/1"].Steps[0].Decline; got != DeclineByText {
			t.Errorf("the prose decline reads as %q, want %q", got, DeclineByText)
		}
		if got := scored["MT-004/meta/2"].Steps[0].Decline; got != DeclineByRefusal {
			t.Errorf("the refused call reads as %q, want %q", got, DeclineByRefusal)
		}
	})

	t.Run("the search is overhead and not a step", func(t *testing.T) {
		if got := scored["MT-002/dynamic/1"].Discovery; got != 1 {
			t.Errorf("Discovery = %d, want the one find call", got)
		}
	})
}

// TestAttemptReaders_FallBackTheWayASessionIsShared covers the three readers a
// verdict's row is built from.
func TestAttemptReaders_FallBackTheWayASessionIsShared(t *testing.T) {
	t.Run("the attempt's own surface wins over the session's", func(t *testing.T) {
		attempt := Attempt{
			Session: modelrecord.Session{Surface: "meta"},
			Line:    modelrecord.Attempt{Surface: "dynamic"},
		}
		if attempt.surface() != "dynamic" {
			t.Errorf("surface() = %q, want the attempt's own", attempt.surface())
		}
	})
	t.Run("an attempt that names no surface reads the session's", func(t *testing.T) {
		attempt := Attempt{Session: modelrecord.Session{Surface: "meta"}}
		if attempt.surface() != "meta" {
			t.Errorf("surface() = %q, want the session's", attempt.surface())
		}
	})
	t.Run("a session with no mode ran with neither protection on", func(t *testing.T) {
		if got := (Attempt{}).mode(); got != ModeDefault {
			t.Errorf("mode() = %q, want %q", got, ModeDefault)
		}
	})
	t.Run("a pinned tier wins over the detected one", func(t *testing.T) {
		attempt := Attempt{
			Run:     modelrecord.Run{Tier: "ultimate"},
			Session: modelrecord.Session{TierPin: "free"},
		}
		if attempt.tier() != "free" {
			t.Errorf("tier() = %q, want the pin: it is what the session served", attempt.tier())
		}
	})
}

// TestEvents_AreReadInCallOrderAndNotInLineOrder keeps a writer that buffered a
// line from changing which call reached which step.
func TestEvents_AreReadInCallOrderAndNotInLineOrder(t *testing.T) {
	facts := factsFor(t, modelcorpus.SurfaceDynamic)
	attempt := Attempt{Calls: []modelrecord.Call{
		{Index: 3, Tool: "c"},
		{Index: 1, Tool: "a"},
		{Index: 2, Tool: "b"},
	}}

	var order []int
	for _, event := range attempt.events(facts) {
		order = append(order, event.Index)
	}

	if !slices.Equal(order, []int{1, 2, 3}) {
		t.Errorf("the calls were read as %v, want them in call order", order)
	}
}

// TestIsDiscovery_ReadsTheServersOwnWordFirst keeps the find call out of every
// column that counts steps.
func TestIsDiscovery_ReadsTheServersOwnWordFirst(t *testing.T) {
	cases := []struct {
		name  string
		event Event
		want  bool
	}{
		{
			name:  "the span names the find tool and no action",
			event: Event{Tool: dynamictools.FindActionToolName, DispatchedTool: dynamictools.FindActionToolName},
			want:  true,
		},
		{
			name:  "no span arrived and the model named the find tool",
			event: Event{Tool: dynamictools.FindActionToolName},
			want:  true,
		},
		{
			name: "the span names the execute tool",
			event: Event{
				Tool:           dynamictools.ExecuteActionToolName,
				DispatchedTool: dynamictools.ExecuteActionToolName,
			},
			want: false,
		},
		{
			name: "a span carrying an action is never a search",
			event: Event{
				Tool:           dynamictools.FindActionToolName,
				DispatchedTool: dynamictools.FindActionToolName,
				Dispatched:     "project.get",
			},
			want: false,
		},
		{
			name: "the model named the find tool and the server saw another",
			event: Event{
				Tool:           dynamictools.FindActionToolName,
				DispatchedTool: dynamictools.ExecuteActionToolName,
			},
			want: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isDiscovery(testCase.event); got != testCase.want {
				t.Errorf("isDiscovery = %v, want %v", got, testCase.want)
			}
		})
	}
}

// factsFor reads the catalog the way one surface does, failing the test if it
// cannot be built at all.
func factsFor(t *testing.T, surface modelcorpus.Surface) catalogFacts {
	t.Helper()
	facts, err := catalogFor(string(surface), string(modelcorpus.TierUltimate))
	if err != nil {
		t.Fatalf("catalogFor(%s): %v", surface, err)
	}
	return facts
}

// TestActionArguments_AreReadWhereTheSurfacePutsThem is the locator, and the
// reason it is a locator rather than a field name: the same call means three
// different shapes on the three surfaces.
func TestActionArguments_AreReadWhereTheSurfacePutsThem(t *testing.T) {
	cases := []struct {
		name    string
		surface modelcorpus.Surface
		event   Event
		want    string
	}{
		{
			name:    "the dynamic execute tool carries them in params",
			surface: modelcorpus.SurfaceDynamic,
			event: Event{Tool: dynamictools.ExecuteActionToolName, Arguments: mustJSON(map[string]any{
				"action": "project.get",
				"params": map[string]any{"project_id": "g/p"},
			})},
			want: "g/p",
		},
		{
			name:    "a meta group tool carries them in params",
			surface: modelcorpus.SurfaceMeta,
			event: Event{Tool: "gitlab_project", Arguments: mustJSON(map[string]any{
				"action": "get",
				"params": map[string]any{"project_id": "g/p"},
			})},
			want: "g/p",
		},
		{
			name:    "an individual tool carries them itself",
			surface: modelcorpus.SurfaceIndividual,
			event: Event{Tool: "gitlab_project_get", Arguments: mustJSON(map[string]any{
				"project_id": "g/p",
			})},
			want: "g/p",
		},
		{
			name:    "a tool outside the catalog carries them itself on a dispatching surface",
			surface: modelcorpus.SurfaceDynamic,
			event: Event{Tool: "gitlab_discover_project", Arguments: mustJSON(map[string]any{
				"project_id": "g/p",
			})},
			want: "g/p",
		},
		{
			name:    "a call whose arguments are not an object carries none",
			surface: modelcorpus.SurfaceIndividual,
			event:   Event{Tool: "gitlab_project_get", Arguments: json.RawMessage(`"g/p"`)},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			facts := factsFor(t, testCase.surface)

			value, _ := canonical(testCase.event.actionArguments(facts)["project_id"])

			if value != testCase.want {
				t.Errorf("project_id read as %q, want %q", value, testCase.want)
			}
		})
	}
}

// TestConfirmed_IsReadWhereTheSurfaceReadsIt covers the one argument whose
// position differs from every other argument's.
//
// On dynamic it sits at the top level beside the action and its params, because
// the execute tool's own schema says so and a confirmation inside params is
// read as a parameter of the action. On the other two it travels with the
// action's own arguments.
func TestConfirmed_IsReadWhereTheSurfaceReadsIt(t *testing.T) {
	cases := []struct {
		name    string
		surface modelcorpus.Surface
		event   Event
		want    bool
	}{
		{
			name:    "dynamic reads it at the top level",
			surface: modelcorpus.SurfaceDynamic,
			event: Event{Tool: dynamictools.ExecuteActionToolName, Arguments: mustJSON(map[string]any{
				"action": "issue.delete", "params": map[string]any{}, "confirm": true,
			})},
			want: true,
		},
		{
			name:    "dynamic does not read one inside params",
			surface: modelcorpus.SurfaceDynamic,
			event: Event{Tool: dynamictools.ExecuteActionToolName, Arguments: mustJSON(map[string]any{
				"action": "issue.delete", "params": map[string]any{"confirm": true},
			})},
			want: false,
		},
		{
			name:    "meta reads it inside params",
			surface: modelcorpus.SurfaceMeta,
			event: Event{Tool: "gitlab_issue", Arguments: mustJSON(map[string]any{
				"action": "delete", "params": map[string]any{"confirm": true},
			})},
			want: true,
		},
		{
			name:    "individual reads it beside the arguments",
			surface: modelcorpus.SurfaceIndividual,
			event: Event{Tool: "gitlab_issue_delete", Arguments: mustJSON(map[string]any{
				"issue_iid": 3, "confirm": true,
			})},
			want: true,
		},
		{
			name:    "a model that wrote the word instead of the value approved all the same",
			surface: modelcorpus.SurfaceIndividual,
			event: Event{Tool: "gitlab_issue_delete", Arguments: mustJSON(map[string]any{
				"confirm": "true",
			})},
			want: true,
		},
		{
			name:    "and one that wrote something else did not",
			surface: modelcorpus.SurfaceIndividual,
			event:   Event{Tool: "gitlab_issue_delete", Arguments: mustJSON(map[string]any{"confirm": "maybe"})},
			want:    false,
		},
		{
			name:    "an absent flag is no approval",
			surface: modelcorpus.SurfaceIndividual,
			event:   Event{Tool: "gitlab_issue_delete", Arguments: mustJSON(map[string]any{"issue_iid": 3})},
			want:    false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			facts := factsFor(t, testCase.surface)

			if got := testCase.event.confirmed(facts); got != testCase.want {
				t.Errorf("confirmed = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestRequested_ResolvesAgainstTheWholeCatalogAndFallsBackToTheRecord is the
// reading a withheld call depends on, and it has to be this package's own: the
// runner resolved what it could reach at the time, and an action a protective
// mode removed is exactly what a served catalog cannot name.
func TestRequested_ResolvesAgainstTheWholeCatalogAndFallsBackToTheRecord(t *testing.T) {
	cases := []struct {
		name    string
		surface modelcorpus.Surface
		call    modelrecord.Call
		want    string
	}{
		{
			name:    "the dynamic action argument",
			surface: modelcorpus.SurfaceDynamic,
			call: modelrecord.Call{Tool: dynamictools.ExecuteActionToolName, Arguments: mustJSON(map[string]any{
				"action": "project.star",
			})},
			want: "project.star",
		},
		{
			name:    "a meta tool and its action argument",
			surface: modelcorpus.SurfaceMeta,
			call: modelrecord.Call{Tool: "gitlab_project", Arguments: mustJSON(map[string]any{
				"action": "star",
			})},
			want: "project.star",
		},
		{
			name:    "an individual tool name, which is declared rather than derived",
			surface: modelcorpus.SurfaceIndividual,
			call:    modelrecord.Call{Tool: "gitlab_project_star"},
			want:    "project.star",
		},
		{
			name:    "a record written against a catalog this tree cannot read",
			surface: modelcorpus.SurfaceIndividual,
			call:    modelrecord.Call{Tool: "gitlab_from_another_release", RequestedAction: "project.star"},
			want:    "project.star",
		},
		{
			name:    "a tool nobody can name",
			surface: modelcorpus.SurfaceIndividual,
			call:    modelrecord.Call{Tool: "gitlab_invented_by_the_model"},
			want:    "",
		},
		{
			// A meta tool the catalog knows, with an operation it does not:
			// the resolver answers with the domain and no action at all, which
			// names nothing rather than naming the domain's first action.
			name:    "a real meta tool and an invented action",
			surface: modelcorpus.SurfaceMeta,
			call: modelrecord.Call{Tool: "gitlab_project", Arguments: mustJSON(map[string]any{
				"action": "reticulate",
			})},
			want: "",
		},
		{
			// The same call, from a run that did resolve it. An identity
			// naming the domain and no action is not an answer, so the record's
			// own reading is what is left.
			name:    "a domain the resolver names and an action it cannot",
			surface: modelcorpus.SurfaceMeta,
			call: modelrecord.Call{
				Tool:            "gitlab_project",
				Arguments:       mustJSON(map[string]any{"action": "reticulate"}),
				RequestedAction: "project.reticulate",
			},
			want: "project.reticulate",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			facts := factsFor(t, testCase.surface)

			if got := facts.requested(testCase.call); got != testCase.want {
				t.Errorf("requested = %q, want %q", got, testCase.want)
			}
		})
	}
}

// TestCatalogFacts_ReadTheFlagsAVerdictDependsOn checks the two questions the
// catalog is consulted for, on an action of each kind and on a tool registered
// outside it.
func TestCatalogFacts_ReadTheFlagsAVerdictDependsOn(t *testing.T) {
	facts := factsFor(t, modelcorpus.SurfaceDynamic)

	cases := []struct {
		name        string
		step        modelcorpus.Step
		known       bool
		destructive bool
		mutating    bool
	}{
		{name: "a read", step: modelcorpus.Step{Action: "project.get"}, known: true},
		{name: "a mutation", step: modelcorpus.Step{Action: "project.star"}, known: true, mutating: true},
		{
			name:        "a destruction",
			step:        modelcorpus.Step{Action: "issue.delete"},
			known:       true,
			destructive: true,
			mutating:    true,
		},
		{
			name:  "a tool registered outside the catalog",
			step:  modelcorpus.Step{Standalone: "gitlab_discover_project"},
			known: true,
		},
		{name: "an action nothing registers", step: modelcorpus.Step{Action: "project.invented"}},
		{name: "a tool nothing registers", step: modelcorpus.Step{Standalone: "gitlab_invented"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			declared, known := facts.stepFacts(testCase.step)
			if known != testCase.known {
				t.Fatalf("known = %v, want %v", known, testCase.known)
			}
			if declared.destructive != testCase.destructive {
				t.Errorf("destructive = %v, want %v", declared.destructive, testCase.destructive)
			}
			if got := known && !declared.readOnly; got != testCase.mutating {
				t.Errorf("mutating = %v, want %v", got, testCase.mutating)
			}
		})
	}
}

// TestCatalogFacts_Mutating_IsFalseForANameTheCatalogDoesNotHave keeps an
// invented action from being read as a mutation, which would fail an attempt
// for a call the server answered with "no such action".
func TestCatalogFacts_Mutating_IsFalseForANameTheCatalogDoesNotHave(t *testing.T) {
	facts := factsFor(t, modelcorpus.SurfaceDynamic)

	if facts.mutating("project.invented_by_a_model") {
		t.Error("mutating = true for an action the catalog does not have")
	}
	if facts.mutating("") {
		t.Error("mutating = true for a call the span named no action for")
	}
}

// TestCatalogFor_ACatalogThatWillNotBuild_IsReportedAndNotMemoized is the one
// failure here that comes from outside the record.
//
// It matters because of what a silent failure would look like: an empty catalog
// reports every action as one the server does not register, which this package
// publishes as a corpus defect, so a build failure would read as a corpus
// nobody can trust rather than as a catalog nobody could build.
func TestCatalogFor_ACatalogThatWillNotBuild_IsReportedAndNotMemoized(t *testing.T) {
	previous := buildCatalog
	t.Cleanup(func() {
		buildCatalog = previous
		catalogsMu.Lock()
		defer catalogsMu.Unlock()
		delete(catalogs, catalogKey{tier: edition.Premium, surface: string(modelcorpus.SurfaceDynamic)})
	})
	buildCatalog = func(bool, gitlabtools.ActionCatalogOptions) (*actioncatalog.Catalog, error) {
		return nil, errors.New("the catalog would not build")
	}

	// A tier no other test reads, so the memo cannot answer from a catalog that
	// was built before this one was swapped in.
	_, err := catalogFor(string(modelcorpus.SurfaceDynamic), string(modelcorpus.TierPremium))

	if err == nil {
		t.Fatal("catalogFor returned no error when the catalog would not build")
	}
	if !strings.Contains(err.Error(), "would not build") {
		t.Errorf("error = %q, want it to carry what the builder said", err)
	}
	catalogsMu.Lock()
	defer catalogsMu.Unlock()
	if _, memoized := catalogs[catalogKey{tier: edition.Premium, surface: string(modelcorpus.SurfaceDynamic)}]; memoized {
		t.Error("a failed build was memoized, so every later attempt at that tier would fail too")
	}
}

// TestDecodeObject_ReadsOnlyAnObject covers the three shapes a call's arguments
// arrive in, of which two carry no arguments at all.
func TestDecodeObject_ReadsOnlyAnObject(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		fields int
	}{
		{name: "an object", raw: `{"project_id":"g/p","confirm":true}`, fields: 2},
		{name: "nothing at all", raw: ``},
		{name: "a value that is not an object", raw: `"g/p"`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := len(decodeObject(json.RawMessage(testCase.raw))); got != testCase.fields {
				t.Errorf("decodeObject read %d field(s), want %d", got, testCase.fields)
			}
		})
	}
}

// TestCatalogFor_IsReadOncePerTierAndSurface pins the memo, which is what keeps
// scoring a few hundred attempts from building the catalog a few hundred times.
func TestCatalogFor_IsReadOncePerTierAndSurface(t *testing.T) {
	first := factsFor(t, modelcorpus.SurfaceMeta)
	second := factsFor(t, modelcorpus.SurfaceMeta)

	if len(first.actions) != len(second.actions) {
		t.Fatalf("two readings hold %d and %d actions", len(first.actions), len(second.actions))
	}
	// The maps are compared by identity rather than by content: two readings of
	// one catalog hold equal content whether or not the memo was used, so only
	// the pointer says which happened.
	if reflect.ValueOf(first.actions).Pointer() != reflect.ValueOf(second.actions).Pointer() {
		t.Error("two readings built two catalogs, so the memo is not being used")
	}
}
