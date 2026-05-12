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
	parts := splitWordTokens(needle)
	if len(parts) == 0 || len(searchTokens) == 0 {
		return 0
	}

	total := 0
	eligibleParts := 0
	for _, part := range parts {
		if len(part) < fuzzyMinTokenLen {
			continue
		}
		eligibleParts++

		best := 0
		for _, token := range searchTokens {
			if !comparableTokenLength(part, token) {
				continue
			}

			distance, ok := boundedLevenshtein(part, token, fuzzyMaxDistance)
			if !ok {
				continue
			}

			score := fuzzyDistanceScore(distance)
			if strings.HasPrefix(token, firstRuneString(part)) {
				score += 2
			}
			if score > best {
				best = score
			}
		}

		if best == 0 {
			return 0
		}
		total += best
	}

	if total == 0 || eligibleParts == 0 {
		return 0
	}
	return total / eligibleParts
}

func fuzzyScoreEntry(entry actionEntry, terms []searchTerm) int {
	if len(terms) == 0 {
		return 0
	}

	totalScore := 0
	matchedCount := 0
	resourceBoost := 0
	for _, term := range terms {
		best := 0
		resourceSignal := false
		for _, alternative := range term.Alternatives {
			score := fuzzyTokenScore(alternative, entry.SearchTokens)
			if score > best {
				best = score
				resourceSignal = fuzzyTermMatchesResourceSignal(entry, term.Raw, alternative)
			}
		}
		if best > 0 {
			matchedCount++
			totalScore += best
			if resourceSignal {
				resourceBoost += fuzzyResourceSignalBoost
			}
		}
	}

	if matchedCount == 0 {
		return 0
	}

	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if matchedCount < minRequired {
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
	if len(terms) == 0 {
		return 0, ScoringExplanation{}
	}

	totalScore := 0
	matchedCount := 0
	reasons := make([]MatchReason, 0, len(terms))
	for _, term := range terms {
		best := 0
		var bestReason MatchReason
		for _, alternative := range term.Alternatives {
			score, reason := fuzzyTokenScoreWithReason(term.Raw, alternative, entry.SearchTokens)
			if score > best {
				best = score
				bestReason = reason
			}
		}
		if best > 0 {
			matchedCount++
			totalScore += best
			reasons = append(reasons, bestReason)
		}
	}

	if matchedCount == 0 {
		return 0, ScoringExplanation{}
	}

	minRequired := len(terms)
	if len(terms) > 2 {
		minRequired = len(terms) - 1
	}
	if matchedCount < minRequired {
		return 0, ScoringExplanation{}
	}

	score := totalScore*matchedCount/len(terms) + fuzzyResourceSignalBoostFor(entry, reasons)
	return score, ScoringExplanation{
		TotalScore:    score,
		MatchedTerms:  matchedCount,
		RequiredTerms: minRequired,
		Reasons:       reasons,
	}
}

func fuzzyResourceSignalBoostFor(entry actionEntry, reasons []MatchReason) int {
	document := documentForEntry(entry)
	boost := 0
	for _, reason := range reasons {
		if termMatchesResourceSignal(reason.MatchedValue, document) || termMatchesResourceSignal(reason.Alternative, document) {
			boost += fuzzyResourceSignalBoost
		}
	}
	return boost
}

func fuzzyTokenScoreWithReason(raw, alternative string, searchTokens []string) (int, MatchReason) {
	parts := splitWordTokens(alternative)
	if len(parts) == 0 || len(searchTokens) == 0 {
		return 0, MatchReason{}
	}

	total := 0
	eligibleParts := 0
	bestMatchedValue := ""
	bestDistance := 0
	for _, part := range parts {
		if len(part) < fuzzyMinTokenLen {
			continue
		}
		eligibleParts++

		best := 0
		matchedValue := ""
		distanceValue := 0
		for _, token := range searchTokens {
			if !comparableTokenLength(part, token) {
				continue
			}

			distance, ok := boundedLevenshtein(part, token, fuzzyMaxDistance)
			if !ok {
				continue
			}

			score := fuzzyDistanceScore(distance)
			if strings.HasPrefix(token, firstRuneString(part)) {
				score += 2
			}
			if score > best {
				best = score
				matchedValue = token
				distanceValue = distance
			}
		}

		if best == 0 {
			return 0, MatchReason{}
		}
		if best > total || bestMatchedValue == "" {
			bestMatchedValue = matchedValue
			bestDistance = distanceValue
		}
		total += best
	}

	if total == 0 || eligibleParts == 0 {
		return 0, MatchReason{}
	}
	score := total / eligibleParts
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
