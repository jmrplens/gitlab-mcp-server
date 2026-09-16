//go:build e2e

// epic_test.go drives the pure halves of the epic builders: the three
// identifier translations the GraphQL documents need, which are where a
// mistake would send a mutation at the wrong object and read as GitLab
// refusing something it never saw.

package fixture

import "testing"

// TestWorkItemGID_SpellsTheGlobalID checks the one spelling every work item
// mutation takes.
func TestWorkItemGID_SpellsTheGlobalID(t *testing.T) {
	if got := workItemGID(42); got != "gid://gitlab/WorkItem/42" {
		t.Errorf("workItemGID(42) = %q, want the work item global id", got)
	}
}

// TestGidNumber_ReadsTheNumberOrRefuses covers both endings: a global id gives
// its number, and anything else is an error rather than a zero, since a zero
// note identifier would send a case to an endpoint about nothing.
func TestGidNumber_ReadsTheNumberOrRefuses(t *testing.T) {
	cases := []struct {
		name    string
		gid     string
		want    int64
		wantErr bool
	}{
		{name: "a note", gid: "gid://gitlab/Note/91", want: 91},
		{name: "a discussion note", gid: "gid://gitlab/DiscussionNote/7", want: 7},
		{name: "no slash", gid: "91", wantErr: true},
		{name: "nothing after the slash", gid: "gid://gitlab/Note/", wantErr: true},
		{name: "not a number", gid: "gid://gitlab/Note/abc", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := gidNumber(tc.gid)
			if (err != nil) != tc.wantErr {
				t.Fatalf("gidNumber(%q) error = %v, wantErr = %t", tc.gid, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("gidNumber(%q) = %d, want %d", tc.gid, got, tc.want)
			}
		})
	}
}

// TestDiscussionHex_ReadsTheIdentifierTheActionsTake checks that a discussion
// is handed to a case as the hexadecimal identifier GitLab's own listings
// show, and that a value that is not a global id is handed back untouched
// rather than truncated.
func TestDiscussionHex_ReadsTheIdentifierTheActionsTake(t *testing.T) {
	cases := []struct {
		name string
		gid  string
		want string
	}{
		{name: "a global id", gid: "gid://gitlab/Discussion/abc123", want: "abc123"},
		{name: "already hexadecimal", gid: "abc123", want: "abc123"},
		{name: "nothing after the slash", gid: "gid://gitlab/Discussion/", want: "gid://gitlab/Discussion/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := discussionHex(tc.gid); got != tc.want {
				t.Errorf("discussionHex(%q) = %q, want %q", tc.gid, got, tc.want)
			}
		})
	}
}
