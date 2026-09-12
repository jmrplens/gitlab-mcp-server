// markdown_test.go contains unit tests for group SCIM Markdown formatting
// functions. Each case compares the whole document the formatter renders.
package groupscim

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// scimCardHints is the guidance section every SCIM identity card ends with.
const scimCardHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'group_scim.update' to modify the external UID\n" +
	"- Use action 'group_scim.delete' to remove this identity\n"

// scimListHints is the guidance section a list of SCIM identities ends with.
const scimListHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'group_scim.get' to view one identity in full\n"

// TestFormatOutputMarkdown verifies the whole card a SCIM identity renders:
// the external UID as a code span a reader copies back verbatim, the active
// flag as the emoji rather than as "true", and nothing at all for an identity
// GitLab never sent.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input Output
		want  string
	}{
		{
			name:  "active identity with all fields",
			input: Output{ExternUID: "ext-uid-123", UserID: 42, Active: true},
			want: "## SCIM Identity\n\n" +
				"- **External UID**: `ext-uid-123`\n" +
				"- **User ID**: 42\n" +
				"- **Active**: " + toolutil.BoolEmoji(true) + "\n" +
				scimCardHints,
		},
		{
			name:  "inactive identity",
			input: Output{ExternUID: "ext-uid-456", UserID: 99, Active: false},
			want: "## SCIM Identity\n\n" +
				"- **External UID**: `ext-uid-456`\n" +
				"- **User ID**: 99\n" +
				"- **Active**: " + toolutil.BoolEmoji(false) + "\n" +
				scimCardHints,
		},
		{
			name:  "zero-value identity returns empty",
			input: Output{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatOutputMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown verifies the whole table a list of SCIM identities
// renders, the external UID in a code span whose pipe is escaped for the cell.
func TestFormatListMarkdown(t *testing.T) {
	const header = "| External UID | User ID | Active |\n| --- | --- | --- |\n"

	tests := []struct {
		name  string
		input ListOutput
		want  string
	}{
		{
			name:  "empty list",
			input: ListOutput{},
			want:  "No SCIM identities found.\n",
		},
		{
			name:  "empty identities slice",
			input: ListOutput{Identities: []Output{}},
			want:  "No SCIM identities found.\n",
		},
		{
			name:  "single identity",
			input: ListOutput{Identities: []Output{{ExternUID: "uid-1", UserID: 10, Active: true}}},
			want: "## SCIM Identities (1)\n\n" + header +
				"| `uid-1` | 10 | " + toolutil.BoolEmoji(true) + " |\n" +
				scimListHints,
		},
		{
			name: "multiple identities",
			input: ListOutput{Identities: []Output{
				{ExternUID: "uid-1", UserID: 10, Active: true},
				{ExternUID: "uid-2", UserID: 20, Active: false},
				{ExternUID: "uid-3", UserID: 30, Active: true},
			}},
			want: "## SCIM Identities (3)\n\n" + header +
				"| `uid-1` | 10 | " + toolutil.BoolEmoji(true) + " |\n" +
				"| `uid-2` | 20 | " + toolutil.BoolEmoji(false) + " |\n" +
				"| `uid-3` | 30 | " + toolutil.BoolEmoji(true) + " |\n" +
				scimListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.input); got != tt.want {
				t.Errorf("FormatListMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestToOutput_Nil verifies the ToOutput_Nil handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestToOutput_Nil(t *testing.T) {
	out := toOutput(nil, toolutil.SCIMIdentityExtra{})
	if out.ExternUID != "" {
		t.Errorf("expected empty ExternUID, got %q", out.ExternUID)
	}
	if out.UserID != 0 {
		t.Errorf("expected UserID 0, got %d", out.UserID)
	}
	if out.Active {
		t.Error("expected Active false, got true")
	}
}

// TestFormatUpdateMarkdown_RendersConfirmation verifies the whole card the
// update confirmation renders, the flag as the emoji rather than as "true".
func TestFormatUpdateMarkdown_RendersConfirmation(t *testing.T) {
	got := FormatUpdateMarkdown(UpdateOutput{Updated: true, Message: "ok"})
	want := "## SCIM Identity Updated\n\n" +
		"- **Updated**: " + toolutil.BoolEmoji(true) + "\n" +
		"- **Message**: ok\n" +
		"\n---\n💡 **Next steps:**\n" +
		"- Use action 'group_scim.get' to verify the new external UID\n"
	if got != want {
		t.Errorf("FormatUpdateMarkdown =\n%q\nwant\n%q", got, want)
	}
}
