// Package metadata reports discovery-metadata gaps (R-META)
// across the canonical ActionSpec catalog. For every action it flags:
//
//   - generic_usage: a placeholder Usage string such as "Use to execute X domain
//     action." (or an empty Usage) instead of a purpose-specific sentence.
//   - aliases_only_toolname: no natural-language aliases beyond the canonical
//     action name and the projected individual-tool name.
//   - empty_related: no RelatedActions cross-links.
//   - missing_individual_description: the individual-tool surface has no
//     "Returns: … See also: …" Description.
//
// The output is the R-META slice of the 1:1 backlog, grouped by owner package.
package metadata
