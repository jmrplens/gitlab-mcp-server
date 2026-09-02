// markdown_test.go contains tests for the to-do Markdown formatters, covering
// the escaping a GitLab-authored target title must survive before it is
// rendered as the visible text of a link.
package todos

import (
	"strings"
	"testing"
)

// TestFormatListMarkdownString_TargetTitle_CannotChooseTheLinkDestination
// verifies that a to-do target title cannot close the link label it sits in
// and open a destination of its own.
//
// The title is the issue or merge request title, written by whoever opened it,
// and the list formatter tells the model to preserve the clickable links. A
// title ending the label used to produce a link whose text reads like a GitLab
// issue and whose destination is a host of the title author's choosing, carried
// to the user on this server's own instruction.
func TestFormatListMarkdownString_TargetTitle_CannotChooseTheLinkDestination(t *testing.T) {
	const targetURL = "https://gitlab.example.com/g/p/-/issues/1"
	tests := []struct {
		name    string
		title   string
		want    string
		wantNot string
	}{
		{
			name:  "ordinary title still links to GitLab",
			title: "Fix login",
			want:  "[Fix login](" + targetURL + ")",
		},
		{
			name:    "title closing the label cannot open its own destination",
			title:   "Fix login](http://attacker.invalid/x)",
			want:    "](" + targetURL + ")",
			wantNot: "[Fix login](http://attacker.invalid/x)",
		},
		{
			name:    "title opening a label of its own is escaped",
			title:   "Fix [login]",
			wantNot: "[Fix [login]](",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatListMarkdownString(ListOutput{
				Todos: []Output{{
					ID:         1,
					ActionName: "assigned",
					Target:     &TodoTargetOut{Title: tt.title},
					TargetType: "Issue",
					State:      "pending",
					TargetURL:  targetURL,
				}},
			})
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("FormatListMarkdownString() = %q, want it to contain %q", got, tt.want)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("FormatListMarkdownString() = %q, must not contain %q", got, tt.wantNot)
			}
		})
	}
}

// TestFormatOutputMarkdownString_TargetTitleAndURL_CannotEndTheLinkTheySitIn
// verifies the same containment in the single-to-do view, on both halves of
// the link: the title cannot end the label, and a target URL carrying a
// parenthesis cannot end the destination and leave the rest as prose.
func TestFormatOutputMarkdownString_TargetTitleAndURL_CannotEndTheLinkTheySitIn(t *testing.T) {
	tests := []struct {
		name      string
		title     string
		targetURL string
		want      string
		wantNot   string
	}{
		{
			name:      "ordinary title still links to GitLab",
			title:     "Fix login",
			targetURL: "https://gitlab.example.com/g/p/-/issues/1",
			want:      "[Fix login](https://gitlab.example.com/g/p/-/issues/1)",
		},
		{
			name:      "title closing the label cannot open its own destination",
			title:     "Fix login](http://attacker.invalid/x)",
			targetURL: "https://gitlab.example.com/g/p/-/issues/1",
			want:      "](https://gitlab.example.com/g/p/-/issues/1)",
			wantNot:   "[Fix login](http://attacker.invalid/x)",
		},
		{
			name:      "parenthesis in the destination is percent-encoded",
			title:     "Fix login",
			targetURL: "https://gitlab.example.com/g/p/-/issues/1)x",
			want:      "%29x)",
			wantNot:   "/issues/1)x)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatOutputMarkdownString(Output{
				ID:         1,
				ActionName: "assigned",
				Target:     &TodoTargetOut{Title: tt.title},
				TargetType: "Issue",
				State:      "pending",
				TargetURL:  tt.targetURL,
			})
			if tt.want != "" && !strings.Contains(got, tt.want) {
				t.Errorf("FormatOutputMarkdownString() = %q, want it to contain %q", got, tt.want)
			}
			if tt.wantNot != "" && strings.Contains(got, tt.wantNot) {
				t.Errorf("FormatOutputMarkdownString() = %q, must not contain %q", got, tt.wantNot)
			}
		})
	}
}
