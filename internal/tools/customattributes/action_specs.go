// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// action_specs.go holds what this package contributes to the canonical action
// catalog. The specs themselves are declared in internal/tools/adminspecs,
// which is the one place the gitlab_admin group is assembled; what stays here
// is the handler adaptation a spec routes to.
//
// This package used to declare a full set of specs of its own that nothing
// ever aggregated, so they reached no surface and no validation.

package customattributes

import (
	"context"
	"errors"
	"fmt"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// unresolvedError says a path or a username names nothing GitLab could find, on
// the read that looks it up. It is a type of its own because the same 404 from
// the attribute route after it means something else: the resource is there, and
// the attribute or the permission is not.
type unresolvedError struct {
	resourceType, identifier string
	// cause is GitLab's 404, nil where a username's list came back empty.
	cause error
}

// Error names what was looked up.
func (e *unresolvedError) Error() string {
	return fmt.Sprintf("no %s named %q was found", e.resourceType, e.identifier)
}

// Unwrap hands back GitLab's answer.
func (e *unresolvedError) Unwrap() error { return e.cause }

// AnswerUnresolved returns the route with a path or a username that names
// nothing answered by a not-found result in place of the error, on every later
// rebinding as well. Any other error passes through, a 404 of the attribute
// route with its hint among them.
func AnswerUnresolved(route toolutil.ActionRoute) toolutil.ActionRoute {
	return route.WrapHandler(func(next toolutil.ActionFunc) toolutil.ActionFunc {
		return func(ctx context.Context, input map[string]any) (any, error) {
			result, err := next(ctx, input)
			if miss, ok := errors.AsType[*unresolvedError](err); ok {
				return unresolvedOutput{ResourceType: miss.resourceType, Identifier: miss.identifier}, nil
			}
			return result, err
		}
	})
}

// DeleteOutput deletes a custom attribute and returns the canonical success message shape.
func DeleteOutput(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (toolutil.DeleteOutput, error) {
	if err := Delete(ctx, client, input); err != nil {
		return toolutil.DeleteOutput{}, err
	}
	return toolutil.DeleteOutput{Status: "success", Message: "Successfully deleted custom_attribute."}, nil
}
