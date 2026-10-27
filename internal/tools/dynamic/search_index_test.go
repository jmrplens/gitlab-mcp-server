// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// search_index_test covers lightweight candidate-index behavior in package
// dynamic, including top-result parity vs full scan and defensive/fallback
// branches using table-driven subtests and tiny in-memory fixtures.
package dynamic

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestSearchIndex_CandidateGenerationPreservesFullScanTopResults verifies that
// the lightweight index narrows candidate entries without changing the top
// lexical results for the baseline query set.
func TestSearchIndex_CandidateGenerationPreservesFullScanTopResults(t *testing.T) {
	registry := realCatalogRegistry(t)

	queries := []string{
		"merge request list open author project",
		"list open issues",
		"pipeline run trigger",
		"ci variable secret",
		"project delete",
		"discover project from remote",
	}

	for _, query := range queries {
		t.Run(query, func(t *testing.T) {
			terms := normalizeSearchTerms(query)
			scored, err := registry.scoredMatches(t.Context(), terms, scoreEntryWithExplanation)
			if err != nil {
				t.Fatalf("scoredMatches(%q) error = %v", query, err)
			}
			indexed := sortAndLimitMatches(scored, 5)
			fullScan := sortAndLimitMatches(fullScanScoredMatches(registry.entries, terms, scoreEntryWithExplanation), 5)
			if len(fullScan) == 0 {
				t.Fatalf("full scan returned no lexical matches for %q", query)
			}
			if !slices.Equal(scoredActionIDs(indexed), scoredActionIDs(fullScan)) {
				t.Fatalf("indexed matches = %v, full scan = %v", scoredActionIDs(indexed), scoredActionIDs(fullScan))
			}
		})
	}
}

// TestSearchIndex_DefensiveBranches verifies empty-index, empty-term, duplicate
// token, and fallback candidate behavior for the lightweight dynamic search
// index. It uses a tiny hand-built index fixture.
func TestSearchIndex_DefensiveBranches(t *testing.T) {
	var emptyIndex searchIndex
	if got := emptyIndex.candidateEntryIndexes(nil); got != nil {
		t.Fatalf("candidateEntryIndexes(empty index) = %v, want nil", got)
	}

	index := searchIndex{
		byToken: map[string][]int{},
		all:     []int{0, 1},
	}
	if got := index.candidateEntryIndexes(nil); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(empty terms) = %v, want all indexes", got)
	}
	index.addValues(index.byToken, []string{"", "project.delete", "project.delete"}, 0)
	if got := index.byToken["project.delete"]; len(got) != 1 || got[0] != 0 {
		t.Fatalf("byToken[project.delete] = %v, want single posting 0", got)
	}
	if got := index.candidateEntryIndexes(normalizeSearchTerms("unknown")); strings.Join(intsToStrings(got), ",") != "0,1" {
		t.Fatalf("candidateEntryIndexes(no candidates) = %v, want full fallback", got)
	}
}

// TestSearchIndex_CandidateEntryIndexesNarrowsToTheMatchingPostings verifies
// that a term with an index bucket narrows the candidate set to that bucket
// instead of handing the full catalog back. Both answers rank identically,
// because the index only decides which entries are scored, so nothing about a
// search result would notice a narrowing that quietly stopped narrowing: what
// it costs is a scoring pass over every action on every query.
func TestSearchIndex_CandidateEntryIndexesNarrowsToTheMatchingPostings(t *testing.T) {
	index := searchIndex{
		byToken:  map[string][]int{"pipeline": {1}},
		byAlias:  map[string][]int{},
		byDomain: map[string][]int{},
		byAction: map[string][]int{},
		all:      []int{0, 1, 2},
	}

	got := index.candidateEntryIndexes(normalizeSearchTerms("pipeline"))
	if strings.Join(intsToStrings(got), ",") != "1" {
		t.Errorf("candidateEntryIndexes(pipeline) = %v, want only the indexed posting 1", got)
	}
}

// TestSearchIndex_CandidateEntryIndexes_ReachesTheActionsTheSingularNames
// verifies that the singular of a plural brings in the actions whose domain
// or action word it is, and no others.
//
// The singular is no alternative of its term and is scored against the
// action ID alone, so the ID buckets are where it has to reach: an action
// that only the singular names would otherwise never be scored. The alias
// and token buckets of the same word are left out, because nothing the
// singular is scored against is found through them.
func TestSearchIndex_CandidateEntryIndexes_ReachesTheActionsTheSingularNames(t *testing.T) {
	index := searchIndex{
		byToken:  map[string][]int{"issue": {3}},
		byAlias:  map[string][]int{"issue": {4}},
		byDomain: map[string][]int{"issue": {1}},
		byAction: map[string][]int{"issue": {2}},
		all:      []int{0, 1, 2, 3, 4},
	}
	terms := []searchTerm{{Raw: "issues", Alternatives: []string{"issues"}, Singular: "issue"}}

	if got := index.candidateEntryIndexes(terms); strings.Join(intsToStrings(got), ",") != "1,2" {
		t.Errorf("candidateEntryIndexes(issues) = %v, want the domain and action postings of issue, 1 and 2", got)
	}
}

// TestSearchDocumentIndexTokens_ReadsEveryField verifies that the token index
// carries a word from every searchable field of a document, in field order and
// without repeats. A field dropped here cannot be searched for at all: the
// index decides which entries a term even reaches, and the scorers below it
// never see an entry the index left out.
func TestSearchDocumentIndexTokens_ReadsEveryField(t *testing.T) {
	document := searchDocument{
		Backend:          "backendword",
		Capability:       "capabilityword",
		Resource:         "resourceword",
		Operation:        "operationword",
		Scope:            "scopeword",
		CanonicalID:      "canonical.idword",
		Tool:             "toolword",
		Domain:           "domainword",
		Action:           "actionword",
		FlatText:         "flat textword",
		IDWords:          []string{"idwordone"},
		DomainWords:      []string{"domainwordone"},
		ActionWords:      []string{"actionwordone"},
		Aliases:          []string{"alias_wordone"},
		Tags:             []string{"tagword", "backendword"},
		RequiredParams:   []string{"required_param"},
		SchemaProperties: []string{"schema_property"},
	}

	want := []string{
		"backendword", "capabilityword", "resourceword", "operationword", "scopeword",
		"canonical", "idword", "toolword", "domainword", "actionword", "flat", "textword",
		"idwordone", "domainwordone", "actionwordone", "alias", "wordone", "tagword",
		"required", "param", "schema", "property",
	}
	if got := searchDocumentIndexTokens(document); !slices.Equal(got, want) {
		t.Errorf("searchDocumentIndexTokens() = %v, want %v", got, want)
	}
}

// TestSearchIndex_SingularOf_KeepsTheFirstSingularTheCatalogSpells verifies
// the plural rule against a hand-built vocabulary: each of the three regular
// plurals is undone, the first candidate that names a domain or action word
// is kept, and a word with no such singular is left alone.
//
// The vocabulary is what keeps the rule from stemming words that only end in
// "s": "access", "status" and "alias" would become "acces", "statu" and
// "alia", none of which the catalog spells, so they have no singular. "notes"
// reads as "note" and not as "not", which is in the vocabulary too, because
// the plain "s" is tried before "es". A word in only one of the two halves of
// the vocabulary is found there, each half on its own.
func TestSearchIndex_SingularOf_KeepsTheFirstSingularTheCatalogSpells(t *testing.T) {
	index := searchIndex{
		byDomain: map[string][]int{"issue": {0}, "branch": {1}, "policy": {2}, "status": {3}, "not": {4}},
		byAction: map[string][]int{"note": {5}, "cookie": {6}, "list": {7}},
	}

	cases := []struct {
		name string
		word string
		want string
	}{
		{name: "an s plural of a domain word", word: "issues", want: "issue"},
		{name: "an es plural", word: "branches", want: "branch"},
		{name: "an ies plural", word: "policies", want: "policy"},
		{name: "an es plural of a word ending in s", word: "statuses", want: "status"},
		{name: "an s plural of an action word", word: "lists", want: "list"},
		{name: "an ies word whose singular takes the s alone", word: "cookies", want: "cookie"},
		{name: "s is tried before es", word: "notes", want: "note"},
		{name: "access is no plural", word: "access"},
		{name: "status is no plural", word: "status"},
		{name: "alias is no plural", word: "alias"},
		{name: "a singular", word: "issue"},
		{name: "a word not ending in s", word: "pipeline"},
		{name: "a plural of a word the catalog does not spell", word: "widgets"},
		{name: "a lone s", word: "s"},
		{name: "a lone ies", word: "ies"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := index.singularOf(tc.word); got != tc.want {
				t.Errorf("singularOf(%q) = %q, want %q", tc.word, got, tc.want)
			}
		})
	}
}

// TestSearchIndex_SearchTerms_ReadsTheSingularAndTheDomainAPairNames verifies
// what the terms a search scores carry beyond what normalizeSearchTerms gives
// them: the singular of a plural, a verb that starts a domain name, and the
// domain a pair written in the plural names. Every field of every term is
// compared, the alternatives included, which have to stay exactly as
// normalizeSearchTerms made them: the singular is kept apart from them, so
// that nothing reading the alternatives sees it.
//
// The real catalog is used for the words a model sends: a plural of a domain
// and an ies plural carry their singular, a plural whose synonym already
// names the singular carries it as well, and "access", "status" and "alias",
// whose stems the catalog does not spell, carry none. Merge starts
// merge_request with request in either spelling; written in the plural the
// pair names the domain on both words, and written in the singular it does
// not. Merge with no word after it is a verb still, and ci, the first word of
// ci_variable, is no verb and is left as it is.
//
// A branch, tag or commit names the point an action with a required ref
// starts from in a query with a verb, written in the plural as well, and in
// no query without one; merge read as the first word of merge request is no
// verb that would make the query say what to do.
func TestSearchIndex_SearchTerms_ReadsTheSingularAndTheDomainAPairNames(t *testing.T) {
	index := realCatalogRegistry(t).SearchIndex

	cases := []struct {
		name      string
		query     string
		singulars []string
		nouns     []bool
		domains   []string
		refs      []bool
	}{
		{name: "a plural of a domain", query: "list branches", singulars: []string{"", "branch"}, nouns: []bool{false, false}, domains: []string{"", ""}, refs: []bool{false, true}},
		{name: "an ies plural", query: "repositories", singulars: []string{"repository"}, nouns: []bool{false}, domains: []string{""}, refs: []bool{false}},
		{name: "a plural whose synonym already names the singular", query: "jobs", singulars: []string{"job"}, nouns: []bool{false}, domains: []string{""}, refs: []bool{false}},
		{name: "a plural whose synonym is its singular alone", query: "tokens", singulars: []string{"token"}, nouns: []bool{false}, domains: []string{""}, refs: []bool{false}},
		{name: "words that only end in s", query: "access status alias", singulars: []string{"", "", ""}, nouns: []bool{false, false, false}, domains: []string{"", "", ""}, refs: []bool{false, false, false}},
		{name: "a verb that starts a domain with a plural", query: "show merge requests", singulars: []string{"", "", "request"}, nouns: []bool{false, true, false}, domains: []string{"", "merge_request", "merge_request"}, refs: []bool{false, false, false}},
		{name: "a verb that starts a domain with a singular", query: "merge request", singulars: []string{"", ""}, nouns: []bool{true, false}, domains: []string{"", ""}, refs: []bool{false, false}},
		{name: "a verb with no word after it", query: "show merge", singulars: []string{"", ""}, nouns: []bool{false, false}, domains: []string{"", ""}, refs: []bool{false, false}},
		{name: "a domain whose first word is no verb", query: "ci variables", singulars: []string{"", "variable"}, nouns: []bool{false, false}, domains: []string{"", ""}, refs: []bool{false, false}},
		{name: "ref resources in the plural", query: "create branches from commits", singulars: []string{"", "branch", "commit"}, nouns: []bool{false, false, false}, domains: []string{"", "", ""}, refs: []bool{false, true, true}},
		{name: "a ref resource with no verb", query: "pipeline for branch", singulars: []string{"", ""}, nouns: []bool{false, false}, domains: []string{"", ""}, refs: []bool{false, false}},
		{name: "a ref resource after a verb that starts a domain", query: "merge request branch", singulars: []string{"", "", ""}, nouns: []bool{true, false, false}, domains: []string{"", "", ""}, refs: []bool{false, false, false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := normalizeSearchTerms(tc.query)
			for position := range want {
				want[position].Singular = tc.singulars[position]
				want[position].Noun = tc.nouns[position]
				want[position].Domain = tc.domains[position]
				want[position].RefResource = tc.refs[position]
			}
			got := index.searchTerms(tc.query)
			if !slices.EqualFunc(got, want, func(a, b searchTerm) bool {
				return a.Raw == b.Raw && slices.Equal(a.Alternatives, b.Alternatives) && a.Singular == b.Singular && a.Noun == b.Noun && a.Domain == b.Domain && a.RefResource == b.RefResource
			}) {
				t.Errorf("searchTerms(%q) = %+v, want %+v", tc.query, got, want)
			}
		})
	}
}

// TestSearchIndex_DomainStartedBy_NamesTheDomainAVerbStarts verifies the
// pair rule against a hand-built vocabulary: a verb the next word completes
// into a whole domain name starts that domain, as written or through the next
// word's singular, which it reports; a word that is no verb, or a verb the
// next word completes into nothing, starts none.
func TestSearchIndex_DomainStartedBy_NamesTheDomainAVerbStarts(t *testing.T) {
	index := searchIndex{byDomain: map[string][]int{
		"merge_request": {0}, "merge": {0}, "request": {0},
		"ci_variable": {1}, "ci": {1}, "variable": {1},
	}}

	cases := []struct {
		name                string
		verb                string
		next                searchTerm
		wantDomain          string
		wantThroughSingular bool
	}{
		{name: "the next word as written", verb: "merge", next: searchTerm{Raw: "request"}, wantDomain: "merge_request"},
		{name: "the next word through its singular", verb: "merge", next: searchTerm{Raw: "requests", Singular: "request"}, wantDomain: "merge_request", wantThroughSingular: true},
		{name: "a first word that is no verb", verb: "ci", next: searchTerm{Raw: "variable"}},
		{name: "a verb the next word completes into nothing", verb: "list", next: searchTerm{Raw: "request"}},
		{name: "a verb and a word of no domain", verb: "merge", next: searchTerm{Raw: "trains", Singular: "train"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			domain, throughSingular := index.domainStartedBy(tc.verb, tc.next)
			if domain != tc.wantDomain || throughSingular != tc.wantThroughSingular {
				t.Errorf("domainStartedBy(%q, %+v) = %q, %v, want %q, %v", tc.verb, tc.next, domain, throughSingular, tc.wantDomain, tc.wantThroughSingular)
			}
		})
	}
}

func intsToStrings(values []int) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, strconv.Itoa(value))
	}
	return out
}
