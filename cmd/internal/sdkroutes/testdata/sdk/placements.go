// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

// The templates Placed, Mixed and combined format.
var (
	routeUserKeysPlaced = route("placed/%s/users/%s/keys/%d/v%s/%s/%s")
	routeMixed          = route("mixed/%s")
	routeCombined       = route("combined/%s")
)

// Mixed formats a local assigned once a piece the reading cannot name and
// once, as the second of a pair, one built out of a parameter, so the piece
// the reading cannot name is read as that parameter too, beside the
// spelling that carries it.
func (s *IssuesService) Mixed(key string) (*Issue, *Response, error) {
	which := opaque()
	_, which = 0, "w"+key
	return do[*Issue](s.client, withPath(routeMixed, which))
}

// Combined hands a helper one piece built out of two parameters, which the
// helper wraps in a call the reading cannot name: both fill its segment.
func (s *IssuesService) Combined(group, name string) (*Issue, *Response, error) {
	return s.combined(group + "/" + name)
}

func (s *IssuesService) combined(path string) (*Issue, *Response, error) {
	return do[*Issue](s.client, withPath(routeCombined, escape(path)))
}

// Placed hands its parameters to its template through every shape a
// placement reading carries a parameter through: a call the reading cannot
// name assigned to a local and reassigned from itself, the first of two
// results of such a call, a parameter in place and beside literal text, two
// parameters built into one piece, and a piece built out of none. The
// closure and the field it sets are assignments the reading reads past.
func (s *IssuesService) Placed(pid, user any, key int64, options ...RequestOptionFunc) (*Issue, *Response, error) {
	escaped := wrap(pid)
	escaped = wrap(escaped)
	run := func() { escaped = wrap(user) }
	run()
	id, err := parseID(user)
	s.client.field = err
	return do[*Issue](s.client, withPath(routeUserKeysPlaced, escaped, id, key, key, join(pid, key), opaque()))
}
