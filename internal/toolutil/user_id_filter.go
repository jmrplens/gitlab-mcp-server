// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"fmt"
	"strconv"
	"strings"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// parseUserIDFilter reads value the way GitLab's assignee_id and reviewer_id
// list filters take it: one user's numeric ID, or one of the literals None
// and Any, matched in any case and returned in the spelling GitLab compares.
// The result is an int64 or a [gl.UserIDValue], or nil for a filter left out.
//
// 0 is refused rather than sent. No user has it, and GitLab answers it
// differently by route: an empty list on the issue lists and the statistics,
// and on the merge request lists, where client-go drops a zero, every merge
// request. The refusal names the literal the caller meant.
func parseUserIDFilter(field string, value StringOrInt) (any, error) {
	text := strings.TrimSpace(value.String())
	if text == "" {
		return nil, nil //nolint:nilnil // a filter left out is no filter
	}
	if literal, ok := approverLiteral(value); ok {
		return gl.UserIDValue(literal), nil
	}
	// Separate ifs rather than the cases of one switch: a coverage profile puts
	// no block over a case expression, so mutation testing could not judge them.
	id, err := strconv.ParseInt(text, 10, 64)
	if err != nil || id < 0 {
		return nil, fmt.Errorf("%s must be a user ID, %q or %q, got %q", field, ApproverNone, ApproverAny, text)
	}
	if id == 0 {
		return nil, fmt.Errorf("%s 0 matches no user: pass %q to match what has nobody in the role, or %q to match what has somebody", field, ApproverNone, ApproverAny)
	}
	return id, nil
}

// AssigneeFilter converts an assignee_id filter into the value client-go's
// list options send, on the terms of [parseUserIDFilter]. It returns nil for a
// filter left out, so callers can assign the result unconditionally.
func AssigneeFilter(field string, value StringOrInt) (*gl.AssigneeIDValue, error) {
	parsed, err := parseUserIDFilter(field, value)
	if err != nil || parsed == nil {
		return nil, err
	}
	return gl.AssigneeID(parsed), nil
}

// ReviewerFilter converts a reviewer_id filter into the value client-go's
// merge request list options send, on the terms of [parseUserIDFilter]. It
// returns nil for a filter left out.
func ReviewerFilter(field string, value StringOrInt) (*gl.ReviewerIDValue, error) {
	parsed, err := parseUserIDFilter(field, value)
	if err != nil || parsed == nil {
		return nil, err
	}
	return gl.ReviewerID(parsed), nil
}

// Int64UserIDFilter reads a user ID filter for an option struct that types
// it *int64 and so cannot carry None or Any, which is how client-go's issue
// statistics options declare assignee_id (upstream register row 125). A user
// ID comes back as the pointer for the option. None and Any come back as a
// request option that writes the literal into the query under field, the
// option struct's field left nil. A filter left out returns neither, and a
// refusal returns only the error.
func Int64UserIDFilter(field string, value StringOrInt) (*int64, gl.RequestOptionFunc, error) {
	parsed, err := parseUserIDFilter(field, value)
	if err != nil || parsed == nil {
		return nil, nil, err
	}
	if id, ok := parsed.(int64); ok {
		return &id, nil, nil
	}
	literal := fmt.Sprint(parsed)
	return nil, func(req *retryablehttp.Request) error {
		query := req.URL.Query()
		query.Set(field, literal)
		req.URL.RawQuery = query.Encode()
		return nil
	}, nil
}
