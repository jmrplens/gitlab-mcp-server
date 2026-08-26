// text_name_references_test.go guards the tool-name references embedded in
// the catalog's free-text surfaces. Usage sentences, descriptions, aliases,
// and parameter guidance all name other tools by their individual-surface
// names; a stale name there points a model at something that does not
// exist, and no schema check can see inside a prose string. This sweep can:
// every gitlab_* token in those fields must be a real individual tool, a
// real dispatcher, or an allowlisted GitLab API vocabulary word.
package tools

import (
	"regexp"
	"strings"
	"testing"
)

// textNameToken matches anything that looks like a tool reference inside
// free text.
var textNameToken = regexp.MustCompile(`\bgitlab_[a-z0-9_]+\b`)

// textNameAllowlist holds gitlab_-prefixed tokens that are GitLab API
// vocabulary, not tool references: template-type enum values in the
// project templates schemas.
var textNameAllowlist = map[string]bool{
	"gitlab_ci_ymls":        true,
	"gitlab_ci_syntax_ymls": true,
}

// TestCatalogTextFields_NameReferencesResolve verifies every tool-name
// token in the catalog's text fields resolves against the catalog itself.
// The sweep that introduced this guard found a ghost alias and two
// archive descriptions whose "(read-only)" parenthetical described the
// archived project, not the action.
func TestCatalogTextFields_NameReferencesResolve(t *testing.T) {
	catalog := mustBuildActionCatalog(t, nil, ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	valid := map[string]bool{
		"gitlab_find_action":    true,
		"gitlab_execute_action": true,
	}
	for _, spec := range StandaloneSurfaceToolSpecs(nil) {
		if action, err := spec.ActionSpec(); err == nil {
			valid[action.IndividualTool.Name] = true
		}
	}
	// Server-maintenance specs need an updater to materialize; their names
	// are stable, so they are pinned rather than constructed.
	valid["gitlab_server_check_update"] = true
	valid["gitlab_server_apply_update"] = true
	for _, action := range catalog.Actions() {
		if action.IndividualTool.Name != "" {
			valid[action.IndividualTool.Name] = true
		}
		if action.ToolName != "" {
			valid[action.ToolName] = true
		}
	}

	check := func(owner, field, text string) {
		t.Helper()
		for _, token := range textNameToken.FindAllString(text, -1) {
			if !valid[token] && !textNameAllowlist[token] {
				t.Errorf("%s (%s) references %q, which is not a tool or dispatcher in the catalog", owner, field, token)
			}
		}
	}
	for _, action := range catalog.Actions() {
		id := string(action.ID)
		check(id, "usage", action.Usage)
		check(id, "description", action.IndividualTool.Description)
		for _, alias := range action.Aliases {
			check(id, "alias", alias)
		}
		for param, guidance := range action.Route.ParameterGuidance {
			check(id, "guidance."+param, strings.Join(append([]string{guidance.ValueSource, guidance.ExampleBinding}, guidance.CommonConfusions...), " "))
		}
	}
}
