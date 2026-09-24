// Command audit_sdk_context reports every call into client-go that does not
// hand the SDK the caller's context.
//
// # Why this needs a rule of its own
//
// client-go takes the context of a request only as a request option,
// gl.WithContext(ctx), and builds the request from context.Background()
// without one. A handler that forgets the option compiles, answers correctly
// and passes every test that does not cancel mid-flight, while three things
// the context carries never reach the request it sends: the action deadline
// (GITLAB_MCP_ACTION_TIMEOUT, applied through the context by the WrapAction
// functions), the cancellation of an abandoned HTTP POST, and the parent of
// the outbound trace span, so the request starts a trace of its own and the
// call it belongs to reads as having issued none. Fourteen calls in four
// packages were in that state when this rule was written, after the same
// defect had already been fixed once in the uploads package.
//
// Neither linter that sounds like it covers this can see it. noctx knows the
// standard library's request constructors, and contextcheck follows
// parameters whose type is context.Context; a context carried as a variadic
// RequestOptionFunc is neither.
//
// # The rules
//
// Every call whose callee takes client-go request options, as a variadic
// `...RequestOptionFunc` or as a `[]RequestOptionFunc` the way the request
// builders do, has to pass the context. The callee is read from the type
// checker's signature, so a service method, a method value kept in a variable
// or a struct field, a function-typed parameter and a helper of this
// repository's own are all judged alike, and client-go's import name does not
// matter. A call passes when one of its options is:
//
//   - gl.WithContext of a context that can end;
//   - a variable that was ever assigned one, which covers a slice initialized
//     with gl.WithContext and appended to on a branch, and a single option
//     held in a variable;
//   - a request-option parameter of the function the call sits in, or of any
//     function around it, handed on untouched. That is forwarding, and whoever
//     calls that function is judged by the same rule instead.
//
// A request built by hand through NewRequest, NewRequestToURL or
// UploadRequest also passes when the variable holding it is rebound through
// the request's own WithContext method before it is sent: `req =
// req.WithContext(ctx)`, or `client.Do(req.WithContext(ctx), &out)`.
//
// gl.WithContext(context.Background()) and context.TODO() count as no
// context at all, and so does the same on the request's own method: either
// compiles, reads like the fix and bounds nothing. Only the direct call is
// recognized, since a command's own context is usually derived from
// context.Background() by a timeout before it is used.
//
// What the walk cannot trace is treated as carrying nothing, so the call is
// reported rather than passed: options kept in a struct field, built by a
// function or a method, or handed over as another call's results. The answer
// is to pass gl.WithContext(ctx) beside them.
//
// # Declarations
//
// A function allowed to call client-go without the caller's context is
// declared in declarations.go, keyed `package:Func` or `package:Type.Method`,
// with a category and a reason. A declaration that excuses nothing is
// reported like every stale declaration in this repository, scoped to the
// packages the run loaded, and so is one naming a category nobody defined.
// The table is empty.
//
// # What it reads, and what it cannot see
//
// ./internal/... and ./cmd/..., loaded through cmd/internal/goprogram without
// test files. That leaves two things out, and both are stated here rather than
// discovered later.
//
// Test files are the blind spot, the end-to-end scenario packages under
// test/e2e/gitlab included, since those are test files in their entirety. A
// test may build a request with no context on purpose, to show what the
// cancellation fixture does with one, and holding tests to this rule would be
// a second rule with a declaration table of its own. The rest of test/e2e,
// the harness and the fixture library, is outside the patterns: it drives a
// real instance from a test process and bounds its own requests.
//
// A file a build constraint leaves out of the load is not read either, and
// that one is not silent: any such file that imports client-go is reported as
// unjudged and fails the gate, since every call in it went unread. None does
// today.
//
// A call through a value whose type is a type parameter has no signature the
// type checker can name and is not judged. None exists in the tree.
//
// Usage:
//
//	go run ./cmd/audit_sdk_context/          # report
//	go run ./cmd/audit_sdk_context/ -check   # fail on a finding (CI gate)
//	go run ./cmd/audit_sdk_context/ -v       # also list what a declaration excuses
//	go run ./cmd/audit_sdk_context/ ./internal/tools/packages  # one package
package main
