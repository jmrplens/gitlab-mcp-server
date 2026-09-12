package settings

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Canonical action IDs the hints name, the one form every surface resolves.
const (
	actionGet    = "admin.settings_get"
	actionUpdate = "admin.settings_update"
)

// emptyListValue is what a setting whose value is an empty JSON array shows.
// An empty array is an answer ("nothing is restricted"), so the row is written
// rather than dropped, and it is written in words rather than as "[]".
const emptyListValue = "none"

// settingCategory is one titled group of application-setting keys, rendered as
// a section of the card.
type settingCategory struct {
	title string
	keys  []string
}

// settingCategories is the curated subset of GitLab's application settings the
// card renders, in the order it renders them.
//
// GitLab answers with several hundred keys and the card names 27 of them; the
// rest reach the caller in the structured result, which the note under the card
// says. It used to claim a total over every key GitLab sent while showing only
// these, so the count and the content disagreed by two orders of magnitude.
var settingCategories = []settingCategory{
	{"General", []string{
		"signup_enabled", "sign_in_text", "after_sign_out_path",
		"default_project_visibility", "default_group_visibility",
		"default_snippet_visibility", "restricted_visibility_levels",
		"can_create_group", "user_default_external",
	}},
	{"CI/CD", []string{
		"auto_devops_enabled", "auto_devops_domain",
		"shared_runners_enabled", "max_artifacts_size",
		"default_ci_config_path", "ci_max_includes",
	}},
	{"Authentication", []string{
		"password_authentication_enabled_for_web",
		"password_authentication_enabled_for_git",
		"two_factor_grace_period", "require_two_factor_authentication",
	}},
	{"Repository", []string{
		"default_branch_name", "default_branch_protection",
		"max_attachment_size", "max_import_size",
	}},
	{"Rate Limits", []string{
		"throttle_authenticated_api_enabled",
		"throttle_unauthenticated_api_enabled",
		"throttle_authenticated_api_requests_per_period",
		"throttle_unauthenticated_api_requests_per_period",
	}},
}

// settingRow is one application setting ready for a card row: the key GitLab
// names it by, the value rendered for its JSON kind, and whether that rendering
// is a document the reader copies rather than prose.
type settingRow struct {
	key   string
	value string
	code  bool
}

// FormatGetMarkdown renders the application settings as a card.
func FormatGetMarkdown(out GetOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatSettingsCard("Application Settings", out.Settings,
		toolutil.HintAction(actionUpdate, "change one of these settings")))
}

// FormatUpdateMarkdown renders the settings an update returned as the same
// card: an update answers with the object, so it is a card like the read.
func FormatUpdateMarkdown(out UpdateOutput) *mcp.CallToolResult {
	return toolutil.ToolResultWithMarkdown(formatSettingsCard("Application Settings Updated", out.Settings,
		toolutil.HintAction(actionGet, "read the settings back")))
}

// formatSettingsCard renders one settings object as a card: a section per
// category that has anything to show, one row per key, then the note saying how
// much of the object the card covers.
//
// A category whose keys GitLab did not send opens no section, since a heading
// with nothing under it says the instance has no such settings.
func formatSettingsCard(title string, values map[string]any, hints ...string) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, title)
	shown := 0
	for _, category := range settingCategories {
		rows := settingRows(category.keys, values)
		if len(rows) == 0 {
			continue
		}
		section := c.Section(category.title)
		for _, row := range rows {
			if row.code {
				section.Code(row.key, row.value)
				continue
			}
			section.Field(row.key, row.value)
		}
		shown += len(rows)
	}
	c.Note(settingsCoverage(shown, len(values)))
	c.End(hints...)
	return b.String()
}

// settingRows renders the keys of one category that the settings object holds,
// dropping the ones GitLab sent nothing for.
func settingRows(keys []string, values map[string]any) []settingRow {
	rows := make([]settingRow, 0, len(keys))
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		row, ok := renderSetting(key, value)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// renderSetting renders one setting by the JSON kind of its value, and reports
// whether GitLab said anything at all.
//
// The value is an administrator-set `any` out of GitLab's application settings,
// and it used to be written into a table cell through "%v" with no escaping at
// all: a value holding a pipe split the row, and an array or an object rendered
// as Go's own container syntax, which is neither JSON a reader can copy nor a
// list they can read.
func renderSetting(key string, value any) (settingRow, bool) {
	switch v := value.(type) {
	case nil:
		return settingRow{}, false
	case bool:
		return settingRow{key: key, value: toolutil.BoolEmoji(v)}, true
	case string:
		if strings.TrimSpace(v) == "" {
			return settingRow{}, false
		}
		return settingRow{key: key, value: v}, true
	case []any:
		if len(v) == 0 {
			return settingRow{key: key, value: emptyListValue}, true
		}
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, settingScalar(item))
		}
		return settingRow{key: key, value: strings.Join(parts, ", ")}, true
	case map[string]any:
		return settingRow{key: key, value: compactJSON(value), code: true}, true
	default:
		return settingRow{key: key, value: settingScalar(value)}, true
	}
}

// settingScalar renders one JSON scalar as the text a reader sees, and anything
// else as the JSON document it arrived as.
func settingScalar(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return toolutil.BoolEmoji(v)
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return compactJSON(value)
	}
}

// compactJSON renders a nested value as the JSON it arrived as. A value
// encoding/json refuses falls back to its Go form, which nothing decoded from a
// JSON response reaches.
func compactJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(encoded)
}

// settingsCoverage is the sentence under the card, saying how many of the keys
// GitLab sent the card actually shows.
func settingsCoverage(shown, total int) string {
	if total == 0 {
		return "GitLab returned no settings."
	}
	if shown >= total {
		return fmt.Sprintf("Showing all %d settings GitLab returned.", total)
	}
	return fmt.Sprintf("Showing %d of %d settings; the remaining keys are in the structured result.", shown, total)
}

func init() {
	toolutil.RegisterMarkdownResult(FormatGetMarkdown)
	toolutil.RegisterMarkdownResult(FormatUpdateMarkdown)
}
