package tools

import (
	"strings"
	"testing"
)

func TestCatalogGroupDescription_MRChangesPagination_StatesReturnAndParameters(t *testing.T) {
	description := catalogGroupDescription("gitlab_mr_review")
	for _, want := range []string{
		"truncated_files, pagination: {has_more, next_page, next_page_token",
		"next_page as page for offset pagination",
		"Use per_page: 100",
		"changes_get: page, per_page, pagination, page_token",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(description, want) {
				t.Errorf("MR review description lacks %q", want)
			}
		})
	}
	for line := range strings.SplitSeq(description, "\n") {
		if strings.HasPrefix(line, "- changes_get:") && strings.Contains(strings.ToLower(line), "keyset") {
			t.Errorf("MR changes description advertises keyset: %s", line)
		}
	}
	if strings.Contains(description, "- changes_get.page_token") {
		t.Error("MR changes parameter guidance advertises page_token")
	}
}
