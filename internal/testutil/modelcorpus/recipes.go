package modelcorpus

// Recipe names the world a case runs in. The builders live in the end-to-end
// fixture library, behind the e2e tag; what lives here is the name a case
// refers to and the fact keys the builder promises to produce.
//
// The facts are the join between the two halves of a case. A prompt may
// interpolate a promised key and nothing else, and an argument may bind its
// truth to a promised key and nothing else, so a case cannot ask about a value
// its world does not build, and a value the world builds is written into the
// stimulus by the run rather than into the corpus by an author.
type Recipe string

// The worlds the Free corpus runs in.
//
// RecipeWorld is the one shared, read-only world: a project with a README and
// a default branch, inside a group, which any number of attempts may read at
// once. Every other recipe builds state of the attempt's own, which is what
// lets a mutating case name a literal ("Evaluation Sprint", "EVAL_TOKEN")
// without two attempts colliding over it.
const (
	// RecipeWorld is the shared read-only project and its group.
	RecipeWorld Recipe = "world"
	// RecipeProject is a project of this attempt's own, with a README and a
	// CI configuration, so a case may create a pipeline in it.
	RecipeProject Recipe = "project"
	// RecipeGroup is a group of this attempt's own, for the cases that
	// create something directly under a group.
	RecipeGroup Recipe = "group"
	// RecipeBranch is a project with a branch beside the default one.
	RecipeBranch Recipe = "branch"
	// RecipeIssue is a project with an issue in it.
	RecipeIssue Recipe = "issue"
	// RecipeMergeRequest is a project with an open merge request.
	RecipeMergeRequest Recipe = "merge_request"
	// RecipeMergeRequestDiscussion is that merge request with an unresolved
	// discussion on it.
	RecipeMergeRequestDiscussion Recipe = "merge_request_discussion"
	// RecipeMergeRequestSource is a project with a branch ahead of the
	// default one, ready for a merge request that does not exist yet.
	RecipeMergeRequestSource Recipe = "merge_request_source"
	// RecipePipelineJob is a project with a pipeline and its jobs. A case
	// that needs the job to have finished says so with Needs.Runner; a case
	// that needs it unfinished, such as the cancel one, does not.
	RecipePipelineJob Recipe = "pipeline_job"
	// RecipeFailedJob is a project with a pipeline whose job failed and left
	// an artifact behind.
	RecipeFailedJob Recipe = "failed_job_artifact"
	// RecipeEnvironment is a project with an environment and a deployment
	// recorded against it.
	RecipeEnvironment Recipe = "environment_deployment"
	// RecipeRelease is a project with a release.
	RecipeRelease Recipe = "release"
	// RecipeSnippet is a personal snippet.
	RecipeSnippet Recipe = "snippet"
	// RecipeMember is a project with another user added to it.
	RecipeMember Recipe = "member"
	// RecipeRunner is a project with a runner registered for it and a job
	// that runner has run.
	RecipeRunner Recipe = "runner"
	// RecipeCIVariable is a project with a CI variable already in it, for
	// the cases that change one rather than create one.
	RecipeCIVariable Recipe = "ci_variable"
	// RecipeInstanceVariable is a CI variable name no other attempt uses.
	// Instance scope is shared by every attempt on the instance, so unlike a
	// project-scoped name this one cannot be a literal.
	RecipeInstanceVariable Recipe = "instance_ci_variable"
	// RecipePackageFiles is a project and local files to publish to it.
	RecipePackageFiles Recipe = "package_files"
)

// The worlds the destructive partition runs in.
//
// Almost every one of them is "a project of this attempt's own with one thing
// already in it", because a case that deletes something needs that something
// to exist and needs it to belong to nobody else. They are named after what
// they seed rather than after the case that destroys it, so a second case
// asking about the same object reuses the world instead of declaring a
// near-copy of it.
const (
	// RecipeMergeableMergeRequest is a merge request whose pipeline is
	// running, so a merge can be asked for and made to wait on it.
	RecipeMergeableMergeRequest Recipe = "mergeable_merge_request"
	// RecipeFile is a project with a file already committed on a branch
	// beside the default one.
	RecipeFile Recipe = "repository_file"
	// RecipeMilestone is a project with a milestone in it.
	RecipeMilestone Recipe = "milestone"
	// RecipeProjectAccessToken is a project with an access token of its own.
	RecipeProjectAccessToken Recipe = "project_access_token"
	// RecipePackage is a project with a generic package published to it.
	RecipePackage Recipe = "package"
	// RecipeBroadcastMessage is an instance broadcast message.
	RecipeBroadcastMessage Recipe = "broadcast_message"
	// RecipeProjectHook is a project with a webhook on it.
	RecipeProjectHook Recipe = "project_hook"
	// RecipeProjectBadge is a project with a badge on it.
	RecipeProjectBadge Recipe = "project_badge"
	// RecipeDraftNote is a merge request with an unpublished draft note.
	RecipeDraftNote Recipe = "draft_note"
	// RecipeJobTokenScope is a project whose CI job token allowlist already
	// names a second project.
	RecipeJobTokenScope Recipe = "job_token_scope"
	// RecipeInstanceVariableSeeded is an instance CI variable that already
	// exists, which is the opposite of [RecipeInstanceVariable]: that one
	// reserves a name nothing has used, this one has used it.
	RecipeInstanceVariableSeeded Recipe = "instance_ci_variable_seeded"
	// RecipeTag is a project with a tag on it.
	RecipeTag Recipe = "tag"
	// RecipePipelineTrigger is a project with a pipeline trigger token.
	RecipePipelineTrigger Recipe = "pipeline_trigger"
	// RecipePipelineSchedule is a project with a pipeline schedule.
	RecipePipelineSchedule Recipe = "pipeline_schedule"
	// RecipeUser is a user of this attempt's own, so blocking one or turning
	// its two-factor authentication off touches nobody else. It is what the
	// old evaluator's skip reason for MT-105 was about: that case ran
	// against the shared evaluator user and had to be skipped.
	RecipeUser Recipe = "user"
	// RecipeMemberCandidate is a project and a user who is not a member of
	// it yet.
	RecipeMemberCandidate Recipe = "project_member_candidate"
	// RecipeFeatureFlag is a project with a feature flag.
	RecipeFeatureFlag Recipe = "feature_flag"
	// RecipeCustomEmoji is a group with a custom emoji.
	RecipeCustomEmoji Recipe = "custom_emoji"
	// RecipeWikiPage is a project with a wiki page.
	RecipeWikiPage Recipe = "wiki_page"
	// RecipeMergeRequestAward is a merge request with an award emoji on it.
	RecipeMergeRequestAward Recipe = "merge_request_award"
	// RecipeIssueAward is an issue with an award emoji on it.
	RecipeIssueAward Recipe = "issue_award"
	// RecipeDeployKey is a project with a deploy key.
	RecipeDeployKey Recipe = "deploy_key"
	// RecipeDeployToken is a project with a deploy token.
	RecipeDeployToken Recipe = "deploy_token"
	// RecipeCommitDiscussion is a project with a discussion on a commit and
	// a note in that discussion.
	RecipeCommitDiscussion Recipe = "commit_discussion"
	// RecipeTerraformState is a project with a locked Terraform state.
	RecipeTerraformState Recipe = "terraform_state"
	// RecipeDatabaseMigration is a database migration version the instance
	// has not marked applied.
	RecipeDatabaseMigration Recipe = "database_migration"
	// RecipeProjectMirror is a project with a remote mirror configured.
	RecipeProjectMirror Recipe = "project_mirror"
)

// The worlds the licensed partitions run in.
//
// A licensed case says what it needs of the instance through [Needs.Tier]; a
// recipe here says what it needs of the world, and the two are separate
// questions. A group holding an epic is a world any tier could describe; only
// GitLab's license decides whether the epic can exist.
const (
	// RecipeEpic is a group with an epic, a note on it and a discussion.
	RecipeEpic Recipe = "epic"
	// RecipeEpicIssue is a group with an epic and a project whose issue is
	// assigned to it.
	RecipeEpicIssue Recipe = "epic_issue"
	// RecipePushRule is a project with a push rule already on it.
	RecipePushRule Recipe = "push_rule"
	// RecipeProjectServiceAccount is a project with a service account and a
	// token for it.
	RecipeProjectServiceAccount Recipe = "project_service_account"
	// RecipeGroupServiceAccount is a group with a service account and a
	// token for it.
	RecipeGroupServiceAccount Recipe = "group_service_account"
	// RecipeServiceAccountName is a service account username nobody on the
	// instance signs in as, for the case that creates an instance service
	// account. A username is unique instance-wide, so it cannot be a
	// literal.
	RecipeServiceAccountName Recipe = "service_account_name"
	// RecipeGroupProtectedBranch is a group with a protected branch rule.
	RecipeGroupProtectedBranch Recipe = "group_protected_branch"
	// RecipeGroupProtectedEnvironment is a group with a protected
	// environment.
	RecipeGroupProtectedEnvironment Recipe = "group_protected_environment"
	// RecipeGeoSite is a registered Geo site.
	RecipeGeoSite Recipe = "geo_site"
	// RecipeGeoSiteName is a Geo site name and URL no site on the instance
	// holds, which is the opposite of [RecipeGeoSite]: that one registers a
	// site, this one reserves what a case may register one as. GitLab holds
	// both a site's name and its URL unique instance-wide, so neither can
	// be a literal: one case runs three times against one instance, and
	// every attempt after the first would be refused.
	RecipeGeoSiteName Recipe = "geo_site_name"
	// RecipeScimIdentity is a group with a SCIM identity.
	RecipeScimIdentity Recipe = "scim_identity"
	// RecipeLDAPLink is a group with an LDAP link.
	RecipeLDAPLink Recipe = "ldap_link"
	// RecipeSAMLLink is a group with a SAML group link.
	RecipeSAMLLink Recipe = "saml_link"
	// RecipeGroupSSHCertificate is a group with an SSH certificate.
	RecipeGroupSSHCertificate Recipe = "group_ssh_certificate"
	// RecipeGroupWikiPage is a group with a wiki page.
	RecipeGroupWikiPage Recipe = "group_wiki_page"
	// RecipeMemberRole is an instance custom member role.
	RecipeMemberRole Recipe = "member_role"
	// RecipeProjectAlias is a project with an alias.
	RecipeProjectAlias Recipe = "project_alias"
	// RecipeProjectAliasName is a project and an alias name nothing on the
	// instance has claimed, for the case that creates one. An alias names a
	// project instance-wide, so it cannot be a literal for the reason
	// [RecipeInstanceVariable]'s key cannot.
	RecipeProjectAliasName Recipe = "project_alias_name"
	// RecipeExternalStatusCheck is a project with an external status check
	// and a merge request the check applies to.
	RecipeExternalStatusCheck Recipe = "external_status_check"
	// RecipeMergeTrainEntry is a project with a merge request on its merge
	// train.
	RecipeMergeTrainEntry Recipe = "merge_train_entry"
	// RecipeStorageMove is a group with a repository storage move recorded
	// against it.
	RecipeStorageMove Recipe = "storage_move"
	// RecipeDependencyExport is a pipeline with a dependency list export.
	RecipeDependencyExport Recipe = "dependency_export"
	// RecipeInstanceAuditEvent is an instance audit event.
	RecipeInstanceAuditEvent Recipe = "instance_audit_event"
	// RecipeAttestation is a project with a build attestation.
	RecipeAttestation Recipe = "attestation"
	// RecipeVulnerability is a project with a vulnerability record.
	RecipeVulnerability Recipe = "vulnerability"
	// RecipeEnterpriseUser is a group with an enterprise user in it.
	RecipeEnterpriseUser Recipe = "enterprise_user"
	// RecipeGroupAccessToken is a group with an access token of its own.
	RecipeGroupAccessToken Recipe = "group_access_token"
	// RecipeModelVersion is a project with a model registry version and a
	// file in it.
	RecipeModelVersion Recipe = "model_version"
	// RecipeDeploymentApproval is a project with a protected environment and
	// a deployment waiting for approval on it.
	RecipeDeploymentApproval Recipe = "deployment_approval"
)

// The fact keys the recipes promise. They are spelled as the evaluator this
// replaces spelled them, so a reader comparing the two corpora is comparing
// cases rather than vocabularies.
const (
	// FactProjectPath is the project a case works in, rendered as its full
	// path. A recipe accepts its numeric ID as a second spelling, so an
	// argument that takes either is satisfied by either.
	FactProjectPath = "project_path"
	// FactProjectID is that same project rendered as its numeric ID, and is
	// a fact of its own for the reason [FactGroupID] is: two actions type
	// project_id as an integer (project_alias.create and
	// storage_move.schedule_project), and an integer argument refuses a
	// path whatever spellings a scorer would accept.
	FactProjectID = "project_id"
	// FactDefaultBranch is that project's default branch.
	FactDefaultBranch = "default_branch"
	// FactRemoteURL is that project's git remote URL, which the project
	// discovery tool resolves.
	FactRemoteURL = "remote_url"
	// FactGroupPath is the group a case works in, rendered as its path and
	// accepting its numeric ID.
	FactGroupPath = "group_path"
	// FactGroupID is that same group rendered as its numeric ID.
	//
	// It is a fact of its own rather than a second spelling of
	// [FactGroupPath], because what a prompt hands the model is the value the
	// fact renders as, not the set of spellings the scorer would accept: an
	// argument typed as an integer refuses a path, so a case binding one has
	// to give the model the number.
	FactGroupID = "group_id"

	FactArtifactPath          = "artifact_path"
	FactBranchName            = "branch_name"
	FactCIVariableKey         = "ci_variable_key"
	FactDeploymentID          = "deployment_id"
	FactDiscussionID          = "discussion_id"
	FactEnvironmentID         = "environment_id"
	FactEnvironmentName       = "environment_name"
	FactInstanceCIVariableKey = "instance_ci_variable_key"
	FactIssueIID              = "issue_iid"
	FactJobID                 = "job_id"
	FactMergeRequestIID       = "merge_request_iid"
	FactMergeRequestSource    = "mr_source_branch"
	FactPackageDir            = "package_dir"
	FactPackageFilesDisplay   = "package_files_display"
	FactPackageName           = "package_name"
	FactPackageTag            = "package_tag"
	FactPackageVersion        = "package_version"
	FactPipelineID            = "pipeline_id"
	FactReleaseName           = "release_name"
	FactReleaseTagName        = "release_tag_name"
	FactRunnerID              = "runner_id"
	FactSnippetID             = "snippet_id"
)

// The fact keys the destructive and licensed worlds add.
//
// Every one of them names a thing a builder has to have made, which is what
// separates this block from the one above: a read case can be satisfied by a
// collection that happens to be empty, and a case that deletes something
// cannot.
const (
	FactAttestationIID          = "attestation_iid"
	FactAuditEventID            = "audit_event_id"
	FactAwardID                 = "award_id"
	FactBadgeID                 = "badge_id"
	FactBroadcastMessageID      = "broadcast_message_id"
	FactCommitDiscussionID      = "commit_discussion_id"
	FactCommitNoteID            = "commit_note_id"
	FactCommitSHA               = "commit_sha"
	FactCustomEmojiID           = "custom_emoji_id"
	FactDatabaseMigration       = "database_migration_version"
	FactDependencyExportID      = "dependency_export_id"
	FactDeployKeyID             = "deploy_key_id"
	FactDeployTokenID           = "deploy_token_id"
	FactEpicDiscussionID        = "epic_discussion_id"
	FactEpicIID                 = "epic_iid"
	FactEpicNoteID              = "epic_note_id"
	FactExternalStatusCheckID   = "external_status_check_id"
	FactFeatureFlagName         = "feature_flag_name"
	FactFilePath                = "file_path"
	FactGeoSiteID               = "geo_site_id"
	FactGeoSiteName             = "geo_site_name"
	FactGeoSiteURL              = "geo_site_url"
	FactGroupAccessTokenID      = "group_access_token_id"
	FactHookID                  = "hook_id"
	FactJobTokenTargetProjectID = "job_token_target_project_id"
	FactLDAPProvider            = "ldap_provider"
	FactMemberRoleID            = "member_role_id"
	FactMilestoneIID            = "milestone_iid"
	FactMirrorID                = "mirror_id"
	FactModelFileName           = "model_file_name"
	FactModelFilePath           = "model_file_path"
	FactModelVersionID          = "model_version_id"
	FactPackageID               = "package_id"
	FactPipelineScheduleID      = "pipeline_schedule_id"
	FactPipelineTriggerID       = "pipeline_trigger_id"
	FactProjectAccessTokenID    = "project_access_token_id"
	FactProjectAliasName        = "project_alias_name"
	FactProtectedBranchName     = "protected_branch_name"
	FactProtectedEnvironment    = "protected_environment_name"
	FactSAMLGroupName           = "saml_group_name"
	FactScimUID                 = "scim_uid"
	FactServiceAccountID        = "service_account_id"
	FactServiceAccountTokenID   = "service_account_token_id"
	FactServiceAccountUsername  = "service_account_username"
	FactSSHCertificateID        = "ssh_certificate_id"
	FactStorageMoveID           = "storage_move_id"
	FactTagName                 = "tag_name"
	FactTerraformStateName      = "terraform_state_name"
	FactUserID                  = "user_id"
	FactVulnerabilityID         = "vulnerability_id"
	FactWikiSlug                = "wiki_slug"
)

// withProjectFacts prefixes the three facts every recipe that builds a project
// promises. A project has a path, a default branch and a clone URL whatever
// else the recipe puts in it, and spelling that out per recipe is how one of
// them ends up missing one.
//
// It builds a slice of its own rather than appending to a shared one, so no
// two recipes can ever come to share a backing array.
func withProjectFacts(extra ...string) []string {
	facts := make([]string, 0, 4+len(extra))
	facts = append(facts, FactProjectPath, FactProjectID, FactDefaultBranch, FactRemoteURL)
	return append(facts, extra...)
}

// withGroupFacts does the same for the two spellings of a group, which every
// recipe that builds one promises whatever else it puts in the group.
func withGroupFacts(extra ...string) []string {
	facts := make([]string, 0, 2+len(extra))
	facts = append(facts, FactGroupPath, FactGroupID)
	return append(facts, extra...)
}

// recipeFacts is what each recipe promises to produce. types_test.go holds
// every Truth.Fact and every {{ .Facts.<key> }} in a prompt to the promises of
// that case's own recipe, so a case cannot bind to a value nothing builds.
var recipeFacts = map[Recipe][]string{
	RecipeWorld:                  withProjectFacts(FactGroupPath, FactGroupID),
	RecipeProject:                withProjectFacts(),
	RecipeGroup:                  withGroupFacts(),
	RecipeBranch:                 withProjectFacts(FactBranchName),
	RecipeIssue:                  withProjectFacts(FactIssueIID),
	RecipeMergeRequest:           withProjectFacts(FactMergeRequestIID),
	RecipeMergeRequestDiscussion: withProjectFacts(FactMergeRequestIID, FactDiscussionID),
	RecipeMergeRequestSource:     withProjectFacts(FactMergeRequestSource),
	RecipePipelineJob:            withProjectFacts(FactPipelineID, FactJobID),
	RecipeFailedJob:              withProjectFacts(FactPipelineID, FactJobID, FactArtifactPath),
	RecipeEnvironment:            withProjectFacts(FactEnvironmentID, FactEnvironmentName, FactDeploymentID),
	RecipeRelease:                withProjectFacts(FactReleaseTagName, FactReleaseName),
	RecipeSnippet:                {FactSnippetID},
	// The member recipe adds a person to the project rather than a value to
	// the prompt, so what it promises is the project it added them to.
	RecipeMember:           withProjectFacts(),
	RecipeRunner:           withProjectFacts(FactRunnerID, FactJobID, FactPipelineID),
	RecipeCIVariable:       withProjectFacts(FactCIVariableKey),
	RecipeInstanceVariable: {FactInstanceCIVariableKey},
	RecipePackageFiles: withProjectFacts(
		FactPackageDir, FactPackageFilesDisplay, FactPackageName, FactPackageVersion, FactPackageTag,
	),

	// The destructive worlds.
	RecipeMergeableMergeRequest:  withProjectFacts(FactMergeRequestIID),
	RecipeFile:                   withProjectFacts(FactBranchName, FactFilePath),
	RecipeMilestone:              withProjectFacts(FactMilestoneIID),
	RecipeProjectAccessToken:     withProjectFacts(FactProjectAccessTokenID),
	RecipePackage:                withProjectFacts(FactPackageID),
	RecipeBroadcastMessage:       {FactBroadcastMessageID},
	RecipeProjectHook:            withProjectFacts(FactHookID),
	RecipeProjectBadge:           withProjectFacts(FactBadgeID),
	RecipeDraftNote:              withProjectFacts(FactMergeRequestIID),
	RecipeJobTokenScope:          withProjectFacts(FactJobTokenTargetProjectID),
	RecipeInstanceVariableSeeded: {FactInstanceCIVariableKey},
	RecipeTag:                    withProjectFacts(FactTagName),
	RecipePipelineTrigger:        withProjectFacts(FactPipelineTriggerID),
	RecipePipelineSchedule:       withProjectFacts(FactPipelineScheduleID),
	RecipeUser:                   {FactUserID},
	RecipeMemberCandidate:        withProjectFacts(FactUserID),
	RecipeFeatureFlag:            withProjectFacts(FactFeatureFlagName),
	RecipeCustomEmoji:            withGroupFacts(FactCustomEmojiID),
	RecipeWikiPage:               withProjectFacts(FactWikiSlug),
	RecipeMergeRequestAward:      withProjectFacts(FactMergeRequestIID, FactAwardID),
	RecipeIssueAward:             withProjectFacts(FactIssueIID, FactAwardID),
	RecipeDeployKey:              withProjectFacts(FactDeployKeyID),
	RecipeDeployToken:            withProjectFacts(FactDeployTokenID),
	RecipeCommitDiscussion: withProjectFacts(
		FactCommitSHA, FactCommitDiscussionID, FactCommitNoteID,
	),
	RecipeTerraformState:    withProjectFacts(FactTerraformStateName),
	RecipeDatabaseMigration: {FactDatabaseMigration},
	RecipeProjectMirror:     withProjectFacts(FactMirrorID),

	// The licensed worlds.
	RecipeEpic:                      withGroupFacts(FactEpicIID, FactEpicNoteID, FactEpicDiscussionID),
	RecipeEpicIssue:                 withProjectFacts(FactGroupPath, FactGroupID, FactEpicIID, FactIssueIID),
	RecipePushRule:                  withProjectFacts(),
	RecipeProjectServiceAccount:     withProjectFacts(FactServiceAccountID, FactServiceAccountTokenID),
	RecipeGroupServiceAccount:       withGroupFacts(FactServiceAccountID, FactServiceAccountTokenID),
	RecipeGroupProtectedBranch:      withGroupFacts(FactProtectedBranchName),
	RecipeGroupProtectedEnvironment: withGroupFacts(FactProtectedEnvironment),
	RecipeGeoSite:                   {FactGeoSiteID},
	RecipeGeoSiteName:               {FactGeoSiteName, FactGeoSiteURL},
	RecipeServiceAccountName:        {FactServiceAccountUsername},
	RecipeScimIdentity:              withGroupFacts(FactScimUID),
	RecipeLDAPLink:                  withGroupFacts(FactLDAPProvider),
	RecipeSAMLLink:                  withGroupFacts(FactSAMLGroupName),
	RecipeGroupSSHCertificate:       withGroupFacts(FactSSHCertificateID),
	RecipeGroupWikiPage:             withGroupFacts(FactWikiSlug),
	RecipeMemberRole:                withGroupFacts(FactMemberRoleID),
	RecipeProjectAlias:              withProjectFacts(FactProjectAliasName),
	RecipeProjectAliasName:          withProjectFacts(FactProjectAliasName),
	RecipeExternalStatusCheck: withProjectFacts(
		FactMergeRequestIID, FactExternalStatusCheckID, FactCommitSHA,
	),
	RecipeMergeTrainEntry:    withProjectFacts(FactMergeRequestIID),
	RecipeStorageMove:        withGroupFacts(FactStorageMoveID),
	RecipeDependencyExport:   withProjectFacts(FactPipelineID, FactDependencyExportID),
	RecipeInstanceAuditEvent: withProjectFacts(FactAuditEventID),
	RecipeAttestation:        withProjectFacts(FactAttestationIID),
	RecipeVulnerability:      withProjectFacts(FactPipelineID, FactVulnerabilityID),
	RecipeEnterpriseUser:     withGroupFacts(FactUserID),
	RecipeGroupAccessToken:   withGroupFacts(FactGroupAccessTokenID),
	RecipeModelVersion: withProjectFacts(
		FactModelVersionID, FactModelFilePath, FactModelFileName,
	),
	RecipeDeploymentApproval: withProjectFacts(
		FactProtectedEnvironment, FactEnvironmentName, FactDeploymentID,
	),
}

// Recipes returns every recipe the corpus names, in declaration order, which
// is what the fixture library's own gate enumerates to check that each has a
// builder.
func Recipes() []Recipe {
	return []Recipe{
		RecipeWorld, RecipeProject, RecipeGroup, RecipeBranch, RecipeIssue,
		RecipeMergeRequest, RecipeMergeRequestDiscussion, RecipeMergeRequestSource,
		RecipePipelineJob, RecipeFailedJob, RecipeEnvironment, RecipeRelease,
		RecipeSnippet, RecipeMember, RecipeRunner, RecipeCIVariable,
		RecipeInstanceVariable, RecipePackageFiles,

		RecipeMergeableMergeRequest, RecipeFile, RecipeMilestone,
		RecipeProjectAccessToken, RecipePackage, RecipeBroadcastMessage,
		RecipeProjectHook, RecipeProjectBadge, RecipeDraftNote,
		RecipeJobTokenScope, RecipeInstanceVariableSeeded, RecipeTag,
		RecipePipelineTrigger, RecipePipelineSchedule, RecipeUser,
		RecipeMemberCandidate, RecipeFeatureFlag, RecipeCustomEmoji,
		RecipeWikiPage, RecipeMergeRequestAward, RecipeIssueAward,
		RecipeDeployKey, RecipeDeployToken, RecipeCommitDiscussion,
		RecipeTerraformState, RecipeDatabaseMigration, RecipeProjectMirror,

		RecipeEpic, RecipeEpicIssue, RecipePushRule,
		RecipeProjectServiceAccount, RecipeGroupServiceAccount,
		RecipeGroupProtectedBranch, RecipeGroupProtectedEnvironment,
		RecipeGeoSite, RecipeGeoSiteName, RecipeServiceAccountName,
		RecipeScimIdentity, RecipeLDAPLink, RecipeSAMLLink,
		RecipeGroupSSHCertificate, RecipeGroupWikiPage, RecipeMemberRole,
		RecipeProjectAlias, RecipeProjectAliasName, RecipeExternalStatusCheck,
		RecipeMergeTrainEntry, RecipeStorageMove, RecipeDependencyExport,
		RecipeInstanceAuditEvent, RecipeAttestation, RecipeVulnerability,
		RecipeEnterpriseUser, RecipeGroupAccessToken, RecipeModelVersion,
		RecipeDeploymentApproval,
	}
}

// Facts returns the fact keys a recipe promises, and whether the recipe is one
// this corpus knows at all.
func Facts(recipe Recipe) ([]string, bool) {
	facts, known := recipeFacts[recipe]
	if !known {
		return nil, false
	}
	return append([]string(nil), facts...), true
}
