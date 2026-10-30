// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"net/url"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// wantRefusalAdvice is how every refusal of [RejectTraversalArg] ends, spelled
// out rather than read from the code under test so that a change to either
// side is seen.
const wantRefusalAdvice = `; pass a value with no "." or ".." segment, written plainly or percent-encoded`

// TestRejectTraversalArg_DotSegments_AreRefused asserts that a value whose
// "/"- or "\"-separated components include a "." or ".." segment, in the value
// as given or once its percent-encoding is undone, is refused with the whole
// expected error: the operation, the parameter, and the exact segment reported
// with the form it was found in. The "." of the advice that closes every
// refusal is not taken for the segment, since the message is compared whole.
func TestRejectTraversalArg_DotSegments_AreRefused(t *testing.T) {
	const decoded = " once its percent-encoding is undone"
	cases := []struct {
		name    string
		value   string
		segment string
	}{
		{"dot dot between names", "a/../b", `".."`},
		{"leading dot dot", "../b", `".."`},
		{"trailing dot dot", "a/..", `".."`},
		{"single dot", "x/./y", `"."`},
		{"bare dot dot", "..", `".."`},
		{"bare dot", ".", `"."`},
		{"backslash separated", `a\..\b`, `".."`},
		{"backslash separated single dot", `a\.\b`, `"."`},
		{"encoded dot dot lower", "a/%2e%2e/b", `".."` + decoded},
		{"encoded dot dot upper", "a/%2E%2E/b", `".."` + decoded},
		{"encoded single dot", "a/%2e/b", `"."` + decoded},
		{"encoded separator after dots", "..%2Fb", `".."` + decoded},
		{"encoded whole segment", "%2e%2e", `".."` + decoded},
		{"invalid escape before an encoded separator", "%zz/..%2Fx", `".."` + decoded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RejectTraversalArg("op", "artifact_path", tc.value)
			if err == nil {
				t.Fatalf("RejectTraversalArg(%q) = nil, want a refusal", tc.value)
			}
			want := "op: artifact_path has the segment " + tc.segment + wantRefusalAdvice
			if got := err.Error(); got != want {
				t.Errorf("RejectTraversalArg(%q) = %q, want %q", tc.value, got, want)
			}
		})
	}
}

// TestRejectTraversalArg_Message_IsRightForEachForm asserts the whole refusal
// for a value with a dot segment as given and for one whose segment appears
// only once its percent-encoding is undone: the operation, the parameter, the
// segment and the form it was found in, and what to pass instead.
func TestRejectTraversalArg_Message_IsRightForEachForm(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{
			name:  "as given",
			value: "a/../b",
			want:  `op: artifact_path has the segment ".."; pass a value with no "." or ".." segment, written plainly or percent-encoded`,
		},
		{
			name:  "once decoded",
			value: "a/%2e%2e/b",
			want:  `op: artifact_path has the segment ".." once its percent-encoding is undone; pass a value with no "." or ".." segment, written plainly or percent-encoded`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := RejectTraversalArg("op", "artifact_path", tc.value)
			if err == nil {
				t.Fatalf("RejectTraversalArg(%q) = nil, want a refusal", tc.value)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("RejectTraversalArg(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

// TestRejectTraversalArg_AgreesWithTheTransportRule asserts that the argument
// check refuses exactly the values the GitLab transport's detector reports, so
// the rule a handler applies and the rule every request is held to cannot come
// to differ.
func TestRejectTraversalArg_AgreesWithTheTransportRule(t *testing.T) {
	values := []string{
		"a/../b", "x/./y", "..", ".", `a\..\b`, "%2e%2e", "a/%2E%2e/b", "..%2Fb", "../%zz", "%zz/..%2Fx",
		"report.txt", "build/report.txt", "v1.2.3", "a..b", "...", "ok%zz", "a/%252e%252e/b", "%zz/v1%2E2",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			_, detected := gitlabclient.HasDotSegment(value)
			refused := RejectTraversalArg("op", "key", value) != nil
			if refused != detected {
				t.Errorf("RejectTraversalArg(%q) refused = %v, HasDotSegment found = %v", value, refused, detected)
			}
		})
	}
}

// TestRejectTraversalArg_OrdinaryValues_AreAllowed asserts that a value that is
// a plain name or an ordinary nested path, with no "." or ".." component, is
// allowed: a component is rejected only when it is exactly "." or "..".
func TestRejectTraversalArg_OrdinaryValues_AreAllowed(t *testing.T) {
	cases := []string{
		"report.txt",
		"build/output/report.txt",
		"v1.2.3",
		"feature/my-branch",
		"a..b",
		"..foo",
		"foo..",
		"...",
		"mit",
		"%2e%2ефайл", // decodes to "..файл", not a dot segment
	}
	for _, value := range cases {
		t.Run(value, func(t *testing.T) {
			if err := RejectTraversalArg("op", "artifact_path", value); err != nil {
				t.Errorf("RejectTraversalArg(%q) = %v, want nil", value, err)
			}
		})
	}
}

// TestRejectTraversalArg_InvalidPercentEncoding_ChecksEveryOtherForm asserts
// that an escape that cannot be undone hides no dot segment: one written as
// given is refused as such, one written escaped beside it, in another
// component or in the same one, is refused once the rest of the value is
// decoded, and a value with neither is allowed, the stray "%" included.
func TestRejectTraversalArg_InvalidPercentEncoding_ChecksEveryOtherForm(t *testing.T) {
	const decoded = " once its percent-encoding is undone"
	refused := []struct {
		name    string
		value   string
		segment string
	}{
		{"literal dot dot", "../%zz", `".."`},
		{"encoded separator in another component", "%zz/..%2Fx", `".."` + decoded},
		{"encoded dot dot in another component", `%2e%2e\%zz`, `".."` + decoded},
		{"encoded separator in the same component", "..%2F%zz", `".."` + decoded},
		{"encoded single dot", "%zz/%2E", `"."` + decoded},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			err := RejectTraversalArg("op", "key", tc.value)
			if err == nil {
				t.Fatalf("RejectTraversalArg(%q) = nil, want a refusal", tc.value)
			}
			want := "op: key has the segment " + tc.segment + wantRefusalAdvice
			if got := err.Error(); got != want {
				t.Errorf("RejectTraversalArg(%q) = %q, want %q", tc.value, got, want)
			}
		})
	}
	for _, value := range []string{"ok%zz", "%zz/v1%2E2", "100%", "a%2F%zz"} {
		t.Run("allows "+value, func(t *testing.T) {
			if err := RejectTraversalArg("op", "key", value); err != nil {
				t.Errorf("RejectTraversalArg(%q) = %v, want nil", value, err)
			}
		})
	}
}

// TestEscapePathSegment_Values_AreEscapedAsOneSegment asserts the exact form
// [EscapePathSegment] gives values with each kind of byte a path segment
// cannot carry as it is, "/" and "%" included, that a value with no such byte
// is returned unchanged, and that net/url keeps every result as written.
func TestEscapePathSegment_Values_AreEscapedAsOneSegment(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"no byte to escape", "TOKEN_A-1.2~x", "TOKEN_A-1.2~x"},
		{"empty", "", ""},
		{"a space", "my key", "my%20key"},
		{"a non-ASCII byte", "clé", "cl%C3%A9"},
		{"a slash", "a/b", "a%2Fb"},
		{"a backslash", `a\b`, "a%5Cb"},
		{"a percent sign", "100%", "100%25"},
		{"an escape as written", "a%2Fb", "a%252Fb"},
		{"a quote and braces", `"{a}"`, "%22%7Ba%7D%22"},
		{"a question mark and a hash", "a?b#c", "a%3Fb%23c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapePathSegment(tc.value)
			if got != tc.want {
				t.Errorf("EscapePathSegment(%q) = %q, want %q", tc.value, got, tc.want)
			}
			assertKeptAsEscaped(t, tc.value, got)
		})
	}
}

// TestEscapePathSegments_Values_AreEscapedPerSegment asserts the exact form
// [EscapePathSegments] gives values that span several segments: each segment
// is escaped as [EscapePathSegment] escapes it and the "/" between segments
// is kept, empty segments included, a value with no byte to escape is
// returned unchanged, and net/url keeps every result as written.
func TestEscapePathSegments_Values_AreEscapedPerSegment(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  string
	}{
		{"no byte to escape", "build/output/report.txt", "build/output/report.txt"},
		{"one segment", "report.txt", "report.txt"},
		{"a space in a segment", "build/my report.txt", "build/my%20report.txt"},
		{"a non-ASCII byte in a segment", "feature/über", "feature/%C3%BCber"},
		{"a backslash stays in its segment", `build\x/y`, "build%5Cx/y"},
		{"a percent sign", "fix/100%", "fix/100%25"},
		{"an escaped separator as written", "a%2Fb/c", "a%252Fb/c"},
		{"empty segments are kept", "a//b/", "a//b/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EscapePathSegments(tc.value)
			if got != tc.want {
				t.Errorf("EscapePathSegments(%q) = %q, want %q", tc.value, got, tc.want)
			}
			assertKeptAsEscaped(t, tc.value, got)
		})
	}
}

// assertKeptAsEscaped checks the two properties a handler relies on: escaped
// decodes back to value, so GitLab reads what the caller passed, and a request
// path that carries it beside an escaped full-path project is sent by net/url
// as written rather than encoded again from its decoded form, which would
// turn the project's "%2F" back into a "/".
func assertKeptAsEscaped(t *testing.T, value, escaped string) {
	t.Helper()
	if decoded, err := url.PathUnescape(escaped); err != nil || decoded != value {
		t.Errorf("url.PathUnescape(%q) = %q, %v, want %q", escaped, decoded, err, value)
	}
	u := url.URL{
		Path:    "/api/v4/projects/group/proj/x/" + value,
		RawPath: "/api/v4/projects/group%2Fproj/x/" + escaped,
	}
	if got := u.EscapedPath(); got != u.RawPath {
		t.Errorf("net/url sends %q as %q, want it as written", u.RawPath, got)
	}
}
