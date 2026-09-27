package awardemoji

import (
	"context"
	"net/http"
	"strings"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// customAward is one award of a custom emoji, as Entities::AwardEmoji renders
// it with the image URL client-go's AwardEmoji does not model.
const customAward = `{"id":7,"name":"party_parrot","user":{"id":1,"username":"admin"},"awardable_id":5,"awardable_type":"Issue",` +
	`"url":"https://gitlab.example.com/uploads/-/system/custom_emoji/file/1/parrot.gif"}`

// customAwardURL is the image URL [customAward] carries.
const customAwardURL = "https://gitlab.example.com/uploads/-/system/custom_emoji/file/1/parrot.gif"

// TestAwardHandlers_ACustomEmoji_CarriesItsImageURL verifies that every
// handler returning awards reads the image URL off the captured answer: the
// three reads of one award and the three creates on an issue, a merge request
// and a snippet, the three reads on a note, the create on a note, and the four
// lists. client-go drops the key, so a handler that forgot the capture would
// publish an award with no image and pass every other test.
func TestAwardHandlers_ACustomEmoji_CarriesItsImageURL(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/award_emoji") && r.Method == http.MethodGet {
			testutil.RespondJSON(w, http.StatusOK, "["+customAward+"]")
			return
		}
		testutil.RespondJSON(w, http.StatusOK, customAward)
	}))
	ctx := t.Context()
	singles := []struct {
		name string
		call func(context.Context) (Output, error)
	}{
		{name: "issue get", call: func(ctx context.Context) (Output, error) {
			return GetIssueAwardEmoji(ctx, client, IssueGetInput{ProjectID: testProjectID, IID: 5, AwardID: 7})
		}},
		{name: "issue create", call: func(ctx context.Context) (Output, error) {
			return CreateIssueAwardEmoji(ctx, client, IssueCreateInput{ProjectID: testProjectID, IID: 5, Name: "party_parrot"})
		}},
		{name: "issue note get", call: func(ctx context.Context) (Output, error) {
			return GetIssueNoteAwardEmoji(ctx, client, IssueGetOnNoteInput{ProjectID: testProjectID, IID: 5, NoteID: 2, AwardID: 7})
		}},
		{name: "issue note create", call: func(ctx context.Context) (Output, error) {
			return CreateIssueNoteAwardEmoji(ctx, client, IssueCreateOnNoteInput{ProjectID: testProjectID, IID: 5, NoteID: 2, Name: "party_parrot"})
		}},
		{name: "merge request get", call: func(ctx context.Context) (Output, error) {
			return GetMRAwardEmoji(ctx, client, MRGetInput{ProjectID: testProjectID, IID: 5, AwardID: 7})
		}},
		{name: "merge request create", call: func(ctx context.Context) (Output, error) {
			return CreateMRAwardEmoji(ctx, client, MRCreateInput{ProjectID: testProjectID, IID: 5, Name: "party_parrot"})
		}},
		{name: "merge request note get", call: func(ctx context.Context) (Output, error) {
			return GetMRNoteAwardEmoji(ctx, client, MRGetOnNoteInput{ProjectID: testProjectID, IID: 5, NoteID: 2, AwardID: 7})
		}},
		{name: "snippet get", call: func(ctx context.Context) (Output, error) {
			return GetSnippetAwardEmoji(ctx, client, SnippetGetInput{ProjectID: testProjectID, IID: 5, AwardID: 7})
		}},
		{name: "snippet create", call: func(ctx context.Context) (Output, error) {
			return CreateSnippetAwardEmoji(ctx, client, SnippetCreateInput{ProjectID: testProjectID, IID: 5, Name: "party_parrot"})
		}},
		{name: "snippet note get", call: func(ctx context.Context) (Output, error) {
			return GetSnippetNoteAwardEmoji(ctx, client, SnippetGetOnNoteInput{ProjectID: testProjectID, IID: 5, NoteID: 2, AwardID: 7})
		}},
	}
	for _, tt := range singles {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tt.call(ctx)
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if out.URL != customAwardURL || out.ID != 7 {
				t.Errorf("award = {ID:%d URL:%q}, want the custom emoji's image", out.ID, out.URL)
			}
		})
	}

	lists := []struct {
		name string
		call func(context.Context) (ListOutput, error)
	}{
		{name: "issue list", call: func(ctx context.Context) (ListOutput, error) {
			return ListIssueAwardEmoji(ctx, client, IssueListInput{ProjectID: testProjectID, IID: 5})
		}},
		{name: "issue note list", call: func(ctx context.Context) (ListOutput, error) {
			return ListIssueNoteAwardEmoji(ctx, client, IssueListOnNoteInput{ProjectID: testProjectID, IID: 5, NoteID: 2})
		}},
		{name: "merge request list", call: func(ctx context.Context) (ListOutput, error) {
			return ListMRAwardEmoji(ctx, client, MRListInput{ProjectID: testProjectID, IID: 5})
		}},
		{name: "snippet list", call: func(ctx context.Context) (ListOutput, error) {
			return ListSnippetAwardEmoji(ctx, client, SnippetListInput{ProjectID: testProjectID, IID: 5})
		}},
	}
	for _, tt := range lists {
		t.Run(tt.name, func(t *testing.T) {
			out, err := tt.call(ctx)
			if err != nil {
				t.Fatalf(fmtUnexpErr, err)
			}
			if len(out.AwardEmoji) != 1 || out.AwardEmoji[0].URL != customAwardURL {
				t.Errorf("awards = %+v, want the one custom emoji with its image", out.AwardEmoji)
			}
		})
	}
}

// TestCreateMRAwardEmoji_ADuplicateCustomEmoji_ReturnsTheExistingAwardsImage
// verifies the one path that answers with an award it did not create: GitLab
// refuses a second award of the same emoji, and the existing one is read off
// the list, image URL included.
func TestCreateMRAwardEmoji_ADuplicateCustomEmoji_ReturnsTheExistingAwardsImage(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v4/user":
			testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"admin"}`)
		case r.Method == http.MethodPost:
			testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Award Emoji Name has already been taken Not Found"}`)
		default:
			testutil.RespondJSON(w, http.StatusOK, `[{"id":6,"name":"eyes","user":{"id":1}},`+customAward+`]`)
		}
	}))

	out, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: testProjectID, IID: 5, Name: "party_parrot"})
	if err != nil {
		t.Fatalf(fmtUnexpErr, err)
	}
	if out.ID != 7 || out.URL != customAwardURL {
		t.Errorf("award = {ID:%d URL:%q}, want the existing custom emoji with its image", out.ID, out.URL)
	}
}

// TestAwardCapture_AnAnswerTheExtrasCannotRead_IsAnError verifies that the
// image URL is never paired with the wrong award: an answer whose url is not a
// string, which client-go never reads, or one holding a different number of
// awards than the SDK decoded, is an error rather than an award with somebody
// else's image. The duplicate path is best-effort, so there it falls back to
// the create's own refusal.
func TestAwardCapture_AnAnswerTheExtrasCannotRead_IsAnError(t *testing.T) {
	const unreadable = `{"id":7,"name":"party_parrot","user":{"id":1},"url":5}`
	t.Run("one award", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, unreadable)
		}))
		if _, err := GetIssueAwardEmoji(t.Context(), client, IssueGetInput{ProjectID: testProjectID, IID: 5, AwardID: 7}); err == nil {
			t.Error("GetIssueAwardEmoji() = nil error, want one")
		}
	})
	t.Run("a list", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			testutil.RespondJSON(w, http.StatusOK, "["+unreadable+"]")
		}))
		if _, err := ListIssueAwardEmoji(t.Context(), client, IssueListInput{ProjectID: testProjectID, IID: 5}); err == nil {
			t.Error("ListIssueAwardEmoji() = nil error, want one")
		}
	})
	t.Run("a count the SDK did not decode", func(t *testing.T) {
		if _, err := capturedAwards(gitlabclient.CapturedBody([]byte(`[{"url":""}]`)), 2); err == nil {
			t.Error("capturedAwards() = nil error, want one")
		}
	})
	t.Run("a duplicate whose existing award cannot be read", func(t *testing.T) {
		client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/api/v4/user":
				testutil.RespondJSON(w, http.StatusOK, `{"id":1,"username":"admin"}`)
			case r.Method == http.MethodPost:
				testutil.RespondJSON(w, http.StatusNotFound, `{"message":"404 Award Emoji Name has already been taken Not Found"}`)
			default:
				testutil.RespondJSON(w, http.StatusOK, "["+unreadable+"]")
			}
		}))
		if _, err := CreateMRAwardEmoji(t.Context(), client, MRCreateInput{ProjectID: testProjectID, IID: 5, Name: "party_parrot"}); err == nil {
			t.Error("CreateMRAwardEmoji() = nil error, want the create's refusal")
		}
	})
}

// TestAwardMarkdown_ACustomEmoji_LinksToItsImage verifies that a custom
// emoji's name links to the image GitLab sent in the table and on the card,
// and that the list then asks for its links to be kept.
func TestAwardMarkdown_ACustomEmoji_LinksToItsImage(t *testing.T) {
	award := Output{ID: 7, Name: "party_parrot", URL: customAwardURL}
	link := "[:party_parrot:](" + customAwardURL + ")"

	if card := FormatMarkdownString(award); !strings.Contains(card, "- **Name**: "+link+"\n") {
		t.Errorf("card = %q, want the name linked to the image", card)
	}
	table := FormatListMarkdownString(ListOutput{AwardEmoji: []Output{award}})
	if !strings.Contains(table, "| 7 | "+link+" |") {
		t.Errorf("table = %q, want the name linked to the image", table)
	}
	if !strings.Contains(table, toolutil.HintPreserveLinks) {
		t.Errorf("table = %q, want the hint that keeps its links", table)
	}
}
