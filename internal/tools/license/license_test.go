// license_test.go contains unit tests for the GitLab license MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package license

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// fmtUnexpErr identifies the fmt unexp err constant used by this package.
const fmtUnexpErr = "unexpected error: %v"

// licenseJSON identifies the license JSON constant used by this package.
const licenseJSON = `{
	"id": 1,
	"plan": "premium",
	"created_at": "2026-01-01T00:00:00Z",
	"starts_at": "2026-01-01",
	"expires_at": "2026-01-01",
	"historical_max": 100,
	"maximum_user_count": 50,
	"expired": false,
	"overage": 0,
	"user_limit": 100,
	"active_users": 42,
	"licensee": {"Name":"John","Company":"Acme","Email":"john@acme.com"},
	"add_ons": {"GitLab_Auditor_User":1,"GitLab_DeployBoard":0,"GitLab_FileLocks":1,"GitLab_Geo":0,"GitLab_ServiceDesk":1}
}`

// TestGet_Success verifies Get when success.
func TestGet_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/license" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK, licenseJSON)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License.ID != 1 {
		t.Errorf("expected ID 1, got %d", out.License.ID)
	}
	if out.License.Plan != "premium" {
		t.Errorf("expected premium, got %s", out.License.Plan)
	}
	if out.License.ActiveUsers != 42 {
		t.Errorf("expected 42 active users, got %d", out.License.ActiveUsers)
	}
	if out.License.Licensee.Name != "John" {
		t.Errorf("expected John, got %s", out.License.Licensee.Name)
	}
}

// TestGet_Error verifies Get when error.
func TestGet_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestAdd_Success verifies Add when success.
func TestAdd_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
	}))

	out, err := Add(t.Context(), client, AddInput{License: "base64encodedlicense"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License.Plan != "premium" {
		t.Errorf("expected premium, got %s", out.License.Plan)
	}
}

// TestDelete_Success verifies Delete when success.
func TestDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/v4/license/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 1})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
}

// TestDelete_InvalidID verifies Delete when invalid ID.
func TestDelete_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name string
		id   int64
	}{
		{"zero", 0},
		{"negative", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Delete(t.Context(), client, DeleteInput{ID: tc.id})
			if err == nil {
				t.Errorf("expected error for ID %d", tc.id)
			}
		})
	}
}

// TestDelete_Error verifies Delete when error.
func TestDelete_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
	}))

	err := Delete(t.Context(), client, DeleteInput{ID: 999})
	if err == nil {
		t.Fatal("expected error")
	}
}

// licenseHints is the guidance section every license card ends with.
const licenseHints = "\n---\n💡 **Next steps:**\n" +
	"- Check license expiry date and plan for renewal if needed\n"

// markdownText returns the one text block a formatter's result carries.
func markdownText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("the formatter returned no content at all")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *mcp.TextContent", result.Content[0])
	}
	return text.Text
}

// TestFormatLicenseMarkdown verifies the whole card of a valid license: the
// licensee is a nested object rather than a "%s (%s) - %s" line, and a license
// that has not expired carries no warning row.
func TestFormatLicenseMarkdown(t *testing.T) {
	got := markdownText(t, FormatLicenseMarkdown(Item{
		ID:          1,
		Plan:        "premium",
		ActiveUsers: 42,
		UserLimit:   100,
		Expired:     false,
		Licensee:    LicenseeItem{Name: "John", Company: "Acme", Email: "john@acme.com"},
	}))
	want := "## License #1\n\n" +
		"- **ID**: 1\n" +
		"- **Plan**: premium\n" +
		"- **Active Users**: 42\n" +
		"- **User Limit**: 100\n" +
		"- **Maximum User Count**: 0\n" +
		"- **Historical Max**: 0\n" +
		"- **Overage**: 0\n" +
		"- **Licensee**:\n" +
		"  - **Name**: John\n" +
		"  - **Company**: Acme\n" +
		"  - **Email**: john@acme.com\n" +
		licenseHints
	if got != want {
		t.Errorf("FormatLicenseMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatLicenseMarkdown_AddOns verifies that the add-on seats reach the
// card, which used to publish them in its JSON and drop them from the Markdown
// entirely, and that an add-on with no seats is left out rather than shown as
// licensed for zero.
func TestFormatLicenseMarkdown_AddOns(t *testing.T) {
	got := markdownText(t, FormatLicenseMarkdown(Item{
		ID:   1,
		Plan: "ultimate",
		AddOns: AddOnsItem{
			GitLabAuditorUser: 1,
			GitLabFileLocks:   2,
			GitLabServiceDesk: 3,
		},
	}))
	want := "## License #1\n\n" +
		"- **ID**: 1\n" +
		"- **Plan**: ultimate\n" +
		"- **Active Users**: 0\n" +
		"- **User Limit**: 0\n" +
		"- **Maximum User Count**: 0\n" +
		"- **Historical Max**: 0\n" +
		"- **Overage**: 0\n" +
		"- **Add-ons**:\n" +
		"  - **Auditor User**: 1\n" +
		"  - **File Locks**: 2\n" +
		"  - **Service Desk**: 3\n" +
		licenseHints
	if got != want {
		t.Errorf("FormatLicenseMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// Add — API error
// ---------------------------------------------------------------------------.

// TestAdd_Error verifies Add when error.
func TestAdd_Error(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid license"}`)
	}))
	_, err := Add(t.Context(), client, AddInput{License: "bad"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// FormatLicenseMarkdown — with dates
// ---------------------------------------------------------------------------.

// TestFormatLicenseMarkdown_WithDates verifies FormatLicenseMarkdown when with dates.
func TestFormatLicenseMarkdown_WithDates(t *testing.T) {
	item := Item{
		ID:               2,
		Plan:             "ultimate",
		StartsAt:         "2026-01-01",
		ExpiresAt:        "2026-12-31",
		CreatedAt:        "2026-01-01T00:00:00Z",
		ActiveUsers:      100,
		UserLimit:        200,
		MaximumUserCount: 150,
		HistoricalMax:    120,
		Overage:          5,
		Expired:          true,
		Licensee:         LicenseeItem{Name: "Jane", Company: "Corp", Email: "jane@corp.com"},
	}
	got := markdownText(t, FormatLicenseMarkdown(item))
	want := "## License #2\n\n" +
		"- **ID**: 2\n" +
		"- **Plan**: ultimate\n" +
		"- ⚠️ **Expired**\n" +
		"- **Active Users**: 100\n" +
		"- **User Limit**: 200\n" +
		"- **Maximum User Count**: 150\n" +
		"- **Historical Max**: 120\n" +
		"- **Overage**: 5\n" +
		"- **Starts At**: 1 Jan 2026\n" +
		"- **Expires At**: 31 Dec 2026\n" +
		"- **Created At**: 1 Jan 2026 00:00 UTC\n" +
		"- **Licensee**:\n" +
		"  - **Name**: Jane\n" +
		"  - **Company**: Corp\n" +
		"  - **Email**: jane@corp.com\n" +
		licenseHints
	if got != want {
		t.Errorf("FormatLicenseMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatGetMarkdown / FormatAddMarkdown — wrappers
// ---------------------------------------------------------------------------.

// licenseCard renders the card a license with an id, a plan and a licensee
// produces, so the two wrapper tests state the whole answer.
func licenseCard(id int64, plan, name, company, email string) string {
	return fmt.Sprintf("## License #%d\n\n- **ID**: %d\n", id, id) +
		"- **Plan**: " + plan + "\n" +
		"- **Active Users**: 0\n" +
		"- **User Limit**: 0\n" +
		"- **Maximum User Count**: 0\n" +
		"- **Historical Max**: 0\n" +
		"- **Overage**: 0\n" +
		"- **Licensee**:\n" +
		"  - **Name**: " + name + "\n" +
		"  - **Company**: " + company + "\n" +
		"  - **Email**: " + email + "\n" +
		licenseHints
}

// TestFormatGetMarkdown_Coverage verifies the read wrapper renders the card of
// the license it was handed.
func TestFormatGetMarkdown_Coverage(t *testing.T) {
	out := GetOutput{License: Item{ID: 1, Plan: "premium", Licensee: LicenseeItem{Name: "A", Company: "B", Email: "c@d.com"}}}
	got := markdownText(t, FormatGetMarkdown(out))
	want := licenseCard(1, "premium", "A", "B", "c@d.com")
	if got != want {
		t.Errorf("FormatGetMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestFormatAddMarkdown_Coverage verifies the install wrapper renders the card
// of the license GitLab answered with.
func TestFormatAddMarkdown_Coverage(t *testing.T) {
	out := AddOutput{License: Item{ID: 3, Plan: "gold", Licensee: LicenseeItem{Name: "X", Company: "Y", Email: "x@y.com"}}}
	got := markdownText(t, FormatAddMarkdown(out))
	want := licenseCard(3, "gold", "X", "Y", "x@y.com")
	if got != want {
		t.Errorf("FormatAddMarkdown() =\n%q\nwant:\n%q", got, want)
	}
}

// TestGet_NoLicenseInstalled verifies that an answer carrying no license is an
// error naming what happened rather than a card reading "License #0" with a
// blank plan, which is what a zero Item rendered as. GitLab answers this way
// on an instance with none installed, and reading the fields off the nil
// pointer would crash the handler.
func TestGet_NoLicenseInstalled(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `null`)
	}))
	_, err := Get(t.Context(), client, GetInput{})
	if err == nil {
		t.Fatal("Get() with no license installed returned no error")
	}
	if !strings.Contains(err.Error(), "this instance has none installed") {
		t.Errorf("Get() error = %v, want it to say no license is installed", err)
	}
}

// TestAdd_NoLicenseReturned verifies the same guard on the install path: an
// accepted request that answers with no license is reported rather than
// dereferenced.
func TestAdd_NoLicenseReturned(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusCreated, `null`)
	}))
	_, err := Add(t.Context(), client, AddInput{License: "base64encodedlicense"})
	if err == nil {
		t.Fatal("Add() with no license in the answer returned no error")
	}
	if !strings.Contains(err.Error(), "this instance has none installed") {
		t.Errorf("Add() error = %v, want it to say no license is installed", err)
	}
}

// TestToItem_NilDates verifies that toItem produces empty date strings when the
// GitLab license has nil CreatedAt, StartsAt, and ExpiresAt fields.
func TestToItem_NilDates(t *testing.T) {
	lic := &gl.License{
		ID:   99,
		Plan: "free",
		Licensee: gl.LicenseLicensee{
			Name:    "Test",
			Company: "Co",
			Email:   "t@co.com",
		},
	}
	item := toItem(lic)
	if item.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", item.CreatedAt)
	}
	if item.StartsAt != "" {
		t.Errorf("expected empty StartsAt, got %q", item.StartsAt)
	}
	if item.ExpiresAt != "" {
		t.Errorf("expected empty ExpiresAt, got %q", item.ExpiresAt)
	}
}

// TestToItem_WithDates verifies that toItem correctly formats non-nil date
// pointers from the GitLab license struct.
func TestToItem_WithDates(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	isoStart := gl.ISOTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	isoEnd := gl.ISOTime(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))

	lic := &gl.License{
		ID:        1,
		Plan:      "premium",
		CreatedAt: &now,
		StartsAt:  &isoStart,
		ExpiresAt: &isoEnd,
		Licensee: gl.LicenseLicensee{
			Name:    "Name",
			Company: "Corp",
			Email:   "e@corp.com",
		},
	}
	item := toItem(lic)
	if item.CreatedAt == "" {
		t.Error("expected non-empty CreatedAt")
	}
	if !strings.Contains(item.CreatedAt, "2026") {
		t.Errorf("expected year 2026 in CreatedAt, got %q", item.CreatedAt)
	}
	if item.StartsAt == "" {
		t.Error("expected non-empty StartsAt")
	}
	if item.ExpiresAt == "" {
		t.Error("expected non-empty ExpiresAt")
	}
}
