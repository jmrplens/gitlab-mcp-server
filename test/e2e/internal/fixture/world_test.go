//go:build e2e

// world_test.go pins the World's digest, the diff that explains a changed
// one, the bindings a read sweep takes from it, and the teardown that reads
// it back and deletes it, all against the stub.

package fixture

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// TestDigest_SameStateAnyOrder_HashesTheSame checks the digest is a
// function of the state and not of the order the map was filled in, and
// that any one field changing changes it.
func TestDigest_SameStateAnyOrder_HashesTheSame(t *testing.T) {
	first := map[string]string{"project.name": "p", "issue.state": "opened", "label.color": "#428BCA"}
	second := map[string]string{"label.color": "#428BCA", "issue.state": "opened", "project.name": "p"}
	if Digest(first) != Digest(second) {
		t.Errorf("Digest() differs for the same state in another order")
	}
	if got := Digest(map[string]string{}); len(got) != 64 {
		t.Errorf("Digest(empty) = %q, want 64 hex characters", got)
	}

	changed := map[string]string{"project.name": "p", "issue.state": "closed", "label.color": "#428BCA"}
	if Digest(first) == Digest(changed) {
		t.Errorf("Digest() did not change with issue.state")
	}
	// A key moved into a value must not hash the same as the key itself.
	if Digest(map[string]string{"a": "b=c"}) == Digest(map[string]string{"a=b": "c"}) {
		t.Errorf("Digest() confuses a key with a value")
	}
}

// TestWorldStateDiff_Readings_NamesEveryDifference checks the explanation
// a changed World produces: changed, gone and new fields, in field order.
func TestWorldStateDiff_Readings_NamesEveryDifference(t *testing.T) {
	before := map[string]string{"issue.state": "opened", "label.name": "l", "project.name": "p"}
	after := map[string]string{"issue.state": "closed", "project.name": "p", "milestone.title": "m"}

	got := worldStateDiff(before, after)
	want := []string{
		`issue.state: "opened" -> "closed"`,
		`label.name: "l" -> (gone)`,
		`milestone.title: (absent) -> "m"`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("worldStateDiff() = %q, want %q", got, want)
	}
	if diff := worldStateDiff(before, before); len(diff) != 0 {
		t.Errorf("worldStateDiff(same) = %q, want none", diff)
	}
}

// TestCompareWorld_Readings_FailsOnlyWhenTheDigestMoved checks the verdict
// names the field, and passes an unchanged World.
func TestCompareWorld_Readings_FailsOnlyWhenTheDigestMoved(t *testing.T) {
	baseline := map[string]string{"issue.state": "opened"}
	if err := compareWorld(baseline, Digest(baseline), map[string]string{"issue.state": "opened"}); err != nil {
		t.Errorf("compareWorld(unchanged) = %v, want nil", err)
	}

	err := compareWorld(baseline, Digest(baseline), map[string]string{"issue.state": "closed"})
	if !errors.Is(err, errWorldChanged) {
		t.Fatalf("compareWorld(changed) = %v, want errWorldChanged", err)
	}
	if want := `the shared World was changed during the run: issue.state: "opened" -> "closed"`; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// TestWorldName_RunID_ScopesEveryObject checks a World object's name carries
// the run ID, so the sweep recognizes it.
func TestWorldName_RunID_ScopesEveryObject(t *testing.T) {
	const runID = "20260912t101500z-0123456789-common"
	name := worldName(runID, worldProjectPrefix)
	if want := "e2e-world-project-" + runID; name != want {
		t.Errorf("worldName() = %q, want %q", name, want)
	}
	if !belongsToRun(name, "", runID) {
		t.Errorf("worldName() %q is not recognized as the run's own", name)
	}

	next := worldNames(runID, worldGroupPrefix)
	first, second, third := next(), next(), next()
	if first != "e2e-world-group-"+runID || second != first+"-2" || third != first+"-3" {
		t.Errorf("worldNames() = %q, %q, %q, want the plain name and then numbered ones", first, second, third)
	}
	for _, name := range []string{first, second, third} {
		t.Run(name, func(t *testing.T) {
			if !belongsToRun(name, "", runID) {
				t.Errorf("worldNames() %q is not recognized as the run's own", name)
			}
		})
	}
}

// TestWorldBindings_Parameters_ComeFromTheObjects checks every binding a
// read sweep takes maps to the object it names, and that Bind says no for a
// parameter the World does not carry.
func TestWorldBindings_Parameters_ComeFromTheObjects(t *testing.T) {
	world := &World{
		Group:        Group{ID: 1},
		Project:      Project{ID: 2, DefaultBranch: "main"},
		Branch:       Branch{Name: "feature/world"},
		Commit:       Commit{SHA: "abc", FilePath: "docs/world.md"},
		MergeRequest: MergeRequest{IID: 3},
		Issue:        Issue{IID: 4, ID: 40},
		Label:        Label{ID: 5},
		Milestone:    Milestone{ID: 6, IID: 8, Title: "world-milestone"},
		Username:     "e2e",
		UserID:       7,
	}
	cases := []struct {
		param string
		want  any
	}{
		{param: "project_id", want: int64(2)},
		{param: "group_id", want: int64(1)},
		{param: "namespace_id", want: int64(1)},
		{param: "issue_iid", want: int64(4)},
		{param: "issue_id", want: int64(40)},
		{param: "merge_request_iid", want: int64(3)},
		{param: "mr_iid", want: int64(3)},
		{param: "branch", want: "feature/world"},
		{param: "branch_name", want: "feature/world"},
		{param: "ref", want: "main"},
		{param: "sha", want: "abc"},
		{param: "commit_sha", want: "abc"},
		{param: "commit_id", want: "abc"},
		{param: "file_path", want: "docs/world.md"},
		{param: "label_id", want: int64(5)},
		{param: "milestone_id", want: int64(6)},
		{param: "milestone_iid", want: int64(8)},
		{param: "user_id", want: int64(7)},
		{param: "username", want: "e2e"},
	}
	for _, testCase := range cases {
		t.Run(testCase.param, func(t *testing.T) {
			got, ok := world.Bind(testCase.param)
			if !ok || got != testCase.want {
				t.Errorf("Bind(%q) = (%v, %t), want (%v, true)", testCase.param, got, ok, testCase.want)
			}
		})
	}
	if _, ok := world.Bind("pipeline_id"); ok {
		t.Errorf("Bind(pipeline_id) = true, want false for a parameter the World does not carry")
	}
	if got, want := len(world.Bindings()), len(cases); got != want {
		t.Errorf("Bindings() has %d entries, want the %d this test names", got, want)
	}
}

// TestWorldPromptBindings_Arguments_ComeFromTheObjects checks every prompt
// argument the World can supply maps to the object it names as a string, and
// that BindPromptArgument says no for an argument the World does not carry.
func TestWorldPromptBindings_Arguments_ComeFromTheObjects(t *testing.T) {
	world := &World{
		Group:        Group{ID: 1},
		Project:      Project{ID: 2, DefaultBranch: "main"},
		Branch:       Branch{Name: "feature/world"},
		MergeRequest: MergeRequest{IID: 3},
		Issue:        Issue{IID: 4},
		Milestone:    Milestone{Title: "world-milestone"},
		Username:     "e2e",
	}
	cases := []struct {
		argument string
		want     string
	}{
		{argument: "project_id", want: "2"},
		{argument: "group_id", want: "1"},
		{argument: "merge_request_iid", want: "3"},
		{argument: "issue_iid", want: "4"},
		{argument: "username", want: "e2e"},
		{argument: "from", want: "main"},
		{argument: "to", want: "feature/world"},
		{argument: "branch", want: "feature/world"},
		{argument: "ref", want: "main"},
		{argument: "target_branch", want: "main"},
		{argument: "milestone", want: "world-milestone"},
	}
	for _, testCase := range cases {
		t.Run(testCase.argument, func(t *testing.T) {
			got, ok := world.BindPromptArgument(testCase.argument)
			if !ok || got != testCase.want {
				t.Errorf("BindPromptArgument(%q) = (%q, %t), want (%q, true)", testCase.argument, got, ok, testCase.want)
			}
		})
	}
	if _, ok := world.BindPromptArgument("days"); ok {
		t.Errorf("BindPromptArgument(days) = true, want false for an argument the World does not carry")
	}
	if got, want := len(world.PromptBindings()), len(cases); got != want {
		t.Errorf("PromptBindings() has %d entries, want the %d this test names", got, want)
	}
}

// stubWorld puts a World's objects into the stub and returns the World that
// names them, with the state the readers will find.
func stubWorld(stub *stubGitLab) *World {
	stub.addGroup(1, "World group", "e2e-world-group-run")
	stub.addProject(2, "World project", "e2e-world-group-run/e2e-world-project-run")
	stub.configure(func() {
		stub.state["/api/v4/projects/2/repository/branches/main"] = map[string]any{"name": "main", "protected": true, "commit": map[string]any{"id": "main-sha"}}
		stub.state["/api/v4/projects/2/repository/branches/feature/world"] = map[string]any{"name": "feature/world", "protected": false, "commit": map[string]any{"id": "feature-sha"}}
		stub.state["/api/v4/projects/2/merge_requests/3"] = map[string]any{"iid": 3, "title": "World merge request", "state": "opened", "source_branch": "feature/world", "target_branch": "main", "labels": []string{"b", "a"}}
		stub.state["/api/v4/projects/2/issues/4"] = map[string]any{"iid": 4, "title": "World issue", "state": "opened", "labels": []string{}, "confidential": false}
		stub.state["/api/v4/projects/2/labels/5"] = map[string]any{"id": 5, "name": "world-label", "color": "#428BCA"}
		stub.state["/api/v4/projects/2/milestones/6"] = map[string]any{"id": 6, "title": "world-milestone", "state": "active"}
	})
	return &World{
		Group:        Group{ID: 1, Path: "e2e-world-group-run"},
		Project:      Project{ID: 2, DefaultBranch: "main"},
		Branch:       Branch{Name: "feature/world"},
		MergeRequest: MergeRequest{IID: 3},
		Issue:        Issue{IID: 4},
		Label:        Label{ID: 5},
		Milestone:    Milestone{ID: 6},
	}
}

// TestWorldState_Stub_ReadsEveryMutableField checks the readers against the
// stub: every field the digest covers is read, and a set comes back sorted.
func TestWorldState_Stub_ReadsEveryMutableField(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := stubWorld(stub)

	state, err := worldState(context.Background(), client, world)
	if err != nil {
		t.Fatalf("worldState() error = %v", err)
	}
	want := map[string]string{
		"group.name": "World group", "group.full_path": "e2e-world-group-run", "group.visibility": "private", "group.description": "",
		"project.name": "World project", "project.path_with_namespace": "e2e-world-group-run/e2e-world-project-run",
		"project.default_branch": "main", "project.visibility": "private", "project.description": "", "project.archived": "false", "project.topics": "",
		"branch.main.sha": "main-sha", "branch.feature/world.sha": "feature-sha",
		"merge_request.title": "World merge request", "merge_request.state": "opened", "merge_request.description": "",
		"merge_request.source_branch": "feature/world", "merge_request.target_branch": "main", "merge_request.labels": "a,b",
		"issue.title": "World issue", "issue.state": "opened", "issue.description": "", "issue.labels": "", "issue.confidential": "false",
		"label.name": "world-label", "label.color": "#428BCA", "label.description": "",
		"milestone.title": "world-milestone", "milestone.state": "active", "milestone.description": "",
	}
	if diff := worldStateDiff(want, state); len(diff) != 0 {
		t.Errorf("worldState() differs from what the stub holds:\n  %s", strings.Join(diff, "\n  "))
	}
}

// TestWorldState_MissingObject_ReportsWhichRead checks that a World object
// nothing can find fails the read naming it, since a digest over a partial
// read would compare two incomplete things.
func TestWorldState_MissingObject_ReportsWhichRead(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := stubWorld(stub)
	stub.configure(func() { delete(stub.state, "/api/v4/projects/2/labels/5") })

	_, err := worldState(context.Background(), client, world)
	if err == nil || !strings.Contains(err.Error(), "reading label 5") {
		t.Errorf("worldState() error = %v, want it to name the label read", err)
	}
}

// TestTeardownWorld_Unchanged_DeletesAndPasses checks the exit hook's happy
// path: the digest still matches and the group is removed with everything
// under it.
func TestTeardownWorld_Unchanged_DeletesAndPasses(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := stubWorld(stub)
	state, err := worldState(context.Background(), client, world)
	if err != nil {
		t.Fatalf("worldState() error = %v", err)
	}
	world.baseline, world.digest = state, Digest(state)

	if err = teardownWorld(client, world); err != nil {
		t.Fatalf("teardownWorld() error = %v, want nil", err)
	}
	if _, groups := stub.remaining(); len(groups) != 0 {
		t.Errorf("groups left = %v, want none", groups)
	}
}

// TestTeardownWorld_Changed_FailsAndStillDeletes checks that a World a test
// wrote to fails the run naming the field, and is deleted anyway.
func TestTeardownWorld_Changed_FailsAndStillDeletes(t *testing.T) {
	stub, client := newStubGitLab(t)
	world := stubWorld(stub)
	state, err := worldState(context.Background(), client, world)
	if err != nil {
		t.Fatalf("worldState() error = %v", err)
	}
	world.baseline, world.digest = state, Digest(state)
	stub.configure(func() {
		stub.state["/api/v4/projects/2/issues/4"] = map[string]any{"iid": 4, "title": "World issue", "state": "closed", "labels": []string{}, "confidential": false}
	})

	err = teardownWorld(client, world)
	if !errors.Is(err, errWorldChanged) {
		t.Fatalf("teardownWorld() error = %v, want errWorldChanged", err)
	}
	if !strings.Contains(err.Error(), `issue.state: "opened" -> "closed"`) {
		t.Errorf("error = %q, want it to name the changed field", err)
	}
	if _, groups := stub.remaining(); len(groups) != 0 {
		t.Errorf("groups left = %v, want the World deleted even though it changed", groups)
	}
}

// TestTeardownWorld_PartialBuild_DeletesWhatExists checks the hook a build
// that failed halfway leaves behind: no digest to compare, and a group to
// remove.
func TestTeardownWorld_PartialBuild_DeletesWhatExists(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.addGroup(1, "World group", "e2e-world-group-run")

	if err := teardownWorld(client, &World{Group: Group{ID: 1, Path: "e2e-world-group-run"}}); err != nil {
		t.Fatalf("teardownWorld(partial) error = %v, want nil", err)
	}
	if _, groups := stub.remaining(); len(groups) != 0 {
		t.Errorf("groups left = %v, want none", groups)
	}
	if err := teardownWorld(client, &World{}); err != nil {
		t.Errorf("teardownWorld(nothing built) error = %v, want nil", err)
	}
}
