package dynamic

import (
	"sort"
	"strings"
)

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
		// The singular of a plural is matched against the action ID alone
		// ([singularMatch]), so the ID buckets are the only ones it adds.
		index.addCandidates(candidates, index.byDomain[term.Singular])
		index.addCandidates(candidates, index.byAction[term.Singular])
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

// searchTerms normalizes query as normalizeSearchTerms does and reads two
// things about its terms that only the catalog's vocabulary can tell: the
// singular a plural names, and a verb that is the first word of a domain
// name. It then marks the branch, tag or commit a query that says what to do
// names ([markRefResources]), in either spelling, once the verbs that start a
// domain name are known to be none.
//
// A model writes a list query in the plural ("list issues", "branches list")
// and the catalog names its actions in the singular (issue.list,
// branch.list). Without the singular a plural reached neither the domain nor
// the action ID and matched only incidental text, such as an optional
// parameter called with_issues_enabled or the value branches of an enum, so
// the action whose schema mentioned the word outranked the action the query
// named (issue 1121). The singular is one rule for every noun rather than a
// synonym per plural, which is how the table used to answer a few of them.
//
// The singular is kept in [searchTerm.Singular] and out of the alternatives,
// because it stands for the plural only where the catalog spells it, in an
// action ID ([singularMatch]), and below the plural's own spelling. Every
// other reader of the alternatives leaves it out: tags and compound tags, the
// parameter signals, the intents that read the query's words, the verb and
// destructive-action gates, and the fuzzy pass, which has no typo to recover
// in a word the catalog spells. As an alternative it did not stay there: it
// formed compound tags with the words around it, so "list ssh keys" put the
// deploy key lists, tagged "ssh key", ahead of user.ssh_keys, and it fired
// intents a plural does not ask for, so "list service accounts" gave every
// group and project service account list the boost of the service account
// intent.
//
// A verb that the next term completes into a domain name, merge before
// request or requests, is the first word of that domain and no verb
// ([searchTerm.Noun]), and a pair written in the plural names the domain
// ([searchTerm.Domain]).
func (index searchIndex) searchTerms(query string) []searchTerm {
	terms := normalizeSearchTerms(query)
	for position := range terms {
		terms[position].Singular = index.singularOf(terms[position].Raw)
	}
	for position := 1; position < len(terms); position++ {
		domain, throughSingular := index.domainStartedBy(terms[position-1].Raw, terms[position])
		if domain == "" {
			continue
		}
		terms[position-1].Noun = true
		if throughSingular {
			terms[position-1].Domain = domain
			terms[position].Domain = domain
		}
	}
	markRefResources(terms)
	return terms
}

// domainStartedBy returns the domain name of this catalog that verb, a word
// the verb intent classifies, starts and next completes, and whether next
// completes it through its singular rather than as written; "" when there is
// none.
//
// Merge is the one such word today: "merge request" and "merge requests"
// name merge_request, and read as a workflow verb they made every query about
// merge requests a request to merge one. A verbless "merge requests" answered
// merge_request.merge, a destructive action, with high confidence, and "show
// merge requests" lost the read that would have made it a list. A domain
// whose first word is no verb reads as its domain already, and is left as it
// is.
func (index searchIndex) domainStartedBy(verb string, next searchTerm) (string, bool) {
	if classifyVerbIntent(verb) == "" {
		return "", false
	}
	// A domain word never holds an underscore, so a key that does is a whole
	// domain name.
	if _, found := index.byDomain[verb+"_"+next.Raw]; found {
		return verb + "_" + next.Raw, false
	}
	if _, found := index.byDomain[verb+"_"+next.Singular]; found {
		return verb + "_" + next.Singular, true
	}
	return "", false
}

// singularOf returns the singular of word that names a domain or action word
// of this catalog, or "" when word is not the plural of one.
//
// It undoes the three regular English plurals, "ies" for "y" (policies), a
// plain "s" (issues) and "es" (branches), in that order, and keeps the first
// candidate the catalog spells. That vocabulary is the gate: a word that only
// ends in "s", such as access, status or alias, has no singular because the
// catalog has no acces, statu or alia, and a plain "s" is tried before "es"
// so that notes reads as note even in a catalog that also spells not.
//
// The vocabulary is the words of the action IDs, the domains and the action
// names, rather than every indexed token, because the index also holds the
// words of schema descriptions, and there "its" has "it" and "is" has "i":
// a singular that short is a substring of most action IDs and would score
// against nearly all of them.
func (index searchIndex) singularOf(word string) string {
	for _, candidate := range singularCandidates(word) {
		if index.namesIdentifierWord(candidate) {
			return candidate
		}
	}
	return ""
}

// singularCandidates returns the singulars word would have under each regular
// English plural it ends with, in the order singularOf tries them.
func singularCandidates(word string) []string {
	candidates := make([]string, 0, 3)
	if stem, found := strings.CutSuffix(word, "ies"); found {
		candidates = append(candidates, stem+"y")
	}
	if stem, found := strings.CutSuffix(word, "s"); found {
		candidates = append(candidates, stem)
		if stem, found = strings.CutSuffix(stem, "e"); found {
			candidates = append(candidates, stem)
		}
	}
	return candidates
}

// namesIdentifierWord reports whether word is a domain or action word of the
// catalog, or a whole domain or action name.
func (index searchIndex) namesIdentifierWord(word string) bool {
	_, domain := index.byDomain[word]
	_, action := index.byAction[word]
	return domain || action
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

// searchDocumentIndexTokens returns every word the token index should carry
// for document. The accumulator is deliberately unsized: each value splits
// into an unknown number of words, so any capacity computed from the field
// counts is a guess, and dedupeStrings allocates the returned slice anyway.
func searchDocumentIndexTokens(document searchDocument) []string {
	var tokens []string

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
