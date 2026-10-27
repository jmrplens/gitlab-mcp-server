// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// work_items_test.go contains unit tests for the shared epic-backed work item
// GraphQL helpers (ResolveEpicGID, ResolveWorkItemGID).
package epicworkitems

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestResolveEpicGID verifies ResolveEpicGID returns a work item GID for a
// valid GraphQL namespace response and preserves the epic-specific not-found message.
func TestResolveEpicGID(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr string
	}{
		{
			name: "resolves id",
			body: `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}`,
			want: "gid://gitlab/WorkItem/1",
		},
		{
			name:    "missing epic",
			body:    `{"namespace":null}`,
			wantErr: "epic not found in group",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, tt.body)
				},
			}))

			got, err := ResolveEpicGID(t.Context(), client, "group", 1)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ResolveEpicGID() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveEpicGID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("ResolveEpicGID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResolveWorkItemGID_GraphQLError verifies GraphQL transport errors are
// returned unchanged so callers can wrap them with tool-specific guidance.
func TestResolveWorkItemGID_GraphQLError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		},
	}))

	_, err := ResolveWorkItemGID(t.Context(), client, "group", 1)
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("ResolveWorkItemGID() error = %v, want forbidden", err)
	}
}

// TestResolveWorkItemGID_NotFound verifies ResolveWorkItemGID returns the
// generic work-item not-found message when GraphQL returns no matching item.
func TestResolveWorkItemGID_NotFound(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":null}}`)
		},
	}))

	_, err := ResolveWorkItemGID(t.Context(), client, "group", 1)
	if err == nil || !strings.Contains(err.Error(), "work item not found") {
		t.Fatalf("ResolveWorkItemGID() error = %v, want work item not found", err)
	}
}

// gidResolver is the shape ResolveEpicGID and ResolveWorkItemGID share, so one
// table can hold both to the request they build and to the message they return.
type gidResolver func(context.Context, *gitlabclient.Client, string, int64) (string, error)

// gidResolvers names both exported resolvers with the prefix each puts on its
// not-found message, which is the only place the two differ.
var gidResolvers = []struct {
	name        string
	resolve     gidResolver
	notFoundMsg string
}{
	{name: "epic", resolve: ResolveEpicGID, notFoundMsg: "epic not found in group"},
	{name: "work_item", resolve: ResolveWorkItemGID, notFoundMsg: "work item not found in"},
}

// capturedRequest holds the GraphQL variables one request carried. The httptest
// handler runs on the server's own goroutine, so the values are taken under a
// mutex and asserted back on the test goroutine, where a failure is attributed
// to the test that caused it.
type capturedRequest struct {
	mu   sync.Mutex
	vars map[string]any
}

func (c *capturedRequest) record(vars map[string]any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vars = vars
}

func (c *capturedRequest) snapshot() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.vars
}

// TestResolveGID_Request_CarriesTheNamespaceAndIIDTheCallerAsked reads back the
// variables GitLab received and holds them to the pair the caller passed in.
//
// Translating a (namespace, iid) pair into the GID every later mutation
// addresses is the whole job of this package, and it is the one part of that
// job nothing else can notice going wrong: the resolver returns whatever id the
// response carries, so one asking about another namespace or another IID hands
// back a GID that is perfectly well-formed and belongs to the wrong work item,
// which the caller then edits or deletes. Asserting only the returned id leaves
// that unguarded — with the request values unasserted, sending `fullPath + "…"`
// and `iid + 1` kept the whole package green.
//
// The expected IID is written out as a literal rather than derived with
// strconv, so a resolver that changed base would move only one side of the
// comparison.
func TestResolveGID_Request_CarriesTheNamespaceAndIIDTheCallerAsked(t *testing.T) {
	const (
		askedPath        = "acme/platform"
		askedIID         = int64(42)
		iidOnTheWire     = "42"
		wantGID          = "gid://gitlab/WorkItem/77"
		wantGIDResponse  = `{"namespace":{"workItem":{"id":"` + wantGID + `"}}}`
		unreadableReason = "unreadable GraphQL variables"
	)

	for _, tt := range gidResolvers {
		t.Run(tt.name, func(t *testing.T) {
			var captured capturedRequest
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, r *http.Request) {
					vars, err := testutil.ParseGraphQLVariables(r)
					if err != nil {
						t.Errorf("ParseGraphQLVariables() error = %v", err)
						http.Error(w, unreadableReason, http.StatusInternalServerError)
						return
					}
					captured.record(vars)
					testutil.RespondGraphQL(w, http.StatusOK, wantGIDResponse)
				},
			}))

			got, err := tt.resolve(t.Context(), client, askedPath, askedIID)
			if err != nil {
				t.Fatalf("resolve() error = %v", err)
			}
			if got != wantGID {
				t.Errorf("resolve() = %q, want %q", got, wantGID)
			}

			vars := captured.snapshot()
			if vars["fullPath"] != askedPath {
				t.Errorf("fullPath variable = %v, want %q", vars["fullPath"], askedPath)
			}
			// GitLab types a work item's iid as String, so the resolver has to
			// send the decimal spelling of the number the caller gave it.
			if vars["iid"] != iidOnTheWire {
				t.Errorf("iid variable = %v, want %q", vars["iid"], iidOnTheWire)
			}
		})
	}
}

// TestResolveGID_NotFound_MessageNamesTheNamespaceAndIID asserts the not-found
// message carries the pair that was asked about, not merely its fixed prefix.
//
// That message is the whole of what a model is told when the epic or work item
// is absent, and it is what a caller re-reads to see which identifier it got
// wrong. Asserting the prefix alone holds while the interpolated values come
// from anywhere at all, so the two arguments are checked by the values a caller
// would recognize.
func TestResolveGID_NotFound_MessageNamesTheNamespaceAndIID(t *testing.T) {
	const (
		askedPath = "acme/platform"
		askedIID  = int64(9001)
		iidInText = "9001"
	)

	for _, tt := range gidResolvers {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
				"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
					testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":null}}`)
				},
			}))

			_, err := tt.resolve(t.Context(), client, askedPath, askedIID)
			if err == nil {
				t.Fatalf("resolve() error = nil, want %q", tt.notFoundMsg)
			}
			if !strings.Contains(err.Error(), tt.notFoundMsg) {
				t.Errorf("resolve() error = %v, want containing %q", err, tt.notFoundMsg)
			}
			if !strings.Contains(err.Error(), askedPath) {
				t.Errorf("resolve() error = %v, want naming namespace %q", err, askedPath)
			}
			if !strings.Contains(err.Error(), iidInText) {
				t.Errorf("resolve() error = %v, want naming IID %s", err, iidInText)
			}
		})
	}
}

// noteOwnerAnswer is the guard's read of a note as GitLab answers it: the
// type and id of what the note is on, and the address GitLab serves it at.
func noteOwnerAnswer(noteableType, address string) string {
	return `{"note":{"noteableId":9,"noteableType":"` + noteableType + `","url":"` + address + `"}}`
}

// TestCheckNoteOnEpic_AcceptsOnlyANoteOfTheEpicNamed holds the note guard to
// what GitLab 19.4 answers a note with (issue 1301). GitLab serves a note of a
// group's work item at /groups/<path>/-/work_items/<iid>, which names the
// group and the epic, and an epic's note reads as one of two noteable types:
// an Issue for a note made on the epic's work item, which is every note this
// server makes, and an Epic for one made through the legacy epic route. Both
// measured on GitLab 19.4.2 EE. A note is the epic's only when its address
// names the group and the iid the caller named, under the instance's own
// root, and its type is one of the two; any other is refused naming where
// GitLab says the note is, and a note GitLab will not show and a refused read
// are refused before anything is written.
func TestCheckNoteOnEpic_AcceptsOnlyANoteOfTheEpicNamed(t *testing.T) {
	const host = "https://gitlab.example.com"
	answer := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) { testutil.RespondGraphQL(w, http.StatusOK, body) }
	}
	tests := []struct {
		name    string
		note    http.HandlerFunc
		wantErr string
	}{
		{name: "a note on the epic's work item", note: answer(noteOwnerAnswer("Issue", host+"/groups/grp/-/work_items/5#note_100"))},
		{name: "a note made through the legacy epic route", note: answer(noteOwnerAnswer("Epic", host+"/groups/grp/-/work_items/5#note_100"))},
		{name: "the group path written in another case", note: answer(noteOwnerAnswer("Issue", host+"/groups/GRP/-/work_items/5#note_100"))},
		{name: "the address an epic had before epics were work items", note: answer(noteOwnerAnswer("Epic", host+"/groups/grp/-/epics/5#note_100"))},
		{
			name:    "a note on another epic of the group",
			note:    answer(noteOwnerAnswer("Issue", host+"/groups/grp/-/work_items/6#note_100")),
			wantErr: `opX: note 100 is not a note of epic &5 in group "grp": GitLab answers it as a note on Issue 9 (https://gitlab.example.com/groups/grp/-/work_items/6#note_100), so nothing was changed; find the epic's notes with group.epic_note_list`,
		},
		{
			name:    "a note on an epic of another group with the same iid",
			note:    answer(noteOwnerAnswer("Epic", host+"/groups/other/-/work_items/5#note_100")),
			wantErr: "a note on Epic 9 (https://gitlab.example.com/groups/other/-/work_items/5#note_100)",
		},
		{
			name:    "a note on an epic of a subgroup",
			note:    answer(noteOwnerAnswer("Issue", host+"/groups/grp/sub/-/work_items/5#note_100")),
			wantErr: "/groups/grp/sub/-/work_items/5#note_100",
		},
		{
			name:    "a note on a project work item whose address ends as the epic's does",
			note:    answer(noteOwnerAnswer("Issue", host+"/a/groups/grp/-/work_items/5#note_100")),
			wantErr: "/a/groups/grp/-/work_items/5#note_100",
		},
		{
			name:    "a note on a project issue",
			note:    answer(noteOwnerAnswer("Issue", host+"/grp/proj/-/issues/5#note_100")),
			wantErr: "/grp/proj/-/issues/5#note_100",
		},
		{
			name:    "a note on a merge request at an address shaped as the epic's",
			note:    answer(noteOwnerAnswer("MergeRequest", host+"/groups/grp/-/work_items/5#note_100")),
			wantErr: "a note on MergeRequest 9",
		},
		{
			name:    "a note GitLab gives no address",
			note:    answer(noteOwnerAnswer("Issue", "")),
			wantErr: "a note on Issue 9 ()",
		},
		{
			name:    "an address that does not parse",
			note:    answer(noteOwnerAnswer("Issue", "http://[::1")),
			wantErr: "a note on Issue 9 (http://[::1)",
		},
		{
			name: "no note, with GitLab's reason",
			note: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `{"data":{"note":null},"errors":[{"message":"The resource that you are attempting to access does not exist"}]}`)
			},
			wantErr: "does not exist",
		},
		{
			name:    "no note and no reason",
			note:    answer(`{"note":null}`),
			wantErr: "opX: note 100 not found, or not one this token can read; find the epic's notes with group.epic_note_list",
		},
		{
			name:    "the note read is refused",
			note:    func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) },
			wantErr: "verify note_id with group.epic_note_list",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{"note(id": tt.note}))
			got, err := CheckNoteOnEpic(t.Context(), client, "opX", "grp", 5, 100)
			if tt.wantErr == "" {
				if err != nil || !strings.HasSuffix(got, "-/work_items/5#note_100") && !strings.HasSuffix(got, "-/epics/5#note_100") {
					t.Errorf("CheckNoteOnEpic() = %q, %v, want the note's address and no error", got, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("CheckNoteOnEpic() error = %v, want containing %q", err, tt.wantErr)
			}
			if got != "" {
				t.Errorf("CheckNoteOnEpic() = %q beside a refusal, want no address", got)
			}
		})
	}
}

// TestCheckNoteOnEpic_ReadsOnlyTheNoteTheCallerNamed holds the guard to one
// read, of the note the caller named, as the global ID the note writes send.
// A guard that read another note would pass or refuse for a note nobody asked
// about, and one that resolved the epic first would withhold the four note
// writes from every fine-grained token, which no grant lets resolve a group's
// work item at GitLab 19.4.1 while every grant that may write a note may read
// it.
func TestCheckNoteOnEpic_ReadsOnlyTheNoteTheCallerNamed(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		"workItem(iid": func(w http.ResponseWriter, _ *http.Request) {
			t.Error("the guard resolved the epic, which no fine-grained token can at GitLab 19.4.1")
			testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}`)
		},
		"note(id": func(w http.ResponseWriter, r *http.Request) {
			vars, err := testutil.ParseGraphQLVariables(r)
			if err != nil || vars["id"] != "gid://gitlab/Note/4242" {
				t.Errorf("the note read carried %v (%v), want the id gid://gitlab/Note/4242", vars, err)
			}
			testutil.RespondGraphQL(w, http.StatusOK, noteOwnerAnswer("Issue", "https://gitlab.example.com/groups/grp/-/work_items/5#note_4242"))
		},
	}))
	got, err := CheckNoteOnEpic(t.Context(), client, "opX", "grp", 5, 4242)
	if err != nil || got != "https://gitlab.example.com/groups/grp/-/work_items/5#note_4242" {
		t.Fatalf("CheckNoteOnEpic() = %q, %v, want the note's address and no error", got, err)
	}
}

// TestNoteOfEpic_HoldsTheAddressToTheGroupAndIIDUnderTheRoot holds the
// address test on its own, where the instance's relative URL root can be set:
// GitLab writes the root it is served under in front of every address, and
// the guard reads the root from the API base the client was configured with,
// so an address is the epic's only when it is that root, then the group the
// caller named and the iid, with nothing else in front.
func TestNoteOfEpic_HoldsTheAddressToTheGroupAndIIDUnderTheRoot(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		root     string
		fullPath string
		want     bool
	}{
		{name: "under a relative URL root", address: "https://h/gitlab/groups/grp/-/work_items/5#note_1", root: "/gitlab", fullPath: "grp", want: true},
		{name: "the root missing from the address", address: "https://h/groups/grp/-/work_items/5#note_1", root: "/gitlab", fullPath: "grp"},
		{name: "a root the client was not configured with", address: "https://h/gitlab/groups/grp/-/work_items/5#note_1", fullPath: "grp"},
		{name: "a subgroup's epic named by its full path", address: "https://h/groups/grp/sub/-/work_items/5", fullPath: "grp/sub", want: true},
		{name: "a full path written with slashes around it", address: "https://h/groups/grp/sub/-/work_items/5", fullPath: "/grp/sub/", want: true},
		{name: "no full path", address: "https://h/groups//-/work_items/5", fullPath: "/"},
		{name: "another kind of item at the iid", address: "https://h/groups/grp/-/issues/5", fullPath: "grp"},
		{name: "an iid with more digits", address: "https://h/groups/grp/-/work_items/55", fullPath: "grp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noteOfEpic(tt.address, tt.root, tt.fullPath, 5); got != tt.want {
				t.Errorf("noteOfEpic(%q, %q, %q, 5) = %v, want %v", tt.address, tt.root, tt.fullPath, got, tt.want)
			}
		})
	}
}

// TestInstanceRoot_IsTheAPIBaseWithoutTheAPIPath holds the root the guard
// reads from the client's API base: whatever stands in front of /api/v4.
func TestInstanceRoot_IsTheAPIBaseWithoutTheAPIPath(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{base: "https://h/api/v4/", want: ""},
		{base: "https://h/gitlab/api/v4/", want: "/gitlab"},
		{base: "https://h/gitlab/api/v4", want: "/gitlab"},
		{base: "https://h/a/b/api/v4/", want: "/a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.base, func(t *testing.T) {
			base, err := url.Parse(tt.base)
			if err != nil {
				t.Fatalf("url.Parse(%q) error = %v", tt.base, err)
			}
			if got := instanceRoot(base); got != tt.want {
				t.Errorf("instanceRoot(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}

// TestRequireEpic_RefusesAnInputThatNamesNoEpic holds the check every epic
// note and discussion action but the lists starts with to the refusals those
// handlers wrote in place before it existed, word for word: an ended context
// as itself, then a missing group path, then a missing epic iid, each under
// the operation the caller named, and nothing for an input that names one.
func TestRequireEpic_RefusesAnInputThatNamesNoEpic(t *testing.T) {
	tests := []struct {
		name     string
		ended    bool
		fullPath string
		iid      int64
		wantErr  string
	}{
		{name: "an epic named", fullPath: "grp", iid: 5},
		{name: "a context that has ended", ended: true, fullPath: "grp", iid: 5, wantErr: context.Canceled.Error()},
		{name: "no group path", iid: 5, wantErr: "opX: full_path is required"},
		{name: "no group path and no iid", wantErr: "opX: full_path is required"},
		{name: "no iid", fullPath: "grp", wantErr: toolutil.ErrRequiredInt64("opX", "epic_iid").Error()},
		{name: "a negative iid", fullPath: "grp", iid: -1, wantErr: toolutil.ErrRequiredInt64("opX", "epic_iid").Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			if tt.ended {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := RequireEpic(ctx, "opX", tt.fullPath, tt.iid)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("RequireEpic() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("RequireEpic() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// TestResolveEpicForNote_ResolvesOnlyAnEpicAndABody holds the shared start of
// the two thread-opening creates: the input is refused before anything is
// sent when it names no epic or carries no body, a resolution that fails is
// reported under the operation with the caller's hint, and an epic GitLab
// resolves answers with its work item's global ID.
func TestResolveEpicForNote_ResolvesOnlyAnEpicAndABody(t *testing.T) {
	const hint = "hintX: verify full_path + iid with group.epic_list"
	tests := []struct {
		name     string
		fullPath string
		body     string
		resolve  http.HandlerFunc
		want     string
		wantErr  []string
	}{
		{
			name:     "an epic GitLab resolves",
			fullPath: "grp",
			body:     "note",
			resolve: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":{"workItem":{"id":"gid://gitlab/WorkItem/1"}}}`)
			},
			want: "gid://gitlab/WorkItem/1",
		},
		{name: "no group path", body: "note", wantErr: []string{"opX: full_path is required"}},
		{name: "no body", fullPath: "grp", wantErr: []string{"opX: body is required"}},
		{
			name:     "an epic GitLab does not hold",
			fullPath: "grp",
			body:     "note",
			resolve: func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondGraphQL(w, http.StatusOK, `{"namespace":null}`)
			},
			wantErr: []string{"opX", `epic not found in group "grp" with IID 5`, hint},
		},
		{
			name:     "a lookup GitLab refuses",
			fullPath: "grp",
			body:     "note",
			resolve:  func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) },
			wantErr:  []string{"opX", hint},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *gitlabclient.Client
			if tt.resolve == nil {
				client = testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			} else {
				client = testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{"workItem(iid": tt.resolve}))
			}
			got, err := ResolveEpicForNote(t.Context(), client, "opX", tt.fullPath, 5, tt.body, hint)
			if len(tt.wantErr) == 0 {
				if err != nil || got != tt.want {
					t.Errorf("ResolveEpicForNote() = %q, %v, want %q and no error", got, err, tt.want)
				}
				return
			}
			if err == nil || got != "" {
				t.Fatalf("ResolveEpicForNote() = %q, %v, want no ID and an error", got, err)
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("ResolveEpicForNote() error = %q, want it to contain %q", err.Error(), want)
				}
			}
		})
	}
}

// TestRequireNoteOnEpic_ChecksTheInputBeforeTheNote holds the start of the two
// deletes: an input naming no epic or no note is refused before anything is
// read, and one naming both is held to the note guard, whose answer it
// returns.
func TestRequireNoteOnEpic_ChecksTheInputBeforeTheNote(t *testing.T) {
	const address = "https://gitlab.example.com/groups/grp/-/work_items/5#note_100"
	tests := []struct {
		name     string
		fullPath string
		noteID   int64
		wantErr  string
	}{
		{name: "an epic and a note of it", fullPath: "grp", noteID: 100},
		{name: "no group path", noteID: 100, wantErr: "opX: full_path is required"},
		{name: "no note", fullPath: "grp", wantErr: toolutil.ErrRequiredInt64("opX", "note_id").Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var client *gitlabclient.Client
			if tt.wantErr != "" {
				client = testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
			} else {
				client = testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
					"note(id": func(w http.ResponseWriter, _ *http.Request) {
						testutil.RespondGraphQL(w, http.StatusOK, noteOwnerAnswer("Issue", address))
					},
				}))
			}
			got, err := RequireNoteOnEpic(t.Context(), client, "opX", tt.fullPath, 5, tt.noteID)
			if tt.wantErr == "" {
				if err != nil || got != address {
					t.Errorf("RequireNoteOnEpic() = %q, %v, want %q and no error", got, err, address)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr || got != "" {
				t.Errorf("RequireNoteOnEpic() = %q, %v, want no address and %q", got, err, tt.wantErr)
			}
		})
	}
}

// TestEpicNotAnswered_NamesWhatGitLabRefusedBeforeAMissingEpic holds the
// refusal for a notes read that carried no work item: GitLab's own reason
// when it refused the document, since a refused query looks exactly like an
// epic that is not there, and otherwise the epic the caller named.
func TestEpicNotAnswered_NamesWhatGitLabRefusedBeforeAMissingEpic(t *testing.T) {
	tests := []struct {
		name string
		errs []toolutil.GraphQLError
		want string
	}{
		{name: "no reason", want: `opX: epic not found in group "grp" with IID 5`},
		{
			name: "GitLab's reason",
			errs: []toolutil.GraphQLError{{Message: "Field 'nope' doesn't exist on type 'WorkItem'"}},
			want: toolutil.GraphQLTopLevelError("opX", []toolutil.GraphQLError{{Message: "Field 'nope' doesn't exist on type 'WorkItem'"}}).Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := EpicNotAnswered("opX", tt.errs, "grp", 5); err == nil || err.Error() != tt.want {
				t.Errorf("EpicNotAnswered() error = %v, want %q", err, tt.want)
			}
		})
	}
}

// TestQueryNoteOwner_Complexity_IsUnderGitLabsLimit holds the note guard's read
// under the complexity GitLab refuses a query above. The figure is the
// estimate testutil computes from the pinned schema; no GitLab has reported
// one for this document, so it is held under the limit rather than pinned.
func TestQueryNoteOwner_Complexity_IsUnderGitLabsLimit(t *testing.T) {
	got, err := testutil.GitLabQueryComplexity(queryNoteOwner, map[string]any{"id": "gid://gitlab/Note/1"})
	if err != nil {
		t.Fatalf("GitLabQueryComplexity() error = %v", err)
	}
	if got > testutil.GitLabAuthenticatedMaxComplexity {
		t.Errorf("queryNoteOwner costs %d, over the %d GitLab refuses a query above", got, testutil.GitLabAuthenticatedMaxComplexity)
	}
}
