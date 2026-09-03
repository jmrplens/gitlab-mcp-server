package main

import "testing"

// TestCatalogActions_ReturnsTheWholeSurface verifies the audit reads the real
// canonical catalog offline: every action arrives with the identity the
// findings quote, and both classifications are present, which is what makes
// the read-only subset meaningful.
func TestCatalogActions_ReturnsTheWholeSurface(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("the canonical catalog produced no actions")
	}

	readOnly, mutating := 0, 0
	for _, item := range actions {
		if item.ID == "" || item.Name == "" || item.Owner == "" {
			t.Fatalf("action %+v is missing the identity a finding has to name", item)
		}
		if item.ReadOnly {
			readOnly++
			continue
		}
		mutating++
	}
	if readOnly == 0 {
		t.Error("no read-only actions, so this audit would check nothing")
	}
	if mutating == 0 {
		t.Error("no mutating actions, which means the classification was not read")
	}
}

// TestCatalogActions_IncludesKnownReadAndWriteActions verifies the two
// classifications land on actions whose nature is not in doubt, so a catalog
// change that inverted the flag would not pass this file.
func TestCatalogActions_IncludesKnownReadAndWriteActions(t *testing.T) {
	actions, err := catalogActions()
	if err != nil {
		t.Fatalf("catalogActions: %v", err)
	}
	byID := make(map[string]action, len(actions))
	for _, item := range actions {
		byID[item.ID] = item
	}

	cases := []struct {
		id           string
		wantReadOnly bool
	}{
		{id: "issue.list", wantReadOnly: true},
		{id: "project.get", wantReadOnly: true},
		{id: "vulnerability.list", wantReadOnly: true},
		{id: "vulnerability.dismiss", wantReadOnly: false},
		{id: "issue.create", wantReadOnly: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			item, ok := byID[testCase.id]
			if !ok {
				t.Fatalf("action %q is not in the catalog", testCase.id)
			}
			if item.ReadOnly != testCase.wantReadOnly {
				t.Errorf("action %q ReadOnly = %t, want %t", testCase.id, item.ReadOnly, testCase.wantReadOnly)
			}
		})
	}
}
