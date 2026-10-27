// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package nested

// A subdirectory is another package and is never read.
func (s *IssuesService) Nested() (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDIssues, nil))
}
