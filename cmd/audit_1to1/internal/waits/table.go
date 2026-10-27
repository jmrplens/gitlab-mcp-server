// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package waits

import "slices"

// appStatisticsCounts are the eleven fields client-go's ApplicationStatistics
// declares as int64, every one of which GitLab renders as a delimited string.
var appStatisticsCounts = []string{
	"Forks", "Issues", "MergeRequests", "Notes", "Snippets", "SSHKeys",
	"Milestones", "Users", "Groups", "Projects", "ActiveUsers",
}

// epicKeysUnmodeled are the keys GET /groups/:id/epics sends that client-go's
// Epic does not model, which epics.epicAPI decodes beside it.
var epicKeysUnmodeled = []string{
	"_links", "color", "due_date_from_inherited_source", "end_date", "imported",
	"imported_from", "parent_iid", "references", "start_date_from_inherited_source",
	"text_color", "web_edit_url", "work_item_id",
}

// featureFlagScopes are the eight SetFeatureFlagOptions fields tagged without
// omitempty that the flag's scope is chosen among.
var featureFlagScopes = []string{"Key", "FeatureGroup", "User", "Group", "Namespace", "Project", "Repository", "Force"}

// jobKeysUnmodeled are the keys a job answers with that client-go's Job, and
// the JobRunner it holds, do not model, which jobs.jobAPI decodes beside it.
var jobKeysUnmodeled = []string{
	"archived", "runner_manager",
	"runner.ip_address", "runner.paused", "runner.runner_type", "runner.online", "runner.status",
}

// jobMeasures are the three float64 measures of client-go's Job, which read
// a null as 0 and jobs.jobAPI decodes as pointers in their place.
var jobMeasures = []string{"Duration", "QueuedDuration", "Coverage"}

// epicBoardKeysUnmodeled are the keys an epic board answers with that
// client-go's GroupEpicBoard, its LabelDetails and its BoardList do not
// model, which groupepicboards.groupEpicBoardAPI decodes beside it.
var epicBoardKeysUnmodeled = []string{
	"hide_backlog_list", "hide_closed_list",
	"labels.title", "labels.group_id", "labels.project_id", "labels.template", "labels.created_at", "labels.updated_at",
	"lists.list_type", "lists.collapsed",
}

// pagedWithoutOptions are the client-go methods of a paginated list that take
// no options struct, which toolutil.PaginationRequestOption pages.
var pagedWithoutOptions = []string{
	"Attestations.ListAttestations",
	"Commits.ListMergeRequestsByCommit",
	"Issues.GetParticipants",
	"MergeRequestContextCommits.ListMergeRequestContextCommits",
	"MergeRequests.GetMergeRequestDependencies",
	"MergeRequests.GetMergeRequestParticipants",
	"MergeRequests.GetMergeRequestReviewers",
	"MergeRequests.ListMergeRequestPipelines",
	"PagesDomains.ListAllPagesDomains",
	"ProjectAliases.ListProjectAliases",
	"ResourceGroup.GetAllResourceGroupsForAProject",
	"ResourceGroup.ListUpcomingJobsForASpecificResourceGroup",
	"SystemHooks.ListHooks",
	"Users.ListEmails",
	"Users.ListGPGKeys",
	"Users.ListGPGKeysForUser",
}

// upstreamWaits are the workarounds that wait on client-go, grouped by the
// register row each waits on.
var upstreamWaits = []Entry{
	{
		Row: 6, Category: CategoryUpstream,
		Sites:      []string{"features.Set", "fixture.setFeature"},
		Replaces:   []string{"Features.SetFeatureFlag"},
		Predicates: writesAlwaysJSON("SetFeatureFlagOptions", featureFlagScopes...),
		Reason:     "SetFeatureFlagOptions tags none of the flag's scope fields omitempty, so SetFeatureFlag sends key, feature_group and user as empty strings and GitLab refuses every call as naming mutually exclusive scopes; features.Set and the e2e fixture post the body themselves",
	},
	{
		Row: 7, Category: CategoryUpstream,
		Sites:      []string{"appstatistics.fetchStatistics"},
		Replaces:   []string{"ApplicationStatistics"},
		Predicates: fieldKinds("ApplicationStatistics", "int64", appStatisticsCounts...),
		Reason:     "appstatistics.Get issues a raw GET application/statistics: client-go's ApplicationStatistics declares int64 fields and GitLab renders every count through number_with_delimiter, a string grouped in the caller's language (\"1,234\", \"1.234\", \"1 234\"), so the wrapper cannot decode a successful response",
	},
	{
		Row: 25, Category: CategoryUpstream,
		Sites:      []string{"projects.doProjectRequest"},
		Replaces:   []string{"Projects.CreateProjectForkRelation"},
		Predicates: []Predicate{DecodesInto{Method: "Projects.CreateProjectForkRelation", Type: "*ProjectForkRelation"}},
		Reason:     "the fork relation create issues the same POST itself because the wrapper declares ProjectForkRelation, a five-field pair GitLab's response (a Project) fills none of",
	},
	{
		Row: 26, Category: CategoryUpstream,
		Sites:    []string{"invites.postInvitation"},
		Replaces: []string{"Invites.GroupInvites", "Invites.ProjectInvites"},
		Predicates: []Predicate{
			LacksKey{Type: "InvitesOptions", Key: "invite_source"},
			LacksKey{Type: "InvitesOptions", Key: "member_role_id"},
			LacksKey{Type: "InvitesResult", Key: "queued_users"},
		},
		Reason: "InvitesOptions models neither invite_source nor member_role_id, and InvitesResult models no queued_users, so both invitations post their own body and decode their own answer",
	},
	{
		Row: 28, Category: CategoryUpstream,
		Sites:    []string{"epics.rawListEpics"},
		Replaces: []string{"Epics"},
		Predicates: slices.Concat(
			lacksKeys("Epic", epicKeysUnmodeled...),
			lacksKeys("Epic", "author.locked", "author.public_email"),
			[]Predicate{FieldKind{Type: "Epic", Field: "Labels", Kind: "[]string"}},
		),
		Reason: "epics.rawListEpics issues raw GETs for groups/:id/epics and .../:epic_iid/epics: the wrapper's Epic type omits twelve documented response fields, its EpicAuthor the locked and public_email the author carries, and it types labels []string, so the documented with_labels_details parameter makes the response undecodable. The other four epic actions take the Work Items GraphQL API, which the interface's own upstream doc comment directs to",
	},
	{
		Row: 34, Category: CategoryUpstream,
		Sites:    []string{"projectimportexport.rawGetImportStatus", "projectimportexport.rawImportFromFile"},
		Replaces: []string{"ProjectImportExport.ImportStatus", "ProjectImportExport.ImportFromFile"},
		Predicates: []Predicate{
			TagName{Type: "ImportStatus", Field: "CreateAt", Encoder: EncoderJSON, Want: "created_at"},
			LacksKey{Type: "ImportStatus", Key: "failed_relations"},
			LacksKey{Type: "ImportStatus", Key: "stats"},
		},
		Reason: "ImportStatus tags its timestamp create_at where GitLab sends created_at, so it never decodes, and models neither failed_relations nor stats; both import status routes are read through a decode of their own",
	},
	{
		Row: 34, Category: CategoryUpstream,
		Sites:      []string{"pipelineschedules.rawGetSchedule"},
		Replaces:   []string{"PipelineSchedules.GetPipelineSchedule"},
		Predicates: []Predicate{LacksKey{Type: "PipelineSchedule", Key: "variables.raw"}},
		Reason:     "PipelineVariable has no raw, which GitLab sends on each variable of a schedule, so the schedule read decodes its variables itself",
	},
	{
		Row: 34, Category: CategoryUpstream,
		Sites:      []string{"repository.withLastCommitParam"},
		Predicates: []Predicate{LacksKey{Type: "ListTreeOptions", Key: "with_last_commit"}},
		Reason:     "ListTreeOptions has no with_last_commit, the parameter GitLab 19.3 added to the tree route, so repository.tree adds it to the query client-go encoded",
	},
	{
		Row: 46, Category: CategoryUpstream,
		Sites:      []string{"mergerequests.postCancelAutoMerge"},
		Predicates: []Predicate{NoMethodFor{Route: "POST /projects/:/merge_requests/:/cancel_auto_merge"}},
		Reason:     "client-go has no method for cancel_auto_merge, the route GitLab 19.5 added (gitlab-org/api/client-go!3101 adds MergeRequests.CancelAutoMerge), so the cancel posts to it itself on an instance that serves it",
	},
	{
		Row: 52, Category: CategoryUpstream,
		Unseen:     []string{"protectedpackages.Update"},
		Predicates: writesAlwaysJSON("UpdatePackageProtectionRulesOptions", "PackageNamePattern", "PackageType"),
		Reason:     "UpdatePackageProtectionRulesOptions tags PackageNamePattern and PackageType without omitempty, so a field the caller did not name reaches GitLab as null and is read as blank; the update's 422 hint tells the caller to send both",
	},
	{
		Row: 58, Category: CategoryUpstream,
		Sites:      []string{"projects.doProjectRequest"},
		Replaces:   []string{"Projects.AddProjectHook", "Projects.EditProjectHook", "Projects.GetProjectHook", "Projects.ListProjectHooks"},
		Predicates: []Predicate{LacksKey{Type: "ProjectHook", Key: "duo_flow_callback_enabled"}},
		Reason:     "ProjectHook has no duo_flow_callback_enabled, which GitLab 19.4 added to the hook entity, so the four project hook actions decode the hook into projects.projectHookAPI",
	},
	{
		Row: 65, Category: CategoryUpstream,
		Sites: []string{"grouprelationsexport.getRelationStatus"},
		Predicates: []Predicate{
			LacksKey{Type: "GroupRelationStatus", Key: "total_objects_count"},
			DecodesInto{Method: "GroupRelationsExport.ListExportStatus", Type: "[]*GroupRelationStatus"},
		},
		Reason: "with relation set, the export status route answers one object, and ListExportStatus decodes every answer into a slice, which refuses it; the relation's status is read through a GET of its own, which also reads the total_objects_count GroupRelationStatus does not model",
	},
	{
		Row: 82, Category: CategoryUpstream,
		Sites:      []string{"fixture.createFineGrainedToken"},
		Predicates: []Predicate{LacksKey{Type: "CreatePersonalAccessTokenForCurrentUserOptions", Key: "granular_scopes"}},
		Reason:     "no client-go create option carries granular_scopes, so the e2e fixture builds the fine-grained token creation through NewRequest",
	},
	{
		Row: 95, Category: CategoryUpstream,
		Sites:      []string{"groupmilestones.includeDescendants"},
		Predicates: []Predicate{TagName{Type: "ListGroupMilestonesOptions", Field: "IncludeDescendents", Encoder: EncoderQuery, Want: "include_descendants"}},
		Reason:     "ListGroupMilestonesOptions spells the parameter include_descendents, a key GitLab ignores, so the group milestone list adds include_descendants to the query itself",
	},
	{
		Row: 98, Category: CategoryUpstream,
		Sites:    []string{"groupboards.newRawRequest"},
		Replaces: []string{"GroupIssueBoards.UpdateIssueBoard"},
		Predicates: []Predicate{
			LacksKey{Type: "UpdateGroupIssueBoardOptions", Key: "hide_backlog_list"},
			LacksKey{Type: "UpdateGroupIssueBoardOptions", Key: "hide_closed_list"},
		},
		Reason: "UpdateGroupIssueBoardOptions models neither list switch the group board update takes, so group.group_board_update sends the PUT itself in a body that embeds the options",
	},
	{
		Row: 115, Category: CategoryUpstream,
		Sites:      []string{"modelregistry.downloadFromPackageRoot"},
		Predicates: []Predicate{NoMethodFor{Route: "GET /projects/:/packages/ml_models/:/files/:"}},
		Reason:     "DownloadMachineLearningModelPackage always sends the file's path segment, so a file at the package root is fetched through a GET of the handler's own",
	},
	{
		Row: 119, Category: CategoryUpstream,
		Sites: []string{"toolutil.ApplyWorkItemWidgets"},
		Predicates: []Predicate{
			FieldKind{Type: "UpdateWorkItemOptions", Field: "MilestoneID", Kind: "*int64"},
			FieldKind{Type: "UpdateWorkItemOptions", Field: "ParentID", Kind: "*int64"},
			FieldKind{Type: "UpdateWorkItemOptions", Field: "IterationID", Kind: "*int64"},
			FieldKind{Type: "UpdateWorkItemOptions", Field: "StartDate", Kind: "*ISOTime"},
			FieldKind{Type: "UpdateWorkItemOptions", Field: "DueDate", Kind: "*ISOTime"},
			FieldKind{Type: "UpdateWorkItemOptions", Field: "HealthStatus", Kind: "*string"},
		},
		Reason: "UpdateWorkItemOptions wraps any reference, 0 included, into a global ID and omits an unset date or health status, so it cannot ask for a removal; the request option writes null into the mutation input for each one asked for",
	},
	{
		Row: 120, Category: CategoryUpstream,
		Sites:      []string{"auditevents.entityFilter"},
		Predicates: lacksKeys("ListAuditEventsOptions", "entity_type", "entity_id"),
		Reason:     "ListAuditEventsOptions models neither entity filter the instance list takes, so audit_event.list_instance adds entity_type and entity_id to the query through a request option",
	},
	{
		Row: 121, Category: CategoryUpstream,
		Sites:      []string{"releases.Update"},
		Predicates: writesAlwaysJSON("UpdateReleaseOptions", "Name", "Description"),
		Reason:     "UpdateReleaseOptions tags the title and the notes without omitempty, so a partial update sends both as null; release.update takes the two keys out of the body whenever the caller did not set them",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"members.Edit"},
		Predicates: []Predicate{NotNullable{Type: "EditProjectMemberOptions", Field: "MemberRoleID"}},
		Reason:     "EditProjectMemberOptions tags member_role_id omitempty, so the project member edit writes the null that removes the custom role into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"groupmembers.EditMember"},
		Predicates: []Predicate{NotNullable{Type: "EditGroupMemberOptions", Field: "MemberRoleID"}},
		Reason:     "EditGroupMemberOptions tags member_role_id omitempty, so the group member edit writes the null that removes the custom role into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"compliancepolicy.Update"},
		Predicates: []Predicate{NotNullable{Type: "UpdateAdminCompliancePolicySettingsOptions", Field: "CSPNamespaceID"}},
		Reason:     "UpdateAdminCompliancePolicySettingsOptions tags csp_namespace_id omitempty, so the settings update writes the null that clears the namespace into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites: []string{"projects.Update"},
		Predicates: []Predicate{
			NotNullable{Type: "EditProjectOptions", Field: "MaxArtifactsSize"},
			NotNullable{Type: "EditProjectOptions", Field: "CIDeletePipelinesInSeconds"},
		},
		Reason: "EditProjectOptions tags max_artifacts_size and ci_delete_pipelines_in_seconds omitempty, so the project update writes the null that returns each to the instance default into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"pages.UpdatePages"},
		Predicates: []Predicate{NotNullable{Type: "UpdatePagesOptions", Field: "PagesPrimaryDomain"}},
		Reason:     "UpdatePagesOptions tags pages_primary_domain omitempty, so the Pages update writes the null that clears the primary domain into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"broadcastmessages.Update"},
		Predicates: []Predicate{NoEmptyList{Type: "UpdateBroadcastMessageOptions", Field: "TargetAccessLevels"}},
		Reason:     "UpdateBroadcastMessageOptions tags target_access_levels omitempty on a slice, so the broadcast message update writes the empty list that targets every role into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites:      []string{"mergerequests.Update"},
		Predicates: []Predicate{NotNullable{Type: "UpdateMergeRequestOptions", Field: "MergeAfter"}},
		Reason:     "UpdateMergeRequestOptions tags merge_after omitempty, so the merge request update writes the null that clears it into the body",
	},
	{
		Row: 122, Category: CategoryUpstream,
		Sites: []string{"releaselinks.Update"},
		Predicates: []Predicate{
			NotNullable{Type: "UpdateReleaseLinkOptions", Field: "FilePath"},
			NotNullable{Type: "UpdateReleaseLinkOptions", Field: "DirectAssetPath"},
		},
		Reason: "UpdateReleaseLinkOptions tags filepath and direct_asset_path omitempty, so the release link update writes the null that clears each into the body",
	},
	{
		Row: 125, Category: CategoryUpstream,
		Sites: []string{"toolutil.Int64UserIDFilter"},
		Predicates: []Predicate{
			FieldKind{Type: "GetIssuesStatisticsOptions", Field: "AssigneeID", Kind: "*int64"},
			FieldKind{Type: "GetGroupIssuesStatisticsOptions", Field: "AssigneeID", Kind: "*int64"},
			FieldKind{Type: "GetProjectIssuesStatisticsOptions", Field: "AssigneeID", Kind: "*int64"},
		},
		Reason: "the issue statistics options type assignee_id *int64, which cannot carry None or Any, so the three statistics actions write either literal into the query through a request option",
	},
	{
		Row: 126, Category: CategoryUpstream,
		Sites: []string{"toolutil.TimestampParam"},
		Predicates: []Predicate{
			FieldKind{Type: "AddSSHKeyOptions", Field: "ExpiresAt", Kind: "*ISOTime"},
			FieldKind{Type: "ListAllSnippetsOptions", Field: "CreatedAfter", Kind: "*ISOTime"},
			FieldKind{Type: "ListAllSnippetsOptions", Field: "CreatedBefore", Kind: "*ISOTime"},
			FieldKind{Type: "AddChangelogOptions", Field: "Date", Kind: "*ISOTime"},
			FieldKind{Type: "GenerateChangelogDataOptions", Field: "Date", Kind: "*ISOTime"},
		},
		Reason: "five option fields carry a parameter GitLab declares DateTime as a date, which GitLab reads as midnight UTC, so the SSH key adds, the snippet list and the two changelog actions leave the field unset and write the instant through a request option",
	},
	{
		Row: 132, Category: CategoryUpstream,
		Sites:      []string{"search.codeSearchParams"},
		Predicates: lacksKeys("SearchOptions", "num_context_lines", "regex"),
		Reason:     "SearchOptions models neither num_context_lines nor regex, so search.code adds both to the query through a request option, each only when set",
	},
	{
		Row: 137, Category: CategoryUpstream,
		Sites:      []string{"jobs.includeRetried"},
		Predicates: []Predicate{LacksKey{Type: "ListPipelineTriggerJobsOptions", Key: "include_retried"}},
		Reason:     "ListPipelineTriggerJobsOptions models no include_retried, which the trigger job routes read, so job.list_bridges adds it to the trigger_jobs query through a request option, only when the caller asked",
	},
	{
		Row: 149, Category: CategoryUpstream,
		Sites:      []string{"deployments.rawGetDeployment", "deployments.rawListDeployments"},
		Replaces:   []string{"Deployments.GetProjectDeployment", "Deployments.ListProjectDeployments"},
		Predicates: lacksKeys("Deployment", "approvals", "approval_summary", "pending_approval_count", "deployable.project"),
		Reason:     "Deployment models none of approvals, approval_summary and pending_approval_count, and DeploymentDeployable no project, so the deployment get and list decode into deployments.deploymentAPI",
	},
	{
		Row: 149, Category: CategoryUpstream,
		Sites:      []string{"jobs.rawListJobs", "jobs.rawGetJob"},
		Replaces:   []string{"Jobs.ListProjectJobs"},
		Predicates: slices.Concat(lacksKeys("Job", jobKeysUnmodeled...), fieldKinds("Job", "float64", jobMeasures...)),
		Reason:     "Job models neither archived nor runner_manager, its JobRunner none of ip_address, paused, runner_type, online and status, and it types the three measures float64, which read a null as 0 (row 116), so the job lists and reads decode into jobs.jobAPI",
	},
	{
		Row: 149, Category: CategoryUpstream,
		Sites: []string{"mrapprovals.rawListApprovalRules", "mrapprovals.rawApprovalState", "mrapprovals.rawMutateApprovalRule"},
		Replaces: []string{
			"MergeRequestApprovals.GetApprovalRules", "MergeRequestApprovals.GetApprovalState",
			"MergeRequestApprovals.CreateApprovalRule", "MergeRequestApprovals.UpdateApprovalRule",
		},
		Predicates: []Predicate{
			LacksKey{Type: "MergeRequestApprovalRule", Key: "overridden"},
			LacksKey{Type: "MergeRequestApprovalState", Key: "rules.overridden"},
		},
		Reason: "MergeRequestApprovalRule has no overridden, so the approval rule list, create and update and the approval state decode into mrapprovals' own rule",
	},
	{
		Row: 149, Category: CategoryUpstream,
		Sites:      []string{"commits.rawGetGPGSignature"},
		Replaces:   []string{"Commits.GetGPGSignature"},
		Predicates: lacksKeys("GPGSignature", "signature_type", "commit_source", "key", "x509_certificate"),
		Reason:     "GPGSignature models the PGP signature alone, so the commit signature decodes the SSH and X.509 variants and the signature's type and source into commits.gpgSignatureAPI",
	},
	{
		Row: 149, Category: CategoryUpstream,
		Sites:      []string{"groupepicboards.rawListBoards", "groupepicboards.rawGetBoard"},
		Replaces:   []string{"GroupEpicBoards"},
		Predicates: lacksKeys("GroupEpicBoard", epicBoardKeysUnmodeled...),
		Reason:     "GroupEpicBoard models neither hide_backlog_list nor hide_closed_list, the LabelDetails of its labels none of title, group_id, project_id, template, created_at and updated_at, and the BoardList of its lists neither list_type nor collapsed, so groupepicboards issues raw GETs for groups/:id/epic_boards and .../:board_id and decodes the documented shape",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites:      []string{"orbit.callMCP"},
		Predicates: []Predicate{NoMethodFor{Route: "POST /orbit/mcp"}},
		Reason:     "client-go has no method for POST /orbit/mcp, the Orbit MCP endpoint orbit.tools, orbit.dsl and the llm schema read since GitLab.com removed the routes Orbit.GetTools, Orbit.GetDsl and Orbit.GetSchema send (rows 145 to 147), so the actions send the JSON-RPC request themselves",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites:      []string{"fixture.listGroupHookEvents"},
		Predicates: []Predicate{NoMethodFor{Route: "GET /groups/:/hooks/:/events"}},
		Reason:     "client-go has no method that lists a group hook's events, so the e2e fixture reads them through NewRequest",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites: []string{"common.uploadModelFile"},
		Predicates: []Predicate{
			NoMethodFor{Route: "PUT /projects/:/packages/ml_models/:/files/:"},
			NoMethodFor{Route: "PUT /projects/:/packages/ml_models/:/files/:/:"},
		},
		Reason: "client-go has no method that uploads a file into a model version's package, so the model registry scenario puts the file through NewRequest",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites:      []string{"toolutil.PaginationRequestOption"},
		Predicates: takesNoOptions(pagedWithoutOptions...),
		Reason:     "sixteen client-go methods of a paginated list take no options struct, so every action that lists through one writes page and per_page into the query through a request option, without which it could only ever ask for the first page",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites:      []string{"uploads.listQueryParameters"},
		Predicates: []Predicate{TakesNoOptions{Method: "ProjectMarkdownUploads.ListProjectMarkdownUploads"}},
		Reason:     "ListProjectMarkdownUploads takes no options, so the upload list writes its paging and ordering into the query through a request option",
	},
	{
		Row: 150, Category: CategoryUpstream,
		Sites:      []string{"groupsshcerts.listQueryParameters"},
		Predicates: []Predicate{TakesNoOptions{Method: "GroupSSHCertificates.ListGroupSSHCertificates"}},
		Reason:     "ListGroupSSHCertificates takes no options, so the certificate list writes its paging into the query through a request option",
	},
}

// rawReasons are the raw requests and request options that wait on nothing
// upstream, each for the reason its category names.
var rawReasons = []Entry{
	{
		Category: CategoryGenericSlugDispatch,
		Sites:    []string{"integrations.GetGroupIntegration", "integrations.SetGroupIntegration", "integrations.DeleteGroupIntegration", "integrations.SetIntegration"},
		Replaces: []string{
			"Integrations.DeleteGroupMattermostIntegration", "Integrations.DeleteGroupMattermostSlashCommandsIntegration",
			"Integrations.DisableGroupHarbor", "Integrations.DisableGroupJira", "Integrations.DisableGroupMicrosoftTeamsNotifications",
			"Integrations.DisableGroupSlack", "Integrations.DisableGroupWebexTeams", "Integrations.DisableProjectGoogleChat",
			"Integrations.GetGroupDiscordSettings", "Integrations.GetGroupGoogleChatSettings", "Integrations.GetGroupHarborSettings",
			"Integrations.GetGroupJiraSettings", "Integrations.GetGroupMatrixSettings", "Integrations.GetGroupMattermostIntegration",
			"Integrations.GetGroupMattermostSettings", "Integrations.GetGroupMattermostSlashCommandsIntegration",
			"Integrations.GetGroupMicrosoftTeamsNotifications", "Integrations.GetGroupSlackSettings", "Integrations.GetGroupTelegramSettings",
			"Integrations.GetGroupWebexTeamsSettings", "Integrations.GetProjectGoogleChatSettings", "Integrations.SetGroupMattermostIntegration",
			"Integrations.SetGroupMattermostSlashCommandsIntegration", "Integrations.SetGroupMicrosoftTeamsNotifications",
			"Integrations.SetGroupSlackSettings", "Integrations.SetGroupWebexTeamsSettings", "Integrations.SetProjectGoogleChatSettings",
			"Integrations.SetUpGroupHarbor", "Integrations.SetUpGroupJira",
			"Services.GetSlackApplication", "Services.SetCustomIssueTrackerService", "Services.SetDataDogService", "Services.SetDiscordService",
			"Services.SetDroneCIService", "Services.SetEmailsOnPushService", "Services.SetExternalWikiService", "Services.SetGithubService",
			"Services.SetHarborService", "Services.SetJenkinsCIService", "Services.SetMatrixService", "Services.SetMattermostService",
			"Services.SetMattermostSlashCommandsService", "Services.SetMicrosoftTeamsService", "Services.SetPipelinesEmailService",
			"Services.SetRedmineService", "Services.SetSlackApplication", "Services.SetSlackService", "Services.SetSlackSlashCommandsService",
			"Services.SetTelegramService", "Services.SetYouTrackService",
		},
		Reason: "the project integration set and the four group integration actions take the slug from the input and send the raw GET, PUT or DELETE for it, which covers every integration client-go models one method per slug, and the ones it does not",
	},
	{
		Category: CategoryStreamedDownload,
		Sites:    []string{"packages.newDownloadRequest"},
		Replaces: []string{"GenericPackages.DownloadPackageFile"},
		Reason:   "package.download streams the file to the caller under the download ceiling, which DownloadPackageFile, reading the body whole, cannot",
	},
	{
		Category: CategoryURLBuilt,
		Unseen:   []string{"repository.Archive"},
		Replaces: []string{"Repositories.Archive"},
		Reason:   "repository.archive returns the archive's download URL for the caller to fetch and sends nothing",
	},
	{
		Category: CategoryFixtureProtocol,
		Sites:    []string{"fixture.publishNpmPackage"},
		Reason:   "the e2e fixture publishes an npm package by putting the npm client's publish document, a protocol no client-go method speaks",
	},
	{
		Category: CategoryProbe,
		Sites:    []string{"fixture.probeREST"},
		Reason:   "the e2e fixture sends whatever request a scenario names, to read the status GitLab answers a token with",
	},
	{
		Category: CategoryPagination,
		Sites:    []string{"toolutil.KeysetContinuation"},
		Reason:   "the keyset continuation every keyset list passes last, which follows the next link a keyset page answers with rather than any parameter an options struct could carry",
	},
}

// writesAlwaysJSON holds each named field of typeName to being written into
// a JSON body on every call.
func writesAlwaysJSON(typeName string, fields ...string) []Predicate {
	out := make([]Predicate, 0, len(fields))
	for _, field := range fields {
		out = append(out, WritesAlways{Type: typeName, Field: field, Encoder: EncoderJSON})
	}
	return out
}

// lacksKeys holds typeName to modeling none of keys.
func lacksKeys(typeName string, keys ...string) []Predicate {
	out := make([]Predicate, 0, len(keys))
	for _, key := range keys {
		out = append(out, LacksKey{Type: typeName, Key: key})
	}
	return out
}

// fieldKinds holds each named field of typeName to the type kind.
func fieldKinds(typeName, kind string, fields ...string) []Predicate {
	out := make([]Predicate, 0, len(fields))
	for _, field := range fields {
		out = append(out, FieldKind{Type: typeName, Field: field, Kind: kind})
	}
	return out
}

// takesNoOptions holds each method to taking no options struct.
func takesNoOptions(methods ...string) []Predicate {
	out := make([]Predicate, 0, len(methods))
	for _, method := range methods {
		out = append(out, TakesNoOptions{Method: method})
	}
	return out
}
