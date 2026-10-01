// Package sdkroutes is the one reader of which request each client-go service
// method sends: the HTTP verb and the path for a REST method, and the GraphQL
// documents for one that posts to the GraphQL endpoint.
//
// Two readers ask it. R-PATH (`cmd/audit_1to1/internal/paths`) asks which
// endpoints answer with a client-go struct, which endpoints one method reaches,
// and which endpoints a method given an option struct sends it to; the action
// request derivation (`cmd/internal/actionrequests`) asks which requests a
// handler's client-go call puts on the wire. Each used to read client-go on its
// own, or would have, and two readers of one fact are how a gate ends up
// answering a narrower question than the one it prints.
//
// # What the reading follows
//
// It parses the module's root package rather than type-checking it, because
// the question is textual and every route template client-go declares is a
// package-level variable registered through route(), which client-go's own test
// suite enforces. What it follows beyond the method's own body is what an
// earlier reader of the same source missed, each measured on the tree:
//
//   - A method answering with nothing but the pagination wrapper (most deletes)
//     is kept. Its routes are a fact about the method whatever it returns.
//   - A method that delegates is followed into the function it delegates to: a
//     method of the same service (s.getAwardEmoji), a method of a helper held in
//     a field (s.timeStats.addSpentTime), and a package function, generic ones
//     included (listMarkdownUploads[T]). The one package function never followed
//     is do, the request machinery every option-form method shares, whose path
//     is whatever withPath set and so is read from there.
//   - String constants flow into the template. A whole segment that is a format
//     verb takes the constant the caller passed, so the award emoji helper
//     called with "issues" reaches /projects/:/issues/:/award_emoji and the one
//     called with "snippets" reaches /projects/:/snippets/:/award_emoji, where
//     reading the template alone gave both the same /projects/:/:/:/award_emoji,
//     a path GitLab does not have.
//   - A path built with fmt.Sprintf for the legacy NewRequest form is folded:
//     the format with every verb replaced by what its argument folds to,
//     through local assignments, concatenation and a string helper of the same
//     package (the generic package routes are built by FormatPackageURL). A
//     verb that is reassigned on the request afterwards (req.Method =
//     http.MethodPut) is the verb the request carries.
//
// A segment that holds anything the reading cannot name becomes the placeholder
// ":", which is client-go's own normalization of a route template, so the
// shapes produced here meet the ones its route registry reports.
//
// # What it cannot read
//
// A legacy request whose path folds to nothing static (a parameter passed
// through whole, a field of a struct) is recorded on the method as unresolved
// rather than as a wrong route, and a reader decides what an unresolved method
// means to it. A directory that cannot be read, and a file that does not parse,
// contribute nothing: this reads a module cache whose state it does not own.
package sdkroutes
