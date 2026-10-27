// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// group_ldap_test.go contains unit tests for the GitLab group LDAP link MCP tool handlers.
package groupldap

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

const (
	pathGroupLDAP = "/api/v4/groups/mygroup/ldap_group_links"
)

// TestList verifies the List handler.
// The mock GitLab API at /api/v4/groups/mygroup (GET) responds with HTTP OK.
// It asserts the returned output matches the expected fields.
func TestList(t *testing.T) {
	tests := []struct {
		name        string
		input       ListInput
		handler     http.HandlerFunc
		wantErr     bool
		wantCount   int
		wantFirstCN string
	}{
		{
			name:  "returns links successfully",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodGet)
				testutil.AssertRequestPath(t, r, pathGroupLDAP)
				testutil.RespondJSON(w, http.StatusOK, `[
					{"cn":"cn1","filter":"","group_access":30,"provider":"main","member_role_id":0},
					{"cn":"cn2","filter":"(uid=*)","group_access":40,"provider":"secondary","member_role_id":99}
				]`)
			}),
			wantCount:   2,
			wantFirstCN: "cn1",
		},
		{
			name:  "returns empty list",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusOK, `[]`)
			}),
			wantCount: 0,
		},
		{
			name:  "returns empty list when GitLab reports no linked groups",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v4/groups/mygroup" {
					testutil.RespondJSON(w, http.StatusOK, `{"id":1,"name":"mygroup"}`)
					return
				}
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"No linked LDAP groups found"}`)
			}),
			wantCount: 0,
		},
		{
			name:    "returns error when group_id is empty",
			input:   ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"server error"}`)
			}),
			wantErr: true,
		},
		{
			name:  "returns error on 404 not found",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Group Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := List(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(out.Links) != tt.wantCount {
				t.Fatalf("len(Links) = %d, want %d", len(out.Links), tt.wantCount)
			}
			if tt.wantFirstCN != "" && out.Links[0].CN != tt.wantFirstCN {
				t.Errorf("Links[0].CN = %q, want %q", out.Links[0].CN, tt.wantFirstCN)
			}
		})
	}
}

// TestList_FieldMapping verifies the List_FieldMapping handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestList_FieldMapping(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `[
			{"cn":"engineers","filter":"(dept=eng)","group_access":40,"provider":"ldap2","member_role_id":55}
		]`)
	}))

	out, err := List(context.Background(), client, ListInput{GroupID: "mygroup"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	link := out.Links[0]
	if link.CN != "engineers" {
		t.Errorf("CN = %q, want %q", link.CN, "engineers")
	}
	if link.Filter != "(dept=eng)" {
		t.Errorf("Filter = %q, want %q", link.Filter, "(dept=eng)")
	}
	if link.GroupAccess != 40 {
		t.Errorf("GroupAccess = %d, want 40", link.GroupAccess)
	}
	if link.Provider != "ldap2" {
		t.Errorf("Provider = %q, want %q", link.Provider, "ldap2")
	}
	if link.MemberRoleID != 55 {
		t.Errorf("MemberRoleID = %d, want 55", link.MemberRoleID)
	}
}

// TestAPIErrorIncludesActionableHint verifies LDAP API failures include the
// provider and cn/filter guidance needed to repair model-generated requests.
func TestAPIErrorIncludesActionableHint(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"provider is invalid"}`)
	}))

	_, err := Add(context.Background(), client, AddInput{
		GroupID:     "mygroup",
		CN:          "cn1",
		GroupAccess: 30,
		Provider:    "missing-provider",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"LDAP provider name", "cn or filter", "Premium/Ultimate", "Owner access"} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not contain %q", err.Error(), want)
			}
		})
	}
}

// TestLinkHints_NameOnlyWhatEachActionTakes verifies the hint a GitLab
// refusal of each LDAP action is wrapped with: only the add and the delete by
// CN or filter take the choice between a common name and a filter, so only
// they are told to send exactly one of the two. The list and the sync take
// the group alone, and the delete for a provider a common name and the
// provider; the hint all five shared told each of those to provide a filter,
// which the delete for a provider does not even accept.
func TestLinkHints_NameOnlyWhatEachActionTakes(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"refused"}`)
	}))
	ctx := context.Background()
	_, listErr := List(ctx, client, ListInput{GroupID: "mygroup"})
	_, syncErr := Sync(ctx, client, SyncInput{GroupID: "mygroup"})
	_, addErr := Add(ctx, client, AddInput{GroupID: "mygroup", CN: "engineers", GroupAccess: 30, Provider: "main"})
	deleteErr := DeleteWithCNOrFilter(ctx, client, DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "engineers", Provider: "main"})
	providerErr := DeleteForProvider(ctx, client, DeleteForProviderInput{GroupID: "mygroup", Provider: "main", CN: "engineers"})
	tests := []struct {
		name string
		err  error
		want []string
		deny []string
	}{
		{name: "list", err: listErr, want: []string{"group.get", "LDAP provider", "Premium/Ultimate", "Owner access"}, deny: []string{"cn or filter", "provide exactly"}},
		{name: "sync", err: syncErr, want: []string{"group.get", "LDAP provider", "Premium/Ultimate", "Owner access"}, deny: []string{"cn or filter", "provide exactly"}},
		{name: "add", err: addErr, want: []string{"exactly one of cn or filter", "LDAP provider name", "Premium/Ultimate", "Owner access"}},
		{name: "delete by cn or filter", err: deleteErr, want: []string{"exactly one of cn or filter", "LDAP provider name", "Premium/Ultimate", "Owner access"}},
		{name: "delete for a provider", err: providerErr, want: []string{"the cn and the LDAP provider name of the link", "Premium/Ultimate", "Owner access"}, deny: []string{"cn or filter"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("error = nil, want GitLab's refusal")
			}
			for _, want := range tc.want {
				if !strings.Contains(tc.err.Error(), want) {
					t.Errorf("error %q does not contain %q", tc.err.Error(), want)
				}
			}
			for _, deny := range tc.deny {
				if strings.Contains(tc.err.Error(), deny) {
					t.Errorf("error %q contains %q, which names a parameter the action does not take", tc.err.Error(), deny)
				}
			}
		})
	}
}

// TestAdd verifies the Add handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestAdd(t *testing.T) {
	roleID := int64(77)
	tests := []struct {
		name       string
		input      AddInput
		handler    http.HandlerFunc
		wantErr    bool
		wantCN     string
		wantFilter string
	}{
		{
			name: "creates link with CN",
			input: AddInput{
				GroupID:     "mygroup",
				CN:          "cn1",
				GroupAccess: 30,
				Provider:    "main",
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, pathGroupLDAP)
				testutil.RespondJSON(w, http.StatusCreated, `{"cn":"cn1","filter":"","group_access":30,"provider":"main"}`)
			}),
			wantCN: "cn1",
		},
		{
			name: "creates link with Filter instead of CN",
			input: AddInput{
				GroupID:     "mygroup",
				Filter:      "(uid=*)",
				GroupAccess: 20,
				Provider:    "secondary",
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, `{"cn":"","filter":"(uid=*)","group_access":20,"provider":"secondary"}`)
			}),
			wantFilter: "(uid=*)",
		},
		{
			name: "creates link with MemberRoleID",
			input: AddInput{
				GroupID:      "mygroup",
				CN:           "admins",
				GroupAccess:  50,
				Provider:     "main",
				MemberRoleID: &roleID,
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.RespondJSON(w, http.StatusCreated, `{"cn":"admins","group_access":50,"provider":"main","member_role_id":77}`)
			}),
			wantCN: "admins",
		},
		{
			name:    "returns error when group_id is empty",
			input:   AddInput{Provider: "main", GroupAccess: 30},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when provider is empty",
			input:   AddInput{GroupID: "mygroup", GroupAccess: 30},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when cn and filter are empty",
			input:   AddInput{GroupID: "mygroup", Provider: "main", GroupAccess: 30},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name: "returns error on API failure",
			input: AddInput{
				GroupID:     "mygroup",
				CN:          "cn1",
				GroupAccess: 30,
				Provider:    "main",
			},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusUnprocessableEntity, `{"message":"422 Unprocessable"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Add(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Add() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if tt.wantCN != "" && out.CN != tt.wantCN {
				t.Errorf("CN = %q, want %q", out.CN, tt.wantCN)
			}
			if tt.wantFilter != "" && out.Filter != tt.wantFilter {
				t.Errorf("Filter = %q, want %q", out.Filter, tt.wantFilter)
			}
		})
	}
}

// TestAdd_Selectors_SendOnlyTheOnesTheCallerSet holds the body Add puts on the
// wire, which is the only place the CN-or-filter choice can be observed: a
// GitLab LDAP link is defined by a common name or by a filter, each is copied
// into the request by a guard of its own, and the response a test writes for
// itself echoes nothing about what was asked for. A guard that stopped copying
// the selector the caller named would create a link nobody asked for, or have
// the request refused for naming neither, and the handler would still return
// the fixture and no error.
func TestAdd_Selectors_SendOnlyTheOnesTheCallerSet(t *testing.T) {
	memberRole := int64(42)
	tests := []struct {
		name  string
		input AddInput
		want  map[string]any
	}{
		{
			name:  "a link defined by a common name sends cn and no filter",
			input: AddInput{GroupID: "mygroup", CN: "engineers", GroupAccess: 30, Provider: "main"},
			want: map[string]any{
				"cn":           "engineers",
				"group_access": float64(30),
				"provider":     "main",
			},
		},
		{
			name:  "a link defined by a filter sends filter and no cn",
			input: AddInput{GroupID: "mygroup", Filter: "(dept=eng)", GroupAccess: 20, Provider: "secondary"},
			want: map[string]any{
				"filter":       "(dept=eng)",
				"group_access": float64(20),
				"provider":     "secondary",
			},
		},
		{
			name: "a member role travels beside the selector",
			input: AddInput{
				GroupID: "mygroup", CN: "admins",
				GroupAccess: 50, Provider: "main", MemberRoleID: &memberRole,
			},
			want: map[string]any{
				"cn":             "admins",
				"group_access":   float64(50),
				"provider":       "main",
				"member_role_id": float64(42),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got map[string]any
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					got = map[string]any{"undecodable body": err.Error()}
				}
				testutil.RespondJSON(w, http.StatusCreated, `{"cn":"","filter":"","group_access":0,"provider":""}`)
			}))

			if _, err := Add(context.Background(), client, tt.input); err != nil {
				t.Fatalf("Add() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("request body = %#v, want %#v", got, tt.want)
			}
		})
	}
}

// TestDeleteWithCNOrFilter validates the DeleteWithCNOrFilter handler covering
// success with various option combinations, validation, and API errors.
func TestDeleteWithCNOrFilter(t *testing.T) {
	tests := []struct {
		name    string
		input   DeleteWithCNOrFilterInput
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name:  "deletes by CN",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "cn1"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathGroupLDAP)
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:  "deletes by Filter",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", Filter: "(uid=*)"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:  "deletes with Provider set",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "cn1", Provider: "ldap2"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:  "deletes by Filter with Provider set",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", Filter: "(uid=*)", Provider: "ldap2"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:    "returns error when group_id is empty",
			input:   DeleteWithCNOrFilterInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when cn and filter are empty",
			input:   DeleteWithCNOrFilterInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "cn1"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			err := DeleteWithCNOrFilter(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteWithCNOrFilter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestDeleteWithCNOrFilter_Selectors_SendOnlyTheOnesTheCallerSet holds the
// query string the delete builds. GitLab answers this endpoint with 204 and no
// body, so the handler's own return value is the same whichever link was
// removed: the three selectors are each copied onto the request by a guard of
// its own, and a guard that inverted would delete by a selector the caller
// never named, or ask GitLab to match an empty one, while the test still saw
// success. Only the request says which link was meant.
func TestDeleteWithCNOrFilter_Selectors_SendOnlyTheOnesTheCallerSet(t *testing.T) {
	tests := []struct {
		name  string
		input DeleteWithCNOrFilterInput
		want  url.Values
	}{
		{
			name:  "deleting by common name names only the cn",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "engineers"},
			want:  url.Values{"cn": {"engineers"}},
		},
		{
			name:  "deleting by filter names only the filter",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", Filter: "(dept=eng)"},
			want:  url.Values{"filter": {"(dept=eng)"}},
		},
		{
			name:  "a provider travels beside the selector it qualifies",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "engineers", Provider: "ldap2"},
			want:  url.Values{"cn": {"engineers"}, "provider": {"ldap2"}},
		},
		{
			name:  "a provider travels beside a filter as well",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", Filter: "(dept=eng)", Provider: "ldap2"},
			want:  url.Values{"filter": {"(dept=eng)"}, "provider": {"ldap2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got url.Values
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				w.WriteHeader(http.StatusNoContent)
			}))

			if err := DeleteWithCNOrFilter(context.Background(), client, tt.input); err != nil {
				t.Fatalf("DeleteWithCNOrFilter() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("request query = %v, want %v", got, tt.want)
			}
		})
	}
}

// selectorRefusals are the two calls GitLab's exactly_one_of on cn and filter
// refuses, as each handler's error states them. A link that named both was
// sent to GitLab, which refused it as mutually exclusive, until the handlers
// refused that call themselves.
var selectorRefusals = []struct {
	name   string
	cn     string
	filter string
	want   []string
}{
	{name: "both", cn: "engineers", filter: "(dept=eng)", want: []string{"exactly one of cn or filter", "not both"}},
	{name: "neither", want: []string{"exactly one of cn or filter is required"}},
}

// countingClient returns a client whose mock GitLab counts the requests it is
// sent and answers each as the action would succeed, so a refusal that let a
// request through is seen as one sent rather than as a GitLab error.
func countingClient(t *testing.T) (*gitlabclient.Client, *atomic.Int32) {
	t.Helper()
	var requests atomic.Int32
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, `{"cn":"engineers","group_access":30,"provider":"main"}`)
	}))
	return client, &requests
}

// TestAdd_BothOrNeitherSelector_IsRefusedBeforeAnyRequest verifies that Add
// refuses a link naming both a common name and a filter, and one naming
// neither, with an error that says exactly one of the two is sent, and that
// neither call reaches GitLab.
func TestAdd_BothOrNeitherSelector_IsRefusedBeforeAnyRequest(t *testing.T) {
	for _, tc := range selectorRefusals {
		t.Run(tc.name, func(t *testing.T) {
			client, requests := countingClient(t)
			_, err := Add(context.Background(), client, AddInput{
				GroupID: "mygroup", CN: tc.cn, Filter: tc.filter, GroupAccess: 30, Provider: "main",
			})
			assertSelectorRefusal(t, "Add", err, tc.want, requests)
		})
	}
}

// TestDeleteWithCNOrFilter_BothOrNeitherSelector_IsRefusedBeforeAnyRequest
// verifies the same two refusals for the delete, which GitLab declares with
// the same exactly_one_of: a delete naming both would otherwise be sent with
// both in its query.
func TestDeleteWithCNOrFilter_BothOrNeitherSelector_IsRefusedBeforeAnyRequest(t *testing.T) {
	for _, tc := range selectorRefusals {
		t.Run(tc.name, func(t *testing.T) {
			client, requests := countingClient(t)
			err := DeleteWithCNOrFilter(context.Background(), client, DeleteWithCNOrFilterInput{
				GroupID: "mygroup", CN: tc.cn, Filter: tc.filter, Provider: "main",
			})
			assertSelectorRefusal(t, "DeleteWithCNOrFilter", err, tc.want, requests)
		})
	}
}

// assertSelectorRefusal holds a handler's answer to a selector refusal: an
// error carrying every wanted phrase, and no request sent.
func assertSelectorRefusal(t *testing.T, handler string, err error, want []string, requests *atomic.Int32) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s() error = nil, want the call refused", handler)
	}
	for _, phrase := range want {
		if !strings.Contains(err.Error(), phrase) {
			t.Errorf("%s() error = %q, want it to say %q", handler, err.Error(), phrase)
		}
	}
	if sent := requests.Load(); sent != 0 {
		t.Errorf("%s() sent %d request(s), want the call refused before any", handler, sent)
	}
}

// TestDeleteForProvider verifies the DeleteForProvider handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestDeleteForProvider(t *testing.T) {
	tests := []struct {
		name    string
		input   DeleteForProviderInput
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name:  "deletes link for provider successfully",
			input: DeleteForProviderInput{GroupID: "mygroup", Provider: "main", CN: "cn1"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodDelete)
				testutil.AssertRequestPath(t, r, pathGroupLDAP+"/main/cn1")
				w.WriteHeader(http.StatusNoContent)
			}),
		},
		{
			name:    "returns error when group_id is empty",
			input:   DeleteForProviderInput{Provider: "main", CN: "cn1"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when provider is empty",
			input:   DeleteForProviderInput{GroupID: "mygroup", CN: "cn1"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:    "returns error when cn is empty",
			input:   DeleteForProviderInput{GroupID: "mygroup", Provider: "main"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on 404 API response",
			input: DeleteForProviderInput{GroupID: "mygroup", Provider: "main", CN: "missing"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			err := DeleteForProvider(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteForProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSync verifies the Sync handler that triggers an LDAP synchronization.
// The mock GitLab API at /api/v4/groups/mygroup/ldap_sync (POST) responds with HTTP 202.
// It asserts success on the happy path and validation/API errors otherwise.
func TestSync(t *testing.T) {
	tests := []struct {
		name    string
		input   SyncInput
		handler http.HandlerFunc
		wantErr bool
	}{
		{
			name:  "triggers sync successfully",
			input: SyncInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				testutil.AssertRequestMethod(t, r, http.MethodPost)
				testutil.AssertRequestPath(t, r, "/api/v4/groups/mygroup/ldap_sync")
				w.WriteHeader(http.StatusAccepted)
			}),
		},
		{
			name:    "returns error when group_id is empty",
			input:   SyncInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on 404 API response",
			input: SyncInput{GroupID: "missing"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Not Found"}`)
			}),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := testutil.NewTestClient(t, tt.handler)
			out, err := Sync(context.Background(), client, tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Sync() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && out.Status != "success" {
				t.Errorf("Sync() status = %q, want success", out.Status)
			}
		})
	}
}

// ldapLinkHints is the guidance section every LDAP link card ends with.
const ldapLinkHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'group.ldap_link_delete' to remove this link\n"

// ldapListHints is the guidance section a list of LDAP links ends with.
const ldapListHints = "\n---\n💡 **Next steps:**\n" +
	"- Use action 'group.ldap_link_add' to add another LDAP group link\n" +
	"- Use action 'group.ldap_sync' to trigger an LDAP sync for this group\n"

// TestFormatOutputMarkdown verifies the whole card an LDAP group link renders:
// the access level named as well as numbered, no row for a field GitLab did
// not send, and a heading that names the filter when the link is defined by
// one and so carries no common name.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name   string
		output Output
		want   string
	}{
		{
			name: "renders basic link with CN and provider",
			output: Output{
				CN:          "engineers",
				GroupAccess: 30,
				Provider:    "main",
			},
			want: "## LDAP Link: engineers\n\n" +
				"- **CN**: engineers\n" +
				"- **Access Level**: Developer (30)\n" +
				"- **Provider**: main\n" +
				ldapLinkHints,
		},
		{
			name: "renders link with Filter",
			output: Output{
				CN:          "devs",
				Filter:      "(dept=engineering)",
				GroupAccess: 40,
				Provider:    "ldap2",
			},
			want: "## LDAP Link: devs\n\n" +
				"- **CN**: devs\n" +
				"- **Filter**: (dept=engineering)\n" +
				"- **Access Level**: Maintainer (40)\n" +
				"- **Provider**: ldap2\n" +
				ldapLinkHints,
		},
		{
			name: "renders link with MemberRoleID",
			output: Output{
				CN:           "admins",
				GroupAccess:  50,
				Provider:     "main",
				MemberRoleID: 99,
			},
			want: "## LDAP Link: admins\n\n" +
				"- **CN**: admins\n" +
				"- **Access Level**: Owner (50)\n" +
				"- **Provider**: main\n" +
				"- **Member Role ID**: 99\n" +
				ldapLinkHints,
		},
		{
			name: "a filter-based link is headed by its filter",
			output: Output{
				Filter:      "(dept=engineering)",
				GroupAccess: 30,
				Provider:    "main",
			},
			want: "## LDAP Link: (dept=engineering)\n\n" +
				"- **Filter**: (dept=engineering)\n" +
				"- **Access Level**: Developer (30)\n" +
				"- **Provider**: main\n" +
				ldapLinkHints,
		},
		{
			name:   "a link with neither is headed by the object it is",
			output: Output{GroupAccess: 10, Provider: "main"},
			want: "## LDAP Link\n\n" +
				"- **Access Level**: Guest (10)\n" +
				"- **Provider**: main\n" +
				ldapLinkHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatOutputMarkdown(tt.output); got != tt.want {
				t.Errorf("FormatOutputMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// TestFormatListMarkdown verifies the whole document a list of LDAP links
// renders: the heading a list opens with, the member role column the table
// used to leave out, and the guidance last rather than first.
func TestFormatListMarkdown(t *testing.T) {
	const header = "| CN | Filter | Access Level | Provider | Member Role ID |\n" +
		"| --- | --- | --- | --- | --- |\n"

	tests := []struct {
		name   string
		output ListOutput
		want   string
	}{
		{
			name:   "renders empty list message",
			output: ListOutput{Links: []Output{}},
			want:   "No LDAP group links found.\n",
		},
		{
			name: "renders single link table",
			output: ListOutput{Links: []Output{
				{CN: "engineers", Filter: "(dept=eng)", GroupAccess: 30, Provider: "main", MemberRoleID: 7},
			}},
			want: "## LDAP Group Links (1)\n\n" + header +
				"| engineers | (dept=eng) | Developer (30) | main | 7 |\n" +
				ldapListHints,
		},
		{
			name: "renders multiple links table",
			output: ListOutput{Links: []Output{
				{CN: "devs", GroupAccess: 30, Provider: "main"},
				{CN: "admins", GroupAccess: 50, Provider: "ldap2"},
			}},
			want: "## LDAP Group Links (2)\n\n" + header +
				"| devs |  | Developer (30) | main | - |\n" +
				"| admins |  | Owner (50) | ldap2 | - |\n" +
				ldapListHints,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatListMarkdown(tt.output); got != tt.want {
				t.Errorf("FormatListMarkdown =\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}
