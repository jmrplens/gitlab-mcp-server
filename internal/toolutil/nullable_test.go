// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// nullableProbe stands in for an update input with one field GitLab clears
// with null, beside an ordinary pointer.
type nullableProbe struct {
	Weight Nullable[int64] `json:"weight,omitzero" jsonschema:"Weight, 0 or higher. Send null to remove it"`
	Title  *string         `json:"title,omitempty"`
}

// TestNullable_Decode_TellsAbsentNullAndValueApart pins the three answers on
// both decode paths an action's parameters take: encoding/json, which the
// individual surface's arguments reach through [UnmarshalParams] as well, and
// [UnmarshalParams] with the coercion chain the meta and dynamic surfaces add.
// A pointer reads null and absence as the same nil, which is the defect of
// issue 1390, so the null case is the one that matters, and 0 has to stay a
// value of its own.
func TestNullable_Decode_TellsAbsentNullAndValueApart(t *testing.T) {
	cases := []struct {
		name      string
		params    map[string]any
		wantZero  bool
		wantNull  bool
		wantValue *int64
	}{
		{name: "absent", params: map[string]any{"title": "t"}, wantZero: true},
		{name: "null", params: map[string]any{"weight": nil}, wantNull: true},
		{name: "zero is a value", params: map[string]any{"weight": 0}, wantValue: new(int64(0))},
		{name: "a value", params: map[string]any{"weight": 3}, wantValue: new(int64(3))},
		{name: "a numeric string", params: map[string]any{"weight": "3"}, wantValue: new(int64(3))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := UnmarshalParams[nullableProbe](tc.params)
			if err != nil {
				t.Fatalf("UnmarshalParams(%v) error = %v", tc.params, err)
			}
			if got := input.Weight.IsZero(); got != tc.wantZero {
				t.Errorf("IsZero() = %t, want %t", got, tc.wantZero)
			}
			if got := input.Weight.IsNull(); got != tc.wantNull {
				t.Errorf("IsNull() = %t, want %t", got, tc.wantNull)
			}
			got := input.Weight.Ptr()
			switch {
			case tc.wantValue == nil && got != nil:
				t.Errorf("Ptr() = %d, want nil", *got)
			case tc.wantValue != nil && (got == nil || *got != *tc.wantValue):
				t.Errorf("Ptr() = %v, want %d", got, *tc.wantValue)
			}
		})
	}
}

// TestNullable_Decode_RefusesWhatTheTypeRefuses verifies a value the held type
// cannot take is an error rather than a removal, on the direct decode and
// through [UnmarshalParams], which names the parameter.
func TestNullable_Decode_RefusesWhatTheTypeRefuses(t *testing.T) {
	var probe nullableProbe
	if err := json.Unmarshal([]byte(`{"weight":"heavy"}`), &probe); err == nil {
		t.Fatalf("json.Unmarshal of a word into a weight succeeded with %+v", probe)
	}
	if err := json.Unmarshal([]byte(`{"weight":true}`), &probe); err == nil {
		t.Fatalf("json.Unmarshal of a boolean into a weight succeeded with %+v", probe)
	}
	_, err := UnmarshalParams[nullableProbe](map[string]any{"weight": "heavy"})
	if err == nil || !strings.Contains(err.Error(), `"weight"`) {
		t.Fatalf("UnmarshalParams with a word error = %v, want one naming the weight", err)
	}
}

// TestNullable_Marshal_WritesWhatWasSent verifies an input marshals back to
// what the caller sent: an absent key left out by omitzero, null as null, and
// a value, 0 included, as itself. A safe-mode preview and a log line marshal
// the input, and an absent weight written as null there would read as a
// removal nobody asked for.
func TestNullable_Marshal_WritesWhatWasSent(t *testing.T) {
	cases := []struct {
		name  string
		probe nullableProbe
		want  string
	}{
		{name: "absent", probe: nullableProbe{}, want: `{}`},
		{name: "null", probe: nullableProbe{Weight: NullableNull[int64]()}, want: `{"weight":null}`},
		{name: "zero", probe: nullableProbe{Weight: NullableOf[int64](0)}, want: `{"weight":0}`},
		{name: "a value", probe: nullableProbe{Weight: NullableOf[int64](7)}, want: `{"weight":7}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.probe)
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("json.Marshal() = %s, want %s", got, tc.want)
			}
		})
	}
	// A field declared without omitzero still says nothing was sent rather
	// than inventing a value.
	got, err := json.Marshal(struct {
		Weight Nullable[int64] `json:"weight"`
	}{})
	if err != nil || string(got) != `{"weight":null}` {
		t.Errorf("json.Marshal(absent without omitzero) = %s, %v, want {\"weight\":null}", got, err)
	}
}

// TestNullable_Ptr_IsACopy verifies the pointer handed to an option struct
// does not alias the input, so a handler that adjusts the value it sends
// cannot change what the input says was asked.
func TestNullable_Ptr_IsACopy(t *testing.T) {
	weight := NullableOf[int64](5)
	sent := weight.Ptr()
	*sent = 9
	if again := weight.Ptr(); *again != 5 {
		t.Errorf("Ptr() after editing an earlier pointer = %d, want 5", *again)
	}
}

// TestSchemaForOptions_Nullable_PublishesWhatAPointerDid pins that moving a
// field from *int64 to Nullable[int64] leaves the published schema as it was:
// the pair [null integer], the description kept, and the field optional.
func TestSchemaForOptions_Nullable_PublishesWhatAPointerDid(t *testing.T) {
	schema := buildSchemaForType(reflect.TypeFor[nullableProbe]())
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("the schema carries no properties: %v", schema)
	}
	weight, ok := properties["weight"].(map[string]any)
	if !ok {
		t.Fatalf("the schema carries no weight property: %v", properties)
	}
	types, ok := weight["type"].([]any)
	if !ok || len(types) != 2 || types[0] != "null" || types[1] != "integer" {
		t.Errorf("weight publishes type %v, want the pair [null integer] a *int64 publishes", weight["type"])
	}
	if weight["description"] != "Weight, 0 or higher. Send null to remove it" {
		t.Errorf("weight lost its description: %v", weight["description"])
	}
	if _, hasProperties := weight["properties"]; hasProperties {
		t.Errorf("weight publishes the struct's own fields: %v", weight)
	}
	required, _ := schema["required"].([]any)
	for _, name := range required {
		if name == "weight" {
			t.Errorf("weight is required, want it optional through omitzero: %v", required)
		}
	}
}

// TestNullableElemType_ReadsTheHeldType verifies the coercion chain is told
// the type a Nullable holds and every other type as itself.
func TestNullableElemType_ReadsTheHeldType(t *testing.T) {
	if got := nullableElemType(reflect.TypeFor[Nullable[int64]]()); got != reflect.TypeFor[int64]() {
		t.Errorf("nullableElemType(Nullable[int64]) = %v, want int64", got)
	}
	if got := nullableElemType(reflect.TypeFor[string]()); got != reflect.TypeFor[string]() {
		t.Errorf("nullableElemType(string) = %v, want string", got)
	}
}
