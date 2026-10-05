# Error Handling

**How a failure is turned into something a model can act on**: the
classification, the wrapping functions a handler calls, the informational
not-found result, and how to test them.

> **Diátaxis type**: Reference & Explanation · **Audience**: 🛠️ Contributors & maintainers

What a caller receives when a call fails, with examples, is on the site under
[Error Handling](https://jmrp.io/docs/gitlab-mcp-server/operations/error-handling/).
The decision is [ADR-0007](adr/adr-0007-rich-error-semantics.md), and the
short version every handler author needs is in the
[Development Guide](development.md#error-handling-in-tool-handlers). This page
is the code behind both. Everything below lives in `internal/toolutil/` unless
it names another package.

| File                                    | Purpose                                                                                                                                                                                                           |
| --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/toolutil/errors.go`           | `WrapErr`, `WrapErrWithMessage`, `WrapErrWithHint`, `WrapErrWithStatusHint`, `ExtractGitLabMessage`, `ClassifyError`, `ClassifyHTTPStatus`, `IsHTTPStatus`, `IsPermissionRefusal`, `ContainsAny`, `SanitizeError` |
| `internal/gitlab/credential_refusal.go` | `UnauthorizedNamesCredential` and `RefusalMayBePermission`, the two readings of a refusal that the description, the hints and the HTTP pool share                                                                 |
| `internal/gitlab/granular_refusal.go`   | `ParseGranularRefusal`, which reads GitLab's fine-grained token refusals                                                                                                                                          |
| `internal/toolutil/not_found.go`        | `NotFoundResult`, `ActionRoute.WrapNotFound` and `ParamText`, the informational 404 for get handlers                                                                                                              |
| `internal/toolutil/confirm.go`          | The destructive action confirmation flow                                                                                                                                                                          |
| `internal/toolutil/output.go`           | `SuccessResult`, `ErrorResult`, `ErrorResultAnnotated`                                                                                                                                                            |

---

## Classification

### `ClassifyError`

Inspects the error chain and returns the sentence that describes it, checking
in this order:

| Error                                | Description                                                                                             |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------- |
| `nil`                                | "unknown error"                                                                                         |
| `context.Canceled`                   | "the request was canceled by the client"                                                                |
| `context.DeadlineExceeded`           | "the request exceeded its deadline and was canceled"                                                    |
| `gitlabclient.ErrUnboundClient`      | `UnattributedRequestMessage`: the request could not be attributed to a credential and was not sent      |
| `gitlabclient.ErrDestinationRefused` | `DestinationRefusedMessage`: this server refused to connect to that address                             |
| A fine-grained token refusal         | The refusal in GitLab's terms (below)                                                                   |
| A GitLab answer, REST or GraphQL     | `ClassifyHTTPStatus` of the status, except a 401 that names the credential (below)                      |
| Connection refused                   | "GitLab server is unreachable (connection refused). Check GITLAB_URL and whether the server is running" |
| DNS failure                          | "GitLab server hostname could not be resolved (DNS error). Check GITLAB_URL"                            |
| Timeout                              | "Request to GitLab timed out. The server may be overloaded or unreachable"                              |
| TLS or certificate error             | "TLS/SSL handshake failed. If using self-signed certificates, set GITLAB_MCP_SKIP_TLS_VERIFY=true"      |
| Another `*url.Error`                 | "network error reaching GitLab (\<op\>)"                                                                |
| Anything else                        | "unexpected error"                                                                                      |

Cancellation is checked first because a cancelled request often surfaces as a
transport failure underneath, which every later branch would describe as
something GitLab did. The unbound client and the refused destination are
checked before the network branches, which would otherwise describe a request
that never left the process as an unreachable host.

A 404 is one of GitLab's answers although client-go hands it back without the
response: it answers every 404 with its `ErrNotFound` sentinel, over REST as it
is and over GraphQL wrapped in the query error, and the sentinel records the
status alone. `ClassifyError` reads the status off the error when it carries no
response, so a 404 on either surface is described as not found. client-go
returns a GraphQL refusal as `*gl.GraphQLResponseError`, which keeps the
response in a field and does not unwrap to it; `gitLabResponseOf` is the one
way this file finds the response, and every reader (`ClassifyError`,
`IsHTTPStatus`, `ExtractGitLabMessage`, the sanitizer) goes through it, so
they cannot disagree about a GraphQL refusal.

### `ClassifyHTTPStatus`

Maps a status to actionable guidance; any other status reads "GitLab returned
HTTP \<code\>".

| Code | Description                                                                                                                                                                                                                                                                              |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 400  | "bad request: check your input parameters"                                                                                                                                                                                                                                               |
| 401  | "unauthorized: either the token (GITLAB_TOKEN) is invalid or expired, or it is valid and lacks a permission this action needs, since some GitLab endpoints answer a missing permission with 401 rather than 403. If the token works for other calls, treat this as a permission refusal" |
| 403  | "access denied: your token lacks the required permissions. This can mean: (1) missing API scope on the token, (2) insufficient project role (some operations require Maintainer or Owner), or (3) the feature is restricted by instance admin settings"                                  |
| 404  | "not found: the requested resource does not exist, you lack access, or the feature requires a higher GitLab tier. Verify the ID/path is correct"                                                                                                                                         |
| 405  | "method not allowed: the action cannot be performed on this resource in its current state"                                                                                                                                                                                               |
| 409  | "conflict: the resource already exists or there is a state conflict"                                                                                                                                                                                                                     |
| 422  | "validation failed: GitLab rejected the request due to invalid data"                                                                                                                                                                                                                     |
| 429  | "rate limited: too many requests, please wait before retrying"                                                                                                                                                                                                                           |
| 500  | "GitLab internal server error: the server encountered an unexpected condition"                                                                                                                                                                                                           |
| 502  | "GitLab is temporarily unavailable (bad gateway): try again shortly"                                                                                                                                                                                                                     |
| 503  | "GitLab is under maintenance or overloaded (service unavailable): try again shortly"                                                                                                                                                                                                     |

### Why a 401 names two causes

GitLab answers 401 for two different things. Its API guard answers it for a
credential it cannot use, and at a family of REST routes GitLab's own API
helper `unauthorized!` answers it for a **valid** credential that lacks a
permission: merging, cancelling auto-merge, approving and resetting
approvals, adding to a merge train, remote mirrors, access token reads, lists
and rotation, external status checks, security settings, group SAML links,
award emoji removal, group updates and fork links. Approving a merge request
you opened, on an instance that prevents approval by the author, is the
common one. The upstream half is
[entry 55 of the upstream bugs register](upstream-bugs.md#a-permission-refusal-is-answered-401-rather-than-403).

The status cannot tell the two apart, so `ClassifyHTTPStatus(401)` names both
and ends with the test that separates them. It opens with "unauthorized"
rather than "authentication failed", because for a permission refusal
authentication succeeded.

`ClassifyError` has the whole response and narrows the answer in one
direction only. It describes a 401 as a rejected credential ("authentication
failed: GitLab rejected the token (GITLAB_TOKEN) itself as invalid, expired,
revoked or without the api or read_api scope, so renew or replace it") when:

- the body carries the RFC 6750 code `invalid_token`, which GitLab's REST API
  guard writes for an expired, revoked or impersonation-disabled token and
  nothing else in the REST API writes;
- the GraphQL endpoint answered it. That endpoint answers 401 only from its
  authentication checks, with `{"errors":[{"message":"Invalid token"}]}` and
  no code, and refuses a field the caller may not see with a 200, so a GraphQL
  401 has no permission refusal to be confused with. One of those checks is
  the scope: the endpoint authenticates a token only when it carries `api` or
  `read_api`, which is why the sentence names the scope. The endpoint is
  recognized by the request path as sent, escaped, so a REST path parameter
  that decodes to `api/graphql` does not pass for it.

The opposite verdict cannot be read off a response: a token GitLab has no
record of at all is answered through `unauthorized!` too, byte for byte like a
permission refusal, so a REST 401 without the code keeps the sentence that
names both causes.

The rule is not `ClassifyError`'s own. It is `UnauthorizedNamesCredential` in
`internal/gitlab/credential_refusal.go`, and it has a second reader: in HTTP
mode the client's innermost transport applies it to every 401 GitLab answers a
request the SDK makes, and the credential pool ends a caller's pooled entry at
once only when it says the credential was refused. A 401 it cannot place is
confirmed with `GET /api/v4/user` first, and the entry is kept when GitLab
still accepts the token. One rule is what keeps the two from disagreeing, so
a permission refusal is never described to a model as a missing permission
while the pool treats it as a revoked token.

### Why a fine-grained refusal names its permission

A fine-grained personal access token carries no scopes, only a grant of named
permissions fixed when it is created, and GitLab judges every call against
that grant after the token authenticates. Its refusal is not the 403 above,
whose missing scope, project role and admin setting are none of them what is
missing, so `ClassifyError` reads it first and describes it in GitLab's terms.
GitLab writes the refusal in one service
(`Authz::Tokens::AuthorizeGranularScopesService` at v19.4.1-ee) and in four
texts, and `gitlabclient.ParseGranularRefusal` reads each one whole:

| GitLab's text                                                                                                                                   | What `ClassifyError` says                                                                                                                                                                                                                                                                                                     |
| ----------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| "Access denied: This operation requires a fine-grained personal access token with the following project permissions: [Merge Request: Approve]." | The call needs those permissions, quoted as GitLab listed them; a grant cannot be changed after it is created, so the way out is a new fine-grained token that grants them, or a classic token where the group does not refuse one. A classic token refused with this text is under a group that requires fine-grained tokens |
| "Access denied: This operation doesn't support fine-grained personal access tokens."                                                            | GitLab declares no fine-grained permission for the operation, so no fine-grained token can call it on this instance; the way out is a classic token                                                                                                                                                                           |
| "Access denied: Fine-grained personal access tokens are not yet supported."                                                                     | Fine-grained tokens are not enabled for the token's user (GitLab's `granular_personal_access_tokens` feature flag); the way out is a classic token or the instance's administrator                                                                                                                                            |
| "404 Not Found", as a GraphQL `errors[]` entry                                                                                                  | What the call names was not found, or is outside what the token may see                                                                                                                                                                                                                                                       |

Over REST the text is the `error_description` of a 403 whose code is
`insufficient_granular_scope`, and the code is the evidence: a text this
server cannot read is still described as the fine-grained refusal it is. Over
GraphQL a mutation outside the grant answers 200 with the field `null` and the
text as one `errors[]` entry, which reaches the errors layer through
`GraphQLTopLevelError`, through client-go's `*gl.GraphQLResponseError`, or
joined with "; " by client-go's achievement and scan profile services; each
entry is read whole, and "404 Not Found" only from an entry, never from a
joined message, where client-go's own 404 sentinel carries the same words.
GitLab's generic GraphQL refusal ("The resource that you are attempting to
access does not exist or you don't have permission to perform this action"),
which an undeclared mutation answers, is left as it was, since GitLab answers
every token with those words wherever a permission is missing.

The description reaches a model whichever route the error took: the wrapping
functions put it in front of the cause, and `SanitizeError`, which the meta,
individual and standalone dispatchers call, puts it in front of an error a
handler returned without wrapping, and never twice. No role hint follows a
refusal of the token's grant: `IsPermissionRefusal` is false for all of them,
and `WrapErrWithHint` (so `WrapErrWithStatusHint` too) drops the handler's
hint from such a refusal and composes the error as `WrapErrWithMessage` does.

A GraphQL mutation's refusal reaches the errors layer only when the handler
reads the top-level `errors[]` it arrives in. The payload of a refused
mutation is `null`, and a handler that decodes the payload as a value reads
that as the zero payload, which is a success. So a mutation handler decodes
the payload as a pointer beside the top-level entries, returns
`GraphQLTopLevelError` when the payload is absent, and an error naming the
missing payload when GitLab gave no reason either.

---

## Wrapping in a handler

A handler returns its typed output and a Go error; the dispatcher turns the
error into the result:

- **Success**: `return out, nil`
- **Read** (list, get, search): `return zero, toolutil.WrapErr("listIssues", err)`
- **Write** (create, update, delete): `return zero, toolutil.WrapErrWithMessage("createIssue", err)`

All four wrapping functions first catch an unattributed request and a refused
destination, which they describe with their own messages.

### `WrapErr`

For reads. Classifies the error and prefixes the operation:

```go
err := toolutil.WrapErr("list_issues", originalErr)
// For an expired token: "list_issues: authentication failed: GitLab rejected the token (GITLAB_TOKEN) itself
// as invalid, expired, revoked or without the api or read_api scope, so renew or replace it: <original>"
```

### `ExtractGitLabMessage`

Extracts GitLab's own detail from the response in the error chain, over REST
and GraphQL alike. It drops a message that only restates the status
(`405 Method Not Allowed`, or the wrapped `{message: 405 Method Not Allowed}`)
and a body that is not a GitLab message at all, flattens what is left onto one
line, since GitLab's messages quote input an attacker may have chosen, and
caps it at 300 characters:

```go
msg := toolutil.ExtractGitLabMessage(err)
// "A file with this name already exists"
// "[title is too long (maximum is 255 characters)]"
// "" when no useful detail is available
```

### `WrapErrWithMessage`

For writes, where GitLab's detail tells the model what went wrong. Adds that
detail in parentheses when there is one:

```go
err := toolutil.WrapErrWithMessage("fileCreate", originalErr)
// "fileCreate: bad request: check your input parameters (A file with this name already exists): <original>"
```

### `WrapErrWithHint`

Like `WrapErrWithMessage`, with a suggestion appended, when the corrective
action is known. A hint names an action by its canonical ID:

```go
if toolutil.IsHTTPStatus(err, http.StatusConflict) {
    return toolutil.WrapErrWithHint("branchProtect", err,
        "protected branch rule already exists, use branch.get_protected to view current rules")
}
// "branchProtect: conflict: the resource already exists or there is a state conflict
//  (Protected branch rule already exists). Suggestion: protected branch rule already exists,
//  use branch.get_protected to view current rules: <original>"
```

### `WrapErrWithStatusHint`

The common single-status case in one call: `WrapErrWithHint` when the error
carries the status, `WrapErrWithMessage` otherwise.

```go
return toolutil.WrapErrWithStatusHint("issueGet", err, http.StatusNotFound,
    "verify issue_iid with issue.list")
```

A handler that needs a different hint per status uses a `switch` over
`IsHTTPStatus` instead. GraphQL error sites mostly use `WrapErrWithHint`,
because an error GitLab reports inside a 200 carries no status to match. A
GraphQL refusal GitLab answers with an error status does carry one, and
`IsHTTPStatus` reads it through `*gl.GraphQLResponseError`, so
`WrapErrWithStatusHint` attaches its hint there as it does over REST. A 404
never arrives in that type: client-go answers it with the `ErrNotFound`
sentinel, and a status hint on 404 matches through the sentinel.

### A permission hint on a 401

A hint that names a role, a license or an owner is keyed on
`IsPermissionRefusal(err)`, never on a status alone. At the routes
[entry 55](upstream-bugs.md#a-permission-refusal-is-answered-401-rather-than-403)
lists, GitLab refuses a missing permission with 401, so a hint scoped to 403
is never shown there, and a hint keyed on 401 alone would follow the verdict
that GitLab rejected the token itself, which it then contradicts.
`IsPermissionRefusal` is true for a REST 401 or 403 whose body carries no RFC
6750 error code, which is what GitLab's API helpers `unauthorized!`,
`forbidden!` and `render_api_error!` render, and false for every GraphQL
answer and for both ways the API guard refuses the credential rather than the
call: its token and scope refusals carry a code (`invalid_token`,
`dpop_error` and `restricted_language_server_client_error` on a 401,
`insufficient_scope` and `insufficient_granular_scope` on a 403), and an
account the API will not serve (blocked, deactivated, pending approval, the
Terms of Service not accepted and the like) is refused with a plain 403 whose
message is one of the fixed reasons in GitLab's
`lib/gitlab/auth/user_access_denied_reason.rb`. The rule is
`RefusalMayBePermission` in `internal/gitlab/credential_refusal.go`, beside
the rule that decides the description, so the two cannot drift apart.

```go
if toolutil.IsPermissionRefusal(err) {
    return toolutil.WrapErrWithHint("mrMerge", err,
        "merging needs the right to push to the merge request's target branch (read it with branch.get_protected)")
}
return toolutil.WrapErrWithMessage("mrMerge", err)
```

It is a predicate and not another wrapper on purpose: `make check-action-ids`
reads a hint where it is passed to `WrapErrWithHint`, `WrapErrWithStatusHint`
or `NotFoundResult`, so a hint passed through a new wrapper would escape the
gate unless the wrapper takes it under a parameter named for a hint.

A route whose 403 means something other than the permission its 401 refuses
pairs the predicate with `IsHTTPStatus`. The external status check routes of
a merge request answer the license with 401 and the caller's role with 403;
the fork link answers the target namespace with 401 and the caller's
abilities with 403; the security settings answer the role with 401 and the
license, an archived project or an instance-enforced setting with 403, so
that handler reads the 403 first. Two kinds of action are not keyed on the
predicate: the four group SAML link actions add their hint to every error,
because the licensed end-to-end suite quotes that wording, and the two
self-rotations keep a 401 hint about the calling token, which agrees with the
verdict that the token was refused.

### Every sentence a model reads names actions by their canonical ID

A hint is not the only prose a model reads. The message of an error a handler
returns, a refusal `toolutil.ErrorResult` answers with, the next steps a
formatter writes, a spec's `Usage` line and parameter guidance, and every
schema description reach it too, on every surface. A `gitlab_*` tool name in
any of them is right for one surface of three, so each names an action by its
canonical ID (`project.list`, not `gitlab_project_list`):

```go
return Output{}, errors.New("commitCreate: project_id is required. Use project.list to find the ID first, then pass it as project_id")
```

`make check-action-ids` reads them and fails on a tool name, an alias or an
ID that resolves nowhere. Where it may name a tool, and what it does not read,
is in [Command-Line Utilities](cmd-utilities.md#the-rule-over-served-prose).

### Which function

| Scenario                                | Function                                  | Example                                                                          |
| --------------------------------------- | ----------------------------------------- | -------------------------------------------------------------------------------- |
| Read (list, get, search)                | `WrapErr`                                 | `WrapErr("listBranches", err)`                                                   |
| Get that returned 404                   | `NotFoundResult`, through `WrapNotFound`  | `NotFoundResult("Branch", "main in project 42", hints...)`                       |
| Write (create, update, delete)          | `WrapErrWithMessage`                      | `WrapErrWithMessage("fileCreate", err)`                                          |
| A known corrective action               | `WrapErrWithHint`                         | `WrapErrWithHint("branchDelete", err, "use branch.unprotect first")`             |
| A hint for one status (the common case) | `WrapErrWithStatusHint`                   | `WrapErrWithStatusHint("issueGet", err, 404, "verify issue_iid")`                |
| A permission refused with 401 or 403    | `IsPermissionRefusal` + `WrapErrWithHint` | `if toolutil.IsPermissionRefusal(err) { WrapErrWithHint("mrMerge", err, hint) }` |

---

## Not-found as an informational result

A get handler's 404 is not returned as a Go error. The route is wrapped with
`ActionRoute.WrapNotFound`, which turns a 404 into a typed not-found output
returned with a nil error, and the package registers a formatter that turns
that output into the result:

```go
// In the domain's action_specs.go, wrapping the get route.
route := toolutil.RouteAction(client, Get).WrapNotFound(func(params map[string]any) any {
    branchName, _ := params["branch_name"].(string)
    return branchNotFoundOutput{Identifier: fmt.Sprintf("%q in project %s", branchName, toolutil.ParamText(params["project_id"]))}
})

// In the domain's markdown.go, registered from init().
func formatBranchNotFound(out branchNotFoundOutput) *mcp.CallToolResult {
    return toolutil.NotFoundResult("Branch", out.Identifier,
        toolutil.HintAction("branch.list", "list the project's branches"),
        "Verify the branch name is spelled correctly (case-sensitive)",
    )
}
toolutil.RegisterMarkdownResult(formatBranchNotFound)
```

`WrapNotFound` hands the builder the arguments as the handler read them, with
the documented parameter aliases already resolved, and `ParamText` renders a
value as the caller wrote it (a JSON number whole rather than in the exponent
form `%v` gives a `float64` of a million or more). `NotFoundResult` builds a
Markdown result with `IsError: true`, a `## ❓ {Resource} Not Found` heading
with the identifier, and the hints as next steps. Because the route returns a
nil error, the call is logged at INFO rather than ERROR, and
`LogToolCallAll` reads `IsError` off the result so the record still says the
call failed.

The pattern is applied through one shared formatter in each of 22 domains:
award emoji, badges, branches, dependency firewall, deployments, environments,
files, groups, group service accounts, labels, merge requests, milestones,
Orbit, packages, pipelines, projects, project service accounts, releases,
snippets, tags, users and wikis. A domain's formatter covers every get
variant it has, which is why the typed output carries the identifier rather
than the formatter hardcoding it.

## Result hygiene

An error result is written for a model to correct itself from, not to expose
the request. It carries the operation, the classification, the status,
GitLab's own message when it adds something, and the hint. The cause it wraps
renders the request line GitLab refused (`METHOD scheme://host/path: status`),
so a value the call put in the path is repeated there; the query string and
the request body are not copied into it. The sanitizer bounds what GitLab
authored: a REST
message and the `errors[].message` list client-go appends to a GraphQL error
are each flattened onto one line and capped at 300 characters, and the list is
dropped altogether when the body carries a top-level key other than `data`,
`errors` and `extensions`, since GitLab did not compose that body.

## Network helpers

| Helper                | Detects                                 |
| --------------------- | --------------------------------------- |
| `isConnectionRefused` | ECONNREFUSED at any depth of the chain  |
| `isDNSError`          | A `*net.DNSError` in the chain          |
| `isTimeout`           | Any error implementing `Timeout() bool` |
| `isTLSError`          | A TLS or certificate error              |
| `ContainsAny`         | A substring match on `err.Error()`      |

## Parameter names a model mistypes

Two mechanisms work together when a model sends the wrong argument name:

1. **Unknown keys are refused.** `strictUnmarshal` in
   `internal/toolutil/meta_tool.go` decodes the `params` envelope with
   `json.Decoder.DisallowUnknownFields()`, after the reserved meta keys (such
   as `confirm`) are stripped, so a key no field of the input struct maps to
   is an immediate `json: unknown field "foo"` rather than a value silently
   dropped and defaulted.
2. **Required values name the exact parameter.**

| Helper                         | Use                         | Message                                                                                                                                        |
| ------------------------------ | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| `ErrRequiredInt64(op, field)`  | A required int64 field is 0 | "\<op\>: \<field\> is required (must be > 0). Ensure you use the exact parameter name '\<field\>' as documented in the tool description"       |
| `ErrRequiredString(op, field)` | A required string is empty  | "\<op\>: \<field\> is required (must be non-empty). Ensure you use the exact parameter name '\<field\>' as documented in the tool description" |

They are used where models confuse names: `milestone_id` and
`milestone_iid`, `branch` and `branch_name`, `iid` and `merge_request_iid`.

## Types nothing in production builds

`ToolError` (tool, message, status code), `DetailedError` (domain, action,
message, details, GitLab status, request ID, with a Markdown rendering),
`NewDetailedError` and `ErrorResultMarkdown` are defined in `errors.go` and
used only by its tests. The wrapping functions above are the path every
handler takes. Whether to wire these in or delete them is an open decision;
until then, do not build on them.

## Destructive action confirmation

The flow in `confirm.go` (the YOLO switch, an explicit `confirm`, elicitation,
then failing closed) and the dynamic surface's exception are described in
[Internal Architecture](architecture.md#destructive-action-confirmation). A
declined or cancelled confirmation is a `CancelledResult`, an error result,
not a Go error.

## Testing error handling

`testutil.NewTestClient` builds its client with retries disabled
(`DisableRetries`), so a mock may answer any status, 500 included, and the
handler sees it at once. A test that builds a client another way keeps
client-go's retries (up to five, with backoff) on a 5xx and sits through them;
build it with `gitlabclient.NewClientWithTokenRetries(..., true)` or through
`NewTestClient`.

```go
testutil.RespondJSON(w, http.StatusBadRequest, map[string]string{
    "message": "A file with this name already exists",
})
```

Assert the classification and GitLab's message in the error text, and for a
hinted path the canonical action ID the hint names.

## Hint coverage

Counted on 2026-10-05 over the non-test Go files under `internal/`, with
`internal/toolutil` itself excluded:

| Call                    | Sites                               |
| ----------------------- | ----------------------------------- |
| `WrapErrWithStatusHint` | 869                                 |
| `WrapErrWithHint`       | 378                                 |
| `WrapErrWithMessage`    | 365                                 |
| `WrapErr`               | 261                                 |
| `NotFoundResult`        | 22, one shared formatter per domain |

164 of the 180 packages under `internal/tools` carry at least one hint, in 193
files. A `WrapErrWithMessage` that stays unhinted is usually one whose error
did not come from GitLab: input validation (`ErrFieldRequired`,
`ErrRequiredInt64`, `ErrRequiredString`), body parsing (`json.Unmarshal`,
`io.ReadAll`, `os.ReadFile`, base64 decoding), time parsing, building a
request locally (`NewRequest`) and a cancelled context.
