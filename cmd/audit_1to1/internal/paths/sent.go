package paths

import (
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apiexposes"
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
	// Sent is one of always, when and unknown.
	Sent string `json:"sent"`
	// If, Unless, Tier and Edition are what the conditions record says about
	// the field on that entity, empty when it says nothing.
	If      string `json:"if,omitempty"`
	Unless  string `json:"unless,omitempty"`
	Tier    string `json:"tier,omitempty"`
	Edition string `json:"edition,omitempty"`
}

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
}

// unsurfacedCounts returns how many findings say always and how many say
// when; the unknown remainder is the difference from the total.
func unsurfacedCounts(fields []UnsurfacedField) (always, when int) {
	for _, field := range fields {
		switch field.Sent {
		case sentAlways:
			always++
		case sentWhen:
			when++
		}
	}
	return always, when
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
func (s responseSources) note(pkg, operation, entity string, fields []string) {
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
		// The first operation naming a component for the field, not the
		// first carrying the field: one that names none would otherwise hold
		// the answer to unknown however many after it name one.
		if sources.entity == "" && entity != "" {
			sources.entity = entity
		}
	}
}

// sentCheck lists every response field a package's endpoints carry that none
// of its output types publishes, saying when GitLab sends each.
func sentCheck(root string, sources responseSources, published []publishedType) SentCheck {
	check := SentCheck{}
	conditions := newConditionIndex(root)
	if conditions != nil {
		check.Ran = true
		check.Record = apiexposes.FileName
	}

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

// conditionIndex answers, for a component and a field, what the conditions
// record says, resolving each component's effective fields once.
type conditionIndex struct {
	doc       apiexposes.Document
	effective map[string]map[string]apiexposes.Field
}

// newConditionIndex reads the conditions record beside the OpenAPI record,
// or returns nil when there is none to read.
func newConditionIndex(root string) *conditionIndex {
	doc, err := apiexposes.Read(recordDir(root))
	if err != nil {
		return nil
	}
	return &conditionIndex{doc: doc, effective: map[string]map[string]apiexposes.Field{}}
}

// annotate fills a finding's Sent and conditions from the record.
func (c *conditionIndex) annotate(finding *UnsurfacedField) {
	finding.Sent = sentUnknown
	if c == nil || finding.Entity == "" {
		return
	}
	fields, ok := c.fields(finding.Entity)
	if !ok {
		return
	}
	field, exposed := fields[finding.Field]
	if !exposed {
		return
	}
	finding.If, finding.Unless, finding.Tier, finding.Edition = field.If, field.Unless, field.Tier, field.Edition
	if field.If == "" && field.Unless == "" {
		finding.Sent = sentAlways
		return
	}
	finding.Sent = sentWhen
}

// fields returns a component's effective fields by name, the last declaration
// of a name winning as Grape's does, and false for a component the record
// does not hold.
func (c *conditionIndex) fields(entity string) (map[string]apiexposes.Field, bool) {
	if cached, ok := c.effective[entity]; ok {
		return cached, true
	}
	effective, ok := c.doc.Effective(entity)
	if !ok {
		return nil, false
	}
	byName := make(map[string]apiexposes.Field, len(effective))
	for _, field := range effective {
		// A splat names its fields at run time, so nothing here can be
		// looked up by it; the field it stood for stays unknown.
		if !field.Splat {
			byName[field.Name] = field
		}
	}
	c.effective[entity] = byName
	return byName, true
}
