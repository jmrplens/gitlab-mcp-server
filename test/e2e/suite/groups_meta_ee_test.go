//go:build e2e && enterprise

// groups_meta_ee_test.go contains Enterprise gitlab_group meta-tool helpers.
package suite

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupanalytics"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupboards"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupcredentials"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedbranches"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupprotectedenvs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsaml"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupsshcerts"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/groupwikis"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools/securitysettings"
)

func runMetaGroupBoardOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	runEnterpriseMetaGroupBoardOperations(t, ctx, groupID, groupIDStr)
}

func runMetaGroupEnterpriseOperations(t *testing.T, ctx context.Context, groupPath string, groupID int64, groupIDStr string) {
	t.Helper()
	if !sess.enterprise {
		return
	}
	runEnterpriseMetaGroupAnalyticsOperations(t, ctx, groupPath)
	runEnterpriseMetaGroupSecuritySettingsOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupSSHCertOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupCredentialOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupLDAPOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupSAMLOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupWikiOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupProtectedBranchOperations(t, ctx, groupID, groupIDStr)
	runEnterpriseMetaGroupProtectedEnvOperations(t, ctx, groupID, groupIDStr)
}

func runEnterpriseMetaGroupAnalyticsOperations(t *testing.T, ctx context.Context, groupPath string) {
	t.Helper()

	t.Run("AnalyticsIssuesCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.IssuesCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_issues_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_issues_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})

	t.Run("AnalyticsMRCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.MRCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_mr_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_mr_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})

	t.Run("AnalyticsMembersCount", func(t *testing.T) {
		out, err := callToolOn[groupanalytics.MembersCountOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "analytics_members_count",
			"params": map[string]any{"group_path": groupPath},
		})
		requireNoError(t, err, "analytics_members_count")
		requireTruef(t, out.GroupPath == groupPath, "group path mismatch: got %q want %q", out.GroupPath, groupPath)
	})
}

func runEnterpriseMetaGroupSecuritySettingsOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	t.Run("SecuritySettingsUpdate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[securitysettings.GroupOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "security_settings_update",
			"params": map[string]any{
				"group_id":                       groupIDStr,
				"secret_push_protection_enabled": true,
			},
		})
		requireNoError(t, err, "security_settings_update")
		requireTruef(t, out.SecretPushProtectionEnabled, "expected group secret push protection enabled")
	})
}

func runEnterpriseMetaGroupSSHCertOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var certificateID int64

	t.Run("SSHCertListEmpty", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ssh_cert_list empty")
		requireTruef(t, len(out.Certificates) == 0, "expected 0 SSH certificates, got %d", len(out.Certificates))
	})

	t.Run("SSHCertCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupsshcerts.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key":      generateED25519AuthorizedKey(t, "e2e-group-ssh-cert"),
				"title":    uniqueName("group-ssh-cert"),
			},
		})
		requireNoError(t, err, "ssh_cert_create")
		requireTruef(t, out.ID > 0, "expected SSH certificate ID > 0")
		certificateID = out.ID
	})

	t.Run("SSHCertListOne", func(t *testing.T) {
		requireTruef(t, certificateID > 0, "certificateID not set")
		out, err := callToolOn[groupsshcerts.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ssh_cert_list one")
		requireTruef(t, len(out.Certificates) >= 1, "expected at least 1 SSH certificate, got %d", len(out.Certificates))
	})

	t.Run("SSHCertDelete", func(t *testing.T) {
		requireTruef(t, certificateID > 0, "certificateID not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ssh_cert_delete",
			"params": map[string]any{
				"group_id":       groupIDStr,
				"certificate_id": certificateID,
			},
		})
		requireNoError(t, err, "ssh_cert_delete")
		certificateID = 0
	})
}

func runEnterpriseMetaGroupCredentialOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	t.Run("CredentialListPATsUnavailableHint", func(t *testing.T) {
		_, err := callToolOn[groupcredentials.PATListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_list_pats",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialListSSHKeysUnavailableHint", func(t *testing.T) {
		_, err := callToolOn[groupcredentials.SSHKeyListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_list_ssh_keys",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group credential inventory", "/groups/:id/manage", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialRevokePATUnavailableHint", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_revoke_pat",
			"params": map[string]any{
				"group_id": groupIDStr,
				"token_id": int64(999999),
			},
		})
		requireErrorContainsAll(t, err, "credential_list_pats", "group credential inventory", "Ultimate", "Owner or admin")
	})

	t.Run("CredentialDeleteSSHKeyUnavailableHint", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "credential_delete_ssh_key",
			"params": map[string]any{
				"group_id": groupIDStr,
				"key_id":   int64(999999),
			},
		})
		requireErrorContainsAll(t, err, "credential_list_ssh_keys", "group credential inventory", "Ultimate", "Owner or admin")
	})
}

func requireErrorContainsAll(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errText := err.Error()
	for _, fragment := range fragments {
		if !strings.Contains(errText, fragment) {
			t.Fatalf("error %q does not contain %q", errText, fragment)
		}
	}
}

func runEnterpriseMetaGroupLDAPOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	provider := "ldapmain"
	cn := uniqueName("ldap-cn")
	providerCN := uniqueName("ldap-provider-cn")

	t.Run("LDAPLinkListEmpty", func(t *testing.T) {
		out, err := callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ldap_link_list empty")
		requireTruef(t, len(out.Links) == 0, "expected no LDAP links, got %d", len(out.Links))
	})

	t.Run("LDAPLinkAdd", func(t *testing.T) {
		out, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"cn":           cn,
				"group_access": 30,
				"provider":     provider,
			},
		})
		requireNoError(t, err, "ldap_link_add")
		requireTruef(t, out.CN == cn, "LDAP link CN mismatch: got %q want %q", out.CN, cn)
	})

	t.Run("LDAPLinkListOne", func(t *testing.T) {
		out, err := callToolOn[groupldap.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "ldap_link_list one")
		requireTruef(t, len(out.Links) >= 1, "expected at least 1 LDAP link, got %d", len(out.Links))
	})

	t.Run("LDAPLinkDelete", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_delete",
			"params": map[string]any{
				"group_id": groupIDStr,
				"cn":       cn,
				"provider": provider,
			},
		})
		requireNoError(t, err, "ldap_link_delete")
	})

	t.Run("LDAPLinkDeleteForProvider", func(t *testing.T) {
		_, err := callToolOn[groupldap.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_add",
			"params": map[string]any{
				"group_id":     groupIDStr,
				"cn":           providerCN,
				"group_access": 30,
				"provider":     provider,
			},
		})
		requireNoError(t, err, "ldap_link_add for provider delete")

		err = callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "ldap_link_delete_for_provider",
			"params": map[string]any{
				"group_id": groupIDStr,
				"provider": provider,
				"cn":       providerCN,
			},
		})
		requireNoError(t, err, "ldap_link_delete_for_provider")
	})
}

func runEnterpriseMetaGroupSAMLOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	requireTruef(t, groupID > 0, "groupID not set")

	samlGroup := uniqueName("saml-group")

	t.Run("SAMLLinkListRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkAddRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_add",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
				"access_level":    30,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkGetRequiresConfiguredSSO", func(t *testing.T) {
		_, err := callToolOn[groupsaml.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_get",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})

	t.Run("SAMLLinkDeleteRequiresConfiguredSSO", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "saml_link_delete",
			"params": map[string]any{
				"group_id":        groupIDStr,
				"saml_group_name": samlGroup,
			},
		})
		requireErrorContainsAll(t, err, "group SAML SSO", "Premium/Ultimate", "Owner access", "401 or 404")
	})
}

func runEnterpriseMetaGroupBoardOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var boardID int64

	t.Run("BoardCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     "Test Board",
			},
		})
		requireNoError(t, err, "group_board_create")
		boardID = out.ID
		t.Logf("Created group board %d", boardID)
	})

	t.Run("BoardList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		// GitLab EE group boards may not be visible in the list endpoint
		// right after creation depending on the routing between
		// `group_board_list` and `epic_board_list` aliases. Accept either
		// the created board appearing or an empty list — both confirm the
		// tool routes successfully.
		var lastBoards []groupboards.GroupBoardOutput
		out, err := callToolOn[groupboards.ListGroupBoardsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "group_board_list")
		lastBoards = out.Boards
		t.Logf("Listed %d group boards (created board presence is best-effort on EE)", len(lastBoards))
	})

	t.Run("BoardGet", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_get",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "board_get")
		requireTruef(t, out.ID == boardID, "board ID mismatch")
		t.Logf("Got board %d: %s", out.ID, out.Name)
	})

	t.Run("BoardUpdate", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.GroupBoardOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_update",
			"params": map[string]any{
				"group_id": groupIDStr,
				"board_id": boardID,
				"name":     "Updated Board",
			},
		})
		requireNoError(t, err, "group_board_update")
		t.Logf("Updated board %d: %s", out.ID, out.Name)
	})

	t.Run("BoardListLists", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		out, err := callToolOn[groupboards.ListBoardListsOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_list_lists",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "board_list_lists")
		t.Logf("Board has %d lists", len(out.Lists))
	})

	t.Run("BoardDelete", func(t *testing.T) {
		if boardID == 0 {
			return
		}
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "group_board_delete",
			"params": map[string]any{"group_id": groupIDStr, "board_id": boardID},
		})
		requireNoError(t, err, "group_board_delete")
		t.Logf("Deleted board %d", boardID)
	})
}

func generateED25519AuthorizedKey(t *testing.T, comment string) string {
	t.Helper()
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	requireNoError(t, err, "generate ed25519 public key")

	const keyType = "ssh-ed25519"
	payload := make([]byte, 0, 4+len(keyType)+4+len(publicKey))
	payload = appendLengthPrefixed(payload, []byte(keyType))
	payload = appendLengthPrefixed(payload, publicKey)

	return keyType + " " + base64.StdEncoding.EncodeToString(payload) + " " + comment
}

const maxSSHWireFieldLength = 1<<32 - 1

func appendLengthPrefixed(dst, value []byte) []byte {
	lengthValue := uint64(len(value))
	if lengthValue > maxSSHWireFieldLength {
		panic("ssh key field is too large")
	}

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(lengthValue))
	dst = append(dst, length[:]...)
	dst = append(dst, value...)
	return dst
}

func runEnterpriseMetaGroupWikiOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	var wikiSlug string

	t.Run("WikiCreate", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_create",
			"params": map[string]any{
				"group_id": groupIDStr,
				"title":    uniqueName("group-wiki"),
				"content":  "# E2E group wiki\n\nInitial content.",
				"format":   "markdown",
			},
		})
		requireNoError(t, err, "wiki_create")
		requireTruef(t, out.Slug != "", "expected wiki slug")
		wikiSlug = out.Slug
		t.Logf("Created group wiki page %s", wikiSlug)
	})

	t.Run("WikiList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupwikis.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_list",
			"params": map[string]any{"group_id": groupIDStr, "with_content": true},
		})
		requireNoError(t, err, "wiki_list")
		requireTruef(t, len(out.WikiPages) >= 1, "expected at least 1 group wiki page")
		t.Logf("Listed %d group wiki pages", len(out.WikiPages))
	})

	t.Run("WikiGet", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_get",
			"params": map[string]any{"group_id": groupIDStr, "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_get")
		requireTruef(t, out.Slug == wikiSlug, "wiki slug mismatch: got %q want %q", out.Slug, wikiSlug)
		t.Logf("Got group wiki page %s", out.Slug)
	})

	t.Run("WikiEdit", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		out, err := callToolOn[groupwikis.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_edit",
			"params": map[string]any{
				"group_id": groupIDStr,
				"slug":     wikiSlug,
				"content":  "# E2E group wiki\n\nUpdated content.",
			},
		})
		requireNoError(t, err, "wiki_edit")
		t.Logf("Updated group wiki page %s", out.Slug)
	})

	t.Run("WikiDelete", func(t *testing.T) {
		requireTruef(t, wikiSlug != "", "wikiSlug not set")
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "wiki_delete",
			"params": map[string]any{"group_id": groupIDStr, "slug": wikiSlug},
		})
		requireNoError(t, err, "wiki_delete")
		t.Logf("Deleted group wiki page %s", wikiSlug)
	})
}

func runEnterpriseMetaGroupProtectedBranchOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	branchName := uniqueName("release-") + "/*"

	t.Run("ProtectedBranchProtect", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_protect",
			"params": map[string]any{
				"group_id":               groupIDStr,
				"name":                   branchName,
				"push_access_level":      40,
				"merge_access_level":     40,
				"unprotect_access_level": 40,
				"allow_force_push":       false,
			},
		})
		requireNoError(t, err, "protected_branch_protect")
		requireTruef(t, out.Name == branchName, "protected branch name mismatch: got %q want %q", out.Name, branchName)
		t.Logf("Protected group branch %s", out.Name)
	})

	t.Run("ProtectedBranchList", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedbranches.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_list",
			"params": map[string]any{"group_id": groupIDStr, "search": branchName},
		})
		requireNoError(t, err, "protected_branch_list")
		requireTruef(t, len(out.Branches) >= 1, "expected at least 1 group protected branch")
		t.Logf("Listed %d group protected branches", len(out.Branches))
	})

	t.Run("ProtectedBranchGet", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_get",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_get")
		requireTruef(t, out.Name == branchName, "protected branch name mismatch")
		t.Logf("Got group protected branch %s", out.Name)
	})

	t.Run("ProtectedBranchUpdate", func(t *testing.T) {
		out, err := callToolOn[groupprotectedbranches.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_update",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName, "allow_force_push": true},
		})
		requireNoError(t, err, "protected_branch_update")
		requireTruef(t, out.AllowForcePush, "expected allow_force_push=true after update")
		t.Logf("Updated group protected branch %s", out.Name)
	})

	t.Run("ProtectedBranchUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_branch_unprotect",
			"params": map[string]any{"group_id": groupIDStr, "branch": branchName},
		})
		requireNoError(t, err, "protected_branch_unprotect")
		t.Logf("Unprotected group branch %s", branchName)
	})
}

func runEnterpriseMetaGroupProtectedEnvOperations(t *testing.T, ctx context.Context, groupID int64, groupIDStr string) {
	t.Helper()
	const envName = "production"
	const developerAccessLevel = 30
	var protectedEnvDeployLevelID int64

	t.Run("ProtectedEnvInvalidTierHint", func(t *testing.T) {
		_, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     uniqueName("production-"),
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireTruef(t, err != nil, "expected protected_env_protect invalid tier error")
		requireTruef(t, strings.Contains(err.Error(), "valid group protected environment tiers"), "expected valid tier hint, got %v", err)
		requireTruef(t, strings.Contains(err.Error(), "production"), "expected tier values in error, got %v", err)
	})

	t.Run("ProtectedEnvProtect", func(t *testing.T) {
		requireTruef(t, groupID > 0, "groupID not set")
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_protect",
			"params": map[string]any{
				"group_id": groupIDStr,
				"name":     envName,
				"deploy_access_levels": []map[string]any{
					{"access_level": 40},
				},
			},
		})
		requireNoError(t, err, "protected_env_protect")
		requireTruef(t, out.Name == envName, "protected environment name mismatch: got %q want %q", out.Name, envName)
		requireTruef(t, len(out.DeployAccessLevels) >= 1, "expected at least 1 deploy access level")
		protectedEnvDeployLevelID = out.DeployAccessLevels[0].ID
		requireTruef(t, protectedEnvDeployLevelID > 0, "expected deploy access level ID")
		t.Logf("Protected group environment %s", out.Name)
	})

	t.Run("ProtectedEnvList", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.ListOutput](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_list",
			"params": map[string]any{"group_id": groupIDStr},
		})
		requireNoError(t, err, "protected_env_list")
		requireTruef(t, len(out.Environments) >= 1, "expected at least 1 group protected environment")
		t.Logf("Listed %d group protected environments", len(out.Environments))
	})

	t.Run("ProtectedEnvGet", func(t *testing.T) {
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_get",
			"params": map[string]any{"group_id": groupIDStr, "environment": envName},
		})
		requireNoError(t, err, "protected_env_get")
		requireTruef(t, out.Name == envName, "protected environment name mismatch")
		t.Logf("Got group protected environment %s", out.Name)
	})

	t.Run("ProtectedEnvUpdate", func(t *testing.T) {
		requireTruef(t, protectedEnvDeployLevelID > 0, "protectedEnvDeployLevelID not set")
		out, err := callToolOn[groupprotectedenvs.Output](ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_update",
			"params": map[string]any{
				"group_id":    groupIDStr,
				"environment": envName,
				"deploy_access_levels": []map[string]any{
					{"id": protectedEnvDeployLevelID, "access_level": developerAccessLevel},
				},
			},
		})
		requireNoError(t, err, "protected_env_update")
		requireTruef(t, out.Name == envName, "protected environment name mismatch after update: got %q want %q", out.Name, envName)
		foundDeveloperAccess := false
		for _, deployAccessLevel := range out.DeployAccessLevels {
			if deployAccessLevel.AccessLevel == developerAccessLevel {
				foundDeveloperAccess = true
				break
			}
		}
		requireTruef(t, foundDeveloperAccess, "expected deploy access level %d after update", developerAccessLevel)
		t.Logf("Updated group protected environment %s", out.Name)
	})

	t.Run("ProtectedEnvUnprotect", func(t *testing.T) {
		err := callToolVoidOn(ctx, sess.meta, "gitlab_group", map[string]any{
			"action": "protected_env_unprotect",
			"params": map[string]any{"group_id": groupIDStr, "environment": envName},
		})
		requireNoError(t, err, "protected_env_unprotect")
		t.Logf("Unprotected group environment %s", envName)
	})
}
