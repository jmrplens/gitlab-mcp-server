package finegrained

import (
	"errors"
	"strings"
	"testing"
)

// TestAuthority_UnconfirmedWrite_LeavesAClassicSessionItsOwnError verifies
// that a session with no authority, which is every credential that is not a
// fine-grained token, is answered with the handler's own error unchanged: a
// classic token is never told that its write was probably committed, since
// nothing GitLab does to a classic token's answer gives a reason to think so.
func TestAuthority_UnconfirmedWrite_LeavesAClassicSessionItsOwnError(t *testing.T) {
	var authority *Authority
	classic := errors.New("create_custom_emoji: no emoji returned")

	got := authority.UnconfirmedWrite("create_custom_emoji", "custom emoji", classic)

	if !errors.Is(got, classic) || got.Error() != classic.Error() {
		t.Fatalf("UnconfirmedWrite on a classic session = %v, want the handler's own error %v unchanged", got, classic)
	}
	if errors.Is(got, ErrUnconfirmedWrite) {
		t.Errorf("a classic session's error reads as an unconfirmed write: %v", got)
	}
}

// TestAuthority_UnconfirmedWrite_TellsAFineGrainedSessionTheWriteProbablyCommitted
// verifies the words a fine-grained session gets when GitLab ran a write and
// answered without the object it returns: the operation and the object, why
// GitLab answers that way, that the write was probably committed, and what to
// do before repeating it. It also holds the sentence to the rules every served
// sentence here is held to, and to the one this error adds: it never reads as
// "not found", since FineGrainedErrorNote appends the null note to a
// not-found error, and it does not unwrap to the handler's error, which for a
// client-go write is the not-found sentinel every 404 matches. It names no
// GitLab version, since the order of the two checks it describes is not a
// state of the recorded declarations.
func TestAuthority_UnconfirmedWrite_TellsAFineGrainedSessionTheWriteProbablyCommitted(t *testing.T) {
	authority := Unevaluated(testTable(), FallbackNone, "")
	classic := errors.New("create_custom_emoji: no emoji returned")

	got := authority.UnconfirmedWrite("create_custom_emoji", "custom emoji", classic)

	if !errors.Is(got, ErrUnconfirmedWrite) {
		t.Fatalf("UnconfirmedWrite = %v, want it to be ErrUnconfirmedWrite", got)
	}
	if errors.Is(got, classic) {
		t.Errorf("UnconfirmedWrite unwraps to the handler's error %v", classic)
	}
	text := got.Error()
	const want = "create_custom_emoji: GitLab answered without the custom emoji this write returns. " +
		"GitLab checks that object against a fine-grained personal access token only after the write has run, " +
		"and answers null for one whose GraphQL type declares no fine-grained permission or needs one this token " +
		"was not granted, so the write was probably committed. Before repeating it, check in GitLab or with a " +
		"classic token whether it took effect: repeating a create makes a second one."
	if text != want {
		t.Errorf("UnconfirmedWrite =\n%s\nwant\n%s", text, want)
	}
	for _, forbidden := range []string{"not found", "gitlab_", ";", "custom_emoji.", testTable().DisplayVersion()} {
		t.Run("without "+forbidden, func(t *testing.T) {
			if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
				t.Errorf("UnconfirmedWrite contains %q: %s", forbidden, text)
			}
		})
	}
	for _, r := range text {
		if r > 0x7e {
			t.Errorf("UnconfirmedWrite carries the non-ASCII rune %q: %s", r, text)
		}
	}
}

// TestUnconfirmedWrite_Is_MatchesOnlyItsSentinel verifies that the error a
// fine-grained session gets is the sentinel and nothing else, so a caller
// matching another error, a not-found among them, does not take it for one.
func TestUnconfirmedWrite_Is_MatchesOnlyItsSentinel(t *testing.T) {
	got := Unevaluated(testTable(), FallbackNone, "").UnconfirmedWrite("op", "object", errors.New("op: no object"))
	if !errors.Is(got, ErrUnconfirmedWrite) {
		t.Errorf("errors.Is(%v, ErrUnconfirmedWrite) = false, want true", got)
	}
	if other := errors.New("another"); errors.Is(got, other) {
		t.Errorf("errors.Is(%v, %v) = true, want false", got, other)
	}
}
