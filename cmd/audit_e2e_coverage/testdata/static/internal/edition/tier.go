// Package edition mirrors the real tier model's constants, in the same order,
// so the fake harness's Tier takes the same values the static gate compares.
package edition

// Tier is a licensing tier, ordered Free < Premium < Ultimate.
type Tier int

const (
	// Free is the unlicensed tier.
	Free Tier = iota
	// Premium is the first licensed tier.
	Premium
	// Ultimate is the highest tier.
	Ultimate
)
