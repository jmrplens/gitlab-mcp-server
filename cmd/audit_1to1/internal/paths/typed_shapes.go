package paths

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/structs"
)

// TypedShapeCheck is the same question [ShapeCheck] asks, at type grain: not
// what the endpoints a package calls answer with, but what the endpoints one
// output type actually models answer with.
//
// The chain is deterministic and every link already existed. A converter pairs
// an output type with a client-go struct, which is what the field diff runs
// over; client-go's service methods say which endpoints answer with that
// struct; and GitLab's document says what those endpoints send. Nothing in it
// consults the request inventory, which records a package and never an action
// and so cannot be sharpened.
//
// It reports and never gates, for the same reason the package-grain join does,
// and it is kept beside that join rather than replacing it: a reader comparing
// the two sees which of the package-grain findings survive a comparison that
// searched only the right responses.
//
// It asks the same question one level down as well, against the nested
// properties schema version 2 of the record carries. See [unpublishedNested].
//
// A finding here can be answered rather than fixed: the oracle is generated
// from GitLab's own Grape entities and is not always complete, so a finding
// carries the declaration accounting for it when shape_declarations.go has one.
type TypedShapeCheck struct {
	// Ran is false when the tool packages could not be loaded or their import
	// graph named no client-go to read. It is also false, with the whole check
	// off beside it, when GitLab's record could not be read at all, since the
	// package-grain join is what reads it.
	Ran bool `json:"ran"`
	// Compared counts the output types held against the response of the
	// operations their client-go struct models.
	Compared int `json:"compared"`
	// SkippedNoPairing counts the output types no converter pairs with a
	// client-go struct. They are our own wrappers around a JSON array, our own
	// answers to a 204 and to a not-found, and the synthetic results of a
	// handler that calls nothing: no endpoint sends their keys because they are
	// not an endpoint's response, and the package-grain join already carries
	// them as the lower bound it is.
	SkippedNoPairing int `json:"skipped_no_pairing"`
	// SkippedNoRoute counts the output types whose client-go struct no service
	// method was seen answering with, so there is no endpoint to ask about.
	SkippedNoRoute int `json:"skipped_no_route"`
	// SkippedNoSchema counts the output types whose every route either is
	// absent from GitLab's document or carries no response schema there. An
	// empty union means the document does not say rather than that GitLab sends
	// nothing, so judging against one would condemn a whole type for a hole in
	// the record.
	SkippedNoSchema int `json:"skipped_no_schema"`
	// NestedCompared counts the nested output types held against the properties
	// GitLab's document gives the response property they sit under. A type
	// whose property the record describes no object for is not among them, for
	// the reason [unpublishedNested] records.
	NestedCompared int `json:"nested_compared"`
	// Unpublished are the findings: an output field GitLab's document does not
	// list for any operation the type models, each carrying the declaration
	// that accounts for it when one does.
	Unpublished []UnpublishedField `json:"unpublished,omitempty"`
	// Nested are the same findings one level down, each naming the response
	// property its type sits under.
	Nested []UnpublishedField `json:"nested_unpublished,omitempty"`
	// UnusedDeclarations names the shape declarations that matched no finding
	// in this run, sorted. Each is a claim about GitLab's record that no longer
	// describes it.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
}

// undeclared counts the findings no declaration accounts for, at both levels,
// which is the number a reader of this check is being asked to act on.
func (c TypedShapeCheck) undeclared() int {
	count := 0
	for _, findings := range [][]UnpublishedField{c.Unpublished, c.Nested} {
		for _, finding := range findings {
			if !finding.declared() {
				count++
			}
		}
	}
	return count
}

// staleDeclarations renders this run's unused declarations as the findings the
// report lists beside the endpoint ones.
//
// A run that did not compare anything has nothing to say about them, for the
// reason [EndpointCheck.staleDeclarations] records: every declaration would
// look unused, and the loudest wrong answer this could give is that they are
// all stale.
func (c TypedShapeCheck) staleDeclarations() []string {
	if !c.Ran {
		return nil
	}
	stale := make([]string, 0, len(c.UnusedDeclarations))
	for _, key := range c.UnusedDeclarations {
		stale = append(stale, key+" is declared as a field GitLab sends and its record does not list, and no finding matched it: the record now lists the field, or the type no longer publishes it")
	}
	return stale
}

// Grain names which join produced a finding, spelled once.
const (
	grainPackage = "package"
	grainType    = "type"
)

// Seams for the two inputs the real tree resolves and a test cannot: loading
// the typed tool packages costs twenty seconds, and the client-go source lives
// in a module cache. Each is a variable a test restores.
var (
	collectPairings = structs.CollectOutputPairings
	readRoutes      = readSDKRoutes
)

// typedShapeCheck compares each output type with the responses of the
// operations its client-go struct models.
func typedShapeCheck(root string, index *operationIndex, published []publishedType) TypedShapeCheck {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return TypedShapeCheck{}
	}
	routes := readRoutes(pairings.ClientGoDir)
	sdkTypes := pairedSDKTypes(pairings.Outputs)

	check := TypedShapeCheck{Ran: true}
	for _, candidate := range published {
		paired := sdkTypes[[2]string{shortPackage(candidate.Package), candidate.Name}]
		if len(paired) == 0 {
			check.SkippedNoPairing++
			continue
		}
		described := describedRoutes(paired, routes, index)
		switch {
		case !described.Routed:
			check.SkippedNoRoute++
		case len(described.Known) == 0:
			check.SkippedNoSchema++
		default:
			check.Compared++
			check.Unpublished = append(check.Unpublished, unpublishedAtTypeGrain(candidate, paired, described)...)
			nested, compared := unpublishedNested(candidate, paired, described)
			check.Nested = append(check.Nested, nested...)
			check.NestedCompared += compared
		}
	}
	sortFindings(check.Unpublished)
	sortFindings(check.Nested)
	check.Unpublished, check.Nested, check.UnusedDeclarations = classifyShapeFindings(check.Unpublished, check.Nested)
	return check
}

// pairedSDKTypes indexes the client-go structs each output type models.
func pairedSDKTypes(pairings []structs.OutputPairing) map[[2]string][]string {
	out := map[[2]string][]string{}
	for _, pairing := range pairings {
		key := [2]string{pairing.Package, pairing.MCPType}
		if !slices.Contains(out[key], pairing.SDKType) {
			out[key] = append(out[key], pairing.SDKType)
		}
	}
	for key := range out {
		sort.Strings(out[key])
	}
	return out
}

// describedResponses is what GitLab's record says about the endpoints one
// output type models.
type describedResponses struct {
	// Routed is false for a type no client-go service method answers with, so
	// there is no endpoint to ask about. It is separate from an empty Known,
	// which is a type whose routes GitLab's record says nothing about: only the
	// second is a statement about the record, and folding them would hide a
	// parser regression as a document gap.
	Routed bool
	// Operations names the endpoints that were actually searched, so the count
	// a finding carries is the number of responses that failed to name the
	// field rather than the number of endpoints the type touches.
	Operations []string
	// Known is the union of the top-level property names those responses carry.
	Known map[string]bool
	// Nested is the union, per top-level property, of the property names the
	// object under it carries. A property absent from it is one no searched
	// response describes an object for.
	Nested map[string]map[string]bool
}

// describedRoutes collects the endpoints every paired client-go struct is
// answered from whose response the document actually spells, and the names it
// gives them.
func describedRoutes(paired []string, routes map[string][]sdkRoute, index *operationIndex) describedResponses {
	seen := map[string]bool{}
	described := describedResponses{Known: map[string]bool{}, Nested: map[string]map[string]bool{}}
	for _, sdkType := range paired {
		for _, route := range routes[sdkType] {
			described.Routed = true
			// Only an exact match: a loose one accepts a literal segment of
			// ours where GitLab has a placeholder, which is evidence about a
			// fixture value in the inventory and would be a guess here. It is
			// also right on its own terms for this join: a client-go route is a
			// template, so its placeholders are already placeholders, and there
			// is no fixture value in one for a loose match to be evidence about.
			operation, quality, _ := index.lookup(route.Method, route.Path)
			if quality != matchExact || len(operation.Response) == 0 {
				continue
			}
			if name := route.operation(); !seen[name] {
				seen[name] = true
				described.Operations = append(described.Operations, name)
			}
			for _, name := range operation.Response {
				described.Known[name] = true
			}
			for property, names := range operation.Nested {
				under := described.Nested[property]
				if under == nil {
					under = map[string]bool{}
					described.Nested[property] = under
				}
				for _, name := range names {
					under[name] = true
				}
			}
		}
	}
	sort.Strings(described.Operations)
	return described
}

// unpublishedAtTypeGrain reports every field of one type that no operation it
// models declares.
func unpublishedAtTypeGrain(candidate publishedType, paired []string, described describedResponses) []UnpublishedField {
	var out []UnpublishedField
	for _, field := range candidate.Fields {
		if described.Known[field] {
			continue
		}
		out = append(out, UnpublishedField{
			Grain:      grainType,
			Package:    candidate.Package,
			Type:       candidate.Name,
			Field:      field,
			SDKType:    strings.Join(paired, ", "),
			Endpoints:  len(described.Operations),
			Operations: described.Operations,
		})
	}
	return out
}

// unpublishedNested reports the same thing one level down: a field of a nested
// output type that no searched response lists among the properties of the
// object it sits under.
//
// A nested type whose property the record describes no object for is not
// compared at all, and is not counted either. That is the same reticence
// SkippedNoSchema is: an empty union means the document does not say rather
// than that the object is empty, and judging against one would condemn every
// field of a type over a hole in the record. It is why this can be run over
// every compared type without reproducing the 1418 findings that made nested
// types uncomparable in the first place.
func unpublishedNested(candidate publishedType, paired []string, described describedResponses) (found []UnpublishedField, compared int) {
	for _, tag := range sortedKeys(candidate.Nested) {
		child := candidate.Nested[tag]
		known := described.Nested[tag]
		if len(known) == 0 {
			continue
		}
		compared++
		for _, field := range child.Fields {
			if known[field] {
				continue
			}
			found = append(found, UnpublishedField{
				Grain:      grainType,
				Package:    candidate.Package,
				Type:       child.Name,
				Under:      tag,
				Field:      field,
				SDKType:    strings.Join(paired, ", "),
				Endpoints:  len(described.Operations),
				Operations: described.Operations,
			})
		}
	}
	return found, compared
}

// sortedKeys walks a map of nested types in a stable order, so two runs over
// one tree produce the same report.
func sortedKeys(nested map[string]nestedType) []string {
	keys := make([]string, 0, len(nested))
	for key := range nested {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// shortPackage reduces the repository-relative package path the published types
// carry to the domain name the pairings are keyed on.
func shortPackage(pkg string) string {
	return strings.TrimPrefix(pkg, toolsDir+"/")
}
