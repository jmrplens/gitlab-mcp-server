package dynamic

import "sort"

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

func (index searchIndex) addValues(target map[string][]int, values []string, entryIndex int) {
	for _, value := range dedupeStrings(values) {
		if value == "" {
			continue
		}
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

func appendEntryIndex(entryIndexes []int, entryIndex int) []int {
	if len(entryIndexes) > 0 && entryIndexes[len(entryIndexes)-1] == entryIndex {
		return entryIndexes
	}
	return append(entryIndexes, entryIndex)
}

func searchDocumentIndexTokens(document searchDocument) []string {
	values := []string{
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
	}
	values = append(values, document.IDWords...)
	values = append(values, document.DomainWords...)
	values = append(values, document.ActionWords...)
	values = append(values, document.Aliases...)
	values = append(values, document.Tags...)
	values = append(values, document.RequiredParams...)
	values = append(values, document.SchemaProperties...)

	tokens := make([]string, 0, len(values))
	for _, value := range values {
		tokens = append(tokens, splitSearchFieldWords(value)...)
	}
	return dedupeStrings(tokens)
}
