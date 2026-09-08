// Package provenance holds the one age verdict the commands that pin an
// external truth pass on their committed record.
//
// Three of them do it. [github.com/jmrplens/gitlab-mcp-server/v3/cmd/gen_api_shapes]
// commits what GitLab's own generated OpenAPI document says each REST
// operation accepts and returns, cmd/gen_api_exposes commits the condition
// under which each field a REST entity exposes is sent, and
// cmd/gen_graphql_schema commits the GraphQL schema this repository validates
// every raw document against. Each is a copy of something that lives in
// gitlab-org/gitlab and keeps moving there; each is written by a network run
// that cannot gate, and read by a --check that can precisely because it needs
// no network; and each therefore carries the day it was taken, because a
// record's only defense against the drift it cannot see is to say how long it
// has been standing.
//
// That last decision is what this package holds, and nothing else: the
// retrieval date's arithmetic ([Age], [Days]), the default clock the check
// halves share ([Clock]), the three ways a date stops a record being one a
// gate can rest on ([Problems]), and the one window with its one recorded
// reason ([MaxAge]). The window is the reason it is a package rather than
// three tidy copies. It was chosen once, for a fact about GitLab's release
// cadence rather than about any of the three records, and it was written down
// three times with three copies of the reason and a comment in one of them
// pointing at its twin; three constants is how a shared window drifts apart
// from why it was picked, and a fourth pinned truth would have copied it
// again.
//
// What stays with each command is everything that makes its record its own:
// its Source type and the fields it records, the floor below which the
// artifact is a truncated download rather than a whole one
// (apishapes.MinimumOperations, apiexposes.MinimumEntities,
// graphqlintrospect.MinimumTypes), the identity checks that ask what the
// record is a record of, its artifact dialect, its make targets and its
// binary. [Subject] carries the two words that differ between the three
// messages, so the sentences stay each command's own while the decision
// behind them is one.
//
// Two neighbors are deliberately not members. cmd/gen_request_inventory
// commits no provenance at all: its -check is byte equality against a fresh
// recording of the unit suite, so there is no date to judge and no window to
// share. cmd/audit_graphql_documents does not own a record either; it reads
// the pinned schema's, and calls [Age] only to say how long ago the pin was
// taken in a drift report, where an unreadable date is left silent rather
// than turned into a second complaint about a field the gate beside it
// already refuses.
package provenance
