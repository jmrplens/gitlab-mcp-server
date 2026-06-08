package cases

const evalMT099 = "MT-099"

//nolint:maintidx // Static table keeps destructive case definitions close to their expected workflow order.
func destructiveEvalCases() []Case {
	return []Case{
		baseDestructiveEvalCase("MT-008", "Delete subgroup `my-org/eval-temp`.", destructiveStep("gitlab_group", "delete", params("group_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-013", "Delete issue `42` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm"))),
		baseDestructiveEvalCase("MT-017", "Merge merge request `7` in project `my-org/tools/gitlab-mcp-server` when the pipeline succeeds.", destructiveStep("gitlab_merge_request", "merge", params("project_id", "merge_request_iid"), params("auto_merge", "confirm"))),
		baseDestructiveEvalCase("MT-024", "Delete artifacts for job `999` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_job", "delete_artifacts", params("project_id", "job_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-028", "Delete CI variable `EVAL_TOKEN` from production scope in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_ci_variable", "delete", params("project_id", "key"), params("environment_scope", "confirm"))),
		baseDestructiveEvalCase("MT-031", "Delete file `tmp/eval.txt` with commit_message `Delete evaluation file` from branch `feature/eval` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_repository", "file_delete", params("project_id", "file_path", "branch", "commit_message"), params("confirm"))),
		baseDestructiveEvalCase("MT-035", "Delete milestone IID `7` named `Evaluation Sprint` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_project", "milestone_delete", params("project_id", "milestone_iid"), params("confirm"))),
		baseDestructiveEvalCase("MT-037", "Delete release `v0.0.0-eval` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_release", "delete", params("project_id", "tag_name"), params("confirm"))),
		baseDestructiveEvalCase("MT-042", "Revoke project access token ID `77` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_access", "token_project_revoke", params("project_id", "token_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-044", "Delete package ID `55` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_package", "delete", params("project_id", "package_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-047", "Remove runner ID `99`.", destructiveStep("gitlab_runner", "remove", params("runner_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-049", "Stop environment ID `7` in project `my-org/tools/gitlab-mcp-server`, forcing the stop if needed.", destructiveStep("gitlab_environment", "stop", params("project_id", "environment_id"), params("force", "confirm"))),
		baseDestructiveEvalCase("MT-051", "Delete personal snippet ID `33`.", destructiveStep("gitlab_snippet", "delete", params("snippet_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-054", "Delete broadcast message ID `12`.", destructiveStep("gitlab_admin", "broadcast_message_delete", params("id"), params("confirm"))),
		baseDestructiveEvalCase("MT-055", "Archive project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_project", "archive", params("project_id"), nil)),
		baseDestructiveEvalCase("MT-057", "Delete webhook ID `5` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_project", "hook_delete", params("project_id", "hook_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-059", "Delete badge ID `8` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_project", "badge_delete", params("project_id", "badge_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-063", "Publish all draft review notes for MR `7` in project `my-org/tools/gitlab-mcp-server`.", readStep("gitlab_mr_review", "draft_note_publish_all", params("project_id", "merge_request_iid"), nil)),
		baseDestructiveEvalCase("MT-066", "Remove project ID `123` from the CI job token allowlist of project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_job", "token_scope_remove_project", params("project_id", "target_project_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-069", "Delete instance CI variable `INSTANCE_EVAL_TOKEN`.", destructiveStep("gitlab_ci_variable", "instance_delete", params("key"), params("confirm"))),
		baseDestructiveEvalCase(evalMT099, "Delete branch `obsolete/eval` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_branch", "delete", params("project_id", "branch_name"), params("confirm"))),
		baseDestructiveEvalCase("MT-100", "Delete tag `v0.0.0-eval` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_tag", "delete", params("project_id", "tag_name"), params("confirm"))),
		baseDestructiveEvalCase("MT-101", "Permanently delete pipeline `12345` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_pipeline", "delete", params("project_id", "pipeline_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-102", "Delete pipeline trigger token ID `77` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_pipeline", "trigger_delete", params("project_id", "trigger_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-103", "Delete pipeline schedule ID `12` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_pipeline", "schedule_delete", params("project_id", "schedule_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-104", "Block user ID `55`.", destructiveStep("gitlab_user", "block", params("user_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-105", "Disable two-factor authentication for user ID `55`.", destructiveStep("gitlab_user", "disable_two_factor", params("user_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-106", "Delete feature flag `eval_flag` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_feature_flags", "feature_flag_delete", params("project_id", "name"), params("confirm"))),
		baseDestructiveEvalCase("MT-107", "Delete custom emoji GID `gid://gitlab/CustomEmoji/77`.", destructiveStep("gitlab_custom_emoji", "delete", params("id"), params("confirm"))),
		baseDestructiveEvalCase("MT-108", "Delete wiki page `obsolete-eval` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_wiki", "delete", params("project_id", "slug"), params("confirm"))),
		baseDestructiveEvalCase("MT-109", "Remove award emoji ID `12` from merge request `7` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_merge_request", "emoji_mr_delete", params("project_id", "merge_request_iid", "award_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-110", "Remove award emoji ID `12` from issue `42` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_issue", "emoji_issue_delete", params("project_id", "issue_iid", "award_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-111", "Delete deploy key ID `88` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_access", "deploy_key_delete", params("project_id", "deploy_key_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-112", "Delete project deploy token ID `66` from project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_access", "deploy_token_delete_project", params("project_id", "deploy_token_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-113", "Delete commit discussion note `999` from discussion `abc123` on commit `abc1234` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_repository", "commit_discussion_delete_note", params("project_id", "commit_sha", "discussion_id", "note_id"), params("confirm"))),
		baseDestructiveEvalCase("MT-114", "Unlock Terraform state `production` in project `my-org/tools/gitlab-mcp-server`.", destructiveStep("gitlab_admin", "terraform_state_unlock", params("project_id", "name"), params("confirm"))),
		baseDestructiveEvalCase("MT-115", "Mark database migration version `20260101000000` as applied.", destructiveStep("gitlab_admin", "db_migration_mark", params("version"), params("database", "confirm"))),
		baseDestructiveEvalCase(
			"MS-003", "Prepare a batch review for MR `7` in project `my-org/tools/gitlab-mcp-server`: inspect the MR, inspect changes, create a draft note saying `Please add a regression test`, then publish all draft notes.",
			readStep("gitlab_merge_request", "get", params("project_id", "merge_request_iid"), nil),
			readStep("gitlab_mr_review", "changes_get", params("project_id", "merge_request_iid"), nil),
			readStep("gitlab_mr_review", "draft_note_create", params("project_id", "merge_request_iid", "note"), params("position")),
			readStep("gitlab_mr_review", "draft_note_publish_all", params("project_id", "merge_request_iid"), nil),
		),
		baseDestructiveEvalCase(
			"MS-004", "Clean up release `v0.0.0-eval` in project `my-org/tools/gitlab-mcp-server`: verify the tag, verify the release, list release links, delete the release, then delete the tag.",
			readStep("gitlab_tag", "get", params("project_id", "tag_name"), nil),
			readStep("gitlab_release", "get", params("project_id", "tag_name"), nil),
			readStep("gitlab_release", "link_list", params("project_id", "tag_name"), nil),
			destructiveStep("gitlab_release", "delete", params("project_id", "tag_name"), params("confirm")),
			destructiveStep("gitlab_tag", "delete", params("project_id", "tag_name"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-007", "Clean up an obsolete package in project `my-org/tools/gitlab-mcp-server`: list generic packages, list files for package ID `55`, then delete package ID `55`.",
			readStep("gitlab_package", "list", params("project_id"), params("package_type")),
			readStep("gitlab_package", "file_list", params("project_id", "package_id"), nil),
			destructiveStep("gitlab_package", "delete", params("project_id", "package_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-009", "Schedule and then remove an instance maintenance banner: read current instance settings, immediately create broadcast message `Evaluation maintenance`, then delete the broadcast message created in the previous step using the returned ID.",
			readStep("gitlab_admin", "settings_get", nil, nil),
			readStep("gitlab_admin", "broadcast_message_create", params("message"), params("starts_at", "ends_at", "broadcast_type")),
			destructiveStep("gitlab_admin", "broadcast_message_delete", params("id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-013", "Remove a temporary feature rollout from project `my-org/tools/gitlab-mcp-server`: inspect feature flag `eval_flag`, list feature flag user lists, then delete the flag.",
			readStep("gitlab_feature_flags", "feature_flag_get", params("project_id", "name"), nil),
			readStep("gitlab_feature_flags", "ff_user_list_list", params("project_id"), params("per_page")),
			destructiveStep("gitlab_feature_flags", "feature_flag_delete", params("project_id", "name"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-014", "Exercise issue CRUD in project `my-org/tools/gitlab-mcp-server`: create issue `eval-crud-issue`, fetch it with issue get using the returned issue IID, update its title to `eval-crud-issue-updated`, close it, reopen it, then delete it.",
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description", "labels")),
			readStep("gitlab_issue", "get", params("project_id", "issue_iid"), nil),
			readStep("gitlab_issue", "update", params("project_id", "issue_iid"), params("title", "description", "labels")),
			readStep("gitlab_issue", "update", params("project_id", "issue_iid", "state_event"), nil),
			readStep("gitlab_issue", "update", params("project_id", "issue_iid", "state_event"), nil),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-015", "Exercise issue note CRUD in project `my-org/tools/gitlab-mcp-server`: create issue `eval-note-issue`, add a note saying `first note`, fetch that note with note get using the returned note ID, update the note to `updated note`, delete the note, then delete the issue.",
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description", "labels")),
			readStep("gitlab_issue", "note_create", params("project_id", "issue_iid", "body"), nil),
			readStep("gitlab_issue", "note_get", params("project_id", "issue_iid", "note_id"), nil),
			readStep("gitlab_issue", "note_update", params("project_id", "issue_iid", "note_id", "body"), nil),
			destructiveStep("gitlab_issue", "note_delete", params("project_id", "issue_iid", "note_id"), params("confirm")),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-016", "Exercise issue link CRUD in project `my-org/tools/gitlab-mcp-server`: create source issue `eval-link-source`, create target issue `eval-link-target`, link source to target as `relates_to`, list source issue links, delete the returned issue link, then delete both issues.",
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description")),
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description")),
			readStep("gitlab_issue", "link_create", params("project_id", "issue_iid", "target_project_id", "target_issue_iid"), params("link_type")),
			readStep("gitlab_issue", "link_list", params("project_id", "issue_iid"), nil),
			destructiveStep("gitlab_issue", "link_delete", params("project_id", "issue_iid", "issue_link_id"), params("confirm")),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-017", "Exercise repository file CRUD in project `my-org/tools/gitlab-mcp-server`: create file `tmp/eval-crud.txt` on branch `feature/eval`, read it, update its content, then delete it from the same branch.",
			readStep("gitlab_repository", "file_create", params("project_id", "file_path", "branch", "content", "commit_message"), nil),
			readStep("gitlab_repository", "file_get", params("project_id", "file_path", "ref"), nil),
			readStep("gitlab_repository", "file_update", params("project_id", "file_path", "branch", "content", "commit_message"), params("last_commit_id")),
			destructiveStep("gitlab_repository", "file_delete", params("project_id", "file_path", "branch", "commit_message"), params("last_commit_id", "confirm")),
		),
		baseDestructiveEvalCase(
			"MS-018", "Exercise release asset-link CRUD in project `my-org/tools/gitlab-mcp-server`: use the release create operation directly to create release `v0.0.0-crud` from ref `main` named `Evaluation CRUD release` without creating a tag separately and without passing `assets`; only after the release exists, add asset link `eval-crud-link`, fetch the returned link with the link get operation, update the link URL, delete the link, delete the release, then delete the tag.",
			readStep("gitlab_release", "create", params("project_id", "tag_name", "name", "ref"), params("description")),
			readStep("gitlab_release", "link_create", params("project_id", "tag_name", "name", "url"), params("link_type")),
			readStep("gitlab_release", "link_get", params("project_id", "tag_name", "link_id"), nil),
			readStep("gitlab_release", "link_update", params("project_id", "tag_name", "link_id"), params("name", "url", "link_type")),
			destructiveStep("gitlab_release", "link_delete", params("project_id", "tag_name", "link_id"), params("confirm")),
			destructiveStep("gitlab_release", "delete", params("project_id", "tag_name"), params("confirm")),
			destructiveStep("gitlab_tag", "delete", params("project_id", "tag_name"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-019", "Exercise pipeline trigger CRUD in project `my-org/tools/gitlab-mcp-server`: create trigger `eval-crud-trigger`, fetch it with trigger get using the returned trigger ID, update the description, then delete it.",
			readStep("gitlab_pipeline", "trigger_create", params("project_id", "description"), nil),
			readStep("gitlab_pipeline", "trigger_get", params("project_id", "trigger_id"), nil),
			readStep("gitlab_pipeline", "trigger_update", params("project_id", "trigger_id"), params("description")),
			destructiveStep("gitlab_pipeline", "trigger_delete", params("project_id", "trigger_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-020", "Exercise pipeline schedule CRUD in project `my-org/tools/gitlab-mcp-server`: create inactive schedule `eval-crud-schedule` on `main`, get it, update its cron, create variable `SCHEDULE_CRUD_TOKEN`, update that variable, delete the variable, then delete the schedule.",
			readStep("gitlab_pipeline", "schedule_create", params("project_id", "description", "ref", "cron"), params("cron_timezone", "active")),
			readStep("gitlab_pipeline", "schedule_get", params("project_id", "schedule_id"), nil),
			readStep("gitlab_pipeline", "schedule_update", params("project_id", "schedule_id"), params("cron", "cron_timezone", "active")),
			readStep("gitlab_pipeline", "schedule_create_variable", params("project_id", "schedule_id", "key", "value"), params("variable_type")),
			readStep("gitlab_pipeline", "schedule_edit_variable", params("project_id", "schedule_id", "key", "value"), params("variable_type")),
			destructiveStep("gitlab_pipeline", "schedule_delete_variable", params("project_id", "schedule_id", "key"), params("confirm")),
			destructiveStep("gitlab_pipeline", "schedule_delete", params("project_id", "schedule_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-021", "Exercise project webhook CRUD in project `my-org/tools/gitlab-mcp-server`: add webhook `https://example.com/eval-crud-hook`, fetch it with hook get using the returned hook ID, edit it to disable SSL verification, then delete it.",
			readStep("gitlab_project", "hook_add", params("project_id", "url"), params("push_events", "enable_ssl_verification")),
			readStep("gitlab_project", "hook_get", params("project_id", "hook_id"), nil),
			readStep("gitlab_project", "hook_edit", params("project_id", "hook_id"), params("push_events", "enable_ssl_verification")),
			destructiveStep("gitlab_project", "hook_delete", params("project_id", "hook_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-022", "Exercise project badge CRUD in project `my-org/tools/gitlab-mcp-server`: add badge `eval-crud-badge`, fetch it with badge get using the returned badge ID, edit the badge name to `Evaluation CRUD badge link`, then delete it.",
			readStep("gitlab_project", "badge_add", params("project_id", "link_url", "image_url"), params("name")),
			readStep("gitlab_project", "badge_get", params("project_id", "badge_id"), nil),
			readStep("gitlab_project", "badge_edit", params("project_id", "badge_id"), params("name", "link_url", "image_url")),
			destructiveStep("gitlab_project", "badge_delete", params("project_id", "badge_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-023", "Exercise wiki CRUD in project `my-org/tools/gitlab-mcp-server`: create wiki page titled `Evaluation CRUD wiki` with content containing `eval-crud-wiki`, fetch the created page with the returned slug, update its title to `Evaluation CRUD wiki v2`, then delete it.",
			readStep("gitlab_wiki", "create", params("project_id", "title", "content"), params("format")),
			readStep("gitlab_wiki", "get", params("project_id", "slug"), params("render_html")),
			readStep("gitlab_wiki", "update", params("project_id", "slug"), params("title", "content", "format")),
			destructiveStep("gitlab_wiki", "delete", params("project_id", "slug"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-024", "Exercise project snippet CRUD in project `my-org/tools/gitlab-mcp-server`: create project snippet `eval-crud-snippet` titled `Evaluation CRUD snippet`, fetch it with project snippet get using the returned snippet ID, update its content with a `files` entry using action `update` and `file_path` set to the returned file path, not `previous_path`, then delete it.",
			readStep("gitlab_snippet", "project_create", params("project_id", "title", "file_name", "content"), params("visibility")),
			readStep("gitlab_snippet", "project_get", params("project_id", "snippet_id"), nil),
			readStep("gitlab_snippet", "project_update", params("project_id", "snippet_id", "files"), params("title", "visibility")),
			destructiveStep("gitlab_snippet", "project_delete", params("project_id", "snippet_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-025", "Exercise scoped project CI variable CRUD in project `my-org/tools/gitlab-mcp-server`: create variable `EVAL_CRUD_TOKEN` with value `crud-value-1` and environment scope `review/eval`, list variables, update the scoped variable to value `crud-value-2`, then delete that same scoped variable.",
			readStep("gitlab_ci_variable", "create", params("project_id", "key", "value"), params("environment_scope", "masked")),
			readStep("gitlab_ci_variable", "list", params("project_id"), params("page", "per_page")),
			readStep("gitlab_ci_variable", "update", params("project_id", "key"), params("value", "environment_scope")),
			destructiveStep("gitlab_ci_variable", "delete", params("project_id", "key"), params("environment_scope", "confirm")),
		),
		baseDestructiveEvalCase(
			"MS-026", "Exercise scoped group CI variable CRUD in group `my-org`: create variable `GROUP_EVAL_CRUD_TOKEN` with value `group-crud-value-1` and environment scope `review/eval`, get it using top-level `environment_scope`, update it to value `group-crud-value-2`, then delete that same scoped variable.",
			readStep("gitlab_ci_variable", "group_create", params("group_id", "key", "value"), params("environment_scope", "masked")),
			readStep("gitlab_ci_variable", "group_get", params("group_id", "key"), params("environment_scope")),
			readStep("gitlab_ci_variable", "group_update", params("group_id", "key"), params("value", "environment_scope")),
			destructiveStep("gitlab_ci_variable", "group_delete", params("group_id", "key"), params("environment_scope", "confirm")),
		),
		baseDestructiveEvalCase(
			"MS-027", "Exercise merge request note CRUD in project `my-org/tools/gitlab-mcp-server`: add note `eval-mr-note` to MR `7`, fetch the created note using the returned note ID, update it to `eval-mr-note-updated`, then delete it.",
			readStep("gitlab_mr_review", "note_create", params("project_id", "merge_request_iid", "body"), nil),
			readStep("gitlab_mr_review", "note_get", params("project_id", "merge_request_iid", "note_id"), nil),
			readStep("gitlab_mr_review", "note_update", params("project_id", "merge_request_iid", "note_id", "body"), nil),
			destructiveStep("gitlab_mr_review", "note_delete", params("project_id", "merge_request_iid", "note_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-028", "Exercise branch protection lifecycle in project `my-org/tools/gitlab-mcp-server`: create branch `eval-protect-branch` from `main`, protect it with Maintainer push and merge access, fetch the protected branch, update it to allow force push, unprotect it, then delete the branch.",
			readStep("gitlab_branch", "create", params("project_id", "branch_name", "ref"), nil),
			readStep("gitlab_branch", "protect", params("project_id", "branch_name"), params("push_access_level", "merge_access_level", "allow_force_push")),
			readStep("gitlab_branch", "get_protected", params("project_id", "branch_name"), nil),
			readStep("gitlab_branch", "update_protected", params("project_id", "branch_name"), params("allow_force_push")),
			destructiveStep("gitlab_branch", "unprotect", params("project_id", "branch_name"), params("confirm")),
			destructiveStep("gitlab_branch", "delete", params("project_id", "branch_name"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-029", "Exercise feature flag and user-list lifecycle in project `my-org/tools/gitlab-mcp-server`: create feature flag user list `eval-feature-list` with user IDs `u1,u2`, fetch it, update the user IDs to `u2,u3`, create feature flag `eval-feature-flag-crud` using version `new_version_flag`, fetch the flag, update it inactive, delete the flag, then delete the user list.",
			readStep("gitlab_feature_flags", "ff_user_list_create", params("project_id", "name", "user_xids"), nil),
			readStep("gitlab_feature_flags", "ff_user_list_get", params("project_id", "user_list_iid"), nil),
			readStep("gitlab_feature_flags", "ff_user_list_update", params("project_id", "user_list_iid"), params("name", "user_xids")),
			readStep("gitlab_feature_flags", "feature_flag_create", params("project_id", "name", "version"), params("description", "active", "strategies")),
			readStep("gitlab_feature_flags", "feature_flag_get", params("project_id", "name"), nil),
			readStep("gitlab_feature_flags", "feature_flag_update", params("project_id", "name"), params("description", "active", "strategies")),
			destructiveStep("gitlab_feature_flags", "feature_flag_delete", params("project_id", "name"), params("confirm")),
			destructiveStep("gitlab_feature_flags", "ff_user_list_delete", params("project_id", "user_list_iid"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-030", "Exercise project deploy token lifecycle in project `my-org/tools/gitlab-mcp-server`: create deploy token `eval-deploy-token` with scope `read_repository`, fetch it with the returned deploy token ID, list project deploy tokens, then delete that deploy token.",
			readStep("gitlab_access", "deploy_token_create_project", params("project_id", "name", "scopes"), params("expires_at", "username")),
			readStep("gitlab_access", "deploy_token_get_project", params("project_id", "deploy_token_id"), nil),
			readStep("gitlab_access", "deploy_token_list_project", params("project_id"), params("page", "per_page")),
			destructiveStep("gitlab_access", "deploy_token_delete_project", params("project_id", "deploy_token_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-031", "Exercise project deploy key lifecycle in project `my-org/tools/gitlab-mcp-server`: add deploy key `eval-deploy-key` with public key `ssh-rsa AAAAevalcrud`, fetch it with deploy key get using the returned deploy key ID, update the title to `eval-deploy-key-updated`, then delete it.",
			readStep("gitlab_access", "deploy_key_add", params("project_id", "title", "key"), params("can_push", "expires_at")),
			readStep("gitlab_access", "deploy_key_get", params("project_id", "deploy_key_id"), nil),
			readStep("gitlab_access", "deploy_key_update", params("project_id", "deploy_key_id"), params("title", "can_push")),
			destructiveStep("gitlab_access", "deploy_key_delete", params("project_id", "deploy_key_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-032", "Exercise issue time tracking in project `my-org/tools/gitlab-mcp-server`: create issue `eval-time-issue`, set estimate `2h`, add spent time `30m` with summary `pairing`, reset spent time, reset the estimate, then delete the issue.",
			readStep("gitlab_issue", "create", params("project_id", "title"), params("description")),
			readStep("gitlab_issue", "time_estimate_set", params("project_id", "issue_iid", "duration"), nil),
			readStep("gitlab_issue", "spent_time_add", params("project_id", "issue_iid", "duration"), params("summary")),
			readStep("gitlab_issue", "spent_time_reset", params("project_id", "issue_iid"), nil),
			readStep("gitlab_issue", "time_estimate_reset", params("project_id", "issue_iid"), nil),
			destructiveStep("gitlab_issue", "delete", params("project_id", "issue_iid"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-033", "Exercise merge request time tracking and emoji in project `my-org/tools/gitlab-mcp-server`: set estimate `1h` on MR `7`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.",
			readStep("gitlab_merge_request", "time_estimate_set", params("project_id", "merge_request_iid", "duration"), nil),
			readStep("gitlab_merge_request", "spent_time_add", params("project_id", "merge_request_iid", "duration"), params("summary")),
			readStep("gitlab_merge_request", "emoji_mr_create", params("project_id", "merge_request_iid", "name"), nil),
			readStep("gitlab_merge_request", "emoji_mr_list", params("project_id", "merge_request_iid"), params("page", "per_page")),
			destructiveStep("gitlab_merge_request", "emoji_mr_delete", params("project_id", "merge_request_iid", "award_id"), params("confirm")),
			readStep("gitlab_merge_request", "spent_time_reset", params("project_id", "merge_request_iid"), nil),
			readStep("gitlab_merge_request", "time_estimate_reset", params("project_id", "merge_request_iid"), nil),
		),
		baseDestructiveEvalCase(
			"MS-034", "Exercise project member lifecycle in project `my-org/tools/gitlab-mcp-server`: add user ID `55` as Reporter, fetch that project member, edit access level to Developer, then remove the member.",
			readStep("gitlab_project", "member_add", params("project_id", "user_id", "access_level"), params("expires_at")),
			readStep("gitlab_project", "member_get", params("project_id", "user_id"), nil),
			readStep("gitlab_project", "member_edit", params("project_id", "user_id", "access_level"), params("expires_at")),
			destructiveStep("gitlab_project", "member_delete", params("project_id", "user_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-035", "Exercise group label lifecycle in group `my-org`: create label `eval-group-label` with color `#1f75cb`, fetch it by label ID or name, rename it to `eval-group-label-v2`, then delete it.",
			readStep("gitlab_group", "group_label_create", params("group_id", "name", "color"), params("description", "priority")),
			readStep("gitlab_group", "group_label_get", params("group_id", "label_id"), nil),
			readStep("gitlab_group", "group_label_update", params("group_id", "label_id"), params("new_name", "color", "description")),
			destructiveStep("gitlab_group", "group_label_delete", params("group_id", "label_id"), params("confirm")),
		),
		baseDestructiveEvalCase(
			"MS-036", "Exercise group milestone lifecycle in group `my-org`: create milestone `Evaluation Group Milestone` with due date `2026-12-31`, fetch it using the returned milestone IID, update title to `Evaluation Group Milestone v2`, then delete it.",
			readStep("gitlab_group", "group_milestone_create", params("group_id", "title"), params("description", "due_date")),
			readStep("gitlab_group", "group_milestone_get", params("group_id", "milestone_iid"), nil),
			readStep("gitlab_group", "group_milestone_update", params("group_id", "milestone_iid"), params("title", "description", "state_event")),
			destructiveStep("gitlab_group", "group_milestone_delete", params("group_id", "milestone_iid"), params("confirm")),
		),
	}
}

func baseDestructiveEvalCase(id, prompt string, steps ...Step) Case {
	evalCase := Case{
		ID:          id,
		Prompt:      prompt,
		Steps:       steps,
		Edition:     editionCE,
		Presets:     []string{presetDockerDestructiveSafe},
		Partition:   partitionBaseDestructive,
		Mutating:    true,
		Destructive: true,
		ReportGroup: partitionBaseDestructive,
	}
	if template, fixtures := baseDestructivePromptTemplateAndFixtures(id); template != "" {
		evalCase.PromptTemplate = PromptTemplate{Text: template}
		evalCase.Fixtures = fixtures
	}
	if reason := liveSkipReasonForCase(id); reason != "" {
		evalCase.SkipReasons = []string{reason}
	}
	return evalCase
}

func liveSkipReasonForCase(id string) string {
	switch id {
	case "MT-105":
		return "disabling two-factor authentication is not safe for the shared live evaluator user"
	case "MT-115":
		return "marking database migrations as applied is not safe for live evaluator instances"
	default:
		return ""
	}
}

func destructiveStep(tool, action string, requiredParams, optionalParams []string) Step {
	step := readStep(tool, action, requiredParams, optionalParams)
	step.Destructive = true
	return step
}

func baseDestructivePromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch {
	case len(id) >= 3 && id[:3] == "MS-":
		return baseDestructiveWorkflowPromptTemplateAndFixtures(id)
	case id >= evalMT099:
		return baseDestructiveLateSinglePromptTemplateAndFixtures(id)
	default:
		return baseDestructiveEarlySinglePromptTemplateAndFixtures(id)
	}
}

//nolint:gocyclo // Keeping this ID-to-fixture mapping in one switch makes the migration table auditable.
func baseDestructiveEarlySinglePromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch id {
	case "MT-008":
		return "Delete subgroup `{{ .Group.Path }}`.", []string{fixtureGroupDelete}
	case "MT-013":
		return "Delete issue `{{ .Issue.IID }}` from project `{{ .Project.Path }}`.", []string{fixtureIssueDelete}
	case "MT-017":
		return "Enable auto-merge for merge request `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}` by calling merge with `auto_merge=true`.", []string{fixtureMergeableMergeRequest}
	case "MT-024":
		return "Delete artifacts for job `{{ .Job.ID }}` in project `{{ .Project.Path }}`.", []string{fixtureFailedJobArtifact}
	case "MT-028":
		return "Delete CI variable `{{ .Values.ci_variable_key }}` from production scope in project `{{ .Project.Path }}`.", []string{fixtureProjectCIVariableDelete}
	case "MT-031":
		return "Delete file `{{ .Values.file_path }}` with commit_message `Delete evaluation file` from branch `{{ .Branch.Name }}` in project `{{ .Project.Path }}`. Call repository.file_delete directly with exactly that file_path and branch; do not call repository.tree or switch to a different file path.", []string{fixtureRepositoryFileDelete}
	case "MT-035":
		return "Delete milestone IID `{{ .Values.milestone_iid }}` from project `{{ .Project.Path }}`.", []string{fixtureMilestoneDelete}
	case "MT-037":
		return "Delete release `{{ .Release.TagName }}` from project `{{ .Project.Path }}`.", []string{fixtureReleaseDelete}
	case "MT-042":
		return "Revoke project access token ID `{{ .Token.ID }}` in project `{{ .Project.Path }}`.", []string{fixtureProjectAccessTokenRevoke}
	case "MT-044":
		return "Delete package ID `{{ .Package.ID }}` in project `{{ .Project.Path }}`.", []string{fixturePackageDelete}
	case "MT-047":
		return "Remove runner ID `{{ .Runner.ID }}`.", []string{fixtureRunnerRemove}
	case "MT-049":
		return "Stop environment ID `{{ .Environment.ID }}` named `{{ .Environment.Name }}` in project `{{ .Project.Path }}`, forcing the stop if needed.", []string{fixtureEnvironmentStop}
	case "MT-051":
		return "Delete personal snippet ID `{{ .Values.snippet_id }}`.", []string{fixtureSnippetDelete}
	case "MT-054":
		return "Delete broadcast message ID `{{ .Values.id }}`.", []string{fixtureBroadcastMessageDelete}
	case "MT-055":
		return "Archive project `{{ .Project.Path }}`.", []string{fixtureProjectArchive}
	case "MT-057":
		return "Delete webhook ID `{{ .Values.hook_id }}` from project `{{ .Project.Path }}`.", []string{fixtureProjectHookDelete}
	case "MT-059":
		return "Delete badge ID `{{ .Values.badge_id }}` from project `{{ .Project.Path }}`.", []string{fixtureProjectBadgeDelete}
	case "MT-063":
		return "Publish all draft review notes for MR `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}`.", []string{fixtureDraftNotePublishAll}
	case "MT-066":
		return "Remove project ID `{{ .Values.target_project_id }}` from the CI job token allowlist of project `{{ .Project.ID }}`.", []string{fixtureJobTokenScopeProject}
	case "MT-069":
		return "Delete instance CI variable `{{ .Values.instance_ci_variable_key }}`.", []string{fixtureInstanceCIVariableDelete}
	default:
		return "", nil
	}
}

func baseDestructiveLateSinglePromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch id {
	case evalMT099:
		return "Delete branch `{{ .Branch.Name }}` from project `{{ .Project.Path }}`.", []string{fixtureBranchDelete}
	case "MT-100":
		return "Delete tag `{{ .Tag.Name }}` from project `{{ .Project.Path }}`.", []string{fixtureTagDelete}
	case "MT-101":
		return "Permanently delete pipeline `{{ .Pipeline.ID }}` from project `{{ .Project.Path }}`.", []string{fixturePipelineDelete}
	case "MT-102":
		return "Delete pipeline trigger token ID `{{ .Values.pipeline_trigger_id }}` from project `{{ .Project.Path }}`.", []string{fixturePipelineTriggerDelete}
	case "MT-103":
		return "Delete pipeline schedule ID `{{ .Values.pipeline_schedule_id }}` from project `{{ .Project.Path }}`.", []string{fixturePipelineScheduleDelete}
	case "MT-104":
		return "Block user ID `{{ .Values.user_id }}`.", []string{fixtureUserBlock}
	case "MT-106":
		return "Delete feature flag `{{ .Values.feature_flag_name }}` from project `{{ .Project.Path }}`.", []string{fixtureFeatureFlagDelete}
	case "MT-108":
		return "Delete wiki page `{{ .Values.wiki_slug }}` from project `{{ .Project.Path }}`.", []string{fixtureWikiDelete}
	case "MT-109":
		return "Remove award emoji ID `{{ .Award.ID }}` from merge request `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}`.", []string{fixtureMergeRequestAwardEmoji}
	case "MT-110":
		return "Remove award emoji ID `{{ .Award.ID }}` from issue `{{ .Issue.IID }}` in project `{{ .Project.Path }}`.", []string{fixtureIssueAwardEmoji}
	case "MT-111":
		return "Delete deploy key ID `{{ .DeployKey.ID }}` from project `{{ .Project.Path }}`.", []string{fixtureDeployKeyDelete}
	case "MT-112":
		return "Delete project deploy token ID `{{ .Values.deploy_token_id }}` from project `{{ .Project.Path }}`.", []string{fixtureDeployTokenDelete}
	case "MT-113":
		return "Delete commit discussion note `{{ .Values.note_id }}` from discussion `{{ .Values.discussion_id }}` on commit `{{ .Values.commit_sha }}` in project `{{ .Project.Path }}`.", []string{fixtureCommitDiscussionDeleteNote}
	default:
		return "", nil
	}
}

func baseDestructiveWorkflowPromptTemplateAndFixtures(id string) (template string, fixtures []string) {
	switch id {
	case "MS-013":
		return "Remove a temporary feature rollout from project `{{ .Project.Path }}`: inspect feature flag `{{ .Values.feature_flag_name }}`, list feature flag user lists, then delete the flag.", []string{fixtureFeatureFlagDelete}
	case "MS-003":
		return "Prepare a batch review for MR `{{ .MergeRequest.IID }}` in project `{{ .Project.Path }}`: inspect the MR, inspect changes, create a draft note saying `Please add a regression test`, then publish all draft notes.", []string{fixtureMergeRequest}
	case "MS-004":
		return "Clean up release `{{ .Release.TagName }}` in project `{{ .Project.Path }}`: verify the tag, verify the release, list release links, delete the release, then delete the tag.", []string{fixtureReleaseDelete}
	case "MS-007":
		return "Clean up an obsolete package in project `{{ .Project.Path }}`: list generic packages, list files for package ID `{{ .Package.ID }}`, then delete package ID `{{ .Package.ID }}`.", []string{fixturePackageDelete}
	case "MS-018":
		return "Exercise release asset-link CRUD in project `{{ .Project.Path }}`: use release create directly to create release `{{ .Release.TagName }}` from ref `{{ .Branch.Default }}` named `{{ .Release.Name }}` without creating a tag separately and without passing `assets`; after the release exists, add asset link `{{ .Values.release_link_name }}` with URL `{{ .Values.release_link_url }}`, fetch the returned link with link get, update the link URL to `{{ .Values.release_link_updated_url }}`, delete the link, delete the release, then delete the tag.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MS-025":
		return "Exercise scoped project CI variable CRUD in project `{{ .Project.Path }}`: create variable `{{ .Values.ci_variable_key }}` with value `crud-value-1` and environment scope `review/eval`, list variables, update the scoped variable to value `crud-value-2`, then delete that same scoped variable.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MS-026":
		return "Exercise scoped group CI variable CRUD in group `{{ .Group.Path }}`: create variable `{{ .Values.group_ci_variable_key }}` with value `group-crud-value-1` and environment scope `review/eval`, get it using top-level `environment_scope`, update it to value `group-crud-value-2`, then delete that same scoped variable.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MS-027":
		return "Exercise merge request note CRUD in project `{{ .Project.Path }}`: add note `eval-mr-note` to merge request `{{ .MergeRequest.IID }}`, fetch the created note using the returned note ID, update it to `eval-mr-note-updated`, then delete it.", []string{fixtureMergeRequest}
	case "MS-028":
		return "Exercise branch protection lifecycle in project `{{ .Project.Path }}`: create branch `{{ .Branch.Name }}` from `{{ .Branch.Default }}`, protect it with Maintainer push and merge access, fetch the protected branch, update it to allow force push, unprotect it, then delete the branch.", []string{fixtureBranchProtectionLifecycle}
	case "MS-031":
		return "Exercise project deploy key lifecycle in project `{{ .Project.Path }}`: add deploy key `{{ .Values.deploy_key_title }}` with public key `{{ .Values.deploy_key_key }}`, fetch it with deploy key get using the returned deploy key ID, update the title to `{{ .Values.deploy_key_updated_title }}`, then delete it.", []string{fixtureDeployKeyLifecycle}
	case "MS-033":
		return "Exercise merge request time tracking and emoji in project `{{ .Project.Path }}`: set estimate `1h` on merge request `{{ .MergeRequest.IID }}`, add spent time `15m`, add award emoji `eyes`, list MR awards, delete the returned award emoji, reset spent time, then reset the estimate.", []string{fixtureMergeRequest}
	case "MS-034":
		return "Exercise project member lifecycle in project `{{ .Project.Path }}`: add user ID `{{ .Values.user_id }}` as Reporter, fetch that project member, edit access level to Developer, then remove the member.", []string{fixtureUserBlock}
	case "MS-023":
		return "Exercise wiki CRUD in project `{{ .Project.Path }}`: create wiki page titled `{{ .Values.wiki_title }}` with content containing `eval-crud-wiki`, fetch the created page with the returned slug, update its title to `{{ .Values.wiki_title_v2 }}`, then delete it.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MS-029":
		return "Exercise feature flag and user-list lifecycle in project `{{ .Project.Path }}`: create feature flag user list `{{ .Values.feature_flag_user_list_name }}` with user IDs `u1,u2`, fetch it, update the user IDs to `u2,u3`, create feature flag `{{ .Values.feature_flag_crud_name }}` using version `new_version_flag`, fetch the flag, update it inactive, delete the flag, then delete the user list.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	case "MS-035":
		return "Exercise group label lifecycle in group `{{ .Group.Path }}`: create label `{{ .Values.group_label_name }}` with color `#1f75cb`, fetch it by label ID or name, rename it to `{{ .Values.group_label_name_v2 }}`, then delete it.", []string{fixtureBootstrapProject, fixtureAttemptNames}
	default:
		return "", nil
	}
}
