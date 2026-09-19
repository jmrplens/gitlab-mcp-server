// Package actionids is the one reader of the canonical action IDs this
// repository publishes, and the one rule for spotting one inside prose.
//
// Two gates ask the same question of two corpora. `cmd/audit_action_ids` asks
// it of the source: the RelatedActions list of an ActionSpec, the first
// argument of toolutil.HintAction, and the dotted IDs a Usage line or an
// individual tool's Description spells. `cmd/audit_doc_tool_names` asks it of
// the documentation: the dotted IDs a page teaches a reader to call. What
// "resolves" means has to be one answer for both, or a spelling the docs gate
// accepts is one the source gate refuses and a reader cannot tell which is
// right.
//
// # The oracle
//
// [Build] assembles the catalog this tree builds, at the Ultimate tier, twice:
// once against a self-managed stub instance and once against GitLab.com. The
// union is the answer. Orbit registers only for GitLab.com, so a single
// self-managed build reports its six IDs as dead; taking the union rather than
// the GitLab.com build alone means an action gated the other way would not
// read as dead either. The standalone dynamic actions are added the way
// cmd/server adds them, because gitlab_execute_action takes those IDs too.
//
// # A canonical ID and a registered alias are kept apart
//
// [IDs.IsID] answers for the canonical IDs alone; [IDs.Alias] answers for a
// spelling that resolves to one. gitlab_execute_action resolves an alias, so a
// cross-link naming one works when it is followed; gitlab_find_action
// publishes canonical IDs, so a model that looks that name up in a listing
// does not find it. Which of the two a caller demands is the caller's
// decision, and both gates demand the canonical ID.
//
// # Spotting an ID inside prose
//
// [IDs.Candidates] picks the dotted tokens of a sentence that are offered as
// action IDs. The shape alone is far too generous, so a token qualifies only
// when either half is one the catalog uses: a known domain, or a known action
// name. Requiring the domain alone is blind to the commonest defect, since the
// usual wrong spelling keeps the action and invents the domain: work_item.get
// and pipeline_schedule.list are both real actions under domains the catalog
// does not have. Requiring neither half would admit every example.com and
// go.mod in the tree. Requiring either half turns those away and keeps the
// invented domain.
package actionids
