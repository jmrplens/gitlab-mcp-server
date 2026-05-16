package adminspecs

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
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

func specsByName(t *testing.T, specs []toolutil.ActionSpec) map[string]toolutil.ActionSpec {
	t.Helper()
	byName := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	return byName
}

func assertBoolOverride(t *testing.T, name string, got *bool, want *bool) {
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
