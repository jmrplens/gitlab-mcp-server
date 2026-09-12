// markdown_test.go asserts the whole Markdown document each integration
// formatter writes: the project and group tables, the card one integration
// renders as, and the group Datadog card with its configuration as the nested
// object it is.
package integrations

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// jiraIntegration and slackIntegration are the two shapes an integration list
// returns: one active with timestamps, one inactive without.
var (
	jiraIntegration = IntegrationItem{
		ID: 1, Title: "Jira", Slug: "jira", Active: true,
		CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-02-01T00:00:00Z",
	}
	slackIntegration = IntegrationItem{ID: 2, Title: "Slack", Slug: "slack", Active: false}
)

// datadogProperties is the configuration GitLab returns for a fully set up
// group Datadog integration.
func datadogProperties() *GroupDatadogProperties {
	return &GroupDatadogProperties{
		APIURL:              "https://api.datadoghq.com",
		DatadogEnv:          "prod",
		DatadogService:      "gitlab",
		DatadogSite:         "datadoghq.com",
		DatadogTags:         "team:platform",
		DatadogCIVisibility: true,
		ArchiveTraceEvents:  true,
	}
}

// The rows and sections the shared writers produce, so an expectation names
// them once.
const (
	jiraCardRows = "- **ID**: 1\n" +
		"- **Slug**: `jira`\n" +
		"- **Active**: ✅\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Updated**: 1 Feb 2026 00:00 UTC\n"

	datadogConfigurationRows = "- **Datadog Configuration**:\n" +
		"  - **API URL**: https://api.datadoghq.com\n" +
		"  - **Datadog Env**: prod\n" +
		"  - **Datadog Service**: gitlab\n" +
		"  - **Datadog Site**: datadoghq.com\n" +
		"  - **Datadog Tags**: team:platform\n" +
		"  - **Datadog CI Visibility**: ✅\n" +
		"  - **Archive Trace Events**: ✅\n"
)

// assertRendered fails when the rendered document is not exactly want.
func assertRendered(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("rendered Markdown mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestFormatListMarkdownString verifies the project integration table and the
// empty sentence, which names the scope: "No integrations found." over a
// project read as an instance with no integrations at all.
func TestFormatListMarkdownString(t *testing.T) {
	t.Run("with integrations", func(t *testing.T) {
		assertRendered(t, formatListMarkdownString(ListOutput{
			Integrations: []IntegrationItem{jiraIntegration, slackIntegration},
		}),
			"## Project Integrations (2)\n\n"+
				"| ID | Title | Slug | Active |\n"+
				"| --- | --- | --- | --- |\n"+
				"| 1 | Jira | `jira` | ✅ |\n"+
				"| 2 | Slack | `slack` | ❌ |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'project.integration_get' to read one project integration by its slug\n"+
				"- Use action 'project.integration_set' to configure one\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, formatListMarkdownString(ListOutput{}), "No active project integrations found.\n")
	})
}

// TestFormatGroupIntegrationListString verifies the group integration table and
// its own scoped empty sentence.
func TestFormatGroupIntegrationListString(t *testing.T) {
	t.Run("with integrations", func(t *testing.T) {
		assertRendered(t, formatGroupIntegrationListString(ListGroupIntegrationsOutput{
			Integrations: []IntegrationItem{slackIntegration},
		}),
			"## Group Integrations (1)\n\n"+
				"| ID | Title | Slug | Active |\n"+
				"| --- | --- | --- | --- |\n"+
				"| 2 | Slack | `slack` | ❌ |\n"+
				"\n---\n💡 **Next steps:**\n"+
				"- Use action 'project.integration_get_group' to read one group integration by its slug\n"+
				"- Use action 'project.integration_set_group' to configure one\n")
	})
	t.Run("empty", func(t *testing.T) {
		assertRendered(t, formatGroupIntegrationListString(ListGroupIntegrationsOutput{}),
			"No active group integrations found.\n")
	})
}

// TestIntegrationCards verifies every card built from the shared integration
// writer: the heading each caller composes, the rows, and the hints, which say
// what a read returns rather than promising the configuration the get output
// does not carry.
func TestIntegrationCards(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "project get",
			got:  formatGetMarkdownString(GetOutput{Integration: jiraIntegration}),
			want: "## Integration: Jira\n\n" + jiraCardRows +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_set' to change this integration's configuration\n" +
				"- Use action 'project.integration_delete' to disable it\n" +
				"- " + hintWhatAReadReturns + "\n",
		},
		{
			name: "project set",
			got:  formatSetIntegrationMarkdownString(SetIntegrationOutput{Integration: jiraIntegration}),
			want: "## Integration Updated: Jira\n\n" + jiraCardRows +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_get' to read the integration back\n" +
				"- " + hintWhatAReadReturns + "\n",
		},
		{
			name: "project set falls back to the slug when GitLab sent no title",
			got:  formatSetIntegrationMarkdownString(SetIntegrationOutput{Integration: IntegrationItem{ID: 1, Slug: "harbor", Active: false}}),
			want: "## Integration Updated: harbor\n\n" +
				"- **ID**: 1\n" +
				"- **Slug**: `harbor`\n" +
				"- **Active**: ❌\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_get' to read the integration back\n" +
				"- " + hintWhatAReadReturns + "\n",
		},
		{
			name: "jira set",
			got:  formatSetJiraMarkdownString(SetJiraOutput{Integration: IntegrationItem{Slug: "jira", Active: true}}),
			want: "## Jira Integration Updated: jira\n\n" +
				"- **ID**: 0\n" +
				"- **Slug**: `jira`\n" +
				"- **Active**: ✅\n" +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_get' to read the integration back\n" +
				"- Jira credentials (username and password, or the API token) are write-only and are never returned\n",
		},
		{
			name: "group get",
			got:  formatGetGroupIntegrationString(GetGroupIntegrationOutput{Integration: jiraIntegration}),
			want: "## Group Integration: Jira\n\n" + jiraCardRows +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_set_group' to change this integration's configuration\n" +
				"- Use action 'project.integration_delete_group' to disable it\n" +
				"- " + hintWhatAReadReturns + "\n",
		},
		{
			name: "group set",
			got:  formatSetGroupIntegrationString(SetGroupIntegrationOutput{Integration: jiraIntegration}),
			want: "## Group Integration Updated: Jira\n\n" + jiraCardRows +
				"\n---\n💡 **Next steps:**\n" +
				"- Use action 'project.integration_get_group' to read the integration back\n" +
				"- " + hintWhatAReadReturns + "\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRendered(t, tc.got, tc.want)
		})
	}
}

// TestFormatGetGroupDatadogMarkdownString verifies the group Datadog read card,
// with the configuration as the nested object it is and the timestamps the read
// response carries.
func TestFormatGetGroupDatadogMarkdownString(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'project.integration_set_group_datadog' to update the Datadog configuration\n" +
		"- Use action 'project.integration_delete_group_datadog' to remove it from the group\n"
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, formatGetGroupDatadogMarkdownString(GetGroupDatadogOutput{
			Integration: GroupDatadogItem{
				ID:         1,
				Title:      "Datadog Production",
				Slug:       "datadog",
				Active:     true,
				CreatedAt:  "2026-01-02T03:04:05.000Z",
				UpdatedAt:  "2026-06-08T11:12:13.000Z",
				Properties: datadogProperties(),
			},
		}),
			"## Group Datadog Integration: Datadog Production\n\n"+
				"- **ID**: 1\n"+
				"- **Slug**: `datadog`\n"+
				"- **Active**: ✅\n"+
				datadogConfigurationRows+
				"- **Created**: 2 Jan 2026 03:04 UTC\n"+
				"- **Updated**: 8 Jun 2026 11:12 UTC\n"+
				hints)
	})
	t.Run("no title, no slug and no properties", func(t *testing.T) {
		assertRendered(t, formatGetGroupDatadogMarkdownString(GetGroupDatadogOutput{
			Integration: GroupDatadogItem{ID: 1, Active: true},
		}),
			"## Group Datadog Integration: datadog\n\n"+
				"- **ID**: 1\n"+
				"- **Slug**: `datadog`\n"+
				"- **Active**: ✅\n"+
				hints)
	})
}

// TestFormatSetGroupDatadogMarkdownString verifies the group Datadog upsert
// card, which carries no timestamps because the set response does not send
// them, and says that the API key is write-only.
func TestFormatSetGroupDatadogMarkdownString(t *testing.T) {
	hints := "\n---\n💡 **Next steps:**\n" +
		"- Use action 'project.integration_list_group' to see every integration configured on the group\n" +
		"- The api_key value is write-only and is never returned by the read endpoint; rotate the key in Datadog if you need to replace it on the group\n"
	t.Run("every field populated", func(t *testing.T) {
		assertRendered(t, formatSetGroupDatadogMarkdownString(SetGroupDatadogOutput{
			Integration: GroupDatadogItem{
				ID:         1,
				Title:      "Datadog Production",
				Slug:       "datadog",
				Active:     true,
				Properties: datadogProperties(),
			},
		}),
			"## Group Datadog Integration Updated: Datadog Production\n\n"+
				"- **ID**: 1\n"+
				"- **Slug**: `datadog`\n"+
				"- **Active**: ✅\n"+
				datadogConfigurationRows+
				hints)
	})
	t.Run("one property only", func(t *testing.T) {
		assertRendered(t, formatSetGroupDatadogMarkdownString(SetGroupDatadogOutput{
			Integration: GroupDatadogItem{ID: 1, Active: true, Properties: &GroupDatadogProperties{DatadogSite: "datadoghq.com"}},
		}),
			"## Group Datadog Integration Updated: datadog\n\n"+
				"- **ID**: 1\n"+
				"- **Slug**: `datadog`\n"+
				"- **Active**: ✅\n"+
				"- **Datadog Configuration**:\n"+
				"  - **Datadog Site**: datadoghq.com\n"+
				"  - **Datadog CI Visibility**: ❌\n"+
				"  - **Archive Trace Events**: ❌\n"+
				hints)
	})
}

// TestIntegrationFormattersAreRegistered verifies every output type resolves
// through the shared Markdown registry, which is how a tool result reaches its
// formatter at runtime. A formatter written but not registered renders as raw
// JSON, and one registered but unreachable is dead code the audit reported.
func TestIntegrationFormattersAreRegistered(t *testing.T) {
	list := ListOutput{Integrations: []IntegrationItem{jiraIntegration}}
	groupList := ListGroupIntegrationsOutput{Integrations: []IntegrationItem{jiraIntegration}}
	datadog := GroupDatadogItem{ID: 1, Active: true, Properties: datadogProperties()}
	cases := []struct {
		name     string
		output   any
		rendered string
	}{
		{name: "ListOutput", output: list, rendered: formatListMarkdownString(list)},
		{
			name:     "GetOutput",
			output:   GetOutput{Integration: jiraIntegration},
			rendered: formatGetMarkdownString(GetOutput{Integration: jiraIntegration}),
		},
		{
			name:     "SetIntegrationOutput",
			output:   SetIntegrationOutput{Integration: jiraIntegration},
			rendered: formatSetIntegrationMarkdownString(SetIntegrationOutput{Integration: jiraIntegration}),
		},
		{
			name:     "SetJiraOutput",
			output:   SetJiraOutput{Integration: jiraIntegration},
			rendered: formatSetJiraMarkdownString(SetJiraOutput{Integration: jiraIntegration}),
		},
		{name: "ListGroupIntegrationsOutput", output: groupList, rendered: formatGroupIntegrationListString(groupList)},
		{
			name:     "GetGroupIntegrationOutput",
			output:   GetGroupIntegrationOutput{Integration: jiraIntegration},
			rendered: formatGetGroupIntegrationString(GetGroupIntegrationOutput{Integration: jiraIntegration}),
		},
		{
			name:     "SetGroupIntegrationOutput",
			output:   SetGroupIntegrationOutput{Integration: jiraIntegration},
			rendered: formatSetGroupIntegrationString(SetGroupIntegrationOutput{Integration: jiraIntegration}),
		},
		{
			name:     "GetGroupDatadogOutput",
			output:   GetGroupDatadogOutput{Integration: datadog},
			rendered: formatGetGroupDatadogMarkdownString(GetGroupDatadogOutput{Integration: datadog}),
		},
		{
			name:     "SetGroupDatadogOutput",
			output:   SetGroupDatadogOutput{Integration: datadog},
			rendered: formatSetGroupDatadogMarkdownString(SetGroupDatadogOutput{Integration: datadog}),
		},
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
