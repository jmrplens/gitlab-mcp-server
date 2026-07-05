package toolutil

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestNewCustomAttributeOutputs pins the custom-attribute conversion:
// nil/empty and all-nil inputs return nil, nil elements are skipped, and
// populated entries mirror key and value.
func TestNewCustomAttributeOutputs(t *testing.T) {
	if got := NewCustomAttributeOutputs(nil); got != nil {
		t.Fatalf("NewCustomAttributeOutputs(nil) = %+v, want nil", got)
	}
	if got := NewCustomAttributeOutputs([]*gl.CustomAttribute{nil, nil}); got != nil {
		t.Fatalf("NewCustomAttributeOutputs(all nil) = %+v, want nil", got)
	}

	got := NewCustomAttributeOutputs([]*gl.CustomAttribute{
		{Key: "department", Value: "platform"},
		nil,
		{Key: "location", Value: "remote"},
	})
	if len(got) != 2 || got[0].Key != "department" || got[1].Value != "remote" {
		t.Errorf("NewCustomAttributeOutputs = %+v, want 2 mirrored entries", got)
	}
}

// TestNewUserRefOutput pins the user-resource reference conversion:
// nil-on-nil, full field mirror, and RFC3339 created_at only when present.
func TestNewUserRefOutput(t *testing.T) {
	if got := NewUserRefOutput(nil); got != nil {
		t.Fatalf("NewUserRefOutput(nil) = %+v, want nil", got)
	}

	created := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	got := NewUserRefOutput(&gl.BasicUser{
		ID: 8, Username: "jdoe", Name: "John Doe", State: "active",
		AvatarURL: "https://example.com/a.png", WebURL: "https://example.com/jdoe",
		CreatedAt: &created,
	})
	if got == nil || got.ID != 8 || got.Username != "jdoe" || got.CreatedAt != created.Format(time.RFC3339) {
		t.Errorf("NewUserRefOutput = %+v, want full mirror with RFC3339 created_at", got)
	}

	noDate := NewUserRefOutput(&gl.BasicUser{ID: 9, Username: "x"})
	if noDate == nil || noDate.CreatedAt != "" {
		t.Errorf("NewUserRefOutput without CreatedAt = %+v, want empty created_at", noDate)
	}
}

// TestResolveProjectWebURLs pins the URL resolution helper: zero IDs are
// skipped, duplicate IDs are fetched once, lookup failures yield an empty
// URL, and successful lookups map to the project's web_url.
func TestResolveProjectWebURLs(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.Contains(r.URL.Path, "/projects/10") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":10,"web_url":"https://gitlab.example.com/group/project"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := gl.NewClient("test-token", gl.WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("gl.NewClient: %v", err)
	}

	urls := ResolveProjectWebURLs(context.Background(), client.Projects, []int64{0, 10, 10, 404})
	if got := urls[10]; got != "https://gitlab.example.com/group/project" {
		t.Errorf("urls[10] = %q, want the project web_url", got)
	}
	if got, ok := urls[404]; !ok || got != "" {
		t.Errorf("urls[404] = %q (present=%v), want empty entry for failed lookup", got, ok)
	}
	if _, ok := urls[0]; ok {
		t.Error("urls[0] present, want zero IDs skipped")
	}
	if calls != 2 {
		t.Errorf("API calls = %d, want 2 (duplicate ID fetched once)", calls)
	}
}
