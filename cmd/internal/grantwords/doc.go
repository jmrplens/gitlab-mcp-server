// Package grantwords is the one wording of what a fine-grained personal access
// token needs for an action, in English and Spanish, for the two generators
// that print it.
//
// cmd/gen_action_grants writes the fine-grained permissions page, one row per
// action, and cmd/gen_tool_reference writes the same requirement on each
// action's entry of the per-domain tool reference, beside its tier and the
// scope a classic or OAuth token needs. A reader who meets an action on both
// pages has to read the same sentence on both, so the phrases and the
// functions that assemble them from a finegrained.Description live here
// rather than in either command: two copies would drift a word at a time, and
// a drift on the permissions page shows as churn on every one of its rows.
//
// Permission names, GraphQL selections and the elements GitLab names are left
// as GitLab spells them in every language, since a reader matches them
// against GitLab rather than reads them as prose. A boundary is written in the
// Spanish fine-grained tokens guide's word for it where it has one.
package grantwords
