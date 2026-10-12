// work_item_widgets_test.go holds the work item update inputs GitLab can be
// asked to remove to the request client-go sends: a value reaches the option
// struct, a removal reaches the mutation as null, and nothing else changes.
package toolutil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// workItemLookupAnswer answers the global-ID lookup client-go makes before
// every work item update, with the same request options as the update itself.
const workItemLookupAnswer = `{"data":{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/42"}}}}`

// workItemUpdateAnswer is the shortest answer an update mutation accepts.
const workItemUpdateAnswer = `{"data":{"workItemUpdate":{"workItem":{` +
	`"id":"gid://gitlab/WorkItem/42","iid":"42","workItemType":{"name":"Task"},` +
	`"state":"OPEN","title":"T","author":{"username":"dev"},"widgets":[]}}}}`

// sendWorkItemUpdate runs client-go's UpdateWorkItem with the options and the
// request option ApplyWorkItemWidgets produced, and returns the input object
// of the mutation GitLab received, so a test reads what reached the wire
// rather than what the helper meant to send.
func sendWorkItemUpdate(t *testing.T, opts *gl.UpdateWorkItemOptions, removals gl.RequestOptionFunc) map[string]any {
	t.Helper()
	var input map[string]any
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		if !strings.Contains(string(raw), "workItemUpdate") {
			testutil.RespondJSON(w, http.StatusOK, workItemLookupAnswer)
			return
		}
		var body struct {
			Variables struct {
				Input map[string]any `json:"input"`
			} `json:"variables"`
		}
		if err = json.Unmarshal(raw, &body); err != nil {
			t.Errorf("decode mutation: %v", err)
		}
		input = body.Variables.Input
		testutil.RespondJSON(w, http.StatusOK, workItemUpdateAnswer)
	}))
	if _, _, err := client.GL().WorkItems.UpdateWorkItem("g/p", 42, opts, gl.WithContext(t.Context()), removals); err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}
	if input == nil {
		t.Fatal("no workItemUpdate mutation reached the mock")
	}
	return input
}

// widgetOf reads one widget object out of a mutation input, failing when the
// mutation carries none.
func widgetOf(t *testing.T, input map[string]any, name string) map[string]any {
	t.Helper()
	widget, ok := input[name].(map[string]any)
	if !ok {
		t.Fatalf("mutation input carries no %s: %v", name, input)
	}
	return widget
}

// TestApplyWorkItemWidgets_Values_SetTheOptions verifies a value is copied
// onto the option struct client-go encodes, and that no request option is
// returned when nothing is removed, so an ordinary update is sent exactly as
// client-go builds it.
func TestApplyWorkItemWidgets_Values_SetTheOptions(t *testing.T) {
	milestone, parent, iteration := int64(4), int64(9), int64(7)
	start, due, health := "2026-06-01", "2026-06-30T10:00:00Z", "atRisk"
	var opts gl.UpdateWorkItemOptions
	removals, err := ApplyWorkItemWidgets(&opts, WorkItemWidgetInputs{
		MilestoneID: &milestone, ParentID: &parent, IterationID: &iteration,
		StartDate: &start, DueDate: &due, HealthStatus: &health,
	})
	if err != nil {
		t.Fatalf("ApplyWorkItemWidgets: %v", err)
	}
	if removals != nil {
		t.Error("a request option was returned for an update that removes nothing")
	}
	if *opts.MilestoneID != 4 || *opts.ParentID != 9 || *opts.IterationID != 7 {
		t.Errorf("references = %d, %d, %d, want 4, 9, 7", *opts.MilestoneID, *opts.ParentID, *opts.IterationID)
	}
	if opts.StartDate.String() != "2026-06-01" || opts.DueDate.String() != "2026-06-30" {
		t.Errorf("dates = %s, %s, want 2026-06-01, 2026-06-30", opts.StartDate, opts.DueDate)
	}
	if *opts.HealthStatus != "atRisk" {
		t.Errorf("health status = %q, want atRisk", *opts.HealthStatus)
	}
}

// TestApplyWorkItemWidgets_Omitted_LeavesEverythingAlone verifies an input
// left out sets nothing and asks for no removal.
func TestApplyWorkItemWidgets_Omitted_LeavesEverythingAlone(t *testing.T) {
	var opts gl.UpdateWorkItemOptions
	removals, err := ApplyWorkItemWidgets(&opts, WorkItemWidgetInputs{})
	if err != nil || removals != nil {
		t.Fatalf("ApplyWorkItemWidgets(empty) = %v, %v, want no option and no error", removals != nil, err)
	}
	if !reflect.DeepEqual(opts, gl.UpdateWorkItemOptions{}) {
		t.Errorf("options = %+v, want none set", opts)
	}
}

// TestApplyWorkItemWidgets_Removals_ReachGitLabAsNull drives each removal
// through client-go's own UpdateWorkItem and reads the mutation the mock
// received: 0 for a reference and "" for a date or the health status arrive
// as null in the widget GitLab reads them from, which is how its schema says a
// value is removed, and never as gid://gitlab/Milestone/0 or as nothing at all
// (issue 1303).
func TestApplyWorkItemWidgets_Removals_ReachGitLabAsNull(t *testing.T) {
	zero, empty := int64(0), ""
	cases := []struct {
		name   string
		in     WorkItemWidgetInputs
		widget string
		field  string
	}{
		{"milestone_id", WorkItemWidgetInputs{MilestoneID: &zero}, "milestoneWidget", "milestoneId"},
		{"parent_id", WorkItemWidgetInputs{ParentID: &zero}, "hierarchyWidget", "parentId"},
		{"iteration_id", WorkItemWidgetInputs{IterationID: &zero}, "iterationWidget", "iterationId"},
		{"start_date", WorkItemWidgetInputs{StartDate: &empty}, "startAndDueDateWidget", "startDate"},
		{"due_date", WorkItemWidgetInputs{DueDate: &empty}, "startAndDueDateWidget", "dueDate"},
		{"health_status", WorkItemWidgetInputs{HealthStatus: &empty}, "healthStatusWidget", "healthStatus"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts gl.UpdateWorkItemOptions
			removals, err := ApplyWorkItemWidgets(&opts, tc.in)
			if err != nil {
				t.Fatalf("ApplyWorkItemWidgets: %v", err)
			}
			if removals == nil {
				t.Fatal("no request option for a removal")
			}
			widget := widgetOf(t, sendWorkItemUpdate(t, &opts, removals), tc.widget)
			value, present := widget[tc.field]
			if !present || value != nil {
				t.Errorf("%s.%s = %#v (present %v), want null", tc.widget, tc.field, value, present)
			}
		})
	}
}

// TestApplyWorkItemWidgets_RemovalBesideAValue_KeepsBoth verifies a removal
// is written into the widget client-go already built rather than replacing
// it: removing the start date while moving the due date sends both, and the
// rest of the mutation (the work item's ID, a title set beside them) is sent
// as client-go wrote it.
func TestApplyWorkItemWidgets_RemovalBesideAValue_KeepsBoth(t *testing.T) {
	empty, due := "", "2026-07-01"
	opts := gl.UpdateWorkItemOptions{Title: new("Kept")}
	removals, err := ApplyWorkItemWidgets(&opts, WorkItemWidgetInputs{StartDate: &empty, DueDate: &due})
	if err != nil {
		t.Fatalf("ApplyWorkItemWidgets: %v", err)
	}
	input := sendWorkItemUpdate(t, &opts, removals)
	widget := widgetOf(t, input, "startAndDueDateWidget")
	if start, present := widget["startDate"]; !present || start != nil {
		t.Errorf("startDate = %#v (present %v), want null", start, present)
	}
	if widget["dueDate"] != "2026-07-01" {
		t.Errorf("dueDate = %#v, want 2026-07-01", widget["dueDate"])
	}
	if input["id"] != "gid://gitlab/WorkItem/42" || input["title"] != "Kept" {
		t.Errorf("id, title = %#v, %#v, want the work item's ID and the title set beside the removal", input["id"], input["title"])
	}
}

// TestApplyWorkItemWidgets_NegativeReference_IsRefused verifies a negative
// ID is refused before any request, naming the field, rather than sent as a
// global ID no object carries.
func TestApplyWorkItemWidgets_NegativeReference_IsRefused(t *testing.T) {
	negative := int64(-3)
	cases := []struct {
		name string
		in   WorkItemWidgetInputs
	}{
		{"milestone_id", WorkItemWidgetInputs{MilestoneID: &negative}},
		{"parent_id", WorkItemWidgetInputs{ParentID: &negative}},
		{"iteration_id", WorkItemWidgetInputs{IterationID: &negative}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts gl.UpdateWorkItemOptions
			removals, err := ApplyWorkItemWidgets(&opts, tc.in)
			if err == nil || removals != nil {
				t.Fatalf("ApplyWorkItemWidgets(%s=-3) = %v, %v, want a refusal", tc.name, removals != nil, err)
			}
			if !strings.Contains(err.Error(), tc.name) || !strings.Contains(err.Error(), "-3") {
				t.Errorf("error %q does not name the field and the value", err)
			}
		})
	}
}

// TestApplyWorkItemWidgets_MalformedDate_IsRefused verifies a date that does
// not parse is refused, naming the field, rather than dropped.
func TestApplyWorkItemWidgets_MalformedDate_IsRefused(t *testing.T) {
	bad := "2026-02-30"
	cases := []struct {
		name string
		in   WorkItemWidgetInputs
	}{
		{"start_date", WorkItemWidgetInputs{StartDate: &bad}},
		{"due_date", WorkItemWidgetInputs{DueDate: &bad}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var opts gl.UpdateWorkItemOptions
			if _, err := ApplyWorkItemWidgets(&opts, tc.in); err == nil || !strings.Contains(err.Error(), tc.name) {
				t.Errorf("ApplyWorkItemWidgets(%s=%q) error = %v, want a refusal naming the field", tc.name, bad, err)
			}
		})
	}
}

// TestWorkItemNullOption_RequestWithoutAnUpdateInput_IsLeftAlone verifies
// the option does nothing to a request carrying no update input: client-go
// hands the same options to the global-ID lookup it sends first, and to a
// request with no body at all.
func TestWorkItemNullOption_RequestWithoutAnUpdateInput_IsLeftAlone(t *testing.T) {
	option := workItemNullOption([]workItemNull{{widget: "milestoneWidget", field: "milestoneId"}})
	bodies := []struct {
		name string
		body any
	}{
		{"lookup query", []byte(`{"query":"query($fullPath: ID!, $iid: String) {}","variables":{"fullPath":"g/p","iid":"42"}}`)},
		{"no variables", []byte(`{"query":"{ currentUser { id } }"}`)},
		{"no body", nil},
	}
	for _, tc := range bodies {
		t.Run(tc.name, func(t *testing.T) {
			req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/graphql", tc.body)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if err = option(req); err != nil {
				t.Fatalf("option: %v", err)
			}
			got, err := req.BodyBytes()
			if err != nil {
				t.Fatalf("BodyBytes: %v", err)
			}
			want, _ := tc.body.([]byte)
			if string(got) != string(want) {
				t.Errorf("body = %s, want it unchanged: %s", got, want)
			}
		})
	}
}

// TestWorkItemNullOption_UnreadableBody_IsAnError verifies a body the option
// cannot read or decode fails the request rather than going out without the
// removal the caller asked for.
func TestWorkItemNullOption_UnreadableBody_IsAnError(t *testing.T) {
	option := workItemNullOption([]workItemNull{{widget: "milestoneWidget", field: "milestoneId"}})
	t.Run("not JSON", func(t *testing.T) {
		req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/graphql", []byte("not json"))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err = option(req); err == nil {
			t.Error("option accepted a body that is not JSON")
		}
	})
	t.Run("reader fails", func(t *testing.T) {
		// retryablehttp calls the reader once when the request is built, to
		// size it, so the reader fails from its second call on.
		calls := 0
		req, err := retryablehttp.NewRequest(http.MethodPost, "https://gitlab.example.com/api/graphql",
			retryablehttp.ReaderFunc(func() (io.Reader, error) {
				calls++
				if calls > 1 {
					return nil, errors.New("gone")
				}
				return strings.NewReader("{}"), nil
			}))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if err = option(req); err == nil || !strings.Contains(err.Error(), "gone") {
			t.Errorf("option error = %v, want the reader's error", err)
		}
	})
}
