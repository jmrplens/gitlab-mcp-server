package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/tiktoken-go/tokenizer"
)

// TestCountTokens_UsesRealTokenizer verifies countTokens runs the cl100k_base
// tokenizer rather than the bytes/4 fallback. A run of identical characters is
// compressed by BPE into far fewer tokens than length/4, so a count equal to
// length/4 would mean the tokenizer never engaged (a regression this guards).
func TestCountTokens_UsesRealTokenizer(t *testing.T) {
	data := []byte(strings.Repeat("a", 64))
	fallback := len(data) / 4 // 16

	got := countTokens(data)
	if got <= 0 {
		t.Fatalf("countTokens = %d, want > 0", got)
	}
	if got >= fallback {
		t.Fatalf("countTokens = %d, want < %d (BPE compresses repeated chars; bytes/4 fallback did not engage)", got, fallback)
	}
}

// TestCountTokens_KnownText pins the token count of a short, stable phrase so a
// tokenizer/vocabulary swap is caught. "hello world" is two cl100k_base tokens.
func TestCountTokens_KnownText(t *testing.T) {
	if got := countTokens([]byte("hello world")); got != 2 {
		t.Fatalf(`countTokens("hello world") = %d, want 2 (cl100k_base)`, got)
	}
}

// TestCountTokens_Empty returns zero for empty input under both the tokenizer
// and the fallback.
func TestCountTokens_Empty(t *testing.T) {
	if got := countTokens(nil); got != 0 {
		t.Fatalf("countTokens(nil) = %d, want 0", got)
	}
}

// stubCodec is a [tokenizer.Codec] whose Encode either fails or returns a
// fixed number of ids, so the fallback branch of countTokens can be driven
// without a corrupt vocabulary.
type stubCodec struct {
	ids    []uint
	encErr error
}

func (c stubCodec) GetName() string { return "stub" }

func (c stubCodec) Count(string) (int, error) { return len(c.ids), c.encErr }

func (c stubCodec) Encode(string) ([]uint, []string, error) {
	if c.encErr != nil {
		return nil, nil, c.encErr
	}
	return c.ids, nil, nil
}

func (c stubCodec) Decode([]uint) (string, error) { return "", c.encErr }

// TestCountTokens_TokenizerUnavailable_UsesByteHeuristic verifies the two ways
// the tokenizer can drop out — an unavailable codec and an encode failure —
// both fall back to the documented bytes/4 approximation rather than
// reporting zero tokens, which would understate every published figure.
func TestCountTokens_TokenizerUnavailable_UsesByteHeuristic(t *testing.T) {
	data := []byte(strings.Repeat("a", 64))
	tests := []struct {
		name  string
		codec func() tokenizer.Codec
		want  int
	}{
		{name: "no codec falls back to bytes/4", codec: func() tokenizer.Codec { return nil }, want: len(data) / 4},
		{
			name:  "encode failure falls back to bytes/4",
			codec: func() tokenizer.Codec { return stubCodec{encErr: errors.New("vocabulary corrupt")} },
			want:  len(data) / 4,
		},
		{
			name:  "a working codec reports its own ids",
			codec: func() tokenizer.Codec { return stubCodec{ids: []uint{1, 2, 3}} },
			want:  3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := activeCodec
			activeCodec = tt.codec
			t.Cleanup(func() { activeCodec = original })

			if got := countTokens(data); got != tt.want {
				t.Fatalf("countTokens() = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestTokenCodec_RepeatedCalls_ReturnTheSameCodec verifies the lazy
// initializer memoizes: every measurement in one run must be made by one
// tokenizer, or figures taken at different moments would not be comparable.
func TestTokenCodec_RepeatedCalls_ReturnTheSameCodec(t *testing.T) {
	first := tokenCodec()
	if first == nil {
		t.Fatal("tokenCodec() = nil, want the cl100k_base codec")
	}
	if second := tokenCodec(); second != first {
		t.Fatalf("tokenCodec() returned a different codec on the second call: %v vs %v", second, first)
	}
	if name := first.GetName(); name != "cl100k_base" {
		t.Fatalf("tokenCodec().GetName() = %q, want cl100k_base", name)
	}
}
