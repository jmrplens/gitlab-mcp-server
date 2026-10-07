package grants

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/actionrequests"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"
)

// Report is this scope's native JSON shape.
type Report struct {
	SchemaVersion int `json:"schema_version"`
	// Requests, Record and Inventory name the committed inputs, and
	// GitLabVersion the release the table was joined at.
	Requests      string  `json:"requests"`
	Record        string  `json:"record"`
	Inventory     string  `json:"inventory"`
	GitLabVersion string  `json:"gitlab_version"`
	Summary       Summary `json:"summary"`
	// Inconsistencies are what the gate fails on: a committed artifact that
	// disagrees with the live record or with the other artifact.
	Inconsistencies []string `json:"inconsistencies"`
	// Actions are every action's requirement in GitLab's words. Context, and
	// left out with -gaps-only.
	Actions []ActionGrant `json:"actions,omitempty"`
	// PhaseA is what no fine-grained token reaches at the recorded release.
	PhaseA PhaseA `json:"phase_a"`
	// Degraded and PublicOperations are context too, left out with
	// -gaps-only.
	Degraded         []DegradedAction  `json:"degraded,omitempty"`
	PublicOperations []PublicOperation `json:"public_operations,omitempty"`
	// Worklist is issue 1055's: the undeclared GraphQL elements this server
	// reaches.
	Worklist []WorklistEntry `json:"graphql_worklist"`
	// Classic is what a classic or OAuth token needs: the actions per scope,
	// the routes that are not a GET a read_api token is served, and the
	// actions whose read or write classification departs from that reach.
	Classic ClassicView `json:"classic"`
	// InventoryCheck and E2E are the two cross-checks of the derivation.
	InventoryCheck InventoryCheck `json:"inventory_check"`
	E2E            E2ECheck       `json:"e2e_check"`
}

// Summary is the count of everything the report holds, the figures a reader
// quotes.
type Summary struct {
	// Actions counts the table's rows; Reachable those some fine-grained
	// token reaches, and Withheld the rest, WithheldByGraphQL being the part
	// only GraphQL withholds (issue 1054's inventory).
	Actions           int `json:"actions"`
	Reachable         int `json:"reachable"`
	Withheld          int `json:"withheld"`
	WithheldByGraphQL int `json:"withheld_by_graphql"`
	// PartlyWithheld counts reachable actions with an input that sends a
	// request no fine-grained token passes.
	PartlyWithheld int `json:"partly_withheld"`
	// NotJudged counts actions with a way GitLab does not judge by the grant.
	NotJudged int `json:"actions_with_a_way_not_judged_by_the_grant"`
	// Degraded counts reachable actions served with a part of the answer
	// empty, always or unless the grant holds more.
	Degraded int `json:"served_with_parts_empty"`
	// ShapedByDirective and ShapedByDeclaration count the actions whose
	// requests a //gitlab:request directive or a declaration qualified.
	ShapedByDirective   int `json:"shaped_by_a_directive"`
	ShapedByDeclaration int `json:"shaped_by_a_declaration"`
	// PublicOperations counts the REST operations a public project or group
	// serves with no grant; zero when the table carries no evaluated set,
	// which PublicKnown says.
	PublicOperations int  `json:"public_operations"`
	PublicKnown      bool `json:"public_sets_known"`
	// WorklistElements counts the GraphQL elements of issue 1055's worklist,
	// and WorklistLeads those a REST route this server calls is a lead for.
	WorklistElements int `json:"graphql_worklist_elements"`
	WorklistLeads    int `json:"graphql_worklist_elements_with_a_rest_lead"`
	// The inventory and end-to-end figures repeat the checks' own, so a
	// reader of the summary has every number in one place.
	InventoryRESTRows           int `json:"inventory_rest_rows"`
	InventoryRESTRowsDerived    int `json:"inventory_rest_rows_derived"`
	InventoryGraphQLRows        int `json:"inventory_graphql_rows"`
	InventoryGraphQLRowsDerived int `json:"inventory_graphql_rows_derived"`
	DerivedREST                 int `json:"derived_rest_requests"`
	DerivedRESTRecorded         int `json:"derived_rest_requests_recorded"`
	DerivedGraphQL              int `json:"derived_graphql_requests"`
	DerivedGraphQLRecorded      int `json:"derived_graphql_requests_recorded"`
	E2EActionsCompared          int `json:"e2e_actions_compared"`
	E2EActionsConsistent        int `json:"e2e_actions_consistent"`
	// ReachedByReadAPI counts the actions a classic token carrying read_api
	// and not api is served, before the tier and the group scopes narrow it.
	ReachedByReadAPI int `json:"reached_by_read_api"`
	// Inconsistencies counts the gate's findings.
	Inconsistencies int `json:"inconsistencies"`
}

// clean reports whether the gate passes: the committed artifacts agree with
// the live record and with each other. Nothing else here gates, for the
// reasons each part records.
func (s Summary) clean() bool { return s.Inconsistencies == 0 }

// Options is what a run of this scope needs beyond the tree.
type Options struct {
	// GapsOnly keeps the work and drops the context: the per-action
	// requirements, the served-empty positions and the public operations.
	GapsOnly bool
	// E2ECallsDir is the shard directory an end-to-end run recorded its calls
	// into. Empty leaves the action-grain cross-check out.
	E2ECallsDir string
}

// Seams for the inputs, each committed beside the binary or compiled into it,
// and for the JSON encoder that never fails on a report of strings and ints.
// Each is a variable a test restores.
var (
	marshalIndent = json.MarshalIndent
	readRecord    = actionrequests.ReadRecord
	readInventory = requestinventory.Read
	catalog       = actionrequests.Catalog
	grantTable    = actiongrants.Table
	readE2ECalls  = e2ecalls.ReadForCalls
	readLive      = func(root string) (apilive.Document, error) {
		return apilive.Read(filepath.Join(root, apilive.DefaultDir))
	}
)

// Run builds the report for the repository at root and returns it as indented
// JSON with a trailing newline, together with the gate's outcome.
func Run(root string, opts Options) (content []byte, clean bool, err error) {
	report, err := buildReport(root, opts)
	if err != nil {
		return nil, false, err
	}
	content, err = marshalIndent(report, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal report: %w", err)
	}
	return append(content, '\n'), report.Summary.clean(), nil
}

// buildReport reads the committed inputs and assembles every part.
func buildReport(root string, opts Options) (Report, error) {
	record, err := readRecord(root)
	if err != nil {
		return Report{}, err
	}
	live, err := readLive(root)
	if err != nil {
		return Report{}, fmt.Errorf("read the live record: %w", err)
	}
	inventory, err := readInventory(root)
	if err != nil {
		return Report{}, err
	}
	actions, err := catalog()
	if err != nil {
		return Report{}, fmt.Errorf("build the action catalog: %w", err)
	}
	owners := make(map[string]string, len(actions))
	for _, action := range actions {
		owners[action.ID] = action.Owner
	}
	table := grantTable()

	view := viewActions(table, record, owners)
	elements := worklist(table, record, &live, owners)
	crossCheck := inventoryCheck(inventory.Requests, record, &live, owners)
	e2e := e2eCheck(opts.E2ECallsDir, record)
	found := inconsistencies(table, record, &live, actions)
	classic := classicView(table, actions)

	report := Report{
		SchemaVersion:    shared.SchemaVersion,
		Requests:         actionrequests.RecordPath,
		Record:           filepath.ToSlash(apilive.Path(apilive.DefaultDir)),
		Inventory:        requestinventory.Path,
		GitLabVersion:    table.DisplayVersion(),
		Inconsistencies:  found,
		Actions:          view.grants,
		PhaseA:           view.phaseA,
		Degraded:         view.degraded,
		PublicOperations: view.public,
		Worklist:         elements,
		Classic:          classic,
		InventoryCheck:   crossCheck,
		E2E:              e2e,
		Summary:          summarize(table, &view, elements, &crossCheck, &e2e, found),
	}
	report.Summary.ReachedByReadAPI = classic.ReachedByReadAPI
	if report.Inconsistencies == nil {
		report.Inconsistencies = []string{}
	}
	if opts.GapsOnly {
		report.Actions = nil
		report.Degraded = nil
		report.PublicOperations = nil
	}
	return report, nil
}

// summarize counts what the parts hold.
func summarize(table *finegrained.Table, view *actionView, elements []WorklistEntry, inventory *InventoryCheck, e2e *E2ECheck, found []string) Summary {
	summary := Summary{
		Actions:                     len(table.Actions),
		PartlyWithheld:              len(view.phaseA.PartlyWithheld),
		NotJudged:                   view.notJudged,
		Degraded:                    len(view.degraded),
		ShapedByDirective:           view.byDirective,
		ShapedByDeclaration:         view.byDeclaration,
		PublicOperations:            len(view.public),
		PublicKnown:                 table.PublicKnown,
		WorklistElements:            len(elements),
		InventoryRESTRows:           inventory.RESTRows,
		InventoryRESTRowsDerived:    inventory.RESTRowsDerived,
		InventoryGraphQLRows:        inventory.GraphQLRows,
		InventoryGraphQLRowsDerived: inventory.GraphQLRowsDerived,
		DerivedREST:                 inventory.DerivedREST,
		DerivedRESTRecorded:         inventory.DerivedRESTRecorded,
		DerivedGraphQL:              inventory.DerivedGraphQL,
		DerivedGraphQLRecorded:      inventory.DerivedGraphQLRecorded,
		E2EActionsCompared:          e2e.Compared,
		E2EActionsConsistent:        e2e.Consistent,
		Inconsistencies:             len(found),
	}
	for _, group := range view.phaseA.ByCause {
		summary.Withheld += group.Count
		if group.GraphQL {
			summary.WithheldByGraphQL += group.Count
		}
	}
	summary.Reachable = summary.Actions - summary.Withheld
	for i := range elements {
		if len(elements[i].RESTLeads) > 0 {
			summary.WorklistLeads++
		}
	}
	return summary
}
