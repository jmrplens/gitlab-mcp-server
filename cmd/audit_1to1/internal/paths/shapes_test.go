package paths

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apishapes"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/requestinventory"
)

// recordIn writes a GitLab API record a test can join against.
func recordIn(t *testing.T, operations map[string]apishapes.Operation) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, apishapes.DefaultDir)
	if err := apishapes.Write(dir, apishapes.Document{
		Source:     apishapes.Source{Ref: "master", RetrievedAt: "2026-09-07", SHA256: "abc", Operations: len(operations)},
		Operations: operations,
	}); err != nil {
		t.Fatalf("prepare the record: %v", err)
	}
	return root
}

// TestShapeCheck_JoinsOurSpellingToGitLabs verifies the three ways an endpoint
// can be looked up, because the join decides everything downstream: a row that
// does not match contributes no names, and a package whose rows all miss would
// have every field of its output reported as one GitLab does not send.
func TestShapeCheck_JoinsOurSpellingToGitLabs(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/issues/{issue_iid}": {Response: []string{"iid", "state", "title"}},
		"GET /api/v4/version":                          {Response: []string{"revision", "version"}},
	})

	cases := []struct {
		name    string
		row     requestinventory.Row
		quality string
		literal string
	}{
		{
			name:    "the same path with the same placeholder names",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/version"},
			quality: "exact",
		},
		{
			name:    "our placeholder names differ from GitLab's",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/:project_id/issues/:issue_id"},
			quality: "exact",
		},
		{
			name:    "a fixture value stands where an identifier belongs",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/myproject/issues/:issue_id"},
			quality: "loose",
			literal: "myproject",
		},
		{
			name:    "an endpoint the document does not carry",
			row:     requestinventory.Row{Package: "p", Kind: "rest", Method: "GET", Path: "/projects/:project_id/nowhere"},
			quality: "unmatched",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			check := shapeCheck(root, []requestinventory.Row{testCase.row}, nil)

			if !check.Ran {
				t.Fatal("shapeCheck() did not run against a record it should have read")
			}
			got := map[string]int{"exact": check.Join.Exact, "loose": check.Join.Loose, "unmatched": check.Join.Unmatched}
			if got[testCase.quality] != 1 {
				t.Errorf("join = %+v, want one %s match", check.Join, testCase.quality)
			}
			switch {
			case testCase.literal == "" && len(check.Untemplated) > 0:
				t.Errorf("reported %q as untemplated for a match that needed no literal", check.Untemplated[0].Segment)
			case testCase.literal != "" && (len(check.Untemplated) != 1 || check.Untemplated[0].Segment != testCase.literal):
				t.Errorf("untemplated = %+v, want the one segment %q", check.Untemplated, testCase.literal)
			}
		})
	}
}

// TestShapeCheck_APublishedFieldNoEndpointSends_IsReported verifies the finding
// this whole comparison exists for, which is
// https://github.com/jmrplens/gitlab-mcp-server/issues/580 in miniature: a type
// publishing fields GitLab's own document does not list for any endpoint the
// package calls.
func TestShapeCheck_APublishedFieldNoEndpointSends_IsReported(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"GET /api/v4/projects/{id}/merge_requests/{iid}/approvals": {
			Response: []string{"approved", "approved_by", "user_can_approve", "user_has_approved"},
		},
	})
	rows := []requestinventory.Row{{
		Package: "internal/tools/mrapprovals", Kind: "rest", Method: "GET",
		Path: "/projects/:project_id/merge_requests/:merge_request_id/approvals",
	}}
	published := []publishedType{{
		Package: "internal/tools/mrapprovals",
		Name:    "ConfigOutput",
		Fields:  []string{"approved", "approved_by", "id", "state", "title"},
	}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 3 {
		t.Fatalf("reported %d field(s), want the three GitLab does not send: %+v", len(check.Unpublished), check.Unpublished)
	}
	var names []string
	for _, field := range check.Unpublished {
		names = append(names, field.Field)
		if field.Endpoints != 1 {
			t.Errorf("%s says %d endpoints were searched, want 1", field.Field, field.Endpoints)
		}
	}
	if strings.Join(names, ",") != "id,state,title" {
		t.Errorf("unpublished = %v, want id, state and title", names)
	}
}

// TestShapeCheck_APackageWhoseEndpointsDeclareNothing_IsNotJudged verifies the
// silence that keeps the check honest. GitLab's document names no response
// schema for 553 of its operations, and an empty union means it does not say
// rather than that it sends nothing: reporting against one would call every
// field of that package's output a field GitLab never sends.
func TestShapeCheck_APackageWhoseEndpointsDeclareNothing_IsNotJudged(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"DELETE /api/v4/projects/{id}/silent": {},
	})
	rows := []requestinventory.Row{{Package: "internal/tools/quiet", Kind: "rest", Method: "DELETE", Path: "/projects/:project_id/silent"}}
	published := []publishedType{{Package: "internal/tools/quiet", Name: "Output", Fields: []string{"anything"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 0 {
		t.Errorf("reported %+v against an endpoint whose response the document does not describe", check.Unpublished)
	}
}

// TestShapeCheck_GraphQLRows_AreNotJoined verifies that a GraphQL row is left
// alone. The record describes the REST API, so a GraphQL request has no
// operation there and counting it as unmatched would report a miss that is only
// about looking in the wrong document.
func TestShapeCheck_GraphQLRows_AreNotJoined(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{"GET /api/v4/version": {Response: []string{"version"}}})
	rows := []requestinventory.Row{{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "query"}}

	check := shapeCheck(root, rows, nil)

	if check.Join.RESTRows != 0 || check.Join.Unmatched != 0 {
		t.Errorf("join = %+v, want a GraphQL row counted nowhere", check.Join)
	}
}

// TestShapeCheck_NoRecord_DoesNotRun verifies the one way this check is
// skipped. A missing record is a repository that has not generated one yet, and
// the scope around this must keep working rather than fail for a reason
// unrelated to what it audits.
func TestShapeCheck_NoRecord_DoesNotRun(t *testing.T) {
	check := shapeCheck(t.TempDir(), []requestinventory.Row{{Kind: "rest", Method: "GET", Path: "/version"}}, nil)

	if check.Ran {
		t.Error("shapeCheck() ran without a record to read")
	}
	if len(check.Unpublished) != 0 || check.Join.RESTRows != 0 {
		t.Errorf("check = %+v, want nothing reported", check)
	}
}

// TestShapeCheck_TwoOperationsShareAShape_UnionTheirResponses verifies that a
// shape carrying two operations contributes both their fields. Taking one
// arbitrarily would report the other's fields as unpublished, which is a
// finding about the lookup rather than about GitLab.
func TestShapeCheck_TwoOperationsShareAShape_UnionTheirResponses(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"GET /api/v4/groups/{id}/thing":    {Response: []string{"from_groups"}},
		"GET /api/v4/groups/{name}/thing":  {Response: []string{"from_name"}},
		"GET /api/v4/projects/{id}/absent": {},
	})
	rows := []requestinventory.Row{{Package: "p", Kind: "rest", Method: "GET", Path: "/groups/:group_id/thing"}}
	published := []publishedType{{Package: "p", Name: "Output", Fields: []string{"from_groups", "from_name", "invented"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "invented" {
		t.Errorf("unpublished = %+v, want only the field neither operation declares", check.Unpublished)
	}
}

// TestShapeCheck_ARecordKeyThatNamesNoMethod_IsSkipped verifies that a key the
// index cannot split is passed over rather than indexed under a method of "".
// Every key the generator writes is "METHOD /path", so this is a guard against
// a hand-edited or future-schema record, and skipping is the honest response:
// an entry nothing can address contributes no response names, and inventing a
// method for it would answer a lookup with somebody else's fields.
func TestShapeCheck_ARecordKeyThatNamesNoMethod_IsSkipped(t *testing.T) {
	root := recordIn(t, map[string]apishapes.Operation{
		"malformed-key-with-no-method": {Response: []string{"never_seen"}},
		"GET /api/v4/version":          {Response: []string{"version"}},
	})
	rows := []requestinventory.Row{{Package: "p", Kind: "rest", Method: "GET", Path: "/version"}}
	published := []publishedType{{Package: "p", Name: "Output", Fields: []string{"never_seen", "version"}}}

	check := shapeCheck(root, rows, published)

	if len(check.Unpublished) != 1 || check.Unpublished[0].Field != "never_seen" {
		t.Errorf("unpublished = %+v, want the field only the unaddressable entry declares", check.Unpublished)
	}
}
