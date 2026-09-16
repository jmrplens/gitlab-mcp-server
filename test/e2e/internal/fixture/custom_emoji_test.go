//go:build e2e

// custom_emoji_test.go drives the custom emoji builder's pure halves against
// the stub's GraphQL route: what a create reads back, how a payload refusal is
// reported, and the one tolerance the removal has, which is a document error
// rather than a status because GraphQL answers a refusal inside a 200.

package fixture

import (
	"testing"
)

// TestCreateCustomEmoji_Created_ReadsTheGlobalIDBack checks that the builder
// hands back the identifier every emoji action takes.
func TestCreateCustomEmoji_Created_ReadsTheGlobalIDBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.configure(func() {
		stub.graphqlAnswers = []string{
			`{"data":{"createCustomEmoji":{"customEmoji":{"id":"gid://gitlab/CustomEmoji/7","name":"e2e_eval_emoji","url":"https://fixture.invalid/emoji.png"},"errors":[]}}}`,
		}
	})

	got, err := createCustomEmoji(t.Context(), client, "group/path", customEmojiName, "https://fixture.invalid/emoji.png")
	if err != nil {
		t.Fatalf("createCustomEmoji() error = %v, want nil", err)
	}
	want := CustomEmoji{ID: "gid://gitlab/CustomEmoji/7", Name: customEmojiName, URL: "https://fixture.invalid/emoji.png"}
	if got != want {
		t.Errorf("createCustomEmoji() = %+v, want %+v", got, want)
	}
}

// TestCreateCustomEmoji_Refusals covers the two ways the mutation can fail
// without failing: a payload carrying errors, and a payload carrying no emoji
// at all, which would otherwise hand a case an empty identifier.
func TestCreateCustomEmoji_Refusals(t *testing.T) {
	cases := []struct {
		name   string
		answer string
	}{
		{
			name:   "payload errors",
			answer: `{"data":{"createCustomEmoji":{"customEmoji":null,"errors":["Name has already been taken"]}}}`,
		},
		{
			name:   "no emoji",
			answer: `{"data":{"createCustomEmoji":{"customEmoji":null,"errors":[]}}}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{tc.answer} })

			if _, err := createCustomEmoji(t.Context(), client, "group/path", customEmojiName, "https://fixture.invalid/emoji.png"); err == nil {
				t.Error("createCustomEmoji() error = nil, want the refusal")
			}
		})
	}
}

// TestDeleteCustomEmoji_Endings_ToleratesOneACaseDeleted checks the tolerance
// that has to read words rather than a status: GraphQL answers a destroy of an
// emoji that is gone with a 200 carrying an error.
func TestDeleteCustomEmoji_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{name: "deleted", answer: `{"data":{"destroyCustomEmoji":{"errors":[]}}}`},
		{name: "already gone", answer: `{"data":{"destroyCustomEmoji":{"errors":["Custom emoji not found"]}}}`},
		{name: "document refused", answer: `{"errors":[{"message":"The resource that you are attempting to access does not exist"}]}`},
		{
			name: "refused", answer: `{"data":{"destroyCustomEmoji":{"errors":["You have insufficient permissions"]}}}`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{tc.answer} })

			err := deleteCustomEmoji(t.Context(), client, "gid://gitlab/CustomEmoji/7")
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteCustomEmoji() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}

// TestCustomEmojiExists_Answers covers the three the read-back distinguishes,
// including a group GitLab answered nothing for.
func TestCustomEmojiExists_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		want    bool
		wantErr bool
	}{
		{
			name:   "there",
			answer: `{"data":{"group":{"customEmoji":{"nodes":[{"id":"gid://gitlab/CustomEmoji/7"}]}}}}`,
			want:   true,
		},
		{name: "gone", answer: `{"data":{"group":{"customEmoji":{"nodes":[]}}}}`},
		{name: "no group", answer: `{"data":{"group":null}}`},
		{name: "refused", answer: `{"errors":[{"message":"forbidden"}]}`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{tc.answer} })

			got, err := CustomEmojiExists(t.Context(), client, "group/path", "gid://gitlab/CustomEmoji/7")
			if (err != nil) != tc.wantErr {
				t.Fatalf("CustomEmojiExists() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("CustomEmojiExists() = %t, want %t", got, tc.want)
			}
		})
	}
}
