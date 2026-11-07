// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs_test.go contains canonical-route tests for commit discussion delete behavior.
package commitdiscussions

import (
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/commits"
)

// repositoryDomainIDs is the set of canonical IDs the gitlab_repository group
// really holds for the two packages a commit discussion cross-links to: this
// one and the commit package it points at for the commit itself.
//
// Both are built from their own ActionSpecs rather than listed here, because a
// list written beside the constants would agree with them whatever either said.
// They share one prefix because they share one catalog group, so there is no
// second spelling of the domain to drift either.
func repositoryDomainIDs(t *testing.T) map[string]bool {
	t.Helper()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	ids := map[string]bool{}
	for _, spec := range ActionSpecs(client) {
		ids[canonicalID(spec.Name)] = true
	}
	for _, spec := range commits.ActionSpecs(client) {
		ids[canonicalID(spec.Name)] = true
	}
	return ids
}

// TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds asserts that every
// related action a commit discussion spec publishes is an ID the catalog
// really holds.
//
// Sixteen of them named a commit_discussion domain that does not exist. These
// specs join the gitlab_repository group beside the commit ones, so the ID of
// every action here is "repository.commit_discussion_…"; the two cross-links
// that already read "repository." were right, which is what made the rest look
// deliberate. The spec names and the IDs are now one constant block, and this
// holds the published IDs to the specs the two packages register.
func TestActionSpecs_RelatedActionsNameActionsTheCatalogHolds(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	registered := repositoryDomainIDs(t)

	for _, spec := range ActionSpecs(client) {
		t.Run(spec.Name, func(t *testing.T) {
			if len(spec.RelatedActions) == 0 {
				t.Fatalf("%s publishes no related actions", spec.Name)
			}
			for _, related := range spec.RelatedActions {
				if !registered[related] {
					t.Errorf("%s relates to %q, which the repository group holds no action under", spec.Name, related)
				}
			}
		})
	}
}

// TestDecorateCommitDiscussionMeta_UnknownTool_PublishesResolvableRelations
// asserts the fallback arm too.
//
// Every individual tool has an arm of its own, so the default arm is reached by
// nothing today and is exactly the kind of code a rename silently starts using.
// Its two cross-links are held to the same set as the rest.
func TestDecorateCommitDiscussionMeta_UnknownTool_PublishesResolvableRelations(t *testing.T) {
	registered := repositoryDomainIDs(t)

	options := commitDiscussionOptions("gitlab_commit_discussion_tool_that_does_not_exist")
	if len(options.RelatedActions) == 0 {
		t.Fatal("the fallback arm publishes no related actions")
	}
	for _, related := range options.RelatedActions {
		t.Run(related, func(t *testing.T) {
			if !registered[related] {
				t.Errorf("the fallback arm relates to %q, which the repository group holds no action under", related)
			}
		})
	}
}

// TestActionSpecs_DeleteNoteError validates the DeleteNoteError route through the catalog surface.
// The test exercises the DELETE path of the underlying GitLab API call.
// It asserts that the returned error is wrapped and contains a useful hint.
func TestActionSpecs_DeleteNoteError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	client := testutil.NewTestClient(t, mux)
	byTool := commitDiscussionSpecsByTool(t, ActionSpecs(client))

	_, err := byTool["gitlab_delete_commit_discussion_note"].Route.Handler(t.Context(), map[string]any{"project_id": "42", "commit_sha": "abc123", "discussion_id": "d1", "note_id": 1})
	if err == nil {
		t.Fatal("expected error from delete with failing backend")
	}
}

// TestDecorateCommitDiscussionMeta_Default verifies the fallback metadata arm
// used for any individual tool name without a dedicated switch case.
func TestDecorateCommitDiscussionMeta_Default(t *testing.T) {
	opts := commitDiscussionOptions("gitlab_unknown_commit_discussion_tool")
	if opts.Usage == "" {
		t.Error("expected fallback usage to be set")
	}
	if len(opts.RelatedActions) == 0 {
		t.Error("expected fallback related actions")
	}
	if opts.ParameterGuidance["commit_sha"].SemanticRole == "" {
		t.Error("expected fallback commit_sha guidance")
	}
}

// TestActionSpecs_PositionGuidance_NamesWhereEachPartComesFrom verifies that
// repository.commit_discussion_create sends a caller for the SHAs of a
// position to repository.commit_get, whose output carries the commit's id and
// parent_ids, and for the file and the line to repository.commit_diff. GitLab
// holds a commit position to the commit's own diff refs, base_sha and
// start_sha both its first parent and head_sha the commit
// (Commit#diff_refs, DiffPositionableNote#diff_refs_match_commit). The
// guidance used to send a caller to repository.commit_diff for the SHAs too,
// which lists the diff's files and none of its SHAs. It also asks for both of
// the file's paths, in its example as well: GitLab looks a commit position's
// file up in the commit's diff by the paths it carries, as it does a merge
// request's, so one path finds a renamed file only as a deleted or an added
// one. It used to offer either path as sufficient. It gives a text
// position's line by [lineRule], where it used to ask for new_line or
// old_line. A file position may name a file outside the commit's diff,
// which GitLab accepts and which has no entry in repository.commit_diff, so
// the guidance and the two path fields say that such a file takes the path
// repository.tree lists at the commit as both, where they used to send the
// caller to that entry for every file. And the action's Usage names the two
// things a position this action sends anchors a comment to, a line of the
// commit's diff or a whole file, where it used to offer only a file and a
// line.
func TestActionSpecs_PositionGuidance_NamesWhereEachPartComesFrom(t *testing.T) {
	detail := reflect.TypeFor[commits.DetailOutput]()
	id, hasID := detail.FieldByName("ID")
	parents, hasParents := detail.FieldByName("ParentIDs")
	if !hasID || id.Tag.Get("json") != "id" || !hasParents || parents.Tag.Get("json") != "parent_ids" {
		t.Fatalf("repository.commit_get's output publishes no id and parent_ids for the guidance to point at")
	}
	client := testutil.NewTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("building the specs should reach no GitLab")
	}))
	spec := commitDiscussionSpecsByTool(t, ActionSpecs(client))["gitlab_create_commit_discussion"]
	if want := "Provide a position to attach an inline comment to a line of the commit's diff or a whole file, or omit it for a general commit discussion."; !strings.Contains(spec.Usage, want) {
		t.Errorf("Usage = %q, want it to say %q", spec.Usage, want)
	}
	guidance := spec.ParameterGuidance["position"]
	for _, want := range []string{
		"base_sha and start_sha are both the commit's first parent and head_sha is the commit itself, from parent_ids and id in repository.commit_get,",
		"the file's old_path and new_path and the line come from repository.commit_diff.",
		"A file position may name a file outside the diff, whose path repository.tree lists at head_sha.",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(guidance.ValueSource, want) {
				t.Errorf("position guidance = %q, want it to say %q", guidance.ValueSource, want)
			}
		})
	}
	confusions := strings.Join(guidance.CommonConfusions, " ")
	for _, want := range []string{
		"a commit with no parent takes the all-zero SHA for base_sha and start_sha",
		"Send old_path and new_path, both from the file's entry in repository.commit_diff, which repeats the path for a file that was not renamed.",
		"A file outside the diff has no entry there, so the path repository.tree lists at head_sha goes in both.",
		"finds a renamed file's lines only by both",
		"text also needs a line of the commit diff (" + lineRule + ")",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(confusions, want) {
				t.Errorf("position guidance confusions = %q, want them to say %q", confusions, want)
			}
		})
	}
	if !strings.Contains(guidance.ExampleBinding, `"old_path":"main.go","new_path":"main.go"`) {
		t.Errorf("position example = %q, want it to send both paths", guidance.ExampleBinding)
	}
	for _, name := range []string{"OldPath", "NewPath"} {
		t.Run(name, func(t *testing.T) {
			field, _ := reflect.TypeFor[PositionInput]().FieldByName(name)
			if description, want := field.Tag.Get("jsonschema"), "or from repository.tree for a file outside the diff."; !strings.Contains(description, want) {
				t.Errorf("%s description = %q, want it to say %q", name, description, want)
			}
		})
	}
}

// TestPositionInput_Lines_AreDescribedByTheLineRule verifies that the
// position_type, new_line and old_line descriptions of a commit position give
// a text position's line by [lineRule]: an unchanged line needs both of its
// numbers. They used to say a text position needs new_line or old_line, and
// the two line fields said nothing about which to send, while this action
// reads no diff before the write and GitLab refuses one number on an
// unchanged line with a 400 about the line code.
func TestPositionInput_Lines_AreDescribedByTheLineRule(t *testing.T) {
	for name, want := range map[string]string{
		"PositionType": "which needs a line (" + lineRule + ")",
		"NewLine":      "Send it alone for an added line, and with old_line for an unchanged one",
		"OldLine":      "Send it alone for a removed line, and with new_line for an unchanged one",
	} {
		t.Run(name, func(t *testing.T) {
			field, _ := reflect.TypeFor[PositionInput]().FieldByName(name)
			if description := field.Tag.Get("jsonschema"); !strings.Contains(description, want) {
				t.Errorf("%s description = %q, want it to say %q", name, description, want)
			}
		})
	}
}
