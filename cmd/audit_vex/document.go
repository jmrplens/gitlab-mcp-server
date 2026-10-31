// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// openvexContext is the one context this command reads: OpenVEX v0.2.0.
const openvexContext = "https://openvex.dev/ns/v0.2.0"

// advisoryPage is where the Go vulnerability database publishes an advisory,
// which is the only @id a statement's vulnerability may carry.
const advisoryPage = "https://pkg.go.dev/vuln/"

// The statuses OpenVEX defines.
const (
	statusNotAffected        = "not_affected"
	statusAffected           = "affected"
	statusFixed              = "fixed"
	statusUnderInvestigation = "under_investigation"
)

// level is how deep the source scan reached into an advisory's module.
type level int

// The levels govulncheck reports a finding at, from the shallowest.
const (
	// levelModule is a module the loaded packages come from, with none of
	// the advisory's packages imported.
	levelModule level = iota + 1
	// levelPackage is a vulnerable package imported and none of its
	// vulnerable functions called.
	levelPackage
	// levelSymbol is a vulnerable function called.
	levelSymbol
)

// String names the level as a reader of the report needs it.
func (l level) String() string {
	switch l {
	case levelModule:
		return "against the module only"
	case levelPackage:
		return "with a vulnerable package imported and no vulnerable function called"
	default:
		return "with a vulnerable function called"
	}
}

// justifications are the five OpenVEX defines, each with the deepest level
// the scan may report for a not_affected statement giving it. A justification
// about reachability is disproved by the scan reaching further; one about who
// controls the input or what mitigates it is not a claim the scan can judge,
// although no statement of any kind may name an advisory whose function the
// scan reports called, which [judge] refuses before it reads this table.
var justifications = map[string]level{
	"component_not_present":                             levelModule,
	"vulnerable_code_not_present":                       levelModule,
	"vulnerable_code_not_in_execute_path":               levelPackage,
	"vulnerable_code_cannot_be_controlled_by_adversary": levelSymbol,
	"inline_mitigations_already_exist":                  levelSymbol,
}

// advisoryID is the shape of a Go vulnerability database id, the only kind
// of advisory the source scan can hold a statement to.
var advisoryID = regexp.MustCompile(`^GO-\d{4}-\d{4,}$`)

// imageDigest is the version of an image product: the sha256 digest of the
// index it names.
var imageDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// document is an OpenVEX v0.2.0 document, with every member the
// specification defines for it. identifiers and hashes, which the
// specification allows on a component, are not among them: this repository
// names a product by its package URL, and a member this command does not
// read is refused rather than passed over.
type document struct {
	Context     string      `json:"@context"`
	ID          string      `json:"@id"`
	Author      string      `json:"author"`
	Role        string      `json:"role"`
	Timestamp   string      `json:"timestamp"`
	LastUpdated string      `json:"last_updated"`
	Version     *int        `json:"version"`
	Tooling     string      `json:"tooling"`
	Statements  []statement `json:"statements"`
}

// statement is one OpenVEX statement.
type statement struct {
	ID                       string        `json:"@id"`
	Version                  *int          `json:"version"`
	Vulnerability            vulnerability `json:"vulnerability"`
	Timestamp                string        `json:"timestamp"`
	LastUpdated              string        `json:"last_updated"`
	Products                 []component   `json:"products"`
	Status                   string        `json:"status"`
	Supplier                 string        `json:"supplier"`
	StatusNotes              string        `json:"status_notes"`
	Justification            string        `json:"justification"`
	ImpactStatement          string        `json:"impact_statement"`
	ActionStatement          string        `json:"action_statement"`
	ActionStatementTimestamp string        `json:"action_statement_timestamp"`
}

// vulnerability is the advisory a statement is about.
type vulnerability struct {
	ID          string   `json:"@id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
}

// component is a product or a subcomponent of one.
type component struct {
	ID            string      `json:"@id"`
	Subcomponents []component `json:"subcomponents"`
}

// readDocument reads the document at path.
//
// The decode keeps encoding/json's semantics except for three refusals, each
// of which a hand-written document can trip without noticing: a member
// written twice (v1 keeps the last and says nothing), a member the
// specification does not define, which is how a misspelled justification
// would otherwise vanish, and a member whose case differs from the
// specification's.
func readDocument(path string) (document, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- the operator's own -file, inside the tree being checked
	if err != nil {
		return document{}, err
	}
	var doc document
	if decodeErr := jsonv2.Unmarshal(data, &doc, json.DefaultOptionsV1(), jsontext.AllowDuplicateNames(false),
		jsonv2.RejectUnknownMembers(true), jsonv2.MatchCaseInsensitiveNames(false)); decodeErr != nil {
		return document{}, fmt.Errorf("%s does not read as OpenVEX: %w", path, decodeErr)
	}
	return doc, nil
}

// validate returns every way the document departs from OpenVEX v0.2.0 or
// from the products and advisories this repository states, in document
// order. module is the module path go.mod declares.
func validate(doc document, module string) []string {
	problems := headerProblems(doc)
	for i, st := range doc.Statements {
		problems = append(problems, statementProblems(fmt.Sprintf("statement %d (%s)", i+1, st.Vulnerability.Name), st, module)...)
	}
	return problems
}

// headerProblems judges the members of the document itself.
func headerProblems(doc document) []string {
	var problems []string
	if doc.Context != openvexContext {
		problems = append(problems, fmt.Sprintf("the document's @context is %q, and this command reads %s", doc.Context, openvexContext))
	}
	if !isIRI(doc.ID) {
		problems = append(problems, fmt.Sprintf("the document's @id %q is not an https IRI", doc.ID))
	}
	if strings.TrimSpace(doc.Author) == "" {
		problems = append(problems, "the document names no author")
	}
	problems = append(problems, timeProblems("the document", doc.Timestamp, doc.LastUpdated, true)...)
	if doc.Version == nil || *doc.Version < 1 {
		problems = append(problems, "the document's version is not a positive integer")
	}
	if len(doc.Statements) == 0 {
		problems = append(problems, "the document carries no statement")
	}
	return problems
}

// statementProblems judges one statement, which subject names.
func statementProblems(subject string, st statement, module string) []string {
	var problems []string
	if st.ID != "" && !isIRI(st.ID) {
		problems = append(problems, fmt.Sprintf("%s: @id %q is not an https IRI", subject, st.ID))
	}
	if st.Version != nil && *st.Version < 1 {
		problems = append(problems, subject+": version is not a positive integer")
	}
	if !advisoryID.MatchString(st.Vulnerability.Name) {
		problems = append(problems, subject+": the vulnerability's name is not a Go vulnerability database id, the only kind the source scan can hold it to")
	} else if st.Vulnerability.ID != "" && st.Vulnerability.ID != advisoryPage+st.Vulnerability.Name {
		problems = append(problems, fmt.Sprintf("%s: the vulnerability's @id %q is not %s%s", subject, st.Vulnerability.ID, advisoryPage, st.Vulnerability.Name))
	}
	problems = append(problems, timeProblems(subject, st.Timestamp, st.LastUpdated, false)...)
	if _, err := time.Parse(time.RFC3339, st.ActionStatementTimestamp); st.ActionStatementTimestamp != "" && err != nil {
		problems = append(problems, fmt.Sprintf("%s: action_statement_timestamp %q is not an RFC 3339 time", subject, st.ActionStatementTimestamp))
	}
	problems = append(problems, statusProblems(subject, st)...)
	if len(st.Products) == 0 {
		problems = append(problems, subject+": names no product")
	}
	for _, product := range st.Products {
		problems = append(problems, productProblems(subject, product, module)...)
	}
	return problems
}

// statusProblems judges a statement's status and the members that go with it.
func statusProblems(subject string, st statement) []string {
	var problems []string
	switch st.Status {
	case statusNotAffected:
		if _, ok := justifications[st.Justification]; !ok {
			problems = append(problems, fmt.Sprintf("%s: not_affected with the justification %q, which is not one OpenVEX defines", subject, st.Justification))
		}
		if strings.TrimSpace(st.ImpactStatement) == "" {
			problems = append(problems, subject+": not_affected with no impact_statement saying why, which this repository asks for beside the justification")
		}
	case statusAffected:
		if strings.TrimSpace(st.ActionStatement) == "" {
			problems = append(problems, subject+": affected with no action_statement, which OpenVEX requires")
		}
	case statusUnderInvestigation:
	case statusFixed:
		problems = append(problems, subject+": fixed, which this document does not state: an advisory a dependency update fixed is one the dependency set no longer contains, and its statement leaves the document")
	default:
		problems = append(problems, fmt.Sprintf("%s: the status %q is not one OpenVEX defines", subject, st.Status))
	}
	if st.Status != statusNotAffected && st.Justification != "" {
		problems = append(problems, subject+": a justification belongs to a not_affected statement only")
	}
	return problems
}

// productProblems judges one product of a statement and its subcomponents.
func productProblems(subject string, product component, module string) []string {
	p, err := parsePURL(product.ID)
	if err != nil {
		return []string{fmt.Sprintf("%s: product: %v", subject, err)}
	}
	var problems []string
	image := p.kind == "oci"
	switch name := imageName(module); p.kind {
	case "golang":
		if p.path != module || p.version != "" || len(p.qualifiers) != 0 {
			problems = append(problems, fmt.Sprintf("%s: the product %q is not pkg:golang/%s, this module at every version", subject, product.ID, module))
		}
	case "oci":
		if p.path != name || !imageDigest.MatchString(p.version) || !strings.HasSuffix(p.qualifiers["repository_url"], "/"+name) {
			problems = append(problems, fmt.Sprintf("%s: the product %q is not the %s image pinned by the sha256 digest of its index, with the repository_url it was published to", subject, product.ID, name))
		}
	default:
		problems = append(problems, fmt.Sprintf("%s: the product %q is a %s package, and this repository publishes a Go module and an OCI image", subject, product.ID, p.kind))
	}
	if len(product.Subcomponents) == 0 {
		problems = append(problems, fmt.Sprintf("%s: the product %q names no subcomponent, so nothing says which module the advisory is against", subject, product.ID))
	}
	for _, sub := range product.Subcomponents {
		problems = append(problems, subcomponentProblems(subject, product.ID, sub, image)...)
	}
	return problems
}

// subcomponentProblems judges one subcomponent of the product named product.
// A subcomponent of an image has to name a version: a published image is
// one build, carrying one version of each module, and that version is what
// its statement is about, since the current dependency set says nothing of
// it.
func subcomponentProblems(subject, product string, sub component, image bool) []string {
	p, err := parsePURL(sub.ID)
	switch {
	case err != nil:
		return []string{fmt.Sprintf("%s: a subcomponent of %q: %v", subject, product, err)}
	case p.kind != "golang" || len(p.qualifiers) != 0:
		return []string{fmt.Sprintf("%s: the subcomponent %q of %q is not a Go module, which is what the source scan reports advisories against", subject, sub.ID, product)}
	case len(sub.Subcomponents) != 0:
		return []string{fmt.Sprintf("%s: the subcomponent %q of %q has subcomponents of its own, which OpenVEX does not define", subject, sub.ID, product)}
	case image && p.version == "":
		return []string{fmt.Sprintf("%s: the subcomponent %q of the image %q names no version, and a published image carries one version of each module", subject, sub.ID, product)}
	}
	return nil
}

// timeProblems judges a timestamp and the last_updated beside it. A
// statement's timestamp may be left out, since it inherits the document's;
// the document's may not.
func timeProblems(subject, timestamp, lastUpdated string, required bool) []string {
	var problems []string
	issued, err := time.Parse(time.RFC3339, timestamp)
	if err != nil && (required || timestamp != "") {
		problems = append(problems, fmt.Sprintf("%s: timestamp %q is not an RFC 3339 time", subject, timestamp))
	}
	if lastUpdated == "" {
		return problems
	}
	updated, err := time.Parse(time.RFC3339, lastUpdated)
	switch {
	case err != nil:
		problems = append(problems, fmt.Sprintf("%s: last_updated %q is not an RFC 3339 time", subject, lastUpdated))
	case !issued.IsZero() && updated.Before(issued):
		problems = append(problems, fmt.Sprintf("%s: last_updated %s is before its timestamp %s", subject, lastUpdated, timestamp))
	}
	return problems
}

// isIRI reports whether s is an absolute https URL with a host, which is how
// this repository writes every @id that is not a package URL.
func isIRI(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

// ownModules returns the modules a statement names as subcomponents of this
// module's own product, each once and sorted. Those are the claims the
// current dependency set can hold, and the ones that account for an advisory
// the scan of this module reports.
func ownModules(st statement, module string) []string {
	var modules []string
	for _, product := range st.Products {
		if p, err := parsePURL(product.ID); err != nil || p.kind != "golang" || p.path != module {
			continue
		}
		for _, sub := range product.Subcomponents {
			if p, err := parsePURL(sub.ID); err == nil && !slices.Contains(modules, p.path) {
				modules = append(modules, p.path)
			}
		}
	}
	slices.Sort(modules)
	return modules
}

// imageModule is one module a statement names in one published image.
type imageModule struct {
	product, path, version string
}

// imageModules returns every module a statement names as a subcomponent of
// an image product, in document order.
//
// A published image is pinned by its digest and never changes, so what the
// statement says of it is about the version its subcomponent names, and the
// current dependency set, which describes the next build and not that one,
// does not judge it: the advisory and the module leaving go.mod does not
// make a statement about an image that still carries them stale.
func imageModules(st statement) []imageModule {
	var modules []imageModule
	for _, product := range st.Products {
		if p, err := parsePURL(product.ID); err != nil || p.kind != "oci" {
			continue
		}
		for _, sub := range product.Subcomponents {
			if p, err := parsePURL(sub.ID); err == nil {
				modules = append(modules, imageModule{product: product.ID, path: p.path, version: p.version})
			}
		}
	}
	return modules
}
