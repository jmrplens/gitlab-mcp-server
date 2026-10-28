// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package params

import "testing"

// TestOfferer_EachWayAnInputPlacesAParam_IsFound verifies every way a param
// is found among the inputs of the actions reaching its route: by name, under
// the field an alias names, flattened the way the negated filters are
// spelled, and through the nested schema of a list or an object, and the
// ways it is not: a name no input has, a nested key the item schema lacks,
// and a segment no field holds.
func TestOfferer_EachWayAnInputPlacesAParam_IsFound(t *testing.T) {
	object := func(properties map[string]any) map[string]any { return map[string]any{"properties": properties} }
	offer := offerer{
		schemas: map[string]map[string]any{
			"a.list": object(map[string]any{"search": map[string]any{}, "not_author_id": map[string]any{}}),
			"a.edit": object(map[string]any{
				"title":    map[string]any{},
				"headers":  map[string]any{"items": object(map[string]any{"key": map[string]any{}})},
				"position": object(map[string]any{"line_range": object(map[string]any{"start": map[string]any{}})}),
			}),
		},
		aliases: map[string]map[string]string{"a.edit": {"name": "title"}},
		actions: []string{"a.edit", "a.list"},
	}
	cases := map[string]bool{
		"search":                      true,
		"name":                        true,
		"not[author_id]":              true,
		"headers[key]":                true,
		"headers[][key]":              true,
		"position[line_range][start]": true,
		"absent":                      false,
		"headers[value]":              false,
		"missing[key]":                false,
		"position[line_range][end]":   false,
	}
	for param, want := range cases {
		t.Run(param, func(t *testing.T) {
			if got := offer.offers(param); got != want {
				t.Errorf("offers(%q) = %t, want %t", param, got, want)
			}
		})
	}
}
