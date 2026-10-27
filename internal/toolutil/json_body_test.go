// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
)

// bodyRequest builds the request an option is applied to, carrying body as
// client-go would have encoded it.
func bodyRequest(t *testing.T, body string) *retryablehttp.Request {
	t.Helper()
	req, err := retryablehttp.NewRequest(http.MethodPut, "https://gitlab.example.com/api/v4/x", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	return req
}

// sentBody reads back the body a request will send.
func sentBody(t *testing.T, req *retryablehttp.Request) string {
	t.Helper()
	raw, err := req.BodyBytes()
	if err != nil {
		t.Fatalf("BodyBytes() error: %v", err)
	}
	return string(raw)
}

// TestNullWhenZero_SendsNullOnlyForTheZeroValue holds the three answers an
// update field GitLab clears only with null takes: a field left out edits
// nothing and returns nil, a zero value records the key as null and returns
// nil so the option struct leaves it to the edit, and any other value comes
// back as given and edits nothing. It runs for a text and an ID reference, the
// two kinds the handlers read through it, and with an edit that already holds
// a key, which a second field must not replace.
func TestNullWhenZero_SendsNullOnlyForTheZeroValue(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		var edit BodyEdit
		if got := NullWhenZero[string](nil, "filepath", &edit); got != nil || edit.Set != nil {
			t.Errorf("left out: got %v, Set %v, want nil and no edit", got, edit.Set)
		}
		path := "/bin/app"
		if got := NullWhenZero(&path, "filepath", &edit); got != &path || edit.Set != nil {
			t.Errorf("a path: got %v, Set %v, want the same pointer and no edit", got, edit.Set)
		}
		if got := NullWhenZero(new(""), "filepath", &edit); got != nil {
			t.Errorf("empty: got %v, want nil", got)
		}
		if value, ok := edit.Set["filepath"]; !ok || value != nil {
			t.Errorf("empty: Set = %v, want filepath set to null", edit.Set)
		}
	})
	t.Run("ID reference", func(t *testing.T) {
		edit := BodyEdit{Set: map[string]any{"other": "kept"}}
		if got := NullWhenZero(new(int64(0)), "member_role_id", &edit); got != nil {
			t.Errorf("zero: got %v, want nil", got)
		}
		if value, ok := edit.Set["member_role_id"]; !ok || value != nil || edit.Set["other"] != "kept" {
			t.Errorf("zero: Set = %v, want member_role_id null beside the key already there", edit.Set)
		}
		if got := NullWhenZero(new(int64(7)), "member_role_id", &edit); got == nil || *got != 7 {
			t.Errorf("a role: got %v, want 7", got)
		}
	})
}

// TestEditJSONBody_SetsAndDropsKeys holds the option to its contract on the
// body client-go encoded: a key in Set is written with its value, nil writing
// null, a key in Drop is removed, and every other key is left as it was, a
// number included, which a decode through float64 would have rewritten.
func TestEditJSONBody_SetsAndDropsKeys(t *testing.T) {
	tests := []struct {
		name string
		body string
		edit BodyEdit
		want string
	}{
		{"drop a key", `{"name":null,"released_at":"2026-01-01T00:00:00Z"}`, BodyEdit{Drop: []string{"name"}}, `{"released_at":"2026-01-01T00:00:00Z"}`},
		{"drop a key that is absent", `{"a":1}`, BodyEdit{Drop: []string{"b"}}, `{"a":1}`},
		{"write null", `{"a":1}`, BodyEdit{Set: map[string]any{"member_role_id": nil}}, `{"a":1,"member_role_id":null}`},
		{"replace a value", `{"levels":[30]}`, BodyEdit{Set: map[string]any{"levels": []int{}}}, `{"levels":[]}`},
		{"a large number is kept as written", `{"id":12345678901234567890}`, BodyEdit{Drop: []string{"x"}}, `{"id":12345678901234567890}`},
		{"an empty body becomes an object", ``, BodyEdit{Set: map[string]any{"a": nil}}, `{"a":null}`},
		{"set and drop together", `{"a":1,"b":2}`, BodyEdit{Set: map[string]any{"c": "x"}, Drop: []string{"a"}}, `{"b":2,"c":"x"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := bodyRequest(t, tt.body)
			if err := EditJSONBody(tt.edit)(req); err != nil {
				t.Fatalf("option error: %v", err)
			}
			if got := sentBody(t, req); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestEditJSONBody_NothingToEdit_ReturnsNoOption holds an empty edit to no
// option at all, so a request that needs no rewrite goes out exactly as
// client-go built it.
func TestEditJSONBody_NothingToEdit_ReturnsNoOption(t *testing.T) {
	if EditJSONBody(BodyEdit{}) != nil {
		t.Error("EditJSONBody(BodyEdit{}) returned an option, want nil")
	}
	if EditJSONBody(BodyEdit{Set: map[string]any{}, Drop: []string{}}) != nil {
		t.Error("EditJSONBody with empty Set and Drop returned an option, want nil")
	}
}

// TestEditJSONBody_UnreadableBody_FailsTheRequest holds a body that is not a
// JSON object, and one that cannot be read, to an error, so the request is not
// sent without the edit it was asked to carry.
func TestEditJSONBody_UnreadableBody_FailsTheRequest(t *testing.T) {
	t.Run("not an object", func(t *testing.T) {
		err := EditJSONBody(BodyEdit{Drop: []string{"a"}})(bodyRequest(t, `[1,2]`))
		if err == nil || !strings.Contains(err.Error(), "decode the request body") {
			t.Errorf("error = %v, want a decode refusal", err)
		}
	})
	t.Run("unreadable", func(t *testing.T) {
		// retryablehttp calls the reader once when the request is built, to
		// size it, so the reader fails from its second call on.
		calls := 0
		req, err := retryablehttp.NewRequest(http.MethodPut, "https://gitlab.example.com/api/v4/x",
			retryablehttp.ReaderFunc(func() (io.Reader, error) {
				calls++
				if calls > 1 {
					return nil, io.ErrUnexpectedEOF
				}
				return strings.NewReader("{}"), nil
			}))
		if err != nil {
			t.Fatalf("NewRequest() error: %v", err)
		}
		err = EditJSONBody(BodyEdit{Drop: []string{"a"}})(req)
		if err == nil || !strings.Contains(err.Error(), "read the request body") {
			t.Errorf("error = %v, want a read refusal", err)
		}
	})
}

// TestEditJSONBody_KeepsTheBodyValidJSON round-trips a rewritten body through
// encoding/json, the decoder GitLab's Grape stands in for here.
func TestEditJSONBody_KeepsTheBodyValidJSON(t *testing.T) {
	req := bodyRequest(t, `{"a":{"b":[1,2,{"c":null}]}}`)
	if err := EditJSONBody(BodyEdit{Set: map[string]any{"d": true}})(req); err != nil {
		t.Fatalf("option error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(sentBody(t, req)), &decoded); err != nil {
		t.Fatalf("rewritten body is not JSON: %v", err)
	}
	if decoded["d"] != true || decoded["a"] == nil {
		t.Errorf("decoded = %v, want the nested object kept and d set", decoded)
	}
}
