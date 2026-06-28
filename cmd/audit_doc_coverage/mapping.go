// Package main: README "Domains" table parser + group→doc mapping.
//
// The mapping is derived from docs/tools/README.md. Most rows list
// one or more gitlab_* meta-tool names that map directly to catalog
// groups; a handful of rows use "various" or include routed tools
// that must be split off their owning group. Both cases are
// enumerated explicitly here so the auditor's main logic stays
// table-driven and the special cases are reviewable in one place.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// docMappingRow captures one row of the README "Domains" table.
type docMappingRow struct {
	Domain        string
	ExpectedCount int
	MetaToolsCSV  string
	DocLink       string
}

// docMapping is the full README "Domains" table plus the
// hardcoded group→doc overrides needed to handle routed tools and
// "various" rows.
type docMapping struct {
	Rows     []docMappingRow
	Override docOverrideMap
}

// docOverrideMap is the set of catalog tools that should be
// attributed to a doc file even though their owning group maps
// elsewhere. Keys are doc files (relative to docs/tools); values
// are the IndividualTool.Name strings expected in that doc.
//
// Examples of why this exists:
//   - branch-rules.md owns gitlab_list_branch_rules, whose owning
//     group is gitlab_branch (not its own meta-tool).
//   - project-discovery.md owns gitlab_discover_project, whose
//     owning "group" is the standalone surface utility group
//     gitlab_discover_project.
//   - capabilities.md owns the four gitlab_interactive_* elicitation
//     tools plus gitlab_server_status, whose owning groups are
//     gitlab_server (MCP maintenance) and the standalone surface
//     utility.
type docOverrideMap map[string][]string

// loadDocMapping parses docs/tools/README.md and merges in the
// hardcoded overrides that handle routed tools and "various" rows.
func loadDocMapping(readmePath string) (*docMapping, error) {
	rows, err := parseDomainsTable(readmePath)
	if err != nil {
		return nil, err
	}
	return &docMapping{
		Rows:     rows,
		Override: hardcodedDocOverrides(),
	}, nil
}

// parseDomainsTable reads the Domains section of docs/tools/README.md
// and returns one row per table entry. Only the | Domain | Tools |
// Meta-tool | Document | columns are parsed; other columns (when
// added) are ignored.
func parseDomainsTable(readmePath string) ([]docMappingRow, error) {
	f, err := os.Open(readmePath)
	if err != nil {
		return nil, fmt.Errorf("open readme: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inDomains := false
	var rows []docMappingRow

	// Markdown table row pattern. Captures the four data columns and
	// tolerates varying whitespace around the pipes.
	rowRE := regexp.MustCompile(`^\|\s*(.+?)\s*\|\s*(\d+)\s*\|\s*(.+?)\s*\|\s*\[(.+?)\]\((.+?)\)\s*\|`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Domains" {
			inDomains = true
			continue
		}
		if !inDomains {
			continue
		}
		// End of the Domains table when we hit a non-table line that
		// isn't the separator row.
		if trimmed != "" && !strings.HasPrefix(trimmed, "|") {
			break
		}
		matches := rowRE.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		count, convErr := strconv.Atoi(matches[2])
		if convErr != nil {
			return nil, fmt.Errorf("invalid tool count %q in row %q: %w", matches[2], line, convErr)
		}
		rows = append(rows, docMappingRow{
			Domain:        matches[1],
			ExpectedCount: count,
			MetaToolsCSV:  matches[3],
			DocLink:       matches[5],
		})
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan readme: %w", scanErr)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no Domains rows parsed from %s", readmePath)
	}
	return rows, nil
}

// relativeDocPath converts a README "Document" link target (e.g.
// "access.md") to the repo-relative doc path used in the report
// (e.g. "docs/tools/access.md").
func relativeDocPath(link string) string {
	link = strings.TrimSpace(link)
	link = strings.TrimSuffix(link, ".md")
	base := filepath.Base(link)
	if base == "" || base == "." {
		return "docs/tools/"
	}
	return "docs/tools/" + base + ".md"
}

// expectedGroupsForRow returns the catalog group ToolNames that the
// given README row maps to. The result is the parsed "Meta-tool"
// column with backticks, parenthetical annotations, and other noise
// stripped. Rows annotated "(routed)" return nil: the listed group is
// a routing conduit, but the row owns only the tools enumerated in
// the hardcoded override (see parsePrefixAllowlists for the
// explicit per-doc naming-prefix lists that handle shared-group
// docs).
//
// First-claimer-wins in computeExpectedByDoc means the FIRST row to
// claim a given group is the canonical home for all tools in that
// group. Rows after the first that ALSO claim a group are interpreted
// via the prefix allowlist, not by re-claiming the group wholesale.
func expectedGroupsForRow(row docMappingRow) []string {
	if isRoutedRow(row) {
		return nil
	}

	// Parse the "Meta-tool" column. Tokens are comma-separated, may
	// contain backticks, and may include "etc.", "(routed)", or
	// "(enterprise routes)" annotations that we strip.
	csv := row.MetaToolsCSV
	csv = strings.ReplaceAll(csv, "`", "")
	csv = strings.ReplaceAll(csv, "etc.", "")
	csv = strings.ReplaceAll(csv, "(routed)", "")
	csv = strings.ReplaceAll(csv, "(enterprise routes)", "")
	csv = strings.ReplaceAll(csv, "(with `TOOL_SURFACE=meta`, routed as a branch action)", "")
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		// Keep only canonical gitlab_* meta-tool names; drop empty
		// tokens and parenthesised notes that may be left over.
		if trimmed == "" || !strings.HasPrefix(trimmed, "gitlab_") {
			continue
		}
		if strings.ContainsAny(trimmed, "()") {
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hardcodedDocOverrides returns the explicit list of routed tools
// per doc file. Tools listed here are attributed to the doc even
// though their owning group is mapped elsewhere by the README.
//
// The keys are doc basenames (e.g. "branch-rules.md"); the values are
// the IndividualTool.Name strings. Keep this list small and well-
// commented; the auditor's job is to verify the docs match the
// catalog, so any new routing exception should be reviewed here.
func hardcodedDocOverrides() docOverrideMap {
	return docOverrideMap{
		// Branch rules routes through gitlab_branch but its docs live
		// in branch-rules.md rather than branches.md.
		"branch-rules.md": {
			"gitlab_list_branch_rules",
		},
		// Project discovery is a standalone surface utility that
		// ships in its own meta-tool but is documented separately.
		"project-discovery.md": {
			"gitlab_discover_project",
		},
		// MCP capabilities: 4 elicitation tools + gitlab_server_status
		// (the rest of gitlab_server lives under admin.md). The
		// README's "15" count is the sum across these tools; we
		// enumerate them here to avoid depending on gitlab_server's
		// full action set.
		"capabilities.md": {
			"gitlab_interactive_issue_create",
			"gitlab_interactive_mr_create",
			"gitlab_interactive_project_create",
			"gitlab_interactive_release_create",
			"gitlab_server_status",
		},
		// Orbit is a GitLab.com-only meta-tool. The auditclient
		// mock reports a non-gitlab.com URL so the catalog walk
		// skips it entirely. We hardcode the six Orbit Knowledge
		// Graph tools here so orbit.md's expected set reflects the
		// production deployment and the Phase-1 agents can write
		// prose against a known set.
		"orbit.md": {
			"gitlab_orbit_dsl",
			"gitlab_orbit_graph_status",
			"gitlab_orbit_query",
			"gitlab_orbit_schema",
			"gitlab_orbit_status",
			"gitlab_orbit_tools",
		},
	}
}

// computeExpectedByDoc walks the catalog and assigns each tool to
// the doc file that should own it, using first-claimer-wins:
//
//  1. The hardcoded routed-tool overrides take precedence
//     (branch-rules.md, capabilities.md, project-discovery.md).
//  2. Otherwise the FIRST README row in table order whose claimed
//     groups include the tool's owning group is its primary home.
//  3. Otherwise the tool's name prefix is matched against the
//     per-doc prefix allowlist (covers shared-group docs like
//     boards, mirrors, access, security, notifications,
//     integrations, identity-security, analytics-compliance).
//  4. Otherwise the tool is unassigned.
//
// First-claimer-wins keeps the README's ordering meaningful: each
// row in the Domains table is the canonical home for the groups it
// claims, and any later row that ALSO claims a group is interpreted
// as "only owns the subset of tools whose name matches the shared
// naming convention". The prefix allowlist handles that subset
// without forcing every doc to enumerate every tool by hand.
func computeExpectedByDoc(mapping *docMapping, catalog *catalogSnapshot) (out map[string][]string, unassigned []string) {
	firstClaimer := buildFirstClaimer(mapping)
	prefixRules := buildPrefixRules()
	overrideDoc := buildOverrideDoc(mapping)

	out = make(map[string][]string)
	seenInCatalog := make(map[string]bool)

	// Step 4: assign each catalog tool to its primary home. Tools
	// matching multiple prefix rules go to the doc that owns the
	// longest matching prefix.
	for toolName, info := range catalog.Tools {
		seenInCatalog[toolName] = true
		if doc, ok := assignTool(toolName, info, firstClaimer, prefixRules, overrideDoc); ok {
			out[doc] = append(out[doc], toolName)
			continue
		}
		if info.OwnerPackage == "events" {
			out["docs/tools/users.md"] = append(out["docs/tools/users.md"], toolName)
			out["docs/tools/notifications.md"] = append(out["docs/tools/notifications.md"], toolName)
			continue
		}
		unassigned = append(unassigned, toolName)
	}

	// Step 5: the override list may include tools that the catalog
	// walk didn't surface (standalone surface utilities like
	// gitlab_interactive_* and gitlab_discover_project). Always
	// add them to the target doc so the Phase-1 agents know they're
	// in scope, even when the catalog snapshot doesn't enumerate
	// them. These tools are NOT counted as missing — the doc that
	// documents them satisfies the override.
	for tool, doc := range overrideDoc {
		if seenInCatalog[tool] {
			continue
		}
		out[doc] = append(out[doc], tool)
	}

	// Dedupe and sort each doc's expected list.
	for d, tools := range out {
		out[d] = sortedUnique(tools)
	}

	unassigned = sortedUnique(unassigned)
	return out, unassigned
}

// parsePrefixAllowlists returns the per-doc naming-prefix allowlists
// used by computeExpectedByDoc to handle docs that share groups with
// other docs. The longest matching prefix wins, so the lists are
// ordered from most specific to most generic.
//
// Each entry is a comment-and-table combo:
//   - The doc basename identifies the file under docs/tools/.
//   - The list of prefixes enumerates every IndividualTool.Name
//     segment that this doc owns, regardless of owning group.
//
// The lists are derived from the docs as they exist today (PR #190
// shipped the prose), plus the README's "(routed)" annotations for
// tools that are documented in a non-primary doc.
// groupExtensions adds extra catalog groups to a doc's claim beyond
// what the README "Meta-tool" column lists. The README truncates
// multi-group rows with "etc." or "various"; groupExtensions captures
// the omitted groups so the first-claimer lookup finds a home for
// every catalog tool. Keys are doc basenames (the form
// canonicalOverridePath expects).
func groupExtensions() map[string][]string {
	return map[string][]string{
		// CI/CD's README row lists `gitlab_pipeline`, `gitlab_job`,
		// "etc." but omits the adjacent groups that build the rest
		// of the CI/CD surface (CI variables, CI lint, CI catalog).
		"ci-cd.md": {
			"gitlab_ci_variable",
			"gitlab_ci_lint",
			"gitlab_ci_catalog",
		},
		// MCP Capabilities lists gitlab_server as its meta-tool but
		// the first-claimer rule (admin.md claims gitlab_server
		// earlier) would route gitlab_server_status to admin.md.
		// Claiming gitlab_server here ensures capabilities.md owns
		// its share of the MCP maintenance surface.
		"capabilities.md": {
			"gitlab_server",
		},
		// Notifications & Events's README row says "various". The
		// canonical groups it touches are gitlab_notification (the
		// global/group/project notification settings meta-tool) and
		// the events surface (handled by the OwnerPackage special
		// case in computeExpectedByDoc).
		"notifications.md": {
			"gitlab_notification",
		},
		// Analytics & Compliance lists gitlab_group (enterprise
		// routes), gitlab_compliance_policy, gitlab_project_alias
		// — DORA metrics is also documented here but its group
		// name is gitlab_dora_metrics, not gitlab_group.
		"analytics-compliance.md": {
			"gitlab_compliance_policy",
			"gitlab_project_alias",
			"gitlab_dora_metrics",
		},
		// Identity & Security lists gitlab_group_scim,
		// gitlab_member_role, "etc." The "etc." captures the
		// enterprise routes inside gitlab_group / gitlab_project.
		"identity-security.md": {
			"gitlab_group_scim",
			"gitlab_member_role",
		},
	}
}

func parsePrefixAllowlists() map[string][]string {
	return map[string][]string{
		// branches.md owns all gitlab_branch_* and
		// gitlab_protected_branch_* tools (one group, gitlab_branch).
		"docs/tools/branches.md": {
			"gitlab_branch_",
			"gitlab_protected_branch_",
		},
		// tags.md owns all gitlab_tag_* and gitlab_protected_tag_*
		// tools. Note: gitlab_tag_protect is a mutating tool that
		// should be a Write annotation; auditor's destructive
		// detection handles that.
		"docs/tools/tags.md": {
			"gitlab_tag_",
			"gitlab_protected_tag_",
		},
		// mirrors.md owns a small set of project mirror tools that
		// live inside gitlab_project (per buildProjectActionSpecs).
		// The README's "gitlab_project (enterprise routes)"
		// annotation is the source.
		"docs/tools/mirrors.md": {
			"gitlab_add_project_mirror",
			"gitlab_delete_project_mirror",
			"gitlab_edit_project_mirror",
			"gitlab_force_push_mirror_update",
			"gitlab_get_project_mirror",
			"gitlab_get_project_mirror_public_key",
			"gitlab_list_project_mirrors",
		},
		// boards.md owns the board/label/milestone surfaces even
		// though they live inside gitlab_project / gitlab_group
		// groups. Naming prefixes are how the team distinguishes
		// "project-management" surfaces from "core project CRUD".
		"docs/tools/boards.md": {
			"gitlab_board_",
			"gitlab_label_",
			"gitlab_milestone_",
		},
		// access.md owns every token, deploy key, deploy token,
		// access request, invite, and job-token-scoped tool — plus
		// the CRUD half of project/group member management. The
		// README's "various" annotation means the team explicitly
		// hand-curated these tools; the allowlist mirrors that.
		"docs/tools/access.md": {
			"gitlab_project_access_token_",
			"gitlab_group_access_token_",
			"gitlab_personal_access_token_",
			"gitlab_deploy_token_",
			"gitlab_deploy_key_",
			"gitlab_access_request_",
			"gitlab_project_invite",
			"gitlab_group_invite",
			"gitlab_get_job_token_access_settings",
			"gitlab_patch_job_token_access_settings",
			"gitlab_list_job_token_inbound_allowlist",
			"gitlab_list_job_token_group_allowlist",
			"gitlab_add_project_job_token_allowlist",
			"gitlab_add_group_job_token_allowlist",
			"gitlab_remove_project_job_token_allowlist",
			"gitlab_remove_group_job_token_allowlist",
			"gitlab_project_member_add",
			"gitlab_project_member_edit",
			"gitlab_project_member_delete",
			"gitlab_group_member_add",
			"gitlab_group_member_edit",
			"gitlab_group_member_delete",
		},
		// security.md owns feature flags, secure files, error
		// tracking, alert metric images, impersonation tokens, and
		// the admin-side user-token creation tool
		// (gitlab_create_personal_access_token). The PAT listing
		// tools live in access.md via the "personal_access_token_"
		// prefix; admin-user PAT creation lives here.
		"docs/tools/security.md": {
			"gitlab_feature_flag_",
			"gitlab_ff_user_list_",
			"gitlab_list_secure_files",
			"gitlab_show_secure_file",
			"gitlab_create_secure_file",
			"gitlab_remove_secure_file",
			"gitlab_get_error_tracking_settings",
			"gitlab_enable_disable_error_tracking",
			"gitlab_list_error_tracking_client_keys",
			"gitlab_create_error_tracking_client_key",
			"gitlab_delete_error_tracking_client_key",
			"gitlab_list_alert_metric_images",
			"gitlab_upload_alert_metric_image",
			"gitlab_update_alert_metric_image",
			"gitlab_delete_alert_metric_image",
			"gitlab_list_impersonation_tokens",
			"gitlab_get_impersonation_token",
			"gitlab_create_impersonation_token",
			"gitlab_revoke_impersonation_token",
			"gitlab_create_personal_access_token",
		},
		// notifications.md owns notification settings, award emoji
		// surfaces (issue/mr/snippet/note), and resource events.
		// The events package tools also belong here per the README
		// footnote, but those are handled by the special-case in
		// computeExpectedByDoc.
		"docs/tools/notifications.md": {
			"gitlab_notification_",
			"gitlab_issue_emoji_",
			"gitlab_issue_note_emoji_",
			"gitlab_issue_label_event_",
			"gitlab_issue_milestone_event_",
			"gitlab_issue_state_event_",
			"gitlab_mr_emoji_",
			"gitlab_mr_note_emoji_",
			"gitlab_mr_label_event_",
			"gitlab_mr_milestone_event_",
			"gitlab_mr_state_event_",
			"gitlab_snippet_emoji_",
			"gitlab_snippet_note_emoji_",
		},
		// integrations.md owns the integration/badge/topic/import
		// surface that lives inside gitlab_project and gitlab_group.
		"docs/tools/integrations.md": {
			"gitlab_list_integrations",
			"gitlab_get_integration",
			"gitlab_delete_integration",
			"gitlab_set_jira_integration",
			"gitlab_list_project_badges",
			"gitlab_get_project_badge",
			"gitlab_add_project_badge",
			"gitlab_edit_project_badge",
			"gitlab_delete_project_badge",
			"gitlab_preview_project_badge",
			"gitlab_list_group_badges",
			"gitlab_get_group_badge",
			"gitlab_add_group_badge",
			"gitlab_edit_group_badge",
			"gitlab_delete_group_badge",
			"gitlab_preview_group_badge",
			"gitlab_list_topics",
			"gitlab_get_topic",
			"gitlab_create_topic",
			"gitlab_update_topic",
			"gitlab_delete_topic",
			"gitlab_import_from_github",
			"gitlab_import_from_bitbucket_server",
			"gitlab_import_from_bitbucket_cloud",
			"gitlab_import_github_gists",
			"gitlab_cancel_github_import",
			"gitlab_get_group_datadog_integration",
			"gitlab_set_group_datadog_integration",
			"gitlab_delete_group_datadog_integration",
			// Epic discussion surface ships inside gitlab_group
			// but is documented in integrations per the README.
			"gitlab_list_epic_discussions",
			"gitlab_get_epic_discussion",
			"gitlab_create_epic_discussion",
			"gitlab_add_epic_discussion_note",
			"gitlab_delete_epic_discussion_note",
			"gitlab_update_epic_discussion_note",
		},
		// identity-security.md owns SCIM, member roles, group
		// credentials, SSH certificates, security settings, LDAP,
		// SAML — the "identity" surface spread across gitlab_group,
		// gitlab_project, gitlab_group_scim, and gitlab_member_role
		// groups.
		"docs/tools/identity-security.md": {
			"gitlab_group_scim",
			"gitlab_list_group_scim_identities",
			"gitlab_get_group_scim_identity",
			"gitlab_update_group_scim_identity",
			"gitlab_delete_group_scim_identity",
			"gitlab_member_role",
			"gitlab_list_instance_member_roles",
			"gitlab_list_group_member_roles",
			"gitlab_create_instance_member_role",
			"gitlab_create_group_member_role",
			"gitlab_delete_instance_member_role",
			"gitlab_delete_group_member_role",
			"gitlab_group_credential",
			"gitlab_list_group_personal_access_tokens",
			"gitlab_revoke_group_personal_access_token",
			"gitlab_list_group_ssh_keys",
			"gitlab_delete_group_ssh_key",
			"gitlab_group_ssh_certificate",
			"gitlab_list_group_ssh_certificates",
			"gitlab_create_group_ssh_certificate",
			"gitlab_delete_group_ssh_certificate",
			"gitlab_security_settings",
			"gitlab_get_project_security_settings",
			"gitlab_update_project_secret_push_protection",
			"gitlab_update_group_secret_push_protection",
			"gitlab_group_ldap_link_list",
			"gitlab_group_ldap_link_add",
			"gitlab_group_ldap_link_delete",
			"gitlab_group_ldap_link_delete_for_provider",
			"gitlab_group_ldap_sync",
			"gitlab_group_saml_link_list",
			"gitlab_group_saml_link_get",
			"gitlab_group_saml_link_add",
			"gitlab_group_saml_link_delete",
			"gitlab_group_saml_users_list",
		},
		// analytics-compliance.md owns the group activity analytics,
		// DORA metrics, compliance policy, project aliases, and
		// project statistics surfaces.
		"docs/tools/analytics-compliance.md": {
			"gitlab_get_recently_created_issues_count",
			"gitlab_get_recently_created_mr_count",
			"gitlab_get_recently_added_members_count",
			"gitlab_get_project_dora_metrics",
			"gitlab_get_group_dora_metrics",
			"gitlab_get_compliance_policy_settings",
			"gitlab_update_compliance_policy_settings",
			"gitlab_list_project_aliases",
			"gitlab_get_project_alias",
			"gitlab_create_project_alias",
			"gitlab_delete_project_alias",
			"gitlab_get_project_statistics",
		},
		// epics.md owns the epic core surface (epics, epic notes,
		// epic issues, epic discussions, group epic boards). These
		// tools live in the gitlab_group catalog group per
		// buildGroupActionSpecs — the README's "gitlab_epic" entry
		// refers to a routing label rather than a real group, so
		// the prefix allowlist is the canonical ownership signal.
		"docs/tools/epics.md": {
			"gitlab_epic_",
			"gitlab_list_epic_discussions",
			"gitlab_get_epic_discussion",
			"gitlab_create_epic_discussion",
			"gitlab_add_epic_discussion_note",
			"gitlab_delete_epic_discussion_note",
			"gitlab_update_epic_discussion_note",
			"gitlab_group_epic_board_",
		},
		// merge-requests.md owns the merge-train and external-status-
		// check surfaces (Premium/Ultimate groups) whose owning
		// catalog group has no dedicated meta-tool doc. The README
		// Domains table doesn't list these groups; they live in
		// merge-requests.md by team convention.
		"docs/tools/merge-requests.md": {
			"gitlab_list_project_merge_trains",
			"gitlab_list_merge_request_in_merge_train",
			"gitlab_get_merge_request_on_merge_train",
			"gitlab_add_merge_request_to_merge_train",
			"gitlab_list_project_status_checks",
			"gitlab_list_project_mr_external_status_checks",
			"gitlab_list_project_external_status_checks",
			"gitlab_create_project_external_status_check",
			"gitlab_update_project_external_status_check",
			"gitlab_delete_project_external_status_check",
			"gitlab_retry_failed_external_status_check_for_project_mr",
			"gitlab_set_project_mr_external_status_check_status",
		},
		// admin.md owns the audit-event surface (Premium group) whose
		// owning catalog group `gitlab_audit_event` has no dedicated
		// meta-tool doc. The README Domains table doesn't list this
		// group; audit events live in admin.md by team convention
		// (admin is the natural home for instance-level endpoints).
		"docs/tools/admin.md": {
			"gitlab_list_instance_audit_events",
			"gitlab_get_instance_audit_event",
			"gitlab_list_group_audit_events",
			"gitlab_get_group_audit_event",
			"gitlab_list_project_audit_events",
			"gitlab_get_project_audit_event",
		},
		// packages.md owns the dependency-list surface (Ultimate
		// group) whose owning catalog group `dependencies` has no
		// dedicated meta-tool doc. The README Domains table doesn't
		// list this group; dependencies live in packages.md by team
		// convention (packages is the natural home for inventory /
		// export workflows).
		"docs/tools/packages.md": {
			"gitlab_list_project_dependencies",
			"gitlab_create_dependency_list_export",
			"gitlab_get_dependency_list_export",
			"gitlab_download_dependency_list_export",
		},
	}
}

// buildFirstClaimer returns a map from catalog group ToolName to
// the canonical "docs/tools/<name>.md" path of the first README
// row that claims it. groupExtensions patches in groups the README
// truncated with "etc." or otherwise omitted (e.g. the
// "gitlab_ci_variable" group claimed by ci-cd.md) so the
// first-claimer lookup never falls through to unassigned for those
// groups.
func buildFirstClaimer(mapping *docMapping) map[string]string {
	firstClaimer := make(map[string]string)
	for _, row := range mapping.Rows {
		canonicalDoc := relativeDocPath(row.DocLink)
		for _, g := range expectedGroupsForRow(row) {
			if _, taken := firstClaimer[g]; taken {
				continue
			}
			firstClaimer[g] = canonicalDoc
		}
	}
	for d, groups := range groupExtensions() {
		canonicalDoc := canonicalOverridePath(d)
		for _, g := range groups {
			if _, taken := firstClaimer[g]; taken {
				continue
			}
			firstClaimer[g] = canonicalDoc
		}
	}
	return firstClaimer
}

// prefixRule is one (doc, prefix) tuple from parsePrefixAllowlists.
// Multiple rules can match a single tool; the longest prefix wins.
type prefixRule struct {
	doc    string
	prefix string
}

// buildPrefixRules flattens the per-doc prefix allowlist into a
// single slice, sorted longest-prefix-first so the best match is
// found first during assignTool.
func buildPrefixRules() []prefixRule {
	prefixByDoc := parsePrefixAllowlists()
	docKeys := make([]string, 0, len(prefixByDoc))
	for d := range prefixByDoc {
		docKeys = append(docKeys, d)
	}
	sort.Strings(docKeys)
	var rules []prefixRule
	for _, d := range docKeys {
		prefixes := prefixByDoc[d]
		sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
		for _, p := range prefixes {
			rules = append(rules, prefixRule{d, p})
		}
	}
	return rules
}

// buildOverrideDoc materializes the hardcoded routed-tool overrides
// as a tool→doc map. canonicalOverridePath lifts the basenames to
// the canonical form so the rest of the resolution logic can use
// the map uniformly.
func buildOverrideDoc(mapping *docMapping) map[string]string {
	out := make(map[string]string)
	for d, tools := range mapping.Override {
		canonicalDoc := canonicalOverridePath(d)
		for _, t := range tools {
			out[t] = canonicalDoc
		}
	}
	return out
}

// assignTool returns the canonical doc path for a single catalog
// tool. The lookup order is: hardcoded override (routed tools win)
// → longest-matching prefix rule (shared-group docs) → first-claimer
// of the owning group. Returns ("", false) when none of these
// rules claim the tool; the caller is then responsible for the
// events-package fallback and unassigned tracking.
func assignTool(toolName string, info catalogTool, firstClaimer map[string]string, prefixRules []prefixRule, overrideDoc map[string]string) (string, bool) {
	if doc, ok := overrideDoc[toolName]; ok {
		return doc, true
	}
	var bestDoc string
	var bestLen int
	for _, rule := range prefixRules {
		if !strings.HasPrefix(toolName, rule.prefix) {
			continue
		}
		if len(rule.prefix) > bestLen {
			bestDoc = rule.doc
			bestLen = len(rule.prefix)
		}
	}
	if bestDoc != "" {
		return bestDoc, true
	}
	if doc, ok := firstClaimer[info.Group]; ok {
		return doc, true
	}
	return "", false
}

// canonicalOverridePath converts a hardcoded override key (basename
// like "branch-rules.md") to the canonical "docs/tools/<name>.md"
// form so it matches relativeDocPath's output and the per-doc
// lookup in buildReport can translate to the filesystem-discovered
// path with a single basename map.
func canonicalOverridePath(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "docs/tools/") {
		return name
	}
	return "docs/tools/" + strings.TrimPrefix(name, "./")
}

// isRoutedRow reports whether the README row's "Meta-tool" column
// carries the "(routed)" annotation. Routed rows do not claim the
// listed group wholesale — they only own the tools enumerated in the
// hardcoded override for that doc (e.g. branch-rules.md).
func isRoutedRow(row docMappingRow) bool {
	return strings.Contains(row.MetaToolsCSV, "(routed)")
}

// sortedUnique returns the sorted, deduplicated copy of values.
func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
