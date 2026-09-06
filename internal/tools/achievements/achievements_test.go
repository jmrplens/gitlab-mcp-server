// achievements_test.go contains unit tests for the GitLab achievement
// handlers.
//
// Every endpoint is GraphQL, so the tests mock POST /api/graphql with
// [testutil.GraphQLHandler] keyed on a distinctive fragment of each query, and
// then call the handlers directly to verify the variables sent, the input
// validation, the error wrapping, and the conversion of the SDK types into the
// package's own output shapes. The response bodies mirror the ones the SDK's
// own achievements tests assert against, so a change in the shape the SDK
// unmarshals shows up here as a decoding failure rather than as a silent zero
// value.
package achievements

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// achievementNode is the Achievement selection every mutation and the list
// query return.
const achievementNode = `{
	"id": "gid://gitlab/Achievements::Achievement/1",
	"namespace": {"id": "gid://gitlab/Namespace/10"},
	"name": "First Commit",
	"avatarUrl": "https://example.com/badge.png",
	"description": "Awarded for the first commit",
	"createdAt": "2025-05-25T13:47:41Z",
	"updatedAt": "2025-05-25T13:47:41Z"
}`

// bareAchievementNode is the same selection with every optional field null, so
// the pointer-dereference branches of the converter are exercised too.
const bareAchievementNode = `{
	"id": "gid://gitlab/Achievements::Achievement/2",
	"namespace": {"id": "gid://gitlab/Namespace/10"},
	"name": "Second Commit",
	"avatarUrl": null,
	"description": null,
	"createdAt": "2025-05-25T13:47:41Z",
	"updatedAt": "2025-05-25T13:47:41Z"
}`

// userAchievementNode is the UserAchievement selection, with every optional
// field populated.
const userAchievementNode = `{
	"id": "gid://gitlab/Achievements::UserAchievement/88",
	"achievement": {"id": "gid://gitlab/Achievements::Achievement/1"},
	"user": {"id": "gid://gitlab/User/2"},
	"awardedByUser": {"id": "gid://gitlab/User/3"},
	"revokedByUser": {"id": "gid://gitlab/User/4"},
	"createdAt": "2025-05-25T13:47:41Z",
	"updatedAt": "2025-05-26T09:00:00Z",
	"revokedAt": "2025-05-26T09:00:00Z",
	"priority": 1,
	"showOnProfile": true,
	"awardMessage": "Shipped the first release"
}`

// bareUserAchievementNode is the same selection with the optional fields null.
const bareUserAchievementNode = `{
	"id": "gid://gitlab/Achievements::UserAchievement/89",
	"achievement": {"id": "gid://gitlab/Achievements::Achievement/1"},
	"user": {"id": "gid://gitlab/User/5"},
	"awardedByUser": {"id": "gid://gitlab/User/3"},
	"revokedByUser": null,
	"createdAt": "2025-05-25T13:47:41Z",
	"updatedAt": "2025-05-25T13:47:41Z",
	"revokedAt": null,
	"priority": null,
	"showOnProfile": false,
	"awardMessage": null
}`

const pageInfoNode = `{
	"endCursor": "cursor123",
	"hasNextPage": true,
	"startCursor": "cursor000",
	"hasPreviousPage": false
}`

// graphQLQueryKeys names the query fragment that identifies each operation to
// [testutil.GraphQLHandler]. Keeping them in one place stops a renamed
// mutation from silently matching the wrong stub.
const (
	keyCreate      = "achievementsCreate"
	keyUpdate      = "achievementsUpdate"
	keyDeleteA     = "achievementsDelete"
	keyAward       = "achievementsAward"
	keyRevoke      = "achievementsRevoke"
	keyUAUpdate    = "userAchievementsUpdate"
	keyUADelete    = "userAchievementsDelete"
	keyUAReorder   = "userAchievementPrioritiesUpdate"
	keyUserList    = "userAchievements(includeHidden:"
	keyList        = "achievements(ids: $ids"
	keyRecipients  = "userAchievements(after:"
	keyUniqueUsers = "uniqueUsers(after:"
)

// respond writes a canned GraphQL body and records the variables the handler
// sent, so a test can assert on them after the call returns rather than from
// inside the server goroutine.
func respond(t *testing.T, body string, captured *map[string]any) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		vars, err := testutil.ParseGraphQLVariables(r)
		if err != nil {
			t.Errorf("ParseGraphQLVariables error: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if captured != nil {
			*captured = vars
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}
}

// errorResponse answers every query with a top-level GraphQL error, which is
// how these endpoints report a missing namespace or a feature that is off.
func errorResponse(w http.ResponseWriter, _ *http.Request) {
	testutil.RespondJSON(w, http.StatusOK, `{"errors":[{"message":"namespace not found"}]}`)
}

// nullDataResponse answers with a well-formed body whose payload is null,
// which the SDK turns into its not-found sentinel.
func nullDataResponse(field string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{"data":{"`+field+`":null}}`)
	}
}

// cancelledContext returns a context that is already done, for the guard every
// handler runs before it touches the network.
func cancelledContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	return ctx
}

// TestCreate_Success verifies that Create sends the namespace global ID and the
// optional description, and converts the returned achievement.
func TestCreate_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyCreate: respond(t, `{"data":{"achievementsCreate":{"achievement":`+achievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Create(t.Context(), client, CreateInput{
		NamespaceID: 10,
		Name:        "  First Commit  ",
		Description: " Awarded for the first commit ",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	input, ok := vars["input"].(map[string]any)
	if !ok {
		t.Fatalf("GraphQL input = %#v, want map", vars["input"])
	}
	if got := input["namespaceId"]; got != "gid://gitlab/Namespace/10" {
		t.Errorf("namespaceId = %v, want the Namespace global ID", got)
	}
	if got := input["name"]; got != "First Commit" {
		t.Errorf("name = %v, want the trimmed name", got)
	}
	if got := input["description"]; got != "Awarded for the first commit" {
		t.Errorf("description = %v, want the trimmed description", got)
	}
	if out.Achievement.ID != 1 || out.Achievement.NamespaceID != 10 {
		t.Errorf("Achievement ids = (%d, %d), want (1, 10)", out.Achievement.ID, out.Achievement.NamespaceID)
	}
	if out.Achievement.AvatarURL != "https://example.com/badge.png" {
		t.Errorf("AvatarURL = %q, want the URL from the response", out.Achievement.AvatarURL)
	}
	if out.Achievement.CreatedAt != "2025-05-25T13:47:41Z" {
		t.Errorf("CreatedAt = %q, want the RFC 3339 timestamp", out.Achievement.CreatedAt)
	}
}

// TestCreate_OmitsEmptyDescription verifies that a blank description is left
// out of the mutation rather than sent as an empty string, which GitLab would
// store as a cleared description.
func TestCreate_OmitsEmptyDescription(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyCreate: respond(t, `{"data":{"achievementsCreate":{"achievement":`+bareAchievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Create(t.Context(), client, CreateInput{NamespaceID: 10, Name: "Second Commit", Description: "   "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if _, present := input["description"]; present {
		t.Errorf("description present in mutation input, want it omitted")
	}
	if out.Achievement.Description != "" || out.Achievement.AvatarURL != "" {
		t.Errorf("optional fields = (%q, %q), want both empty for a null response",
			out.Achievement.Description, out.Achievement.AvatarURL)
	}
}

// TestCreate_ValidationErrors verifies the guards that run before any request.
func TestCreate_ValidationErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	cases := []struct {
		name  string
		input CreateInput
		want  string
	}{
		{name: "missing namespace id", input: CreateInput{Name: "x"}, want: "namespace_id is required"},
		{name: "negative namespace id", input: CreateInput{NamespaceID: -1, Name: "x"}, want: "namespace_id is required"},
		{name: "missing name", input: CreateInput{NamespaceID: 10}, want: "name is required"},
		{name: "blank name", input: CreateInput{NamespaceID: 10, Name: "   "}, want: "name is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Create(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Create() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

// TestCreate_ContextCancelled verifies the cancellation guard fires before the
// input is even validated.
func TestCreate_ContextCancelled(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	if _, err := Create(cancelledContext(t), client, CreateInput{NamespaceID: 10, Name: "x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Create() error = %v, want context.Canceled", err)
	}
}

// TestCreate_AvatarError verifies that an unusable avatar stops the call before
// it reaches GitLab.
func TestCreate_AvatarError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	input := CreateInput{
		NamespaceID:         10,
		Name:                "x",
		AvatarFilename:      "badge.png",
		AvatarContentBase64: "!!not base64!!",
	}
	if _, err := Create(t.Context(), client, input); err == nil {
		t.Fatal("Create() error = nil, want a decoding failure")
	}
}

// TestCreate_APIError verifies that a GraphQL-level failure is wrapped with the
// namespace hint rather than surfaced raw.
func TestCreate_APIError(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyCreate: errorResponse,
	}))
	_, err := Create(t.Context(), client, CreateInput{NamespaceID: 10, Name: "x"})
	if err == nil || !strings.Contains(err.Error(), "namespace_id names an existing group") {
		t.Errorf("Create() error = %v, want the namespace hint", err)
	}
}

// TestCreate_SendsAvatarAsMultipart verifies the avatar really leaves the
// process: the SDK switches to a multipart request when the mutation carries an
// upload, so the assertion is on the file part rather than on the JSON body.
func TestCreate_SendsAvatarAsMultipart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "badge.png")
	if err := os.WriteFile(path, []byte("PNG-BYTES"), 0o600); err != nil {
		t.Fatalf("write avatar fixture: %v", err)
	}
	t.Setenv(toolutil.UploadDirAllowlistEnv, dir)

	cases := []struct {
		name  string
		input AvatarInput
	}{
		{
			name:  "from a local path",
			input: AvatarInput{AvatarFilePath: path, AvatarFilename: "badge.png", AvatarContentType: "image/png"},
		},
		{
			name: "from inline base64",
			input: AvatarInput{
				AvatarContentBase64: base64.StdEncoding.EncodeToString([]byte("PNG-BYTES")),
				AvatarFilename:      "badge.png",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var uploaded string
			client := testutil.NewTestClient(t, uploadCapturingHandler(t, &uploaded,
				`{"data":{"achievementsCreate":{"achievement":`+achievementNode+`,"errors":[]}}}`))

			if _, err := Create(t.Context(), client, CreateInput{NamespaceID: 10, Name: "x", AvatarInput: tc.input}); err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if uploaded != "badge.png:PNG-BYTES" {
				t.Errorf("uploaded part = %q, want the avatar file name and its bytes", uploaded)
			}
		})
	}
}

// uploadCapturingHandler answers a multipart GraphQL request with body, after
// recording the single uploaded part as "filename:content" in captured. It runs
// on the httptest server goroutine, so every failure is reported with t.Errorf
// and answered deterministically rather than aborting the test from there.
func uploadCapturingHandler(t *testing.T, captured *string, body string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		if err := r.ParseMultipartForm(1 << 20); err != nil { //nolint:gosec // Test handler bounds r.Body with http.MaxBytesReader above.
			t.Errorf("ParseMultipartForm error: %v, want a multipart upload request", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		for _, headers := range r.MultipartForm.File {
			for _, header := range headers {
				*captured = readUploadedPart(t, header)
			}
		}
		testutil.RespondJSON(w, http.StatusOK, body)
	}
}

// readUploadedPart returns one multipart file part as "filename:content", or
// an empty string when it could not be read.
func readUploadedPart(t *testing.T, header *multipart.FileHeader) string {
	t.Helper()
	file, err := header.Open()
	if err != nil {
		t.Errorf("open uploaded part: %v", err)
		return ""
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Errorf("read uploaded part: %v", err)
		return ""
	}
	return header.Filename + ":" + string(content)
}

// TestUpdate_Success verifies that Update sends the achievement global ID and
// only the fields the caller filled in.
func TestUpdate_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUpdate: respond(t, `{"data":{"achievementsUpdate":{"achievement":`+achievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Update(t.Context(), client, UpdateInput{AchievementID: 1, Name: "Renamed", Description: "New text"})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if got := input["achievementId"]; got != "gid://gitlab/Achievements::Achievement/1" {
		t.Errorf("achievementId = %v, want the Achievement global ID", got)
	}
	if got := input["name"]; got != "Renamed" {
		t.Errorf("name = %v, want Renamed", got)
	}
	if got := input["description"]; got != "New text" {
		t.Errorf("description = %v, want New text", got)
	}
	if out.Achievement.Name != "First Commit" {
		t.Errorf("Name = %q, want the name from the response", out.Achievement.Name)
	}
}

// TestUpdate_OmitsUnsetFields verifies that an omitted field stays out of the
// mutation, which is what makes an update a partial one.
func TestUpdate_OmitsUnsetFields(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUpdate: respond(t, `{"data":{"achievementsUpdate":{"achievement":`+achievementNode+`,"errors":[]}}}`, &vars),
	}))

	if _, err := Update(t.Context(), client, UpdateInput{AchievementID: 1}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	for _, field := range []string{"name", "description", "avatar"} {
		t.Run(field, func(t *testing.T) {
			if _, present := input[field]; present {
				t.Errorf("%s present in mutation input, want it omitted", field)
			}
		})
	}
}

// TestUpdate_ValidationAndErrors verifies the guard, the cancellation check,
// the avatar failure, and the wrapped API error.
func TestUpdate_ValidationAndErrors(t *testing.T) {
	t.Run("missing achievement id", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := Update(t.Context(), client, UpdateInput{})
		if err == nil || !strings.Contains(err.Error(), "achievement_id is required") {
			t.Errorf("Update() error = %v, want the achievement_id guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		if _, err := Update(cancelledContext(t), client, UpdateInput{AchievementID: 1}); !errors.Is(err, context.Canceled) {
			t.Errorf("Update() error = %v, want context.Canceled", err)
		}
	})
	t.Run("unusable avatar", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		input := UpdateInput{AchievementID: 1, AvatarFilename: "badge.png", AvatarContentBase64: "%%%"}
		if _, err := Update(t.Context(), client, input); err == nil {
			t.Error("Update() error = nil, want a decoding failure")
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUpdate: errorResponse}))
		_, err := Update(t.Context(), client, UpdateInput{AchievementID: 1})
		if err == nil || !strings.Contains(err.Error(), "achievement_id is the numeric ID") {
			t.Errorf("Update() error = %v, want the achievement_id hint", err)
		}
	})
}

// TestDelete_Success verifies that Delete reports the removal and returns the
// achievement the mutation echoed back.
func TestDelete_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyDeleteA: respond(t, `{"data":{"achievementsDelete":{"achievement":`+achievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Delete(t.Context(), client, DeleteInput{AchievementID: 1})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "every award") {
		t.Errorf("Delete() status/message = (%q, %q), want a success naming the awards it took with it", out.Status, out.Message)
	}
	if out.Achievement.ID != 1 {
		t.Errorf("Achievement.ID = %d, want 1", out.Achievement.ID)
	}
	input, _ := vars["input"].(map[string]any)
	if got := input["achievementId"]; got != "gid://gitlab/Achievements::Achievement/1" {
		t.Errorf("achievementId = %v, want the Achievement global ID", got)
	}
}

// TestDelete_ValidationAndErrors verifies the guard, the cancellation check,
// and that a null payload becomes the not-found message with the feature-flag
// note attached.
func TestDelete_ValidationAndErrors(t *testing.T) {
	t.Run("missing achievement id", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := Delete(t.Context(), client, DeleteInput{})
		if err == nil || !strings.Contains(err.Error(), "achievement_id is required") {
			t.Errorf("Delete() error = %v, want the achievement_id guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		if _, err := Delete(cancelledContext(t), client, DeleteInput{AchievementID: 1}); !errors.Is(err, context.Canceled) {
			t.Errorf("Delete() error = %v, want context.Canceled", err)
		}
	})
	t.Run("not found carries the availability note", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
			keyDeleteA: nullDataResponse("achievementsDelete"),
		}))
		_, err := Delete(t.Context(), client, DeleteInput{AchievementID: 1})
		if err == nil || !strings.Contains(err.Error(), "achievements feature flag") {
			t.Errorf("Delete() error = %v, want the availability note on a not-found", err)
		}
	})
}

// TestAward_Success verifies that Award sends both global IDs and the message,
// and returns the award with its own separate ID.
func TestAward_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyAward: respond(t, `{"data":{"achievementsAward":{"userAchievement":`+userAchievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Award(t.Context(), client, AwardInput{AchievementID: 1, UserID: 2, AwardMessage: " Shipped the first release "})
	if err != nil {
		t.Fatalf("Award() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if got := input["achievementId"]; got != "gid://gitlab/Achievements::Achievement/1" {
		t.Errorf("achievementId = %v, want the Achievement global ID", got)
	}
	if got := input["userId"]; got != "gid://gitlab/User/2" {
		t.Errorf("userId = %v, want the User global ID", got)
	}
	if got := input["awardMessage"]; got != "Shipped the first release" {
		t.Errorf("awardMessage = %v, want the trimmed message", got)
	}
	if out.UserAchievement.ID != 88 || out.UserAchievement.AchievementID != 1 {
		t.Errorf("award ids = (%d, %d), want the award ID 88 and achievement ID 1",
			out.UserAchievement.ID, out.UserAchievement.AchievementID)
	}
	if out.UserAchievement.RevokedByUserID == nil || *out.UserAchievement.RevokedByUserID != 4 {
		t.Errorf("RevokedByUserID = %v, want 4", out.UserAchievement.RevokedByUserID)
	}
	if out.UserAchievement.Priority == nil || *out.UserAchievement.Priority != 1 {
		t.Errorf("Priority = %v, want 1", out.UserAchievement.Priority)
	}
}

// TestAward_OmitsBlankMessage verifies a whitespace-only message is dropped
// rather than sent, and that the null-heavy node converts to zero values.
func TestAward_OmitsBlankMessage(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyAward: respond(t, `{"data":{"achievementsAward":{"userAchievement":`+bareUserAchievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Award(t.Context(), client, AwardInput{AchievementID: 1, UserID: 5, AwardMessage: "  "})
	if err != nil {
		t.Fatalf("Award() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if _, present := input["awardMessage"]; present {
		t.Errorf("awardMessage present in mutation input, want it omitted")
	}
	if out.UserAchievement.AwardMessage != "" || out.UserAchievement.RevokedByUserID != nil ||
		out.UserAchievement.Priority != nil || out.UserAchievement.RevokedAt != "" {
		t.Errorf("optional fields = %+v, want them all empty for a null response", out.UserAchievement)
	}
}

// TestAward_ValidationAndErrors verifies both required-ID guards, the
// cancellation check, and the wrapped API error.
func TestAward_ValidationAndErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	cases := []struct {
		name  string
		input AwardInput
		want  string
	}{
		{name: "missing achievement id", input: AwardInput{UserID: 2}, want: "achievement_id is required"},
		{name: "missing user id", input: AwardInput{AchievementID: 1}, want: "user_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Award(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Award() error = %v, want containing %q", err, tc.want)
			}
		})
	}
	t.Run("context cancelled", func(t *testing.T) {
		if _, err := Award(cancelledContext(t), client, AwardInput{AchievementID: 1, UserID: 2}); !errors.Is(err, context.Canceled) {
			t.Errorf("Award() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyAward: errorResponse}))
		_, err := Award(t.Context(), failing, AwardInput{AchievementID: 1, UserID: 2})
		if err == nil || !strings.Contains(err.Error(), "200 characters") {
			t.Errorf("Award() error = %v, want the award hint", err)
		}
	})
}

// TestRevoke_Success verifies that Revoke addresses the award, not the
// achievement, and says the record was kept.
func TestRevoke_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyRevoke: respond(t, `{"data":{"achievementsRevoke":{"userAchievement":`+userAchievementNode+`,"errors":[]}}}`, &vars),
	}))

	out, err := Revoke(t.Context(), client, RevokeInput{UserAchievementID: 88})
	if err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if got := input["userAchievementId"]; got != "gid://gitlab/Achievements::UserAchievement/88" {
		t.Errorf("userAchievementId = %v, want the UserAchievement global ID", got)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "kept") {
		t.Errorf("Revoke() status/message = (%q, %q), want a success saying the record is kept", out.Status, out.Message)
	}
	if out.UserAchievement.RevokedAt == "" {
		t.Error("RevokedAt = empty, want the revocation timestamp from the response")
	}
}

// TestRevoke_ValidationAndErrors verifies the guard, the cancellation check,
// and the wrapped API error.
func TestRevoke_ValidationAndErrors(t *testing.T) {
	t.Run("missing award id", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := Revoke(t.Context(), client, RevokeInput{})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is required") {
			t.Errorf("Revoke() error = %v, want the user_achievement_id guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		if _, err := Revoke(cancelledContext(t), client, RevokeInput{UserAchievementID: 88}); !errors.Is(err, context.Canceled) {
			t.Errorf("Revoke() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyRevoke: errorResponse}))
		_, err := Revoke(t.Context(), client, RevokeInput{UserAchievementID: 88})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is the numeric ID") {
			t.Errorf("Revoke() error = %v, want the award-ID hint", err)
		}
	})
}

// TestUserAchievementUpdate_Success verifies that the visibility flag is passed
// through as given.
func TestUserAchievementUpdate_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUAUpdate: respond(t, `{"data":{"userAchievementsUpdate":{"userAchievement":`+userAchievementNode+`,"errors":[]}}}`, &vars),
	}))

	show := false
	out, err := UserAchievementUpdate(t.Context(), client, UserAchievementUpdateInput{UserAchievementID: 88, ShowOnProfile: &show})
	if err != nil {
		t.Fatalf("UserAchievementUpdate() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	if got := input["showOnProfile"]; got != false {
		t.Errorf("showOnProfile = %v, want false", got)
	}
	if !out.UserAchievement.ShowOnProfile {
		t.Error("ShowOnProfile = false, want the value the response reported")
	}
}

// TestUserAchievementUpdate_ValidationAndErrors verifies the guard, the
// cancellation check, and the wrapped API error.
func TestUserAchievementUpdate_ValidationAndErrors(t *testing.T) {
	t.Run("missing award id", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := UserAchievementUpdate(t.Context(), client, UserAchievementUpdateInput{})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is required") {
			t.Errorf("UserAchievementUpdate() error = %v, want the user_achievement_id guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := UserAchievementUpdate(cancelledContext(t), client, UserAchievementUpdateInput{UserAchievementID: 88})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("UserAchievementUpdate() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUAUpdate: errorResponse}))
		_, err := UserAchievementUpdate(t.Context(), client, UserAchievementUpdateInput{UserAchievementID: 88})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is the numeric ID") {
			t.Errorf("UserAchievementUpdate() error = %v, want the award-ID hint", err)
		}
	})
}

// TestUserAchievementDelete_Success verifies the removal is reported as an
// erasure rather than as a revocation.
func TestUserAchievementDelete_Success(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUADelete: respond(t, `{"data":{"userAchievementsDelete":{"userAchievement":`+userAchievementNode+`,"errors":[]}}}`, nil),
	}))

	out, err := UserAchievementDelete(t.Context(), client, UserAchievementDeleteInput{UserAchievementID: 88})
	if err != nil {
		t.Fatalf("UserAchievementDelete() error = %v", err)
	}
	if out.Status != "success" || !strings.Contains(out.Message, "deleted the award record") {
		t.Errorf("status/message = (%q, %q), want a success naming the deleted record", out.Status, out.Message)
	}
}

// TestUserAchievementDelete_ValidationAndErrors verifies the guard, the
// cancellation check, and the wrapped API error.
func TestUserAchievementDelete_ValidationAndErrors(t *testing.T) {
	t.Run("missing award id", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := UserAchievementDelete(t.Context(), client, UserAchievementDeleteInput{})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is required") {
			t.Errorf("UserAchievementDelete() error = %v, want the user_achievement_id guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := UserAchievementDelete(cancelledContext(t), client, UserAchievementDeleteInput{UserAchievementID: 88})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("UserAchievementDelete() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUADelete: errorResponse}))
		_, err := UserAchievementDelete(t.Context(), client, UserAchievementDeleteInput{UserAchievementID: 88})
		if err == nil || !strings.Contains(err.Error(), "user_achievement_id is the numeric ID") {
			t.Errorf("UserAchievementDelete() error = %v, want the award-ID hint", err)
		}
	})
}

// TestUserAchievementReorder_Success verifies every ID becomes a global ID in
// the order given, and that the whole reordered set comes back.
func TestUserAchievementReorder_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUAReorder: respond(t, `{"data":{"userAchievementPrioritiesUpdate":{"userAchievements":[`+
			userAchievementNode+`,`+bareUserAchievementNode+`],"errors":[]}}}`, &vars),
	}))

	out, err := UserAchievementReorder(t.Context(), client, UserAchievementReorderInput{UserAchievementIDs: []int64{88, 89}})
	if err != nil {
		t.Fatalf("UserAchievementReorder() error = %v", err)
	}
	input, _ := vars["input"].(map[string]any)
	ids, ok := input["userAchievementIds"].([]any)
	if !ok || len(ids) != 2 {
		t.Fatalf("userAchievementIds = %#v, want two global IDs", input["userAchievementIds"])
	}
	if ids[0] != "gid://gitlab/Achievements::UserAchievement/88" || ids[1] != "gid://gitlab/Achievements::UserAchievement/89" {
		t.Errorf("userAchievementIds = %v, want the IDs in the order given", ids)
	}
	if len(out.UserAchievements) != 2 || out.Status != "success" {
		t.Errorf("reorder result = (%d awards, %q), want two awards and a success", len(out.UserAchievements), out.Status)
	}
}

// TestUserAchievementReorder_ValidationAndErrors verifies the empty-list guard,
// the per-ID guard, the cancellation check, and the wrapped API error.
func TestUserAchievementReorder_ValidationAndErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	cases := []struct {
		name  string
		input UserAchievementReorderInput
		want  string
	}{
		{name: "no ids", input: UserAchievementReorderInput{}, want: "user_achievement_ids is required"},
		{name: "zero id in the list", input: UserAchievementReorderInput{UserAchievementIDs: []int64{88, 0}}, want: "user_achievement_ids is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UserAchievementReorder(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("UserAchievementReorder() error = %v, want containing %q", err, tc.want)
			}
		})
	}
	t.Run("context cancelled", func(t *testing.T) {
		_, err := UserAchievementReorder(cancelledContext(t), client, UserAchievementReorderInput{UserAchievementIDs: []int64{88}})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("UserAchievementReorder() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUAReorder: errorResponse}))
		_, err := UserAchievementReorder(t.Context(), failing, UserAchievementReorderInput{UserAchievementIDs: []int64{88}})
		if err == nil || !strings.Contains(err.Error(), "same user") {
			t.Errorf("UserAchievementReorder() error = %v, want the same-user hint", err)
		}
	})
}

// TestUserList_Success verifies the username, the hidden-awards flag and the
// cursor all reach the query, and that the page metadata is carried through.
func TestUserList_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUserList: respond(t, `{"data":{"user":{"userAchievements":{"nodes":[`+
			userAchievementNode+`],"pageInfo":`+pageInfoNode+`}}}}`, &vars),
	}))

	includeHidden := true
	input := UserListInput{
		Username:      "  jsmith  ",
		IncludeHidden: &includeHidden,
		After:         "cursor000",
		First:         new(int64(50)),
	}
	out, err := UserList(t.Context(), client, input)
	if err != nil {
		t.Fatalf("UserList() error = %v", err)
	}
	if got := vars["username"]; got != "jsmith" {
		t.Errorf("username = %v, want the trimmed name", got)
	}
	if got := vars["includeHidden"]; got != true {
		t.Errorf("includeHidden = %v, want true", got)
	}
	if got := vars["after"]; got != "cursor000" {
		t.Errorf("after = %v, want cursor000", got)
	}
	if got := vars["first"]; got != float64(50) {
		t.Errorf("first = %v, want 50", got)
	}
	if len(out.UserAchievements) != 1 {
		t.Fatalf("UserAchievements = %d, want 1", len(out.UserAchievements))
	}
	if !out.Pagination.HasNextPage || out.Pagination.EndCursor != "cursor123" {
		t.Errorf("Pagination = %+v, want the cursor metadata from the response", out.Pagination)
	}
}

// TestUserList_ValidationAndErrors verifies the guard, the cancellation check,
// and the wrapped API error.
func TestUserList_ValidationAndErrors(t *testing.T) {
	t.Run("blank username", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := UserList(t.Context(), client, UserListInput{Username: "   "})
		if err == nil || !strings.Contains(err.Error(), "username is required") {
			t.Errorf("UserList() error = %v, want the username guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		if _, err := UserList(cancelledContext(t), client, UserListInput{Username: "jsmith"}); !errors.Is(err, context.Canceled) {
			t.Errorf("UserList() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUserList: errorResponse}))
		_, err := UserList(t.Context(), client, UserListInput{Username: "jsmith"})
		if err == nil || !strings.Contains(err.Error(), "without a leading @") {
			t.Errorf("UserList() error = %v, want the username hint", err)
		}
	})
}

// TestList_Success verifies the namespace path and the ID filter reach the
// query, and that the default page size is applied when the caller names none.
func TestList_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyList: respond(t, `{"data":{"namespace":{"achievements":{"nodes":[`+
			achievementNode+`,`+bareAchievementNode+`],"pageInfo":`+pageInfoNode+`}}}}`, &vars),
	}))

	out, err := List(t.Context(), client, ListInput{FullPath: " my-group/my-project ", IDs: []int64{1, 2}})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := vars["fullPath"]; got != "my-group/my-project" {
		t.Errorf("fullPath = %v, want the trimmed path", got)
	}
	ids, ok := vars["ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != "gid://gitlab/Achievements::Achievement/1" {
		t.Errorf("ids = %#v, want the achievement global IDs", vars["ids"])
	}
	if got := vars["first"]; got != float64(toolutil.GraphQLDefaultFirst) {
		t.Errorf("first = %v, want the default page size", got)
	}
	if len(out.Achievements) != 2 {
		t.Fatalf("Achievements = %d, want 2", len(out.Achievements))
	}
	if out.Achievements[1].Description != "" {
		t.Errorf("second Description = %q, want empty for a null response field", out.Achievements[1].Description)
	}
}

// TestList_ValidationAndErrors verifies the guard, the cancellation check, and
// the wrapped API error.
func TestList_ValidationAndErrors(t *testing.T) {
	t.Run("blank full path", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		_, err := List(t.Context(), client, ListInput{FullPath: "  "})
		if err == nil || !strings.Contains(err.Error(), "full_path is required") {
			t.Errorf("List() error = %v, want the full_path guard", err)
		}
	})
	t.Run("context cancelled", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
		if _, err := List(cancelledContext(t), client, ListInput{FullPath: "my-group"}); !errors.Is(err, context.Canceled) {
			t.Errorf("List() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyList: errorResponse}))
		_, err := List(t.Context(), client, ListInput{FullPath: "my-group"})
		if err == nil || !strings.Contains(err.Error(), "full_path is the group or project path") {
			t.Errorf("List() error = %v, want the namespace-path hint", err)
		}
	})
}

// TestRecipients_Success verifies the achievement is named as a single-element
// ID list, the way the nested query expects it, and that the inner connection's
// page info is the one reported.
func TestRecipients_Success(t *testing.T) {
	var vars map[string]any
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyRecipients: respond(t, `{"data":{"namespace":{"achievements":{"nodes":[{"userAchievements":{"nodes":[`+
			userAchievementNode+`],"pageInfo":`+pageInfoNode+`}}]}}}}`, &vars),
	}))

	input := RecipientsInput{
		FullPath:      "my-group",
		AchievementID: 1,
		Before:        "cursor000",
		Last:          new(int64(5)),
	}
	out, err := Recipients(t.Context(), client, input)
	if err != nil {
		t.Fatalf("Recipients() error = %v", err)
	}
	ids, ok := vars["achievementId"].([]any)
	if !ok || len(ids) != 1 || ids[0] != "gid://gitlab/Achievements::Achievement/1" {
		t.Errorf("achievementId = %#v, want a one-element list of the global ID", vars["achievementId"])
	}
	if got := vars["before"]; got != "cursor000" {
		t.Errorf("before = %v, want cursor000", got)
	}
	if got := vars["last"]; got != float64(5) {
		t.Errorf("last = %v, want 5", got)
	}
	if vars["first"] != nil {
		t.Errorf("first = %v, want it unset when the caller paged backwards", vars["first"])
	}
	if len(out.UserAchievements) != 1 || out.Pagination.EndCursor != "cursor123" {
		t.Errorf("Recipients() = (%d awards, %+v), want one award and the inner page info",
			len(out.UserAchievements), out.Pagination)
	}
}

// TestRecipients_ValidationAndErrors verifies both guards, the cancellation
// check, and the wrapped API error.
func TestRecipients_ValidationAndErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	cases := []struct {
		name  string
		input RecipientsInput
		want  string
	}{
		{name: "blank full path", input: RecipientsInput{AchievementID: 1}, want: "full_path is required"},
		{name: "missing achievement id", input: RecipientsInput{FullPath: "my-group"}, want: "achievement_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Recipients(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Recipients() error = %v, want containing %q", err, tc.want)
			}
		})
	}
	t.Run("context cancelled", func(t *testing.T) {
		_, err := Recipients(cancelledContext(t), client, RecipientsInput{FullPath: "my-group", AchievementID: 1})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Recipients() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyRecipients: errorResponse}))
		_, err := Recipients(t.Context(), failing, RecipientsInput{FullPath: "my-group", AchievementID: 1})
		if err == nil || !strings.Contains(err.Error(), "achievement_id is the numeric ID") {
			t.Errorf("Recipients() error = %v, want the achievement-ID hint", err)
		}
	})
}

// TestUniqueUsers_Success verifies the distinct-recipient query returns user
// profiles rather than award records.
func TestUniqueUsers_Success(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{
		keyUniqueUsers: respond(t, `{"data":{"namespace":{"achievements":{"nodes":[{"uniqueUsers":{"nodes":[{
			"id": "gid://gitlab/User/2",
			"username": "octocat",
			"name": "Octo Cat",
			"state": "active",
			"createdAt": "2025-05-25T13:47:41Z",
			"avatarUrl": "https://example.com/avatar.png",
			"webUrl": "https://example.com/octocat"
		}],"pageInfo":`+pageInfoNode+`}}]}}}}`, nil),
	}))

	out, err := UniqueUsers(t.Context(), client, UniqueUsersInput{FullPath: "my-group", AchievementID: 1})
	if err != nil {
		t.Fatalf("UniqueUsers() error = %v", err)
	}
	if len(out.Users) != 1 {
		t.Fatalf("Users = %d, want 1", len(out.Users))
	}
	if out.Users[0].Username != "octocat" || out.Users[0].WebURL != "https://example.com/octocat" {
		t.Errorf("user = %+v, want the octocat profile", out.Users[0])
	}
	if !out.Pagination.HasNextPage {
		t.Error("HasNextPage = false, want the page info from the inner connection")
	}
}

// TestUniqueUsers_ValidationAndErrors verifies both guards, the cancellation
// check, and the wrapped API error.
func TestUniqueUsers_ValidationAndErrors(t *testing.T) {
	client := testutil.NewTestClient(t, testutil.GraphQLHandler(nil))
	cases := []struct {
		name  string
		input UniqueUsersInput
		want  string
	}{
		{name: "blank full path", input: UniqueUsersInput{AchievementID: 1}, want: "full_path is required"},
		{name: "missing achievement id", input: UniqueUsersInput{FullPath: "my-group"}, want: "achievement_id is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := UniqueUsers(t.Context(), client, tc.input)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("UniqueUsers() error = %v, want containing %q", err, tc.want)
			}
		})
	}
	t.Run("context cancelled", func(t *testing.T) {
		_, err := UniqueUsers(cancelledContext(t), client, UniqueUsersInput{FullPath: "my-group", AchievementID: 1})
		if !errors.Is(err, context.Canceled) {
			t.Errorf("UniqueUsers() error = %v, want context.Canceled", err)
		}
	})
	t.Run("api error", func(t *testing.T) {
		failing := testutil.NewTestClient(t, testutil.GraphQLHandler(map[string]http.HandlerFunc{keyUniqueUsers: errorResponse}))
		_, err := UniqueUsers(t.Context(), failing, UniqueUsersInput{FullPath: "my-group", AchievementID: 1})
		if err == nil || !strings.Contains(err.Error(), "achievement_id is the numeric ID") {
			t.Errorf("UniqueUsers() error = %v, want the achievement-ID hint", err)
		}
	})
}

// TestAvatarInput_Upload covers the branches of the dual file shape that no
// handler test reaches: no avatar at all, a missing file name, and a
// constructor that fails on the reader.
func TestAvatarInput_Upload(t *testing.T) {
	t.Run("no avatar returns no upload", func(t *testing.T) {
		upload, cleanup, err := AvatarInput{}.upload(opCreate)
		if err != nil || upload != nil {
			t.Errorf("upload() = (%v, %v), want no upload and no error", upload, err)
		}
		cleanup()
	})
	t.Run("missing file name is refused", func(t *testing.T) {
		_, cleanup, err := AvatarInput{AvatarContentBase64: "Zm9v"}.upload(opCreate)
		if err == nil || !strings.Contains(err.Error(), "avatar_filename is required") {
			t.Errorf("upload() error = %v, want the avatar_filename guard", err)
		}
		cleanup()
	})
	t.Run("both sources at once is refused", func(t *testing.T) {
		_, cleanup, err := AvatarInput{
			AvatarFilename:      "badge.png",
			AvatarFilePath:      "/tmp/badge.png",
			AvatarContentBase64: "Zm9v",
		}.upload(opCreate)
		if err == nil || !strings.Contains(err.Error(), "not both") {
			t.Errorf("upload() error = %v, want the mutual-exclusion guard", err)
		}
		cleanup()
	})
	t.Run("a failing constructor is wrapped", func(t *testing.T) {
		original := newGraphQLUpload
		t.Cleanup(func() { newGraphQLUpload = original })
		newGraphQLUpload = func(io.Reader, string, string) (*gl.GraphQLUpload, error) {
			return nil, errors.New("unreadable")
		}
		_, cleanup, err := AvatarInput{AvatarFilename: "badge.png", AvatarContentBase64: "Zm9v"}.upload(opCreate)
		if err == nil || !strings.Contains(err.Error(), "avatar_filename must be set") {
			t.Errorf("upload() error = %v, want the wrapped constructor failure", err)
		}
		cleanup()
	})
}

// TestCursorInput_Resolve verifies that exactly one of first and last is ever
// produced, which is what a GraphQL connection accepts.
func TestCursorInput_Resolve(t *testing.T) {
	cases := []struct {
		name      string
		input     CursorInput
		wantFirst *int64
		wantLast  *int64
	}{
		{name: "no page size defaults forward", input: CursorInput{}, wantFirst: new(int64(toolutil.GraphQLDefaultFirst))},
		{name: "first is honored", input: CursorInput{First: new(int64(7))}, wantFirst: new(int64(7))},
		{name: "last is honored", input: CursorInput{Last: new(int64(9))}, wantLast: new(int64(9))},
		{name: "first wins over last", input: CursorInput{First: new(int64(3)), Last: new(int64(9))}, wantFirst: new(int64(3))},
		{name: "first is clamped up", input: CursorInput{First: new(int64(0))}, wantFirst: new(int64(1))},
		{name: "first is clamped down", input: CursorInput{First: new(int64(5000))}, wantFirst: new(int64(toolutil.GraphQLMaxFirst))},
		{name: "last is clamped down", input: CursorInput{Last: new(int64(5000))}, wantLast: new(int64(toolutil.GraphQLMaxFirst))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, first, last := tc.input.resolve()
			if !equalInt64Ptr(first, tc.wantFirst) {
				t.Errorf("first = %v, want %v", derefInt64(first), derefInt64(tc.wantFirst))
			}
			if !equalInt64Ptr(last, tc.wantLast) {
				t.Errorf("last = %v, want %v", derefInt64(last), derefInt64(tc.wantLast))
			}
		})
	}

	t.Run("cursors are passed through", func(t *testing.T) {
		after, before, _, _ := CursorInput{After: "a", Before: "b"}.resolve()
		if after == nil || *after != "a" || before == nil || *before != "b" {
			t.Errorf("cursors = (%v, %v), want a and b", after, before)
		}
	})
	t.Run("absent cursors stay nil", func(t *testing.T) {
		after, before, _, _ := CursorInput{}.resolve()
		if after != nil || before != nil {
			t.Errorf("cursors = (%v, %v), want both nil", after, before)
		}
	})
}

func equalInt64Ptr(got, want *int64) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}
	return *got == *want
}

func derefInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// TestPagination_MissingMetadata verifies the cursor block is zero rather than
// a panic when the SDK hung no page info off the response, which is every
// mutation and every error path.
func TestPagination_MissingMetadata(t *testing.T) {
	cases := []struct {
		name string
		resp *gl.Response
	}{
		{name: "no response", resp: nil},
		{name: "response without page info", resp: &gl.Response{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pagination(tc.resp); got != (toolutil.GraphQLPaginationOutput{}) {
				t.Errorf("pagination() = %+v, want the zero value", got)
			}
		})
	}
}

// TestConverters_NilInput verifies the converters answer a nil SDK value with a
// zero struct. The SDK returns its not-found sentinel rather than a nil value
// today, so these are the defensive branches no handler test can reach.
func TestConverters_NilInput(t *testing.T) {
	t.Run("achievement", func(t *testing.T) {
		if got := toAchievement(nil); got != (Achievement{}) {
			t.Errorf("toAchievement(nil) = %+v, want the zero value", got)
		}
	})
	t.Run("user achievement", func(t *testing.T) {
		if got := toUserAchievement(nil); got != (UserAchievement{}) {
			t.Errorf("toUserAchievement(nil) = %+v, want the zero value", got)
		}
	})
	t.Run("user achievement slice", func(t *testing.T) {
		if got := toUserAchievements(nil); len(got) != 0 {
			t.Errorf("toUserAchievements(nil) = %v, want an empty slice", got)
		}
	})
}
