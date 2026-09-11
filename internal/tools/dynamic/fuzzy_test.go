// fuzzy_test.go contains unit tests for the fuzzy-matching scoring layer used by the dynamic tool surface.
package dynamic

import "testing"

// TestBuildSearchTokens_NormalizesAndDeduplicates verifies that fuzzy search tokens are normalized,
// deduplicated, and omitted when the search text has no word characters.
// This matters because every dynamic action entry reuses these cached tokens
// during typo fallback search.
func TestBuildSearchTokens_NormalizesAndDeduplicates(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "normalizes separators and removes duplicates", text: "Merge merge_request merge", want: []string{"merge", "request"}},
		{name: "returns nil for punctuation only input", text: "...---___", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSearchTokens(tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("buildSearchTokens() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("buildSearchTokens() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

// TestBoundedLevenshtein_CoversThresholdBranches validates the edit-distance helper used by DIY fuzzy
// search. It covers exact matches, close typo recovery, distance cutoffs, empty
// strings, swapped input lengths, final threshold rejection, and Unicode text.
//
// Two cases are about the interior of the algorithm rather than its contract.
// The length gap is checked one rune either side of the bound, because a
// tightened comparison there silently drops every two-edit deletion such as
// "merge" against "mer". And "zab" against "abcd" is three edits only while
// the first row counts the prefix of the shorter string it skips: an unseeded
// row would let the match start anywhere in "zab" for free and report two.
func TestBoundedLevenshtein_CoversThresholdBranches(t *testing.T) {
	tests := []struct {
		name        string
		a           string
		b           string
		maxDistance int
		wantOK      bool
		wantDist    int
	}{
		{name: "exact", a: "merge", b: "merge", maxDistance: 2, wantOK: true, wantDist: 0},
		{name: "single substitution", a: "merje", b: "merge", maxDistance: 2, wantOK: true, wantDist: 1},
		{name: "single insertion", a: "request", b: "requesst", maxDistance: 2, wantOK: true, wantDist: 1},
		{name: "empty left within threshold", a: "", b: "ab", maxDistance: 2, wantOK: true, wantDist: 2},
		{name: "empty left beyond threshold", a: "", b: "abc", maxDistance: 2, wantOK: false, wantDist: 0},
		{name: "empty right within threshold", a: "ab", b: "", maxDistance: 2, wantOK: true, wantDist: 2},
		{name: "empty right beyond threshold", a: "abc", b: "", maxDistance: 2, wantOK: false, wantDist: 0},
		{name: "swaps longer left input", a: "merge", b: "merg", maxDistance: 2, wantOK: true, wantDist: 1},
		{name: "length gap exactly at the bound", a: "merge", b: "mer", maxDistance: 2, wantOK: true, wantDist: 2},
		{name: "length gap one past the bound", a: "merge", b: "me", maxDistance: 2, wantOK: false, wantDist: 0},
		{name: "no free start from a shared suffix", a: "zab", b: "abcd", maxDistance: 3, wantOK: true, wantDist: 3},
		{name: "rejects by final threshold", a: "abc", b: "abd", maxDistance: 0, wantOK: false, wantDist: 0},
		{name: "unicode substitution counts runes", a: "proyécto", b: "proyecto", maxDistance: 1, wantOK: true, wantDist: 1},
		{name: "too far", a: "abc", b: "project", maxDistance: 2, wantOK: false, wantDist: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotDist, gotOK := boundedLevenshtein(tt.a, tt.b, tt.maxDistance)
			if gotOK != tt.wantOK {
				t.Fatalf("boundedLevenshtein() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotDist != tt.wantDist {
				t.Fatalf("boundedLevenshtein() distance = %d, want %d", gotDist, tt.wantDist)
			}
		})
	}
}

// TestFuzzyDistanceScore_MapsDistanceToScore validates the fixed score mapping used after a token
// passes the bounded Levenshtein check. The default branch is covered directly
// so future scoring changes do not silently accept out-of-range distances.
func TestFuzzyDistanceScore_MapsDistanceToScore(t *testing.T) {
	tests := []struct {
		name     string
		distance int
		want     int
	}{
		{name: "exact_match", distance: 0, want: 40},
		{name: "one_edit", distance: 1, want: 34},
		{name: "two_edits", distance: 2, want: 28},
		{name: "beyond_bound", distance: 3, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyDistanceScore(tt.distance)
			if got != tt.want {
				t.Fatalf("fuzzyDistanceScore(%d) = %d, want %d", tt.distance, got, tt.want)
			}
		})
	}
}

// TestFirstRuneString_HandlesUnicodeTokens verifies that prefix scoring works with both ASCII and
// Unicode tokens. This protects the fuzzy bonus from byte-slicing multibyte
// runes in non-English project or action terms.
func TestFirstRuneString_HandlesUnicodeTokens(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "ascii_token", value: "merge", want: "m"},
		{name: "multibyte_leading_rune", value: "ñandú", want: "ñ"},
		{name: "empty_token", value: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstRuneString(tt.value)
			if got != tt.want {
				t.Fatalf("firstRuneString(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestFuzzyTokenScore_CoversTokenMatching validates scoring for exact tokens, typo tokens,
// multi-token typo recovery, ignored short tokens, empty inputs, and non-matches.
// These are the primitive behaviors that make dynamic catalog search recover from
// small spelling mistakes without replacing exact search ranking.
func TestFuzzyTokenScore_CoversTokenMatching(t *testing.T) {
	tokens := buildSearchTokens("merge_request list project issue")

	tests := []struct {
		name    string
		query   string
		tokens  []string
		wantMin int
	}{
		{name: "exact token", query: "merge", tokens: tokens, wantMin: 30},
		{name: "typo token", query: "merje", tokens: tokens, wantMin: 20},
		{name: "multi token typo", query: "merje requesy", tokens: tokens, wantMin: 20},
		{name: "short token ignored", query: "mr", tokens: tokens, wantMin: 0},
		{name: "short token does not penalize eligible token", query: "mr merje", tokens: tokens, wantMin: 30},
		{name: "empty query", query: "", tokens: tokens, wantMin: 0},
		{name: "empty search tokens", query: "merge", tokens: nil, wantMin: 0},
		{name: "no match", query: "abcdef", tokens: tokens, wantMin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyTokenScore(tt.query, tt.tokens)
			if tt.wantMin == 0 {
				if got != 0 {
					t.Fatalf("fuzzyTokenScore() = %d, want 0", got)
				}
				return
			}
			if got < tt.wantMin {
				t.Fatalf("fuzzyTokenScore() = %d, want >= %d", got, tt.wantMin)
			}
		})
	}
}

// TestFuzzyScoreEntry_CoversActionScoring validates action-level fuzzy scoring across empty terms,
// no matches, full matches, and N-1 threshold rejection. This is the final gate
// before typo fallback results are added to the dynamic search result set.
func TestFuzzyScoreEntry_CoversActionScoring(t *testing.T) {
	entry := actionEntry{
		ID:           "merge_request.list",
		SearchTokens: buildSearchTokens("merge_request list project author"),
	}

	tests := []struct {
		name    string
		terms   []searchTerm
		wantMin int
	}{
		{name: "empty terms", terms: nil, wantMin: 0},
		{name: "no matched terms", terms: normalizeSearchTerms("abcdef"), wantMin: 0},
		{name: "matches typo terms", terms: normalizeSearchTerms("merje requesy"), wantMin: 20},
		{name: "rejects too many missing terms", terms: normalizeSearchTerms("merje requesy zzz yyy"), wantMin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyScoreEntry(entry, tt.terms)
			if tt.wantMin == 0 {
				if got != 0 {
					t.Fatalf("fuzzyScoreEntry() = %d, want 0", got)
				}
				return
			}
			if got < tt.wantMin {
				t.Fatalf("fuzzyScoreEntry() = %d, want >= %d", got, tt.wantMin)
			}
		})
	}
}

// TestFuzzyTokenScoreWithReason_DefensiveBranches covers zero-score defensive
// paths and the typo path that returns a fuzzy-token explanation. It uses a
// fixed token fixture built with buildSearchTokens and table-driven subtests.
func TestFuzzyTokenScoreWithReason_DefensiveBranches(t *testing.T) {
	tokens := buildSearchTokens("merge request")
	tests := []struct {
		name        string
		query       string
		alternative string
		tokens      []string
		wantMatch   bool
	}{
		{name: "empty query", query: "", alternative: "", tokens: tokens},
		{name: "short query", query: "mr", alternative: "mr", tokens: tokens},
		{name: "far query", query: "abcdef", alternative: "abcdef", tokens: tokens},
		{name: "empty tokens", query: "merge", alternative: "merge"},
		{name: "typo fuzzy token", query: "marge", alternative: "merge", tokens: tokens, wantMatch: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := fuzzyTokenScoreWithReason(tt.query, tt.alternative, tt.tokens)
			if !tt.wantMatch {
				if score != 0 || reason.Field != "" {
					t.Fatalf("fuzzyTokenScoreWithReason() = %d, %+v; want zero result", score, reason)
				}
				return
			}
			if score == 0 {
				t.Fatal("score = 0, want fuzzy match")
			}
			if !reason.Fuzzy {
				t.Fatalf("reason.Fuzzy = false, want true: %+v", reason)
			}
			if reason.Field != searchFieldFuzzyToken {
				t.Fatalf("reason.Field = %q, want %q", reason.Field, searchFieldFuzzyToken)
			}
			if reason.QueryTerm != "marge" {
				t.Fatalf("reason.QueryTerm = %q, want marge", reason.QueryTerm)
			}
			if reason.Alternative != "merge" {
				t.Fatalf("reason.Alternative = %q, want merge", reason.Alternative)
			}
		})
	}
}

// TestFuzzyScoreEntryWithExplanation_EmptyTerms verifies the FuzzyScoreEntryWithExplanation_EmptyTerms handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestFuzzyScoreEntryWithExplanation_EmptyTerms(t *testing.T) {
	if score, explanation := fuzzyScoreEntryWithExplanation(actionEntry{}, nil); score != 0 || len(explanation.Reasons) != 0 {
		t.Fatalf("fuzzyScoreEntryWithExplanation(empty) = %d, %+v; want zero result", score, explanation)
	}
}

// TestFuzzyScoreEntryWithExplanation_MatchesNonExplanationScore verifies the FuzzyScoreEntryWithExplanation_MatchesNonExplanationScore handler.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the returned output matches the expected fields.
func TestFuzzyScoreEntryWithExplanation_MatchesNonExplanationScore(t *testing.T) {
	entry := actionEntry{
		ID:           "merge_request.list",
		Domain:       "merge_request",
		Action:       "list",
		Tags:         []string{"merge_request", "project"},
		Aliases:      []string{"merge request list"},
		SearchTokens: buildSearchTokens("merge_request list project"),
	}
	terms := normalizeSearchTerms("marge request project")

	want := fuzzyScoreEntry(entry, terms)
	got, explanation := fuzzyScoreEntryWithExplanation(entry, terms)

	if want == 0 {
		t.Fatal("fuzzyScoreEntry returned 0, want a positive score for typo-recovery query")
	}
	if got != want {
		t.Fatalf("fuzzyScoreEntryWithExplanation() = %d, want %d", got, want)
	}
	if explanation.TotalScore != got {
		t.Fatalf("explanation.TotalScore = %d, want %d", explanation.TotalScore, got)
	}
	if explanation.MatchedTerms == 0 || len(explanation.Reasons) == 0 {
		t.Fatalf("explanation missing matches/reasons: %+v", explanation)
	}
}

// TestFuzzyTermMatchesResourceSignal_EitherSpellingCounts verifies that a term
// carries the resource signal when the word the user typed names the entry's
// resource, or when the alternative it was expanded into does. Either alone is
// enough: a query that never uses the domain's own word reaches the resource
// through a synonym, and demanding both spellings would withhold the boost
// from every expanded term.
func TestFuzzyTermMatchesResourceSignal_EitherSpellingCounts(t *testing.T) {
	entry := actionEntry{ID: "merge_request.list", Domain: "merge_request", Action: "list"}

	tests := []struct {
		name        string
		raw         string
		alternative string
		want        bool
	}{
		{name: "raw names the domain", raw: "merge", alternative: "zzzzz", want: true},
		{name: "alternative names the domain", raw: "zzzzz", alternative: "merge", want: true},
		{name: "raw names the action", raw: "list", alternative: "zzzzz", want: true},
		{name: "neither names the resource", raw: "zzzzz", alternative: "yyyyy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fuzzyTermMatchesResourceSignal(entry, tt.raw, tt.alternative); got != tt.want {
				t.Errorf("fuzzyTermMatchesResourceSignal(%q, %q) = %v, want %v", tt.raw, tt.alternative, got, tt.want)
			}
		})
	}
}

// TestFuzzyScoreEntry_MissingTermBudgetFollowsTermCount verifies how many query
// terms an action may miss and still score. One or two terms must all match,
// and only from three terms on may one be missing. That threshold is what
// keeps a two-word query from recovering an action that answers half of it,
// while still letting a long natural-language query lose one word to a
// spelling nothing in the catalog carries.
func TestFuzzyScoreEntry_MissingTermBudgetFollowsTermCount(t *testing.T) {
	entry := actionEntry{
		ID:           "merge_request.list",
		SearchTokens: buildSearchTokens("merge_request list project author"),
	}

	tests := []struct {
		name      string
		query     string
		wantScore bool
	}{
		{name: "two terms both matched", query: "merge request", wantScore: true},
		{name: "two terms one missing", query: "merge zzzzz"},
		{name: "three terms one missing", query: "merge request zzzzz", wantScore: true},
		{name: "three terms two missing", query: "merge zzzzz yyyyy"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyScoreEntry(entry, normalizeSearchTerms(tt.query))
			if tt.wantScore && got <= 0 {
				t.Errorf("fuzzyScoreEntry(%q) = %d, want a positive score", tt.query, got)
			}
			if !tt.wantScore && got != 0 {
				t.Errorf("fuzzyScoreEntry(%q) = %d, want 0", tt.query, got)
			}
		})
	}
}

// TestFuzzyTokenScore_ExactScores pins the arithmetic behind one fuzzy token
// score, which the other tests only assert a floor for. It fixes the prefix
// bonus, the shortest part that is still compared, the widest length gap that
// still is, the best rather than the last matching token, and the averaging
// over the parts of a multi-word alternative. Each is a number a scorer change
// would move silently: nothing above a floor assertion notices a bonus that
// stopped being applied or a two-part alternative that stopped being averaged.
func TestFuzzyTokenScore_ExactScores(t *testing.T) {
	tests := []struct {
		name   string
		needle string
		tokens []string
		want   int
	}{
		{name: "exact match with prefix bonus", needle: "merge", tokens: buildSearchTokens("merge"), want: 42},
		{name: "one edit without a shared prefix", needle: "merge", tokens: buildSearchTokens("xerge"), want: 34},
		{name: "one edit with a shared prefix", needle: "merge", tokens: buildSearchTokens("marge"), want: 36},
		{name: "two edits at the length bound", needle: "merge", tokens: buildSearchTokens("mer"), want: 30},
		{name: "three letter part is compared", needle: "tag", tokens: buildSearchTokens("tag list"), want: 42},
		{name: "two letter part is skipped", needle: "mr", tokens: buildSearchTokens("mr list")},
		{name: "best token wins over the later one", needle: "merge", tokens: buildSearchTokens("merge marge"), want: 42},
		{name: "multi word alternative averages its parts", needle: "merge request", tokens: buildSearchTokens("merge request"), want: 42},
		{name: "one weak part drags the average down", needle: "merge request", tokens: buildSearchTokens("merge requesq"), want: 39},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fuzzyTokenScore(tt.needle, tt.tokens); got != tt.want {
				t.Errorf("fuzzyTokenScore(%q, %v) = %d, want %d", tt.needle, tt.tokens, got, tt.want)
			}
		})
	}
}

// TestFuzzyTokenScoreWithReason_ReportsTheBestMatchedToken verifies which token
// a fuzzy explanation names when several match. The reason is what a caller
// reads to understand a recovered match, so it must name the token that earned
// the score and, on a tie, the first one rather than whichever happened to be
// scored last.
func TestFuzzyTokenScoreWithReason_ReportsTheBestMatchedToken(t *testing.T) {
	tests := []struct {
		name         string
		alternative  string
		tokens       []string
		wantMatched  string
		wantDistance int
		wantScore    int
	}{
		{
			name:        "first part of a tie is reported",
			alternative: "merge request",
			tokens:      buildSearchTokens("merge request"),
			wantMatched: "merge",
			wantScore:   42,
		},
		{
			name:         "first token of a tie is reported",
			alternative:  "merge",
			tokens:       buildSearchTokens("merje merga"),
			wantMatched:  "merje",
			wantDistance: 1,
			wantScore:    36,
		},
		{
			name:         "closest token wins over an earlier one",
			alternative:  "merge",
			tokens:       buildSearchTokens("marge merge"),
			wantMatched:  "merge",
			wantDistance: 0,
			wantScore:    42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, reason := fuzzyTokenScoreWithReason(tt.alternative, tt.alternative, tt.tokens)
			if score != tt.wantScore {
				t.Errorf("fuzzyTokenScoreWithReason(%q) score = %d, want %d", tt.alternative, score, tt.wantScore)
			}
			if reason.MatchedValue != tt.wantMatched {
				t.Errorf("reason.MatchedValue = %q, want %q", reason.MatchedValue, tt.wantMatched)
			}
			if reason.Distance != tt.wantDistance {
				t.Errorf("reason.Distance = %d, want %d", reason.Distance, tt.wantDistance)
			}
		})
	}
}

// TestBestFuzzyTermMatch_KeepsTheFirstAlternativeOfATie verifies that when two
// expansions of one query term score the same, the earlier one is kept with
// its resource signal. The alternatives are ordered with the word the user
// typed first and its synonyms after it, so a later synonym replacing an equal
// earlier match would decide the resource boost by synonym order.
func TestBestFuzzyTermMatch_KeepsTheFirstAlternativeOfATie(t *testing.T) {
	entry := actionEntry{
		ID:           "merge_request.list",
		Domain:       "merge_request",
		SearchTokens: buildSearchTokens("merge project"),
	}
	term := searchTerm{Raw: "zzzzz", Alternatives: []string{"merge", "project"}}

	match := bestFuzzyTermMatch(entry, term, false)
	if match.score != 42 {
		t.Errorf("bestFuzzyTermMatch() score = %d, want 42 from both alternatives", match.score)
	}
	if !match.resourceSignal {
		t.Error("bestFuzzyTermMatch() resourceSignal = false, want the first alternative's domain signal")
	}
}

// TestComparableTokenLength_BoundsTheRuneDifference verifies the cheap length
// pre-filter that runs before the edit-distance matrix. It must accept a gap of
// exactly fuzzyMaxDistance in either direction, since that is a real two-edit
// deletion, and count runes rather than bytes so an accented token is not
// rejected for the byte its accent costs.
func TestComparableTokenLength_BoundsTheRuneDifference(t *testing.T) {
	tests := []struct {
		name   string
		needle string
		token  string
		want   bool
	}{
		{name: "same length", needle: "merge", token: "marge", want: true},
		{name: "needle longer by one", needle: "merge", token: "merg", want: true},
		{name: "needle longer by the bound", needle: "merge", token: "mer", want: true},
		{name: "needle longer past the bound", needle: "merge", token: "me"},
		{name: "token longer by the bound", needle: "mer", token: "merge", want: true},
		{name: "token longer past the bound", needle: "me", token: "merge"},
		{name: "accents count as one rune", needle: "proyécto", token: "proyecto", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := comparableTokenLength(tt.needle, tt.token); got != tt.want {
				t.Errorf("comparableTokenLength(%q, %q) = %v, want %v", tt.needle, tt.token, got, tt.want)
			}
		})
	}
}
