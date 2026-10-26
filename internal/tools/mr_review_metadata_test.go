package tools

import (
	"strings"
	"testing"
)

func TestCatalogGroupDescription_MRChangesPagination_StatesReturnAndParameters(t *testing.T) {
	description := catalogGroupDescription("gitlab_mr_review")
	for _, want := range []string{
		"truncated_files, pagination: {has_more, next_page, next_page_token",
		"next_page_token unchanged as page_token",
		"changes_get: page, per_page, pagination, page_token",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(description, want) {
				t.Errorf("MR review description lacks %q", want)
			}
		})
	}
}
