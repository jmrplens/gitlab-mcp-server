package dynamic

import "testing"

func TestBoundedLevenshtein(t *testing.T) {
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

func TestFuzzyTokenScore(t *testing.T) {
	tokens := buildSearchTokens("merge_request list project issue")

	tests := []struct {
		name    string
		query   string
		wantMin int
	}{
		{name: "exact token", query: "merge", wantMin: 30},
		{name: "typo token", query: "merje", wantMin: 20},
		{name: "multi token typo", query: "merje requesy", wantMin: 20},
		{name: "no match", query: "abcdef", wantMin: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fuzzyTokenScore(tt.query, tokens)
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
