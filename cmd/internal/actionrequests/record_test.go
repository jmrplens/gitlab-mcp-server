package actionrequests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeRecord writes content where a repository rooted at a temporary
// directory keeps the record, and returns the root.
func writeRecord(t *testing.T, content string) string {
	t.Helper()
	root := t.TempDir()
	target := filepath.Join(root, filepath.FromSlash(RecordPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		t.Fatalf("make the record's directory: %v", err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write the record: %v", err)
	}
	return root
}

// TestRenderRecord_ReadRecord_RoundTripsWhatTheWriterWrote verifies the one
// shape both sides hold: what the generator renders reads back as the same
// actions, under the note, with an action that has no path written as an
// empty list of paths rather than none.
func TestRenderRecord_ReadRecord_RoundTripsWhatTheWriterWrote(t *testing.T) {
	actions := []RecordAction{
		{
			ID: "issue.get", Handlers: []string{"issues.Get"}, Paths: [][]int{{0}}, Classic: "read_api", GroupScopes: []string{"admin_mode"},
			Requests: []RecordRequest{{
				Kind: KindREST, Route: "GET /projects/:id/issues/:issue_iid", Class: ClassMandatory,
				Classic: "read_api", ClassicDeclaration: "read-api-every-method",
				Sites: []string{"issues.Get"}, SDKMethods: []string{"Issues.GetIssue"},
			}},
		},
		{ID: "repository.archive", Handlers: []string{"repository.Archive"}, Declaration: "sends-nothing", Classic: "no-request"},
	}
	rendered := RenderRecord(actions)
	if !strings.Contains(string(rendered), `"paths": []`) {
		t.Errorf("an action with no path is not written with an empty list:\n%s", rendered)
	}
	root := writeRecord(t, string(rendered))

	got, err := ReadRecord(root)
	if err != nil {
		t.Fatalf("ReadRecord: %v", err)
	}
	if got.Note != RecordNote {
		t.Errorf("Note = %q, want the record's note", got.Note)
	}
	want := []RecordAction{actions[0], actions[1]}
	want[1].Paths = [][]int{}
	if !reflect.DeepEqual(got.Actions, want) {
		t.Errorf("Actions = %#v, want %#v", got.Actions, want)
	}
}

// TestRenderRecord_NoAction_IsAnEmptyList verifies a record of nothing is
// still a list a reader can range over, which is what ReadRecord then refuses
// by name.
func TestRenderRecord_NoAction_IsAnEmptyList(t *testing.T) {
	if got := string(RenderRecord(nil)); !strings.Contains(got, `"actions": []`) {
		t.Errorf("RenderRecord(nil) = %s, want an empty list of actions", got)
	}
}

// TestReadRecord_Failures_SayWhichStepFailed verifies a record that is not
// there, one that is not JSON and one that holds no action are each refused
// in words naming the step, the last one with the command that writes it.
func TestReadRecord_Failures_SayWhichStepFailed(t *testing.T) {
	cases := []struct {
		name string
		root func(t *testing.T) string
		want string
	}{
		{name: "missing", root: func(t *testing.T) string { t.Helper(); return t.TempDir() }, want: "read the action request record"},
		{name: "not_json", root: func(t *testing.T) string { t.Helper(); return writeRecord(t, "{") }, want: "parse " + RecordPath},
		{name: "empty", root: func(t *testing.T) string { t.Helper(); return writeRecord(t, `{"actions": []}`) }, want: "make gen-action-grants"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ReadRecord(testCase.root(t))
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("ReadRecord error = %v, want it to contain %q", err, testCase.want)
			}
		})
	}
}
