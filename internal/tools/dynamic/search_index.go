package dynamic

import "sort"

// buildSearchIndex constructs the core lookup index used by dynamic search.
// It keeps parallel alias/domain/action/token maps so candidate selection can
// narrow quickly across multiple query dimensions before scoring.
func buildSearchIndex(entries []actionEntry) searchIndex {
	index := searchIndex{
		byToken:  make(map[string][]int),
		byAlias:  make(map[string][]int),
		byDomain: make(map[string][]int),
		byAction: make(map[string][]int),
		all:      make([]int, 0, len(entries)),
	}
	for entryIndex, entry := range entries {
		index.all = append(index.all, entryIndex)
		document := documentForEntry(entry)
		index.addValues(index.byAlias, document.Aliases, entryIndex)
		index.addValues(index.byDomain, []string{document.Domain}, entryIndex)
		index.addValues(index.byAction, []string{document.Action}, entryIndex)
		index.addValues(index.byToken, searchDocumentIndexTokens(document), entryIndex)
	}
	return index
}

// candidateEntryIndexes returns indexed candidates for normalized terms. When
// no index bucket matches, it intentionally falls back to a copy of index.all
// so downstream ranking can still evaluate the full catalog deterministically.
func (index searchIndex) candidateEntryIndexes(terms []searchTerm) []int {
	if len(index.all) == 0 {
		return nil
	}
	if len(terms) == 0 {
		return append([]int(nil), index.all...)
	}

	candidates := make(map[int]struct{})
	for _, term := range terms {
		for _, alternative := range term.Alternatives {
			index.addCandidates(candidates, index.byAlias[alternative])
			index.addCandidates(candidates, index.byDomain[alternative])
			index.addCandidates(candidates, index.byAction[alternative])
			index.addCandidates(candidates, index.byToken[alternative])
		}
	}
	if len(candidates) == 0 {
		return append([]int(nil), index.all...)
	}

	entryIndexes := make([]int, 0, len(candidates))
	for entryIndex := range candidates {
		entryIndexes = append(entryIndexes, entryIndex)
	}
	sort.Ints(entryIndexes)
	return entryIndexes
}

// addValues indexes each deduplicated full value plus its split word tokens.
// dedupeStrings avoids repeated work, and appendEntryIndex keeps entry lists
// stable while preventing adjacent duplicates.
func (index searchIndex) addValues(target map[string][]int, values []string, entryIndex int) {
	for _, value := range dedupeStrings(values) {
		target[value] = appendEntryIndex(target[value], entryIndex)
		for _, word := range splitSearchFieldWords(value) {
			target[word] = appendEntryIndex(target[word], entryIndex)
		}
	}
}

func (index searchIndex) addCandidates(candidates map[int]struct{}, entryIndexes []int) {
	for _, entryIndex := range entryIndexes {
		candidates[entryIndex] = struct{}{}
	}
}

// appendEntryIndex deduplicates only adjacent duplicates; callers rely on
// monotonic index construction so the same entryIndex can only repeat
// consecutively for a given posting list.
func appendEntryIndex(entryIndexes []int, entryIndex int) []int {
	if len(entryIndexes) > 0 && entryIndexes[len(entryIndexes)-1] == entryIndex {
		return entryIndexes
	}
	return append(entryIndexes, entryIndex)
}

func searchDocumentIndexTokens(document searchDocument) []string {
	tokens := make(
		[]string, 0,
		10+
			len(document.IDWords)+
			len(document.DomainWords)+
			len(document.ActionWords)+
			len(document.Aliases)+
			len(document.Tags)+
			len(document.RequiredParams)+
			len(document.SchemaProperties),
	)

	for _, value := range []string{
		document.Backend,
		document.Capability,
		document.Resource,
		document.Operation,
		document.Scope,
		document.CanonicalID,
		document.Tool,
		document.Domain,
		document.Action,
		document.FlatText,
	} {
		tokens = append(tokens, splitSearchFieldWords(value)...)
	}

	for _, values := range [][]string{
		document.IDWords,
		document.DomainWords,
		document.ActionWords,
		document.Aliases,
		document.Tags,
		document.RequiredParams,
		document.SchemaProperties,
	} {
		for _, value := range values {
			tokens = append(tokens, splitSearchFieldWords(value)...)
		}
	}
	return dedupeStrings(tokens)
}
