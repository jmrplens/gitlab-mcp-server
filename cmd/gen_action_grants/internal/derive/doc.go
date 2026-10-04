// Package derive answers, for every catalog action, which requests its
// handler can put on the wire and how they combine.
//
// It is the first of the three facts cmd/gen_action_grants joins, and the one
// this repository owns: GitLab's half is what each request demands of a
// fine-grained token, recorded in the live record, and the join is the
// generator's. What it reads is cmd/internal/actionrequests' resolution of an
// action to its handlers and the bodies that reach, cmd/internal/sdkroutes'
// reading of which request each client-go method sends, and the
// //gitlab:request directives a handler's author writes beside a request.
//
// # From a body to a structure
//
// Each reached body is read as a structure of sequences, alternatives and
// optional parts, built from its syntax, because which of an action's
// requests are needed on every call is the question a fine-grained grant is
// judged on and a flat set cannot answer it. The defaults:
//
//   - A request outside any branch runs on every call.
//   - The arms of an if with an else, of a switch, of a type switch, and the
//     entries of a composite literal holding several functions (a dispatch
//     table), are alternatives; so are two or more functions handed to one
//     call, which is how a helper is given the variants it chooses between.
//   - A request inside an if with no else, a loop body, a select, or the right
//     side of && or || may not run, and is optional.
//   - A construct one of whose arms ends in a return takes the rest of the
//     block as the arm that did not return: `if final { return }; poll()`
//     makes poll an alternative to returning, not a request of every call.
//     A label changes nothing about it. A return is read where it ends an
//     arm and not deeper: one nested in an inner construct of an arm that
//     does not itself end the function ends only that inner construct, so a
//     request after the outer construct reads as running on every path
//     through the arm, the early one included, which a directive answers.
//   - An arm that sends nothing and returns an error is a failed call, not a
//     way the action runs, so it is not an alternative of its own: an error
//     check after a request does not make the request optional.
//   - A function literal runs where it is written.
//   - A client-go method that can send several routes sends one of them per
//     call (the label methods pick by name or by ID), and one that posts
//     several GraphQL documents posts all of them (a work item delete reads
//     the item's ID, then deletes it).
//
// Where the syntax is wrong, the author says so beside the request, and the
// directive applies to every action that reaches the statement it is written
// above:
//
//	//gitlab:request optional: <why this request may not run>
//	//gitlab:request mandatory: <why every request here runs on every call>
//	//gitlab:request alternatives: <why exactly one of the requests here runs>
//
// # From a structure to paths
//
// An action's structure is expanded, through every function it calls, into
// its paths: the sets of requests one way of running it makes, minimal, so a
// path that holds another is dropped. A request in every path is mandatory,
// one in some is an alternative, and one in none is optional. What each
// request is, where it came from and which directive shaped it is kept, for
// the committed record of what this server sends and for the gates that hold
// the derivation to an author's intent.
package derive
