package adminspecs

import (
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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

		if spec.OwnerPackage == "" {
			t.Fatalf("%s OwnerPackage is empty", spec.Name)
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

// TestActionSpecs_OwnerPackage_IsThePackageTheHandlerComesFrom verifies that
// every admin action names the domain package whose handler it routes to,
// rather than this one, which declares the specs and issues no request of its
// own.
//
// The owner is a join key rather than a label: the request inventory records a
// package and nothing else, so an action owned by a package that issues nothing
// can be joined to no recording at all, and one owned by the wrong package is
// quietly credited with somebody else's requests. That second error is the one
// R-PATH cannot catch by itself, which is why it is caught here.
//
// The oracle is the route rather than a second table, so this asserts a fact
// about the tree instead of restating the constants it checks: every admin
// handler takes the input type its own package declares, so the package path of
// Route.InputType names the package that will issue the request. A
// copy-pasted owner constant therefore fails here.
func TestActionSpecs_OwnerPackage_IsThePackageTheHandlerComesFrom(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			if spec.Route.InputType == nil {
				t.Fatalf("%s carries no input type to read an owner from", spec.Name)
			}
			pkgPath := spec.Route.InputType.PkgPath()
			want := path.Base(pkgPath)
			if spec.OwnerPackage != want {
				t.Errorf("OwnerPackage = %q, want %q: the handler's input type is %s", spec.OwnerPackage, want, pkgPath)
			}
		})
	}
}

// TestActionSpecs_MarkMigration_IsProjectedAsNonDestructive verifies the one
// place this group deliberately disagrees with itself: marking a migration is
// registered through NewDeleteActionSpec, so the action stays destructive for
// the confirmation prompt and the read-only filter, while the tool an
// individual-surface client is listed declares destructiveHint false.
//
// Both halves are asserted together because either one alone is satisfied by
// deleting the override: with it gone the annotation simply follows the
// catalog, and every other test here still passes. The assertion reads the
// projected tool rather than the override field, because what a client is told
// is the annotation, and the projection may narrow an override on its way out.
func TestActionSpecs_MarkMigration_IsProjectedAsNonDestructive(t *testing.T) {
	spec := specsByName(t, ActionSpecs(nil))["db_migration_mark"]
	if !spec.Destructive {
		t.Fatal("db_migration_mark should stay destructive in the catalog")
	}

	tool, err := toolutil.IndividualToolFromActionSpec(spec, toolutil.IndividualToolProjectionOptions{
		Description: "Mark a pending database migration as successfully executed.",
	})
	if err != nil {
		t.Fatalf("IndividualToolFromActionSpec() error = %v", err)
	}
	if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
		t.Fatalf("individual tool declares no destructive hint: %+v", tool.Annotations)
	}
	if *tool.Annotations.DestructiveHint {
		t.Error("destructiveHint = true, want false: the individual projection overrides the catalog classification")
	}
}

// TestActionSpecs_Tags_CarryTheDomainTagsOfTheirOwnerPackage verifies every
// admin action is tagged with the domain tags its owner package contributes,
// and that adminOwnerTags describes the tree in both directions.
//
// Those tags were the one part of the discovery metadata this group had no copy
// of. Each of the twenty-three owner packages used to declare a full set of
// specs that nothing aggregated, tagged for its own domain, while the served
// spec carried "admin" and nothing else: a model searching for "sidekiq" or
// "secure file" matched a spec no surface would ever register. The table is
// checked from both ends so an owner that loses its last action fails here
// rather than leaving behind an entry nothing reads.
func TestActionSpecs_Tags_CarryTheDomainTagsOfTheirOwnerPackage(t *testing.T) {
	specs := ActionSpecs(nil)
	owners := make(map[string]bool, len(adminOwnerTags))
	for _, spec := range specs {
		owners[spec.OwnerPackage] = true
		t.Run(spec.Name, func(t *testing.T) {
			for _, tag := range adminOwnerTags[spec.OwnerPackage] {
				if !slices.Contains(spec.Tags, tag) {
					t.Errorf("Tags = %v, want the %s tag %q", spec.Tags, spec.OwnerPackage, tag)
				}
			}
			seen := make(map[string]bool, len(spec.Tags))
			for _, tag := range spec.Tags {
				if seen[tag] {
					t.Errorf("Tags = %v, want %q once", spec.Tags, tag)
				}
				seen[tag] = true
			}
		})
	}

	for owner := range adminOwnerTags {
		if !owners[owner] {
			t.Errorf("adminOwnerTags names %q, which owns no admin action", owner)
		}
	}
	for owner := range owners {
		if _, ok := adminOwnerTags[owner]; !ok {
			t.Errorf("owner %q contributes no domain tag; every owner of an admin action belongs in adminOwnerTags", owner)
		}
	}
}

// TestActionSpecs_Usage_NamesEachActionsOwnOperation verifies that no two
// admin actions publish the same usage line.
//
// The usage is what a model reads to choose between siblings, and a pair that
// shares one sentence tells it nothing: reading the plan limits and writing
// them were described identically until each got a sentence of its own, and
// every surface still registered, routed and answered while the text was
// wrong. That is a discovery defect rather than a runtime one, which is the
// worse half to leave.
func TestActionSpecs_Usage_NamesEachActionsOwnOperation(t *testing.T) {
	byUsage := map[string]string{}
	for _, spec := range ActionSpecs(nil) {
		if previous, shared := byUsage[spec.Usage]; shared {
			t.Errorf("%s and %s publish the same usage %q", previous, spec.Name, spec.Usage)
			continue
		}
		byUsage[spec.Usage] = spec.Name
	}
}

// TestActionSpecs_ParameterGuidance_NamesOnlyParametersTheActionDeclares
// verifies that no admin action carries guidance for a parameter its own input
// schema has no property for.
//
// The projection refuses such a spec outright, so this asserts the same rule
// where a maintainer edits rather than where the catalog is assembled. It is
// also the rule the deleted per-package copies of these specs broke without
// anybody noticing: one guidance map was written per package and applied to
// every action in it, so a list action advertised guidance for the identifier
// only its get, update and delete siblings take. Nothing validated them,
// because nothing built them.
func TestActionSpecs_ParameterGuidance_NamesOnlyParametersTheActionDeclares(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		if len(spec.ParameterGuidance) == 0 {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			for parameter := range spec.ParameterGuidance {
				if _, ok := properties[parameter]; !ok {
					t.Errorf("guidance names parameter %q, which %s does not declare", parameter, spec.Name)
				}
			}
		})
	}
}

// TestApplyAdminMeta_Guidance_IsAddedBesideWhatTheBuilderWrote verifies that an
// entry's guidance is merged into whatever guidance the options already carry
// rather than replacing the map, and that an entry stating none leaves it
// alone. Both halves matter: the dedicated builders that write their own
// guidance run after this, and an entry that silently emptied the map would
// still produce a valid spec.
func TestApplyAdminMeta_Guidance_IsAddedBesideWhatTheBuilderWrote(t *testing.T) {
	built := toolutil.ParameterGuidance{SemanticRole: "built_role"}
	fromEntry := toolutil.ParameterGuidance{SemanticRole: "entry_role"}

	tests := []struct {
		name string
		meta adminActionMetaEntry
		want map[string]toolutil.ParameterGuidance
	}{
		{
			name: "entry states guidance",
			meta: adminActionMetaEntry{guidance: map[string]toolutil.ParameterGuidance{"entry_param": fromEntry}},
			want: map[string]toolutil.ParameterGuidance{"built_param": built, "entry_param": fromEntry},
		},
		{
			name: "entry states none",
			meta: adminActionMetaEntry{usage: "Usage stated by the table entry."},
			want: map[string]toolutil.ParameterGuidance{"built_param": built},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := toolutil.ActionSpecOptions{
				ParameterGuidance: map[string]toolutil.ParameterGuidance{"built_param": built},
			}
			applyAdminMeta(&options, tt.meta)
			if len(options.ParameterGuidance) != len(tt.want) {
				t.Fatalf("ParameterGuidance = %v, want %v", options.ParameterGuidance, tt.want)
			}
			for parameter, want := range tt.want {
				if got := options.ParameterGuidance[parameter]; got.SemanticRole != want.SemanticRole {
					t.Errorf("ParameterGuidance[%q].SemanticRole = %q, want %q", parameter, got.SemanticRole, want.SemanticRole)
				}
			}
		})
	}
}

// TestApplyAdminMeta_Guidance_OnOptionsThatCarryNone_StartsAMapOfItsOwn
// verifies the nil case: an action whose builder wrote no guidance gains
// exactly the entry's, in a map the table does not share. Writing into the
// table's own map would leak one action's later edits into every sibling that
// names the same shared guidance value.
func TestApplyAdminMeta_Guidance_OnOptionsThatCarryNone_StartsAMapOfItsOwn(t *testing.T) {
	entry := adminActionMetaEntry{guidance: map[string]toolutil.ParameterGuidance{"entry_param": {SemanticRole: "entry_role"}}}
	var options toolutil.ActionSpecOptions
	applyAdminMeta(&options, entry)

	if got := options.ParameterGuidance["entry_param"].SemanticRole; got != "entry_role" {
		t.Fatalf("ParameterGuidance[\"entry_param\"].SemanticRole = %q, want %q", got, "entry_role")
	}
	options.ParameterGuidance["added_after"] = toolutil.ParameterGuidance{SemanticRole: "added_role"}
	if _, ok := entry.guidance["added_after"]; ok {
		t.Error("writing to the options guidance reached the entry's own map")
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
		// No annotation override: the base classification is a create, and a
		// read-only/idempotent override would widen it, which
		// IndividualToolAnnotationOverrides.NarrowingOnly discards. Firing a
		// test event POSTs a sample payload to the hook URL, so the create
		// classification is the honest one and the override was never served.
		{name: "system_hook_test", individualTool: "gitlab_test_system_hook"},
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
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(metadataSpec.IndividualTool.Description, want) {
				t.Fatalf("metadata_get description = %q, want %q", metadataSpec.IndividualTool.Description, want)
			}
		})
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

// TestActionSpecs_InputSchemaOverrides_ConstrainTheServedInputSchema verifies
// that every admin parameter GitLab restricts to a closed set is served with
// that set, and that the two parameters the package deliberately leaves
// free-form are served with the corrected description instead.
//
// The expected values are spelled out here rather than read back from
// adminInputSchemaOverrides, so the assertion is about what a model is shown
// and not about the table agreeing with itself. It matters because the schema
// is the only place a model learns that a value is not free-form: client-go
// types all of these as plain strings and several struct tags describe the
// vocabulary in prose ("Type: banner or notification") without constraining
// it, so an override that stopped being applied would compile, pass every
// other test in this file, and quietly widen the surface back to any string
// the instance then refuses.
func TestActionSpecs_InputSchemaOverrides_ConstrainTheServedInputSchema(t *testing.T) {
	specs := specsByName(t, ActionSpecs(nil))

	broadcastThemes := []string{"indigo", "light-indigo", "blue", "light-blue", "green", "light-green", "red", "light-red", "dark", "light"}
	bulkImportStatuses := []string{"created", "started", "finished", "timeout", "failed", "canceled"}
	customAttributeTypes := []string{"user", "group", "project"}

	tests := []struct {
		action      string
		path        string
		wantEnum    []string
		description string
	}{
		{action: "broadcast_message_create", path: "broadcast_type", wantEnum: []string{"banner", "notification"}},
		{action: "broadcast_message_create", path: "theme", wantEnum: broadcastThemes},
		{action: "broadcast_message_update", path: "broadcast_type", wantEnum: []string{"banner", "notification"}},
		{action: "broadcast_message_update", path: "theme", wantEnum: broadcastThemes},
		{action: "bulk_import_list", path: "status", wantEnum: bulkImportStatuses},
		{action: "bulk_import_entity_list", path: "status", wantEnum: bulkImportStatuses},
		{action: "bulk_import_start", path: "entities.source_type", wantEnum: []string{"group_entity", "project_entity"}},
		{action: "custom_attr_list", path: "resource_type", wantEnum: customAttributeTypes},
		{action: "custom_attr_get", path: "resource_type", wantEnum: customAttributeTypes},
		{action: "custom_attr_set", path: "resource_type", wantEnum: customAttributeTypes},
		{action: "custom_attr_delete", path: "resource_type", wantEnum: customAttributeTypes},
		{action: "feature_set", path: "key", wantEnum: []string{"percentage_of_actors", "percentage_of_time"}},
		{action: "import_github", path: "timeout_strategy", wantEnum: []string{"optimistic", "pessimistic"}},
		{action: "import_bitbucket_server", path: "timeout_strategy", wantEnum: []string{"optimistic", "pessimistic"}},
		// No enum on purpose: the database names and project paths below are
		// instance data, not a fixed vocabulary, so the override rewrites the
		// description the struct tag read a value list into and stops there.
		{action: "db_migration_mark", path: "database", description: "Database the migration belongs to. Defaults to main."},
		{action: "feature_set", path: "project", description: "Project path such as gitlab-org/gitlab-foss. Separate several paths with commas."},
		{action: "terraform_state_unlock", path: "name", description: "Terraform state name. Use params.name for values such as production or eval-unlock-123. Do not use id."},
	}

	for _, tt := range tests {
		t.Run(tt.action+"."+tt.path, func(t *testing.T) {
			spec, ok := specs[tt.action]
			if !ok {
				t.Fatalf("missing action %q", tt.action)
			}
			property := schemaPropertyAt(t, spec.Route.InputSchema, tt.path)
			if got := schemaEnumValues(t, property); !slices.Equal(got, tt.wantEnum) {
				t.Fatalf("%s %s enum = %v, want %v", tt.action, tt.path, got, tt.wantEnum)
			}
			if tt.description == "" {
				return
			}
			if got, _ := property["description"].(string); got != tt.description {
				t.Fatalf("%s %s description = %q, want %q", tt.action, tt.path, got, tt.description)
			}
		})
	}
}

// TestActionSpecs_InputSchemaOverrides_CoverExactlyTheDocumentedActions verifies
// that the admin actions carrying an input-schema override are exactly the ones
// the test above states the served schema for. An override added for a new
// action, or lost by one that had it, is invisible to every other assertion in
// this file, so without this the vocabularies would be checked only where
// somebody remembered to check them.
func TestActionSpecs_InputSchemaOverrides_CoverExactlyTheDocumentedActions(t *testing.T) {
	var overridden []string
	for _, spec := range ActionSpecs(nil) {
		if len(spec.InputSchemaOverrides) > 0 {
			overridden = append(overridden, spec.Name)
		}
	}
	slices.Sort(overridden)

	want := []string{
		"broadcast_message_create",
		"broadcast_message_update",
		"bulk_import_entity_list",
		"bulk_import_list",
		"bulk_import_start",
		"custom_attr_delete",
		"custom_attr_get",
		"custom_attr_list",
		"custom_attr_set",
		"db_migration_mark",
		"feature_set",
		"import_bitbucket_server",
		"import_github",
		"terraform_state_unlock",
	}
	if !slices.Equal(overridden, want) {
		t.Fatalf("actions with input-schema overrides = %v, want %v", overridden, want)
	}
}

// TestApplyAdminMeta_EntryOmittingAField_KeepsWhatTheBuilderSet verifies that an
// entry stating only part of an action's discovery metadata replaces that part
// and leaves the rest exactly as the dedicated builder wrote it.
//
// Three entries are already partial in this way: the system-hook ones carry
// usage, aliases and related actions while adminSystemHookEditSpec and its two
// siblings write the description by hand, and overwriting it with an empty
// string would serve a model a nameless tool. No entry omits an alias or a
// related list today, so nothing projected from ActionSpecs would notice either
// of those guards becoming an unconditional assignment; the contract is
// therefore stated against the function rather than discovered through the
// specs, where it currently holds by the shape of the data alone.
func TestApplyAdminMeta_EntryOmittingAField_KeepsWhatTheBuilderSet(t *testing.T) {
	const (
		builtUsage       = "Usage written by the dedicated builder."
		builtDescription = "Description written by the dedicated builder. Returns: nothing. See also: nothing."
		entryUsage       = "Usage stated by the table entry."
		entryDescription = "Description stated by the table entry. Returns: a thing. See also: another thing."
	)
	builtAliases := []string{"gitlab_builder_tool"}
	builtRelated := []string{"admin.builder_related"}
	entryAliases := []string{"entry alias one", "entry alias two"}
	entryRelated := []string{"admin.entry_related"}

	tests := []struct {
		name            string
		meta            adminActionMetaEntry
		wantUsage       string
		wantAliases     []string
		wantRelated     []string
		wantDescription string
	}{
		{
			name:            "every field stated",
			meta:            adminActionMetaEntry{usage: entryUsage, aliases: entryAliases, related: entryRelated, description: entryDescription},
			wantUsage:       entryUsage,
			wantAliases:     entryAliases,
			wantRelated:     entryRelated,
			wantDescription: entryDescription,
		},
		{
			name:            "usage omitted",
			meta:            adminActionMetaEntry{aliases: entryAliases, related: entryRelated, description: entryDescription},
			wantUsage:       builtUsage,
			wantAliases:     entryAliases,
			wantRelated:     entryRelated,
			wantDescription: entryDescription,
		},
		{
			name:            "aliases omitted",
			meta:            adminActionMetaEntry{usage: entryUsage, related: entryRelated, description: entryDescription},
			wantUsage:       entryUsage,
			wantAliases:     builtAliases,
			wantRelated:     entryRelated,
			wantDescription: entryDescription,
		},
		{
			name:            "related actions omitted",
			meta:            adminActionMetaEntry{usage: entryUsage, aliases: entryAliases, description: entryDescription},
			wantUsage:       entryUsage,
			wantAliases:     entryAliases,
			wantRelated:     builtRelated,
			wantDescription: entryDescription,
		},
		{
			// The shape the three system-hook entries are in.
			name:            "description omitted",
			meta:            adminActionMetaEntry{usage: entryUsage, aliases: entryAliases, related: entryRelated},
			wantUsage:       entryUsage,
			wantAliases:     entryAliases,
			wantRelated:     entryRelated,
			wantDescription: builtDescription,
		},
		{
			name:            "nothing stated",
			meta:            adminActionMetaEntry{},
			wantUsage:       builtUsage,
			wantAliases:     builtAliases,
			wantRelated:     builtRelated,
			wantDescription: builtDescription,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := toolutil.ActionSpecOptions{
				Usage:          builtUsage,
				Aliases:        slices.Clone(builtAliases),
				RelatedActions: slices.Clone(builtRelated),
				IndividualTool: toolutil.IndividualToolSpec{Description: builtDescription},
			}

			applyAdminMeta(&options, tt.meta)

			if options.Usage != tt.wantUsage {
				t.Fatalf("Usage = %q, want %q", options.Usage, tt.wantUsage)
			}
			if !slices.Equal(options.Aliases, tt.wantAliases) {
				t.Fatalf("Aliases = %v, want %v", options.Aliases, tt.wantAliases)
			}
			if !slices.Equal(options.RelatedActions, tt.wantRelated) {
				t.Fatalf("RelatedActions = %v, want %v", options.RelatedActions, tt.wantRelated)
			}
			if options.IndividualTool.Description != tt.wantDescription {
				t.Fatalf("IndividualTool.Description = %q, want %q", options.IndividualTool.Description, tt.wantDescription)
			}
		})
	}
}

// TestApplyAdminMeta_EntrySlices_AreCopiedNotAliased verifies the aliases and related
// actions written onto the options do not share backing arrays with the entry
// they came from. adminActionMeta is a package-level table read once per spec
// build and every surface is projected from those specs, so a caller sorting or
// appending to a spec's aliases would otherwise rewrite the table for every
// later build in the process.
func TestApplyAdminMeta_EntrySlices_AreCopiedNotAliased(t *testing.T) {
	meta := adminActionMetaEntry{
		aliases: []string{"entry alias"},
		related: []string{"admin.entry_related"},
	}

	var options toolutil.ActionSpecOptions
	applyAdminMeta(&options, meta)
	options.Aliases[0] = "scribbled"
	options.RelatedActions[0] = "admin.scribbled"

	if meta.aliases[0] != "entry alias" {
		t.Fatalf("entry aliases = %v, want the table value untouched", meta.aliases)
	}
	if meta.related[0] != "admin.entry_related" {
		t.Fatalf("entry related actions = %v, want the table value untouched", meta.related)
	}
}

// schemaPropertyAt walks a JSON Schema down a dotted property path and returns
// the property object at the end of it, stepping through an array's items the
// way toolutil resolves an override target so a path into a list of objects
// ("entities.source_type") reads the same property the override patched.
func schemaPropertyAt(t *testing.T, schema map[string]any, path string) map[string]any {
	t.Helper()
	parts := strings.Split(path, ".")
	current := schema
	for i, part := range parts {
		properties, ok := current["properties"].(map[string]any)
		if !ok {
			t.Fatalf("schema at %q carries no properties", strings.Join(parts[:i], "."))
		}
		child, ok := properties[part].(map[string]any)
		if !ok {
			t.Fatalf("schema carries no property %q", strings.Join(parts[:i+1], "."))
		}
		if items, hasItems := child["items"].(map[string]any); hasItems && i < len(parts)-1 {
			child = items
		}
		current = child
	}
	return current
}

// schemaEnumValues returns the enum a schema property publishes, or nil when it
// publishes none, refusing an entry that is not a string so an enum rendered in
// some other shape reads as a failure rather than as an absence.
func schemaEnumValues(t *testing.T, property map[string]any) []string {
	t.Helper()
	raw, ok := property["enum"]
	if !ok {
		return nil
	}
	entries, ok := raw.([]any)
	if !ok {
		t.Fatalf("enum = %T, want []any", raw)
	}
	values := make([]string, 0, len(entries))
	for _, entry := range entries {
		value, isString := entry.(string)
		if !isString {
			t.Fatalf("enum entry = %T (%v), want string", entry, entry)
		}
		values = append(values, value)
	}
	return values
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
