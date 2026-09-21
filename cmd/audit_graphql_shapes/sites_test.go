package main

import (
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadProgram_TreeThatCannotBeWalked_IsRefused verifies the two refusals
// the loader has beside a directory that is not a module: a package that does
// not type-check, which would fold no constant and type no call, and a pattern
// that matches nothing, which is the shape a mistyped pattern produces and
// must not become a clean run over nothing.
func TestLoadProgram_TreeThatCannotBeWalked_IsRefused(t *testing.T) {
	root := fixtureModule(t, map[string]string{"bad": "package bad\n\nfunc use() { undefinedHelper() }\n"})
	if err := os.Mkdir(filepath.Join(root, "empty"), 0o750); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	cases := []struct {
		name     string
		patterns []string
		want     string
	}{
		{name: "a package that does not type-check", patterns: []string{"./bad/..."}, want: "load fixture/bad: "},
		{name: "a pattern matching nothing", patterns: []string{"./empty/..."}, want: "no packages matched ./empty/..."},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			loaded, err := loadProgram(root, testCase.patterns)

			if err == nil {
				t.Fatalf("loadProgram() error = nil, want one naming %q", testCase.want)
			}
			if loaded != nil {
				t.Errorf("loadProgram() = %v, want nil on failure", loaded)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("loadProgram() error = %q, want it to name %q", err, testCase.want)
			}
		})
	}
}

// TestPairingLess_OrdersByTheCallThenByWhereTheDocumentCameFrom verifies the
// order the report is read in, at each of the four levels that decide it and
// in both directions.
//
// The order matters because a pairing is collected in whatever order the walk
// reaches it — a document handed over through a wrapper is completed after
// every direct send, so the slice arrives unsorted — and two runs over one
// tree have to print the same list. Each level is checked both ways round,
// since a comparator that answers one direction correctly and the other by
// accident sorts correctly only on input that was already sorted.
func TestPairingLess_OrdersByTheCallThenByWhereTheDocumentCameFrom(t *testing.T) {
	at := func(file string, line, column int) token.Position {
		return token.Position{Filename: file, Line: line, Column: column}
	}
	first := at("a.go", 10, 4)
	for _, testCase := range []struct {
		name  string
		left  pairing
		right pairing
		want  bool
	}{
		{
			name:  "an earlier file comes first",
			left:  pairing{Position: at("a.go", 99, 9)},
			right: pairing{Position: at("b.go", 1, 1)},
			want:  true,
		},
		{
			name:  "a later file does not",
			left:  pairing{Position: at("b.go", 1, 1)},
			right: pairing{Position: at("a.go", 99, 9)},
			want:  false,
		},
		{
			name:  "within one file an earlier line comes first",
			left:  pairing{Position: at("a.go", 3, 40)},
			right: pairing{Position: at("a.go", 9, 1)},
			want:  true,
		},
		{
			name:  "within one file a later line does not",
			left:  pairing{Position: at("a.go", 9, 1)},
			right: pairing{Position: at("a.go", 3, 40)},
			want:  false,
		},
		{
			name:  "two sends on one line are ordered by column",
			left:  pairing{Position: at("a.go", 9, 6)},
			right: pairing{Position: at("a.go", 9, 40)},
			want:  true,
		},
		{
			name:  "and the later column does not come first",
			left:  pairing{Position: at("a.go", 9, 40)},
			right: pairing{Position: at("a.go", 9, 6)},
			want:  false,
		},
		{
			name:  "one call reached from two hand-overs is ordered by where the document came from",
			left:  pairing{Position: first, Origin: at("caller.go", 2, 2)},
			right: pairing{Position: first, Origin: at("caller.go", 7, 2)},
			want:  true,
		},
		{
			name:  "and the later hand-over does not come first",
			left:  pairing{Position: first, Origin: at("caller.go", 7, 2)},
			right: pairing{Position: first, Origin: at("caller.go", 2, 2)},
			want:  false,
		},
		{
			name:  "a pairing is not before itself",
			left:  pairing{Position: first},
			right: pairing{Position: first},
			want:  false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if got := pairingLess(testCase.left, testCase.right); got != testCase.want {
				t.Errorf("pairingLess(%v@%v, %v@%v) = %t, want %t",
					testCase.left.Position, testCase.left.Origin,
					testCase.right.Position, testCase.right.Origin, got, testCase.want)
			}
		})
	}
}
