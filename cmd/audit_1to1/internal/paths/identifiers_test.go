package paths

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// TestIdentifierCheck_APlaceholderSeenWithOneValue_IsALead verifies the whole
// of what this rule claims: a placeholder the recording only ever stood one
// value behind is reported, and one it varied is not.
//
// The distinction is the entire value of the check. Every other rule of this
// scope reads the inventory after the templating has folded /projects/1 and
// /projects/57 into one row, and with one fixture value a handler that reads
// the caller's project and a handler with an id written into it produce
// identical rows. The count is the only thing that separates them, and it
// separates them as a lead rather than a verdict, which is why the varied
// siblings travel with the finding.
func TestIdentifierCheck_APlaceholderSeenWithOneValue_IsALead(t *testing.T) {
	check := identifierCheck([]requestinventory.Row{
		{
			Package: "internal/tools/repository", Kind: "rest", Method: "GET",
			Path:        "/projects/:project_id/repository/tree",
			Identifiers: map[string]int{":project_id": 1},
		},
		{
			Package: "internal/tools/issues", Kind: "rest", Method: "GET",
			Path:        "/projects/:project_id/issues/:issue_id",
			Identifiers: map[string]int{":project_id": 1, ":issue_id": 4},
		},
		{
			Package: "internal/tools/groups", Kind: "rest", Method: "GET",
			Path:        "/groups/:group_id",
			Identifiers: map[string]int{":group_id": 3},
		},
	})

	if !check.Ran {
		t.Fatal("ran = false, want true: three rows carried counts")
	}
	if check.Rows != 3 || check.Placeholders != 4 {
		t.Errorf("rows = %d, placeholders = %d, want 3 and 4", check.Rows, check.Placeholders)
	}
	if check.Single != 2 || len(check.Leads) != 2 {
		t.Fatalf("single = %d with %d lead(s), want 2 and 2: %+v", check.Single, len(check.Leads), check.Leads)
	}
	// The sharper lead first: the suite varied the issue and never the project
	// on that row, which is the shape a hard-coded project makes.
	sharp := check.Leads[0]
	if sharp.Package != "internal/tools/issues" || sharp.Placeholder != ":project_id" {
		t.Errorf("first lead = %+v, want the issues row's :project_id", sharp)
	}
	if sharp.Others[":issue_id"] != 4 {
		t.Errorf("first lead's varied placeholders = %v, want :issue_id at 4", sharp.Others)
	}
	if blunt := check.Leads[1]; blunt.Package != "internal/tools/repository" || blunt.Others != nil {
		t.Errorf("second lead = %+v, want the repository row with nothing varied beside it", blunt)
	}
}

// TestIdentifierCheck_AnInventoryWithNoCounts_SaysItDidNotRun verifies the
// distinction a zero finding count cannot make on its own.
//
// Every inventory recorded before the recorder counted identifiers carries no
// counts at all, and so does a tree where nothing was reached twice. Reporting
// both as "no leads" would tell a reader the question was asked and answered
// when it was never asked, which is the belief this section exists to prevent.
func TestIdentifierCheck_AnInventoryWithNoCounts_SaysItDidNotRun(t *testing.T) {
	check := identifierCheck([]requestinventory.Row{
		{Package: "internal/tools/issues", Kind: "rest", Method: "GET", Path: "/projects/:project_id/issues"},
		{Package: "internal/tools/epics", Kind: "graphql", Method: "POST", Path: "/graphql", Operation: "query group"},
	})

	if check.Ran || check.Rows != 0 || check.Single != 0 || check.Grain != "" {
		t.Errorf("check = %+v, want it to say it never ran", check)
	}
}

// TestIdentifierCheck_ManyLeads_ArePrintedSharpestFirstAndCapped verifies that
// the report stays readable when the count is large, which is the expected
// case rather than the exception: a fixture that makes one project and drives
// forty endpoints against it produces forty innocent leads.
//
// The cap has to keep the sharp leads, or truncation would drop exactly the
// rows worth reading, and the true count has to survive it, or a reader would
// take the printed list for the whole answer.
func TestIdentifierCheck_ManyLeads_ArePrintedSharpestFirstAndCapped(t *testing.T) {
	rows := make([]requestinventory.Row, 0, maxIdentifierLeads+2)
	for i := range maxIdentifierLeads + 1 {
		rows = append(rows, requestinventory.Row{
			Package: "internal/tools/issues", Kind: "rest", Method: "GET",
			Path:        "/projects/:project_id/issues/" + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Identifiers: map[string]int{":project_id": 1},
		})
	}
	rows = append(rows, requestinventory.Row{
		Package: "internal/tools/zzz", Kind: "rest", Method: "GET",
		Path:        "/projects/:project_id/issues/:issue_id",
		Identifiers: map[string]int{":project_id": 1, ":issue_id": 2},
	})

	check := identifierCheck(rows)

	if check.Single != maxIdentifierLeads+2 {
		t.Errorf("single = %d, want %d: the count is the true one whatever the list prints", check.Single, maxIdentifierLeads+2)
	}
	if len(check.Leads) != maxIdentifierLeads {
		t.Fatalf("printed %d lead(s), want the cap of %d", len(check.Leads), maxIdentifierLeads)
	}
	if check.Leads[0].Package != "internal/tools/zzz" {
		t.Errorf("first lead = %+v, want the one row where something else varied", check.Leads[0])
	}
}

// TestCompareLead_SharpFirstThenEveryIdentityField verifies the lead order a
// reader and the cap both rely on: a lead whose row varied something else comes
// first whatever else it holds, how many things varied does not rank it
// further, and after that every field of the identity is compared in turn.
//
// Each identity case differs from the base in one field alone and every field
// before it is equal, so a comparison that stopped early, skipped a field or
// read one backwards answers zero or the wrong sign for exactly that case.
func TestCompareLead_SharpFirstThenEveryIdentityField(t *testing.T) {
	base := IdentifierLead{Package: "internal/tools/x", Method: "GET", Path: "/a/:a_id", Placeholder: ":a_id"}
	if got := compareLead(base, base); got != 0 {
		t.Errorf("compareLead(l, l) = %d, want 0", got)
	}

	sharp := base
	sharp.Package = "internal/tools/z"
	sharp.Others = map[string]int{":b_id": 2}
	if compareLead(sharp, base) >= 0 || compareLead(base, sharp) <= 0 {
		t.Error("a sharp lead did not sort before a blunt one that is earlier by name")
	}
	sharper := sharp
	sharper.Others = map[string]int{":b_id": 2, ":c_id": 3}
	if got := compareLead(sharp, sharper); got != 0 {
		t.Errorf("compareLead(one varied, two varied) = %d, want 0: sharpness is whether, not how many", got)
	}

	tests := []struct {
		name  string
		later func(*IdentifierLead)
	}{
		{name: "package", later: func(l *IdentifierLead) { l.Package = "internal/tools/y" }},
		{name: "path", later: func(l *IdentifierLead) { l.Path = "/b/:a_id" }},
		{name: "method", later: func(l *IdentifierLead) { l.Method = "PUT" }},
		{name: "placeholder", later: func(l *IdentifierLead) { l.Placeholder = ":b_id" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			later := base
			tt.later(&later)
			if compareLead(base, later) >= 0 || compareLead(later, base) <= 0 {
				t.Errorf("compareLead did not order two leads that differ only in the %s", tt.name)
			}
		})
	}
}
