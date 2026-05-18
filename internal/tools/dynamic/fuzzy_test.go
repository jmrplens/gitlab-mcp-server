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
		distance int
		want     int
	}{
		{distance: 0, want: 40},
		{distance: 1, want: 34},
		{distance: 2, want: 28},
		{distance: 3, want: 0},
	}

	for _, tt := range tests {
		got := fuzzyDistanceScore(tt.distance)
		if got != tt.want {
			t.Fatalf("fuzzyDistanceScore(%d) = %d, want %d", tt.distance, got, tt.want)
		}
	}
}

// TestFirstRuneString_HandlesUnicodeTokens verifies that prefix scoring works with both ASCII and
// Unicode tokens. This protects the fuzzy bonus from byte-slicing multibyte
// runes in non-English project or action terms.
func TestFirstRuneString_HandlesUnicodeTokens(t *testing.T) {
	tests := []struct {
		value string
		want  string
	}{
		{value: "merge", want: "m"},
		{value: "ñandú", want: "ñ"},
		{value: "", want: ""},
	}

	for _, tt := range tests {
		got := firstRuneString(tt.value)
		if got != tt.want {
			t.Fatalf("firstRuneString(%q) = %q, want %q", tt.value, got, tt.want)
		}
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

// TestFuzzyScoreEntryWithExplanation_EmptyTerms verifies FuzzyScoreEntryWithExplanation when empty terms.
func TestFuzzyScoreEntryWithExplanation_EmptyTerms(t *testing.T) {
	if score, explanation := fuzzyScoreEntryWithExplanation(actionEntry{}, nil); score != 0 || len(explanation.Reasons) != 0 {
		t.Fatalf("fuzzyScoreEntryWithExplanation(empty) = %d, %+v; want zero result", score, explanation)
	}
}

// TestFuzzyScoreEntryWithExplanation_MatchesNonExplanationScore verifies FuzzyScoreEntryWithExplanation matches non explanation score.
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
