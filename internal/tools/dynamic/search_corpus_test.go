package dynamic

import (
	"encoding/json"
	"os"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/internal/tools"
)

type dynamicSearchCorpusCase struct {
	Category             string                     `json:"category"`
	Query                string                     `json:"query"`
	WantTop              string                     `json:"want_top"`
	WantTopN             []string                   `json:"want_top_n"`
	Limit                int                        `json:"limit"`
	CustomAliases        []dynamicSearchCorpusAlias `json:"custom_aliases"`
	ExpectZero           bool                       `json:"expect_zero"`
	ExpectAmbiguous      bool                       `json:"expect_ambiguous"`
	ExpectDestructiveTop bool                       `json:"expect_destructive_top"`
	ForbidDestructiveTop bool                       `json:"forbid_destructive_top"`
	Notes                string                     `json:"notes"`
}

type dynamicSearchCorpusAlias struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
}

// TestDynamicSearchCorpus validates the versioned dynamic search query corpus.
// Run it directly with:
//
//	go test ./internal/tools/dynamic/ -run TestDynamicSearchCorpus -count=1
func TestDynamicSearchCorpus(t *testing.T) {
	cases := loadDynamicSearchCorpus(t)
	baseCatalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{IncludeMCP: true})
	if err != nil {
		t.Fatalf("BuildActionCatalog() error = %v", err)
	}
	baseCatalog, err = AddStandaloneCatalog(baseCatalog, nil, StandaloneOptions{})
	if err != nil {
		t.Fatalf("AddStandaloneCatalog() error = %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.Category, func(t *testing.T) {
			registry := NewRegistryFromCatalog(baseCatalog)
			if len(tc.CustomAliases) > 0 {
				aliases := append([]actionAlias(nil), actionAliases()...)
				for _, customAlias := range tc.CustomAliases {
					aliases = append(aliases, actionAlias{Alias: customAlias.Alias, Canonical: customAlias.Canonical, Source: aliasSourceCompatibility, Searchable: true})
				}
				registry = newRegistryFromCatalog(baseCatalog, aliases)
			}

			_, output, searchErr := registry.Search(t.Context(), nil, SearchInput{Query: tc.Query, Limit: tc.Limit})
			if searchErr != nil {
				t.Fatalf("Search() error = %v", searchErr)
			}
			if tc.ExpectZero {
				if len(output.Results) != 0 {
					t.Fatalf("Search(%q) results = %+v, want zero results", tc.Query, output.Results)
				}
				return
			}
			if len(output.Results) == 0 {
				t.Fatalf("Search(%q) returned no results; notes: %s", tc.Query, tc.Notes)
			}
			if tc.WantTop != "" && output.Results[0].ID != tc.WantTop {
				t.Fatalf("Search(%q) top = %s, want %s; results = %+v", tc.Query, output.Results[0].ID, tc.WantTop, output.Results)
			}
			for _, want := range tc.WantTopN {
				if !slices.ContainsFunc(output.Results, func(result SearchResult) bool { return result.ID == want }) {
					t.Fatalf("Search(%q) results = %+v, want top-N action %s", tc.Query, output.Results, want)
				}
			}
			if tc.ExpectAmbiguous {
				if !slices.ContainsFunc(output.Results, func(result SearchResult) bool { return len(result.AmbiguousWith) > 0 }) {
					t.Fatalf("Search(%q) results = %+v, want ambiguity annotation", tc.Query, output.Results)
				}
			}
			if tc.ExpectDestructiveTop && !output.Results[0].Destructive {
				t.Fatalf("Search(%q) top = %+v, want destructive top result", tc.Query, output.Results[0])
			}
			if tc.ForbidDestructiveTop && output.Results[0].Destructive {
				t.Fatalf("Search(%q) top = %+v, want non-destructive top result", tc.Query, output.Results[0])
			}
		})
	}
}

func loadDynamicSearchCorpus(t *testing.T) []dynamicSearchCorpusCase {
	t.Helper()
	content, err := os.ReadFile("testdata/dynamic_search_queries.json")
	if err != nil {
		t.Fatalf("ReadFile(dynamic_search_queries.json) error = %v", err)
	}
	var cases []dynamicSearchCorpusCase
	if unmarshalErr := json.Unmarshal(content, &cases); unmarshalErr != nil {
		t.Fatalf("Unmarshal(dynamic_search_queries.json) error = %v", unmarshalErr)
	}
	if len(cases) == 0 {
		t.Fatal("dynamic search corpus is empty")
	}
	return cases
}
