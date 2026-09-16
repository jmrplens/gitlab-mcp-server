package modelcorpus

// The licensed mutating partition: cases that create or change something a
// Premium or an Ultimate instance offers and a Free one does not.
//
// The rewrites are the ones the destructive file lists, with one that only
// bites here: the licensed surface has long argument names ("deploy access
// levels", "secret push protection enabled", "base access level", "commit
// message regex") whose English reading is exactly how a person would state
// the request, so most of these texts say the thing rather than the setting.
// "Turn secret push protection on" asks for what the case asks for without
// handing over the field that does it, and where no such phrasing existed the
// case says what the rule should achieve instead of what to set.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see readCases.
func licensedMutatingCases() []Case {
	return []Case{
		{
			ID: "MS-006",
			Prompt: "Check the deployment gate of project `{{ .Facts.project_path }}`, whose git remote is " +
				"`{{ .Facts.remote_url }}`: resolve that remote to the project, list the environments it " +
				"has, look at the protected environment `{{ .Facts.protected_environment_name }}`, list " +
				"the deployments made to it, then approve deployment `{{ .Facts.deployment_id }}`. Do not " +
				"approve anything until the deployment listing has come back.",
			Recipe: RecipeDeploymentApproval,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				standalone("gitlab_discover_project", req("remote_url", fact(FactRemoteURL))),
				step("environment.list", project()),
				step("environment.protected_get", project(),
					req("environment", fact(FactProtectedEnvironment))),
				step("environment.deployment_list", project()),
				step("environment.deployment_approve_or_reject", project(),
					req("deployment_id", fact(FactDeploymentID)),
					req("status", authored())),
			}},
		},
		{
			ID:     "MT-120",
			Prompt: "Point the instance compliance policy settings at namespace `{{ .Facts.group_id }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate, Admin: true},
			key: Key{Steps: []Step{
				step("compliance_policy.update", req("csp_namespace_id", fact(FactGroupID))),
			}},
		},
		{
			// The vulnerability world rather than the plain pipeline one:
			// what an export is made from is a pipeline that ran a security
			// scan, and that is the world a scan leaves behind. The argument
			// is the instance's number for that pipeline, where the two
			// security reads of the same world take the project's, which is
			// why the world publishes each under a fact of its own.
			ID:     "MT-121",
			Prompt: "Create a dependency list export for pipeline `{{ .Facts.pipeline_id }}`.",
			Recipe: RecipeVulnerability,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("dependency.export_create",
					req("pipeline_id", fact(FactPipelineID)),
					opt("export_type", authored())),
			}},
		},
		{
			ID: "MT-126",
			Prompt: "Create the external status check `Eval Gate` on project `{{ .Facts.project_path }}`, " +
				"pointing at `https://example.com/check`.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("external_status_check.create_project", project(),
					req("name", literal("Eval Gate")),
					req("external_url", literal("https://example.com/check"))),
			}},
		},
		{
			ID: "MT-127",
			Prompt: "Mark external status check `{{ .Facts.external_status_check_id }}` as passed for " +
				"merge request `{{ .Facts.merge_request_iid }}` at commit `{{ .Facts.commit_sha }}` in " +
				"project `{{ .Facts.project_path }}`.",
			Recipe: RecipeExternalStatusCheck,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("external_status_check.set_project_mr_status", project(),
					req("merge_request_iid", fact(FactMergeRequestIID)),
					req("sha", fact(FactCommitSHA)),
					req("external_status_check_id", fact(FactExternalStatusCheckID)),
					req("status", authored())),
			}},
		},
		{
			ID: "MT-130",
			Prompt: "Create a Geo secondary site called `{{ .Facts.geo_site_name }}` at " +
				"`{{ .Facts.geo_site_url }}`, and leave it switched off.",
			Recipe: RecipeGeoSiteName,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("geo.create",
					req("name", fact(FactGeoSiteName)),
					req("url", fact(FactGeoSiteURL)),
					opt("enabled", authored())),
			}},
		},
		{
			ID:     "MT-137",
			Prompt: "Create the epic `Evaluation Epic` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_create", fullPath(), req("title", literal("Evaluation Epic"))),
			}},
		},
		{
			ID:     "MT-138",
			Prompt: "Close epic `{{ .Facts.epic_iid }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeEpic,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_update", fullPath(),
					req("epic_iid", fact(FactEpicIID)),
					req("state_event", literal("close"))),
			}},
		},
		{
			ID: "MT-140",
			Prompt: "Assign issue `{{ .Facts.issue_iid }}` of project `{{ .Facts.project_path }}` to epic " +
				"`{{ .Facts.epic_iid }}` in group `{{ .Facts.group_path }}`.",
			Recipe: RecipeEpicIssue,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_issue_assign", fullPath(),
					req("epic_iid", fact(FactEpicIID)),
					req("child_project_path", fact(FactProjectPath)),
					req("child_iid", fact(FactIssueIID))),
			}},
		},
		{
			ID: "MT-142",
			Prompt: "Add the note `Please revise the roadmap` to epic `{{ .Facts.epic_iid }}` in group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeEpic,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_note_create", fullPath(),
					req("epic_iid", fact(FactEpicIID)),
					req("body", literal("Please revise the roadmap"))),
			}},
		},
		{
			ID: "MT-144",
			Prompt: "Link group `{{ .Facts.group_path }}` to the directory provider `ldapmain`, for the " +
				"common name `developers`, at Maintainer level.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("group.ldap_link_add", group(),
					req("provider", literal("ldapmain")),
					req("group_access", authored()),
					opt("cn", literal("developers"))),
			}},
		},
		{
			ID: "MT-146",
			Prompt: "Protect the branch pattern `release/*` for group `{{ .Facts.group_path }}`, letting " +
				"only maintainers merge into it.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.protected_branch_protect", group(),
					req("name", literal("release/*")),
					opt("merge_access_level", authored())),
			}},
		},
		{
			ID: "MT-148",
			Prompt: "Protect the environment `production` of group `{{ .Facts.group_path }}` so that one " +
				"person has to sign a deployment off.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.protected_env_protect", group(),
					req("name", literal("production")),
					req("deploy_access_levels", authored()),
					opt("approval_rules", authored())),
			}},
		},
		{
			ID: "MT-150",
			Prompt: "Link the SAML group `Engineering` to group `{{ .Facts.group_path }}` at Developer " +
				"level.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.saml_link_add", group(),
					req("saml_group_name", literal("Engineering")),
					req("access_level", authored())),
			}},
		},
		{
			ID:     "MT-152",
			Prompt: "Turn secret push protection on for group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("group.security_settings_update", group(),
					req("secret_push_protection_enabled", authored())),
			}},
		},
		{
			ID: "MT-153",
			Prompt: "Create the service account `eval-bot` in the top-level group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			key: Key{Steps: []Step{
				step("group.service_account_create", group(), opt("name", literal("eval-bot"))),
			}},
		},
		{
			ID:     "MT-155",
			Prompt: "Create the SSH certificate `Eval CA` for group `{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.ssh_cert_create", group(),
					req("title", literal("Eval CA")),
					req("key", authored())),
			}},
		},
		{
			ID: "MT-157",
			Prompt: "Create the group wiki page `Evaluation Group Wiki` in group " +
				"`{{ .Facts.group_path }}`.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.wiki_create", group(),
					req("title", literal("Evaluation Group Wiki")),
					req("content", authored())),
			}},
		},
		{
			ID: "MT-160",
			Prompt: "Point the SCIM identity `{{ .Facts.scim_uid }}` of group `{{ .Facts.group_path }}` at " +
				"the outside identifier `external-456`.",
			Recipe: RecipeScimIdentity,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group_scim.update", group(),
					req("uid", fact(FactScimUID)),
					req("extern_uid", literal("external-456"))),
			}},
		},
		{
			ID: "MT-163",
			Prompt: "Create the custom member role `Eval Auditor` in group `{{ .Facts.group_path }}`, on " +
				"top of what a Guest may do.",
			Recipe: RecipeGroup,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("member_role.create_group", group(),
					req("name", literal("Eval Auditor")),
					req("base_access_level", authored())),
			}},
		},
		{
			// The merge train world, which is a project in a group with the
			// train switches on. GitLab keeps merge_trains_enabled only on a
			// project whose namespace carries the licensed feature, so on the
			// mergeable world this case named before, which is a project in a
			// personal namespace, every model was refused for the project
			// having no train at all rather than for anything it did.
			ID: "MT-165",
			Prompt: "Put merge request `{{ .Facts.merge_request_iid }}` on the merge train of project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeTrainEntry,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("merge_train.add", project(), req("merge_request_iid", fact(FactMergeRequestIID))),
			}},
		},
		{
			ID: "MT-167",
			Prompt: "Add a push rule to project `{{ .Facts.project_path }}` that turns away commits which " +
				"carry no signature.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("project.push_rule_add", project(), opt("reject_unsigned_commits", authored())),
			}},
		},
		{
			ID:     "MT-169",
			Prompt: "Turn secret push protection on for project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.security_settings_update", project(),
					req("secret_push_protection_enabled", authored())),
			}},
		},
		{
			ID: "MT-170",
			Prompt: "Create the project alias `{{ .Facts.project_alias_name }}` for project " +
				"`{{ .Facts.project_id }}`.",
			Recipe: RecipeProjectAliasName,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("project_alias.create",
					req("name", fact(FactProjectAliasName)),
					req("project_id", fact(FactProjectID))),
			}},
		},
		{
			ID: "MT-175",
			Prompt: "Create an instance service account called " +
				"`{{ .Facts.service_account_username }}`, signing in as " +
				"`{{ .Facts.service_account_username }}`.",
			Recipe: RecipeServiceAccountName,
			Needs:  Needs{Admin: true},
			key: Key{Steps: []Step{
				step("user.create_service_account",
					req("name", fact(FactServiceAccountUsername)),
					req("username", fact(FactServiceAccountUsername))),
			}},
		},
		{
			ID: "MT-181",
			Prompt: "Create the project service account `eval-project-bot` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeProject,
			key: Key{Steps: []Step{
				step("project.service_account_create", project(),
					opt("name", literal("eval-project-bot"))),
			}},
		},
		{
			ID: "MT-182",
			Prompt: "Rename project service account `{{ .Facts.service_account_id }}` in project " +
				"`{{ .Facts.project_path }}` to `eval-project-bot-v2`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_update", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					opt("name", literal("eval-project-bot-v2"))),
			}},
		},
		{
			ID: "MT-185",
			Prompt: "Create the personal access token `eval-project-bot-token`, with the `api` scope, for " +
				"project service account `{{ .Facts.service_account_id }}` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_pat_create", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					req("name", literal("eval-project-bot-token")),
					req("scopes", literal("api"))),
			}},
		},
		{
			ID: "MT-186",
			Prompt: "Rotate token `{{ .Facts.service_account_token_id }}` of project service account " +
				"`{{ .Facts.service_account_id }}` in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_pat_rotate", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					req("token_id", fact(FactServiceAccountTokenID))),
			}},
		},
		{
			ID: "MT-192",
			Prompt: "Add a push rule to project `{{ .Facts.project_path }}` whose commit message pattern " +
				"is `^EVAL-`. Only add the rule; do not change it or take it away afterwards.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("project.push_rule_add", project(), opt("commit_message_regex", literal("^EVAL-"))),
			}},
		},
		{
			ID: "MT-193",
			Prompt: "Change the push rule of project `{{ .Facts.project_path }}` so that it turns away " +
				"commits which carry no signature.",
			Recipe: RecipePushRule,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("project.push_rule_edit", project(), opt("reject_unsigned_commits", authored())),
			}},
		},
		{
			ID: "MT-194",
			Prompt: "Update the security settings of project `{{ .Facts.project_path }}` so that secret " +
				"push protection is on.",
			Recipe: RecipeProject,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.security_settings_update", project(),
					req("secret_push_protection_enabled", authored())),
			}},
		},
		{
			ID: "MT-195",
			Prompt: "Rename project service account `{{ .Facts.service_account_id }}` in project " +
				"`{{ .Facts.project_path }}` to `eval-project-bot-live`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_update", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					opt("name", literal("eval-project-bot-live"))),
			}},
		},
		{
			ID: "MS-054",
			Prompt: "Exercise the licensed settings of project `{{ .Facts.project_path }}`: read its " +
				"security settings, turn secret push protection on, list its service accounts, then rename " +
				"service account `{{ .Facts.service_account_id }}` to `eval-project-bot-workflow`.",
			Recipe: RecipeProjectServiceAccount,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.security_settings_get", project()),
				step("project.security_settings_update", project(),
					req("secret_push_protection_enabled", authored())),
				step("project.service_account_list", project()),
				step("project.service_account_update", project(),
					req("service_account_id", fact(FactServiceAccountID)),
					opt("name", literal("eval-project-bot-workflow"))),
			}},
		},
	}
}
