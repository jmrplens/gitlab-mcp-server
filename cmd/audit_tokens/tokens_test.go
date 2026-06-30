package main

import (
	"strings"
	"testing"
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
