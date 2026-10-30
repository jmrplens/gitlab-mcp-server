// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"fmt"
	"net/url"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// RejectTraversalArg returns an error, under operation and naming field, when
// value does not name one resource under the action's route: when one of its
// "/"- or "\"-separated components, in the value as given or once its
// percent-encoding is undone, is a "." or ".." path segment. An escape that
// cannot be undone, such as "%zz", is read as written and the rest of the
// value is still decoded, so "%zz/..%2Fx" is refused for its ".." as surely
// as "..%2Fx" is.
//
// A handler calls it for an argument that a client-go route places into the
// request path without escaping it, where the value may span several
// segments. A dot segment is refused because it changes the route: once the
// path is resolved, "." drops out and ".." removes the segment before it, so
// the request would name a route other than the one the action built. The
// value is refused rather than rewritten, because a rewritten value is not the
// one the caller passed. The GitLab transport applies the same rule to every
// request before it is sent, and both read it from
// [gitlabclient.HasDotSegment]; this check runs first and names the
// parameter, so the caller learns which argument to correct.
//
// It allows an ordinary nested path: a component is refused only when it is
// exactly "." or "..", so a value such as "build/report.txt" or "v1.2.3"
// passes.
func RejectTraversalArg(operation, field, value string) error {
	if segment, found := gitlabclient.HasDotSegment(value); found {
		return fmt.Errorf(`%s: %s has the segment %s; pass a value with no "." or ".." segment, written plainly or percent-encoded`, operation, field, segment)
	}
	return nil
}

// EscapePathSegment returns value percent-encoded as one segment of a request
// path, with [url.PathEscape]: every byte a path segment cannot carry as it
// is, "/" and "%" included, becomes an escape, so the whole value stays in
// the one segment its route gives it.
//
// A handler calls it for an argument that a client-go route places into the
// request path as given (the NoEscape path variable) where the route takes a
// single segment, such as a URL variable key or a template name. client-go
// builds the request from the route in its escaped form and in its decoded
// form, and net/url sends the escaped form only while it is a valid encoding;
// a byte that a path cannot carry as it is (a space, a non-ASCII byte, a
// quote, a brace) makes it encode the decoded form again instead, in which the
// "%2F" of another argument, such as a project's full path, has become a "/".
// Escaping the value first keeps the escaped form valid, so every argument
// reaches the path as client-go escaped it. Escaping "%" too means GitLab
// reads the value the caller passed rather than decoding it a second time.
//
// It escapes and does not check: a value with a "." or ".." segment is
// refused before it by [RejectTraversalArg], which reads the value as the
// caller passed it.
func EscapePathSegment(value string) string {
	return url.PathEscape(value)
}

// EscapePathSegments is [EscapePathSegment] for a route that takes a value
// across several segments, such as an artifact path or the ref of the
// artifacts archive route: each "/"-separated segment is escaped and the "/"
// between them is kept, as client-go's PathEscapeFileName does for the
// generic packages API. A value with no byte to escape is returned unchanged.
func EscapePathSegments(value string) string {
	segments := strings.Split(value, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
