// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// declarations_test.go holds the declaration table to what an entry has to
// say, and the validation to every way an entry can fail to say it.
package main

import (
	"slices"
	"testing"
)

// TestAcceptedFindings_EveryEntryIsReviewable holds the real table to a key
// that names an advisory and a package, a defined category, a reason in words
// and, for a fix that cannot be taken yet, the day it can. The table is empty,
// which is the state it is meant to stay in.
func TestAcceptedFindings_EveryEntryIsReviewable(t *testing.T) {
	t.Parallel()

	if invalid := invalidDeclarations(acceptedFindings); len(invalid) != 0 {
		t.Fatalf("declarations that cannot be read as written: %v", invalid)
	}
}

// TestCategories_EachSaysWhatItMeans: a category is an excuse a reviewer
// reads, so one without a description excuses nothing anyone agreed to, and
// the two the maintainer allowed are the only ones there are.
func TestCategories_EachSaysWhatItMeans(t *testing.T) {
	t.Parallel()

	for _, name := range []string{categoryFixNotYetAdoptable, categoryNotAffected} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if categories[name] == "" {
				t.Fatalf("category %q has no description", name)
			}
		})
	}
	if len(categories) != 2 {
		t.Errorf("categories = %v, want the two this file names", categories)
	}
}

// TestDeclarationKey_IsTheAdvisoryAndThePackage: the key a finding is looked
// up under.
func TestDeclarationKey_IsTheAdvisoryAndThePackage(t *testing.T) {
	t.Parallel()

	if got := declarationKey("CVE-2026-85091", "zlib"); got != "CVE-2026-85091 zlib" {
		t.Fatalf("declarationKey = %q, want the advisory and the package separated by one space", got)
	}
}

// TestInvalidDeclarations_NamesEachProblemInOrder covers every way an entry
// can fail to be read, and the entries that read cleanly beside them: a key in
// each advisory spelling Trivy reports for an Alpine package, a package name
// in every character Alpine allows, and each category with what it needs.
func TestInvalidDeclarations_NamesEachProblemInOrder(t *testing.T) {
	t.Parallel()

	const reason = "the reason"
	pending := declaration{category: categoryFixNotYetAdoptable, reason: reason, until: "2026-11-02"}
	got := invalidDeclarations(map[string]declaration{
		"CVE-2026-85091 zlib":         pending,
		"GHSA-2c8m-4q7x-9vfw libssl3": pending,
		"ALPINE-13661 busybox-binsh":  {category: categoryNotAffected, reason: reason},
		"CVE-2026-1 py3-c++_lib.x":    {category: categoryNotAffected, reason: reason},
		"CVE-2026-85091":              pending,
		"CVE-2026-85091 zlib more":    pending,
		"CVE-2026-85092  zlib":        pending,
		" CVE-2026-85093 zlib":        pending,
		"cve-2026-85094 zlib":         pending,
		"CVE-2026-85095 Zlib":         pending,
		"CVE- zlib":                   pending,
		"CVE-2026-85096 zlib":         {category: "invented", reason: reason, until: "2026-11-02"},
		"CVE-2026-85097 zlib":         {category: categoryNotAffected, reason: "   "},
		"CVE-2026-85098 zlib":         {category: categoryFixNotYetAdoptable, reason: reason},
		"CVE-2026-85099 zlib":         {category: categoryFixNotYetAdoptable, reason: reason, until: "2026-11-31"},
		"CVE-2026-85100 zlib":         {category: categoryFixNotYetAdoptable, reason: reason, until: "next week"},
		"CVE-2026-85101 zlib":         {category: categoryNotAffected, reason: reason, until: "2026-11-02"},
		"CVE-2026-85102 zlib":         {category: categoryFixNotYetAdoptable, reason: reason, until: "2026-11-2"},
		"CVE-2026-85103 zlib":         {category: categoryNotAffected, reason: reason, until: "next week"},
	})
	want := []string{
		`" CVE-2026-85093 zlib": the key is not an advisory id and a package separated by one space`,
		`"CVE- zlib": the key is not an advisory id and a package separated by one space`,
		`"CVE-2026-85091 zlib more": the key is not an advisory id and a package separated by one space`,
		`"CVE-2026-85091": the key is not an advisory id and a package separated by one space`,
		`"CVE-2026-85092  zlib": the key is not an advisory id and a package separated by one space`,
		`"CVE-2026-85095 Zlib": the key is not an advisory id and a package separated by one space`,
		`"CVE-2026-85096 zlib": the category "invented" is not one this command defines`,
		`"CVE-2026-85097 zlib": gives no reason`,
		`"CVE-2026-85098 zlib": a fix-not-yet-adoptable entry names the day its fix can be taken as until, YYYY-MM-DD, and this one gives ""`,
		`"CVE-2026-85099 zlib": a fix-not-yet-adoptable entry names the day its fix can be taken as until, YYYY-MM-DD, and this one gives "2026-11-31"`,
		`"CVE-2026-85100 zlib": a fix-not-yet-adoptable entry names the day its fix can be taken as until, YYYY-MM-DD, and this one gives "next week"`,
		`"CVE-2026-85101 zlib": a not-affected entry holds until the image changes and names no until, and this one gives "2026-11-02"`,
		`"CVE-2026-85102 zlib": a fix-not-yet-adoptable entry names the day its fix can be taken as until, YYYY-MM-DD, and this one gives "2026-11-2"`,
		`"CVE-2026-85103 zlib": a not-affected entry holds until the image changes and names no until, and this one gives "next week"`,
		`"cve-2026-85094 zlib": the key is not an advisory id and a package separated by one space`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("invalidDeclarations =\n%q\nwant\n%q", got, want)
	}
}
