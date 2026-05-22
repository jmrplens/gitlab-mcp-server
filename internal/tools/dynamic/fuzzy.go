package dynamic

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	// fuzzyMaxDistance and fuzzyMinTokenLen bound typo tolerance so short tokens
	// still require exact matches while longer tokens allow two edit mistakes.
	fuzzyMaxDistance         = 2
	fuzzyMinTokenLen         = 3
	fuzzyResourceSignalBoost = 20
)

type fuzzyCandidateMode int

const (
	fuzzyDisabled fuzzyCandidateMode = iota
	fuzzyZeroResults
	fuzzyLowConfidence
)

func buildSearchTokens(searchText string) []string {
	tokens := splitWordTokens(searchText)
	if len(tokens) == 0 {
		return nil
	}
	return dedupeStrings(tokens)
}

func fuzzyTokenScore(needle string, searchTokens []string) int {
	score, _, _, ok := fuzzyTokenScoreCore(needle, searchTokens)
	if !ok {
		return 0
	}
	return score
}

func fuzzyScoreEntry(entry actionEntry, terms []searchTerm) int {
	totalScore, matchedCount, minRequired, resourceBoost, _ := fuzzyScoreCore(entry, terms, false)
	if matchedCount == 0 || matchedCount < minRequired {
		return 0
	}

	return totalScore*matchedCount/len(terms) + resourceBoost
}

func fuzzyScoreEntryWithoutExplanation(entry actionEntry, terms []searchTerm) (int, ScoringExplanation) {
	return fuzzyScoreEntry(entry, terms), ScoringExplanation{}
}

func fuzzyTermMatchesResourceSignal(entry actionEntry, raw, alternative string) bool {
	document := documentForEntry(entry)
	return termMatchesResourceSignal(raw, document) || termMatchesResourceSignal(alternative, document)
}

func fuzzyScoreEntryWithExplanation(entry actionEntry, terms []searchTerm) (int, ScoringExplanation) {
	totalScore, matchedCount, minRequired, resourceBoost, reasons := fuzzyScoreCore(entry, terms, true)
	if matchedCount == 0 || matchedCount < minRequired {
		return 0, ScoringExplanation{}
	}

	score := totalScore*matchedCount/len(terms) + resourceBoost
	return score, ScoringExplanation{
		TotalScore:    score,
		MatchedTerms:  matchedCount,
		RequiredTerms: minRequired,
		Reasons:       reasons,
	}
}

type fuzzyTermMatch struct {
	score          int
	reason         MatchReason
	resourceSignal bool
}

func fuzzyScoreCore(entry actionEntry, terms []searchTerm, includeReason bool) (totalScore, matchedCount, minRequired, resourceBoost int, reasons []MatchReason) {
	if len(terms) == 0 {
		return 0, 0, 0, 0, nil
	}
	if includeReason {
		reasons = make([]MatchReason, 0, len(terms))
	}
	for _, term := range terms {
		match := bestFuzzyTermMatch(entry, term, includeReason)
		if match.score == 0 {
			continue
		}
		matchedCount++
		totalScore += match.score
		if match.resourceSignal {
			resourceBoost += fuzzyResourceSignalBoost
		}
		if includeReason {
			reasons = append(reasons, match.reason)
		}
	}

	minRequired = len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	return totalScore, matchedCount, minRequired, resourceBoost, reasons
}

func bestFuzzyTermMatch(entry actionEntry, term searchTerm, includeReason bool) fuzzyTermMatch {
	best := fuzzyTermMatch{}
	for _, alternative := range term.Alternatives {
		var candidateScore int
		candidateReason := MatchReason{}
		if includeReason {
			candidateScore, candidateReason = fuzzyTokenScoreWithReason(term.Raw, alternative, entry.SearchTokens)
		} else {
			candidateScore = fuzzyTokenScore(alternative, entry.SearchTokens)
		}
		if candidateScore > best.score {
			best.score = candidateScore
			best.reason = candidateReason
			best.resourceSignal = fuzzyTermMatchesResourceSignal(entry, term.Raw, alternative)
		}
	}
	return best
}

func fuzzyTokenScoreWithReason(raw, alternative string, searchTokens []string) (int, MatchReason) {
	score, bestMatchedValue, bestDistance, ok := fuzzyTokenScoreCore(alternative, searchTokens)
	if !ok {
		return 0, MatchReason{}
	}
	reason := MatchReason{
		Field:        searchFieldFuzzyToken,
		QueryTerm:    raw,
		MatchedValue: bestMatchedValue,
		Score:        score,
		Fuzzy:        true,
		Distance:     bestDistance,
	}
	if raw != alternative {
		reason.Alternative = alternative
	}
	return score, reason
}

func fuzzyTokenScoreCore(alternative string, searchTokens []string) (score int, bestMatchedValue string, bestDistance int, ok bool) {
	parts := splitWordTokens(alternative)
	if len(parts) == 0 || len(searchTokens) == 0 {
		return 0, "", 0, false
	}

	total := 0
	eligibleParts := 0
	bestOverallScore := -1
	for _, part := range parts {
		if len(part) < fuzzyMinTokenLen {
			continue
		}
		eligibleParts++
		partScore, partMatchedValue, partDistance, matched := bestFuzzyTokenMatch(part, searchTokens)
		if !matched {
			return 0, "", 0, false
		}
		if partScore > bestOverallScore {
			bestOverallScore = partScore
			bestMatchedValue = partMatchedValue
			bestDistance = partDistance
		}
		total += partScore
	}

	if total == 0 || eligibleParts == 0 {
		return 0, "", 0, false
	}
	return total / eligibleParts, bestMatchedValue, bestDistance, true
}

func bestFuzzyTokenMatch(part string, searchTokens []string) (score int, matchedValue string, distance int, ok bool) {
	partPrefix := firstRuneString(part)
	for _, token := range searchTokens {
		candidateScore, candidateDistance, matched := scoreFuzzyTokenCandidate(part, partPrefix, token)
		if matched && candidateScore > score {
			score = candidateScore
			matchedValue = token
			distance = candidateDistance
			ok = true
		}
	}
	return score, matchedValue, distance, ok
}

func scoreFuzzyTokenCandidate(part, partPrefix, token string) (score, distance int, ok bool) {
	if !comparableTokenLength(part, token) {
		return 0, 0, false
	}
	distance, withinThreshold := boundedLevenshtein(part, token, fuzzyMaxDistance)
	if !withinThreshold {
		return 0, 0, false
	}
	score = fuzzyDistanceScore(distance)
	if partPrefix != "" && strings.HasPrefix(token, partPrefix) {
		score += 2
	}
	return score, distance, true
}

func splitWordTokens(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func comparableTokenLength(needle, token string) bool {
	ln := utf8.RuneCountInString(needle)
	lt := utf8.RuneCountInString(token)
	diff := ln - lt
	if diff < 0 {
		diff = -diff
	}
	return diff <= fuzzyMaxDistance
}

func firstRuneString(value string) string {
	for _, r := range value {
		return string(r)
	}
	return ""
}

func fuzzyDistanceScore(distance int) int {
	switch distance {
	case 0:
		return 40
	case 1:
		return 34
	case 2:
		return 28
	default:
		return 0
	}
}

func boundedLevenshtein(a, b string, maxDistance int) (int, bool) {
	if a == b {
		return 0, true
	}
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		if len(br) <= maxDistance {
			return len(br), true
		}
		return 0, false
	}
	if len(br) == 0 {
		if len(ar) <= maxDistance {
			return len(ar), true
		}
		return 0, false
	}

	if len(ar) > len(br) {
		ar, br = br, ar
	}
	if len(br)-len(ar) > maxDistance {
		return 0, false
	}

	previous := make([]int, len(ar)+1)
	current := make([]int, len(ar)+1)
	for i := 0; i <= len(ar); i++ {
		previous[i] = i
	}

	for i := 1; i <= len(br); i++ {
		current[0] = i
		minInRow := current[0]
		brune := br[i-1]
		for j := 1; j <= len(ar); j++ {
			cost := 0
			if ar[j-1] != brune {
				cost = 1
			}

			deletion := previous[j] + 1
			insertion := current[j-1] + 1
			substitution := previous[j-1] + cost
			best := min(deletion, insertion, substitution)

			current[j] = best
			if best < minInRow {
				minInRow = best
			}
		}

		if minInRow > maxDistance {
			return 0, false
		}
		previous, current = current, previous
	}

	distance := previous[len(ar)]
	if distance > maxDistance {
		return 0, false
	}
	return distance, true
}
