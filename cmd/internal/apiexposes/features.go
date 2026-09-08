package apiexposes

import (
	"regexp"
	"strings"
)

// featureList opens one of the table's symbol arrays: `NAME = %i[`.
var featureList = regexp.MustCompile(`^\s*([A-Z_]+FEATURES[A-Z_]*)\s*=\s*%i\[\s*$`)

// featureSymbol is one symbol a condition asks a license about, in the three
// spellings the entities use.
var featureSymbol = regexp.MustCompile(`(?:licensed_feature_available\?|feature_available\?)\(\s*:([a-z0-9_]+)`)

// ParseFeatures reads GitLab's feature table, mapping every licensed feature
// symbol to the tier whose list it appears in. A symbol listed under several
// tiers, which the table does not do, would keep the highest.
//
// The table's lists are named by tier: the STARTER lists are premium, since
// GitLab folded that tier into Premium and kept the list; the GLOBAL list is
// every paid plan. Lists the table builds by concatenation (ALL_PREMIUM_FEATURES
// and the like) are not arrays in the source and are not read.
func ParseFeatures(src []byte) map[string]string {
	tiers := map[string]string{}
	current := ""
	for raw := range strings.SplitSeq(string(src), "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if current == "" {
			if match := featureList.FindStringSubmatch(raw); match != nil {
				current = listTier(match[1])
			}
			continue
		}
		if strings.HasPrefix(line, "]") {
			current = ""
			continue
		}
		for symbol := range strings.FieldsSeq(line) {
			if rank(current) > rank(tiers[symbol]) {
				tiers[symbol] = current
			}
		}
	}
	return tiers
}

// listTier is the tier a list's name stands for, or "" for a list that is
// none of them and is skipped.
func listTier(name string) string {
	switch {
	case strings.Contains(name, "ULTIMATE"):
		return TierUltimate
	case strings.Contains(name, "PREMIUM"), strings.Contains(name, "STARTER"):
		return TierPremium
	case strings.Contains(name, "GLOBAL"):
		return TierGlobal
	default:
		return ""
	}
}

// rank orders tiers so the highest one a symbol appears under wins.
func rank(tier string) int {
	switch tier {
	case TierUltimate:
		return 3
	case TierPremium:
		return 2
	case TierGlobal:
		return 1
	default:
		return 0
	}
}

// licensedFeatures lists the feature symbols a condition asks a license about,
// in order of appearance, without repeats.
func licensedFeatures(condition string) []string {
	var symbols []string
	seen := map[string]bool{}
	for _, match := range featureSymbol.FindAllStringSubmatch(condition, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			symbols = append(symbols, match[1])
		}
	}
	return symbols
}

// tierOf is the highest tier the table puts one of the symbols under.
func tierOf(symbols []string, features map[string]string) string {
	tier := ""
	for _, symbol := range symbols {
		if candidate := features[symbol]; rank(candidate) > rank(tier) {
			tier = candidate
		}
	}
	return tier
}
