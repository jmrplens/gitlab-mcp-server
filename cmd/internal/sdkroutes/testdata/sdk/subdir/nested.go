package nested

// A subdirectory is another package and is never read.
func (s *IssuesService) Nested() (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeProjectsIDIssues, nil))
}
