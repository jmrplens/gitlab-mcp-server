// markdown_test.go asserts the whole Markdown document each error tracking
// formatter writes: the settings card, the client key table, and the key card
// whose public key and DSN are the values a reader copies into a client's
// configuration.
package errortracking

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatSettingsMarkdown verifies the settings card, where both flags are
// always stated and the Sentry fields appear only when the integration is
// configured.
func TestFormatSettingsMarkdown(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.error_tracking_list' to see the client keys this project publishes\n"
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, FormatSettingsMarkdown(SettingsOutput{
			Active:            true,
			ProjectName:       "test",
			SentryExternalURL: "https://sentry.io",
			APIURL:            "https://sentry.io/api",
			Integrated:        false,
		}),
			"## Error Tracking Settings\n\n"+
				"- **Active**: ✅\n"+
				"- **Integrated**: ❌\n"+
				"- **Project Name**: test\n"+
				"- **Sentry URL**: https://sentry.io\n"+
				"- **API URL**: https://sentry.io/api\n"+
				hints)
	})
	t.Run("no Sentry integration", func(t *testing.T) {
		assertRendered(t, FormatSettingsMarkdown(SettingsOutput{Active: false, Integrated: true}),
			"## Error Tracking Settings\n\n"+
				"- **Active**: ❌\n"+
				"- **Integrated**: ✅\n"+
				hints)
	})
}

// TestFormatListKeysMarkdown verifies the client key table and the sentence an
// empty page renders instead of a heading counting zero.
func TestFormatListKeysMarkdown(t *testing.T) {
	t.Run("with keys", func(t *testing.T) {
		assertRendered(t, FormatListKeysMarkdown(ListClientKeysOutput{
			Keys: []ClientKeyItem{
				{ID: 1, Active: true, PublicKey: "pk", SentryDsn: "dsn"},
				{ID: 2, Active: false, PublicKey: "pk2", SentryDsn: "dsn2"},
			},
		}),
			"## Error Tracking Client Keys (2)\n\n"+
				"| ID | Active | Public Key |\n"+
				"| --- | --- | --- |\n"+
				"| 1 | ✅ | `pk` |\n"+
				"| 2 | ❌ | `pk2` |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'admin.error_tracking_create' to generate another key\n"+
				"- Use action 'admin.error_tracking_delete' to revoke one by its ID\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, FormatListKeysMarkdown(ListClientKeysOutput{Keys: []ClientKeyItem{}}), "No client keys found.\n")
	})
	t.Run("nil keys", func(t *testing.T) {
		assertRendered(t, FormatListKeysMarkdown(ListClientKeysOutput{}), "No client keys found.\n")
	})
}

// TestFormatKeyMarkdown verifies the client key card, with the key and the DSN
// as code spans because a reader has to copy both verbatim.
func TestFormatKeyMarkdown(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'admin.error_tracking_delete' to revoke this key\n" +
		"- Use action 'admin.error_tracking_get_settings' to check whether error tracking is enabled for the project\n"
	t.Run("active", func(t *testing.T) {
		assertRendered(t, FormatKeyMarkdown(ClientKeyItem{ID: 42, Active: true, PublicKey: "pk-123", SentryDsn: "https://dsn.example.com"}),
			"## Error Tracking Client Key #42\n\n"+
				"- **ID**: 42\n"+
				"- **Active**: ✅\n"+
				"- **Public Key**: `pk-123`\n"+
				"- **Sentry DSN**: `https://dsn.example.com`\n"+
				hints)
	})
	t.Run("inactive", func(t *testing.T) {
		assertRendered(t, FormatKeyMarkdown(ClientKeyItem{ID: 7, Active: false, PublicKey: "pk-xyz", SentryDsn: "dsn2"}),
			"## Error Tracking Client Key #7\n\n"+
				"- **ID**: 7\n"+
				"- **Active**: ❌\n"+
				"- **Public Key**: `pk-xyz`\n"+
				"- **Sentry DSN**: `dsn2`\n"+
				hints)
	})
}

// TestErrorTrackingFormattersAreRegistered verifies each output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime.
func TestErrorTrackingFormattersAreRegistered(t *testing.T) {
	settings := SettingsOutput{Active: true, ProjectName: "test"}
	keys := ListClientKeysOutput{Keys: []ClientKeyItem{{ID: 1, Active: true, PublicKey: "pk"}}}
	key := ClientKeyItem{ID: 1, Active: true, PublicKey: "pk", SentryDsn: "dsn"}
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "SettingsOutput", output: settings, rendered: FormatSettingsMarkdown(settings)},
		{name: "ListClientKeysOutput", output: keys, rendered: FormatListKeysMarkdown(keys)},
		{name: "ClientKeyItem", output: key, rendered: FormatKeyMarkdown(key)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := toolutil.MarkdownForResult(tc.output)
			if result == nil {
				t.Fatal("MarkdownForResult returned nil, want a registered formatter for the type")
			}
			if len(result.Content) != 1 {
				t.Fatalf("content blocks = %d, want 1", len(result.Content))
			}
			text, ok := result.Content[0].(*mcp.TextContent)
			if !ok {
				t.Fatalf("content block = %T, want *mcp.TextContent", result.Content[0])
			}
			assertRendered(t, text.Text, tc.rendered)
		})
	}
}
