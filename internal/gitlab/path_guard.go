// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ErrRequestPathRefused reports that this server declined to send a request
// because its path has a "." or ".." segment, before anything was sent.
//
// It is a refusal by this server rather than a failure of the network or an
// answer from GitLab, which is the distinction a reader of the message needs:
// nothing was sent, and what to change is the argument the path was built
// from rather than the connection or the credential. The error that wraps it
// names the path and the segment.
var ErrRequestPathRefused = errors.New("request path refused")

// DotSegment is a "." or ".." component found by [HasDotSegment].
type DotSegment struct {
	// Name is the component itself, "." or "..".
	Name string
	// Decoded is true when the component appears only once the
	// percent-encoding of the text is undone, as in "%2e%2e" or "..%2F".
	Decoded bool
}

// String describes the segment for a message: its quoted name, followed by
// "once its percent-encoding is undone" when that is the only form it appears
// in, so the message is right whichever form the text took.
func (s DotSegment) String() string {
	if s.Decoded {
		return fmt.Sprintf("%q once its percent-encoding is undone", s.Name)
	}
	return strconv.Quote(s.Name)
}

// HasDotSegment reports whether text, a request path or a value headed for
// one, has a "." or ".." component, and returns the first one found.
//
// The text is read as given first and, when nothing is found there, once its
// percent-encoding is undone, so an escape written with either case of hex
// digits ("%2e", "%2E") and an escaped separator ("%2F", "%5C") are read as
// the characters they stand for. It is decoded once: a value encoded twice
// names the escape, not the dot. An escape that cannot be undone ("%zz") is
// kept as written and the rest of the text is still decoded
// ([dotSegmentEscapes]), so it does not hide a segment written escaped
// elsewhere in the text. Components are split on "/" and "\"; a component is
// a dot segment only when it is exactly "." or "..", so a component that
// names a resource ("v1.2.3", "a..b", "...") is never reported.
//
// It is the one statement of the rule: the transport applies it to every
// request path, and the handlers apply it to the arguments they place into a
// path unescaped, so the two cannot come to differ.
func HasDotSegment(text string) (DotSegment, bool) {
	if name, found := dotComponent(text); found {
		return DotSegment{Name: name}, true
	}
	if name, found := dotComponent(dotSegmentEscapes.Replace(text)); found {
		return DotSegment{Name: name, Decoded: true}, true
	}
	return DotSegment{}, false
}

// dotComponent returns the first component of text that is exactly "." or
// "..", and whether there is one.
func dotComponent(text string) (string, bool) {
	for _, component := range strings.FieldsFunc(text, isPathSeparator) {
		if component == "." || component == ".." {
			return component, true
		}
	}
	return "", false
}

// dotSegmentEscapes undoes the escapes that can make a dot segment: a dot and
// either separator, with either case of hex digits.
//
// [HasDotSegment] reads the decoded form of a text through it rather than
// through [net/url.PathUnescape], so that an escape that cannot be undone does
// not hide one that can. A component is a dot segment only when it is made of
// dots alone, and only these six escapes produce a dot or a separator; an
// escape of any other byte leaves its component something other than "." or
// "..", undone or not. So on text whose percent-encoding can be undone it
// finds what url.PathUnescape would, and on text where it cannot, such as
// "%zz/..%2Fx", it still reads "..%2Fx" as "../x", where url.PathUnescape
// returns an error and decodes nothing. The replacement is one pass, so an
// escaped escape ("%252e") stays an escape.
var dotSegmentEscapes = strings.NewReplacer(
	"%2e", ".", "%2E", ".",
	"%2f", "/", "%2F", "/",
	"%5c", `\`, "%5C", `\`,
)

// pathTraversalGuardTransport refuses to send a request whose path carries a
// dot segment ("." or ".."), in the path as it stands or once its
// percent-encoding is undone.
//
// A path argument names one resource under the route its action builds. Most
// routes escape each argument, so a "/" inside a value reaches the path as
// "%2F" and the value stays one segment; a few client-go routes place an
// argument into the path without escaping it (the NoEscape path variable), so
// there the value can span several segments. An escaped value is a dot
// segment too when it is "." or ".." as a whole, since [dotUnescapeTransport]
// sends its dots as dots. A dot segment is refused because it changes the
// route: once the path is resolved, "." drops out and ".." removes the segment
// before it, so the request would name a route other than the one the action
// built. Every surface and every handler reaches GitLab through this chain, so
// the rule holds for every action, including those added later, without a
// check of its own in each handler.
//
// It sits just inside [dotUnescapeTransport] so it reads the path after the
// "%2E" -> "." rewrite, and it refuses rather than rewrites: a rewritten path
// would not be the request the action built.
type pathTraversalGuardTransport struct {
	base http.RoundTripper
}

// RoundTrip refuses a request whose path carries a dot segment with an
// [ErrRequestPathRefused] that names the path and the segment, and otherwise
// delegates to the base transport unchanged.
//
// The refusal names the path as escaped and without its query, since the
// path is what was refused; it does not say which argument the segment came
// from, which only the handler knows.
func (t *pathTraversalGuardTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	escaped := req.URL.EscapedPath()
	if segment, found := HasDotSegment(escaped); found {
		return nil, fmt.Errorf("%w: the path %q has the segment %s", ErrRequestPathRefused, escaped, segment)
	}
	return t.base.RoundTrip(req)
}

// isPathSeparator reports whether r separates path components: a forward
// slash, or a backslash, which GitLab's own path check treats as a separator
// too.
func isPathSeparator(r rune) bool { return r == '/' || r == '\\' }
