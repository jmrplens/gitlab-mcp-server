package modelcorpus

// The licensed read partition: cases whose steps read something a Premium or
// an Ultimate instance serves and a Free one does not.
//
// **The tier is a need, not a partition.** Which file a case sits in is
// editorial; what decides whether it runs is [Needs.Tier], and what decides
// whether a step is a read is the catalog. A couple of cases here therefore
// change something (dismissing a vulnerability, scheduling a storage move):
// they sit beside the reads because that is the surface they exercise, and the
// report groups them by what the catalog says rather than by this file.
//
// A case that reads a collection runs in the shared world, because an empty
// collection is a perfectly good answer to "list the Geo sites" and the
// question under test is whether the model reached the right action with the
// right scope. A case that reads one named object runs in a world that has
// that object in it, which is what the licensed recipes are for: R08 either
// builds each of them on a Docker instance or retires the case with the
// no-recipe category, and that judgement belongs to the step with the
// evidence.
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: see readCases.
func licensedReadCases() []Case {
	return []Case{
		{
			ID:     "MT-070",
			Prompt: "List the attestations of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key:    Key{Steps: []Step{step("attestation.list", project())}},
		},
		{
			ID:     "MT-074",
			Prompt: "List the dependency inventory of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key:    Key{Steps: []Step{step("dependency.list", project())}},
		},
		{
			ID: "MT-075",
			Prompt: "Get the deployment frequency DORA metrics for project `{{ .Facts.project_path }}` " +
				"between `2026-01-01` and `2026-01-31`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("dora_metrics.project", project(),
					req("metric", authored()),
					opt("start_date", literal("2026-01-01")),
					opt("end_date", literal("2026-01-31"))),
			}},
		},
		{
			ID:     "MT-076",
			Prompt: "List the enterprise users of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("enterprise_user.list", group())}},
		},
		{
			ID:     "MT-078",
			Prompt: "List the Geo sites of this instance.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("geo.list")}},
		},
		{
			ID:     "MT-079",
			Prompt: "List the SCIM identities of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("group_scim.list", group())}},
		},
		{
			ID:     "MT-084",
			Prompt: "List the custom member roles of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key:    Key{Steps: []Step{step("member_role.list_group", group())}},
		},
		{
			ID:     "MT-085",
			Prompt: "List the merge trains of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("merge_train.list_project", project())}},
		},
		{
			ID: "MT-086",
			Prompt: "Download the model registry file `{{ .Facts.model_file_name }}` from " +
				"`{{ .Facts.model_file_path }}` for model version `{{ .Facts.model_version_id }}` in " +
				"project `{{ .Facts.project_path }}`.",
			Recipe: RecipeModelVersion,
			key: Key{Steps: []Step{
				step("model_registry.download", project(),
					req("model_version_id", fact(FactModelVersionID)),
					req("path", fact(FactModelFilePath)),
					req("filename", fact(FactModelFileName))),
			}},
		},
		{
			ID:     "MT-087",
			Prompt: "List the project aliases on this instance.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("project_alias.list")}},
		},
		{
			ID: "MT-088",
			Prompt: "List the security findings of pipeline `{{ .Facts.pipeline_id }}` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeVulnerability,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("security_finding.list",
					req("project_path", fact(FactProjectPath)),
					req("pipeline_iid", fact(FactPipelineID))),
			}},
		},
		{
			ID:     "MT-089",
			Prompt: "Retrieve every repository storage move of every project on this instance.",
			Recipe: RecipeWorld,
			Needs:  Needs{Admin: true},
			key:    Key{Steps: []Step{step("storage_move.retrieve_all_project")}},
		},
		{
			ID:     "MT-091",
			Prompt: "List the vulnerabilities of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("vulnerability.list", req("project_path", fact(FactProjectPath))),
			}},
		},
		{
			ID: "MT-117",
			Prompt: "Download attestation `{{ .Facts.attestation_iid }}` from project " +
				"`{{ .Facts.project_path }}`. That number is the project-scoped one, not the database " +
				"identifier.",
			Recipe: RecipeAttestation,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("attestation.download", project(), req("attestation_iid", fact(FactAttestationIID))),
			}},
		},
		{
			ID:     "MT-118",
			Prompt: "Read audit event `{{ .Facts.audit_event_id }}` from this instance's audit log.",
			Recipe: RecipeInstanceAuditEvent,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("audit_event.get_instance", req("event_id", fact(FactAuditEventID))),
			}},
		},
		{
			ID: "MT-119",
			Prompt: "List the audit events of project `{{ .Facts.project_path }}` that were recorded during " +
				"January 2026.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("audit_event.list_project", project(),
					opt("created_after", authored()),
					opt("created_before", authored())),
			}},
		},
		{
			ID:     "MT-122",
			Prompt: "Download dependency list export `{{ .Facts.dependency_export_id }}`.",
			Recipe: RecipeDependencyExport,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("dependency.export_download", req("export_id", fact(FactDependencyExportID))),
			}},
		},
		{
			ID: "MT-123",
			Prompt: "Get the DORA lead time metrics for group `{{ .Facts.group_path }}` between " +
				"`2026-01-01` and `2026-01-31`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("dora_metrics.group", group(),
					req("metric", authored()),
					opt("start_date", literal("2026-01-01")),
					opt("end_date", literal("2026-01-31"))),
			}},
		},
		{
			ID:     "MT-124",
			Prompt: "Read enterprise user `{{ .Facts.user_id }}` of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeEnterpriseUser,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("enterprise_user.get", group(), req("user_id", fact(FactUserID))),
			}},
		},
		{
			ID:     "MT-129",
			Prompt: "Read Geo site `{{ .Facts.geo_site_id }}`.",
			Recipe: RecipeGeoSite,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key:    Key{Steps: []Step{step("geo.get", req("id", fact(FactGeoSiteID)))}},
		},
		{
			ID:     "MT-132",
			Prompt: "Count the issues in the analytics of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.analytics_issues_count", req("group_path", fact(FactGroupPath))),
			}},
		},
		{
			ID: "MT-133",
			Prompt: "List the personal access tokens of group `{{ .Facts.group_path }}` that are still " +
				"`active`.",
			Recipe: RecipeGroupAccessToken,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("group.credential_list_pats", group(), opt("state", literal("active"))),
			}},
		},
		{
			ID:     "MT-135",
			Prompt: "List the epic boards of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("group.epic_board_list", group())}},
		},
		{
			ID: "MT-136",
			Prompt: "List the epics of group `{{ .Facts.group_path }}`, including those that live in its " +
				"descendant groups.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_list", fullPath(), opt("include_descendants", authored())),
			}},
		},
		{
			ID:     "MT-159",
			Prompt: "Read the SCIM identity `{{ .Facts.scim_uid }}` of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeScimIdentity,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group_scim.get", group(), req("uid", fact(FactScimUID))),
			}},
		},
		{
			ID:     "MT-161",
			Prompt: "List the iterations of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("issue.iteration_list_group", group())}},
		},
		{
			ID:     "MT-162",
			Prompt: "List the iterations of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("issue.iteration_list_project", project())}},
		},
		{
			ID: "MT-166",
			Prompt: "Read the merge train entry of merge request `{{ .Facts.merge_request_iid }}` in " +
				"project `{{ .Facts.project_path }}`.",
			Recipe: RecipeMergeTrainEntry,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("merge_train.get", project(), req("merge_request_iid", fact(FactMergeRequestIID))),
			}},
		},
		{
			ID: "MT-172",
			Prompt: "Schedule a repository storage move for project `{{ .Facts.project_id }}` onto the " +
				"`default` shard.",
			Recipe: RecipeWorld,
			Needs:  Needs{Admin: true},
			key: Key{Steps: []Step{
				step("storage_move.schedule_project",
					req("project_id", fact(FactProjectID)),
					opt("destination_storage_name", literal("default"))),
			}},
		},
		{
			ID: "MT-173",
			Prompt: "Read the storage move `{{ .Facts.storage_move_id }}` of group " +
				"`{{ .Facts.group_id }}`.",
			Recipe: RecipeStorageMove,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("storage_move.get_group_for_group",
					req("group_id", fact(FactGroupID)),
					req("id", fact(FactStorageMoveID))),
			}},
		},
		{
			ID: "MT-174",
			Prompt: "Schedule a storage move for snippet `{{ .Facts.snippet_id }}` onto the `default` " +
				"shard.",
			Recipe: RecipeSnippet,
			Needs:  Needs{Admin: true},
			key: Key{Steps: []Step{
				step("storage_move.schedule_snippet",
					req("snippet_id", fact(FactSnippetID)),
					opt("destination_storage_name", literal("default"))),
			}},
		},
		{
			ID:     "MT-176",
			Prompt: "Read vulnerability `{{ .Facts.vulnerability_id }}`.",
			Recipe: RecipeVulnerability,
			Needs:  Needs{Tier: TierUltimate},
			key:    Key{Steps: []Step{step("vulnerability.get", req("id", fact(FactVulnerabilityID)))}},
		},
		{
			ID: "MT-177",
			Prompt: "Dismiss vulnerability `{{ .Facts.vulnerability_id }}` as a false positive, with a " +
				"short note saying why.",
			Recipe: RecipeVulnerability,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("vulnerability.dismiss",
					req("id", fact(FactVulnerabilityID)),
					opt("dismissal_reason", authored()),
					opt("comment", authored())),
			}},
		},
		{
			ID: "MT-178",
			Prompt: "Read the security summary of pipeline `{{ .Facts.pipeline_id }}` in project " +
				"`{{ .Facts.project_path }}`.",
			Recipe: RecipeVulnerability,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("vulnerability.pipeline_security_summary",
					req("project_path", fact(FactProjectPath)),
					req("pipeline_iid", fact(FactPipelineID))),
			}},
		},
		{
			ID:     "MT-180",
			Prompt: "List the service accounts of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			key:    Key{Steps: []Step{step("project.service_account_list", project())}},
		},
		{
			ID: "MT-184",
			Prompt: "List the personal access tokens of service account `{{ .Facts.service_account_id }}` " +
				"in project `{{ .Facts.project_path }}`.",
			Recipe: RecipeProjectServiceAccount,
			key: Key{Steps: []Step{
				step("project.service_account_pat_list", project(),
					req("service_account_id", fact(FactServiceAccountID))),
			}},
		},
		{
			ID:     "MT-188",
			Prompt: "Read the security settings of project `{{ .Facts.project_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key:    Key{Steps: []Step{step("project.security_settings_get", project())}},
		},
		{
			ID:     "MT-189",
			Prompt: "List the service accounts of project `{{ .Facts.project_path }}`, `1` to a page.",
			Recipe: RecipeWorld,
			key: Key{Steps: []Step{
				step("project.service_account_list", project(), opt("per_page", literal("1"))),
			}},
		},
		{
			ID:     "MT-190",
			Prompt: "List the protected branch rules of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("group.protected_branch_list", group())}},
		},
		{
			ID:     "MT-191",
			Prompt: "List the protected environments of group `{{ .Facts.group_path }}`.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key:    Key{Steps: []Step{step("group.protected_env_list", group())}},
		},
		{
			ID: "MS-010",
			Prompt: "Build a compliance snapshot for group `{{ .Facts.group_path }}`: list the top-level " +
				"groups, read that group, list its audit events, then read the compliance policy " +
				"configuration.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("group.list", opt("top_level_only", authored())),
				step("group.get", group()),
				step("audit_event.list_group", group()),
				step("compliance_policy.get"),
			}},
		},
		{
			ID: "MS-044",
			Prompt: "Build a licensed read-only inventory for project `{{ .Facts.project_path }}` and " +
				"group `{{ .Facts.group_path }}`: read the project's security settings, list its service " +
				"accounts, list the group's protected branches, list the group's protected environments, " +
				"then list the group's epic boards.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.security_settings_get", project()),
				step("project.service_account_list", project()),
				step("group.protected_branch_list", group()),
				step("group.protected_env_list", group()),
				step("group.epic_board_list", group()),
			}},
		},
		{
			ID: "MS-ENT-DYN-1",
			Prompt: "Give me the security posture of project `{{ .Facts.project_path }}` in one go: say " +
				"whether secret push protection is on, count the project's service accounts, and pull the " +
				"two most recent audit events with what each of them did and when.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.security_settings_get", project()),
				step("project.service_account_list", project()),
				step("audit_event.list_project", project(), opt("per_page", authored())),
			}},
		},
		{
			ID: "MS-ENT-DYN-2",
			Prompt: "First check that project `{{ .Facts.project_path }}` is there by looking it up, then " +
				"pull its deployment-frequency DORA metric for the thirty days ending on `2026-06-04`, day " +
				"by day. That call takes an explicit range, so work both bounds out yourself as " +
				"YYYY-MM-DD.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("project.get", project()),
				step("dora_metrics.project", project(),
					req("metric", authored()),
					req("start_date", authored()),
					req("end_date", authored()),
					opt("interval", authored())),
			}},
		},
		{
			ID: "MS-ENT-DYN-3",
			Prompt: "For group `{{ .Facts.group_path }}`, give me a quick governance view: how many custom " +
				"member roles this instance defines in all, which enterprise users of the group are still " +
				"there, and the most recent audit event of the group. On self-managed GitLab ask for the " +
				"instance-wide member roles; the group-level ones are deprecated there.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("member_role.list_instance"),
				step("enterprise_user.list", group(), opt("active", authored())),
				step("audit_event.list_group", group(), opt("per_page", authored())),
			}},
		},
		{
			ID: "MS-ENT-DYN-4",
			Prompt: "For project `{{ .Facts.project_path }}`: first count the critical and high " +
				"vulnerabilities through the call made for counting rather than through a paginated list, " +
				"then list the project's vulnerabilities `5` to a page.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierUltimate},
			key: Key{Steps: []Step{
				step("vulnerability.severity_count", req("project_path", fact(FactProjectPath))),
				step("vulnerability.list",
					req("project_path", fact(FactProjectPath)),
					opt("first", literal("5"))),
			}},
		},
		{
			ID: "MS-ENT-DYN-5",
			Prompt: "List every project alias on this GitLab instance, then read the alias " +
				"`{{ .Facts.project_alias_name }}` in full. Report it and the identifier of the project it " +
				"points at.",
			Recipe: RecipeProjectAlias,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("project_alias.list"),
				step("project_alias.get", req("name", fact(FactProjectAliasName))),
			}},
		},
		{
			ID: "MS-ENT-DYN-6",
			Prompt: "In project `{{ .Facts.project_path }}`, fetch the audit events recorded during " +
				"January 2026, bounded by `2026-01-01T00:00:00Z` and `2026-02-01T00:00:00Z`, `50` to a " +
				"page.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("audit_event.list_project", project(),
					opt("created_after", literal("2026-01-01T00:00:00Z")),
					opt("created_before", literal("2026-02-01T00:00:00Z")),
					opt("per_page", literal("50"))),
			}},
		},
		{
			ID: "MS-ENT-DYN-7",
			Prompt: "For group `{{ .Facts.group_path }}`: list its open epics `5` to a page, then fetch " +
				"its iterations whatever condition they are in.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium},
			key: Key{Steps: []Step{
				step("group.epic_list", fullPath(), opt("state", authored()), opt("first", literal("5"))),
				step("issue.iteration_list_group", group()),
			}},
		},
		{
			ID: "MS-ENT-DYN-8",
			Prompt: "List the Geo sites configured on this GitLab instance, explaining the answer either " +
				"way in case Geo is not set up here, then list every snippet repository storage move " +
				"recorded on the instance.",
			Recipe: RecipeWorld,
			Needs:  Needs{Tier: TierPremium, Admin: true},
			key: Key{Steps: []Step{
				step("geo.list"),
				step("storage_move.retrieve_all_snippet"),
			}},
		},
	}
}
