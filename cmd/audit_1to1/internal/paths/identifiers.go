package paths

import (
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// IdentifierCheck is the half of R-PATH that reads values rather than names.
//
// Every other check of this scope reads the inventory as a list of endpoints
// and parameter names, which is what the recorder's templating leaves of a
// request: /projects/1/issues and /projects/57/issues are one row,
// /projects/:project_id/issues. That fold is what makes the file readable, and
// it hides one whole class of defect. With a single fixture value, a handler
// that builds the path out of the caller's project id and a handler that has
// the fixture's id written into it produce exactly the same row, so no rule
// here could tell them apart. Several handlers were in the second state and
// every one of them had to be found by hand.
//
// The recorder now counts, per row and per placeholder, how many distinct raw
// values it templated away, and this reads that count back: a placeholder seen
// with exactly one is a lead.
//
// # It reports, and it is meant to keep reporting
//
// Most of these leads are innocent. A fixture that creates one project and
// exercises forty endpoints against it produces a count of one on all forty,
// and nothing is wrong with any of them. So the count says where a hard-coded
// identifier could be hiding and never that one is, and a gate built on it
// would fail on the ordinary shape of a test suite rather than on a defect.
// What would make it gateable is fixtures that use two identifiers wherever
// they can, which is a change to hundreds of tests and not to this rule.
//
// There is deliberately no cheap static form of this question either. A lint
// over numeric literals in handler source would fire on every legitimate
// constant, and one over test files would fire on every fixture in the tree.
// The count is the only form of the question that is about the request the
// handler actually built.
type IdentifierCheck struct {
	// Ran is false when no row of the inventory carries identifier counts,
	// which is every inventory recorded before the recorder wrote them. It is
	// stated rather than inferred from an empty finding list, because "no row
	// was seen with one identifier" and "no row was asked" are opposite
	// answers that would otherwise print the same.
	Ran bool `json:"ran"`
	// Grain is the caveat spelled on the check as well as in the summary,
	// since a reader who opens the findings may never read the summary.
	Grain string `json:"grain,omitempty"`
	// Rows counts the inventory rows carrying at least one placeholder count,
	// which is the size of the question this was put to.
	Rows int `json:"rows_with_identifiers"`
	// Placeholders counts the placeholder positions across those rows.
	Placeholders int `json:"placeholders"`
	// Single counts the placeholders seen with exactly one distinct value,
	// which is the true size of the lead list whether or not the report prints
	// all of it.
	Single int `json:"placeholders_with_one_value"`
	// Leads are those placeholders, each naming the row it sits in, capped at
	// [maxIdentifierLeads]. Single is what says how many there really are.
	//
	// The cap is there because the expected number is large and made mostly of
	// innocent rows: a fixture that creates one project and drives forty
	// endpoints against it produces forty leads and no defect. A list nobody
	// can read is a list nobody reads, so the sharper leads are printed first
	// and the rest are counted.
	Leads []IdentifierLead `json:"leads,omitempty"`
}

// maxIdentifierLeads bounds the printed lead list. It is generous enough to
// hold every lead of a suite that varies its identifiers and small enough that
// the section stays readable in a report a `make` target prints.
const maxIdentifierLeads = 50

// IdentifierLead is one placeholder a package's requests only ever stood one
// value behind.
type IdentifierLead struct {
	Package string `json:"package"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	// Placeholder is the position, as the path spells it.
	Placeholder string `json:"placeholder"`
	// Others names the placeholders of the same path that were seen with more
	// than one value, with their counts, so a reader can tell a row whose
	// every position is single-valued from one where this position alone is.
	// The second is the sharper lead: the suite varied what it could and left
	// this one fixed.
	Others map[string]int `json:"others_varied,omitempty"`
}

// identifierGrain says what a count is a count of, spelled on the check for the
// same reason the observation and pagination grains are: a reader who opens the
// findings may never read the prose.
const identifierGrain = "a count is over one package's calls to one endpoint, so a placeholder seen with one value is a lead about that package's handlers and never a verdict"

// identifierCheck reads the identifier counts the inventory carries and reports
// every placeholder seen with exactly one distinct value.
func identifierCheck(requests []requestinventory.Row) IdentifierCheck {
	check := IdentifierCheck{Leads: []IdentifierLead{}}
	for _, request := range requests {
		if len(request.Identifiers) == 0 {
			continue
		}
		check.Ran = true
		check.Rows++
		check.Placeholders += len(request.Identifiers)
		check.Single += appendLeads(&check.Leads, request)
	}
	if check.Ran {
		check.Grain = identifierGrain
	}
	sort.Slice(check.Leads, func(i, j int) bool { return lessLead(check.Leads[i], check.Leads[j]) })
	if len(check.Leads) > maxIdentifierLeads {
		check.Leads = check.Leads[:maxIdentifierLeads]
	}
	return check
}

// appendLeads adds one row's single-valued placeholders to the lead list and
// reports how many it added.
func appendLeads(leads *[]IdentifierLead, request requestinventory.Row) int {
	added := 0
	for placeholder, count := range request.Identifiers {
		if count != 1 {
			continue
		}
		added++
		*leads = append(*leads, IdentifierLead{
			Package:     request.Package,
			Method:      request.Method,
			Path:        request.Path,
			Placeholder: placeholder,
			Others:      variedPlaceholders(request.Identifiers),
		})
	}
	return added
}

// variedPlaceholders lists the placeholders of one row that were seen with more
// than one value, or nothing when none was.
func variedPlaceholders(counts map[string]int) map[string]int {
	varied := map[string]int{}
	for placeholder, count := range counts {
		if count > 1 {
			varied[placeholder] = count
		}
	}
	if len(varied) == 0 {
		return nil
	}
	return varied
}

// lessLead orders two leads, the sharper ones first.
//
// A lead whose row has another placeholder that did vary is the sharper one:
// the suite reached this endpoint with two of something and only ever one of
// this, which is the shape a hard-coded identifier makes. A row where nothing
// varied is usually a fixture that made one project, so those sort last and are
// what the cap drops. Every field of the identity takes part after that, so the
// order is total and two runs over one recording print the same list.
func lessLead(a, b IdentifierLead) bool {
	if sharp, otherSharp := len(a.Others) > 0, len(b.Others) > 0; sharp != otherSharp {
		return sharp
	}
	left := []string{a.Package, a.Path, a.Method, a.Placeholder}
	right := []string{b.Package, b.Path, b.Method, b.Placeholder}
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}
