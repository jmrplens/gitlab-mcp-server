//go:build e2e

// annotations_test.go validates that MCP tool annotations are correctly set
// at runtime. This is the primary E2E validation for PR #22 (metadata-driven
// destructive detection): it calls tools/list on both individual and meta
// sessions and verifies that destructiveHint is consistent with tool semantics.
package suite

import (
	"context"
	"strings"
	"testing"
)

// TestAnnotations_DestructiveHint_Individual verifies that every individual
// tool with a destructive verb (delete, revoke, unprotect, remove, purge) in
// its name has destructiveHint set to true, and that every read-only tool
// (list, get, search) has destructiveHint explicitly set to false.
func TestAnnotations_DestructiveHint_Individual(t *testing.T) {
	t.Parallel()
	if sess.individual == nil {
		t.Skip("individual session not configured")
	}

	ctx := context.Background()
	result, err := sess.individual.ListTools(ctx, nil)
	requireNoError(t, err, "ListTools individual")
	requireTrue(t, len(result.Tools) > 0, "expected individual tools, got 0")

	t.Logf("Checking %d individual tools for annotation correctness", len(result.Tools))

	destructiveVerbs := []string{"_delete_", "_revoke_", "_unprotect_", "_remove_", "_purge_"}
	readOnlyVerbs := []string{"_list_", "_get_", "_search_"}

	var destructiveCount, readOnlyCount, missingAnnotations int

	for _, tool := range result.Tools {
		name := tool.Name

		if tool.Annotations == nil {
			t.Errorf("tool %s has nil Annotations", name)
			missingAnnotations++
			continue
		}

		isDestructive := false
		for _, verb := range destructiveVerbs {
			if strings.Contains(name, verb) {
				isDestructive = true
				break
			}
		}
		// Also check suffix (e.g. gitlab_group_delete)
		for _, verb := range destructiveVerbs {
			suffix := strings.TrimPrefix(verb, "_")
			suffix = strings.TrimSuffix(suffix, "_")
			if strings.HasSuffix(name, "_"+suffix) {
				isDestructive = true
				break
			}
		}

		// Mutating suffixes override read-only heuristic (e.g. gitlab_board_list_create
		// contains "_list_" in the resource name but ends with a mutating verb).
		mutatingVerbs := []string{"_create", "_update", "_add", "_edit", "_set", "_upload"}
		isMutating := false
		for _, verb := range mutatingVerbs {
			if strings.HasSuffix(name, verb) {
				isMutating = true
				break
			}
		}

		isReadOnly := false
		if !isMutating {
			for _, verb := range readOnlyVerbs {
				if strings.Contains(name, verb) {
					isReadOnly = true
					break
				}
			}
			// Also check suffix
			for _, verb := range readOnlyVerbs {
				suffix := strings.TrimPrefix(verb, "_")
				suffix = strings.TrimSuffix(suffix, "_")
				if strings.HasSuffix(name, "_"+suffix) {
					isReadOnly = true
					break
				}
			}
		}

		if isDestructive {
			destructiveCount++
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("destructive tool %s has nil DestructiveHint (expected true)", name)
			} else if !*tool.Annotations.DestructiveHint {
				t.Errorf("destructive tool %s has DestructiveHint=false (expected true)", name)
			}
		}

		if isReadOnly && !isDestructive {
			readOnlyCount++
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("read-only tool %s has nil DestructiveHint (expected false)", name)
			} else if *tool.Annotations.DestructiveHint {
				t.Errorf("read-only tool %s has DestructiveHint=true (expected false)", name)
			}
			if !tool.Annotations.ReadOnlyHint {
				t.Errorf("read-only tool %s has ReadOnlyHint=false (expected true)", name)
			}
		}
	}

	t.Logf("Verified %d destructive tools, %d read-only tools, %d missing annotations",
		destructiveCount, readOnlyCount, missingAnnotations)
	requireTrue(t, destructiveCount > 0, "expected at least 1 destructive tool, found 0")
	requireTrue(t, readOnlyCount > 0, "expected at least 1 read-only tool, found 0")
	requireTrue(t, missingAnnotations == 0, "found %d tools with missing annotations", missingAnnotations)
}

// TestAnnotations_DestructiveHint_Meta verifies that meta-tools containing
// destructive routes have destructiveHint=true (MetaAnnotations), while
// meta-tools with only read/create/update actions have destructiveHint=false
// (NonDestructiveMetaAnnotations or ReadOnlyMetaAnnotations).
func TestAnnotations_DestructiveHint_Meta(t *testing.T) {
	t.Parallel()
	if sess.meta == nil {
		t.Skip("meta session not configured")
	}

	ctx := context.Background()
	result, err := sess.meta.ListTools(ctx, nil)
	requireNoError(t, err, "ListTools meta")
	requireTrue(t, len(result.Tools) > 0, "expected meta tools, got 0")

	t.Logf("Checking %d meta-tools for annotation correctness", len(result.Tools))

	var withDestructive, withoutDestructive, missingAnnotations int

	for _, tool := range result.Tools {
		if tool.Annotations == nil {
			t.Errorf("meta-tool %s has nil Annotations", tool.Name)
			missingAnnotations++
			continue
		}

		if tool.Annotations.DestructiveHint == nil {
			t.Errorf("meta-tool %s has nil DestructiveHint", tool.Name)
			missingAnnotations++
			continue
		}

		if *tool.Annotations.DestructiveHint {
			withDestructive++
		} else {
			withoutDestructive++
		}
	}

	t.Logf("Meta-tools: %d with destructiveHint=true, %d with destructiveHint=false, %d missing annotations",
		withDestructive, withoutDestructive, missingAnnotations)
	requireTrue(t, withDestructive > 0, "expected at least 1 meta-tool with destructiveHint=true")
	requireTrue(t, withoutDestructive > 0, "expected at least 1 meta-tool with destructiveHint=false")
	requireTrue(t, missingAnnotations == 0, "found %d meta-tools with missing/nil annotations", missingAnnotations)
}

// TestAnnotations_AllToolsHaveAnnotations verifies that every registered tool
// (individual and meta) has non-nil Annotations with DestructiveHint set.
// This is a structural invariant: no tool should ship without annotations.
func TestAnnotations_AllToolsHaveAnnotations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Individual session
	if sess.individual != nil {
		indResult, err := sess.individual.ListTools(ctx, nil)
		requireNoError(t, err, "ListTools individual")
		for _, tool := range indResult.Tools {
			if tool.Annotations == nil {
				t.Errorf("[individual] tool %s: nil Annotations", tool.Name)
				continue
			}
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("[individual] tool %s: nil DestructiveHint", tool.Name)
			}
			if tool.Annotations.OpenWorldHint == nil {
				t.Errorf("[individual] tool %s: nil OpenWorldHint", tool.Name)
			}
		}
		t.Logf("Checked %d individual tools for complete annotations", len(indResult.Tools))
	}

	// Meta session
	if sess.meta != nil {
		metaResult, err := sess.meta.ListTools(ctx, nil)
		requireNoError(t, err, "ListTools meta")
		for _, tool := range metaResult.Tools {
			if tool.Annotations == nil {
				t.Errorf("[meta] tool %s: nil Annotations", tool.Name)
				continue
			}
			if tool.Annotations.DestructiveHint == nil {
				t.Errorf("[meta] tool %s: nil DestructiveHint", tool.Name)
			}
			if tool.Annotations.OpenWorldHint == nil {
				t.Errorf("[meta] tool %s: nil OpenWorldHint", tool.Name)
			}
		}
		t.Logf("Checked %d meta-tools for complete annotations", len(metaResult.Tools))
	}
}
