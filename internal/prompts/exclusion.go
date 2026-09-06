package prompts

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// registrar is the subset of *mcp.Server that prompt registration uses.
//
// The registration helpers take this interface rather than the concrete server
// so a wrapper can decide, per prompt, whether the call reaches the server at
// all. That is the only place the decision fits: every prompt in this package
// is registered through [addPrompt], and a filter there covers all 37 without
// any of them knowing it exists.
type registrar interface {
	AddPrompt(prompt *mcp.Prompt, handler mcp.PromptHandler)
}

// RegisterOptions narrows the prompt surface the way --exclude-tools narrows
// the tool surface.
//
// Prompts were the third request path to GitLab and the last one an operator
// could not narrow. A prompt runs handler code holding the caller's credential
// and returns the same data a tool would: review_mr reads the merge request and
// its diffs, team_overview reads a group's members. So an operator who excluded
// gitlab_mr_changes_get removed it from tools/list and from resources/read, and
// was still served the identical diffs by asking for the review_mr prompt. That
// mattered most where exclusion is the recommended mitigation for a tool, since
// the mitigation covered two thirds of the ways to reach it.
type RegisterOptions struct {
	// ExcludedActions holds canonical catalog action IDs ("merge_request.get")
	// the operator removed on the active surface.
	//
	// Canonical IDs rather than tool names on purpose, and for the reason
	// [resources.RegisterOptions] gives: --exclude-tools accepts a group name,
	// an individual tool name or an action ID, and only the action catalog can
	// resolve those three spellings to the same action. The caller resolves
	// them once against the catalog it just filtered, so this package needs one
	// table keyed by one kind of name.
	ExcludedActions []string
}

// promptBackingActions maps every prompt this package registers to the
// canonical catalog actions that return the same GitLab data through a tool.
//
// Hand-kept, like the resource table it mirrors, because nothing relates a
// prompt to the actions its handler calls: the handlers reach the GitLab client
// directly rather than dispatching through the catalog, so the relationship
// exists only in what the code does. Keeping it here, beside [Register], is
// also the point: it is the one place a reviewer can see which tools a prompt
// duplicates. Two tests hold it honest, one failing when a registered prompt is
// missing from the table and the other when an action ID in it is not in the
// real catalog.
//
// A prompt lists every action whose data it reads, not just its headline one.
// project_health_check reads the project, its latest pipeline, its merge
// requests and its branches, and an operator who excluded any one of those
// asked for that data not to be reachable.
var promptBackingActions = map[string][]string{
	// Project prompts.
	"summarize_mr_changes":      {"mr_review.changes_get"},
	"review_mr":                 {"merge_request.get", "mr_review.changes_get"},
	"summarize_pipeline_status": {"pipeline.latest", "job.list"},
	"suggest_mr_reviewers":      {"project.members"},
	"generate_release_notes":    {"repository.compare"},
	"summarize_open_mrs":        {"merge_request.list"},
	"project_health_check":      {"project.get", "pipeline.latest", "merge_request.list", "branch.list"},
	"compare_branches":          {"repository.compare"},
	"daily_standup":             {"user.event_list_contributions", "user.contribution_events", "merge_request.list", "issue.list"},
	"mr_risk_assessment":        {"merge_request.get", "mr_review.changes_get"},
	"team_member_workload":      {"merge_request.list", "issue.list"},
	"user_stats":                {"user.current", "user.list", "merge_request.list", "issue.list"},

	// Cross-project prompts (personal dashboards).
	"my_open_mrs":         {"merge_request.list_global"},
	"my_pending_reviews":  {"merge_request.list_global"},
	"my_issues":           {"issue.list_all"},
	"my_activity_summary": {"merge_request.list_global"},

	// Team prompts (group-level).
	"user_activity_report": {"merge_request.list_global"},
	"team_overview":        {"group.members", "merge_request.list_group"},
	"group_mr_dashboard":   {"merge_request.list_group"},
	"reviewer_workload":    {"group.members", "merge_request.list_group"},

	// Project report prompts.
	"branch_mr_summary":       {"merge_request.list"},
	"project_activity_report": {"user.event_list_project", "merge_request.list", "issue.list"},
	"mr_discussion_health":    {"merge_request.list", "mr_review.discussion_list"},
	"unassigned_items":        {"merge_request.list", "issue.list"},
	"stale_items_report":      {"merge_request.list", "issue.list"},

	// Analytics prompts.
	"merge_velocity":    {"merge_request.list"},
	"release_readiness": {"merge_request.list", "mr_review.discussion_list"},
	"release_cadence":   {"release.list"},
	"weekly_team_recap": {"merge_request.list_group", "issue.list_group"},

	// Milestone, label and contributor prompts.
	"milestone_progress":       {"project.milestone_list", "project.milestone_issues", "project.milestone_merge_requests"},
	"label_distribution":       {"project.label_list"},
	"group_milestone_progress": {"group.group_milestone_list", "group.group_milestone_issues", "group.group_milestone_merge_requests"},
	"project_contributors":     {"repository.contributors"},

	// Git workflow prompts.
	"audit_commit_hygiene":   {"repository.compare"},
	"mr_description_quality": {"merge_request.get", "mr_review.changes_get"},

	// Project audit prompts.
	"audit_project_workflow": {
		"project.get",
		"project.label_list",
		"project.milestone_list",
		"template.project_template_list",
	},
	"audit_project_full": {
		"project.get",
		"branch.list_protected",
		"project.members",
		"project.label_list",
		"project.milestone_list",
		"template.project_template_list",
		"project.push_rule_get",
		"project.hook_list",
	},
}

// excludingRegistrar drops the prompts whose data an excluded action served,
// and forwards the rest unchanged.
//
// It wraps the registrar rather than editing [Register] so the registration
// calls stay a flat list of what this server offers, and the narrowing stays
// one decision in one place.
type excludingRegistrar struct {
	inner    registrar
	excluded map[string]struct{}
}

func (r *excludingRegistrar) AddPrompt(prompt *mcp.Prompt, handler mcp.PromptHandler) {
	if _, blocked := r.excluded[prompt.Name]; blocked {
		return
	}
	r.inner.AddPrompt(prompt, handler)
}

// registrarFor returns the registrar the registration calls should be given
// for these options: the plain one when nothing is excluded, and a filtering
// wrapper otherwise.
func registrarFor(inner registrar, opts []RegisterOptions) registrar {
	excluded := excludedPromptNames(opts)
	if len(excluded) == 0 {
		return inner
	}
	return &excludingRegistrar{inner: inner, excluded: excluded}
}

// excludedPromptNames returns the names of the prompts whose data an excluded
// action served.
//
// A prompt goes when *any* of its backing actions was excluded, not only when
// all of them were, which is the same conservative reading the resource surface
// takes. The operator removed a way to read that data; a prompt that reads it
// with the same credential is what the exclusion was meant to close, and a
// prompt stripped of one of its sources would report on data it could no longer
// see. A prompt missing from the table is kept, because withholding a prompt
// because a table is incomplete would be a worse failure than the one this
// closes, and the drift test is what keeps the table complete.
func excludedPromptNames(opts []RegisterOptions) map[string]struct{} {
	actions := make(map[string]struct{})
	for _, opt := range opts {
		for _, action := range opt.ExcludedActions {
			if action = strings.TrimSpace(action); action != "" {
				actions[action] = struct{}{}
			}
		}
	}
	if len(actions) == 0 {
		return nil
	}
	excluded := make(map[string]struct{})
	for name, backing := range promptBackingActions {
		for _, action := range backing {
			if _, ok := actions[action]; ok {
				excluded[name] = struct{}{}
				break
			}
		}
	}
	return excluded
}

// attributedRegistrar refuses a prompt this server could not attribute to a
// credential, before the handler runs.
//
// Every handler here resolves the caller's client from the request context,
// because one server is shared by every credential of a configuration shape and
// the client captured at registration is the credential-less one. A prompts/get
// arriving with none would otherwise run the whole handler against a transport
// that refuses everything and report whatever GitLab error each helper happened
// to produce, none of which is what went wrong. The refusal says the one true
// thing instead, in the same words the tool and resource surfaces use.
//
// It wraps the registrar rather than [addPrompt] so that the check is stated
// once for all 37, including the ones that read several endpoints and would
// otherwise fail differently depending on which read came first.
type attributedRegistrar struct {
	inner registrar
	base  *gitlabclient.Client
}

func (r *attributedRegistrar) AddPrompt(prompt *mcp.Prompt, handler mcp.PromptHandler) {
	r.inner.AddPrompt(prompt, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		if r.base.For(ctx).IsUnbound() {
			return nil, toolutil.UnattributedRequestErrorFor(ctx)
		}
		return handler(ctx, req)
	})
}

// attributed wraps inner so every prompt registered through it refuses an
// unattributed request. See [attributedRegistrar].
func attributed(inner registrar, base *gitlabclient.Client) registrar {
	return &attributedRegistrar{inner: inner, base: base}
}
