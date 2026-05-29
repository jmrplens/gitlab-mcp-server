package dynamic

import (
	"context"
	"fmt"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/tools"
)

func TestDebugScores_SearchProjectsVsMRList(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistryFromCatalog(catalog)

	query := "project list search gitlab-mcp-server"
	terms := normalizeSearchTerms(query)

	fmt.Printf("Query: %q → terms: %v\n", query, terms)

	for _, entry := range reg.entries {
		doc := documentForEntry(entry)
		key := doc.Domain + "." + doc.Action
		if key == "search.projects" || key == "merge_request.list" {
			score := scoreEntry(entry, terms)
			intentScore := scoreSearchProjectsIntentValue(entry, terms)
			fmt.Printf("  %s → total=%d  searchProjectsIntent=%d\n", key, score, intentScore)
		}
	}
}

func TestDebugScores_ActionSpecificityForSearchProjects(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistryFromCatalog(catalog)

	query := "project list search gitlab-mcp-server"
	terms := normalizeSearchTerms(query)

	for _, entry := range reg.entries {
		doc := documentForEntry(entry)
		key := doc.Domain + "." + doc.Action
		if key == "search.projects" {
			base := 0
			base += scoreVerbIntentValue(entry, terms)
			base += scoreRequiredParamSignalValue(entry, terms)
			base += scoreCompoundTagSignalValue(entry, terms)
			base += scoreServiceAccountIntentValue(entry, terms)
			base += scoreScopeIntentValue(entry, terms)
			base += scoreCompareRefsIntentValue(entry, terms)
			base += scoreReleaseListIntentValue(entry, terms)
			base += scoreAnalyzeReleaseNotesIntentValue(entry, terms)
			base += scoreMRSecurityIntentValue(entry, terms)
			base += scoreDiscoverProjectIntentValue(entry, terms)
			base += scoreProjectGetIntentValue(entry, terms)
			boost := scoreSearchProjectsIntentValue(entry, terms)
			spec := scoreActionSpecificityValue(entry, terms)
			fmt.Printf("search.projects:\n  base=%d\n  searchProjectsBoost=%d\n  specificityPenalty=%d\n  total_before_floor=%d\n",
				base, boost, spec, base+boost+spec)
		}
	}
}

func TestDebugScores_SearchWithDefaultLimit(t *testing.T) {
	catalog, err := tools.BuildActionCatalog(nil, tools.ActionCatalogOptions{Enterprise: true, IncludeMCP: true})
	if err != nil {
		t.Fatal(err)
	}
	reg := NewRegistryFromCatalog(catalog)
	catalog2, err := AddStandaloneCatalog(catalog, nil, StandaloneOptions{})
	if err == nil {
		reg = NewRegistryFromCatalog(catalog2)
	}

	query := "project list search gitlab-mcp-server"

	// default limit (20) - should fail to find search.projects
	ctx := context.Background()
	_, out20, _ := reg.Search(ctx, nil, SearchInput{Query: query, Limit: 20})
	// limit 8 (as in unit test) - should find search.projects
	_, out8, _ := reg.Search(ctx, nil, SearchInput{Query: query, Limit: 8})

	fmt.Printf("Limit 8 top result: ")
	if len(out8.Results) > 0 {
		fmt.Printf("%s\n", out8.Results[0].ID)
	} else {
		fmt.Println("no results")
	}
	fmt.Printf("Limit 20 top result: ")
	if len(out20.Results) > 0 {
		fmt.Printf("%s\n", out20.Results[0].ID)
	} else {
		fmt.Println("no results")
	}
}
