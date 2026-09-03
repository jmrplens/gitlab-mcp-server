package resources

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterOptions narrows the resource surface the way --exclude-tools narrows
// the tool surface.
//
// The two surfaces are one request path each to the same GitLab data with the
// same credential, and until this existed only the tool one could be narrowed:
// an operator who excluded gitlab_project_get was still served the identical
// project object at gitlab://project/{id}. That mattered most where exclusion
// is the recommended mitigation for a tool, because the mitigation covered half
// the ways to reach it.
//
// Prompts are the other half of the same gap and are not covered here; see the
// package documentation.
type RegisterOptions struct {
	// ExcludedActions holds canonical catalog action IDs ("project.get") the
	// operator removed on the active surface.
	//
	// Canonical IDs rather than tool names on purpose: --exclude-tools accepts
	// a group name, an individual tool name or an action ID, and only the
	// action catalog can resolve those three spellings to the same action. The
	// caller resolves them once against the catalog it just filtered, so this
	// package needs one table keyed by one kind of name.
	ExcludedActions []string
}

// resourceBackingActions maps every resource this package registers to the
// canonical catalog actions that return the same GitLab data through a tool.
//
// It is a hand-kept table because no mapping exists in either direction:
// tool_manifest.go projects tools into a manifest, and nothing relates a
// resource template to the action serving its data. Keeping it here, beside
// registerAll, is also the point — it is the one place a reviewer can see the
// overlap between the two surfaces. Two tests hold it honest: one fails when a
// registered resource is missing from the table, the other when an action ID in
// the table is not in the real catalog.
var resourceBackingActions = map[string][]string{
	"gitlab://user/current":                                            {"user.current"},
	"gitlab://groups":                                                  {"group.list"},
	"gitlab://group/{group_id}":                                        {"group.get"},
	"gitlab://group/{group_id}/members":                                {"group.members"},
	"gitlab://group/{group_id}/projects":                               {"group.projects"},
	"gitlab://group/{group_id}/milestone/{milestone_iid}":              {"group.group_milestone_get"},
	"gitlab://group/{group_id}/label/{label_id}":                       {"group.group_label_get"},
	"gitlab://project/{project_id}":                                    {"project.get"},
	"gitlab://project/{project_id}/members":                            {"project.members"},
	"gitlab://project/{project_id}/issues":                             {"issue.list"},
	"gitlab://project/{project_id}/issue/{issue_iid}":                  {"issue.get"},
	"gitlab://project/{project_id}/pipelines/latest":                   {"pipeline.latest"},
	"gitlab://project/{project_id}/pipeline/{pipeline_id}":             {"pipeline.get"},
	"gitlab://project/{project_id}/pipeline/{pipeline_id}/jobs":        {"job.list"},
	"gitlab://project/{project_id}/labels":                             {"project.label_list"},
	"gitlab://project/{project_id}/label/{label_id}":                   {"project.label_get"},
	"gitlab://project/{project_id}/milestones":                         {"project.milestone_list"},
	"gitlab://project/{project_id}/milestone/{milestone_iid}":          {"project.milestone_get"},
	"gitlab://project/{project_id}/mr/{merge_request_iid}":             {"merge_request.get"},
	"gitlab://project/{project_id}/mr/{merge_request_iid}/notes":       {"mr_review.note_list"},
	"gitlab://project/{project_id}/mr/{merge_request_iid}/discussions": {"mr_review.discussion_list"},
	"gitlab://project/{project_id}/branches":                           {"branch.list"},
	"gitlab://project/{project_id}/branch/{branch}":                    {"branch.get"},
	"gitlab://project/{project_id}/releases":                           {"release.list"},
	"gitlab://project/{project_id}/release/{tag_name}":                 {"release.get"},
	"gitlab://project/{project_id}/tags":                               {"tag.list"},
	"gitlab://project/{project_id}/tag/{tag_name}":                     {"tag.get"},
	"gitlab://project/{project_id}/commit/{sha}":                       {"repository.commit_get"},
	"gitlab://project/{project_id}/file/{ref}/{+path}":                 {"repository.file_get", "repository.file_raw"},
	"gitlab://project/{project_id}/wiki/{slug}":                        {"wiki.get"},
	"gitlab://project/{project_id}/deployment/{deployment_id}":         {"environment.deployment_get"},
	"gitlab://project/{project_id}/environment/{environment_id}":       {"environment.get"},
	"gitlab://project/{project_id}/job/{job_id}":                       {"job.get"},
	"gitlab://snippet/{snippet_id}":                                    {"snippet.get"},
	"gitlab://project/{project_id}/snippet/{snippet_id}":               {"snippet.project_get"},
	"gitlab://project/{project_id}/feature_flag/{name}":                {"feature_flags.feature_flag_get"},
	"gitlab://project/{project_id}/deploy_key/{deploy_key_id}":         {"access.deploy_key_get"},
	"gitlab://project/{project_id}/board/{board_id}":                   {"project.board_get"},
}

// excludingRegistrar drops the resources whose data an excluded action served,
// and forwards the rest unchanged.
//
// It wraps the registrar rather than editing registerAll so the 38 registration
// calls stay a flat list of what this server offers, and the narrowing stays one
// decision in one place.
type excludingRegistrar struct {
	inner    registrar
	excluded map[string]struct{}
}

func (r *excludingRegistrar) AddResource(resource *mcp.Resource, handler mcp.ResourceHandler) {
	if _, blocked := r.excluded[resource.URI]; blocked {
		return
	}
	r.inner.AddResource(resource, handler)
}

func (r *excludingRegistrar) AddResourceTemplate(template *mcp.ResourceTemplate, handler mcp.ResourceHandler) {
	if _, blocked := r.excluded[template.URITemplate]; blocked {
		return
	}
	r.inner.AddResourceTemplate(template, handler)
}

// registrarFor returns the registrar registerAll should be given for these
// options: the plain one when nothing is excluded, and a filtering wrapper
// otherwise.
func registrarFor(inner registrar, opts []RegisterOptions) registrar {
	excluded := excludedResourceURIs(opts)
	if len(excluded) == 0 {
		return inner
	}
	return &excludingRegistrar{inner: inner, excluded: excluded}
}

// excludedResourceURIs returns the resource URIs and URI templates whose data
// an excluded action served.
//
// A resource goes when *any* of its backing actions was excluded, not only when
// all of them were. The operator removed a way to read that data; a second way
// to read the same data through the same credential is what the exclusion was
// meant to close, so the conservative reading is the correct one. A resource
// missing from the table is kept: withholding data because a table is
// incomplete would be a worse failure than the one this closes, and the drift
// test is what keeps the table complete.
func excludedResourceURIs(opts []RegisterOptions) map[string]struct{} {
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
	for uri, backing := range resourceBackingActions {
		for _, action := range backing {
			if _, ok := actions[action]; ok {
				excluded[uri] = struct{}{}
				break
			}
		}
	}
	return excluded
}
