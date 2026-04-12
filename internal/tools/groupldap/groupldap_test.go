// Package groupldap tests validate all MCP tool handlers for GitLab group
// LDAP link operations: List, Add, DeleteWithCNOrFilter, DeleteForProvider.
// Tests cover success paths, input validation, API errors, optional field
// branches, and markdown formatting for both single and list outputs.
package groupldap

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/testutil"
)

const (
	pathGroupLDAP = "/api/v4/groups/mygroup/ldap_group_links"
)

// TestList validates the List handler covering success, empty results,
// validation errors, and API failure responses.
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
			name:    "returns error when group_id is empty",
			input:   ListInput{},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.NotFound(w, nil) }),
			wantErr: true,
		},
		{
			name:  "returns error on API failure",
			input: ListInput{GroupID: "mygroup"},
			handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
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

// TestList_FieldMapping verifies toOutput maps all fields from the API response.
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

// TestAdd validates the Add handler covering success with CN, Filter,
// MemberRoleID, validation errors, and API failure responses.
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
			name:  "deletes with all options set",
			input: DeleteWithCNOrFilterInput{GroupID: "mygroup", CN: "cn1", Filter: "(uid=*)", Provider: "ldap2"},
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

// TestDeleteForProvider validates the DeleteForProvider handler covering
// success, all three validation errors, and API failure responses.
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

// TestFormatOutputMarkdown validates markdown rendering for a single LDAP link,
// including optional Filter and MemberRoleID fields.
func TestFormatOutputMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		output   Output
		contains []string
		excludes []string
	}{
		{
			name: "renders basic link with CN and provider",
			output: Output{
				CN:          "engineers",
				GroupAccess: 30,
				Provider:    "main",
			},
			contains: []string{
				"## LDAP Link: engineers",
				"**CN**: engineers",
				"**Access Level**: 30",
				"**Provider**: main",
			},
			excludes: []string{"**Filter**", "**Member Role ID**"},
		},
		{
			name: "renders link with Filter",
			output: Output{
				CN:          "devs",
				Filter:      "(dept=engineering)",
				GroupAccess: 40,
				Provider:    "ldap2",
			},
			contains: []string{
				"**Filter**: (dept=engineering)",
				"**Provider**: ldap2",
			},
		},
		{
			name: "renders link with MemberRoleID",
			output: Output{
				CN:           "admins",
				GroupAccess:  50,
				Provider:     "main",
				MemberRoleID: 99,
			},
			contains: []string{
				"**Member Role ID**: 99",
			},
		},
		{
			name: "includes hint for deletion",
			output: Output{
				CN:          "test",
				GroupAccess: 10,
				Provider:    "main",
			},
			contains: []string{"gitlab_group_ldap_link_delete"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatOutputMarkdown(tt.output)
			for _, want := range tt.contains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
			for _, exclude := range tt.excludes {
				if strings.Contains(md, exclude) {
					t.Errorf("markdown should not contain %q\ngot:\n%s", exclude, md)
				}
			}
		})
	}
}

// TestFormatListMarkdown validates markdown rendering for an LDAP link list,
// including empty and multi-item cases.
func TestFormatListMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		output   ListOutput
		contains []string
	}{
		{
			name:     "renders empty list message",
			output:   ListOutput{Links: []Output{}},
			contains: []string{"No LDAP group links found."},
		},
		{
			name: "renders single link table",
			output: ListOutput{Links: []Output{
				{CN: "engineers", Filter: "(dept=eng)", GroupAccess: 30, Provider: "main"},
			}},
			contains: []string{
				"**1 LDAP link(s)**",
				"| CN | Filter | Access | Provider |",
				"| engineers |",
			},
		},
		{
			name: "renders multiple links table",
			output: ListOutput{Links: []Output{
				{CN: "devs", GroupAccess: 30, Provider: "main"},
				{CN: "admins", GroupAccess: 50, Provider: "ldap2"},
			}},
			contains: []string{
				"**2 LDAP link(s)**",
				"| devs |",
				"| admins |",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := FormatListMarkdown(tt.output)
			for _, want := range tt.contains {
				if !strings.Contains(md, want) {
					t.Errorf("markdown missing %q\ngot:\n%s", want, md)
				}
			}
		})
	}
}
