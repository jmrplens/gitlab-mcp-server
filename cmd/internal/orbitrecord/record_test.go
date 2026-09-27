package orbitrecord

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// recordDay is the retrieval day every fixture carries, and judgedOn the day
// the checks are asked on, well inside the window.
const recordDay = "2026-09-27"

var judgedOn = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// wholeRecord is a record that passes every check: each expected call once,
// answered 200 with a key tree that holds together.
func wholeRecord() Document {
	doc := Document{
		SchemaVersion: SchemaVersion,
		Note:          "fixture",
		Source:        Source{Instance: Instance, OrbitVersion: "0.130.0", Namespace: "plens1", RetrievedAt: recordDay},
	}
	for _, id := range ExpectedCalls() {
		doc.Calls = append(doc.Calls, Call{
			Action:  id.Action,
			Variant: id.Variant,
			Output:  "internal/tools/orbit.StatusOutput",
			Request: Request{Method: "GET", Path: "/orbit/status", Query: []string{"response_format"}},
			Response: Response{Status: 200, ContentType: "application/json", Keys: []Key{
				{Path: Root, Kinds: []string{KindObject}},
				{Path: "system", Kinds: []string{KindObject}},
				{Path: "system.components", Kinds: []string{KindArray}},
				{Path: "system.components[]", Kinds: []string{KindObject}},
				{Path: "system.components[].name", Kinds: []string{KindString}},
			}},
		})
	}
	return doc
}

// TestExpectedCalls_AreTheTwelveInTheOrderTheGeneratorMakesThem pins the call
// set a whole record holds. The status call is first because the record is
// stamped with the version it reports, and every other entry is one answer an
// output type is judged by.
func TestExpectedCalls_AreTheTwelveInTheOrderTheGeneratorMakesThem(t *testing.T) {
	var got []string
	for _, id := range ExpectedCalls() {
		got = append(got, id.String())
	}
	want := []string{
		"orbit.status (raw)", "orbit.status (llm)",
		"orbit.schema (raw)", "orbit.schema (llm)", "orbit.schema (expand)",
		"orbit.tools (default)",
		"orbit.dsl (raw)", "orbit.dsl (llm)",
		"orbit.query (raw)", "orbit.query (llm)",
		"orbit.graph_status (raw)", "orbit.graph_status (llm)",
	}
	if !slices.Equal(got, want) {
		t.Errorf("ExpectedCalls() = %q, want %q", got, want)
	}
}

// TestCall_ID_IsItsActionAndVariant verifies a call's identity is exactly the
// two fields that tell recorded calls apart.
func TestCall_ID_IsItsActionAndVariant(t *testing.T) {
	call := Call{Action: "orbit.schema", Variant: "expand", Output: "ignored"}
	if got := call.ID(); got != (CallID{Action: "orbit.schema", Variant: "expand"}) {
		t.Errorf("ID() = %+v", got)
	}
}

// TestPath_IsTheFileUnderTheDirectory verifies where the record is read from.
func TestPath_IsTheFileUnderTheDirectory(t *testing.T) {
	if got := Path("docs/development"); got != filepath.Join("docs/development", "orbit-responses.json") {
		t.Errorf("Path() = %q", got)
	}
}

// TestRead_ACanonicalRecord_RoundTrips verifies that what Encode writes is
// what Read returns, which is the whole contract between the generator and
// every reader.
func TestRead_ACanonicalRecord_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	doc := Canonical(wholeRecord())
	if err := os.WriteFile(Path(dir), Encode(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Read(dir)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Errorf("Read() = %+v, want %+v", got, doc)
	}
}

// TestDecode_TheBytesOfARecord_RoundTrip verifies the reader of bytes that did
// not come from the file on disk, which is how the generator reads the record
// a commit holds: what Encode writes, Decode returns.
func TestDecode_TheBytesOfARecord_RoundTrip(t *testing.T) {
	doc := Canonical(wholeRecord())
	got, err := Decode(Encode(doc))
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if !reflect.DeepEqual(got, doc) {
		t.Errorf("Decode() = %+v, want %+v", got, doc)
	}
	if _, err = Decode([]byte(`{"schema_version": 0}`)); err == nil {
		t.Error("Decode() of another schema version = nil error, want it refused")
	}
}

// TestRead_WhatIsNotARecordOfThisBuild_IsRefused verifies the three ways a
// file fails to be read: it is not there, it is not JSON, and it is a schema
// version this build was not written for, which is refused rather than decoded
// into fields that mean something else.
func TestRead_WhatIsNotARecordOfThisBuild_IsRefused(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{name: "missing", want: "reading the Orbit response record"},
		{name: "not JSON", content: "{", want: "decoding the Orbit response record"},
		{name: "another schema version", content: `{"schema_version": 2}`, want: "schema version 2 and this build reads version 1: regenerate it with make gen-orbit-record"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.content != "" {
				if err := os.WriteFile(Path(dir), []byte(tc.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := Read(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Read() error = %v, want it to say %q", err, tc.want)
			}
		})
	}
}

// TestEncode_WritesTheCanonicalFormAndLeavesTheInputAlone verifies every list
// is sorted, the root key first, an empty name list omitted, the output ends
// in a newline, and the caller's document is not reordered in place.
func TestEncode_WritesTheCanonicalFormAndLeavesTheInputAlone(t *testing.T) {
	doc := Document{SchemaVersion: SchemaVersion, Calls: []Call{
		{
			Action: "orbit.tools", Variant: "default", Request: Request{Query: []string{"b", "a"}, Body: []string{"z", "y"}},
			Response: Response{Keys: []Key{{Path: "b", Kinds: []string{KindString, KindNull}}, {Path: Root, Kinds: []string{KindObject}}, {Path: "a", Kinds: []string{KindNumber}}}},
		},
		{Action: "orbit.status", Variant: "raw"},
		{Action: "orbit.status", Variant: "llm", Request: Request{Query: []string{}}},
	}}
	before := Encode(doc)

	canonical := Canonical(doc)
	var order []string
	for _, call := range canonical.Calls {
		order = append(order, call.ID().String())
	}
	if want := []string{"orbit.status (llm)", "orbit.status (raw)", "orbit.tools (default)"}; !slices.Equal(order, want) {
		t.Errorf("call order = %q, want %q", order, want)
	}
	tools := canonical.Calls[2]
	if !slices.Equal(tools.Request.Query, []string{"a", "b"}) || !slices.Equal(tools.Request.Body, []string{"y", "z"}) {
		t.Errorf("names = %q, %q, want sorted", tools.Request.Query, tools.Request.Body)
	}
	var paths []string
	for _, key := range tools.Response.Keys {
		paths = append(paths, key.Path)
	}
	if !slices.Equal(paths, []string{Root, "a", "b"}) {
		t.Errorf("key order = %q, want the root first", paths)
	}
	if !slices.Equal(tools.Response.Keys[2].Kinds, []string{KindNull, KindString}) {
		t.Errorf("kinds = %q, want sorted", tools.Response.Keys[2].Kinds)
	}
	if canonical.Calls[0].Request.Query != nil {
		t.Errorf("an empty name list = %#v, want nil so it is omitted", canonical.Calls[0].Request.Query)
	}
	if doc.Calls[0].Action != "orbit.tools" || doc.Calls[0].Request.Query[0] != "b" || doc.Calls[0].Response.Keys[0].Path != "b" || doc.Calls[0].Response.Keys[0].Kinds[0] != KindString {
		t.Errorf("the input was reordered: %+v", doc.Calls[0])
	}
	if !strings.HasSuffix(string(before), "}\n") || strings.Contains(string(before), `"query": []`) {
		t.Errorf("Encode() = %s, want a trailing newline and no empty name list", before)
	}
	if string(before) != string(Encode(canonical)) {
		t.Error("Encode() of a document and of its canonical form differ")
	}
}

// TestValidKey_IsAnIdentifier verifies which object keys may become a path
// segment: an identifier, and nothing that cannot be a name. It pins the
// check's limit too: data that happens to be shaped like an identifier (a
// namespace, a metric name) passes, which is why a data-keyed subtree needs a
// verbatim path rather than this check.
func TestValidKey_IsAnIdentifier(t *testing.T) {
	cases := map[string]bool{
		"name": true, "_private": true, "Node2": true, "last_duration_ms": true,
		"plens1": true, "query_latency_p99_ms": true,
		"": false, "$defs": false, "2fa": false, "plens1/kg-fixtures": false, "a b": false, "a[]": false,
	}
	for key, want := range cases {
		t.Run(key, func(t *testing.T) {
			if got := ValidKey(key); got != want {
				t.Errorf("ValidKey(%q) = %t, want %t", key, got, want)
			}
		})
	}
}

// TestValidSegment_IsAKeyOrElementMarkers verifies the segments a recorded
// path may hold.
func TestValidSegment_IsAKeyOrElementMarkers(t *testing.T) {
	cases := map[string]bool{
		"name": true, "items[]": true, "matrix[][]": true, "[]": true, "[][]": true,
		"": false, "[": false, "items]": false, "[]name": false, "$ref": false, "a-b": false,
	}
	for segment, want := range cases {
		t.Run(segment, func(t *testing.T) {
			if got := ValidSegment(segment); got != want {
				t.Errorf("ValidSegment(%q) = %t, want %t", segment, got, want)
			}
		})
	}
}

// TestPathHelpers_WalkUpDownAndFlatten verifies the four path functions the
// generator builds paths with and the audit reads them by. A path that begins
// with a dot has an empty parent and never the root, so the check refuses it
// for a parent nothing records rather than taking it for a top-level key.
func TestPathHelpers_WalkUpDownAndFlatten(t *testing.T) {
	parents := map[string]string{
		"name": Root, "[]": Root, "[].name": "[]", "domains[]": "domains",
		"domains[].items[].count": "domains[].items[]", "a[][]": "a[]", "system.version": "system",
		".name": "",
	}
	for path, want := range parents {
		t.Run("parent of "+path, func(t *testing.T) {
			if got := Parent(path); got != want {
				t.Errorf("Parent(%q) = %q, want %q", path, got, want)
			}
		})
	}
	if got := Child(Root, "system"); got != "system" {
		t.Errorf("Child(root) = %q", got)
	}
	if got := Child("system", "version"); got != "system.version" {
		t.Errorf("Child(system) = %q", got)
	}
	if got := ElementOf(Root); got != "[]" {
		t.Errorf("ElementOf(root) = %q", got)
	}
	if got := ElementOf("domains"); got != "domains[]" {
		t.Errorf("ElementOf(domains) = %q", got)
	}
	elided := map[string]string{
		Root: "", "[]": "", "[].name": "name", "system.components[].name": "system.components.name", "a[][]": "a", "name": "name",
	}
	for path, want := range elided {
		t.Run("elided "+path, func(t *testing.T) {
			if got := Elided(path); got != want {
				t.Errorf("Elided(%q) = %q, want %q", path, got, want)
			}
		})
	}
}

// TestProblems_AWholeRecord_HasNone verifies the fixture every refusal below
// is a single edit of passes on its own, so each case proves its one rule.
func TestProblems_AWholeRecord_HasNone(t *testing.T) {
	if problems := Problems(wholeRecord(), judgedOn); len(problems) != 0 {
		t.Errorf("Problems() = %q, want none", problems)
	}
}

// TestProblems_EachDefect_IsNamed verifies every refusal the check makes, one
// edit of the whole record each, and that the message says what is wrong.
func TestProblems_EachDefect_IsNamed(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Document)
		want string
	}{
		{name: "another instance", edit: func(d *Document) { d.Source.Instance = "https://gitlab.example.com" }, want: `taken from "https://gitlab.example.com", not https://gitlab.com`},
		{name: "no version", edit: func(d *Document) { d.Source.OrbitVersion = "" }, want: "names no Orbit version"},
		{name: "no namespace", edit: func(d *Document) { d.Source.Namespace = "" }, want: "names no fixture namespace"},
		{name: "no date", edit: func(d *Document) { d.Source.RetrievedAt = "yesterday" }, want: `taken on "yesterday", which is not a date`},
		{name: "a future date", edit: func(d *Document) { d.Source.RetrievedAt = "2026-12-01" }, want: "has not happened yet"},
		{name: "too old", edit: func(d *Document) { d.Source.RetrievedAt = "2025-01-01" }, want: "the record is 638 days old and the window is 180: the Knowledge Graph API is in beta"},
		{name: "a call missing", edit: func(d *Document) { d.Calls = d.Calls[1:] }, want: "orbit.status (raw) is not recorded"},
		{name: "a call twice", edit: func(d *Document) { d.Calls = append(d.Calls, d.Calls[0]) }, want: "orbit.status (raw) is recorded 2 times"},
		{name: "a call nothing makes", edit: func(d *Document) { d.Calls[0].Variant = "xml" }, want: "orbit.status (xml) is recorded and no generator run makes it"},
		{name: "an output with no package", edit: func(d *Document) { d.Calls[0].Output = "StatusOutput" }, want: `names no output type the audit can find ("StatusOutput")`},
		{name: "another route", edit: func(d *Document) { d.Calls[0].Request.Path = "/projects" }, want: `was sent to "/projects", which is not an Orbit route`},
		{name: "a refusal", edit: func(d *Document) { d.Calls[0].Response.Status = 406 }, want: "was answered 406"},
		{name: "no body", edit: func(d *Document) { d.Calls[0].Response.Keys = d.Calls[0].Response.Keys[1:2] }, want: "records no body ($)"},
		{name: "a value as a key", edit: func(d *Document) { d.Calls[0].Response.Keys[1].Path = "plens1/kg" }, want: `whose segment "plens1/kg" is not a key name`},
		{name: "an orphan", edit: func(d *Document) { d.Calls[0].Response.Keys = slices.Delete(d.Calls[0].Response.Keys, 2, 3) }, want: `records "system.components[]" and not its parent "system.components"`},
		{name: "below verbatim", edit: func(d *Document) { d.Calls[0].Response.Keys[1].Verbatim = true }, want: `records "system.components" below "system", which is verbatim`},
		{name: "no kind", edit: func(d *Document) { d.Calls[0].Response.Keys[4].Kinds = nil }, want: `records "system.components[].name" with no kind`},
		{name: "not a kind", edit: func(d *Document) { d.Calls[0].Response.Keys[4].Kinds = []string{"integer"} }, want: `as "integer", which is not a kind`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := wholeRecord()
			tc.edit(&doc)
			problems := Problems(doc, judgedOn)
			if !slices.ContainsFunc(problems, func(p string) bool { return strings.Contains(p, tc.want) }) {
				t.Errorf("Problems() = %q, want one saying %q", problems, tc.want)
			}
		})
	}
}

// TestProblems_ABadSegment_IsNamedOncePerPath verifies a path with several
// bad segments is reported once, at its first, rather than once per segment.
func TestProblems_ABadSegment_IsNamedOncePerPath(t *testing.T) {
	doc := wholeRecord()
	doc.Calls[0].Response.Keys = append(doc.Calls[0].Response.Keys, Key{Path: "system.$a.$b", Kinds: []string{KindString}})
	count := 0
	for _, problem := range Problems(doc, judgedOn) {
		if strings.Contains(problem, "is not a key name") {
			count++
			if !strings.Contains(problem, `segment "$a"`) {
				t.Errorf("problem = %q, want the first bad segment", problem)
			}
		}
	}
	if count != 1 {
		t.Errorf("bad-segment problems = %d, want 1", count)
	}
}

// TestDiff_NamesEveryChangeToTheKeyTree verifies the lines the generator prints
// when a recording differs from the committed record: a call added or
// dropped, a key added or dropped, kinds changed and a verbatim flag flipped,
// and nothing for provenance or kinds merely listed in another order.
func TestDiff_NamesEveryChangeToTheKeyTree(t *testing.T) {
	before := Document{Source: Source{OrbitVersion: "0.129.0"}, Calls: []Call{
		{Action: "orbit.status", Variant: "raw", Response: Response{Keys: []Key{
			{Path: Root, Kinds: []string{KindObject}},
			{Path: "gone", Kinds: []string{KindString}},
			{Path: "kind", Kinds: []string{KindString}},
			{Path: "flag", Kinds: []string{KindObject}},
			{Path: "same", Kinds: []string{KindString, KindNull}},
		}}},
		{Action: "orbit.dsl", Variant: "llm"},
	}}
	after := Document{Source: Source{OrbitVersion: "0.130.0"}, Calls: []Call{
		{Action: "orbit.status", Variant: "raw", Response: Response{Keys: []Key{
			{Path: Root, Kinds: []string{KindObject}},
			{Path: "new", Kinds: []string{KindNumber, KindNull}},
			{Path: "kind", Kinds: []string{KindNumber}},
			{Path: "flag", Kinds: []string{KindObject}, Verbatim: true},
			{Path: "same", Kinds: []string{KindNull, KindString}},
		}}},
		{Action: "orbit.tools", Variant: "default"},
	}}
	want := []string{
		"+ orbit.status (raw): new number|null",
		"+ orbit.tools (default): a call the previous record did not hold",
		"- orbit.dsl (llm): a call this recording did not make",
		"- orbit.status (raw): gone string",
		"~ orbit.status (raw): flag verbatim false -> true",
		"~ orbit.status (raw): kind string -> number",
	}
	if got := Diff(before, after); !slices.Equal(got, want) {
		t.Errorf("Diff() =\n%q\nwant\n%q", got, want)
	}
	if got := Diff(before, before); got != nil {
		t.Errorf("Diff() of a record with itself = %q, want nil", got)
	}
}
