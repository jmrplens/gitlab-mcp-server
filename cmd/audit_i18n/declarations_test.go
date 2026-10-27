// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/sourcegate"
)

// TestConversionsOnPurpose_EveryEntry_GivesADefinedCategoryAndAReason holds
// the table to the terms every declaration table here keeps: a category
// somebody defined, and words saying why.
func TestConversionsOnPurpose_EveryEntry_GivesADefinedCategoryAndAReason(t *testing.T) {
	table := sourcegate.Table{Declared: conversionsOnPurpose, Categories: categories}
	if unknown := table.UnknownCategories(); len(unknown) != 0 {
		t.Errorf("entries with an undefined category: %v", unknown)
	}
	for key, entry := range conversionsOnPurpose {
		t.Run(key, func(t *testing.T) {
			if strings.TrimSpace(entry.Reason) == "" {
				t.Errorf("%s gives no reason", key)
			}
		})
	}
}

// TestConversionsOnPurpose_TheEscapingAudit_IsExcusedByItsEntry holds the one
// entry to the conversion it stands for, over the real tree: the Markdown
// escaping audit asks the shipped catalog for a key it read as a string, and
// its entry answers that and nothing else.
func TestConversionsOnPurpose_TheEscapingAudit_IsExcusedByItsEntry(t *testing.T) {
	report, err := audit(auditConfig{
		dir:      repoRoot(t),
		patterns: []string{"./cmd/audit_md_escaping"},
		catalog:  catalogPath,
		declared: conversionsOnPurpose,
	})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !report.OK() {
		t.Errorf("the escaping audit is not clean: %s", lines(spell(report.Findings)))
	}
	if len(report.Excused) != 1 || declarationKey(report.Excused[0]) != "cmd/audit_md_escaping:catalogEnglishTemplate" {
		t.Errorf("excused = %s, want the escaping audit's one conversion", lines(spell(report.Excused)))
	}
}

// TestDeclarationKey_Finding_IsPackageAndFunction holds the spelling an entry
// is written in.
func TestDeclarationKey_Finding_IsPackageAndFunction(t *testing.T) {
	if got := declarationKey(Finding{Package: "cmd/x", Func: "T.M"}); got != "cmd/x:T.M" {
		t.Errorf("declarationKey = %q", got)
	}
}
