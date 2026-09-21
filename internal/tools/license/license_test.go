// license_test.go contains unit tests for the GitLab license MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package license

import (
	"encoding/json"
	"fmt"
	"io"
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

// licensePath is the one endpoint the instance license lives on. The handlers
// answer it and nothing else, so a request built for another path is a failure
// rather than a pass on a mock that accepts everything.
const licensePath = "/api/v4/license"

// suggestionMarker is what toolutil.WrapErrWithHint writes in front of the
// corrective advice, and so the one thing in a wrapped error that separates
// "here is what to do about it" from "here is what happened".
const suggestionMarker = "Suggestion: "

// suggestionIn returns the corrective advice a wrapped error carries, and
// whether it carries any at all.
//
// The hint sits between the marker and the cause the wrapper appends, so a
// hint that went missing is reported as absent and one that was emptied comes
// back as "". Both matter here, because every one of the three handlers
// reaches for WrapErrWithStatusHint precisely to attach that advice on one
// status and to withhold it on every other.
func suggestionIn(msg string) (advice string, found bool) {
	_, rest, found := strings.Cut(msg, suggestionMarker)
	if !found {
		return "", false
	}
	advice, _, _ = strings.Cut(rest, ": ")
	return advice, true
}

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

// distinctLicenseJSON is the license GitLab answers with in which no two
// values agree: eleven numbers that share no figure, three licensee strings
// that share no word, two dates that share no day and five add-on seat counts
// that are five different numbers.
//
// licenseJSON above cannot state what this one states. It gives user_limit and
// historical_max the same 100, overage and two add-ons the same 0, and two more
// add-ons the same 1, so a converter that read a field off its neighbor
// produced the same Item either way. The keys are spelled the way client-go
// decodes them (`Name`, `GitLab_Auditor_User`), not the way GitLab's
// documentation prints them, or every assertion below would pass on a zero.
const distinctLicenseJSON = `{
	"id": 7,
	"plan": "ultimate",
	"created_at": "2026-03-04T05:06:07Z",
	"starts_at": "2026-04-05",
	"expires_at": "2027-08-09",
	"historical_max": 311,
	"maximum_user_count": 313,
	"expired": true,
	"overage": 317,
	"user_limit": 319,
	"active_users": 323,
	"licensee": {"Name":"Ada","Company":"Analytical Engines","Email":"ada@engines.test"},
	"add_ons": {"GitLab_Auditor_User":2,"GitLab_DeployBoard":3,"GitLab_FileLocks":5,"GitLab_Geo":11,"GitLab_ServiceDesk":13}
}`

// distinctItem is what toItem has to make of distinctLicenseJSON, field for
// field.
var distinctItem = Item{
	ID:               7,
	Plan:             "ultimate",
	CreatedAt:        "2026-03-04T05:06:07Z",
	StartsAt:         "2026-04-05",
	ExpiresAt:        "2027-08-09",
	HistoricalMax:    311,
	MaximumUserCount: 313,
	Expired:          true,
	Overage:          317,
	UserLimit:        319,
	ActiveUsers:      323,
	Licensee:         LicenseeItem{Name: "Ada", Company: "Analytical Engines", Email: "ada@engines.test"},
	AddOns: AddOnsItem{
		GitLabAuditorUser: 2,
		GitLabDeployBoard: 3,
		GitLabFileLocks:   5,
		GitLabGeo:         11,
		GitLabServiceDesk: 13,
	},
}

// TestGet_EveryFieldOfTheLicenseComesFromItsOwnSource asserts the whole Item a
// read produces, against a license in which no two values agree.
//
// toItem is nineteen straight-line assignments, which neither gate can be
// wrong about: gremlins has no operator to flip and gobco has no condition to
// evaluate, so both reported this package clean while the converter could read
// any field off its neighbor. Verified by hand, by crossing StartsAt with
// ExpiresAt, HistoricalMax with MaximumUserCount, the licensee's company with
// its email and the Deploy Board seats with the Geo seats: the suite passed
// every one of those before this test existed.
func TestGet_EveryFieldOfTheLicenseComesFromItsOwnSource(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != licensePath || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, distinctLicenseJSON)
	}))

	out, err := Get(t.Context(), client, GetInput{})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License != distinctItem {
		t.Errorf("Get() license =\n%+v\nwant:\n%+v", out.License, distinctItem)
	}
}

// TestAdd_EveryFieldOfTheLicenseComesFromItsOwnSource asserts the same of the
// install path, which converts GitLab's answer through the same toItem and had
// its own assertion on the plan alone.
func TestAdd_EveryFieldOfTheLicenseComesFromItsOwnSource(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != licensePath || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		testutil.RespondJSON(w, http.StatusCreated, distinctLicenseJSON)
	}))

	out, err := Add(t.Context(), client, AddInput{License: "base64encodedlicense"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.License != distinctItem {
		t.Errorf("Add() license =\n%+v\nwant:\n%+v", out.License, distinctItem)
	}
}

// TestAdd_SendsTheLicenseTheCallerPassed asserts that the license string the
// caller supplied is what reaches GitLab, on the endpoint the license lives on.
//
// Nothing asserted this: TestAdd_Success reads back the mock's own fixture, so
// a handler that posted an empty license, or posted the right one to the wrong
// path, returned exactly the same output. Verified by hand, by replacing the
// caller's string with an empty one, which the suite passed.
func TestAdd_SendsTheLicenseTheCallerPassed(t *testing.T) {
	const want = "c29tZS1saWNlbnNlLWtleQ=="

	var sent string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != licensePath || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil {
			t.Errorf("reading the request body: %v", readErr)
			testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
			return
		}
		var payload struct {
			License string `json:"license"`
		}
		if decodeErr := json.Unmarshal(body, &payload); decodeErr != nil {
			t.Errorf("the install body is not the JSON object GitLab expects: %v (%s)", decodeErr, body)
			testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
			return
		}
		sent = payload.License
		testutil.RespondJSON(w, http.StatusCreated, licenseJSON)
	}))

	if _, err := Add(t.Context(), client, AddInput{License: want}); err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if sent != want {
		t.Errorf("GitLab was sent license %q, want %q", sent, want)
	}
}

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

// TestGet_TheAdministratorAdviceGoesWithTheForbidden asserts which refusal
// carries the advice about administrator access and which does not.
//
// The status a handler pairs its hint with is a constant in a straight-line
// call, so neither gate can be wrong about it, and the tests here asserted only
// that some error came back, which every branch of WrapErrWithStatusHint
// returns. Verified by hand, by changing the status this handler names to
// http.StatusNotFound, which the suite passed.
func TestGet_TheAdministratorAdviceGoesWithTheForbidden(t *testing.T) {
	t.Run("forbidden", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		}))
		_, err := Get(t.Context(), client, GetInput{})
		if err == nil {
			t.Fatal("a refused read returned no error")
		}
		advice, found := suggestionIn(err.Error())
		if !found {
			t.Fatalf("a refused read carries no suggestion: %v", err)
		}
		if !strings.Contains(advice, "administrator access") {
			t.Errorf("the suggestion is %q, want it to say administrator access is needed", advice)
		}
	})

	t.Run("a failure that is not a refusal", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusInternalServerError, `{"message":"500 Internal Server Error"}`)
		}))
		_, err := Get(t.Context(), client, GetInput{})
		if err == nil {
			t.Fatal("a failed read returned no error")
		}
		if advice, found := suggestionIn(err.Error()); found {
			t.Errorf("a 500 carries the permission advice %q, which does not apply to it", advice)
		}
	})
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

// TestDelete_InvalidID asserts that an id that cannot name a license is
// refused here, before any request leaves, and that the refusal names the
// parameter the caller has to correct.
//
// The mock is testutil.ForbiddenHandler, which fails the test if a request
// arrives at all: with a mock that answered 204 to everything, a handler that
// dropped the guard and sent DELETE /license/0 still returned an error only
// because GitLab happened to refuse it, and the test could not tell the two
// apart.
func TestDelete_InvalidID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))

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
				t.Fatalf("expected error for ID %d", tc.id)
			}
			if !strings.Contains(err.Error(), "id is required") {
				t.Errorf("Delete() error = %v, want it to name the id parameter", err)
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

// TestDelete_TheIDAdviceGoesWithTheNotFound asserts that the advice about
// checking the license id goes with the status that means no license carries
// it, and with no other.
//
// The 404 branch was reached by nothing at all: the only failing delete test
// answers 403, so the hint this handler exists to attach was never produced,
// and both gates were clean because the branch that chooses it lives in
// toolutil rather than here.
func TestDelete_TheIDAdviceGoesWithTheNotFound(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 License Not Found"}`)
		}))
		err := Delete(t.Context(), client, DeleteInput{ID: 999})
		if err == nil {
			t.Fatal("deleting a license that is not there returned no error")
		}
		advice, found := suggestionIn(err.Error())
		if !found {
			t.Fatalf("a delete of a license that is not there carries no suggestion: %v", err)
		}
		if !strings.Contains(advice, "license_id") {
			t.Errorf("the suggestion is %q, want it to point at the license id", advice)
		}
	})

	t.Run("a failure that is not a not-found", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		}))
		err := Delete(t.Context(), client, DeleteInput{ID: 999})
		if err == nil {
			t.Fatal("a refused delete returned no error")
		}
		if advice, found := suggestionIn(err.Error()); found {
			t.Errorf("a 403 carries the id advice %q, and the id was never the problem", advice)
		}
	})
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

// TestFormatLicenseMarkdown_EachAddOnUnderItsOwnLabel asserts the five add-on
// rows against a license whose five seat counts are five different numbers.
//
// TestFormatLicenseMarkdown_AddOns above cannot state this: it gives Deploy
// Board and Geo no seats at all, so the two rows are both left out and the card
// reads the same whichever field each label was written from. Verified by hand,
// by crossing those two labels in writeAddOns, which the suite passed.
func TestFormatLicenseMarkdown_EachAddOnUnderItsOwnLabel(t *testing.T) {
	got := markdownText(t, FormatLicenseMarkdown(Item{
		ID:     4,
		Plan:   "ultimate",
		AddOns: AddOnsItem{GitLabAuditorUser: 2, GitLabDeployBoard: 3, GitLabFileLocks: 5, GitLabGeo: 11, GitLabServiceDesk: 13},
	}))
	want := "## License #4\n\n" +
		"- **ID**: 4\n" +
		"- **Plan**: ultimate\n" +
		"- **Active Users**: 0\n" +
		"- **User Limit**: 0\n" +
		"- **Maximum User Count**: 0\n" +
		"- **Historical Max**: 0\n" +
		"- **Overage**: 0\n" +
		"- **Add-ons**:\n" +
		"  - **Auditor User**: 2\n" +
		"  - **Deploy Board**: 3\n" +
		"  - **File Locks**: 5\n" +
		"  - **Geo**: 11\n" +
		"  - **Service Desk**: 13\n" +
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

// TestAdd_TheKeyAdviceGoesWithTheBadRequest asserts that the advice about the
// license key goes with the status that means the key was rejected, and with
// no other. A refusal for want of administrator rights is not a malformed key,
// and telling the caller to check their key would send them to fix the one
// thing that was not wrong.
func TestAdd_TheKeyAdviceGoesWithTheBadRequest(t *testing.T) {
	t.Run("bad request", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusBadRequest, `{"message":"invalid license"}`)
		}))
		_, err := Add(t.Context(), client, AddInput{License: "bad"})
		if err == nil {
			t.Fatal("a rejected install returned no error")
		}
		advice, found := suggestionIn(err.Error())
		if !found {
			t.Fatalf("a rejected install carries no suggestion: %v", err)
		}
		if !strings.Contains(advice, "license key") {
			t.Errorf("the suggestion is %q, want it to point at the license key", advice)
		}
	})

	t.Run("a failure that is not a bad request", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"403 Forbidden"}`)
		}))
		_, err := Add(t.Context(), client, AddInput{License: "good"})
		if err == nil {
			t.Fatal("a refused install returned no error")
		}
		if advice, found := suggestionIn(err.Error()); found {
			t.Errorf("a 403 carries the key advice %q, and the key was never the problem", advice)
		}
	})
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
//
// It also asserts that the way out the hint offers is one a model can take.
// The hint used to name "the license_add action" and "the metadata_get
// action", and neither is a name any surface resolves: the canonical IDs carry
// the admin domain, the individual tools are called gitlab_add_license and
// gitlab_metadata, and a model following the hint verbatim is answered
// "unknown action". Nothing sees this on its own: a hint is a string, so
// neither gate has a branch to flip, and cmd/audit_action_ids reads the
// catalog metadata rather than a handler's prose.
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
	advice, found := suggestionIn(err.Error())
	if !found {
		t.Fatalf("Get() with no license installed carries no suggestion: %v", err)
	}
	if !strings.Contains(advice, "admin.license_add") {
		t.Errorf("the suggestion %q does not name the canonical action ID %q", advice, "admin.license_add")
	}
	if !strings.Contains(advice, "admin.metadata_get") {
		t.Errorf("the suggestion %q does not name the canonical action ID %q", advice, "admin.metadata_get")
	}
}

// TestAdd_NoLicenseReturned verifies the same guard on the install path: an
// accepted request that answers with no license is reported rather than
// dereferenced, and its hint names the canonical ID of the read it points at.
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
	advice, found := suggestionIn(err.Error())
	if !found {
		t.Fatalf("Add() with no license in the answer carries no suggestion: %v", err)
	}
	if !strings.Contains(advice, "admin.license_get") {
		t.Errorf("the suggestion %q does not name the canonical action ID %q", advice, "admin.license_get")
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

// TestToItem_WithDates verifies the form toItem renders each non-nil date in:
// the creation instant in the wire form, RFC 3339 in UTC, and the two license
// dates as plain days, which is what GitLab sends them as.
//
// The comment claimed this before the body did. It asserted that CreatedAt
// held "2026" and that the other two were not empty, which says nothing about
// the layout and nothing about which date landed in which field, so a
// converter that swapped the start with the expiry passed it.
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
	if item.CreatedAt != "2026-06-15T12:00:00Z" {
		t.Errorf("CreatedAt = %q, want the RFC 3339 instant", item.CreatedAt)
	}
	if item.StartsAt != "2026-01-01" {
		t.Errorf("StartsAt = %q, want the day the license starts", item.StartsAt)
	}
	if item.ExpiresAt != "2026-12-31" {
		t.Errorf("ExpiresAt = %q, want the day the license expires", item.ExpiresAt)
	}
}
