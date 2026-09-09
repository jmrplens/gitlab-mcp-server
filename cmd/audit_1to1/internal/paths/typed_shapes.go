package paths

import (
	"slices"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/structs"
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
	// ComparedInner counts how many of [TypedShapeCheck.Compared] are types
	// some struct of their package names as a field, rather than a response
	// this repository returns whole.
	//
	// They are compared because a converter pairs them with a client-go
	// struct, which is the only thing this grain needs: that struct's service
	// methods name the endpoints, and GitLab's document describes what those
	// send. Being named as somebody's field says nothing about whether GitLab
	// answers with the object.
	//
	// Excluding them was the reason this grain saw 26 types out of 441. The
	// convention here is a one-key envelope, so `GetProjectOutput` is
	// `{badge: BadgeItem}` and it is `BadgeItem` that models the response and
	// carries the pairing. The envelope has none and was counted a skip; the
	// modeling type was passed over for being named as its field; and no
	// finding about that endpoint's response could be made at the sharp grain
	// at all.
	ComparedInner int `json:"compared_inner"`
	// SkippedNoPairing counts the top-level output types no converter pairs
	// with a client-go struct. They are our own wrappers around a JSON array,
	// our own answers to a 204 and to a not-found, and the synthetic results
	// of a handler that calls nothing: no endpoint sends their keys because
	// they are not an endpoint's response, and the package-grain join already
	// carries them as the lower bound it is.
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
	// Unsurfaced are the reverse findings at this grain: a response field the
	// operations a type models declare that the type does not publish, each
	// with what the conditions record says about when GitLab sends it. This
	// is the list the field-by-field review reads, since it names the type
	// and the endpoints rather than a package's union of them.
	Unsurfaced []UnsurfacedField `json:"unsurfaced,omitempty"`
	// UnusedDeclarations names the shape declarations that matched no finding
	// in this run, sorted. Each is a claim about GitLab's record that no longer
	// describes it.
	UnusedDeclarations []string `json:"unused_declarations,omitempty"`
	// Skipped names the types behind the three counters above, as
	// "package.Type", sorted.
	//
	// The counters alone say how much this grain declined to judge and nothing
	// about whether declining was right, which is the only question a reader
	// has when the sharp grain compares 26 types and the blunt one reports
	// hundreds of fields. Named, the same numbers answer it: a NoPairing list
	// that is wrappers and delete results is the concession its comment claims,
	// and one carrying a package's real response type is a hole in the pairing
	// that suppresses every finding about it, silently and with no declaration
	// anywhere. Reported rather than gated, because none of the three is a
	// defect on its own.
	Skipped SkippedTypes `json:"skipped,omitzero"`
}

// SkippedTypes names the output types each skip bucket holds.
type SkippedTypes struct {
	// NoPairing is every type no converter pairs with a client-go struct.
	NoPairing []string `json:"no_pairing,omitempty"`
	// NoRoute is every type whose client-go struct no service method answers
	// with.
	NoRoute []string `json:"no_route,omitempty"`
	// NoSchema is every type whose routes GitLab's document describes no
	// response for.
	NoSchema []string `json:"no_schema,omitempty"`
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
func typedShapeCheck(root string, index *operationIndex, conditions *conditionIndex, published []publishedType) TypedShapeCheck {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return TypedShapeCheck{}
	}
	routes := readRoutes(pairings.ClientGoDir)
	sdkTypes := pairedSDKTypes(pairings.Outputs)
	sdkFields := sdkFieldsByType(pairings.Outputs)

	check := TypedShapeCheck{Ran: true}
	for _, candidate := range published {
		named := shortPackage(candidate.Package) + "." + candidate.Name
		paired := sdkTypes[[2]string{shortPackage(candidate.Package), candidate.Name}]
		if candidate.Inner && !candidate.Payload {
			// A reference to another resource sitting inside a response, not a
			// response. Its pairing names the struct of the whole entity, so
			// judging it here would hold a job's project reference to what
			// GET /projects/:id answers with and report all eighty-five fields
			// of a project as missing from it. The nested pass asks the only
			// question that fits, against the property it sits under.
			continue
		}
		if len(paired) == 0 && candidate.Payload {
			// Wrapped and unpaired: the envelope was already counted a skip
			// under its own name, and counting the payload again would double
			// one response.
			continue
		}
		if len(paired) == 0 {
			check.SkippedNoPairing++
			check.Skipped.NoPairing = append(check.Skipped.NoPairing, named)
			continue
		}
		described := describedRoutes(paired, routes, index)
		switch {
		case !described.Routed:
			check.SkippedNoRoute++
			check.Skipped.NoRoute = append(check.Skipped.NoRoute, named)
		case len(described.Known) == 0:
			check.SkippedNoSchema++
			check.Skipped.NoSchema = append(check.Skipped.NoSchema, named)
		default:
			check.Compared++
			// Counted here rather than before the switch, because it is
			// documented as how many of Compared were reached through an
			// envelope. An inner payload whose endpoints have no route or no
			// response is a skip like any other, and counting it above would
			// report a subset larger than the set it is a subset of.
			if candidate.Inner {
				check.ComparedInner++
			}
			check.Unpublished = append(check.Unpublished, unpublishedAtTypeGrain(candidate, paired, described)...)
			nested, compared := unpublishedNested(candidate, paired, described)
			check.Nested = append(check.Nested, nested...)
			check.NestedCompared += compared
			check.Unsurfaced = append(check.Unsurfaced, unsurfacedAtTypeGrain(candidate, paired, sdkFields, described, conditions)...)
		}
	}
	sortFindings(check.Unpublished)
	sortFindings(check.Nested)
	sortUnsurfaced(check.Unsurfaced)
	for _, names := range [][]string{check.Skipped.NoPairing, check.Skipped.NoRoute, check.Skipped.NoSchema} {
		sort.Strings(names)
	}
	check.Unpublished, check.Nested, check.UnusedDeclarations = classifyShapeFindings(check.Unpublished, check.Nested)
	return check
}

// pairedSDKTypes indexes the client-go structs each output type models.
// sdkFieldsByType indexes what each client-go struct deserializes, which is
// what the upstream half of a sent finding is judged against.
//
// Keyed by the struct's name rather than by the pairing, because the question
// is about client-go and not about which of our types happens to model it: two
// packages modeling one SDK struct must get the same answer, and the struct
// carries what it carries.
func sdkFieldsByType(pairings []structs.OutputPairing) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, pairing := range pairings {
		fields := out[pairing.SDKType]
		if fields == nil {
			fields = make(map[string]bool, len(pairing.SDKFields))
			out[pairing.SDKType] = fields
		}
		for _, name := range pairing.SDKFields {
			fields[name] = true
		}
	}
	return out
}

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
	// EntityOf maps each top-level property to the component the first
	// searched response carrying it resolved to, which is what the conditions
	// record is asked about for a field the type lacks. Per property rather
	// than per type, because the responses of one type's routes resolve to
	// different components: the fingerprint lookup of a key is documented as
	// answering with a user, and holding the key's own fields to that
	// component left the three the key type lacked unknown.
	EntityOf map[string]string
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
	described := describedResponses{EntityOf: map[string]string{}, Known: map[string]bool{}, Nested: map[string]map[string]bool{}}
	for _, sdkType := range paired {
		for _, route := range routes[sdkType] {
			described.Routed = true
			// Only an exact match: a loose one accepts a literal segment of
			// ours where GitLab has a placeholder, which is evidence about a
			// fixture value in the inventory and would be a guess here. It is
			// also right on its own terms for this join: a client-go route is a
			// template, so its placeholders are already placeholders, and there
			// is no fixture value in one for a loose match to be evidence about.
			answer, quality, _ := index.lookup(route.Method, route.Path)
			if quality != matchExact || len(answer.Response) == 0 {
				continue
			}
			if name := route.operation(); !seen[name] {
				seen[name] = true
				described.Operations = append(described.Operations, name)
			}
			described.absorb(answer)
		}
	}
	sort.Strings(described.Operations)
	return described
}

// absorb adds what one searched response says: the component it named, when
// none was named yet, its top-level property names, and the names under each
// property that carries an object.
func (d *describedResponses) absorb(operation operation) {
	for _, name := range operation.Response {
		d.Known[name] = true
		// The entity this key came from, not the operation's own: a shape that
		// merged two routes answers for keys of both, and the one it is named
		// for does not expose all of them.
		if from := operation.EntityOf[name]; from != "" && d.EntityOf[name] == "" {
			d.EntityOf[name] = from
		}
	}
	for property, names := range operation.Nested {
		under := d.Nested[property]
		if under == nil {
			under = map[string]bool{}
			d.Nested[property] = under
		}
		for _, name := range names {
			under[name] = true
		}
	}
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

// unsurfacedAtTypeGrain reports every response field the operations a type
// models declare that the type does not publish, with what the conditions
// record says about each.
func unsurfacedAtTypeGrain(candidate publishedType, paired []string, sdkFields map[string]map[string]bool, described describedResponses, conditions *conditionIndex) []UnsurfacedField {
	published := make(map[string]bool, len(candidate.Fields))
	for _, field := range candidate.Fields {
		published[field] = true
	}
	var out []UnsurfacedField
	for name := range described.Known {
		if published[name] {
			continue
		}
		finding := UnsurfacedField{
			Grain:      grainType,
			Package:    candidate.Package,
			Type:       candidate.Name,
			Field:      name,
			Operations: described.Operations,
			Entity:     described.EntityOf[name],
			SDKType:    strings.Join(paired, "|"),
			SDKModels:  modeledBySDK(paired, sdkFields, name),
		}
		conditions.annotate(&finding)
		out = append(out, finding)
	}
	return out
}

// modeledBySDK reports whether any client-go struct the type models carries
// this key.
//
// Any rather than all, because a type modeling several structs publishes the
// union of them: a key one of them carries is a key the SDK can already give
// us, and an upstream contribution would have nothing to add.
func modeledBySDK(paired []string, sdkFields map[string]map[string]bool, name string) bool {
	for _, sdkType := range paired {
		if sdkFields[sdkType][name] {
			return true
		}
	}
	return false
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
