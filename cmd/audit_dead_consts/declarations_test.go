package main

import (
	"strings"
	"testing"
)

// TestDeclarationKey_IsThePackageTheFunctionAndTheName is the one spelling
// both the report and the table use, so a mismatch here silently excuses
// nothing. A local constant carries its function, so an entry for the
// package-level one of the same name cannot excuse it.
func TestDeclarationKey_IsThePackageTheFunctionAndTheName(t *testing.T) {
	cases := []struct {
		name     string
		constant Constant
		want     string
	}{
		{
			name:     "package level",
			constant: Constant{Package: "internal/tools/dynamic", Name: "aliasSourceCatalog"},
			want:     "internal/tools/dynamic:aliasSourceCatalog",
		},
		{
			name:     "inside a function",
			constant: Constant{Package: "internal/tools/dynamic", Func: "search", Name: "limit"},
			want:     "internal/tools/dynamic:search.limit",
		},
		{
			name:     "inside a method",
			constant: Constant{Package: "internal/tools/dynamic", Func: "Registry.Find", Name: "limit"},
			want:     "internal/tools/dynamic:Registry.Find.limit",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := declarationKey(tc.constant); got != tc.want {
				t.Fatalf("declarationKey = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestUnreadOnPurpose_EveryEntry_NamesAPackageAConstantAndAReason keeps the
// table readable: an entry is a claim, and a claim with no reason is a
// silence with extra steps.
func TestUnreadOnPurpose_EveryEntry_NamesAPackageAConstantAndAReason(t *testing.T) {
	for key, reason := range unreadOnPurpose {
		t.Run(key, func(t *testing.T) {
			pkg, name, found := strings.Cut(key, ":")
			if !found || pkg == "" || name == "" {
				t.Fatalf("key %q is not package:constant", key)
			}
			if strings.HasPrefix(pkg, "/") || strings.Contains(pkg, "\\") {
				t.Fatalf("package %q is not spelled the way the repository names one", pkg)
			}
			if len(reason) < 40 {
				t.Fatalf("reason for %q is %q, which does not say why keeping it is right", key, reason)
			}
		})
	}
}

// TestStaleDeclarations_ExcusedEntry_IsNotStale.
func TestStaleDeclarations_ExcusedEntry_IsNotStale(t *testing.T) {
	excused := map[string]struct{}{}
	for key := range unreadOnPurpose {
		excused[key] = struct{}{}
	}
	if stale := staleDeclarations(excused, scannedSet("internal/tools/dynamic")); len(stale) != 0 {
		t.Fatalf("stale = %v, want none", stale)
	}
}

// TestStaleDeclarations_KeyWithoutAPackage_IsPassedOver: the table is written
// by hand, and a malformed key names no package to scope the judgement to.
func TestStaleDeclarations_KeyWithoutAPackage_IsPassedOver(t *testing.T) {
	original := unreadOnPurpose
	t.Cleanup(func() { unreadOnPurpose = original })
	unreadOnPurpose = map[string]string{"noColonHere": "malformed"}
	if stale := staleDeclarations(map[string]struct{}{}, scannedSet("internal/tools/dynamic")); len(stale) != 0 {
		t.Fatalf("stale = %v, want none: a key naming no package cannot be judged", stale)
	}
}

// TestStaleDeclarations_ScannedAndUnused_IsReported.
func TestStaleDeclarations_ScannedAndUnused_IsReported(t *testing.T) {
	original := unreadOnPurpose
	t.Cleanup(func() { unreadOnPurpose = original })
	unreadOnPurpose = map[string]string{"internal/tools/issues:gone": "the constant it named was deleted"}
	stale := staleDeclarations(map[string]struct{}{}, scannedSet("internal/tools/issues"))
	if len(stale) != 1 || stale[0] != "internal/tools/issues:gone" {
		t.Fatalf("stale = %v, want the one entry the run could judge", stale)
	}
}
