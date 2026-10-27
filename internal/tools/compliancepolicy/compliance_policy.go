// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package compliancepolicy

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GetInput holds parameters for retrieving compliance policy settings.
type GetInput struct{}

// UpdateInput holds parameters for updating compliance policy settings.
type UpdateInput struct {
	CSPNamespaceID *int64 `json:"csp_namespace_id,omitempty" jsonschema:"Namespace ID for the compliance security policy project. Pass 0 to remove the designation. Every change locks the setting for ten minutes,required"`
}

// Output represents compliance policy settings.
type Output struct {
	toolutil.HintableOutput
	CSPNamespaceID *int64 `json:"csp_namespace_id"`
}

// Get retrieves the current admin compliance policy settings.
func Get(ctx context.Context, client *gitlabclient.Client, _ GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	result, _, err := client.GL().AdminCompliancePolicySettings.GetCompliancePolicySettings(gl.WithContext(ctx))
	if err != nil {
		return Output{}, permissionError("get compliance policy settings", err)
	}

	return Output{
		CSPNamespaceID: result.CSPNamespaceID,
	}, nil
}

// Update modifies the admin compliance policy settings.
func Update(ctx context.Context, client *gitlabclient.Client, in UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.CSPNamespaceID == nil {
		return Output{}, errors.New("csp_namespace_id is required: GitLab refuses an update without it. Pass 0 to remove the binding")
	}

	// GitLab requires the key on every update and removes the binding only on
	// null, which client-go cannot send: its pointer writes 0 as 0. A 0 is
	// therefore written into the body as null through a request option (issue
	// 1306, upstream register row 122).
	var nulls toolutil.BodyEdit
	opts := &gl.UpdateAdminCompliancePolicySettingsOptions{
		CSPNamespaceID: toolutil.NullWhenZero(in.CSPNamespaceID, "csp_namespace_id", &nulls),
	}
	result, _, err := client.GL().AdminCompliancePolicySettings.UpdateCompliancePolicySettings(opts, gl.WithContext(ctx), toolutil.EditJSONBody(nulls))
	if err != nil {
		return Output{}, updateError(err)
	}

	return Output{
		CSPNamespaceID: result.CSPNamespaceID,
	}, nil
}

// updateError gives a refused update the hint that fits it. Every change of
// the binding locks the setting for ten minutes, and GitLab answers a change
// inside that time with 422 "CSP namespace locked until HH:MM UTC", which is a
// wait rather than a namespace it declines (measured on GitLab 19.4.2).
//
// The branches are separate ifs rather than the cases of one switch, because a
// coverage profile puts no block over a case expression, so mutation testing
// could not judge the conditions that decide which hint a refusal gets.
func updateError(err error) error {
	const op = "update compliance policy settings"
	unprocessable := toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity)
	if unprocessable && toolutil.ContainsAny(err, "locked until") {
		return toolutil.WrapErrWithHint(op, err,
			"GitLab locks this setting for ten minutes after every change. Wait until the time GitLab names, then send the change again")
	}
	if unprocessable || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
		return toolutil.WrapErrWithHint(op, err,
			"csp_namespace_id must name an existing top-level group, or be 0 to remove the binding. A change made within ten minutes of the last one is refused while the setting is locked")
	}
	return permissionError(op, err)
}

// permissionError gives a refusal of a missing permission the hint naming
// what the settings need, and any other error GitLab's own words. It is keyed
// on toolutil.IsPermissionRefusal rather than on 403, because GitLab answers a
// missing permission with 401 at some routes (entry 55 of
// docs/development/upstream-bugs.md) and a hint keyed on 403 alone was never
// shown there, while a refusal of the token itself gets no role hint.
func permissionError(op string, err error) error {
	if toolutil.IsPermissionRefusal(err) {
		return toolutil.WrapErrWithHint(op, err, "the compliance policy settings require an administrator and an Ultimate license")
	}
	return toolutil.WrapErrWithMessage(op, err)
}
