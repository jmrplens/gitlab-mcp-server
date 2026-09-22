// keys_test.go contains unit tests for the SSH key MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package keys

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// TestGetKeyWithUser_Success pins the whole Output a by-ID lookup builds, from
// an answer in which no two values agree, so a field taken from its neighbor
// is visible. Two pairs here have nothing to tell them apart otherwise: the
// title and the public key are both strings off the key, and the owning user's
// ID sits beside the key's own. Checking the ID and the username alone left
// both pairs free to trade places, and crossing either kept the suite green.
func TestGetKeyWithUser_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/keys/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":42,"title":"My Key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z","user":{"id":7,"username":"admin","name":"Root Admin"}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Output{
		ID:        42,
		Title:     "My Key",
		Key:       "ssh-rsa AAAA...",
		CreatedAt: "2026-01-01T00:00:00Z",
		User:      UserOutput{ID: 7, Username: "admin", Name: "Root Admin"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("GetKeyWithUser() = %+v, want %+v", out, want)
	}
}

// TestGetKeyWithUser_ReadsWhatTheSDKDoesNotModel verifies a key carries,
// beside what client-go decoded, the expiry, last use and usage type
// lib/api/entities/ssh_key.rb sends on every key and gl.Key does not.
func TestGetKeyWithUser_ReadsWhatTheSDKDoesNotModel(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":42,"title":"My Key","key":"ssh-rsa AAAA...","created_at":"2026-01-01T00:00:00Z",`+
			`"expires_at":"2026-05-05T00:00:00.000Z","last_used_at":"2026-04-07T00:00:00.000Z","usage_type":"auth_and_signing",`+
			`"user":{"id":1,"username":"admin","name":"Admin"}}`)
	}))

	out, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 42})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ExpiresAt != "2026-05-05T00:00:00Z" || out.LastUsedAt != "2026-04-07T00:00:00Z" || out.UsageType != "auth_and_signing" {
		t.Errorf("GetKeyWithUser() = %+v, want the captured fields", out)
	}
}

// TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported verifies the one
// failure the captured response adds to both handlers: GitLab's answer
// decodes for the SDK and not for the fields read beside it, and the
// handler reports it rather than swallowing it, attributed to the call that
// really ran.
//
// The shared assertion is a single substring test over both handlers, so it
// holds that each reported a decode failure and nothing about which handler
// the failure is blamed on. That left the two operation labels on this branch
// free to trade places: both errors would still name the captured response,
// both cases would still pass, and a caller who asked for a key by ID would be
// told the fingerprint lookup was the call that could not read GitLab's
// answer. Each error is therefore kept and held to its own label afterwards.
func TestHandlers_ACapturedFieldTheTypeCannotHold_IsReported(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"id":42,"title":"My Key","key":"ssh-rsa AAAA...","usage_type":7,"user":{"id":1}}`)
	}))
	reported := make(map[string]error, 2)
	testutil.AssertCapturedDecodeFailures(t, []testutil.CapturedCase{
		{Name: "by id", Call: func() error {
			_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 42})
			reported["key_get"] = err
			return err
		}},
		{Name: "by fingerprint", Call: func() error {
			_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
			reported["key_get_by_fingerprint"] = err
			return err
		}},
	})
	for _, operation := range []string{"key_get", "key_get_by_fingerprint"} {
		t.Run(operation, func(t *testing.T) {
			err := reported[operation]
			if err == nil || !strings.HasPrefix(err.Error(), operation+": ") {
				t.Errorf("captured decode failure %v is not attributed to %s", err, operation)
			}
		})
	}
}

// TestGetKeyWithUser_MissingID asserts the handler refuses a key_id it was not
// given, names the field itself, attributes the refusal to its own operation,
// and never reaches GitLab.
//
// All four halves matter. The mock was a bare 200 with an empty body, so
// deleting the guard entirely left this green: GET /keys/0 came back
// undecodable and an error arrived either way. It proved an error existed, not
// that this handler produced it, and a model would have been answered about a
// malformed response instead of about the field it omitted.
//
// The operation label is asserted at its own position because it is a bare
// string the two handlers can exchange and still compile: a substring test
// cannot tell the label apart from the field name, and a caller who asked for
// a key by ID would be told "key_get_by_fingerprint" about a call it never
// made.
func TestGetKeyWithUser_MissingID(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{})
	if err == nil {
		t.Fatal("expected error for missing key_id")
	}
	if !strings.HasPrefix(err.Error(), "key_get: ") {
		t.Errorf("error %q is not attributed to key_get", err)
	}
	if !strings.HasSuffix(err.Error(), "key_id is required") {
		t.Errorf("error %q does not name the missing field", err)
	}
}

// TestGetKeyByFingerprint_Success pins the whole Output of the fingerprint
// lookup, and the fingerprint it sent, against an answer whose values are all
// distinct. Both handlers share one converter, so the same crossings are
// reachable here; asserting only the ID left every other field of this route
// unread, and GitLab answers a fingerprint lookup with no creation time, which
// is what makes this the case that pins the empty-timestamp side.
func TestGetKeyByFingerprint_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fingerprint") != "SHA256:abc123" {
			t.Errorf("unexpected fingerprint param: %s", r.URL.Query().Get("fingerprint"))
		}
		testutil.RespondJSON(w, http.StatusOK,
			`{"id":10,"title":"Deploy Key","key":"ssh-rsa BBBB...","user":{"id":2,"username":"deploy-bot","name":"Deploy Service"}}`)
	})
	client := testutil.NewTestClient(t, handler)

	out, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Output{
		ID:    10,
		Title: "Deploy Key",
		Key:   "ssh-rsa BBBB...",
		User:  UserOutput{ID: 2, Username: "deploy-bot", Name: "Deploy Service"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("GetKeyByFingerprint() = %+v, want %+v", out, want)
	}
}

// TestGetKeyByFingerprint_MissingFingerprint asserts the same of the other
// handler: it names the field, attributes the refusal to itself, and sends
// nothing. Its guard was equally free, and an empty fingerprint is worse than
// a missing key ID, since GET /keys with no fingerprint is a request GitLab
// may well answer.
//
// The field claim is pinned at the end of the message rather than anywhere in
// it because "key_get_by_fingerprint" contains "fingerprint" itself: a
// substring test was satisfied by the operation label alone, so a guard that
// had dropped the field name entirely, or named the other handler's field,
// still read as naming this one.
func TestGetKeyByFingerprint_MissingFingerprint(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{})
	if err == nil {
		t.Fatal("expected error for missing fingerprint")
	}
	if !strings.HasPrefix(err.Error(), "key_get_by_fingerprint: ") {
		t.Errorf("error %q is not attributed to key_get_by_fingerprint", err)
	}
	if !strings.HasSuffix(err.Error(), "fingerprint is required") {
		t.Errorf("error %q does not name the missing field", err)
	}
}

// TestGetKeyWithUser_APIError asserts that a refusal GitLab produced is
// attributed to this action, by the operation label the error carries.
//
// The label is a bare string in each error branch, so the two handlers can
// exchange theirs and keep compiling; asserting only that an error came back
// let them. A model told "key_get_by_fingerprint failed" after asking for a
// key by ID is being told about a call it never made.
func TestGetKeyWithUser_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	client := testutil.NewTestClient(t, handler)

	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 99})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.HasPrefix(err.Error(), "key_get:") {
		t.Errorf("error %q is not attributed to key_get", err)
	}
}

// TestGetKeyWithUser_NotFound_HintsAtTheKeyID asserts the corrective hint a
// 404 carries is the one that belongs to a lookup by ID.
//
// The hint is the only part of the answer a model can act on, and it is the
// other half of what the two handlers could exchange: crossing the two error
// branches handed a caller who searched by fingerprint advice about verifying
// a key_id it never supplied, and every test still passed. Only the 404 branch
// carries a hint at all, which is why it needs a case of its own beside the
// 403 above.
func TestGetKeyWithUser_NotFound_HintsAtTheKeyID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetKeyWithUser(t.Context(), client, GetByIDInput{KeyID: 99})
	if err == nil {
		t.Fatal("expected error for a key GitLab does not hold")
	}
	if !strings.Contains(err.Error(), "verify key_id") {
		t.Errorf("error %q does not hint at the key ID", err)
	}
}

// keyCardHints is the guidance section every SSH-key card ends with, naming
// the sibling lookup by its canonical catalog ID rather than by a tool name
// the serving surface may not register.
//
// The ID is taken from the constant the formatter itself reads rather than
// spelled out here. Spelled out, this line pinned "keys.key_get_by_fingerprint"
// across seven golden cards, and since the formatter said the same thing the
// tests were green on an ID no surface has ever registered. Whether that
// constant is itself a real action is a question this package cannot answer;
// keys_catalog_test.go puts it to the catalog.
const keyCardHints = "\n---\n\U0001F4A1 **Next steps:**\n" +
	"- Use action '" + actionKeyGetByFingerprint + "' to look a key up by its fingerprint instead of its ID\n"

// TestFormatMarkdownString pins the whole card of a key with only the fields
// GitLab always sends, the owning user as a nested object.
func TestFormatMarkdownString(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:    1,
		Title: "Test Key",
		Key:   "ssh-rsa AAAA...",
		User:  UserOutput{ID: 1, Username: "user", Name: "User"},
	})

	want := "## SSH Key #1\n\n" +
		"- **ID**: 1\n" +
		"- **Title**: Test Key\n" +
		"- **Key**: `ssh-rsa AAAA...`\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: User\n" +
		"  - **Username**: @user\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_SentFields pins the three fields GitLab sends on
// every SSH key that the card never showed: when it expires, when it was last
// used, and what it may be used for. A key whose expiry the card omits is one
// an audit cannot act on.
func TestFormatMarkdownString_SentFields(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:         8,
		Title:      "audit-key",
		Key:        "ssh-ed25519 AAAA",
		CreatedAt:  "2026-01-01T00:00:00Z",
		ExpiresAt:  "2027-02-03T04:05:00Z",
		LastUsedAt: "2026-06-07T08:09:00Z",
		UsageType:  "auth_and_signing",
		User:       UserOutput{ID: 3, Username: "dana", Name: "Dana"},
	})

	want := "## SSH Key #8\n\n" +
		"- **ID**: 8\n" +
		"- **Title**: audit-key\n" +
		"- **Key**: `ssh-ed25519 AAAA`\n" +
		"- **Usage Type**: auth_and_signing\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **Expires**: 3 Feb 2027 04:05 UTC\n" +
		"- **Last Used**: 7 Jun 2026 08:09 UTC\n" +
		"- **User**:\n" +
		"  - **ID**: 3\n" +
		"  - **Name**: Dana\n" +
		"  - **Username**: @dana\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_NoUser pins the card of a key GitLab answered with
// no owner: the nested object is absent rather than rendered as an ID of 0 and
// a bare "@".
func TestFormatMarkdownString_NoUser(t *testing.T) {
	got := FormatMarkdownString(Output{ID: 9, Title: "orphan", Key: "ssh-rsa AAAA"})

	want := "## SSH Key #9\n\n" +
		"- **ID**: 9\n" +
		"- **Title**: orphan\n" +
		"- **Key**: `ssh-rsa AAAA`\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------- Tests consolidated from coverage_test.go ----------.

// ---------------------------------------------------------------------------
// GetKeyByFingerprint — API error
// ---------------------------------------------------------------------------.

// TestGetKeyByFingerprint_APIError asserts the other handler attributes a
// GitLab refusal to its own operation, which is the half of the crossing the
// by-ID test cannot see on its own.
func TestGetKeyByFingerprint_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))

	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
	if !strings.HasPrefix(err.Error(), "key_get_by_fingerprint:") {
		t.Errorf("error %q is not attributed to key_get_by_fingerprint", err)
	}
}

// TestGetKeyByFingerprint_NotFound_HintsAtTheFingerprintFormat asserts the 404
// hint names the two fingerprint spellings GitLab accepts, since a fingerprint
// that matches nothing is far more often mistyped than absent.
func TestGetKeyByFingerprint_NotFound_HintsAtTheFingerprintFormat(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := GetKeyByFingerprint(t.Context(), client, GetByFingerprintInput{Fingerprint: "SHA256:abc123"})
	if err == nil {
		t.Fatal("expected error for a fingerprint GitLab matches to no key")
	}
	if !strings.Contains(err.Error(), "SHA256:base64") || !strings.Contains(err.Error(), "MD5:") {
		t.Errorf("error %q does not hint at the accepted fingerprint formats", err)
	}
}

// ---------------------------------------------------------------------------
// toOutput — CreatedAt populated and nil
// ---------------------------------------------------------------------------.

// TestToOutput_WithCreatedAt pins every field the converter writes at once,
// the SDK's and the captured ones together, from a key and an extra that share
// no value. It is the direct guard on the crossings the handler tests reach
// only through a round trip: both sources feed one struct literal, so the
// expiry and the last use are interchangeable there as surely as the title and
// the public key are, and a converter that assigns a neighbor has no branch
// for either gate to flip.
func TestToOutput_WithCreatedAt(t *testing.T) {
	now := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2027, 8, 9, 10, 11, 0, 0, time.UTC)
	lastUsed := time.Date(2026, 4, 5, 6, 7, 0, 0, time.UTC)
	key := &gl.Key{
		ID:        1,
		Title:     "Test",
		Key:       "ssh-rsa AAAA...",
		CreatedAt: &now,
		User: gl.User{
			ID:       10,
			Username: "tester",
			Name:     "Test User",
		},
	}

	out := toOutput(key, toolutil.KeyExtra{ExpiresAt: &expires, LastUsedAt: &lastUsed, UsageType: "signing"})

	want := Output{
		ID:         1,
		Title:      "Test",
		Key:        "ssh-rsa AAAA...",
		CreatedAt:  "2026-03-15T00:00:00Z",
		ExpiresAt:  "2027-08-09T10:11:00Z",
		LastUsedAt: "2026-04-05T06:07:00Z",
		UsageType:  "signing",
		User:       UserOutput{ID: 10, Username: "tester", Name: "Test User"},
	}
	if !reflect.DeepEqual(out, want) {
		t.Errorf("toOutput() = %+v, want %+v", out, want)
	}
}

// TestToOutput_NilCreatedAt verifies ToOutput when nil created at.
func TestToOutput_NilCreatedAt(t *testing.T) {
	key := &gl.Key{
		ID:    2,
		Title: "No Date",
		Key:   "ssh-ed25519 AAAA",
		User: gl.User{
			ID:       20,
			Username: "nodate",
			Name:     "No Date User",
		},
	}

	out := toOutput(key, toolutil.KeyExtra{})

	if out.CreatedAt != "" {
		t.Errorf("expected empty CreatedAt, got %q", out.CreatedAt)
	}
	if out.ID != 2 {
		t.Errorf("ID = %d, want 2", out.ID)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdownString — branch coverage
// ---------------------------------------------------------------------------.

// TestFormatMarkdownString_WithCreatedAt pins the whole card of a key GitLab
// sent a creation time for, that time rendered through FormatTime.
func TestFormatMarkdownString_WithCreatedAt(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:        1,
		Title:     "My Key",
		Key:       "ssh-rsa short",
		CreatedAt: "2026-01-01T00:00:00Z",
		User:      UserOutput{ID: 1, Username: "admin", Name: "Admin"},
	})

	want := "## SSH Key #1\n\n" +
		"- **ID**: 1\n" +
		"- **Title**: My Key\n" +
		"- **Key**: `ssh-rsa short`\n" +
		"- **Created**: 1 Jan 2026 00:00 UTC\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: Admin\n" +
		"  - **Username**: @admin\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_EmptyTitle pins that a key with no title renders no
// title row, rather than a label with nothing after it.
func TestFormatMarkdownString_EmptyTitle(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:   3,
		Key:  "ssh-rsa short",
		User: UserOutput{ID: 1, Username: "u", Name: "U"},
	})

	want := "## SSH Key #3\n\n" +
		"- **ID**: 3\n" +
		"- **Key**: `ssh-rsa short`\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: U\n" +
		"  - **Username**: @u\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_LongKey pins the whole card of a key longer than
// the card shows: 57 characters and an ellipsis, inside a code span.
func TestFormatMarkdownString_LongKey(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:    4,
		Title: "Long",
		Key:   strings.Repeat("A", 100),
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	})

	want := "## SSH Key #4\n\n" +
		"- **ID**: 4\n" +
		"- **Title**: Long\n" +
		"- **Key**: `" + strings.Repeat("A", 57) + "...`\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: U\n" +
		"  - **Username**: @u\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatMarkdownString_ShortKey pins the other side of the truncation: a
// key that fits is shown whole, with no ellipsis.
func TestFormatMarkdownString_ShortKey(t *testing.T) {
	got := FormatMarkdownString(Output{
		ID:    5,
		Title: "Short",
		Key:   "ssh-rsa AAAA",
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	})

	want := "## SSH Key #5\n\n" +
		"- **ID**: 5\n" +
		"- **Title**: Short\n" +
		"- **Key**: `ssh-rsa AAAA`\n" +
		"- **User**:\n" +
		"  - **ID**: 1\n" +
		"  - **Name**: U\n" +
		"  - **Username**: @u\n" +
		keyCardHints

	if got != want {
		t.Errorf("card mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ---------------------------------------------------------------------------
// FormatMarkdown — returns non-nil CallToolResult
// ---------------------------------------------------------------------------.

// TestFormatMarkdown_ReturnsResult verifies FormatMarkdown returns result.
func TestFormatMarkdown_ReturnsResult(t *testing.T) {
	out := Output{
		ID:    1,
		Title: "Test",
		Key:   "ssh-rsa AAAA",
		User:  UserOutput{ID: 1, Username: "u", Name: "U"},
	}

	result := FormatMarkdown(out)

	if result == nil {
		t.Fatal("expected non-nil CallToolResult")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
}

// ---------------------------------------------------------------------------
// truncateKey — direct tests
// ---------------------------------------------------------------------------.

// TestTruncateKey_LongKey verifies TruncateKey when long key.
func TestTruncateKey_LongKey(t *testing.T) {
	long := strings.Repeat("X", 80)
	got := truncateKey(long)

	if len(got) != 60 {
		t.Errorf("truncated length = %d, want 60", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected ellipsis suffix")
	}
	if got[:57] != long[:57] {
		t.Error("expected first 57 chars to match")
	}
}

// TestTruncateKey_ExactBoundary verifies TruncateKey when exact boundary.
func TestTruncateKey_ExactBoundary(t *testing.T) {
	exactly60 := strings.Repeat("Y", 60)
	got := truncateKey(exactly60)

	if got != exactly60 {
		t.Errorf("key of exactly 60 chars should not be truncated")
	}
}

// TestTruncateKey_ShortKey verifies TruncateKey when short key.
func TestTruncateKey_ShortKey(t *testing.T) {
	short := "ssh-rsa AAAA"
	got := truncateKey(short)

	if got != short {
		t.Errorf("short key should not be truncated, got %q", got)
	}
}

// TestTruncateKey_Empty verifies TruncateKey when empty.
func TestTruncateKey_Empty(t *testing.T) {
	got := truncateKey("")
	if got != "" {
		t.Errorf("empty key should stay empty, got %q", got)
	}
}

// TestActionSpecs_Metadata verifies key action spec metadata.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	genericUsage := "Use to execute keys domain action."
	for _, spec := range specs {
		if spec.OwnerPackage != "keys" || spec.IndividualTool.Name == "" {
			t.Fatalf("unexpected ActionSpec metadata: %+v", spec)
		}
		tool := spec.IndividualTool.Name
		if spec.Usage == genericUsage || spec.Usage == "" {
			t.Errorf("%s: Usage should be action-specific, got %q", tool, spec.Usage)
		}
		// Aliases must be distinctive natural-language phrases, not just the tool name.
		if len(spec.Aliases) < 2 {
			t.Errorf("%s: expected multiple natural-language aliases, got %v", tool, spec.Aliases)
		}
		for _, a := range spec.Aliases {
			if a == tool {
				t.Errorf("%s: alias should not equal the tool name", tool)
			}
		}
		if len(spec.RelatedActions) == 0 {
			t.Errorf("%s: expected canonical RelatedActions", tool)
		}
		for _, r := range spec.RelatedActions {
			if !strings.HasPrefix(r, "user.") {
				t.Errorf("%s: RelatedActions should target the user.* surface, got %q", tool, r)
			}
		}
		desc := spec.IndividualTool.Description
		if !strings.Contains(desc, "Returns:") || !strings.Contains(desc, "See also:") {
			t.Errorf("%s: description must use 'Returns: … See also: …' form, got %q", tool, desc)
		}
	}
}

// TestActionSpecs_Metadata_DescribesItsOwnLookup asserts that the discovery
// metadata each tool serves describes the lookup that tool performs, using the
// one word that tells the two apart: the fingerprint tool says "fingerprint"
// in its usage, its description and every alias, and the by-ID tool says it
// nowhere.
//
// TestActionSpecs_Metadata above checks that the metadata has the right shape
// and never that it landed on the right tool, so exchanging the two entries of
// keyActionMeta wholesale kept every test green. What that ships is a
// discovery surface telling a model the by-ID tool searches by fingerprint,
// which is worse than the generic placeholder it replaced: a model reading
// "Use when you have only a key fingerprint" calls it with the one parameter
// it cannot take.
func TestActionSpecs_Metadata_DescribesItsOwnLookup(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.ForbiddenHandler(t))
	specByTool := make(map[string]toolutil.ActionSpec)
	for _, spec := range ActionSpecs(client) {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tests := []struct {
		name string
		tool string
		// byFingerprint says whether every served string of this tool must
		// mention the fingerprint, or none of them may.
		byFingerprint bool
	}{
		{name: "by id", tool: "gitlab_get_key_with_user", byFingerprint: false},
		{name: "by fingerprint", tool: "gitlab_get_key_by_fingerprint", byFingerprint: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.tool)
			}
			served := append([]string{spec.Usage, spec.IndividualTool.Description}, spec.Aliases...)
			for _, text := range served {
				if got := strings.Contains(text, "fingerprint"); got != tc.byFingerprint {
					t.Errorf("%s serves %q: mentions fingerprint = %v, want %v",
						tc.tool, text, got, tc.byFingerprint)
				}
			}
		})
	}
}

// TestDecorateKeyMeta_UnknownTool verifies the no-op branch leaves generic metadata.
func TestDecorateKeyMeta_UnknownTool(t *testing.T) {
	options := toolutil.ActionSpecOptions{
		Aliases: []string{"gitlab_unknown_key_tool"},
		Usage:   "Use to execute keys domain action.",
	}
	decorateKeyMeta(&options, "gitlab_unknown_key_tool")
	if options.Usage != "Use to execute keys domain action." {
		t.Errorf("expected generic usage unchanged for unknown tool, got %q", options.Usage)
	}
	if len(options.RelatedActions) != 0 {
		t.Errorf("expected no RelatedActions for unknown tool, got %v", options.RelatedActions)
	}
}

// TestDecorateKeyMeta_EntryFillsNothing_LeavesEveryGenericOptionAlone verifies
// each of the four metadata fields is copied only when the entry supplies it.
// Both entries in the real table fill all four, so nothing else reaches the
// other side of those four guards, and a key lookup added with partial
// metadata would silently lose whatever the generic options already carried:
// an entry naming no aliases would replace the individual tool name with
// nothing, leaving the action reachable by its canonical ID alone, and one
// naming no related actions would take the per-user SSH-key surface off the
// only two tools that resolve a bare key to its owner.
func TestDecorateKeyMeta_EntryFillsNothing_LeavesEveryGenericOptionAlone(t *testing.T) {
	const probe = "gitlab_key_meta_probe"
	keyActionMeta[probe] = keyActionMetaEntry{}
	t.Cleanup(func() { delete(keyActionMeta, probe) })

	options := toolutil.ActionSpecOptions{
		Usage:          "Use to execute keys domain action.",
		Aliases:        []string{probe},
		RelatedActions: []string{"user.ssh_keys"},
	}
	options.IndividualTool.Description = "generic description"

	decorateKeyMeta(&options, probe)

	if options.Usage != "Use to execute keys domain action." {
		t.Errorf("Usage = %q, want the generic one untouched", options.Usage)
	}
	if options.IndividualTool.Description != "generic description" {
		t.Errorf("Description = %q, want the generic one untouched", options.IndividualTool.Description)
	}
	if len(options.Aliases) != 1 || options.Aliases[0] != probe {
		t.Errorf("Aliases = %v, want the generic one left as it was", options.Aliases)
	}
	if len(options.RelatedActions) != 1 || options.RelatedActions[0] != "user.ssh_keys" {
		t.Errorf("RelatedActions = %v, want the generic one left as it was", options.RelatedActions)
	}
}

// ---------------------------------------------------------------------------
// ActionSpec route execution
// ---------------------------------------------------------------------------.

// TestActionSpecs_CallRoutes drives each canonical route and holds three
// things about it that only agree by construction: the canonical action name
// the catalog registers, the individual tool name it is published under, and
// which of the two endpoints the route really reaches, read off the returned
// output by the key ID only that endpoint's answer carries.
//
// The name is asserted because nothing else in the package reads it. Crossing
// the two names over their routes (registering user.key_get_by_fingerprint
// for the by-ID handler and the reverse) compiles, leaves both IDs resolvable
// in the catalog, and kept the whole suite green. keys_catalog_test.go cannot
// see it either: it asks whether a published ID resolves, and after the
// crossing both still do. An ID that resolves to the wrong handler is worse
// than one that resolves to nothing, because the model is answered rather
// than corrected, and the card's own hint names one of these two IDs.
func TestActionSpecs_CallRoutes(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v4/keys/"):
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":42,"title":"MCP Key","key":"ssh-rsa AAAA...","created_at":"2026-06-01T12:00:00Z","user":{"id":1,"username":"admin","name":"Admin"}}`)
		case r.URL.Path == "/api/v4/keys" && r.URL.Query().Get("fingerprint") != "":
			testutil.RespondJSON(w, http.StatusOK,
				`{"id":99,"title":"FP Key","key":"ssh-ed25519 BBBB...","user":{"id":2,"username":"deploy","name":"Deploy"}}`)
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, handler)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tests := []struct {
		name string
		tool string
		// action is the canonical name the catalog registers this route
		// under, which becomes user.<action> once the specs are aggregated
		// into the gitlab_user group.
		action string
		args   map[string]any
		// wantsKeyID is the key only this route's endpoint answers with, so
		// the output says which handler ran rather than only that one did.
		wantsKeyID int64
	}{
		{
			name:       "get_key_with_user",
			tool:       "gitlab_get_key_with_user",
			action:     "key_get_with_user",
			args:       map[string]any{"key_id": 42},
			wantsKeyID: 42,
		},
		{
			name:       "get_key_by_fingerprint",
			tool:       "gitlab_get_key_by_fingerprint",
			action:     "key_get_by_fingerprint",
			args:       map[string]any{"fingerprint": "SHA256:abc123"},
			wantsKeyID: 99,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			spec, ok := specByTool[tc.tool]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tc.tool)
			}
			if spec.Name != tc.action {
				t.Errorf("%s registers action %q, want %q", tc.tool, spec.Name, tc.action)
			}
			result, err := spec.Route.Handler(t.Context(), tc.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tc.tool, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tc.tool)
			}
			out, ok := result.(Output)
			if !ok {
				t.Fatalf("Route.Handler(%s) returned %T, want keys.Output", tc.tool, result)
			}
			if out.ID != tc.wantsKeyID {
				t.Errorf("Route.Handler(%s) fetched key %d, want %d: the route reached the other endpoint",
					tc.tool, out.ID, tc.wantsKeyID)
			}
		})
	}
}
