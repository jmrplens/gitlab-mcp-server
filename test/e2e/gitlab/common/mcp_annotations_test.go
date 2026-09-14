//go:build e2e

// mcp_annotations_test.go covers the MCP tool annotations the binary publishes
// on tools/list: the destructive and read-only hints a client reads to decide
// whether a tool needs confirmation and whether it is safe to retry.
//
// The hints are catalog metadata, but nothing else in this suite reads them
// off the served surface: the sweeps and the tier test compare names and
// served sets, and a wrong hint on a registered tool would pass all of them.
// This reads the tool objects the server actually published, on the two
// surfaces that register tools of their own, and holds each hint to the tool's
// own semantics.

package common

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// destructiveVerbMarkers are the verbs an individual tool name carries when it
// removes something, either between underscores or as a terminal suffix.
var destructiveVerbMarkers = []string{"delete", "revoke", "unprotect", "remove", "purge"}

// mutatingPrefixes and mutatingSuffixes are the writes that are not reads, so a
// tool whose name also contains "list" or "get" is still not read-only.
var (
	mutatingPrefixes = []string{"gitlab_create_", "gitlab_update_", "gitlab_add_", "gitlab_edit_", "gitlab_set_", "gitlab_upload_"}
	mutatingSuffixes = []string{"_create", "_update", "_add", "_edit", "_set", "_upload"}
	readVerbMarkers  = []string{"list", "get", "search"}
)

// TestAnnotations_IndividualToolsCarryConsistentHints checks that an individual
// tool whose name names a destructive verb is annotated destructive, and one
// that only reads is annotated read-only and not destructive.
//
// Replaces: TestAnnotations_DestructiveHint_Individual
func TestAnnotations_IndividualToolsCarryConsistentHints(t *testing.T) {
	e := harness.New(t)
	tools := e.On(harness.SurfaceIndividual).ToolDefinitions()
	if len(tools) == 0 {
		t.Fatal("the individual surface served no tools")
	}

	destructive, readOnly := 0, 0
	for _, tool := range tools {
		if tool.Annotations == nil {
			t.Errorf("individual tool %s has no annotations", tool.Name)
			continue
		}
		switch {
		case nameHasDestructiveVerb(tool.Name):
			destructive++
			if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
				t.Errorf("destructive tool %s has destructiveHint %v, want true", tool.Name, boolHint(tool.Annotations.DestructiveHint))
			}
		case nameIsReadOnly(tool.Name):
			readOnly++
			if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Errorf("read-only tool %s has destructiveHint %v, want false", tool.Name, boolHint(tool.Annotations.DestructiveHint))
			}
			if !tool.Annotations.ReadOnlyHint {
				t.Errorf("read-only tool %s has readOnlyHint false, want true", tool.Name)
			}
		}
	}
	if destructive == 0 {
		t.Error("no destructive individual tool was found, so nothing was checked")
	}
	if readOnly == 0 {
		t.Error("no read-only individual tool was found, so nothing was checked")
	}
	t.Logf("checked %d individual tools: %d destructive, %d read-only", len(tools), destructive, readOnly)
}

// TestAnnotations_MetaToolsCarryDestructiveHints checks that the meta
// dispatchers carry a destructive hint, that both values are present across the
// set (a dispatcher of only reads is not destructive, one with a delete route
// is), and that none is missing.
//
// Replaces: TestAnnotations_DestructiveHint_Meta
func TestAnnotations_MetaToolsCarryDestructiveHints(t *testing.T) {
	e := harness.New(t)
	tools := e.On(harness.SurfaceMeta).ToolDefinitions()
	if len(tools) == 0 {
		t.Fatal("the meta surface served no tools")
	}

	withDestructive, withoutDestructive := 0, 0
	for _, tool := range tools {
		if tool.Annotations == nil || tool.Annotations.DestructiveHint == nil {
			t.Errorf("meta tool %s has no destructiveHint", tool.Name)
			continue
		}
		if *tool.Annotations.DestructiveHint {
			withDestructive++
		} else {
			withoutDestructive++
		}
	}
	if withDestructive == 0 {
		t.Error("no meta tool is annotated destructive")
	}
	if withoutDestructive == 0 {
		t.Error("no meta tool is annotated non-destructive")
	}
	t.Logf("checked %d meta tools: %d destructive, %d not", len(tools), withDestructive, withoutDestructive)
}

// TestAnnotations_EveryToolIsAnnotated checks the structural invariant that no
// tool ships without the destructive and open-world hints, on both surfaces
// that register tools.
//
// Replaces: TestAnnotations_AllToolsHaveAnnotations
func TestAnnotations_EveryToolIsAnnotated(t *testing.T) {
	e := harness.New(t)

	for _, surface := range []harness.Surface{harness.SurfaceIndividual, harness.SurfaceMeta} {
		t.Run(string(surface), func(t *testing.T) {
			tools := e.On(surface).ToolDefinitions()
			if len(tools) == 0 {
				t.Fatalf("the %s surface served no tools", surface)
			}
			for _, tool := range tools {
				if tool.Annotations == nil {
					t.Errorf("%s tool %s has no annotations", surface, tool.Name)
					continue
				}
				if tool.Annotations.DestructiveHint == nil {
					t.Errorf("%s tool %s has no destructiveHint", surface, tool.Name)
				}
				if tool.Annotations.OpenWorldHint == nil {
					t.Errorf("%s tool %s has no openWorldHint", surface, tool.Name)
				}
			}
			t.Logf("checked %d %s tools for complete annotations", len(tools), surface)
		})
	}
}

// nameHasDestructiveVerb reports whether an individual tool name carries a
// destructive verb between underscores or as a terminal suffix.
func nameHasDestructiveVerb(name string) bool {
	for _, verb := range destructiveVerbMarkers {
		if strings.Contains(name, "_"+verb+"_") || strings.HasSuffix(name, "_"+verb) {
			return true
		}
	}
	return false
}

// nameIsReadOnly reports whether an individual tool name describes a read that
// is neither a destructive nor a mutating operation.
func nameIsReadOnly(name string) bool {
	if nameHasDestructiveVerb(name) {
		return false
	}
	for _, prefix := range mutatingPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	for _, suffix := range mutatingSuffixes {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	for _, verb := range readVerbMarkers {
		if strings.Contains(name, "_"+verb+"_") || strings.HasSuffix(name, "_"+verb) {
			return true
		}
	}
	return false
}

// boolHint spells an optional bool hint for a failure message.
func boolHint(hint *bool) string {
	if hint == nil {
		return "nil"
	}
	if *hint {
		return "true"
	}
	return "false"
}
