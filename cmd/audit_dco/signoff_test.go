// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// signoff_test.go covers how a commit is judged: reading a trailer as an
// identity, comparing two identities, and the outcome each kind of commit
// reaches.
package main

import "testing"

// jane and john are the people the judging tests commit as.
var (
	jane = identity{name: "Jane Doe", email: "jane@example.org"}
	john = identity{name: "John Roe", email: "john@example.org"}
)

// dependabot is the identity Dependabot's commits carry, as declared.
var dependabot = identity{name: "dependabot[bot]", email: "49699333+dependabot[bot]@users.noreply.github.com"}

// TestIdentity_String_WritesTheNameAndTheEmailInAngleBrackets verifies the
// form an identity is printed in, which is the one a trailer is written in.
func TestIdentity_String_WritesTheNameAndTheEmailInAngleBrackets(t *testing.T) {
	if got, want := jane.String(), "Jane Doe <jane@example.org>"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// TestParseIdentity_Values_ReadANameAndAnEmailOrNobody verifies that a
// trailer value is an identity only when it is a name followed by an address
// in angle brackets, with the surrounding space trimmed, and that every other
// shape names nobody.
func TestParseIdentity_Values_ReadANameAndAnEmailOrNobody(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  identity
		ok    bool
	}{
		{name: "name and email", value: "Jane Doe <jane@example.org>", want: jane, ok: true},
		{name: "surrounding space trimmed", value: "  Jane Doe   <jane@example.org>  ", want: jane, ok: true},
		{name: "space inside the brackets trimmed", value: "Jane Doe < jane@example.org >", want: jane, ok: true},
		{name: "non-ASCII name", value: "José M. Requena Plens <mail@jmrp.io>", want: identity{name: "José M. Requena Plens", email: "mail@jmrp.io"}, ok: true},
		{name: "name alone", value: "Jane Doe"},
		{name: "empty", value: ""},
		{name: "email alone", value: "<jane@example.org>"},
		{name: "blank name", value: "   <jane@example.org>"},
		{name: "empty brackets", value: "Jane Doe <>"},
		{name: "unclosed bracket", value: "Jane Doe <jane@example.org"},
		{name: "text after the bracket", value: "Jane Doe <jane@example.org> (work)"},
		{name: "no at sign", value: "Jane Doe <jane.example.org>"},
		{name: "space in the address", value: "Jane Doe <jane doe@example.org>"},
		{name: "tab in the address", value: "Jane Doe <jane\tdoe@example.org>"},
		{name: "two addresses", value: "Jane Doe <jane@example.org> <john@example.org>"},
		{name: "closing bracket in the name", value: "Jane > Doe <jane@example.org>"},
		{name: "bracket inside the address", value: "Jane Doe <jane<x>@example.org>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseIdentity(tc.value)
			if ok != tc.ok || got != tc.want {
				t.Errorf("parseIdentity(%q) = %#v, %v; want %#v, %v", tc.value, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestSameIdentity_Pairs_MatchTheNameAndTheEmailWithoutRegardToCase verifies
// that two identities are the same when both the name and the email agree,
// letters compared without regard to case, and differ when either does not.
func TestSameIdentity_Pairs_MatchTheNameAndTheEmailWithoutRegardToCase(t *testing.T) {
	tests := []struct {
		name string
		a, b identity
		want bool
	}{
		{name: "identical", a: jane, b: jane, want: true},
		{name: "case differs", a: jane, b: identity{name: "JANE DOE", email: "Jane@Example.ORG"}, want: true},
		{name: "case differs outside ASCII", a: identity{name: "José", email: "j@x.org"}, b: identity{name: "JOSÉ", email: "j@x.org"}, want: true},
		{name: "name differs", a: jane, b: identity{name: "Jane Roe", email: jane.email}},
		{name: "email differs", a: jane, b: identity{name: jane.name, email: "jane@example.com"}},
		{name: "both differ", a: jane, b: john},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameIdentity(tc.a, tc.b); got != tc.want {
				t.Errorf("sameIdentity(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestCommit_Short_AbbreviatesTheHashToTwelveCharacters verifies the
// abbreviation the report prints, and that a hash shorter than that is
// printed whole rather than cut.
func TestCommit_Short_AbbreviatesTheHashToTwelveCharacters(t *testing.T) {
	tests := []struct{ hash, want string }{
		{hash: "0123456789abcdef0123456789abcdef01234567", want: "0123456789ab"},
		{hash: "0123456789ab", want: "0123456789ab"},
		{hash: "0123", want: "0123"},
	}
	for _, tc := range tests {
		t.Run(tc.hash, func(t *testing.T) {
			if got := (commit{hash: tc.hash}).short(); got != tc.want {
				t.Errorf("short() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestJudge_Commits_ReachTheOutcomeTheirSignOffsDecide verifies each outcome:
// a commit passes when a trailer names its author and fails when it has none
// or names only others; a merge that brings no change of its own is not
// judged, and any other merge is judged like any commit; a declared bot's
// commit is exempt only in a pull request that bot opened; and a failing
// commit that carries a bot's identity says which bot.
func TestJudge_Commits_ReachTheOutcomeTheirSignOffsDecide(t *testing.T) {
	tests := []struct {
		name     string
		commit   commit
		openedBy string
		want     outcome
		wantBot  bool
	}{
		{
			name:   "signed off by its author",
			commit: commit{parents: 1, author: jane, signoffs: []string{"Jane Doe <jane@example.org>"}},
			want:   signedOff,
		},
		{
			name:   "signed off by its author in another case",
			commit: commit{parents: 1, author: jane, signoffs: []string{"jane doe <JANE@example.org>"}},
			want:   signedOff,
		},
		{
			name:   "its author's sign-off after somebody else's",
			commit: commit{parents: 1, author: jane, signoffs: []string{"John Roe <john@example.org>", "Jane Doe <jane@example.org>"}},
			want:   signedOff,
		},
		{
			name:   "root commit signed off",
			commit: commit{parents: 0, author: jane, signoffs: []string{"Jane Doe <jane@example.org>"}},
			want:   signedOff,
		},
		{
			name:   "no trailer",
			commit: commit{parents: 1, author: jane},
			want:   unsigned,
		},
		{
			name:   "signed off by somebody else",
			commit: commit{parents: 1, author: jane, signoffs: []string{"John Roe <john@example.org>"}},
			want:   signedByOthers,
		},
		{
			name:   "author's name with another email",
			commit: commit{parents: 1, author: jane, signoffs: []string{"Jane Doe <jane@example.com>"}},
			want:   signedByOthers,
		},
		{
			name:   "trailer that names nobody",
			commit: commit{parents: 1, author: jane, signoffs: []string{"Jane Doe"}},
			want:   signedByOthers,
		},
		{
			name:   "merge that brings no change, without a trailer",
			commit: commit{parents: 2, author: jane, cleanMerge: true},
			want:   merge,
		},
		{
			name:   "merge with a change of its own, without a trailer",
			commit: commit{parents: 2, author: jane},
			want:   unsigned,
		},
		{
			name:   "merge with a change of its own, signed off by its author",
			commit: commit{parents: 2, author: jane, signoffs: []string{"Jane Doe <jane@example.org>"}},
			want:   signedOff,
		},
		{
			name:   "octopus merge without a trailer",
			commit: commit{parents: 3, author: jane},
			want:   unsigned,
		},
		{
			name:     "Dependabot's merge that brings no change, in a pull request a person opened",
			commit:   commit{parents: 2, author: dependabot, cleanMerge: true},
			openedBy: "jmrplens",
			want:     merge,
		},
		{
			name:     "Dependabot in a pull request Dependabot opened",
			commit:   commit{parents: 1, author: dependabot, signoffs: []string{"dependabot[bot] <support@github.com>"}},
			openedBy: "dependabot[bot]",
			want:     exempt,
			wantBot:  true,
		},
		{
			name:     "Dependabot's identity in a pull request a person opened",
			commit:   commit{parents: 1, author: dependabot},
			openedBy: "jmrplens",
			want:     unsigned,
			wantBot:  true,
		},
		{
			name:     "Dependabot's own sign-off in a pull request a person opened",
			commit:   commit{parents: 1, author: dependabot, signoffs: []string{"dependabot[bot] <support@github.com>"}},
			openedBy: "jmrplens",
			want:     signedByOthers,
			wantBot:  true,
		},
		{
			name:     "a person's commit in a pull request Dependabot opened",
			commit:   commit{parents: 1, author: jane},
			openedBy: "dependabot[bot]",
			want:     unsigned,
		},
		{
			name:     "a person's signed commit in a pull request Dependabot opened",
			commit:   commit{parents: 1, author: jane, signoffs: []string{"Jane Doe <jane@example.org>"}},
			openedBy: "dependabot[bot]",
			want:     signedOff,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := judge(tc.commit, tc.openedBy)
			if got.outcome != tc.want {
				t.Errorf("judge(%+v, %q).outcome = %d, want %d", tc.commit, tc.openedBy, got.outcome, tc.want)
			}
			if (got.bot != nil) != tc.wantBot {
				t.Errorf("judge(%+v, %q).bot = %v, want a bot: %v", tc.commit, tc.openedBy, got.bot, tc.wantBot)
			}
			if got.bot != nil && got.bot.login != "dependabot[bot]" {
				t.Errorf("judge(%+v, %q).bot = %q, want dependabot[bot]", tc.commit, tc.openedBy, got.bot.login)
			}
			if got.commit.author != tc.commit.author || got.commit.parents != tc.commit.parents {
				t.Errorf("judge(%+v, %q).commit = %+v, want the commit judged", tc.commit, tc.openedBy, got.commit)
			}
		})
	}
}

// TestVerdict_Failed_IsTrueForTheTwoOutcomesThatFailARun verifies that only a
// commit without its author's sign-off fails the run.
func TestVerdict_Failed_IsTrueForTheTwoOutcomesThatFailARun(t *testing.T) {
	tests := []struct {
		name    string
		outcome outcome
		want    bool
	}{
		{name: "signed off", outcome: signedOff},
		{name: "unsigned", outcome: unsigned, want: true},
		{name: "signed by others", outcome: signedByOthers, want: true},
		{name: "exempt", outcome: exempt},
		{name: "merge", outcome: merge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (verdict{outcome: tc.outcome}).failed(); got != tc.want {
				t.Errorf("failed() = %v, want %v", got, tc.want)
			}
		})
	}
}
