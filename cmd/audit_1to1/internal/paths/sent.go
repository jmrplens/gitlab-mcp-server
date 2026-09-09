package paths

import (
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// How GitLab sends an unsurfaced field, as far as the conditions record says.
const (
	// sentAlways is a field the entity exposes with no condition: every
	// GitLab that answers the endpoint sends it, and this server drops it.
	sentAlways = "always"
	// sentWhen is a field the entity exposes under a condition, recorded
	// beside it: a caller's option, a permission, an Enterprise license.
	sentWhen = "when"
	// sentUnknown is a field the conditions record cannot speak for: the
	// document names no component for the response, the record does not
	// hold the component, or the component does not expose the field, which
	// the ten components rendered outside the entity directories produce.
	sentUnknown = "unknown"
)

// UnsurfacedField is one field GitLab's document says an endpoint a package
// calls returns, that no output type of the package publishes.
//
// It is the reverse of [UnpublishedField]: that one is a field we claim and
// GitLab does not send, this one a field GitLab sends and we do not claim.
// The conditions record is what makes the list worth reading: a field sent
// always is a gap in the 1:1 surface, a field sent under a condition is a gap
// only where the condition holds, and the tier says which struct tag closes
// it.
type UnsurfacedField struct {
	// Grain names the join that produced the finding, package or type, the
	// same two [UnpublishedField] carries.
	Grain   string `json:"grain"`
	Package string `json:"package"`
	// Type is the output type the finding is about, at type grain only.
	Type  string `json:"type,omitempty"`
	Field string `json:"field"`
	// Operations are the endpoints whose responses carry the field, as the
	// inventory spells them.
	Operations []string `json:"operations"`
	// Entity is the component the first operation carrying the field and
	// naming a component resolves to, when the document names one for any;
	// the conditions below were read from it.
	Entity string `json:"entity,omitempty"`
	// SDKType is the client-go struct the type models, at type grain only, and
	// several joined by a pipe where a type models more than one.
	SDKType string `json:"sdk_type,omitempty"`
	// SDKModels says whether that struct carries this key too.
	//
	// It is what splits this list into the two halves a reader acts on
	// differently. False means client-go does not model the field either, so
	// surfacing it here means either an upstream contribution or reading it
	// from the captured response (ADR-0021), and the finding is evidence for
	// the merge request rather than work in this repository. True means the
	// SDK has it and only we do not, which is a local fix.
	//
	// It is only meaningful at type grain, where a finding knows which struct
	// models the response. The package grain unions endpoints across a package
	// and names no struct, so it leaves this false and says so through the
	// empty SDKType beside it.
	SDKModels bool `json:"sdk_models,omitempty"`
	// Sent is one of always, when and unknown.
	Sent string `json:"sent"`
	// If, Unless, Tier and Edition are what the conditions record says about
	// the field on that entity, empty when it says nothing.
	If      string `json:"if,omitempty"`
	Unless  string `json:"unless,omitempty"`
	Tier    string `json:"tier,omitempty"`
	Edition string `json:"edition,omitempty"`
	// Category and Reason are the declaration in sent_declarations.go
	// accounting for the finding, when one does: a field the document lists
	// for the operation and the endpoint does not send. Empty for a finding
	// nothing answers, which is the half a reader is asked to act on.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for the finding.
func (f UnsurfacedField) declared() bool { return f.Category != "" }

// SentCheck is the reverse comparison: what GitLab's document says the
// endpoints a package calls return, held against what the package publishes,
// with the conditions record saying when each missing field is sent.
//
// Like the package-grain comparison it is built from, it unions a package's
// endpoints, so a field one output type of the package publishes counts as
// published for every endpoint the package calls. That makes it a lower bound,
// exact for a package with one output type and looser as the package grows,
// and it is the honest shape available while the inventory records a package
// rather than an action. It over-reports in one known way: a package that
// calls an endpoint for something other than surfacing its answer (the health
// check reads /user to learn who the token is) is reported as missing that
// answer's every field. It is a report and not a gate: a field GitLab sends
// that this server does not surface is a candidate for the 1:1 surface, not a
// defect in it, until somebody reads the condition.
type SentCheck struct {
	// Ran is false when the conditions record could not be read, in which
	// case every finding says unknown and only the document speaks.
	Ran bool `json:"ran"`
	// Record names the conditions record that answered.
	Record string `json:"record,omitempty"`
	// Unsurfaced are the findings, by package then field.
	Unsurfaced []UnsurfacedField `json:"unsurfaced,omitempty"`
	// UnusedDeclarations names the entries of sent_declarations.go that
	// accounted for no finding at either grain, which is a finding of its own
	// for the reason [TypedShapeCheck.UnusedDeclarations] records.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// staleDeclarations renders this run's unused declarations as the findings
// the gate reports, and nothing when the check did not run, for the reason
// [TypedShapeCheck.staleDeclarations] records: every declaration would be
// unused then, and all of them stale.
func (c SentCheck) staleDeclarations() []string {
	if !c.Ran {
		return nil
	}
	stale := make([]string, 0, len(c.UnusedDeclarations))
	for _, key := range c.UnusedDeclarations {
		stale = append(stale, key+" is declared as a field GitLab's document lists and the endpoint does not send, and no finding matched it: the document no longer lists the field on that component, or the package now publishes it")
	}
	return stale
}

// unsurfacedCounts returns how many findings say always, how many say when,
// and how many a declaration accounts for; the unknown remainder is the
// difference of the first two from the total.
func unsurfacedCounts(fields []UnsurfacedField) (always, when, declared int) {
	for _, field := range fields {
		switch field.Sent {
		case sentAlways:
			always++
		case sentWhen:
			when++
		}
		if field.declared() {
			declared++
		}
	}
	return always, when, declared
}

// notModelledBySDK counts the findings client-go's own struct does not carry
// either, which is the half an upstream merge request answers.
func notModelledBySDK(fields []UnsurfacedField) int {
	count := 0
	for _, field := range fields {
		if !field.SDKModels {
			count++
		}
	}
	return count
}

// sortUnsurfaced orders the findings the way a reader reads them: down the
// tree, then by type, then by field.
func sortUnsurfaced(found []UnsurfacedField) {
	sort.Slice(found, func(i, j int) bool {
		if found[i].Package != found[j].Package {
			return found[i].Package < found[j].Package
		}
		if found[i].Type != found[j].Type {
			return found[i].Type < found[j].Type
		}
		return found[i].Field < found[j].Field
	})
}

// fieldSources is where a package met one response field: the endpoints,
// and the component the first of them named.
type fieldSources struct {
	operations map[string]bool
	entity     string
}

// responseSources is every response field a package's endpoints carry, by
// package then field, with where each was met.
type responseSources map[string]map[string]*fieldSources

// note records that a package's request to operation returned a response
// carrying the named fields.
func (s responseSources) note(pkg, operation string, entityOf map[string]string, fields []string) {
	byField := s[pkg]
	if byField == nil {
		byField = map[string]*fieldSources{}
		s[pkg] = byField
	}
	for _, field := range fields {
		sources := byField[field]
		if sources == nil {
			sources = &fieldSources{operations: map[string]bool{}}
			byField[field] = sources
		}
		sources.operations[operation] = true
		// The entity that renders this key, taken from the first operation
		// that named one for it rather than from the operation as a whole: an
		// operation is a merged shape wherever two routes differ only in what
		// they call their placeholders, and it then answers for keys of two
		// entities. Reading a key on the wrong one returns that entity's
		// condition, which is a wrong answer rather than a missing one.
		if sources.entity == "" {
			sources.entity = entityOf[field]
		}
	}
}

// sentCheck lists every response field a package's endpoints carry that none
// of its output types publishes, saying when GitLab sends each.
func sentCheck(conditions *conditionIndex, sources responseSources, published []publishedType) SentCheck {
	check := SentCheck{Ran: true, Record: apilive.FileName}

	publishedBy := map[string]map[string]bool{}
	for _, publishedType := range published {
		fields := publishedBy[publishedType.Package]
		if fields == nil {
			fields = map[string]bool{}
			publishedBy[publishedType.Package] = fields
		}
		for _, field := range publishedType.Fields {
			fields[field] = true
		}
	}

	for pkg, byField := range sources {
		// A package publishing nothing the walk could read is not judged: it
		// would be reported as missing every field of every endpoint, which
		// says something about the walk and nothing about the package.
		if len(publishedBy[pkg]) == 0 {
			continue
		}
		for field, met := range byField {
			if publishedBy[pkg][field] {
				continue
			}
			finding := UnsurfacedField{Grain: grainPackage, Package: pkg, Field: field, Operations: sortedOperations(met.operations), Entity: met.entity}
			conditions.annotate(&finding)
			check.Unsurfaced = append(check.Unsurfaced, finding)
		}
	}
	sortUnsurfaced(check.Unsurfaced)
	return check
}

// sortedOperations lists a set of operations in a stable order.
func sortedOperations(operations map[string]bool) []string {
	names := make([]string, 0, len(operations))
	for name := range operations {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// conditionIndex answers, for an entity and a field, what gates the exposure,
// resolving each entity's gates once.
type conditionIndex struct {
	doc   apilive.Document
	gates map[string]map[string]apilive.Gate
}

// newConditionIndex reads the gates off the record both grains were already
// given, so the conditions and the responses always speak for one GitLab.
func newConditionIndex(doc apilive.Document) *conditionIndex {
	return &conditionIndex{doc: doc, gates: map[string]map[string]apilive.Gate{}}
}

// annotate fills a finding's Sent and conditions from the record.
func (c *conditionIndex) annotate(finding *UnsurfacedField) {
	finding.Sent = sentUnknown
	if c == nil || finding.Entity == "" {
		return
	}
	gates, ok := c.entity(finding.Entity)
	if !ok {
		return
	}
	gate, exposed := gates[finding.Field]
	if !exposed {
		return
	}
	finding.If, finding.Unless, finding.Tier, finding.Edition = gate.If, gate.Unless, gate.Tier, gate.Edition
	if !gate.Gated() {
		finding.Sent = sentAlways
		return
	}
	finding.Sent = sentWhen
}

// entity returns an entity's gates by field name, and false for one the record
// does not hold.
//
// There is no parent chain to walk and no splat to skip: the record holds the
// class as it renders, so a field is present here exactly when GitLab can send
// it. Both of those were losses of the scanner this replaced, which read a
// parent it had a file for and left a run-time splat unresolved.
func (c *conditionIndex) entity(name string) (map[string]apilive.Gate, bool) {
	if cached, ok := c.gates[name]; ok {
		return cached, true
	}
	fields, ok := c.doc.Fields(name)
	if !ok {
		return nil, false
	}
	gates := make(map[string]apilive.Gate, len(fields))
	for fieldName, field := range fields {
		gates[fieldName] = c.doc.GateOf(field)
	}
	c.gates[name] = gates
	return gates, true
}
