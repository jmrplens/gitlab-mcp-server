// Package finegrained holds what a fine-grained personal access token needs
// for each catalog action, and the decision this server takes on it.
//
// Three facts with three owners meet here (issue 952). Which requests an
// action makes is this repository's, derived from the handlers by
// cmd/gen_action_grants; what each REST route and GraphQL element demands of
// a fine-grained token is GitLab's, recorded from a booted instance in
// docs/development/gitlab-api-live.json; and how an action's requests combine
// is the handler author's, written beside the request as a //gitlab:request
// directive. The generator joins the three into one [Table] per recorded
// GitLab version, emitted as Go in internal/tools/actiongrants and compiled
// into the binary, and the types of that table are the ones declared here.
//
// # Two levels, because a wire request and a GraphQL position differ
//
// An [Operation] is one wire request: a REST route or one GraphQL operation.
// GitLab judges every request on its own and carries no authority from one to
// the next, so one execution of an action needs every operation it makes, and
// a [Requirement] is reachable when every operation of some path passes. An
// [Element] is one object position a GraphQL document selects: GitLab checks a
// fine-grained token per object type it resolves, so whether a GraphQL read
// works is decided position by position inside one operation, and a position
// the grant fails either takes the answer with it (it lies on the answer
// spine, or a non-null chain carries its null there) or leaves one field
// empty (it is degraded).
//
// # The decision
//
// An [Authority] is what one credential is worth, in one of two phases.
// Without a grant, phase A ([Unevaluated]), it withholds exactly the actions
// no fine-grained token can reach at the recorded GitLab version (a
// [Requirement] with a [Denial]) and allows the rest; an action the table has
// no row for is unknown authority and allowed, never locked out, since a
// stale table or a newer action is not evidence against the caller. With the
// grant read, phase B ([Evaluate]), it lists an action when the grant alone
// reaches it and lets it be called when the grant or GitLab's anonymous
// access to a public object can, so a call is refused here only where GitLab
// would refuse it whatever the object. [Judge] decides which of the two a
// [Reading] is worth, falling back to phase A with its reason when the
// instance is not a recorded release, the grant could not be read or names a
// permission the table does not know; [Rejudge] is the same decision for a
// re-read, which replaces the authority only on reads that answered.
// [Authority.Decide] and [Authority.Lists] are what the listing and the call
// read; [Authority.WithheldText] writes the one sentence every surface
// answers a withheld action with, naming the permission a phase B refusal
// needs, and [Authority.DegradedNote], [Authority.NullNote] and
// [Authority.EmptyNote] the next steps a served answer is given where GitLab
// leaves part of it empty, answers null or removes items over GraphQL without
// saying so.
//
// # A leaf beside the register
//
// The package imports the standard library and internal/tenancy alone, so
// internal/gitlab can hold an [Authority] and the e2e harness can call the
// same decision the server takes. What one granted scope covers is a policy
// rule, so it lives in the register as tenancy.CoverableAt (row AUT-008) and
// the vocabulary it is written in with it; the types here alias them. Every
// slice of a [Table] holds constants only, which lets the Go compiler lay a
// generated table out statically: the binary runs no initialization code for
// it.
package finegrained
