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
