package modelcorpus

// The licensed destructive partition: cases that take away something a
// Premium or an Ultimate instance offers.
//
// It is the destructive file's rules on the licensed surface, so the note
// there about the confirmation argument holds here too: no key declares it,
// and whether the model sent it is read from the record.
//
// A handful of the actions here are Free by the catalog's reckoning even
// though the old evaluator filed them as enterprise, because what is licensed
// is the feature around them rather than the call: a project service account
// is a Free action on an instance whose license lets a project have one. The
// tier a case declares is the tier its steps need and nothing else, which is
// why some cases in this file declare none.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see readCases.
func licensedDestructiveCases() []Case {
	return []Case{
		{
			ID: "MS-005",
			Prompt: "Review the outside integrations of project `{{ .Facts.project_path }}`: list its " +
				"webhooks, list its status checks, look at the inbound CI job token allowlist, then take " +
				"project `{{ .Facts.job_token_target_project_id }}` off that allowlist.",
			Recipe: RecipeJobTokenScope,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.hook_list", project()),
				step("external_status_check.list_project", project()),
				step("job.token_scope_list_inbound", project()),
				step("job.token_scope_remove_project", project(),
					req("target_project_id", fact(FactJobTokenTargetProjectID))),
			}},
		},
		{
			ID: "MT-116",
			Prompt: "Force-push remote mirror `{{ .Facts.mirror_id }}` of project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectMirror,
			key: Key{Steps: []Step{
				step("project.mirror_force_push", project(), req("mirror_id", fact(FactMirrorID))),
			}},
		},
		{
			ID: "MT-125",
			Prompt: "Disable two-factor authentication for enterprise user `{{ .Facts.user_id }}` of group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeEnterpriseUser,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("enterprise_user.disable_2fa", group(), req("user_id", fact(FactUserID))),
			}},
		},
		{
			ID: "MT-128",
			Prompt: "Delete external status check `{{ .Facts.external_status_check_id }}` from project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeExternalStatusCheck,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("external_status_check.delete_project", project(),
					req("check_id", fact(FactExternalStatusCheckID))),
			}},
		},
		{
			ID:     "MT-131",
			Prompt: "Delete Geo site `{{ .Facts.geo_site_id }}`.",
			Recipe: RecipeGeoSite,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key:    Key{Steps: []Step{step("geo.delete", req("id", fact(FactGeoSiteID)))}},
		},
		{
			ID: "MT-134",
			Prompt: "Revoke the personal access token `{{ .Facts.group_access_token_id }}` of group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupAccessToken,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("group.credential_revoke_pat", group(),
					req("token_id", fact(FactGroupAccessTokenID))),
			}},
		},
		{
			ID:     "MT-139",
			Prompt: "Delete epic `{{ .Facts.epic_iid }}` from group `{{ .Facts.group_path }}`.",
			Recipe: RecipeEpic,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_delete", fullPath(), req("epic_iid", fact(FactEpicIID))),
			}},
		},
		{
			ID: "MT-141",
			Prompt: "Take issue `{{ .Facts.issue_iid }}` of project `{{ .Facts.project_path }}` off epic " +
				"`{{ .Facts.epic_iid }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeEpicIssue,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_issue_remove", fullPath(),
					req("epic_iid", fact(FactEpicIID)),
					req("child_project_path", fact(FactProjectPath)),
					req("child_iid", fact(FactIssueIID))),
			}},
		},
		{
			ID: "MT-143",
			Prompt: "Delete note `{{ .Facts.epic_note_id }}` from epic `{{ .Facts.epic_iid }}` in group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeEpic,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_note_delete", fullPath(),
					req("epic_iid", fact(FactEpicIID)),
					req("note_id", fact(FactEpicNoteID))),
			}},
		},
		{
			ID: "MT-145",
			Prompt: "Delete the directory link of group `{{ .Facts.group_path }}` for the provider " +
				"`{{ .Facts.ldap_provider }}`.",
			Recipe: RecipeLDAPLink,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("group.ldap_link_delete_for_provider", group(),
					req("provider", fact(FactLDAPProvider))),
			}},
		},
		{
			ID: "MT-147",
			Prompt: "Remove the protection from branch pattern " +
				"`{{ .Facts.protected_branch_name }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupProtectedBranch,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.protected_branch_unprotect", group(),
					req("branch", fact(FactProtectedBranchName))),
			}},
		},
		{
			ID: "MT-149",
			Prompt: "Remove the protection from environment " +
				"`{{ .Facts.protected_environment_name }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupProtectedEnvironment,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.protected_env_unprotect", group(),
					req("environment", fact(FactProtectedEnvironment))),
			}},
		},
		{
			ID: "MT-151",
			Prompt: "Delete the SAML group link `{{ .Facts.saml_group_name }}` from group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeSAMLLink,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.saml_link_delete", group(),
					req("saml_group_name", fact(FactSAMLGroupName))),
			}},
		},
		{
			ID: "MT-154",
			Prompt: "Revoke token `{{ .Facts.service_account_token_id }}` of service account " +
				"`{{ .Facts.service_account_id }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupServiceAccount,
			key: Key{Steps: []Step{
				step("group.service_account_pat_revoke", group(),
					req("service_account_id", fact(FactServiceAccountID)),
					req("token_id", fact(FactServiceAccountTokenID))),
			}},
		},
		{
			ID: "MT-156",
			Prompt: "Delete SSH certificate `{{ .Facts.ssh_certificate_id }}` from group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupSSHCertificate,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.ssh_cert_delete", group(),
					req("certificate_id", fact(FactSSHCertificateID))),
			}},
		},
		{
			ID: "MT-158",
			Prompt: "Delete the group wiki page `{{ .Facts.wiki_slug }}` from group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupWikiPage,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.wiki_delete", group(), req("slug", fact(FactWikiSlug))),
			}},
		},
		{
			ID:     "MT-164",
			Prompt: "Delete the instance member role `{{ .Facts.member_role_id }}`.",
			Recipe: RecipeMemberRole,
			Needs:  Needs{Tier: TierUltimate, Admin: true},
			key: Key{Steps: []Step{
				step("member_role.delete_instance", req("member_role_id", fact(FactMemberRoleID))),
			}},
		},
		{
			ID:     "MT-168",
			Prompt: "Delete the push rule of project `{{ .Facts.project_path }}`.",
			Recipe: RecipePushRule,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("project.push_rule_delete", project())}},
		},
		{
			ID:     "MT-171",
			Prompt: "Delete the project alias `{{ .Facts.project_alias_name }}`.",
			Recipe: RecipeProjectAlias,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("project_alias.delete", req("name", fact(FactProjectAliasName))),
			}},
		},
		{
			ID: "MT-183",
			Prompt: "Delete project service account `{{ .Facts.service_account_id }}` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_delete", project(),
					req("service_account_id", fact(FactServiceAccountID))),
			}},
		},
		{
			ID: "MT-187",
			Prompt: "Revoke token `{{ .Facts.service_account_token_id }}` of project service account " +
				"`{{ .Facts.service_account_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_pat_revoke", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					req("token_id", fact(FactServiceAccountTokenID))),
			}},
		},
		{
			ID:     "MT-196",
			Prompt: "Take the push rule off project `{{ .Facts.project_path }}`.",
			Recipe: RecipePushRule,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("project.push_rule_delete", project())}},
		},
		{
			ID: "MT-197",
			Prompt: "Revoke the group service account token " +
				"`{{ .Facts.service_account_token_id }}` belonging to account " +
				"`{{ .Facts.service_account_id }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupServiceAccount,
			key: Key{Steps: []Step{
				step("group.service_account_pat_revoke", group(),
					req("service_account_id", fact(FactServiceAccountID)),
					req("token_id", fact(FactServiceAccountTokenID))),
			}},
		},
		{
			ID: "MT-198",
			Prompt: "Delete group service account `{{ .Facts.service_account_id }}` in group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroupServiceAccount,
			key: Key{Steps: []Step{
				step("group.service_account_delete", group(),
					req("service_account_id", fact(FactServiceAccountID))),
			}},
		},
		{
			ID: "MS-043",
			Prompt: "Exercise the project service account lifecycle in project " +
				"`{{ .Facts.project_path }}`: create the service account `eval-project-service-account`, " +
				"list the service accounts, rename the one you created to " +
				"`eval-project-service-account-v2`, create the personal access token " +
				"`eval-project-service-token` with the `api` scope, list that account's tokens, rotate the " +
				"token, revoke the rotated one, then delete the service account.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				{
					Action:   "project.service_account_create",
					Args:     []Arg{project(), opt("name", literal("eval-project-service-account"))},
					Produces: []string{"id"},
				},
				step("project.service_account_list", project()),
				step("project.service_account_update", project(),
					req("service_account_id", produced(1, "id")),
					opt("name", literal("eval-project-service-account-v2"))),
				{
					Action: "project.service_account_pat_create",
					Args: []Arg{
						project(),
						req("service_account_id", produced(1, "id")),
						req("name", literal("eval-project-service-token")),
						req("scopes", literal("api")),
					},
					Produces: []string{"id"},
				},
				step("project.service_account_pat_list", project(),
					req("service_account_id", produced(1, "id"))),
				{
					Action: "project.service_account_pat_rotate",
					Args: []Arg{
						project(),
						req("service_account_id", produced(1, "id")),
						req("token_id", produced(4, "id")),
					},
					Produces: []string{"id"},
				},
				step("project.service_account_pat_revoke", project(),
					req("service_account_id", produced(1, "id")),
					req("token_id", produced(6, "id"))),
				step("project.service_account_delete", project(),
					req("service_account_id", produced(1, "id"))),
			}},
		},
		{
			ID: "MS-045",
			Prompt: "Exercise the push rule lifecycle of project `{{ .Facts.project_path }}`: add a rule " +
				"whose commit message pattern is `^EVAL-`, read the rule back, change it to turn away " +
				"commits which carry no signature, then delete the rule.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("project.push_rule_add", project(), opt("commit_message_regex", literal("^EVAL-"))),
				step("project.push_rule_get", project()),
				step("project.push_rule_edit", project(), opt("reject_unsigned_commits", authored())),
				step("project.push_rule_delete", project()),
			}},
		},
		{
			ID: "MS-046",
			Prompt: "Exercise the group service account lifecycle in group `{{ .Facts.group_path }}`: list " +
				"the service accounts, create the service account `eval-group-service-account`, rename the " +
				"one you created to `eval-group-service-account-v2`, create the personal access token " +
				"`eval-group-service-token` with the `api` scope, list that account's tokens, revoke the " +
				"token you created, then delete the service account.",
			Recipe: RecipeGroup,
			key: Key{Steps: []Step{
				step("group.service_account_list", group()),
				{
					Action:   "group.service_account_create",
					Args:     []Arg{group(), opt("name", literal("eval-group-service-account"))},
					Produces: []string{"id"},
				},
				step("group.service_account_update", group(),
					req("service_account_id", produced(2, "id")),
					opt("name", literal("eval-group-service-account-v2"))),
				{
					Action: "group.service_account_pat_create",
					Args: []Arg{
						group(),
						req("service_account_id", produced(2, "id")),
						req("name", literal("eval-group-service-token")),
						req("scopes", literal("api")),
					},
					Produces: []string{"id"},
				},
				step("group.service_account_pat_list", group(),
					req("service_account_id", produced(2, "id"))),
				step("group.service_account_pat_revoke", group(),
					req("service_account_id", produced(2, "id")),
					req("token_id", produced(4, "id"))),
				step("group.service_account_delete", group(),
					req("service_account_id", produced(2, "id"))),
			}},
		},
		{
			ID: "MS-047",
			Prompt: "Exercise epic CRUD in group `{{ .Facts.group_path }}`: create the epic " +
				"`Evaluation Enterprise Epic`, list the epics, read the created one back, rename it to " +
				"`Evaluation Enterprise Epic v2`, then delete it.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action:   "group.epic_create",
					Args:     []Arg{fullPath(), req("title", literal("Evaluation Enterprise Epic"))},
					Produces: []string{"iid"},
				},
				step("group.epic_list", fullPath()),
				step("group.epic_get", fullPath(), req("epic_iid", produced(1, "iid"))),
				step("group.epic_update", fullPath(),
					req("epic_iid", produced(1, "iid")),
					opt("title", literal("Evaluation Enterprise Epic v2"))),
				step("group.epic_delete", fullPath(), req("epic_iid", produced(1, "iid"))),
			}},
		},
		{
			ID: "MS-048",
			Prompt: "Exercise the epic note lifecycle in group `{{ .Facts.group_path }}`: create the epic " +
				"`Evaluation Enterprise Note Epic`, add the note `first enterprise note`, list the epic's " +
				"notes, read the created note back, change it to `updated enterprise note`, delete the " +
				"note, then delete the epic.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action:   "group.epic_create",
					Args:     []Arg{fullPath(), req("title", literal("Evaluation Enterprise Note Epic"))},
					Produces: []string{"iid"},
				},
				{
					Action: "group.epic_note_create",
					Args: []Arg{
						fullPath(),
						req("epic_iid", produced(1, "iid")),
						req("body", literal("first enterprise note")),
					},
					Produces: []string{"id"},
				},
				step("group.epic_note_list", fullPath(), req("epic_iid", produced(1, "iid"))),
				step("group.epic_note_get", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("note_id", produced(2, "id"))),
				step("group.epic_note_update", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("note_id", produced(2, "id")),
					req("body", literal("updated enterprise note"))),
				step("group.epic_note_delete", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("note_id", produced(2, "id"))),
				step("group.epic_delete", fullPath(), req("epic_iid", produced(1, "iid"))),
			}},
		},
		{
			ID: "MS-049",
			Prompt: "Exercise the epic discussion lifecycle in group `{{ .Facts.group_path }}`: create the " +
				"epic `Evaluation Enterprise Discussion Epic`, start the discussion " +
				"`first enterprise discussion`, list the discussions, read the one you started, reply to it " +
				"with `enterprise reply`, change that reply to `enterprise reply updated`, delete the " +
				"reply, then delete the epic.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action: "group.epic_create",
					Args: []Arg{
						fullPath(),
						req("title", literal("Evaluation Enterprise Discussion Epic")),
					},
					Produces: []string{"iid"},
				},
				{
					Action: "group.epic_discussion_create",
					Args: []Arg{
						fullPath(),
						req("epic_iid", produced(1, "iid")),
						req("body", literal("first enterprise discussion")),
					},
					Produces: []string{"id"},
				},
				step("group.epic_discussion_list", fullPath(), req("epic_iid", produced(1, "iid"))),
				step("group.epic_discussion_get", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("discussion_id", produced(2, "id"))),
				{
					Action: "group.epic_discussion_add_note",
					Args: []Arg{
						fullPath(),
						req("epic_iid", produced(1, "iid")),
						req("discussion_id", produced(2, "id")),
						req("body", literal("enterprise reply")),
					},
					Produces: []string{"id"},
				},
				step("group.epic_discussion_update_note", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("note_id", produced(5, "id")),
					req("body", literal("enterprise reply updated"))),
				step("group.epic_discussion_delete_note", fullPath(),
					req("epic_iid", produced(1, "iid")),
					req("note_id", produced(5, "id"))),
				step("group.epic_delete", fullPath(), req("epic_iid", produced(1, "iid"))),
			}},
		},
		{
			ID: "MS-050",
			Prompt: "Exercise epic issue assignment in group `{{ .Facts.group_path }}` using project " +
				"`{{ .Facts.project_path }}`: create the issue `eval-enterprise-epic-child`, create the " +
				"epic `Evaluation Enterprise Issue Epic`, assign the issue to the epic, list the epic's " +
				"issues, take the issue off the epic, delete the issue, then delete the epic.",
			Recipe: RecipeEpicIssue,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action:   "issue.create",
					Args:     []Arg{project(), req("title", literal("eval-enterprise-epic-child"))},
					Produces: []string{"iid"},
				},
				{
					Action:   "group.epic_create",
					Args:     []Arg{fullPath(), req("title", literal("Evaluation Enterprise Issue Epic"))},
					Produces: []string{"iid"},
				},
				step("group.epic_issue_assign", fullPath(),
					req("epic_iid", produced(2, "iid")),
					req("child_project_path", fact(FactProjectPath)),
					req("child_iid", produced(1, "iid"))),
				step("group.epic_issue_list", fullPath(), req("epic_iid", produced(2, "iid"))),
				step("group.epic_issue_remove", fullPath(),
					req("epic_iid", produced(2, "iid")),
					req("child_project_path", fact(FactProjectPath)),
					req("child_iid", produced(1, "iid"))),
				step("issue.delete", project(), req("issue_iid", produced(1, "iid"))),
				step("group.epic_delete", fullPath(), req("epic_iid", produced(2, "iid"))),
			}},
		},
		{
			ID: "MS-051",
			Prompt: "Exercise the group protected branch lifecycle in group `{{ .Facts.group_path }}`: " +
				"protect the branch pattern `eval-enterprise/*` so that only maintainers may push or " +
				"merge, list the group's protected branches, read that rule back, change it to permit " +
				"force pushes, then remove the protection.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.protected_branch_protect", group(),
					req("name", literal("eval-enterprise/*")),
					opt("push_access_level", authored()),
					opt("merge_access_level", authored())),
				step("group.protected_branch_list", group()),
				step("group.protected_branch_get", group(),
					req("branch", literal("eval-enterprise/*"))),
				step("group.protected_branch_update", group(),
					req("branch", literal("eval-enterprise/*")),
					opt("allow_force_push", authored())),
				step("group.protected_branch_unprotect", group(),
					req("branch", literal("eval-enterprise/*"))),
			}},
		},
		{
			ID: "MS-052",
			Prompt: "Exercise the group protected environment lifecycle with a throwaway group: create " +
				"the group `eval-enterprise-protected-env` under `{{ .Facts.group_id }}`, protect its " +
				"environment `staging` so that maintainers may deploy to it, list its protected " +
				"environments, read `staging` back, change it to ask for one sign-off, remove the " +
				"protection, then delete the throwaway group.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action: "group.create",
					Args: []Arg{
						req("name", literal("eval-enterprise-protected-env")),
						req("path", literal("eval-enterprise-protected-env")),
						req("parent_id", fact(FactGroupID)),
					},
					Produces: []string{"id"},
				},
				step("group.protected_env_protect",
					req("group_id", produced(1, "id")),
					req("name", literal("staging")),
					req("deploy_access_levels", authored())),
				step("group.protected_env_list", req("group_id", produced(1, "id"))),
				step("group.protected_env_get",
					req("group_id", produced(1, "id")),
					req("environment", literal("staging"))),
				step("group.protected_env_update",
					req("group_id", produced(1, "id")),
					req("environment", literal("staging")),
					opt("approval_rules", authored())),
				step("group.protected_env_unprotect",
					req("group_id", produced(1, "id")),
					req("environment", literal("staging"))),
				step("group.delete", req("group_id", produced(1, "id"))),
			}},
		},
		{
			ID: "MS-053",
			Prompt: "Exercise the project protected environment lifecycle with a throwaway project: " +
				"create the project `eval-enterprise-protected-env-project` under " +
				"`{{ .Facts.group_id }}`, protect its environment `staging` so that maintainers may deploy " +
				"to it, list its protected environments, read `staging` back, remove the protection, then " +
				"delete the throwaway project.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				{
					Action: "project.create",
					Args: []Arg{
						req("name", literal("eval-enterprise-protected-env-project")),
						opt("namespace_id", fact(FactGroupID)),
					},
					Produces: []string{"id"},
				},
				step("environment.protected_protect",
					req("project_id", produced(1, "id")),
					req("name", literal("staging")),
					req("deploy_access_levels", authored())),
				step("environment.protected_list", req("project_id", produced(1, "id"))),
				step("environment.protected_get",
					req("project_id", produced(1, "id")),
					req("environment", literal("staging"))),
				step("environment.protected_unprotect",
					req("project_id", produced(1, "id")),
					req("environment", literal("staging"))),
				step("project.delete", req("project_id", produced(1, "id"))),
			}},
		},
	}
}
