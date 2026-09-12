package paths

import (
	"reflect"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// PaginationCheck is R-PAGE: whether an action that hands a model a list also
// hands it the way to ask for the rest of that list.
//
// It exists because nothing else here can see the question. Every other rule of
// this audit compares a published field against client-go's struct, GitLab's
// documentation or the live entity record, and GitLab's pagination is a field of
// no entity: an offset page arrives in the X-Page, X-Next-Page, X-Per-Page,
// X-Total and X-Total-Pages response headers, and a keyset page in the Link
// header. So six dimensions were green on internal/tools/impersonationtokens,
// which answers user.list_impersonation_tokens with a bare array of tokens while
// GitLab serves twenty of them at a time. A caller cannot tell that it has one
// page, cannot ask for the second, and is not told either.
//
// The oracle for "this endpoint is paginated" is the record a booted GitLab
// produced, which carries the params every mounted route declares. A route
// taking per_page is a route whose answer is one page of something longer, and
// that is a far stronger statement than a guess from an endpoint's name.
//
// # What counts as reading a collection
//
// An action reads a collection when its output type publishes exactly one
// content field and that field is a list of objects. Framing is not content: the
// hints block every output embeds and a pagination block are this server's own
// and are taken out before the count.
//
// The strictness is the whole of it. A response that is one object carrying a
// list under it is a single-object read whose caller was never promised the
// list: a project carries shared_with_groups, a job carries artifacts, a merge
// request carries assignees. Admitting those turned 24 findings into 79, of
// which 55 were about a list nobody asked to page, and no declaration table
// would have been the right place to say so 55 times. This is where the "the
// route declares the params but the action reads a single object" case is
// answered, structurally, rather than one entry at a time.
//
// # The grain, and what it costs
//
// The join from an action to an endpoint is the package, because the inventory
// records the package that built the client and nothing on the wire names an
// action. So an action is asked about when the package that owns it was recorded
// calling an endpoint GitLab paginates, which is not the same as its own
// endpoint being one. [Summary.PaginationGrain] says so beside the number, on the
// same terms [Summary.Grain] does for the observation check.
//
// The cost is measurable and is paid in declarations: of the 24 findings on the
// pinned record, 14 name an action whose own GitLab route declares per_page and
// 10 matched a sibling's endpoint. Each of those 10 is written down in
// pagination_declarations.go with the route the record holds for it, and a
// declaration that stops matching is a finding like every other declaration table
// here. What would remove them is an inventory that named the action, which is a
// different piece of work: nothing the recorder sees today can supply it.
//
// # It reports and does not gate
//
// A finding is a surface change, and the surface is not this command's to
// change. The findings are also not uniform: some are one field on an output
// type, and some need the handler to accept page and per_page first, which is a
// different edit with a different blast radius. The gate it could become is
// named in [PaginationCheck.Unpaginated]'s own comment.
type PaginationCheck struct {
	// Ran is false when the record could not be read, which is the only way this
	// check is skipped. It is stated rather than inherited from the shape check
	// beside it: the two read the record independently, so a reader of one is
	// never told about the other's failure.
	Ran bool `json:"ran"`
	// Record names the artifact that answered, so a reader of a finding knows
	// which GitLab it speaks for.
	Record string `json:"record,omitempty"`
	// Grain is the package-grain caveat, spelled on the check as well as in the
	// summary, since a reader who opens the findings may never read the summary.
	Grain string `json:"grain,omitempty"`
	// Routes is what the record says pagination is, which is the oracle's own
	// size.
	Routes RoutePagination `json:"routes"`
	// Recorded is how much of that reached this server's own requests.
	Recorded RecordedPagination `json:"recorded"`
	// Collections counts the catalog actions whose output is a collection, and
	// how they divide.
	Collections CollectionCounts `json:"collections"`
	// Unpaginated is the finding list: an action reading a collection, in a
	// package recorded calling a paginated endpoint, publishing no pagination.
	Unpaginated []UnpaginatedCollection `json:"unpaginated,omitempty"`
}

// RoutePagination is what the live record says about pagination, before any of
// it is joined to anything.
type RoutePagination struct {
	// Routes is every mounted route the record holds.
	Routes int `json:"routes"`
	// Offset declares page and per_page: the answer's headers carry the page,
	// the total and the pages either side of it.
	Offset int `json:"offset_paginated"`
	// Keyset declares per_page without page, so the next page is asked for with
	// the cursor or page_token the Link header carries. A page-and-total block
	// is the wrong shape to publish for one of these, which is why they are
	// counted apart rather than folded in.
	Keyset int `json:"keyset_paginated"`
}

// RecordedPagination is how much of the record's pagination this server was
// recorded reaching.
type RecordedPagination struct {
	// Endpoints is the distinct paginated endpoints the inventory holds, offset
	// and keyset apart.
	OffsetEndpoints int `json:"offset_endpoints"`
	KeysetEndpoints int `json:"keyset_endpoints"`
	// Packages is how many packages called at least one of each, which is the
	// grain everything below is asked at.
	OffsetPackages int `json:"offset_packages"`
	KeysetPackages int `json:"keyset_packages"`
}

// CollectionCounts is how the collection-reading actions divide.
type CollectionCounts struct {
	// Actions is every read-only action whose output is a collection envelope.
	Actions int `json:"actions"`
	// Paginated publish a pagination block, whatever their endpoint does.
	Paginated int `json:"publishing_pagination"`
	// Unasked publish none and sit in a package recorded calling no paginated
	// endpoint, so this check has nothing to say about them. They are counted
	// rather than reported, because the silence is about the inventory as much
	// as about the action.
	Unasked int `json:"not_asked_about"`
	// Unpaginated publish none and sit in a package that was recorded calling
	// one, which is the finding.
	Unpaginated int `json:"without_pagination"`
	// Undeclared is the half of those that no declaration accounts for, which is
	// what a reader is being asked to act on and what a gate would fail on.
	Undeclared int `json:"without_pagination_undeclared"`
}

// UnpaginatedCollection is one action that hands a model a list and no way to
// ask for the rest of it.
//
// Gating on these needs three things, none of them here. The package grain has
// to become the action grain, or every finding it produces has to be declared,
// which is the state the endpoint check reached before it was allowed to fail a
// run. The 14 real findings have to be fixed, and fixing them is a surface
// change this command does not make. And the fix for several is not a field: an
// action whose handler never sends page or per_page cannot publish a page number
// it did not ask for, so RequestPaginates is on each finding to say which of the
// two edits it is.
type UnpaginatedCollection struct {
	// Action is the canonical catalog ID.
	Action string `json:"action"`
	// Package owns it, spelled as the inventory spells it.
	Package string `json:"package"`
	// Type is the Go output type, and Collection the json name of the list it
	// publishes.
	Type       string `json:"type"`
	Collection string `json:"collection"`
	// RequestPaginates is whether the action's own input schema offers page or
	// per_page. False means the handler asks GitLab for the default page and
	// publishes it as though it were everything, so the fix is the input as well
	// as the output.
	RequestPaginates bool `json:"request_paginates"`
	// Endpoints are the paginated endpoints this action's package was recorded
	// calling. They are the evidence and not the claim: at package grain one of
	// them may belong to a sibling action, which is what the declarations say
	// where it is so.
	Endpoints []string `json:"endpoints"`
	// KeysetEndpoints are the cursor-paginated ones the same package called,
	// listed apart because their answer is a cursor rather than a page number.
	KeysetEndpoints []string `json:"keyset_endpoints,omitempty"`
	// Category and Reason are the declaration accounting for this finding, empty
	// for one nothing accounts for.
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for this finding.
func (c UnpaginatedCollection) declared() bool { return c.Category != "" }

// paginationGrain is what [PaginationCheck.Grain] says, spelled once.
const paginationGrain = "package: an action is asked about when the package that owns it was recorded calling an endpoint GitLab paginates, " +
	"not when its own endpoint was seen. The endpoints on a finding are that package's, and one of them may belong to a sibling action."

// paginationShapes are the blocks an output publishes to say where the page it
// carries sits in the whole.
//
// They are named as types rather than matched by name, so that renaming one in
// internal/toolutil breaks this build instead of quietly emptying the set and
// turning every paginated action into a finding.
var paginationShapes = map[reflect.Type]bool{
	reflect.TypeFor[toolutil.PaginationOutput]():               true,
	reflect.TypeFor[toolutil.GraphQLPaginationOutput]():        true,
	reflect.TypeFor[toolutil.GraphQLForwardPaginationOutput](): true,
	reflect.TypeFor[toolutil.GraphQLPageInfo]():                true,
}

// hintsShape is the block every output embeds that carries the server's own
// next-step hints. It is framing rather than content, so it is taken out before
// a collection envelope is counted.
var hintsShape = reflect.TypeFor[toolutil.HintableOutput]()

// embedDepth bounds the walk through embedded structs. Go forbids a struct from
// embedding itself by value, but a pointer embed can cycle, and a walk that
// cannot end is worse than one that stops short: six levels is deeper than any
// output type here and is reached by none of them.
const embedDepth = 6

// outputShape is what one output type publishes, in the two terms this rule
// asks about.
type outputShape struct {
	// Pagination names the block found, empty for a type carrying none.
	Pagination string
	// Collection is the json name of the single list the type publishes, empty
	// unless the type is a collection envelope.
	Collection string
}

// isCollection reports whether the type is the envelope of a list.
func (s outputShape) isCollection() bool { return s.Collection != "" }

// inspectOutput reads an action's output type the way encoding/json will write
// it: an untagged embed's fields are promoted into the type around it, and a
// field the tag hides publishes nothing.
//
// A pagination block found anywhere in that walk counts, embedded or named,
// because either way the key reaches the model.
func inspectOutput(outputType reflect.Type) outputShape {
	root := dereference(outputType)
	if root == nil || root.Kind() != reflect.Struct {
		return outputShape{}
	}

	var walk outputWalk
	walk.visit(root, 0)

	// One list and nothing else beside it is the envelope of a collection.
	// Anything else is an object that happens to carry a list, whose caller
	// asked for the object.
	if walk.lists != 1 || walk.others != 0 {
		walk.shape.Collection = ""
	}
	return walk.shape
}

// outputWalk is what one pass over an output type has found so far: the shape
// being assembled, and the tally that decides whether the type is an envelope.
//
// The tally cannot live on the shape, because it is not part of the answer: a
// reader of a finding needs the list's name and the pagination block, and the
// count of scalars beside them is the working the rule showed itself.
type outputWalk struct {
	shape  outputShape
	lists  int
	others int
}

// visit reads one struct's fields, following the untagged embeds that promote
// their fields into it.
func (w *outputWalk) visit(structType reflect.Type, depth int) {
	if depth > embedDepth {
		return
	}
	for field := range structType.Fields() {
		w.field(field, depth)
	}
}

// field classifies one field as framing, an embed to follow, the list, or
// content beside it.
func (w *outputWalk) field(field reflect.StructField, depth int) {
	fieldType := dereference(field.Type)
	if fieldType == nil {
		return
	}
	if paginationShapes[fieldType] {
		if w.shape.Pagination == "" {
			w.shape.Pagination = fieldType.Name()
		}
		return
	}
	if field.Anonymous && fieldType.Kind() == reflect.Struct {
		if fieldType != hintsShape {
			w.visit(fieldType, depth+1)
		}
		return
	}
	name, published := publishedName(field)
	if !published {
		return
	}
	if listOfObjects(fieldType) {
		w.lists++
		if w.shape.Collection == "" {
			w.shape.Collection = name
		}
		return
	}
	w.others++
}

// publishedName is the json key a field reaches a model under, and whether it
// reaches one at all.
func publishedName(field reflect.StructField) (string, bool) {
	if field.PkgPath != "" {
		return "", false
	}
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return "", false
	}
	if name == "" {
		return field.Name, true
	}
	return name, true
}

// listOfObjects reports whether the type is a list whose elements are objects,
// which is the shape a page of GitLab's answer takes. A list of strings is a
// value rather than a collection a caller pages through: a project's topics are
// not a page of topics.
func listOfObjects(fieldType reflect.Type) bool {
	if fieldType.Kind() != reflect.Slice {
		return false
	}
	element := dereference(fieldType.Elem())
	return element != nil && element.Kind() == reflect.Struct
}

// dereference follows pointers to the type underneath, and returns nil for a
// nil type so that a route registered without one is skipped rather than
// panicking the audit.
func dereference(t reflect.Type) reflect.Type {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// requestPaginates reports whether an action offers a caller page or per_page,
// which is what tells a finding that needs a field from one that needs a
// parameter first.
func requestPaginates(schema map[string]any) bool {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, page := properties["page"]
	_, perPage := properties["per_page"]
	return page || perPage
}

// paginationCheck asks, of every action that reads a collection, whether it
// publishes the pagination the endpoint its package calls has.
//
// It reads the record itself rather than taking the shape check's copy. The two
// are independent questions over one artifact, each says whether it ran, and a
// reader of one finding is never left inferring the other's state from it; the
// second read of a 2.6 MB file is the price and it is paid once per run.
func paginationCheck(root string, requests []requestinventory.Row, actions []requestinventory.Action) PaginationCheck {
	record, err := apilive.Read(recordDir(root))
	if err != nil {
		return PaginationCheck{Ran: false}
	}

	check := PaginationCheck{Ran: true, Record: apilive.FileName, Grain: paginationGrain}
	for _, route := range record.Routes {
		check.Routes.Routes++
		switch offset, keyset := routePagination(route); {
		case offset:
			check.Routes.Offset++
		case keyset:
			check.Routes.Keyset++
		}
	}

	index := newOperationIndex(record)
	offsetByPackage, keysetByPackage := paginatedEndpoints(index, requests)
	for _, endpoints := range offsetByPackage {
		check.Recorded.OffsetEndpoints += len(endpoints)
	}
	for _, endpoints := range keysetByPackage {
		check.Recorded.KeysetEndpoints += len(endpoints)
	}
	check.Recorded.OffsetPackages = len(offsetByPackage)
	check.Recorded.KeysetPackages = len(keysetByPackage)

	check.Collections, check.Unpaginated = unpaginatedCollections(actions, offsetByPackage, keysetByPackage)
	return check
}

// paginatedEndpoints indexes, per package, the distinct endpoints it was
// recorded calling that GitLab paginates.
//
// A row the index cannot look up contributes nothing. That is the same silence
// the shape check's join reports rather than a new one, and it can only lose a
// finding: an endpoint nobody could resolve is one nobody can say paginates.
func paginatedEndpoints(index *operationIndex, requests []requestinventory.Row) (offset, keyset map[string][]string) {
	offsetSeen := map[string]map[string]bool{}
	keysetSeen := map[string]map[string]bool{}
	for _, request := range requests {
		if !strings.EqualFold(request.Kind, requestinventory.KindREST) {
			continue
		}
		operation, quality, _ := index.lookup(request.Method, request.Path)
		if quality == matchNone {
			continue
		}
		endpoint := request.Method + " " + request.Path
		if operation.Offset {
			noteEndpoint(offsetSeen, request.Package, endpoint)
		}
		if operation.Keyset {
			noteEndpoint(keysetSeen, request.Package, endpoint)
		}
	}
	return sortedEndpoints(offsetSeen), sortedEndpoints(keysetSeen)
}

// noteEndpoint records one endpoint against one package, once.
func noteEndpoint(seen map[string]map[string]bool, pkg, endpoint string) {
	endpoints := seen[pkg]
	if endpoints == nil {
		endpoints = map[string]bool{}
		seen[pkg] = endpoints
	}
	endpoints[endpoint] = true
}

// sortedEndpoints renders the per-package sets as the sorted lists a finding
// carries, so a report is the same between runs.
func sortedEndpoints(seen map[string]map[string]bool) map[string][]string {
	out := make(map[string][]string, len(seen))
	for pkg, endpoints := range seen {
		list := make([]string, 0, len(endpoints))
		for endpoint := range endpoints {
			list = append(list, endpoint)
		}
		sort.Strings(list)
		out[pkg] = list
	}
	return out
}

// unpaginatedCollections holds every collection-reading action against the
// endpoints its package was recorded calling.
func unpaginatedCollections(actions []requestinventory.Action, offset, keyset map[string][]string) (CollectionCounts, []UnpaginatedCollection) {
	var counts CollectionCounts
	found := make([]UnpaginatedCollection, 0)
	for _, action := range actions {
		if !action.ReadOnly {
			continue
		}
		shape := inspectOutput(action.Route.OutputType)
		if !shape.isCollection() {
			continue
		}
		counts.Actions++
		if shape.Pagination != "" {
			counts.Paginated++
			continue
		}
		pkg := requestinventory.PackageName(action.Owner)
		endpoints := offset[pkg]
		if len(endpoints) == 0 {
			counts.Unasked++
			continue
		}
		counts.Unpaginated++
		outputType := dereference(action.Route.OutputType)
		found = append(found, UnpaginatedCollection{
			Action:           action.ID,
			Package:          pkg,
			Type:             outputType.Name(),
			Collection:       shape.Collection,
			RequestPaginates: requestPaginates(action.Route.InputSchema),
			Endpoints:        endpoints,
			KeysetEndpoints:  keyset[pkg],
		})
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Package != found[j].Package {
			return found[i].Package < found[j].Package
		}
		return found[i].Action < found[j].Action
	})
	found = classifyUnpaginated(found)
	for _, finding := range found {
		if !finding.declared() {
			counts.Undeclared++
		}
	}
	return counts, found
}

// keepUndeclaredCollections drops the findings a declaration accounts for,
// which is what -gaps-only asks for.
func keepUndeclaredCollections(found []UnpaginatedCollection) []UnpaginatedCollection {
	kept := make([]UnpaginatedCollection, 0)
	for _, finding := range found {
		if !finding.declared() {
			kept = append(kept, finding)
		}
	}
	return kept
}
