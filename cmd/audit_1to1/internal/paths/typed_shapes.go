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
	// Projections names, as "package.Type", sorted, the types among Compared
	// that no converter pairs and a handler builds field by field out of a
	// client-go struct (see [structs.ProjectionPairing]). Each is judged
	// against the endpoints of the service methods whose answer it is built
	// from, found in the function holding the literal or in the package's
	// functions calling it, rather than against every endpoint answering with
	// the struct, because a compact row is read from one endpoint and a
	// finding about it has to name that one.
	//
	// These are the milestone's issue rows, the resource group's job rows and
	// the rest of the compact projections that nothing paired before, whose
	// gaps only the package grain saw, under two thousand findings of other
	// kinds.
	Projections []string `json:"projections,omitempty"`
	// Envelopes names, as "package.Type", sorted, the output types that carry
	// no pairing because they are this server's packaging around payloads that
	// have one, and whose every payload was compared, so the response each
	// packages was judged under its payload's name. They were counted among
	// the types without a pairing, which read as a response nobody judged
	// while its payload was being judged a line below.
	//
	// Packaging around a payload that is paired and then skipped, for want of
	// a route or of a response schema, is not here: it is counted in that
	// payload's skip, since the response it packages was judged no more than
	// the payload was. Listing it here would call judged a response nobody
	// compared.
	Envelopes []string `json:"envelopes,omitempty"`
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

// Seams for the inputs the real tree resolves and a test cannot: loading the
// typed tool packages costs twenty seconds, and the client-go source lives in
// a module cache. Each is a variable a test restores.
var (
	collectPairings  = structs.CollectOutputPairings
	readRoutes       = readSDKRoutes
	readMethodRoutes = readSDKMethodRoutes
)

// typeJoin is everything the type grain reads once per run and consults per
// output type.
type typeJoin struct {
	index        *operationIndex
	conditions   *conditionIndex
	routes       map[string][]sdkRoute
	methodRoutes map[string][]sdkRoute
	sdkTypes     map[[2]string][]string
	projected    map[[2]string][]structs.ProjectionPairing
	sdkFields    map[string]map[string]bool
}

// typedShapeCheck compares each output type with the responses of the
// operations its client-go struct models.
func typedShapeCheck(root string, index *operationIndex, conditions *conditionIndex, published []publishedType) TypedShapeCheck {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return TypedShapeCheck{}
	}
	join := typeJoin{
		index:        index,
		conditions:   conditions,
		routes:       readRoutes(pairings.ClientGoDir),
		methodRoutes: readMethodRoutes(pairings.ClientGoDir),
		sdkTypes:     pairedSDKTypes(pairings.Outputs),
		projected:    projectionsByType(pairings.Outputs, pairings.Projections),
		sdkFields:    sdkFieldsByType(pairings.Outputs, pairings.Projections),
	}

	check := TypedShapeCheck{Ran: true}
	outcomes := map[[2]string]typeOutcome{}
	var envelopes []publishedType
	for _, candidate := range published {
		if join.envelope(candidate) {
			// Decided once every payload has been judged, since what the
			// envelope is counted as is what became of them.
			envelopes = append(envelopes, candidate)
			continue
		}
		outcomes[[2]string{shortPackage(candidate.Package), candidate.Name}] = join.judge(&check, candidate)
	}
	for _, candidate := range envelopes {
		countEnvelope(&check, candidate, outcomes)
	}
	sortFindings(check.Unpublished)
	sortFindings(check.Nested)
	sortUnsurfaced(check.Unsurfaced)
	for _, names := range [][]string{check.Skipped.NoPairing, check.Skipped.NoRoute, check.Skipped.NoSchema, check.Envelopes, check.Projections} {
		sort.Strings(names)
	}
	check.Unpublished, check.Nested, check.UnusedDeclarations = classifyShapeFindings(check.Unpublished, check.Nested)
	return check
}

// typeOutcome is what the type grain did with one output type, which is what
// an envelope around it is counted by.
type typeOutcome int

// The outcomes of [typeJoin.judge]. Only a paired type reaches the last three,
// and a payload an envelope is decided by is always paired.
const (
	outcomePassedOver typeOutcome = iota
	outcomeNoPairing
	outcomeCompared
	outcomeNoRoute
	outcomeNoSchema
)

// judge decides what the type grain does with one output type that is not an
// envelope: pass it over, count it, or compare it. It returns what it did.
func (join typeJoin) judge(c *TypedShapeCheck, candidate publishedType) typeOutcome {
	named := shortPackage(candidate.Package) + "." + candidate.Name
	paired, projections := join.pairing(candidate)
	if candidate.Inner && !candidate.Payload {
		// A reference to another resource sitting inside a response, not a
		// response. Its pairing names the struct of the whole entity, so
		// judging it here would hold a job's project reference to what
		// GET /projects/:id answers with and report all eighty-five fields of
		// a project as missing from it. The nested pass asks the only question
		// that fits, against the property it sits under.
		return outcomePassedOver
	}
	if len(paired) > 0 {
		return join.compare(c, candidate, named, paired, projections)
	}
	if candidate.Payload {
		// Wrapped and unpaired: the envelope is counted a skip under its own
		// name, and counting the payload again would double one response.
		return outcomePassedOver
	}
	c.SkippedNoPairing++
	c.Skipped.NoPairing = append(c.Skipped.NoPairing, named)
	return outcomeNoPairing
}

// pairing names the client-go structs a type models, through a converter or,
// failing one, through the projections a handler builds it by, and those
// projections.
func (join typeJoin) pairing(candidate publishedType) ([]string, []structs.ProjectionPairing) {
	key := [2]string{shortPackage(candidate.Package), candidate.Name}
	projections := join.projected[key]
	if paired := join.sdkTypes[key]; len(paired) > 0 {
		return paired, projections
	}
	return projectedSDKTypes(projections), projections
}

// envelope reports whether a type is packaging with no pairing of its own
// around payloads that every one have one: a top-level type, since a type some
// struct names is either a payload or a reference, whose response is judged
// under its payloads' names. Such a type cannot be counted until they have
// been judged.
func (join typeJoin) envelope(candidate publishedType) bool {
	if candidate.Inner || candidate.Payload {
		return false
	}
	if paired, _ := join.pairing(candidate); len(paired) > 0 {
		return false
	}
	return wrapsOnlyPaired(candidate, join.sdkTypes, join.projected)
}

// countEnvelope counts packaging by what became of the payloads it carries.
// It is listed among the envelopes when every payload was compared, and is
// otherwise counted in the skip of the first payload, in its sorted list, that
// was not: the response it packages was judged no more than that payload was,
// and listing it among the envelopes would call it judged.
func countEnvelope(c *TypedShapeCheck, candidate publishedType, outcomes map[[2]string]typeOutcome) {
	named := shortPackage(candidate.Package) + "." + candidate.Name
	for _, payload := range candidate.Wraps {
		switch outcomes[[2]string{shortPackage(candidate.Package), payload}] {
		case outcomeNoRoute:
			c.SkippedNoRoute++
			c.Skipped.NoRoute = append(c.Skipped.NoRoute, named)
			return
		case outcomeNoSchema:
			c.SkippedNoSchema++
			c.Skipped.NoSchema = append(c.Skipped.NoSchema, named)
			return
		}
	}
	c.Envelopes = append(c.Envelopes, named)
}

// compare holds one paired output type against the responses of the endpoints
// its pairing names, or counts it among the skips when there is nothing to
// hold it against, and says which it did.
func (join typeJoin) compare(c *TypedShapeCheck, candidate publishedType, named string, paired []string, projections []structs.ProjectionPairing) typeOutcome {
	var described describedResponses
	if len(projections) > 0 {
		described = describedProjections(projections, join.routes, join.methodRoutes, join.index)
	} else {
		described = describedRoutes(paired, join.routes, join.index)
	}
	if !described.Routed {
		c.SkippedNoRoute++
		c.Skipped.NoRoute = append(c.Skipped.NoRoute, named)
		return outcomeNoRoute
	}
	if len(described.Known) == 0 {
		c.SkippedNoSchema++
		c.Skipped.NoSchema = append(c.Skipped.NoSchema, named)
		return outcomeNoSchema
	}
	c.Compared++
	// Counted here rather than before the two skips above, because it is
	// documented as how many of Compared were reached through an envelope. An
	// inner payload whose endpoints have no route or no response is a skip
	// like any other, and counting it earlier would report a subset larger
	// than the set it is a subset of.
	if candidate.Inner {
		c.ComparedInner++
	}
	if len(projections) > 0 {
		c.Projections = append(c.Projections, named)
	}
	c.Unpublished = append(c.Unpublished, unpublishedAtTypeGrain(candidate, paired, described)...)
	nested, compared := unpublishedNested(candidate, paired, described)
	c.Nested = append(c.Nested, nested...)
	c.NestedCompared += compared
	c.Unsurfaced = append(c.Unsurfaced, unsurfacedAtTypeGrain(candidate, paired, join.sdkFields, described, join.conditions)...)
	return outcomeCompared
}

// sdkFieldsByType indexes what each client-go struct deserializes, which is
// what the upstream half of a sent finding is judged against.
//
// Keyed by the struct's name rather than by the pairing, because the question
// is about client-go and not about which of our types happens to model it: two
// packages modeling one SDK struct must get the same answer, and the struct
// carries what it carries. A projection names its struct's fields the same way
// a converter pairing does, so both lists feed one index.
func sdkFieldsByType(pairings []structs.OutputPairing, projections []structs.ProjectionPairing) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	add := func(sdkType string, names []string) {
		fields := out[sdkType]
		if fields == nil {
			fields = make(map[string]bool, len(names))
			out[sdkType] = fields
		}
		for _, name := range names {
			fields[name] = true
		}
	}
	for _, pairing := range pairings {
		add(pairing.SDKType, pairing.SDKFields)
	}
	for _, projection := range projections {
		add(projection.SDKType, projection.SDKFields)
	}
	return out
}

// projectionsByType indexes the projection pairings by package and type, for
// the types no converter pairs. A type a converter pairs keeps the endpoints it
// has always been judged against, every one answering with its struct: that
// pairing says the type models the struct wherever it is answered, which is a
// wider claim than a literal in one handler makes, and narrowing it here would
// silently drop the endpoints its other callers read.
func projectionsByType(outputs []structs.OutputPairing, projections []structs.ProjectionPairing) map[[2]string][]structs.ProjectionPairing {
	converted := make(map[[2]string]bool, len(outputs))
	for _, pairing := range outputs {
		converted[[2]string{pairing.Package, pairing.MCPType}] = true
	}
	out := map[[2]string][]structs.ProjectionPairing{}
	for _, projection := range projections {
		key := [2]string{projection.Package, projection.MCPType}
		if converted[key] {
			continue
		}
		out[key] = append(out[key], projection)
	}
	return out
}

// projectedSDKTypes names the client-go structs a type's projections read,
// sorted and each once.
func projectedSDKTypes(projections []structs.ProjectionPairing) []string {
	var out []string
	for _, projection := range projections {
		if !slices.Contains(out, projection.SDKType) {
			out = append(out, projection.SDKType)
		}
	}
	sort.Strings(out)
	return out
}

// wrapsOnlyPaired reports whether a type is packaging whose every payload is
// paired, by a converter or by a projection, so that the response it wraps is
// judged under the payload's name.
func wrapsOnlyPaired(candidate publishedType, sdkTypes map[[2]string][]string, projected map[[2]string][]structs.ProjectionPairing) bool {
	if len(candidate.Wraps) == 0 {
		return false
	}
	pkg := shortPackage(candidate.Package)
	for _, payload := range candidate.Wraps {
		key := [2]string{pkg, payload}
		if len(sdkTypes[key]) == 0 && len(projected[key]) == 0 {
			return false
		}
	}
	return true
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
	lists := make([][]sdkRoute, 0, len(paired))
	for _, sdkType := range paired {
		lists = append(lists, routes[sdkType])
	}
	return describe(lists, index)
}

// describedProjections is the same for a type built out of client-go structs
// without a converter: the endpoints of the methods each projection is built
// from, and the endpoints of the struct itself for a projection whose method
// was not found, which is what a converter pairing would have searched.
func describedProjections(projections []structs.ProjectionPairing, routes, methodRoutes map[string][]sdkRoute, index *operationIndex) describedResponses {
	var lists [][]sdkRoute
	for _, projection := range projections {
		if len(projection.Methods) == 0 {
			lists = append(lists, routes[projection.SDKType])
			continue
		}
		for _, method := range projection.Methods {
			lists = append(lists, methodRoutes[method])
		}
	}
	return describe(lists, index)
}

// describe reads what GitLab's document says about each route of each list, in
// order, which is the order an entity is credited to a key by.
func describe(lists [][]sdkRoute, index *operationIndex) describedResponses {
	seen := map[string]bool{}
	described := describedResponses{EntityOf: map[string]string{}, Known: map[string]bool{}, Nested: map[string]map[string]bool{}}
	for _, list := range lists {
		for _, route := range list {
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
		// for does not expose all of them. Every key of a response has one,
		// since the record only lists a key it read on an entity, so the first
		// response carrying the key is the one that names it.
		if d.EntityOf[name] == "" {
			d.EntityOf[name] = operation.EntityOf[name]
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
