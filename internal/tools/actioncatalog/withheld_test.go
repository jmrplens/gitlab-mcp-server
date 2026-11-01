// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actioncatalog

import (
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// TestWithheld_Message_WordsEachCauseForTheToolAndTheActionAsNamed pins the
// answer a call for a withheld action gets, cause by cause, in the words the
// dynamic surface has given since issue 956 and the meta surface gives since
// issue 1282. The record is looked up by key and the answer quotes the action
// as the caller named it, which is how the meta surface asks: by the
// canonical ID its group's routes resolved the action argument to, quoting
// the argument.
func TestWithheld_Message_WordsEachCauseForTheToolAndTheActionAsNamed(t *testing.T) {
	var withheld Withheld
	withheld.RecordActions(
		[]ScopeWithheld{{ID: "issue.create", ByGitLab: []string{"api"}}, {ID: "admin.metadata_get", ByServer: []string{"admin_mode"}}},
		[]string{"Project.Hook_Add"},
	)
	withheld.RecordTier([]TierWithheld{{ID: "project.approval_config_get", Edition: "premium"}}, edition.Free, false)

	cases := []struct {
		name, tool, action, key string
		want                    string
	}{
		{
			name: "scope, named by GitLab", tool: "gitlab_issue", action: "create", key: "issue.create",
			want: `gitlab_issue: action "create" exists but is not available to this session: GitLab requires the api scope for it and the credential in use does not carry it, so a narrowed action surface was built for it. Reauthorize with the api scope to use it; do not report the capability as missing.`,
		},
		{
			name: "scope, named by this server", tool: "gitlab_execute_action", action: "admin.metadata_get", key: "admin.metadata_get",
			want: `gitlab_execute_action: action "admin.metadata_get" exists but is not available to this session: this server serves this action's group only to a credential carrying the admin_mode scope and the credential in use does not carry it, so a narrowed action surface was built for it. Reauthorize with the admin_mode scope to use it; do not report the capability as missing.`,
		},
		{
			name: "operator, the key normalized", tool: "gitlab_project", action: "hook_add", key: "  project.hook_add ",
			want: `gitlab_project: action "hook_add" exists but is not available: this deployment is configured to withhold it, so a narrowed action surface was built. Ask the operator to enable it; do not report the capability as missing.`,
		},
		{
			name: "tier, detected", tool: "gitlab_project", action: "approval_config_get", key: "project.approval_config_get",
			want: `gitlab_project: action "approval_config_get" exists but is not available: it needs GitLab Premium or Ultimate, and this instance's tier was detected as GitLab Free, so a narrowed action surface was built. If the instance is licensed for it, the detection was wrong: ask the operator to pin the tier with GITLAB_MCP_TIER or --tier. Do not report the capability as missing.`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := withheld.Message(tc.tool, tc.action, tc.key)
			if !ok || got != tc.want {
				t.Errorf("Message(%q, %q, %q) = %q, %t, want %q, true", tc.tool, tc.action, tc.key, got, ok, tc.want)
			}
			if !withheld.Holds(tc.key) {
				t.Errorf("Holds(%q) = false, want true for a key Message answers", tc.key)
			}
		})
	}

	t.Run("a key nothing withheld is not answered", func(t *testing.T) {
		if got, ok := withheld.Message("gitlab_project", "list", "project.list"); ok || got != "" {
			t.Errorf("Message() = %q, %t, want no answer", got, ok)
		}
		if withheld.Holds("project.list") {
			t.Error("Holds(project.list) = true, want false")
		}
	})
}

// TestWithheld_Message_TierAnswerNamesTheTierAndWhatElseWithholds covers the
// tier answer's variants: the tier the action needs, the one the session was
// built for and whether it was pinned, and the sentence it adds when pinning
// the tier would only uncover read-only mode or a scope the credential lacks.
func TestWithheld_Message_TierAnswerNamesTheTierAndWhatElseWithholds(t *testing.T) {
	cases := []struct {
		name    string
		entry   TierWithheld
		session edition.Tier
		pinned  bool
		want    []string
	}{
		{
			name: "an Ultimate action on a pinned Premium tier", entry: TierWithheld{Edition: "ultimate"}, session: edition.Premium, pinned: true,
			want: []string{"it needs GitLab Ultimate, and this deployment is configured for GitLab Premium", "ask the operator to configure the tier it has with GITLAB_MCP_TIER or --tier. Do not report"},
		},
		{
			name: "a session built for Ultimate names it", entry: TierWithheld{Edition: "Ultimate"}, session: edition.Ultimate, pinned: true,
			want: []string{"this deployment is configured for GitLab Ultimate"},
		},
		{
			name: "a write on a read-only deployment", entry: TierWithheld{Edition: "premium", ReadOnlyMode: true}, session: edition.Free,
			want: []string{" This deployment is also read-only, which withholds the action at any tier, so the operator would have to allow writes as well. Do not report"},
		},
		{
			name: "both scopes the credential lacks", entry: TierWithheld{Edition: "premium", ByGitLab: []string{"api"}, ByServer: []string{"admin_mode"}}, session: edition.Free,
			want: []string{" Once the tier serves it, GitLab requires the api scope for it, this server serves this action's group only to a credential carrying the admin_mode scope, and the credential in use does not carry them: reauthorize with the admin_mode and api scopes as well. Do not report"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var withheld Withheld
			tc.entry.ID = "group.epic_list"
			withheld.RecordTier([]TierWithheld{tc.entry}, tc.session, tc.pinned)
			got, ok := withheld.Message("gitlab_execute_action", "group.epic_list", "group.epic_list")
			if !ok {
				t.Fatal("Message() answered nothing for a tier-withheld key")
			}
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("Message() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// TestWithheld_Message_CredentialKind_NamesTheTokenTheActionNeeds pins the
// answer an action GitLab refuses to an OAuth access token gets (issue 1238,
// register row AUT-009): it exists, the credential in use is an OAuth token
// while GitLab answers only a personal access token, and the way out is to
// authenticate with one, which is the caller's to do.
func TestWithheld_Message_CredentialKind_NamesTheTokenTheActionNeeds(t *testing.T) {
	var withheld Withheld
	withheld.RecordCredentialKind([]string{"access.token_personal_rotate_self", " "})

	const want = `gitlab_execute_action: action "access.token_personal_rotate_self" exists but is not available to this session: GitLab answers what it sends only for a personal access token, and the credential in use is an OAuth access token, so a narrowed action surface was built for it. Authenticate with a personal access token, sent as the Bearer token, to use it; do not report the capability as missing.`
	got, ok := withheld.Message("gitlab_execute_action", "access.token_personal_rotate_self", "access.token_personal_rotate_self")
	if !ok || got != want {
		t.Errorf("Message() = %q, %t, want %q", got, ok, want)
	}
	if !withheld.Holds("Access.Token_Personal_Rotate_Self") || withheld.Holds("") {
		t.Error("Holds() does not read the credential-kind record by its normalized key alone")
	}
}

// TestWithheld_SessionTier_IsWhatRecordTierRecorded verifies the session tier
// a tier-pruned param's refusal is worded with (issue 1239): unknown until a
// tier is recorded, then the tier and whether it was pinned.
func TestWithheld_SessionTier_IsWhatRecordTierRecorded(t *testing.T) {
	var withheld Withheld
	if _, _, known := withheld.SessionTier(); known {
		t.Error("SessionTier() is known before any tier was recorded")
	}
	withheld.RecordTier(nil, edition.Premium, true)
	if tier, pinned, known := withheld.SessionTier(); tier != edition.Premium || !pinned || !known {
		t.Errorf("SessionTier() = %s, %t, %t; want premium, pinned, known", tier, pinned, known)
	}
}

// TestWithheld_Record_DropsBlankKeysAndNamesAScopeForAnEntryNamingNone
// verifies how the records are indexed: each key is lowercased and trimmed, a
// key that is blank or only spaces is not indexed at all, and a scope entry
// naming no scope is recorded as lacking GitLab's api.
//
// The records are looked up with the lowercased action a caller asked for, so
// an untrimmed key would never be found. A blank one is worse than useless: it
// would answer a caller who sent an empty action with the withheld explanation
// instead of the "action is required" refusal.
func TestWithheld_Record_DropsBlankKeysAndNamesAScopeForAnEntryNamingNone(t *testing.T) {
	var withheld Withheld
	withheld.RecordActions([]ScopeWithheld{{ID: " "}, {ID: "Issue.Create"}}, []string{"", "   "})
	withheld.RecordTier([]TierWithheld{{ID: ""}}, edition.Free, false)

	if withheld.Holds("") || withheld.Holds(" ") {
		t.Error("a blank key was recorded as withheld")
	}
	got, ok := withheld.Message("gitlab_execute_action", "issue.create", "issue.create")
	if !ok || !strings.Contains(got, "Reauthorize with the api scope") {
		t.Errorf("Message() = %q, %t, want the api scope named for an entry naming none", got, ok)
	}

	var empty Withheld
	empty.RecordActions(nil, nil)
	empty.RecordTier(nil, edition.Free, false)
	if empty.Holds("issue.create") {
		t.Error("an empty record holds a key")
	}
	if withheldKeySet(nil) != nil || withheldScopes(nil) != nil || withheldTiers(nil) != nil {
		t.Error("an empty list was indexed as something other than nil")
	}
}
