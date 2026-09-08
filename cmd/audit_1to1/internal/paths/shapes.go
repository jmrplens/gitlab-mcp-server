package paths

import (
	"maps"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apishapes"
	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// ShapeCheck is what GitLab's own OpenAPI document says about the endpoints we
// were recorded calling.
//
// It answers a question no other rule in this repository can. The five original
// dimensions compare our types against client-go's, which models what the SDK
// carries rather than what GitLab serves; the endpoint comparison asks only
// whether a path exists. This asks what the answer at that path contains, from
// the document GitLab generates out of the code that renders it.
//
// What it says is an upper bound rather than an answer, because Grape renders a
// conditional expose (`expose :x, if: ->(_, options) { options[...] }`) only
// when the route passes the option, and the generator that writes the record
// cannot see the condition. A field in the record therefore proves that the
// entity can render it and nothing about a given route, so a decision about one
// endpoint's response is only settled by reading the entity's condition or a
// live answer. The epics domain published `subscribed` and `reference` on that
// mistake, both of them in the record and neither ever sent.
//
// It is asked at two grains, both reported and neither gating.
//
// [ShapeCheck.Unpublished] is the package grain: a package's recorded endpoints
// are unioned and every top-level output type of that package is held against
// the union. It is lossy in a way that can only produce a missing finding,
// never a false one (see [ShapeCheck.Join] and [unpublishedFields]), and it
// finds 610 fields across 130 packages, most of which are not phantoms. Three
// shapes dominate: our own wrappers around a JSON array, whose keys no endpoint
// can send because the document describes the element; our own answers to a 204
// and to a not-found; and an endpoint the document gives no schema for in a
// package where some other endpoint has one.
//
// [ShapeCheck.Typed] is the type grain, which cuts those away by asking only
// about the endpoints each type actually models. See [TypedShapeCheck].
type ShapeCheck struct {
	// Ran is false when the record could not be read, which is the only way
	// this check is skipped. Every other outcome is a finding or a pass.
	Ran bool `json:"ran"`
	// Record names the artifact that answered, so a reader of a finding knows
	// which GitLab it speaks for.
	Record string `json:"record,omitempty"`
	// Join says how many recorded endpoints could be looked up at all. It is
	// written even when every count is zero: a reader of a finding needs to
	// know how much of the inventory the comparison could see, and an absent
	// join reads as one that was not attempted.
	Join JoinQuality `json:"join"`
	// Untemplated are the literal path segments the inventory carries where
	// GitLab's document has a placeholder, most frequent first. Each is a
	// fixture value our own templating did not recognize as an identifier, so
	// this is a measure of the inventory's quality rather than a defect in the
	// server.
	Untemplated []UntemplatedSegment `json:"untemplated,omitempty"`
	// Unpublished are the output fields a package publishes that no endpoint it
	// was recorded calling declares in its response.
	Unpublished []UnpublishedField `json:"unpublished,omitempty"`
	// Typed is the same question asked at type grain, beside this one rather
	// than in place of it.
	Typed TypedShapeCheck `json:"typed"`
	// Sent is the reverse question: the fields GitLab's document says the
	// endpoints return that the package does not publish, each with what the
	// conditions record says about when GitLab sends it.
	Sent SentCheck `json:"sent"`
}

// JoinQuality is how much of the inventory could be compared at all.
type JoinQuality struct {
	// RESTRows is how many REST rows the inventory holds.
	RESTRows int `json:"rest_rows"`
	// Exact matched an operation with the placeholders spelled the same way.
	Exact int `json:"exact"`
	// Loose matched only after a literal segment of ours was accepted where
	// GitLab's document has a placeholder.
	Loose int `json:"loose"`
	// Unmatched found no operation at all. GitLab's document covers 1847
	// operations and not every endpoint this server calls, so an unmatched row
	// is not by itself a finding.
	Unmatched int `json:"unmatched"`
}

// UntemplatedSegment is one literal path segment and how often it stood where
// an identifier belongs.
type UntemplatedSegment struct {
	Segment string `json:"segment"`
	Count   int    `json:"count"`
	Example string `json:"example"`
}

// UnpublishedField is one output field this server publishes that GitLab does
// not say it sends.
type UnpublishedField struct {
	// Grain is which join found it: "package" searched every endpoint the
	// owning package was recorded calling, "type" only the endpoints the type
	// itself models.
	Grain string `json:"grain"`
	// Package owns the type.
	Package string `json:"package"`
	// Type is the Go type publishing the field.
	Type string `json:"type"`
	// Field is the json tag.
	Field string `json:"field"`
	// Under is the response property this type sits under, set on a nested
	// finding only: at the top level a type is the response and sits under
	// nothing.
	Under string `json:"under,omitempty"`
	// SDKType is the client-go struct a converter fills the type from, which is
	// what named the operations searched. Type grain only.
	SDKType string `json:"sdk_type,omitempty"`
	// Endpoints is how many operations' responses were searched, counted the
	// same way at both grains: an operation the document leaves without a
	// response was not searched and is not among them. A field absent from
	// every one of them is a field a model is told to expect and will not
	// receive.
	Endpoints int `json:"endpoints_searched"`
	// Operations names those operations, in the collapsed spelling both sides
	// of the join meet in rather than as GitLab spells them. Type grain only:
	// at package grain there are up to thirty of them and the count is what a
	// reader wants.
	Operations []string `json:"operations,omitempty"`
	// Category and Reason are the declaration accounting for GitLab's document
	// not listing a field GitLab does send, and are empty for a finding nothing
	// accounts for. Type grain only: the package grain unions thirty responses
	// and cannot say which of them a field belongs to, so a declaration written
	// against it would excuse more than it read. See [shapeDeclaration].
	Category string `json:"category,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// declared reports whether a declaration accounts for this finding.
func (f UnpublishedField) declared() bool { return f.Category != "" }

// shapeCheck compares the recorded inventory with GitLab's OpenAPI record.
//
// The record is read from the repository rather than fetched, so this needs no
// network and runs wherever the rest of the scope runs.
func shapeCheck(root string, requests []requestinventory.Row, published []publishedType) ShapeCheck {
	record, err := apishapes.Read(recordDir(root))
	if err != nil {
		return ShapeCheck{Ran: false}
	}

	index := newOperationIndex(record)
	check := ShapeCheck{Ran: true, Record: apishapes.FileName}

	segments := map[string]*UntemplatedSegment{}
	byPackage := map[string]map[string]bool{}
	endpointsPerPackage := map[string]int{}
	sources := responseSources{}

	for _, request := range requests {
		if !strings.EqualFold(request.Kind, "rest") {
			continue
		}
		check.Join.RESTRows++

		operation, quality, literal := index.lookup(request.Method, request.Path)
		switch quality {
		case matchExact:
			check.Join.Exact++
		case matchLoose:
			check.Join.Loose++
			segment := segments[literal]
			if segment == nil {
				segment = &UntemplatedSegment{Segment: literal, Example: request.Path}
				segments[literal] = segment
			}
			segment.Count++
		default:
			check.Join.Unmatched++
			continue
		}

		// An operation the document gives no response is nothing to search: it
		// names no field, so counting it would inflate the number a finding
		// reports as the responses it was held against.
		if len(operation.Response) == 0 {
			continue
		}

		endpointsPerPackage[request.Package]++
		fields := byPackage[request.Package]
		if fields == nil {
			fields = map[string]bool{}
			byPackage[request.Package] = fields
		}
		for _, name := range operation.Response {
			fields[name] = true
		}
		sources.note(request.Package, request.Method+" "+request.Path, operation.Entity, operation.Response)
	}

	check.Untemplated = sortedSegments(segments)
	check.Unpublished = unpublishedFields(published, byPackage, endpointsPerPackage)
	check.Typed = typedShapeCheck(root, index, published)
	check.Sent = sentCheck(root, sources, published)
	check.Sent.Unsurfaced, check.Typed.Unsurfaced, check.Sent.UnusedDeclarations = classifySentFindings(declaredUnsurfaced, check.Sent.Unsurfaced, check.Typed.Unsurfaced)
	return check
}

// sortedSegments orders the untemplated segments by how often each stood in for
// an identifier, so the one worth teaching the recorder about is first.
func sortedSegments(segments map[string]*UntemplatedSegment) []UntemplatedSegment {
	out := make([]UntemplatedSegment, 0, len(segments))
	for _, segment := range segments {
		out = append(out, *segment)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Segment < out[j].Segment
	})
	return out
}

// unpublishedFields reports every published field no endpoint of its package
// declares.
//
// The union over a package's endpoints is deliberately generous: a package
// calling twenty endpoints has twenty responses' worth of names, so a field
// only one of them sends still passes. That makes the check a lower bound,
// exact for a package with one endpoint and weaker as the package grows, and it
// is the honest shape available while the inventory records a package rather
// than an action. It cannot report a field GitLab does send, which is the
// property worth keeping.
func unpublishedFields(published []publishedType, byPackage map[string]map[string]bool, endpoints map[string]int) []UnpublishedField {
	var out []UnpublishedField
	for _, publishedType := range published {
		known := byPackage[publishedType.Package]
		if publishedType.Inner || len(known) == 0 {
			continue
		}
		for _, field := range publishedType.Fields {
			if known[field] {
				continue
			}
			out = append(out, UnpublishedField{
				Grain:     grainPackage,
				Package:   publishedType.Package,
				Type:      publishedType.Name,
				Field:     field,
				Endpoints: endpoints[publishedType.Package],
			})
		}
	}
	sortFindings(out)
	return out
}

// sortFindings orders the findings of either grain the way a reader reads them:
// down the tree, then by type, then by field.
func sortFindings(found []UnpublishedField) {
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

// matchQuality says how an endpoint was looked up.
type matchQuality int

const (
	matchNone matchQuality = iota
	matchExact
	matchLoose
)

// operationIndex looks an endpoint up by method and path.
type operationIndex struct {
	// byPath is keyed on the path with placeholder names kept, which is the
	// exact match.
	byPath map[string]apishapes.Operation
	// byShape is keyed on the path with every placeholder collapsed to one
	// token, which is what lets our `:project_id` meet GitLab's `{id}`. Two
	// operations can share a shape, so the value is the union of their
	// responses: taking one arbitrarily would report the other's fields as
	// unpublished.
	byShape map[string]apishapes.Operation
}

// placeholder is the token every identifier segment collapses to when a path is
// reduced to its shape.
const placeholder = ":"

func newOperationIndex(record apishapes.Document) *operationIndex {
	index := &operationIndex{
		byPath:  make(map[string]apishapes.Operation, len(record.Operations)),
		byShape: make(map[string]apishapes.Operation, len(record.Operations)),
	}
	for key, operation := range record.Operations {
		method, path, found := strings.Cut(key, " ")
		if !found {
			continue
		}
		normalized := apishapes.NormalizePath(path)
		index.byPath[method+" "+normalized] = operation
		shapeKey := method + " " + pathShape(normalized)
		merged := index.byShape[shapeKey]
		merged.Response = union(merged.Response, operation.Response)
		merged.Params = union(merged.Params, operation.Params)
		merged.Body = union(merged.Body, operation.Body)
		merged.Nested = unionNested(merged.Nested, operation.Nested)
		// The component is the first one an operation of this shape named.
		// Two operations sharing a shape nearly always render one entity,
		// and a merged name would join the conditions record on nothing.
		if merged.Entity == "" {
			merged.Entity = operation.Entity
		}
		index.byShape[shapeKey] = merged
	}
	return index
}

// lookup finds the operation a recorded request names, and says how. When the
// match needed a literal segment of ours to stand where GitLab has a
// placeholder, that segment is returned: it is a fixture value the recorder
// failed to recognize as an identifier.
func (index *operationIndex) lookup(method, path string) (apishapes.Operation, matchQuality, string) {
	if operation, ok := index.byPath[method+" "+path]; ok {
		return operation, matchExact, ""
	}
	if operation, ok := index.byShape[method+" "+pathShape(path)]; ok {
		return operation, matchExact, ""
	}

	// One literal segment at a time, because two would stop being evidence
	// about a specific segment and start being a search for any operation of
	// the right length.
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if segment == "" || strings.HasPrefix(segment, ":") {
			continue
		}
		trial := make([]string, len(segments))
		copy(trial, segments)
		trial[i] = placeholder
		if operation, ok := index.byShape[method+" "+pathShape(strings.Join(trial, "/"))]; ok {
			return operation, matchLoose, segment
		}
	}
	return apishapes.Operation{}, matchNone, ""
}

// pathShape reduces a path to its segments with every placeholder collapsed, so
// two spellings of the same endpoint meet.
func pathShape(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") {
			segments[i] = placeholder
		}
	}
	return strings.Join(segments, "/")
}

// unionNested merges two operations' nested property maps, property by
// property, for the same reason the name lists are unioned: two operations can
// share a shape, and taking one arbitrarily would report the other's nested
// fields as unpublished.
func unionNested(a, b map[string][]string) map[string][]string {
	if len(a) == 0 {
		return b
	}
	out := make(map[string][]string, len(a)+len(b))
	maps.Copy(out, a)
	for property, names := range b {
		out[property] = union(out[property], names)
	}
	return out
}

// union merges two sorted name lists without duplicates.
func union(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, name := range list {
			if !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
