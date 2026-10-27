// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// TestWithoutContextOnPurpose_EveryEntryIsReviewable holds the real table to
// what an entry has to say: a category that is defined and a reason in words.
// The table is empty today, which is the state it is meant to stay in.
func TestWithoutContextOnPurpose_EveryEntryIsReviewable(t *testing.T) {
	table := sourcegate.Table{Declared: withoutContextOnPurpose, Categories: categories}
	if unknown := table.UnknownCategories(); len(unknown) != 0 {
		t.Fatalf("declarations with an undefined category: %v", unknown)
	}
	for key, entry := range withoutContextOnPurpose {
		t.Run(key, func(t *testing.T) {
			if entry.Reason == "" {
				t.Fatal("declaration gives no reason")
			}
		})
	}
}

// TestCategories_EachSaysWhatItMeans: a category is an excuse a reviewer
// reads, so one without a description excuses nothing anyone agreed to.
func TestCategories_EachSaysWhatItMeans(t *testing.T) {
	if len(categories) == 0 {
		t.Fatal("no category is defined")
	}
	for name, meaning := range categories {
		t.Run(name, func(t *testing.T) {
			if meaning == "" {
				t.Fatal("category has no description")
			}
		})
	}
}

// TestDeclarationKey_IsThePackageAndTheFunction: the key a finding is looked
// up under.
func TestDeclarationKey_IsThePackageAndTheFunction(t *testing.T) {
	if got := declarationKey(Finding{Package: "internal/tools/x", Func: "Type.Method"}); got != "internal/tools/x:Type.Method" {
		t.Fatalf("declarationKey = %q, want internal/tools/x:Type.Method", got)
	}
}
