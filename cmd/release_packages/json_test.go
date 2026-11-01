// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// json_test.go covers the ordered JSON tree: what it reads and refuses, how
// it keeps an object's members in place, and the two ways it prints a
// value. That pretty printing is jq's byte for byte is held by the platform
// record in manifest_test.go; these cases cover what no manifest holds.
package main

import (
	"testing"
)

// TestParseJSON_Documents_AreReadOrRefused covers what parseJSON accepts:
// exactly one document, every kind, a duplicated key in its first place with
// its last value, and a number as written.
func TestParseJSON_Documents_AreReadOrRefused(t *testing.T) {
	t.Parallel()

	// A refusal of the decoder's own is worded by encoding/json, whose words
	// are not this command's to hold, so only the refusal is asserted.
	for _, tc := range []struct {
		name, input, want, refused string
	}{
		{name: "every kind", input: `{"a":[null,true,false,1.50,"x"],"b":{}}`, want: `{"a":[null,true,false,1.50,"x"],"b":{}}`},
		{name: "a duplicated key", input: `{"a":1,"b":2,"a":3}`, want: `{"a":3,"b":2}`},
		{name: "a bare value", input: ` "x" `, want: `"x"`},
		{name: "two documents", input: `{} {}`, refused: "more than one JSON value"},
		{name: "nothing", input: ``, refused: "unexpected EOF"},
		{name: "an unfinished array", input: `[1,`, refused: "*"},
		{name: "an unfinished object", input: `{"a":`, refused: "*"},
		{name: "an object missing its key", input: `{1:2}`, refused: "*"},
		{name: "an array with a bad item", input: `[1,}`, refused: "*"},
		{name: "an unclosed array", input: `[1 2]`, refused: "*"},
		{name: "an unclosed object", input: `{"a":1 "b":2}`, refused: "*"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseJSON([]byte(tc.input))
			if tc.refused != "" {
				if err == nil || (tc.refused != "*" && err.Error() != tc.refused) {
					t.Errorf("parseJSON error = %v, want %q", err, tc.refused)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseJSON: %v", err)
			}
			if got.compact() != tc.want {
				t.Errorf("parseJSON = %s, want %s", got.compact(), tc.want)
			}
		})
	}
}

// TestJSONValue_Pretty_IndentsAsJQDoes covers the printer on nesting and on
// every escape, the shapes the record's escapes manifest holds, written out
// so a change to one is seen here without a record to read.
func TestJSONValue_Pretty_IndentsAsJQDoes(t *testing.T) {
	t.Parallel()

	value, err := parseJSON([]byte(`{"a":[1,{"b":[]}],"c":{},"d":"\"\\\b\f\n\r\t\u0001\u007fé /<>&"}`))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"a\": [\n    1,\n    {\n      \"b\": []\n    }\n  ],\n  \"c\": {},\n  \"d\": \"\\\"\\\\\\b\\f\\n\\r\\t\\u0001\\u007fé /<>&\"\n}\n"
	if got := string(value.pretty()); got != want {
		t.Errorf("pretty =\n%q\nwant\n%q", got, want)
	}
}

// TestJSONValue_Equal_ComparesKindAndContent covers jq's equality on every
// way two values can differ, and that an object's members are compared by
// name rather than by place.
func TestJSONValue_Equal_ComparesKindAndContent(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, a, b string
		want       bool
	}{
		{name: "the same object in another order", a: `{"a":1,"b":2}`, b: `{"b":2,"a":1}`, want: true},
		{name: "two kinds", a: `1`, b: `"1"`},
		{name: "two booleans", a: `true`, b: `false`},
		{name: "two strings", a: `"a"`, b: `"b"`},
		{name: "two lengths of array", a: `[1]`, b: `[1,1]`},
		{name: "two items", a: `[1]`, b: `[2]`},
		{name: "two member counts", a: `{"a":1}`, b: `{"a":1,"b":1}`},
		{name: "a member the other lacks", a: `{"a":1}`, b: `{"b":1}`},
		{name: "two values of a member", a: `{"a":1}`, b: `{"a":2}`},
		{name: "the same nested value", a: `[{"a":[null]}]`, b: `[{"a":[null]}]`, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a, errA := parseJSON([]byte(tc.a))
			b, errB := parseJSON([]byte(tc.b))
			if errA != nil || errB != nil {
				t.Fatal(errA, errB)
			}
			if got := a.equal(b); got != tc.want {
				t.Errorf("%s equal %s = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestJSONValue_Accessors_ReadAsJQReads covers truth, interpolation, a
// missing member, the string items of an array and the type names jq's
// errors spell.
func TestJSONValue_Accessors_ReadAsJQReads(t *testing.T) {
	t.Parallel()

	value, err := parseJSON([]byte(`{"s":"x","n":null,"f":false,"z":0,"l":["a",1,"b"],"o":{"k":true}}`))
	if err != nil {
		t.Fatal(err)
	}
	var missing *jsonValue
	for _, tc := range []struct {
		name string
		got  any
		want any
	}{
		{name: "a string is truthy", got: value.field("s").truthy(), want: true},
		{name: "null is not", got: value.field("n").truthy(), want: false},
		{name: "false is not", got: value.field("f").truthy(), want: false},
		{name: "zero is", got: value.field("z").truthy(), want: true},
		{name: "nothing is not", got: missing.truthy(), want: false},
		{name: "a string interpolates as itself", got: value.field("s").interpolate(), want: "x"},
		{name: "an object interpolates as its JSON", got: value.field("o").interpolate(), want: `{"k":true}`},
		{name: "a missing member is null", got: value.field("absent").kind, want: jsonNull},
		{name: "a member of a string is null", got: value.field("s").field("x").kind, want: jsonNull},
		{name: "has finds a member", got: value.field("o").has("k"), want: true},
		{name: "has finds no other", got: value.field("o").has("j"), want: false},
		{name: "the strings of a list", got: value.field("l").joinStrings(","), want: "a,b"},
		{name: "the type of a list", got: value.field("l").typeName(), want: "array"},
		{name: "the type of a boolean", got: value.field("f").typeName(), want: "boolean"},
		{name: "the type of a number", got: value.field("z").typeName(), want: "number"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if tc.got != tc.want {
				t.Errorf("got %v, want %v", tc.got, tc.want)
			}
		})
	}
}

// TestJSONValue_CloneAndWithout_LeaveTheOriginal verifies that a clone and
// an object without a member are copies: writing to either leaves the value
// they came from as it was.
func TestJSONValue_CloneAndWithout_LeaveTheOriginal(t *testing.T) {
	t.Parallel()

	original, err := parseJSON([]byte(`{"a":[1],"b":{"c":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	clone := original.clone()
	clone.field("a").items[0] = jsonText("x")
	clone.field("b").set("c", jsonText("y"))
	clone.set("d", jsonList())
	without := original.without("a")
	without.set("e", jsonList())
	if got := original.compact(); got != `{"a":[1],"b":{"c":2}}` {
		t.Errorf("the original became %s", got)
	}
	if got := clone.compact(); got != `{"a":["x"],"b":{"c":"y"},"d":[]}` {
		t.Errorf("the clone is %s", got)
	}
	if got := without.compact(); got != `{"b":{"c":2},"e":[]}` {
		t.Errorf("without a is %s", got)
	}
}
