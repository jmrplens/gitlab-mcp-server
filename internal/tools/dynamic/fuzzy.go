package dynamic

import (
	"strings"
	"unicode"
)

const (
	fuzzyMaxDistance = 2
	fuzzyMinTokenLen = 3
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
	for _, part := range parts {
		if len(part) < fuzzyMinTokenLen {
			continue
		}

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
			if score == 0 {
				continue
			}
			if strings.HasPrefix(token, part[:1]) {
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

	if total == 0 {
		return 0
	}
	return total / len(parts)
}

func fuzzyScoreEntry(entry actionEntry, terms []searchTerm) int {
	if len(terms) == 0 {
		return 0
	}

	totalScore := 0
	matchedCount := 0
	for _, term := range terms {
		best := 0
		for _, alternative := range term.Alternatives {
			score := fuzzyTokenScore(alternative, entry.SearchTokens)
			if score > best {
				best = score
			}
		}
		if best > 0 {
			matchedCount++
			totalScore += best
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

	return totalScore * matchedCount / len(terms)
}

func splitWordTokens(value string) []string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func comparableTokenLength(needle, token string) bool {
	ln := len(needle)
	lt := len(token)
	diff := ln - lt
	if diff < 0 {
		diff = -diff
	}
	return diff <= fuzzyMaxDistance
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
	if len(a) == 0 {
		if len(b) <= maxDistance {
			return len(b), true
		}
		return 0, false
	}
	if len(b) == 0 {
		if len(a) <= maxDistance {
			return len(a), true
		}
		return 0, false
	}

	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b)-len(a) > maxDistance {
		return 0, false
	}

	previous := make([]int, len(a)+1)
	current := make([]int, len(a)+1)
	for i := 0; i <= len(a); i++ {
		previous[i] = i
	}

	for i := 1; i <= len(b); i++ {
		current[0] = i
		minInRow := current[0]
		bi := b[i-1]
		for j := 1; j <= len(a); j++ {
			cost := 0
			if a[j-1] != bi {
				cost = 1
			}

			deletion := previous[j] + 1
			insertion := current[j-1] + 1
			substitution := previous[j-1] + cost
			best := deletion
			if insertion < best {
				best = insertion
			}
			if substitution < best {
				best = substitution
			}

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

	distance := previous[len(a)]
	if distance > maxDistance {
		return 0, false
	}
	return distance, true
}
