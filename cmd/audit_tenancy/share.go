package main

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// checkShareWords is G13: the stated reason of a number on a key a caller can
// mint may not call it a share. For every allowance row on a mintable key, the
// doc comments of its reason, alias, argument, pin and enforcing declarations,
// and of the const or var blocks around them, carry none of the share words,
// unless the row carries a finding recorded for INV-003, whose vocabulary
// clause no field of a row can hold because it is a property of a comment.
//
// The reverse holds too: a row carrying such a finding while none of its
// comments uses a share word has a finding that no longer describes the tree.
func (g *gate) checkShareWords() []Finding {
	var found []Finding
	for _, d := range g.reg.decisions {
		if !d.Kind.IsAllowance() || !d.Key.Mintable() {
			continue
		}
		hits := g.shareHits(d)
		// A row without the finding answers for each word, and one with it
		// must still have a word the finding describes.
		if !d.RecordsDeparture(g.rules.shareInvariant) {
			for _, hit := range hits {
				found = append(found, Finding{
					Rule: "G13", Subject: d.ID, Position: hit.at,
					Message: fmt.Sprintf("%s describes a number on the %s key, which a caller can mint, as %q (%s)", hit.key, d.Key, hit.word, g.rules.shareInvariant),
				})
			}
			continue
		}
		if len(hits) == 0 {
			found = append(found, Finding{
				Rule: "G13", Subject: d.ID,
				Message: fmt.Sprintf("carries a finding recorded for %s, and no comment of its sites uses a share word", g.rules.shareInvariant),
			})
		}
	}
	return found
}

// shareHit is one share word in one declaration's comments.
type shareHit struct {
	key, at, word string
}

// shareHits are the share words the row's read declarations use.
func (g *gate) shareHits(d tenancy.Decision) []shareHit {
	sites := []tenancy.Site{d.ReasonAt}
	for _, s := range d.Sites {
		switch s.Role {
		case tenancy.Reason, tenancy.Alias, tenancy.Arg, tenancy.Pin, tenancy.Enforce:
			sites = append(sites, s)
		}
	}
	var hits []shareHit
	seen := map[string]bool{}
	for _, s := range sites {
		decl, err := g.p.lookup(s)
		if err != nil || seen[decl.key] {
			continue
		}
		seen[decl.key] = true
		for _, word := range wordsOf(commentText(decl.comments())) {
			if slices.Contains(g.rules.shareWords, word) {
				hits = append(hits, shareHit{key: decl.key, at: decl.where(g.p), word: word})
			}
		}
	}
	return hits
}

// wordsOf splits text into its lower-case words, so a share word matches as a
// whole word and never inside another ("affair", "unfairly").
func wordsOf(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) })
}
