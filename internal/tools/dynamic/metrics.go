package dynamic

import "sync/atomic"

// RegistryMetrics summarizes the static dynamic registry and search index.
type RegistryMetrics struct {
	ActionCount            int
	IndexTokenCount        int
	IndexPostingCount      int
	AliasCount             int
	SearchableAliasCount   int
	UnsearchableAliasCount int
	AmbiguousAliasCount    int
}

// SearchRuntimeMetrics summarizes process-local dynamic search quality events.
type SearchRuntimeMetrics struct {
	Searches                     uint64
	ZeroResultSearches           uint64
	FuzzyFallbackSearches        uint64
	AmbiguousAliasQueries        uint64
	LowConfidenceSearches        uint64
	DestructiveFuzzySuppressions uint64
}

type searchRuntimeCounters struct {
	searches                     atomic.Uint64
	zeroResultSearches           atomic.Uint64
	fuzzyFallbackSearches        atomic.Uint64
	ambiguousAliasQueries        atomic.Uint64
	lowConfidenceSearches        atomic.Uint64
	destructiveFuzzySuppressions atomic.Uint64
}

var dynamicSearchRuntimeCounters searchRuntimeCounters

// Metrics returns static registry and search-index metrics.
func (r *Registry) Metrics() RegistryMetrics {
	searchableAliases := make(map[string]struct{})
	for _, entry := range r.entries {
		for _, alias := range documentForEntry(entry).Aliases {
			searchableAliases[alias] = struct{}{}
		}
	}
	aliasCount := len(r.aliases)
	for _, targets := range r.ambiguousAliases {
		aliasCount += len(targets)
	}
	return RegistryMetrics{
		ActionCount:            len(r.entries),
		IndexTokenCount:        len(r.SearchIndex.byToken),
		IndexPostingCount:      searchIndexPostingCount(r.SearchIndex),
		AliasCount:             aliasCount,
		SearchableAliasCount:   len(searchableAliases),
		UnsearchableAliasCount: max(0, aliasCount-len(searchableAliases)),
		AmbiguousAliasCount:    len(r.ambiguousAliases),
	}
}

// SearchRuntimeMetricsSnapshot returns process-local search quality counters.
func SearchRuntimeMetricsSnapshot() SearchRuntimeMetrics {
	return SearchRuntimeMetrics{
		Searches:                     dynamicSearchRuntimeCounters.searches.Load(),
		ZeroResultSearches:           dynamicSearchRuntimeCounters.zeroResultSearches.Load(),
		FuzzyFallbackSearches:        dynamicSearchRuntimeCounters.fuzzyFallbackSearches.Load(),
		AmbiguousAliasQueries:        dynamicSearchRuntimeCounters.ambiguousAliasQueries.Load(),
		LowConfidenceSearches:        dynamicSearchRuntimeCounters.lowConfidenceSearches.Load(),
		DestructiveFuzzySuppressions: dynamicSearchRuntimeCounters.destructiveFuzzySuppressions.Load(),
	}
}

// ResetSearchRuntimeMetrics clears process-local search quality counters.
func ResetSearchRuntimeMetrics() {
	dynamicSearchRuntimeCounters.searches.Store(0)
	dynamicSearchRuntimeCounters.zeroResultSearches.Store(0)
	dynamicSearchRuntimeCounters.fuzzyFallbackSearches.Store(0)
	dynamicSearchRuntimeCounters.ambiguousAliasQueries.Store(0)
	dynamicSearchRuntimeCounters.lowConfidenceSearches.Store(0)
	dynamicSearchRuntimeCounters.destructiveFuzzySuppressions.Store(0)
}

func recordSearchRuntimeMetrics(resultCount int, fuzzyUsed, ambiguousAlias, lowConfidence bool, destructiveFuzzySuppressions int) {
	dynamicSearchRuntimeCounters.searches.Add(1)
	if resultCount == 0 {
		dynamicSearchRuntimeCounters.zeroResultSearches.Add(1)
	}
	if fuzzyUsed {
		dynamicSearchRuntimeCounters.fuzzyFallbackSearches.Add(1)
	}
	if ambiguousAlias {
		dynamicSearchRuntimeCounters.ambiguousAliasQueries.Add(1)
	}
	if lowConfidence {
		dynamicSearchRuntimeCounters.lowConfidenceSearches.Add(1)
	}
	if destructiveFuzzySuppressions > 0 {
		dynamicSearchRuntimeCounters.destructiveFuzzySuppressions.Add(uint64(destructiveFuzzySuppressions))
	}
}

func searchIndexPostingCount(index searchIndex) int {
	count := 0
	for _, postings := range index.byToken {
		count += len(postings)
	}
	return count
}
