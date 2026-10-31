// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// declarations_test.go covers how a record knows a top-level declaration of a
// measured file: the key each kind of declaration is named by, a repeated key
// numbered, where its text starts and ends, the hash of that text with the
// doc comment left out, the declaration that holds a block, and a file that
// does not parse.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// declSource is a file holding one declaration of every kind a key is built
// for, a doc comment above the first function, and init declared twice.
const declSource = `package fixture

import "strings"

// A is documented.
func A(y bool) {
	if y {
		_ = strings.ToUpper("a")
	}
}

func (*T) M() {}

func (t T) N() {}

func (g *G[K]) P() {}

type T struct{}

type G[K any] struct{}

var b = func() int { return 3 }

var c, d = 1, 2

const (
	e = 1
	f = 2
)

func init() {}

func init() {}
`

// sum is the hash a declaration's text is recorded by, computed here apart
// from the code under test.
func sum(text string) string {
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:])
}

// TestDeclarationsOf_EveryKind_KeyedInSourceOrder holds each top-level
// declaration to its key, in the order the file declares them: a function by
// its name, a method by its receiver type and name, a type, a var or a const
// group by every name it declares, an import by its path, and a key the file
// repeats numbered from its second occurrence.
func TestDeclarationsOf_EveryKind_KeyedInSourceOrder(t *testing.T) {
	decls, err := declarationsOf("a.go", []byte(declSource))
	if err != nil {
		t.Fatalf("declarationsOf() error = %v", err)
	}

	want := []string{
		`import "strings"`, "func A", "func (*T) M", "func (T) N", "func (*G[K]) P",
		"type T", "type G", "var b", "var c, d", "const e, f", "func init", "func init #2",
	}
	got := make([]string, 0, len(decls))
	for _, d := range decls {
		got = append(got, d.key)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("keys =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestDeclarationsOf_FirstFunction_SpansItsTextWithoutTheDocComment pins the
// documented function: it starts at its func keyword, the line below its doc
// comment, ends just past its closing brace, and is hashed over exactly that
// text.
func TestDeclarationsOf_FirstFunction_SpansItsTextWithoutTheDocComment(t *testing.T) {
	decls, err := declarationsOf("a.go", []byte(declSource))
	if err != nil {
		t.Fatalf("declarationsOf() error = %v", err)
	}

	want := declaration{
		key:   "func A",
		start: position{line: 6, col: 1},
		end:   position{line: 10, col: 2},
		sum:   sum("func A(y bool) {\n\tif y {\n\t\t_ = strings.ToUpper(\"a\")\n\t}\n}"),
	}
	if decls[1] != want {
		t.Errorf("declaration = %+v, want %+v", decls[1], want)
	}
}

// TestDeclarationsOf_Edits_MoveTheHashOnlyWhenTheTextChanges holds the hash to
// the declaration's own text: rewording its doc comment or moving it down a
// line leaves it where it was, and an edit to its body moves it.
func TestDeclarationsOf_Edits_MoveTheHashOnlyWhenTheTextChanges(t *testing.T) {
	original := hashOfA(t, declSource)
	tests := []struct {
		name, edited string
		moves        bool
	}{
		{name: "the doc comment reworded", edited: strings.Replace(declSource, "// A is documented.", "// A is reworded.", 1)},
		{name: "a line above it", edited: strings.Replace(declSource, "import \"strings\"\n", "import \"strings\"\n\n", 1)},
		{name: "its body", edited: strings.Replace(declSource, `ToUpper("a")`, `ToUpper("b")`, 1), moves: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hashOfA(t, tc.edited) != original; got != tc.moves {
				t.Errorf("hash moved = %v, want %v", got, tc.moves)
			}
		})
	}
}

// hashOfA returns the hash of func A in a version of declSource.
func hashOfA(t *testing.T, src string) string {
	t.Helper()

	decls, err := declarationsOf("a.go", []byte(src))
	if err != nil {
		t.Fatalf("declarationsOf() error = %v", err)
	}
	for _, d := range decls {
		if d.key == "func A" {
			return d.sum
		}
	}
	t.Fatal("no func A")
	return ""
}

// TestDeclarationsOf_FileThatDoesNotParse_IsRefusedNamingIt holds a file the
// parser rejects to an error that names it, rather than to the declarations
// it managed to read.
func TestDeclarationsOf_FileThatDoesNotParse_IsRefusedNamingIt(t *testing.T) {
	_, err := declarationsOf("test/e2e/internal/fixture/broken.go", []byte("package fixture\n\nfunc {\n"))

	if err == nil || !strings.Contains(err.Error(), "test/e2e/internal/fixture/broken.go:3:") {
		t.Errorf("declarationsOf() error = %v, want the parser's error naming the file", err)
	}
}

// TestHolding_Positions holds the lookup to the declaration whose text holds
// a block's start: its first position does, a position inside its body does,
// a block of a function literal in a var does, and the position just past its
// end, the package clause and the gap between two declarations do not.
func TestHolding_Positions(t *testing.T) {
	decls, err := declarationsOf("a.go", []byte(declSource))
	if err != nil {
		t.Fatalf("declarationsOf() error = %v", err)
	}
	tests := []struct {
		name string
		at   position
		want string
	}{
		{name: "its first position", at: position{line: 6, col: 1}, want: "func A"},
		{name: "inside its body", at: position{line: 7, col: 9}, want: "func A"},
		{name: "its last position", at: position{line: 10, col: 1}, want: "func A"},
		{name: "a function literal in a var", at: position{line: 22, col: 20}, want: "var b"},
		{name: "just past its end", at: position{line: 10, col: 2}},
		{name: "the package clause", at: position{line: 1, col: 1}},
		{name: "between two declarations", at: position{line: 11, col: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d, ok := holding(decls, tc.at)
			if ok != (tc.want != "") || d.key != tc.want {
				t.Errorf("holding(%v) = %q, %v; want %q", tc.at, d.key, ok, tc.want)
			}
		})
	}
}
