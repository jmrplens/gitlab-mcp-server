package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord"
)

// TestKeyTree_RecordsEveryPathWithTheKindsSeenThere verifies the reduction of
// an answer to its shape: every key and element path, the kinds seen at each
// as the union over the elements of an array, the root first, and no value.
func TestKeyTree_RecordsEveryPathWithTheKindsSeenThere(t *testing.T) {
	body := `{"user":{"available":true},"items":[{"n":1,"x":null},{"n":"two"}],"tags":["a"],"empty":[]}`
	got, err := keyTree("application/json", []byte(body), nil)
	if err != nil {
		t.Fatalf("keyTree() error: %v", err)
	}
	want := []orbitrecord.Key{
		{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindObject}},
		{Path: "empty", Kinds: []string{orbitrecord.KindArray}},
		{Path: "items", Kinds: []string{orbitrecord.KindArray}},
		{Path: "items[]", Kinds: []string{orbitrecord.KindObject}},
		{Path: "items[].n", Kinds: []string{orbitrecord.KindNumber, orbitrecord.KindString}},
		{Path: "items[].x", Kinds: []string{orbitrecord.KindNull}},
		{Path: "tags", Kinds: []string{orbitrecord.KindArray}},
		{Path: "tags[]", Kinds: []string{orbitrecord.KindString}},
		{Path: "user", Kinds: []string{orbitrecord.KindObject}},
		{Path: "user.available", Kinds: []string{orbitrecord.KindBoolean}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keyTree() =\n%+v\nwant\n%+v", got, want)
	}
}

// TestKeyTree_ABareArrayAndAVerbatimSubtree verifies the two shapes the Orbit
// answers need beyond an object: a body that is an array, whose elements begin
// at the element marker, and a subtree kept as its root, whose keys (a JSON
// Schema's $ref here) would otherwise be refused as data.
func TestKeyTree_ABareArrayAndAVerbatimSubtree(t *testing.T) {
	body := `[{"name":"t","parameters":{"properties":{"$ref":{}}}}]`
	got, err := keyTree("application/json", []byte(body), []string{"[].parameters"})
	if err != nil {
		t.Fatalf("keyTree() error: %v", err)
	}
	want := []orbitrecord.Key{
		{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindArray}},
		{Path: "[]", Kinds: []string{orbitrecord.KindObject}},
		{Path: "[].name", Kinds: []string{orbitrecord.KindString}},
		{Path: "[].parameters", Kinds: []string{orbitrecord.KindObject}, Verbatim: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("keyTree() =\n%+v\nwant\n%+v", got, want)
	}
	root, err := keyTree("application/json", []byte(`{"$defs":{}}`), []string{orbitrecord.Root})
	if err != nil || !reflect.DeepEqual(root, []orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindObject}, Verbatim: true}}) {
		t.Errorf("a verbatim body = %+v, %v", root, err)
	}
}

// TestKeyTree_WhatItRefuses verifies the two answers a recording must not
// write down: one that claims to be JSON and is not, and one whose key is not
// a key name outside a verbatim subtree, which would put a value in the record.
func TestKeyTree_WhatItRefuses(t *testing.T) {
	if _, err := keyTree("application/json", []byte("{"), nil); err == nil || !strings.Contains(err.Error(), "declared application/json and does not decode") {
		t.Errorf("keyTree(broken) error = %v", err)
	}
	_, err := keyTree("application/json", []byte(`{"projects":{"plens1/kg":1}}`), nil)
	if err == nil || !strings.Contains(err.Error(), `the key "plens1/kg" under projects, which is not a key name`) {
		t.Errorf("keyTree(data key) error = %v", err)
	}
	if _, err = keyTree("application/json", []byte(`[{"a":{"$x":1}}]`), nil); err == nil || !strings.Contains(err.Error(), `under [].a`) {
		t.Errorf("keyTree(data key in an element) error = %v", err)
	}
}

// TestKeyTree_ABodyThatIsNotJSON_IsText verifies the llm answer to a query,
// which is text/plain, is recorded as one text root.
func TestKeyTree_ABodyThatIsNotJSON_IsText(t *testing.T) {
	got, err := keyTree("text/plain", []byte("@header\n"), nil)
	if err != nil || !reflect.DeepEqual(got, []orbitrecord.Key{{Path: orbitrecord.Root, Kinds: []string{orbitrecord.KindText}}}) {
		t.Errorf("keyTree(text) = %+v, %v", got, err)
	}
}

// TestKindOf_NamesEveryJSONKind verifies each decoded value's kind, numbers
// as json.Number since the decoder keeps them that way.
func TestKindOf_NamesEveryJSONKind(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{nil, orbitrecord.KindNull},
		{true, orbitrecord.KindBoolean},
		{json.Number("1"), orbitrecord.KindNumber},
		{"s", orbitrecord.KindString},
		{[]any{}, orbitrecord.KindArray},
		{map[string]any{}, orbitrecord.KindObject},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			if got := kindOf(tc.value); got != tc.want {
				t.Errorf("kindOf(%v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}
