// Package edition defines the GitLab licensing tier model used to gate tool
// availability across the MCP server.
//
// GitLab licensing has three nested tiers — Free ⊂ Premium ⊂ Ultimate. A given
// instance exposes the union of features available at its tier. The MCP server
// uses [Tier] both for the resolved instance tier (configured or detected) and,
// in later phases, for the minimum required tier of each action.
//
// The zero value of [Tier] is [Free], the most conservative tier.
package edition

import "strings"

// Tier is a GitLab licensing tier. Tiers are totally ordered and nested:
// Free < Premium < Ultimate. The zero value is [Free].
type Tier int

const (
	// Free is the GitLab Free/CE tier. It is the zero value and the
	// conservative fallback when the instance tier cannot be determined.
	Free Tier = iota
	// Premium is the GitLab Premium tier (a superset of Free).
	Premium
	// Ultimate is the GitLab Ultimate tier (a superset of Premium).
	Ultimate
)

// String returns the lowercase canonical name of the tier.
func (t Tier) String() string {
	switch t {
	case Premium:
		return "premium"
	case Ultimate:
		return "ultimate"
	case Free:
		return "free"
	default:
		return "free"
	}
}

// AtLeast reports whether t is at least as high as other in the tier ordering
// (Free < Premium < Ultimate). It is used to decide whether an instance at tier
// t satisfies a minimum required tier.
func (t Tier) AtLeast(other Tier) bool { return t >= other }

// IsEnterprise reports whether t is an Enterprise (Premium or Ultimate) tier.
// This derives the legacy binary "enterprise" notion from the 3-tier model so
// existing positional gating continues to behave identically.
func (t Tier) IsEnterprise() bool { return t >= Premium }

// ParseTier maps a configuration string to a [Tier]. It is case-insensitive and
// trims surrounding whitespace. Accepted values:
//
//   - "free", "ce"  → [Free]
//   - "premium"     → [Premium]
//   - "ultimate"    → [Ultimate]
//
// The empty string and any other value are reported as unrecognized via ok=false;
// callers decide how to handle that (e.g. fall back to detection or [Free]).
func ParseTier(s string) (tier Tier, ok bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "free", "ce":
		return Free, true
	case "premium":
		return Premium, true
	case "ultimate":
		return Ultimate, true
	default:
		return Free, false
	}
}

// TierFromPlan maps a GitLab license plan name (as returned by the License API's
// Plan field) to a [Tier]. The mapping follows GitLab's tier model, including
// legacy plan names:
//
//   - "premium", "starter", "bronze", "silver" → [Premium]
//   - "ultimate", "gold"                        → [Ultimate]
//   - anything else (incl. "free", "", unknown) → [Free]
//
// Legacy paid plans (starter/bronze/silver) predate the Free/Premium/Ultimate
// rename; they are mapped to [Premium] because their feature set corresponds to
// the modern Premium tier. "gold" was the legacy name for Ultimate.
func TierFromPlan(plan string) Tier {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "premium", "starter", "bronze", "silver":
		return Premium
	case "ultimate", "gold":
		return Ultimate
	default:
		return Free
	}
}
