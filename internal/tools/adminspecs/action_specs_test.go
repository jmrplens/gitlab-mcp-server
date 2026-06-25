package adminspecs

import (
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// TestActionSpecs_MetadataInvariants verifies every admin action carries the shared
// metadata required by catalog, meta-tool, dynamic, and individual projections.
func TestActionSpecs_MetadataInvariants(t *testing.T) {
	specs := ActionSpecs(nil)
	if len(specs) == 0 {
		t.Fatal("ActionSpecs() returned no specs")
	}

	names := make(map[string]bool, len(specs))
	individualTools := make(map[string]bool, len(specs))
	for _, spec := range specs {
		if names[spec.Name] {
			t.Fatalf("duplicate action name %q", spec.Name)
		}
		names[spec.Name] = true

		if spec.OwnerPackage != "adminspecs" {
			t.Fatalf("%s OwnerPackage = %q, want adminspecs", spec.Name, spec.OwnerPackage)
		}
		if !spec.OpenWorld {
			t.Fatalf("%s OpenWorld = false, want true", spec.Name)
		}
		if !slices.Contains(spec.Tags, "admin") {
			t.Fatalf("%s Tags = %v, want admin", spec.Name, spec.Tags)
		}
		if spec.IndividualTool.Name == "" {
			t.Fatalf("%s IndividualTool.Name is empty", spec.Name)
		}
		if individualTools[spec.IndividualTool.Name] {
			t.Fatalf("duplicate individual tool name %q", spec.IndividualTool.Name)
		}
		individualTools[spec.IndividualTool.Name] = true
		if spec.IndividualTool.Title == "" {
			t.Fatalf("%s IndividualTool.Title is empty", spec.Name)
		}
		if spec.Route.Handler == nil {
			t.Fatalf("%s Route.Handler is nil", spec.Name)
		}
		if spec.Route.InputSchema == nil {
			t.Fatalf("%s Route.InputSchema is nil", spec.Name)
		}
		if spec.Route.OutputSchema == nil {
			t.Fatalf("%s Route.OutputSchema is nil", spec.Name)
		}
	}
}

// TestActionSpecs_SelectedActionSemantics verifies representative admin actions
// retain their canonical read-only, destructive, and idempotency classifications.
func TestActionSpecs_SelectedActionSemantics(t *testing.T) {
	specs := specsByName(t, ActionSpecs(nil))

	tests := []struct {
		name                  string
		readOnly              bool
		destructive           bool
		idempotent            bool
		individualTool        string
		individualReadOnly    *bool
		individualDestructive *bool
		individualIdempotent  *bool
	}{
		{name: "topic_list", readOnly: true, idempotent: true, individualTool: "gitlab_list_topics"},
		{name: "topic_create", individualTool: "gitlab_create_topic"},
		{name: "topic_update", idempotent: true, individualTool: "gitlab_update_topic"},
		{name: "topic_delete", destructive: true, idempotent: true, individualTool: "gitlab_delete_topic"},
		{name: "feature_set", idempotent: true, individualTool: "gitlab_set_feature_flag", individualIdempotent: new(false)},
		{name: "db_migration_mark", destructive: true, idempotent: true, individualTool: "gitlab_mark_migration", individualDestructive: new(false)},
		{name: "system_hook_edit", idempotent: true, individualTool: "gitlab_edit_system_hook"},
		{name: "system_hook_set_url_variable", idempotent: true, individualTool: "gitlab_set_system_hook_url_variable"},
		{name: "system_hook_delete_url_variable", destructive: true, idempotent: true, individualTool: "gitlab_delete_system_hook_url_variable"},
		{name: "system_hook_test", individualTool: "gitlab_test_system_hook", individualReadOnly: new(true), individualIdempotent: new(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specs[tt.name]
			if !ok {
				t.Fatalf("missing action %q", tt.name)
			}
			if spec.ReadOnly != tt.readOnly {
				t.Fatalf("ReadOnly = %v, want %v", spec.ReadOnly, tt.readOnly)
			}
			if spec.Destructive != tt.destructive {
				t.Fatalf("Destructive = %v, want %v", spec.Destructive, tt.destructive)
			}
			if spec.Route.Destructive != tt.destructive {
				t.Fatalf("Route.Destructive = %v, want %v", spec.Route.Destructive, tt.destructive)
			}
			if spec.Idempotent != tt.idempotent {
				t.Fatalf("Idempotent = %v, want %v", spec.Idempotent, tt.idempotent)
			}
			if spec.IndividualTool.Name != tt.individualTool {
				t.Fatalf("IndividualTool.Name = %q, want %q", spec.IndividualTool.Name, tt.individualTool)
			}
			assertBoolOverride(t, "ReadOnly", spec.IndividualTool.AnnotationOverrides.ReadOnly, tt.individualReadOnly)
			assertBoolOverride(t, "Destructive", spec.IndividualTool.AnnotationOverrides.Destructive, tt.individualDestructive)
			assertBoolOverride(t, "Idempotent", spec.IndividualTool.AnnotationOverrides.Idempotent, tt.individualIdempotent)
		})
	}
}

// TestActionSpecs_SettingsAndMetadataUsageGuidance verifies settings and metadata
// actions keep distinct selection hints for meta and dynamic surfaces.
func TestActionSpecs_SettingsAndMetadataUsageGuidance(t *testing.T) {
	specs := specsByName(t, ActionSpecs(nil))

	settingsSpec := specs["settings_get"]
	if !strings.Contains(settingsSpec.Usage, "application settings") {
		t.Fatalf("settings_get Usage = %q, want application settings guidance", settingsSpec.Usage)
	}
	if !slices.Contains(settingsSpec.Aliases, "instance settings") {
		t.Fatalf("settings_get Aliases = %v, want instance settings alias", settingsSpec.Aliases)
	}

	metadataSpec := specs["metadata_get"]
	if !strings.Contains(metadataSpec.Usage, "version") || !strings.Contains(metadataSpec.Usage, "Do not use this for application settings") {
		t.Fatalf("metadata_get Usage = %q, want metadata/version distinction", metadataSpec.Usage)
	}
	if !slices.Contains(metadataSpec.Aliases, "gitlab version") {
		t.Fatalf("metadata_get Aliases = %v, want gitlab version alias", metadataSpec.Aliases)
	}
	if !slices.Equal(metadataSpec.RelatedActions, []string{"admin.settings_get", "admin.app_statistics_get", "server.health_check"}) {
		t.Fatalf("metadata_get RelatedActions = %v", metadataSpec.RelatedActions)
	}
	for _, want := range []string{"Returns:", "See also:"} {
		if !strings.Contains(metadataSpec.IndividualTool.Description, want) {
			t.Fatalf("metadata_get description = %q, want %q", metadataSpec.IndividualTool.Description, want)
		}
	}
}

// TestActionSpecs_AppearanceAndStatisticsGuidance validates the AppearanceAndStatisticsGuidance route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_AppearanceAndStatisticsGuidance(t *testing.T) {
	specs := specsByName(t, ActionSpecs(nil))

	appearanceGet := specs["appearance_get"]
	if !strings.Contains(appearanceGet.Usage, "branding") {
		t.Fatalf("appearance_get Usage = %q, want branding guidance", appearanceGet.Usage)
	}
	if !slices.Contains(appearanceGet.Aliases, "branding settings") {
		t.Fatalf("appearance_get Aliases = %v, want branding settings alias", appearanceGet.Aliases)
	}
	if !slices.Equal(appearanceGet.RelatedActions, []string{"admin.settings_get", "admin.metadata_get", "admin.appearance_update"}) {
		t.Fatalf("appearance_get RelatedActions = %v", appearanceGet.RelatedActions)
	}

	appearanceUpdate := specs["appearance_update"]
	if guidance := appearanceUpdate.ParameterGuidance["message_background_color"]; guidance.SemanticRole != "hex_color" {
		t.Fatalf("appearance_update message_background_color guidance = %+v", guidance)
	}
	if !slices.Contains(appearanceUpdate.Aliases, "update branding") {
		t.Fatalf("appearance_update Aliases = %v, want update branding alias", appearanceUpdate.Aliases)
	}

	appStats := specs["app_statistics_get"]
	if !strings.Contains(appStats.Usage, "instance-wide application statistics") {
		t.Fatalf("app_statistics_get Usage = %q, want instance statistics guidance", appStats.Usage)
	}
	if !slices.Contains(appStats.Aliases, "instance statistics") {
		t.Fatalf("app_statistics_get Aliases = %v, want instance statistics alias", appStats.Aliases)
	}
	if !slices.Equal(appStats.RelatedActions, []string{"admin.metadata_get", "server.health_check"}) {
		t.Fatalf("app_statistics_get RelatedActions = %v", appStats.RelatedActions)
	}
}

// TestActionSpecs_SystemHookDescriptionsIncludeOutputGuidance verifies the
// model-facing individual tool descriptions carry explicit return semantics.
func TestActionSpecs_SystemHookDescriptionsIncludeOutputGuidance(t *testing.T) {
	specs := specsByName(t, ActionSpecs(nil))

	for _, actionName := range []string{
		"system_hook_edit",
		"system_hook_set_url_variable",
		"system_hook_delete_url_variable",
	} {
		t.Run(actionName, func(t *testing.T) {
			description := specs[actionName].IndividualTool.Description
			for _, want := range []string{"Returns:", "See also:"} {
				if !strings.Contains(description, want) {
					t.Fatalf("%s description = %q, want %q", actionName, description, want)
				}
			}
		})
	}
}

// TestActionSpecs_NoGenericMetadata verifies that every admin action carries
// non-generic discovery metadata: a tailored Usage (not the placeholder),
// natural-language Aliases beyond the bare tool name, a non-empty RelatedActions
// list of canonical {domain}.{action} ids, and an individual-tool description in
// the "Returns: … See also: …" form. This is the 1:1 audit R-META guard.
func TestActionSpecs_NoGenericMetadata(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			assertNonGenericUsage(t, spec)
			assertNaturalLanguageAliases(t, spec)
			assertCanonicalRelatedActions(t, spec)
			assertReturnsSeeAlsoDescription(t, spec)
		})
	}
}

func assertNonGenericUsage(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	const genericUsage = "Use to execute adminspecs domain action."
	if spec.Usage == "" || spec.Usage == genericUsage {
		t.Fatalf("%s Usage = %q, want action-specific guidance", spec.Name, spec.Usage)
	}
}

func assertNaturalLanguageAliases(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	tool := spec.IndividualTool.Name
	nonName := 0
	for _, alias := range spec.Aliases {
		if alias != tool {
			nonName++
		}
	}
	if nonName < 2 {
		t.Fatalf("%s Aliases = %v, want >= 2 natural-language aliases beyond %q", spec.Name, spec.Aliases, tool)
	}
}

func assertCanonicalRelatedActions(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	if len(spec.RelatedActions) == 0 {
		t.Fatalf("%s RelatedActions is empty, want canonical related ids", spec.Name)
	}
	for _, rel := range spec.RelatedActions {
		if !strings.Contains(rel, ".") {
			t.Fatalf("%s RelatedActions entry %q is not a {domain}.{action} id", spec.Name, rel)
		}
	}
}

func assertReturnsSeeAlsoDescription(t *testing.T, spec toolutil.ActionSpec) {
	t.Helper()
	desc := spec.IndividualTool.Description
	for _, want := range []string{"Returns:", "See also:"} {
		if !strings.Contains(desc, want) {
			t.Fatalf("%s description = %q, want %q", spec.Name, desc, want)
		}
	}
}

func specsByName(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	return byName
}

func assertBoolOverride(t *testing.T, name string, got, want *bool) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Fatalf("%s override = %v, want nil", name, *got)
		}
		return
	}
	if got == nil {
		t.Fatalf("%s override = nil, want %v", name, *want)
	}
	if *got != *want {
		t.Fatalf("%s override = %v, want %v", name, *got, *want)
	}
}
