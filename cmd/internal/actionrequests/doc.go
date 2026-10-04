// Package actionrequests answers, for a catalog action, which functions run
// when it is called and what those functions put on the wire.
//
// It is the handler resolver cmd/audit_readonly_graphql was built on, taken
// out of that command because a second reader needs the same answer: the
// derivation of which requests each action makes, which is what decides the
// fine-grained permissions an action needs (issue 952). One reader of one fact
// is the rule cmd/internal/actionids and cmd/internal/graphqldocs were created
// for, and two copies of a resolver would drift apart exactly where a gate
// needs them to agree.
//
// # From an action to its handlers
//
// [Program.Sites] resolves every ActionSpec construction in the loaded source
// to the action name it declares, the individual tool name its options
// declare, and the functions its route runs. The resolution is a small
// interprocedural constant propagation, because the specs are written in many
// shapes: most domains call a package-local helper that forwards the name, the
// route and the options to a toolutil constructor as parameters, and the
// resolver follows the parameters rather than matching one spelling.
//
// [Match] joins a site to a catalog action ([Catalog]) on the canonical ID,
// through the tool name where the name and the package do not decide. They do
// not for the project and group badge actions, which one package declares
// under one set of names, for the two security settings updates and for the
// license template read the admin specs declare again; joined on name and
// owner alone, each of those fifteen actions was handed both sites and both
// handlers.
//
// # From handlers to what they reach
//
// [Program.Roots] turns the matched handlers into the functions a walk starts
// from, and [Program.Reachable] walks what they call, with no depth bound and
// a cycle guard. Three shapes needed more than a call graph of declared
// functions:
//
//   - A handler written as a function literal is indexed as a function of its
//     own, so what it names directly is part of what the action does.
//   - A literal that calls a parameter of the route helper it was written in
//     (awardEmojiDeleteRoute wraps the delete it was handed) calls whatever the
//     helper's caller bound to that parameter, which the frame the literal was
//     resolved in records.
//   - A package-level variable's initializer is indexed as a function of its
//     own, so a test seam holding a literal (the group boards' newRawRequest),
//     a dispatch table of literals (the integrations' getters) and a method
//     expression (commits' newRequest) are reached by the bodies that name them.
//
// Every indexed body records the GraphQL documents it names (by text, since
// what a document asks for is each reader's question), whether it reaches the
// GraphQL transport, and the client-go service methods it names, keyed the way
// cmd/internal/sdkroutes keys the request each one sends.
//
// What it does not do yet is derive a raw request's path, classify a request
// as unconditional or one of several alternatives, or join anything to
// GitLab's own declarations; those belong to the generator that reads this
// package for the fine-grained permission table.
package actionrequests
