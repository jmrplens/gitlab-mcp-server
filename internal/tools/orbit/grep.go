// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// orbitGrepOp is the operation a [Grep] failure is reported under.
const orbitGrepOp = "orbit_grep"

// grepPath is where GitLab serves the search, below /api/v4. client-go has
// no method for it.
const grepPath = "orbit/grep"

// The bounds [Grep] holds a call to before it sends anything.
const (
	// minGrepSearchBytes and maxGrepSearchBytes bound the trimmed search.
	// GitLab.com refused a search of one byte with 400 on 2026-10-29, in a
	// message that gives both bounds (47h in the capture).
	minGrepSearchBytes = 2
	maxGrepSearchBytes = 256
	// maxGrepLimit bounds how many results a search answers, as
	// ee/lib/api/orbit/data.rb declares it (30 in the capture); no captured
	// answer refuses one past it. GitLab's default is 20.
	maxGrepLimit = 50
	// maxGrepPerScope bounds how many results one scope answers, declared
	// beside maxGrepLimit and past which no captured answer refuses one. It
	// never exceeds the limit, which GitLab.com refused with 400 on
	// 2026-10-29 (47j).
	maxGrepPerScope = 10
	// maxGrepScopes bounds how many scopes one search names, a ceiling of
	// this server's own: GitLab declares none, lists ten scopes, and a longer
	// list repeats one or names an alias beside it.
	maxGrepScopes = 20
)

// gitlabPerScopeDefault is the per_scope GitLab applies to a call that gives
// none (ee/lib/api/orbit/data.rb), and refuses as above the limit when the
// limit is lower, so a call that gives a lower limit alone is sent with
// per_scope at the limit.
const gitlabPerScopeDefault = 5

// GrepInput is the search, the one group or project it runs over, the scopes
// it reads and how many results it answers.
type GrepInput struct {
	// Search is what to look for, 2 to 256 bytes once trimmed.
	Search string `json:"search" jsonschema:"What to search for, 2 to 256 bytes once trimmed,required"`
	// NamespaceID, ProjectID and FullPath name the group or project searched;
	// exactly one is given.
	NamespaceID int64  `json:"namespace_id,omitempty" jsonschema:"ID of the group to search. Give exactly one of namespace_id, project_id and full_path"`
	ProjectID   int64  `json:"project_id,omitempty" jsonschema:"ID of the project to search. Give exactly one of namespace_id, project_id and full_path"`
	FullPath    string `json:"full_path,omitempty" jsonschema:"Full path of the group or project to search. Give exactly one of namespace_id, project_id and full_path"`
	// Scopes are what to search: work_items, merge_requests, blobs, commits,
	// notes, milestones, users, projects, groups and branches, an alias or a
	// node name.
	Scopes []string `json:"scopes,omitempty" jsonschema:"What to search: work_items, merge_requests, blobs, commits, notes, milestones, users, projects, groups, branches, an alias (mr, issue, code, comment, branch) or a node name (MergeRequest, File). Every scope but branches when omitted. branches is a project's alone, for a user who can read its code, and a fine-grained access token needs read_branch there; otherwise it is answered unavailable, not_available_for_target"`
	// Limit bounds the results, 1 to 50, GitLab's 20 when zero.
	Limit int `json:"limit,omitempty" jsonschema:"Most results to answer, 1 to 50. GitLab answers 20 when omitted"`
	// PerScope bounds the results of one scope, 1 to 10 and never above
	// limit, GitLab's 5 when zero, or the limit when that is lower.
	PerScope int `json:"per_scope,omitempty" jsonschema:"Most results of one scope, 1 to 10 and never above limit. When omitted, 5, or limit when that is lower"`
	// ResponseFormat is raw or llm.
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"Response format: raw (structured JSON, the default) or llm (compact text). json is read as raw."`
}

// grepOptions is the query string of GET /orbit/grep.
type grepOptions struct {
	Search         string `url:"search"`
	NamespaceID    int64  `url:"namespace_id,omitempty"`
	ProjectID      int64  `url:"project_id,omitempty"`
	FullPath       string `url:"full_path,omitempty"`
	Scopes         string `url:"scopes,omitempty"`
	Limit          int    `url:"limit,omitempty"`
	PerScope       int    `url:"per_scope,omitempty"`
	ResponseFormat string `url:"response_format,omitempty"`
}

// GrepRequest is the search as GitLab ran it.
type GrepRequest struct {
	// Search is the search GitLab ran.
	Search string `json:"search"`
	// Limit and PerScope are the bounds GitLab applied.
	Limit    int `json:"limit"`
	PerScope int `json:"per_scope"`
}

// GrepTarget is the group or project searched.
type GrepTarget struct {
	// Type is Group or Project.
	Type string `json:"type"`
	// ID is its ID.
	ID int64 `json:"id"`
	// Token is its Orbit token, Group[id] or Project[id].
	Token string `json:"token,omitempty"`
	// FullPath is its full path.
	FullPath string `json:"full_path,omitempty"`
}

// GrepScopePagination says whether a scope held more than it answered.
type GrepScopePagination struct {
	// HasMore is true when the scope held more than per_scope.
	HasMore bool `json:"has_more"`
}

// GrepScopeReason says why GitLab did not search a scope.
type GrepScopeReason struct {
	// Code is GitLab's reason: limit_reached for a skipped scope,
	// not_available_for_search_type (answered on 2026-10-29),
	// not_available_for_target or search_type_error for an unavailable one,
	// and invalid_search, rate_limited, search_failed or timeout for an error.
	Code string `json:"code"`
}

// GrepScope is one scope GitLab searched or skipped.
type GrepScope struct {
	// Name is the scope.
	Name string `json:"name"`
	// Status is ok, or why the scope answered nothing: skipped, unavailable
	// or error.
	Status string `json:"status"`
	// Backend is the search that answered: advanced, zoekt or basic.
	Backend string `json:"backend,omitempty"`
	// ResultCount is how many results the scope answered.
	ResultCount int `json:"result_count,omitempty"`
	// Pagination says whether the scope held more.
	Pagination *GrepScopePagination `json:"pagination,omitempty"`
	// Reason says why a scope that is not ok answered nothing.
	Reason *GrepScopeReason `json:"reason,omitempty"`
}

// GrepReference places a hit: a work item or merge request by its id, iid
// and project, a group's work item by its group; a file by its project, path,
// branch and commit; a commit by its project and SHA; a branch by its project
// and name; a note by its id and what it is on.
type GrepReference struct {
	ID            int64   `json:"id,omitempty"`
	IID           int64   `json:"iid,omitempty"`
	ProjectID     int64   `json:"project_id,omitempty"`
	ProjectPath   string  `json:"project_path,omitempty"`
	NamespacePath string  `json:"namespace_path,omitempty"`
	Path          string  `json:"path,omitempty"`
	Branch        string  `json:"branch,omitempty"`
	CommitSHA     *string `json:"commit_sha,omitempty"`
	SHA           string  `json:"sha,omitempty"`
	Name          string  `json:"name,omitempty"`
	// Noteable is the token of the work item or merge request a note is on,
	// which orbit.context takes, and NoteableReference the reference to it, or
	// to the commit it is on.
	Noteable          string `json:"noteable,omitempty"`
	NoteableReference string `json:"noteable_reference,omitempty"`
}

// GrepMatch is one matching passage of a file.
type GrepMatch struct {
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Text      string `json:"text"`
}

// GrepResult is one hit.
type GrepResult struct {
	// Scope is the scope that found it.
	Scope string `json:"scope"`
	// Type is the hit's node type: WorkItem, MergeRequest, File, Branch and
	// the other searchable types.
	Type string `json:"type"`
	// ID is the id orbit.query filters on, as GitLab sends it: a number, a
	// commit's SHA, or null for a file or a branch.
	ID any `json:"id"`
	// Token is the hit's Orbit token, Type[id], or path@sha for a commit and
	// path@branch for a branch, null for a file. orbit.context takes a work
	// item's, merge request's, commit's and branch's.
	Token *string `json:"token"`
	// Reference places the hit.
	Reference GrepReference `json:"reference"`
	// Text is the hit's title or first line.
	Text string `json:"text,omitempty"`
	// State is a work item's or merge request's state.
	State string `json:"state,omitempty"`
	// Matches are a file's matching passages.
	Matches []GrepMatch `json:"matches,omitempty"`
}

// GrepOutput is what the search found.
type GrepOutput struct {
	toolutil.HintableOutput
	// ResponseFormat is the format that produced the answer.
	ResponseFormat string `json:"response_format,omitempty"`
	// Version is the version of GitLab's answer.
	Version string `json:"version,omitempty"`
	// Request is the search as GitLab ran it.
	Request *GrepRequest `json:"request,omitempty"`
	// Target is the group or project searched.
	Target *GrepTarget `json:"target,omitempty"`
	// Scopes are the scopes GitLab searched or skipped.
	Scopes []GrepScope `json:"scopes,omitempty"`
	// Results are the hits.
	Results []GrepResult `json:"results,omitempty"`
	// ResultCount is how many hits GitLab answered.
	ResultCount int `json:"result_count,omitempty"`
	// Truncated is true when the limit ended the search early or a scope held
	// more than per_scope.
	Truncated bool `json:"truncated,omitempty"`
	// FormattedText is the compact text GitLab answered the llm format with.
	FormattedText string `json:"formatted_text,omitempty"`
}

// Grep searches what one group or project holds through GitLab Orbit: work
// items, merge requests, code, commits, notes, milestones, users, projects,
// groups and, when asked, branches.
//
// Source: GET /api/v4/orbit/grep, which client-go has no method for. GitLab
// marks the route hidden and experimental and runs the searches /search runs,
// so it needs read_orbit and use_global_search, and the branches scope, which a
// fine-grained token is checked for inside the route (grep_service.rb at
// v19.6.1-ee), read_branch on the project too. The bounds its source declares
// are refused here first, naming the parameter: the search of 2 to 256 bytes,
// exactly one target, limit 1 to 50 and per_scope 1 to 10 and never above
// limit; so is a list of more scopes than [maxGrepScopes]. A limit below
// GitLab's default per_scope is sent with per_scope at the limit when the
// caller gives none, since GitLab refuses the two together. Every hit but a
// file carries its token and, for most types, the id orbit.query filters on.
func Grep(ctx context.Context, client *gitlabclient.Client, input GrepInput) (GrepOutput, error) {
	if err := ctx.Err(); err != nil {
		return GrepOutput{}, err
	}
	opts, err := grepQuery(input)
	if err != nil {
		return GrepOutput{}, err
	}
	format, sent, err := mcpFormat(input.ResponseFormat)
	if err != nil {
		return GrepOutput{}, err
	}
	if format != nil {
		opts.ResponseFormat = string(sent)
	}
	out := GrepOutput{ResponseFormat: responseFormatName(format)}
	if sent == gl.OrbitResponseFormatLLM {
		var text bytes.Buffer
		if err = restRequest(ctx, client, http.MethodGet, grepPath, opts, &text); err != nil {
			return GrepOutput{}, wrapGrepErr(err)
		}
		out.FormattedText = text.String()
		return out, nil
	}
	var answer GrepOutput
	if err = restRequest(ctx, client, http.MethodGet, grepPath, opts, &answer); err != nil {
		return GrepOutput{}, wrapGrepErr(err)
	}
	answer.ResponseFormat = out.ResponseFormat
	return answer, nil
}

// errGrepTarget is what [grepQuery] answers for a call that names no target
// or more than one.
var errGrepTarget = errors.New("orbit_grep: give exactly one of namespace_id, project_id and full_path")

// grepQuery checks a search against the bounds GitLab takes and builds its
// query string. The response format is set by the caller.
func grepQuery(input GrepInput) (*grepOptions, error) {
	search := strings.TrimSpace(input.Search)
	if n := len(search); n < minGrepSearchBytes || n > maxGrepSearchBytes {
		return nil, fmt.Errorf("%s: search must contain between %d and %d bytes once trimmed, and holds %d", orbitGrepOp, minGrepSearchBytes, maxGrepSearchBytes, n)
	}
	fullPath := strings.TrimSpace(input.FullPath)
	targets := 0
	for _, given := range []bool{input.NamespaceID != 0, input.ProjectID != 0, fullPath != ""} {
		if given {
			targets++
		}
	}
	if targets != 1 {
		return nil, errGrepTarget
	}
	if input.Limit < 0 || input.Limit > maxGrepLimit {
		return nil, fmt.Errorf("%s: limit is 1 to %d, and %d was given", orbitGrepOp, maxGrepLimit, input.Limit)
	}
	if input.PerScope < 0 || input.PerScope > maxGrepPerScope {
		return nil, fmt.Errorf("%s: per_scope is 1 to %d, and %d was given", orbitGrepOp, maxGrepPerScope, input.PerScope)
	}
	if input.Limit > 0 && input.PerScope > input.Limit {
		return nil, fmt.Errorf("%s: per_scope cannot exceed limit (%d over %d)", orbitGrepOp, input.PerScope, input.Limit)
	}
	scopes, err := grepScopes(input.Scopes)
	if err != nil {
		return nil, err
	}
	return &grepOptions{
		Search: search, NamespaceID: input.NamespaceID, ProjectID: input.ProjectID, FullPath: fullPath,
		Scopes: scopes, Limit: input.Limit, PerScope: grepPerScope(input.Limit, input.PerScope),
	}, nil
}

// grepPerScope returns the per_scope a search is sent with: the caller's, or
// the limit when the caller gave none and a limit below the default GitLab
// would apply and refuse, or none, which a limit of zero is too.
func grepPerScope(limit, perScope int) int {
	if perScope == 0 && limit < gitlabPerScopeDefault {
		return limit
	}
	return perScope
}

// grepNotFound is what orbit.grep answers for a 404, which GitLab never says of
// the search (ee/lib/api/orbit/data.rb). The route says it of the group or
// project the search names: it resolves to none, the user cannot read it, or
// Orbit is off for its top-level group. The check before every Orbit route
// says it of a target that exists too, when Orbit is off for the user. The
// card names the target by the parameter that gave it and keeps the checks on
// Orbit being served, which answer for the second. The arguments are read as
// the handler read them, which a call that reached GitLab decoded without an
// error.
func grepNotFound(params map[string]any) any {
	input, _ := toolutil.UnmarshalParams[GrepInput](params)
	target := "full_path " + strings.TrimSpace(input.FullPath)
	if input.NamespaceID != 0 {
		target = fmt.Sprintf("namespace_id %d", input.NamespaceID)
	} else if input.ProjectID != 0 {
		target = fmt.Sprintf("project_id %d", input.ProjectID)
	}
	return orbitNotFoundOutput{Resource: "GitLab Orbit Search Target", Identifier: target}
}

// grepInput is what orbit.grep says about its parameters: exactly one of the
// three that name the target, which the handler refuses a call without, and
// that namespace_id is a group's. The catalog fills in for a namespace_id with
// no guidance one that offers a user namespace too, while GitLab looks the id
// up as a group's and searches no project of a user namespace either
// (ee/lib/analytics/orbit/target_resolver.rb at v19.6.1-ee).
func grepInput() orbitInput {
	return orbitInput{
		guidance: map[string]toolutil.ParameterGuidance{
			"namespace_id": {
				SemanticRole:     "scope_group",
				ValueSource:      "Numeric ID of the group to search, which group.get returns.",
				CommonConfusions: []string{"A user namespace is answered not found, and so is a project in one."},
				ExampleBinding:   "params.namespace_id:42",
			},
		},
		overrides: []toolutil.InputSchemaOverride{
			toolutil.SchemaOneOfRequired("namespace_id", "project_id", "full_path"),
		},
	}
}

// grepScopes joins the scopes into the comma list GitLab takes, trimmed and
// with the empty ones dropped, and refuses a list longer than
// [maxGrepScopes]. Which names GitLab takes is GitLab's to judge: it answers
// one it does not take 400 "scopes does not have a valid value".
func grepScopes(scopes []string) (string, error) {
	kept := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if trimmed := strings.TrimSpace(scope); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) > maxGrepScopes {
		return "", fmt.Errorf("%s: scopes names %d scopes and at most %d are taken: GitLab searches ten", orbitGrepOp, len(kept), maxGrepScopes)
	}
	return strings.Join(kept, ","), nil
}

// grepRefusedHint follows a search GitLab refused with 400.
const grepRefusedHint = "GitLab's message names the parameter it refused. scopes takes work_items, merge_requests, blobs, commits, notes, milestones, users, projects, groups and branches, an alias or a node name"

// wrapGrepErr classifies a failed [Grep]: a 400 is GitLab's refusal of a
// parameter, followed by [grepRefusedHint], and anything else is classified
// the way every Orbit failure is, by [wrapOrbitErr].
func wrapGrepErr(err error) error {
	if toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint(orbitGrepOp, err, grepRefusedHint)
	}
	return wrapOrbitErr(orbitGrepOp, err)
}
