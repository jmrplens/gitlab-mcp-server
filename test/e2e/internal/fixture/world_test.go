//go:build e2e

// world_test.go pins the World's digest, the diff that explains a changed
// one, the bindings a read sweep takes from it, and the teardown that reads
// it back and deletes it, all against the stub.

package fixture

import (
	"context"
	"errors"
	"net/http"
	"slices"
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

// TestWorldBindings_Extras_JoinOnlyOnceMade checks the plain bindings the
// extras add: each of the six names that mean one object across the catalog
// is bound to that object once the World made it, and absent while it has
// not, so the read sweep names it unbound rather than calling an action with
// a zero.
func TestWorldBindings_Extras_JoinOnlyOnceMade(t *testing.T) {
	bare := extrasWorld()
	core := len(bare.Bindings())
	made := madeWorld()
	want := map[string]any{
		"pipeline_id":    stubPipelineID,
		"job_id":         stubJobID,
		"tag_name":       "world-tag-run",
		"environment_id": stubEnvironmentID,
		"deployment_id":  stubDeploymentID,
		"deploy_key_id":  stubDeployKeyID,
	}
	for name, value := range want {
		t.Run(name, func(t *testing.T) {
			if got, bound := made.Bind(name); !bound || got != value {
				t.Errorf("Bind(%s) on a World that made it = (%v, %t), want %v", name, got, bound, value)
			}
			if got, bound := bare.Bind(name); bound {
				t.Errorf("Bind(%s) on a World that did not make it = %v, want no binding", name, got)
			}
		})
	}
	if got := len(made.Bindings()); got != core+len(want) {
		t.Errorf("Bindings() of a World with every extra has %d entries, want the %d of the core and the %d plain extras", got, core, len(want))
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

// TestTeardownWorld_PersonalSnippet_IsDeletedWithTheRest checks the one World
// object neither the project's deletion nor the group's takes along: the
// personal snippet belongs to the user, so the teardown deletes it itself, and
// a refusal there is reported without keeping the project and the group from
// going.
func TestTeardownWorld_PersonalSnippet_IsDeletedWithTheRest(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr string
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Snippet Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: "tearing down the World's personal snippet"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.addGroup(1, "World group", "e2e-world-group-run")
			stub.addProject(2, "World project", "e2e-world-group-run/e2e-world-project-run")
			stub.answers(http.MethodDelete, "/api/v4/snippets/10", testCase.answer)
			world := &World{
				Group:   Group{ID: 1, Path: "e2e-world-group-run"},
				Project: Project{ID: 2, Path: "e2e-world-group-run/e2e-world-project-run"},
				Snippet: Snippet{ID: 10},
			}

			err := teardownWorld(client, world)

			if testCase.wantErr == "" && err != nil {
				t.Errorf("teardownWorld() error = %v, want nil", err)
			}
			if testCase.wantErr != "" && (err == nil || !strings.Contains(err.Error(), testCase.wantErr)) {
				t.Errorf("teardownWorld() error = %v, want it to name the snippet", err)
			}
			if sent := sentPaths(stub); !slices.Equal(sent, []string{"DELETE /api/v4/snippets/10"}) {
				t.Errorf("the scripted requests were %v, want exactly the snippet's deletion", sent)
			}
			if projects, groups := stub.remaining(); len(projects) != 0 || len(groups) != 0 {
				t.Errorf("left projects %v and groups %v, want both deleted whatever the snippet answered", projects, groups)
			}
		})
	}
}

// TestTeardownWorld_DeletionRefused_ReportsItAndDeletesTheRest checks the
// teardown goes on past an object GitLab would not delete: the failure names
// the object, and the other one is still removed.
func TestTeardownWorld_DeletionRefused_ReportsItAndDeletesTheRest(t *testing.T) {
	cases := []struct {
		name         string
		refuse       func(*stubGitLab)
		wantErr      string
		wantProjects int
		wantGroups   int
	}{
		{
			name: "project",
			refuse: func(stub *stubGitLab) {
				stub.projects[2].permanentRemoveUnsupported = true
				stub.projects[2].permanentRemoveStatus = http.StatusForbidden
			},
			wantErr: "tearing down the World project", wantProjects: 1,
		},
		{
			name: "group",
			refuse: func(stub *stubGitLab) {
				stub.groups[1].permanentRemoveUnsupported = true
				stub.groups[1].permanentRemoveStatus = http.StatusForbidden
			},
			wantErr: "tearing down the World:", wantGroups: 1,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.addGroup(1, "World group", "e2e-world-group-run")
			stub.addProject(2, "World project", "e2e-world-group-run/e2e-world-project-run")
			stub.configure(func() { testCase.refuse(stub) })
			world := &World{
				Group:   Group{ID: 1, Path: "e2e-world-group-run"},
				Project: Project{ID: 2, Path: "e2e-world-group-run/e2e-world-project-run"},
			}

			err := teardownWorld(client, world)

			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("teardownWorld() error = %v, want it to say %q", err, testCase.wantErr)
			}
			if projects, groups := stub.remaining(); len(projects) != testCase.wantProjects || len(groups) != testCase.wantGroups {
				t.Errorf("left projects %v and groups %v, want %d and %d", projects, groups, testCase.wantProjects, testCase.wantGroups)
			}
		})
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

// scriptWorldBuild tells the stub to accept everything a World's build asks
// for, the core and every extra, and puts each object where the digest reads
// it back.
//
// The commits route answers twice, in the order the build commits: the World
// file on its branch, then the pipeline configuration on the default branch.
// The merge request is answered as already mergeable, which is what ends the
// build's wait for GitLab to prepare it.
func scriptWorldBuild(stub *stubGitLab) {
	stubWorld(stub)
	scriptWorldExtras(stub)
	stubExtraState(stub)
	stub.answers(http.MethodPost, "/api/v4/groups", stubCreated(map[string]any{
		"id": 1, "name": "World group", "full_path": "e2e-world-group-run", "path": "e2e-world-group-run",
	}))
	stub.answers(http.MethodPost, "/api/v4/projects", stubCreated(map[string]any{
		"id": 2, "name": "World project", "path_with_namespace": "e2e-world-group-run/e2e-world-project-run",
		"default_branch": "main", "namespace": map[string]any{"id": 1},
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/branches", stubCreated(map[string]any{
		"name": worldBranch, "commit": map[string]any{"id": "feature-sha"},
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/repository/commits",
		stubCreated(map[string]any{"id": "feature-sha", "short_id": "feature"}),
		stubCreated(map[string]any{"id": "ci-sha", "short_id": "ci-sha"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/merge_requests", stubCreated(map[string]any{
		"iid": 3, "title": worldMergeTitle, "source_branch": worldBranch, "target_branch": "main",
	}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/issues", stubCreated(map[string]any{"id": 40, "iid": 4, "title": worldIssueTitle}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/labels", stubCreated(map[string]any{"id": 5, "name": "world-label"}))
	stub.answers(http.MethodPost, "/api/v4/projects/2/milestones", stubCreated(map[string]any{"id": 6, "iid": 1, "title": "world-milestone"}))
	stub.configure(func() {
		mergeRequest, _ := stub.state["/api/v4/projects/2/merge_requests/3"].(map[string]any)
		mergeRequest["detailed_merge_status"] = "mergeable"
	})
}

// TestBuildWorld_Detached_MakesTheCoreThenTheExtrasThenTakesTheDigest drives
// the whole build against the stub: the group and project named for the run,
// the core objects filled in from what GitLab answered, every extra made, and
// the digest taken last, so its baseline holds the extras and the commit the
// pipeline configuration added rather than reading them as a change at the
// end of the run.
func TestBuildWorld_Detached_MakesTheCoreThenTheExtrasThenTakesTheDigest(t *testing.T) {
	shortWorldPipelineWaits(t)
	stub, e := detachedStub(t)
	scriptWorldBuild(stub)
	world := &World{}

	buildWorld(e, world)

	core := []struct {
		name      string
		got, want any
	}{
		{name: "group", got: world.Group.ID, want: int64(1)},
		{name: "project", got: world.Project.ID, want: int64(2)},
		{name: "branch", got: world.Branch.Name, want: worldBranch},
		{name: "commit", got: world.Commit.SHA, want: "feature-sha"},
		{name: "merge request", got: world.MergeRequest.IID, want: int64(3)},
		{name: "issue", got: world.Issue.IID, want: int64(4)},
		{name: "label", got: world.Label.ID, want: int64(5)},
		{name: "milestone", got: world.Milestone.ID, want: int64(6)},
	}
	for _, check := range core {
		t.Run("core "+check.name, func(t *testing.T) {
			if check.got != check.want {
				t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
			}
		})
	}
	if len(world.unbound) != 0 {
		t.Errorf("unbound = %v, want every extra made", world.unbound)
	}
	for _, extra := range worldExtras {
		t.Run("extra "+extra.name, func(t *testing.T) {
			if !extra.made(world) {
				t.Errorf("the World holds no %s", extra.name)
			}
		})
	}
	for _, field := range []string{"group.name", "milestone.title", "tag.target", "pipeline.sha", "job.name"} {
		t.Run("baseline "+field, func(t *testing.T) {
			if _, read := world.baseline[field]; !read {
				t.Errorf("the baseline has no %s, want the digest taken after every object was made", field)
			}
		})
	}
	if world.digest == "" || world.digest != Digest(world.baseline) {
		t.Errorf("digest = %q, want the digest of the baseline", world.digest)
	}

	group := requestTo(t, stub, http.MethodPost, "/api/v4/groups").Body
	if group["name"] != worldName(e.RunID(), worldGroupPrefix) || group["visibility"] != "private" {
		t.Errorf("the group was asked for as %v, want the run's World group, private", group)
	}
	project := requestTo(t, stub, http.MethodPost, "/api/v4/projects").Body
	wantProject := map[string]any{
		"name": worldName(e.RunID(), worldProjectPrefix), "description": worldDescription(e.RunID()),
		"namespace_id": float64(1), "initialize_with_readme": true, "default_branch": "main", "visibility": "private",
	}
	for field, want := range wantProject {
		t.Run("project "+field, func(t *testing.T) {
			if project[field] != want {
				t.Errorf("the project was asked for with %s = %v, want %v", field, project[field], want)
			}
		})
	}
}
