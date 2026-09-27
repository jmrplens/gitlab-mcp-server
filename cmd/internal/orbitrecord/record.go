package orbitrecord

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// SchemaVersion is the shape of this record. A reader refuses a version it
// was not written for rather than guessing at a field that moved.
const SchemaVersion = 1

// DefaultDir is where the record lives, beside the other pinned records.
const DefaultDir = "docs/development"

// FileName is the record's name on disk.
const FileName = "orbit-responses.json"

// Instance is the one GitLab that serves Orbit, and so the one a record may
// be taken from.
const Instance = "https://gitlab.com"

// Root is the path of the response body itself.
const Root = "$"

// Element is the segment suffix for the elements of an array: the elements
// of the array at "domains" are at "domains[]", and a bare array body's at
// "[]".
const Element = "[]"

// The kinds a key can be recorded with: the six JSON kinds, and Text for a
// body that is not JSON at all, which is what the llm answer to a query is.
const (
	KindObject  = "object"
	KindArray   = "array"
	KindString  = "string"
	KindNumber  = "number"
	KindBoolean = "boolean"
	KindNull    = "null"
	KindText    = "text"
)

// Document is the committed record.
type Document struct {
	SchemaVersion int    `json:"schema_version"`
	Note          string `json:"note"`
	Source        Source `json:"source"`
	// Calls are the recorded calls, sorted by action and variant.
	Calls []Call `json:"calls"`
}

// Source is what the record was taken from and when.
type Source struct {
	// Instance is the GitLab the calls were made against.
	Instance string `json:"instance"`
	// OrbitVersion is the Knowledge Graph service version GitLab.com
	// reported in the status call, which is what moves between two
	// recordings far more often than GitLab's own release does.
	OrbitVersion string `json:"orbit_version"`
	// Namespace is the fixture namespace the indexing status and the query
	// were asked about (test/fixtures/orbit, docs/development/orbit-fixtures.md).
	Namespace string `json:"namespace"`
	// RetrievedAt is the day of the recording, YYYY-MM-DD.
	RetrievedAt string `json:"retrieved_at"`
}

// Call is one handler invocation and what GitLab answered it with.
type Call struct {
	// Action is the canonical action ID whose handler made the call.
	Action string `json:"action"`
	// Variant tells two calls of one action apart: the response format it
	// asked for, or what else it asked (an expanded node).
	Variant string `json:"variant"`
	// Output is the Go type the handler returned, repository relative
	// (internal/tools/orbit.StatusOutput). It is what the audit judges
	// against the keys below.
	Output   string   `json:"output"`
	Request  Request  `json:"request"`
	Response Response `json:"response"`
}

// Request is what the handler sent, by name only.
type Request struct {
	Method string `json:"method"`
	// Path is the route below /api/v4.
	Path string `json:"path"`
	// Query are the query parameter names the handler sent, sorted.
	Query []string `json:"query,omitempty"`
	// Body are the top-level names of the JSON body the handler sent, sorted.
	Body []string `json:"body,omitempty"`
}

// Response is what GitLab answered, reduced to its key tree.
type Response struct {
	Status int `json:"status"`
	// ContentType is the media type without its parameters.
	ContentType string `json:"content_type"`
	// Keys is every path the body carried, sorted, the body itself first.
	Keys []Key `json:"keys"`
}

// Key is one path of a response body and the kinds seen there.
type Key struct {
	Path  string   `json:"path"`
	Kinds []string `json:"kinds"`
	// Verbatim marks a subtree recorded as its root only, because its keys
	// are data rather than a response shape. Nothing below it is recorded.
	Verbatim bool `json:"verbatim,omitempty"`
}

// CallID names one recorded call.
type CallID struct {
	Action  string
	Variant string
}

// String renders the ID the way the report and the diff spell a call.
func (id CallID) String() string { return id.Action + " (" + id.Variant + ")" }

// ID is the call's identity in the record.
func (c Call) ID() CallID { return CallID{Action: c.Action, Variant: c.Variant} }

// ExpectedCalls is every call a whole record holds, in the order the
// generator makes them. The status call comes first because it carries the
// version the record is stamped with.
func ExpectedCalls() []CallID {
	return []CallID{
		{Action: "orbit.status", Variant: "raw"},
		{Action: "orbit.status", Variant: "llm"},
		{Action: "orbit.schema", Variant: "raw"},
		{Action: "orbit.schema", Variant: "llm"},
		{Action: "orbit.schema", Variant: "expand"},
		{Action: "orbit.tools", Variant: "raw"},
		{Action: "orbit.dsl", Variant: "raw"},
		{Action: "orbit.dsl", Variant: "llm"},
		{Action: "orbit.query", Variant: "raw"},
		{Action: "orbit.query", Variant: "llm"},
		{Action: "orbit.graph_status", Variant: "raw"},
		{Action: "orbit.graph_status", Variant: "llm"},
	}
}

// Path is where the record lives under a directory.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Read loads the committed record.
//
// A schema version this build was not written for is refused rather than
// decoded, for the reason [SchemaVersion] gives.
func Read(dir string) (Document, error) {
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		return Document{}, fmt.Errorf("reading the Orbit response record: %w", err)
	}
	var doc Document
	if decodeErr := json.Unmarshal(raw, &doc); decodeErr != nil {
		return Document{}, fmt.Errorf("decoding the Orbit response record: %w", decodeErr)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf(
			"the Orbit response record is schema version %d and this build reads version %d: regenerate it with make gen-orbit-record",
			doc.SchemaVersion, SchemaVersion,
		)
	}
	return doc, nil
}

// Encode renders a record in its canonical form: calls sorted by action and
// variant, keys by path, kinds and parameter names alphabetically, indented
// so a re-recording is a readable diff. A committed record that is not in
// this form was edited by hand or written by another build, which is what
// the generator's -check holds it to.
func Encode(doc Document) []byte {
	canonical := Canonical(doc)
	// Marshaling a struct of strings, numbers, booleans and slices of them
	// cannot fail, and this repository's rule for that is to say so at the
	// leaf rather than carry a branch no test can reach.
	return append(cmdutil.Must(json.MarshalIndent(canonical, "", "  ")), '\n')
}

// Canonical returns a copy of doc with every list in its sorted order. The
// input is left as it was.
func Canonical(doc Document) Document {
	out := doc
	out.Calls = make([]Call, len(doc.Calls))
	for i, call := range doc.Calls {
		call.Request.Query = sortedCopy(call.Request.Query)
		call.Request.Body = sortedCopy(call.Request.Body)
		keys := make([]Key, len(call.Response.Keys))
		for j, key := range call.Response.Keys {
			key.Kinds = sortedCopy(key.Kinds)
			keys[j] = key
		}
		SortKeys(keys)
		call.Response.Keys = keys
		out.Calls[i] = call
	}
	slices.SortStableFunc(out.Calls, func(a, b Call) int {
		return cmp.Or(strings.Compare(a.Action, b.Action), strings.Compare(a.Variant, b.Variant))
	})
	return out
}

// SortKeys orders keys by path, the root before every other path, which is
// where a reader looks for what the body was.
func SortKeys(keys []Key) {
	slices.SortStableFunc(keys, func(a, b Key) int { return strings.Compare(keyOrder(a.Path), keyOrder(b.Path)) })
}

// keyOrder is the sort key of a path: the root sorts first.
func keyOrder(path string) string {
	if path == Root {
		return ""
	}
	return path
}

// sortedCopy returns names sorted, as a new slice, or nil for none.
func sortedCopy(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(names))
}

// keyPattern is what a key name may be: an identifier, which is how GitLab's
// code names a key. It is the check that keeps a value out of the record's
// paths: a map keyed by data would spell its keys here.
var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidKey reports whether an object key of an answer is a key name.
func ValidKey(key string) bool {
	return keyPattern.MatchString(key)
}

// segmentPattern is what one path segment may be: a key name, a key name
// followed by element markers, or element markers alone, which is how the
// elements of a bare array body begin a path.
var segmentPattern = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z0-9_]*(?:\[\])*|(?:\[\])+)$`)

// ValidSegment reports whether one segment of a path is one of the shapes
// segmentPattern allows.
func ValidSegment(segment string) bool {
	return segmentPattern.MatchString(segment)
}

// Parent is the path one level up, the root for a top-level key.
func Parent(path string) string {
	if trimmed, isElement := strings.CutSuffix(path, Element); isElement {
		if trimmed == "" {
			return Root
		}
		return trimmed
	}
	if cut := strings.LastIndex(path, "."); cut >= 0 {
		return path[:cut]
	}
	return Root
}

// Child is the path of a key under parent.
func Child(parent, key string) string {
	if parent == Root {
		return key
	}
	return parent + "." + key
}

// ElementOf is the path of the elements of the array at parent.
func ElementOf(parent string) string {
	if parent == Root {
		return Element
	}
	return parent + Element
}

// Elided is a path with its element markers removed, which is how a Go
// field path reads: "system.components[].name" is "system.components.name",
// and the elements of a bare array body are the body.
func Elided(path string) string {
	if path == Root {
		return ""
	}
	elided := strings.ReplaceAll(path, Element, "")
	return strings.Trim(elided, ".")
}

// validKinds is every kind a key may carry.
var validKinds = map[string]bool{
	KindObject: true, KindArray: true, KindString: true, KindNumber: true,
	KindBoolean: true, KindNull: true, KindText: true,
}

// subject is how the provenance verdict names this record.
var subject = provenance.Subject{
	Noun:        "record",
	Consequence: "the Knowledge Graph API is in beta and moves weekly, so a record this old can no longer report a key GitLab.com added or dropped since",
}

// Problems reports every way a record fails to be a whole, current and
// value-free recording of the expected calls. It judges the document as
// decoded; whether the file is in canonical form is the caller's question,
// since only the caller holds the bytes.
func Problems(doc Document, now time.Time) []string {
	var problems []string
	if doc.Source.Instance != Instance {
		problem := fmt.Sprintf("the record was taken from %q, not %s: no other instance serves Orbit, so nothing else can say what it answers",
			doc.Source.Instance, Instance)
		problems = append(problems, problem)
	}
	if doc.Source.OrbitVersion == "" {
		problems = append(problems, "the record names no Orbit version: nothing can then say which Knowledge Graph service it speaks for")
	}
	if doc.Source.Namespace == "" {
		problems = append(problems, "the record names no fixture namespace: the indexing status and the query were asked about something nobody can find again")
	}
	problems = append(problems, provenance.Problems(subject, doc.Source.RetrievedAt, now)...)
	problems = append(problems, callSetProblems(doc.Calls)...)
	for _, call := range doc.Calls {
		problems = append(problems, callProblems(call)...)
	}
	return problems
}

// callSetProblems reports an expected call the record lacks, one it holds
// twice, and one nothing expects.
func callSetProblems(calls []Call) []string {
	count := map[CallID]int{}
	for _, call := range calls {
		count[call.ID()]++
	}
	var problems []string
	expected := map[CallID]bool{}
	for _, id := range ExpectedCalls() {
		expected[id] = true
		switch count[id] {
		case 0:
			problems = append(problems, fmt.Sprintf("%s is not recorded: the audit cannot judge what its handler returns", id))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s is recorded %d times: a reader could not say which answer the audit read", id, count[id]))
		}
	}
	for _, call := range calls {
		if !expected[call.ID()] {
			problems = append(problems, fmt.Sprintf("%s is recorded and no generator run makes it", call.ID()))
		}
	}
	return problems
}

// callProblems reports what makes one call unusable: an answer that was not
// a success, a request that is not an Orbit route, and a key tree that does
// not hold together.
func callProblems(call Call) []string {
	id := call.ID()
	var problems []string
	if !strings.Contains(call.Output, ".") {
		problems = append(problems, fmt.Sprintf("%s names no output type the audit can find (%q)", id, call.Output))
	}
	if !strings.HasPrefix(call.Request.Path, "/orbit/") {
		problems = append(problems, fmt.Sprintf("%s was sent to %q, which is not an Orbit route", id, call.Request.Path))
	}
	if call.Response.Status != 200 {
		problems = append(problems, fmt.Sprintf("%s was answered %d: a refusal records no shape, so the call would judge nothing", id, call.Response.Status))
	}
	return append(problems, keyProblems(id, call.Response.Keys)...)
}

// keyProblems reports a key tree without its root, a path that is not made of
// key names, a path whose parent is not recorded, a kind that is not one, and
// a key recorded below a verbatim one.
func keyProblems(id CallID, keys []Key) []string {
	recorded := make(map[string]Key, len(keys))
	for _, key := range keys {
		recorded[key.Path] = key
	}
	var problems []string
	if _, hasRoot := recorded[Root]; !hasRoot {
		problems = append(problems, fmt.Sprintf("%s records no body (%s): the rest of its tree hangs from nothing", id, Root))
	}
	for _, key := range keys {
		problems = append(problems, keyShapeProblems(id, key)...)
		if key.Path == Root {
			continue
		}
		parent, hasParent := recorded[Parent(key.Path)]
		switch {
		case !hasParent:
			problems = append(problems, fmt.Sprintf("%s records %q and not its parent %q", id, key.Path, Parent(key.Path)))
		case parent.Verbatim:
			problems = append(problems, fmt.Sprintf("%s records %q below %q, which is verbatim and records nothing under it", id, key.Path, parent.Path))
		}
	}
	return problems
}

// keyShapeProblems reports a path whose segments are not key names, and kinds
// that are missing or are not kinds.
func keyShapeProblems(id CallID, key Key) []string {
	var problems []string
	if key.Path != Root {
		for segment := range strings.SplitSeq(key.Path, ".") {
			if !ValidSegment(segment) {
				problems = append(problems, fmt.Sprintf("%s records %q, whose segment %q is not a key name: a record holds names and never values", id, key.Path, segment))
				break
			}
		}
	}
	if len(key.Kinds) == 0 {
		problems = append(problems, fmt.Sprintf("%s records %q with no kind", id, key.Path))
	}
	for _, kind := range key.Kinds {
		if !validKinds[kind] {
			problems = append(problems, fmt.Sprintf("%s records %q as %q, which is not a kind", id, key.Path, kind))
		}
	}
	return problems
}

// Diff lists what changed between two records' key trees, one line per
// change, sorted: a call added or dropped, a key added or dropped, a key's
// kinds changed, and a key that became or stopped being verbatim. Provenance
// and request names are not compared, since a new day or version is what
// every recording brings and is not a change to the shape.
func Diff(before, after Document) []string {
	beforeCalls := callsByID(before.Calls)
	afterCalls := callsByID(after.Calls)
	var lines []string
	for id, call := range afterCalls {
		previous, had := beforeCalls[id]
		if !had {
			lines = append(lines, fmt.Sprintf("+ %s: a call the previous record did not hold", id))
			continue
		}
		lines = append(lines, keyDiff(id, previous.Response.Keys, call.Response.Keys)...)
	}
	for id := range beforeCalls {
		if _, kept := afterCalls[id]; !kept {
			lines = append(lines, fmt.Sprintf("- %s: a call this recording did not make", id))
		}
	}
	sort.Strings(lines)
	return lines
}

// callsByID indexes calls by their identity.
func callsByID(calls []Call) map[CallID]Call {
	out := make(map[CallID]Call, len(calls))
	for _, call := range calls {
		out[call.ID()] = call
	}
	return out
}

// keyDiff lists the key changes of one call.
func keyDiff(id CallID, before, after []Key) []string {
	beforeKeys := make(map[string]Key, len(before))
	for _, key := range before {
		beforeKeys[key.Path] = key
	}
	afterKeys := make(map[string]Key, len(after))
	for _, key := range after {
		afterKeys[key.Path] = key
	}
	var lines []string
	for path, key := range afterKeys {
		previous, had := beforeKeys[path]
		if !had {
			lines = append(lines, fmt.Sprintf("+ %s: %s %s", id, path, strings.Join(key.Kinds, "|")))
			continue
		}
		if !slices.Equal(sortedCopy(previous.Kinds), sortedCopy(key.Kinds)) {
			lines = append(lines, fmt.Sprintf("~ %s: %s %s -> %s", id, path, strings.Join(sortedCopy(previous.Kinds), "|"), strings.Join(sortedCopy(key.Kinds), "|")))
			continue
		}
		if previous.Verbatim != key.Verbatim {
			lines = append(lines, fmt.Sprintf("~ %s: %s verbatim %t -> %t", id, path, previous.Verbatim, key.Verbatim))
		}
	}
	for path, key := range beforeKeys {
		if _, kept := afterKeys[path]; !kept {
			lines = append(lines, fmt.Sprintf("- %s: %s %s", id, path, strings.Join(key.Kinds, "|")))
		}
	}
	return lines
}
