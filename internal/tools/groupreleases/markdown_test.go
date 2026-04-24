// markdown_test.go contains unit tests for group release Markdown formatting functions.
package groupreleases

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// TestFormatListMarkdown validates the Markdown formatter for group releases.
// It covers empty results, a single release, multiple releases, and special
// characters that require escaping in Markdown table cells.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    ListOutput
		wantSub  []string
		wantNot  []string
		wantFull string
	}{
		{
			name:     "empty releases returns no-results message",
			input:    ListOutput{},
			wantFull: "No group releases found.\n",
		},
		{
			name: "single release renders table with one row",
			input: ListOutput{
				Releases: []Output{
					{
						TagName:    "v1.0.0",
						Name:       "First Release",
						ReleasedAt: "2026-06-01",
						Author:     "admin",
					},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 1, TotalPages: 1},
			},
			wantSub: []string{
				"| Tag | Name | Released | Author |",
				"| v1.0.0 | First Release | 2026-06-01 | admin |",
			},
		},
		{
			name: "multiple releases renders all rows",
			input: ListOutput{
				Releases: []Output{
					{TagName: "v2.0.0", Name: "Second", ReleasedAt: "2026-07-01", Author: "dev1"},
					{TagName: "v1.0.0", Name: "First", ReleasedAt: "2026-06-01", Author: "dev2"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 2, TotalPages: 1},
			},
			wantSub: []string{
				"| v2.0.0 | Second | 2026-07-01 | dev1 |",
				"| v1.0.0 | First | 2026-06-01 | dev2 |",
			},
		},
		{
			name: "special characters in tag and name are escaped",
			input: ListOutput{
				Releases: []Output{
					{TagName: "v1|beta", Name: "Rel|ease", ReleasedAt: "2026-01-01", Author: "user"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 1, TotalPages: 1},
			},
			wantSub: []string{"v1"},
			wantNot: []string{"| v1|beta |"},
		},
		{
			name: "empty optional fields render blank cells",
			input: ListOutput{
				Releases: []Output{
					{TagName: "v0.1.0", Name: "Early"},
				},
				Pagination: toolutil.PaginationOutput{TotalItems: 1, TotalPages: 1},
			},
			wantSub: []string{
				"| v0.1.0 | Early |  |  |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdown(tt.input)
			if tt.wantFull != "" && got != tt.wantFull {
				t.Fatalf("got %q, want %q", got, tt.wantFull)
			}
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("output missing %q\ngot:\n%s", sub, got)
				}
			}
			for _, not := range tt.wantNot {
				if strings.Contains(got, not) {
					t.Errorf("output should NOT contain %q\ngot:\n%s", not, got)
				}
			}
		})
	}
}
