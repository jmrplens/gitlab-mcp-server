// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

// A test file is client-go's own and is never read.
func (s *IssuesService) FromATest() (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDIssues, nil))
}
