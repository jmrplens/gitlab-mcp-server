# Capabilities and Icons

**How the server implements the MCP capabilities, and how a handler uses
them**: the progress tracker, the elicitation flow, the completion handler, and
how the icons are built and attached.

> **Diátaxis type**: Reference · **Audience**: 🛠️ Contributors & maintainers

What each capability does for a client, which clients support it, and the
wire shapes are on the documentation site, under
[Capabilities](https://jmrp.io/docs/gitlab-mcp-server/capabilities/overview/)
and [Icons](https://jmrp.io/docs/gitlab-mcp-server/capabilities/icons/). This
page is the Go side. Resource subscriptions have their decisions in
[ADR-0015](adr/adr-0015-polled-resource-subscriptions.md) and their invariants
in [CLAUDE.md](../../CLAUDE.md) under "Resource subscriptions".

---

## How the capabilities are declared

`newServerShell` in `cmd/server/main.go` builds the capabilities before the
server, because the handlers travel in `mcp.ServerOptions`:

```go
serverCapabilities := &mcp.ServerCapabilities{
    Tools:     &mcp.ToolCapabilities{ListChanged: true},
    Resources: &mcp.ResourceCapabilities{ListChanged: true},
}
if capabilitySurface == config.CapabilitySurfaceFull {
    serverCapabilities.Prompts = &mcp.PromptCapabilities{ListChanged: true}
}
if subscribeHandler != nil {
    serverCapabilities.Resources.Subscribe = true
}

server := mcp.NewServer(&mcp.Implementation{
    Name:  "gitlab-mcp-server",
    Icons: toolutil.IconBrand,
    // Title, Description, Version, WebsiteURL
}, &mcp.ServerOptions{
    Capabilities:                serverCapabilities,
    CompletionHandler:           /* completions.NewHandler(client).Complete */,
    ProgressNotificationHandler: /* logs a client's progress at debug */,
    SubscribeHandler:            subscribeHandler,
    UnsubscribeHandler:          unsubscribeHandler,
    // Instructions, Logger, KeepAlive and the rest
})
```

Setting the subscribe handlers is what makes the SDK advertise
`resources.subscribe`. The bit is also stated explicitly, because the SDK only
derives it once a resource has been registered, and the handshake is answered
while registration is still in flight. Both handlers are nil on
`GITLAB_MCP_CAPABILITY_SURFACE=minimal`, which registers no GitLab resources to
subscribe to.

`full` also advertises `prompts` and registers the full resource and prompt
catalog. `minimal` omits the prompt capability while leaving tool execution,
completions and progress available, and still registers `gitlab://tools` and
`gitlab://tools/{id}`.

Elicitation is a client capability: the server declares nothing for it, and a
handler checks for it per call through the helpers below.

## Design principles

- **Zero-value safety.** `progress.Tracker` and `elicitation.Client` are value
  types whose zero values are safe no-ops, so a handler never checks for nil.
- **Graceful degradation.** A client without a capability gets an informational
  result or no notifications, never a crash: a tracker with no token sends
  nothing, and a guided flow on a client that cannot prompt names the action
  that takes every field in one call.
- **Error isolation.** A failed progress notification is logged at debug level
  and never aborts the operation it reports on.
- **Checked answers.** The typed prompts parse their answers against what
  they asked for (a selection against its options, a number for NaN and
  infinities), and `Flow.GatherData` checks a structured answer against the
  schema that asked for it, so a malformed answer reaches a handler as an
  error rather than as a value (`ErrMalformedAnswer` from `Confirm` and from
  `Flow.GatherData`). `Client.GatherData` returns the
  content unexamined, which is one more reason a handler uses `Flow`.

---

## Progress (`internal/progress`)

### Creating a tracker

```go
tracker := progress.FromRequest(req)
```

`FromRequest` returns a `Tracker` bound to the request's session and progress
token. It returns an inactive tracker, whose every method is a no-op, when the
request carries no progress token, has no session, or its session was never
initialized.

### Methods

| Method                                  | Signature                                     | Purpose                                                                                  |
| --------------------------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `IsActive()`                            | `() bool`                                     | Whether the tracker can send notifications                                               |
| `Update(ctx, progress, total, message)` | `(context.Context, float64, float64, string)` | Send progress with explicit values; drops a non-monotonic update (logged at debug)       |
| `Step(ctx, step, total, message)`       | `(context.Context, int, int, string)`         | Report the 1-based step of N; `Step(ctx, 1, 3, msg)` sends `progress=0, total=3`         |
| `Done(ctx, total, message)`             | `(context.Context, float64, string)`          | Send a final notification with `progress == total`; a no-op when `total` is not positive |
| `OnScale(base, outerTotal)`             | `(float64, float64) Tracker`                  | A tracker sharing this one's state that places a sub-step's numbers in the outer measure |

`Update` returns early when the context is done. Starting `Step` at zero is
this project's convention, not a protocol rule: the specification only
requires that `progress` increase, and allows a fraction.

### One tracker per call

The specification requires the `progress` value of every notification within
a request to strictly increase. `Update` enforces it: an update whose value is
less than or equal to the last one sent is dropped. It holds its mutex across
the send, so concurrent callers cannot reorder notifications on the wire.

The guard is per tracker and the invariant is per token. A tool call has one
progress token, so it must have one tracker: two built from the same request
each get their own counter, neither sees the other, and their notifications
interleave into a sequence that goes backwards. That happened:
`publish_directory` counted files while the `Publish` it called counted bytes,
and a client watching one token saw `200000` followed by `1`, with `total`
alternating between the two meanings. A handler that delegates therefore
passes its tracker down rather than letting the callee build one, and hands
it on through `OnScale` when the two levels measure in different units.

### Step-based progress

The pattern the guided flows use, and the only place `Step` is called:

```go
tracker := progress.FromRequest(req)
tracker.Step(ctx, 1, 4, "Collecting issue details...")
// ... prompt the user ...
tracker.Step(ctx, 2, 4, "Gathering optional fields...")
// ... prompt the user ...
tracker.Step(ctx, 3, 4, "Confirming creation...")
// ... prompt the user ...
tracker.Step(ctx, 4, 4, "Creating the issue...")
```

Each flow ends with `Done`, so the bar reaches 100% once the object exists.
Outside the flows, progress is reported by the project upload (`uploads`,
counting bytes through `toolutil.NewProgressReader`), the package publish
actions (`packages`, byte-counted through `OnScale`), the wait actions that
poll until a pipeline or job settles (`pipelines`, `jobs`, through
`waitpoll`), and the project and group transfers while they wait for GitLab
to apply the move (`waitpoll.Until`). A new handler calls
`progress.FromRequest(req)` at its start and reports between its significant
operations.

---

## Elicitation (`internal/elicitation`)

### Two wire mechanisms, one flow API

How a prompt travels depends on the protocol revision the client declared:
synchronous `elicitation/create` requests before 2026-07-28, and multi-round
trip requests (MRTR) from 2026-07-28, where the tool result carries an
`inputRequests` map and the client retries the call with `inputResponses`.
Which revision decides, and why it is the declared one rather than the
negotiated one, is on the site under
[Elicitation](https://jmrp.io/docs/gitlab-mcp-server/capabilities/elicitation/).
`elicitation.Flow` chooses the mechanism per request, so a handler is written
once:

```go
flow, err := elicitation.FlowFromRequest(req)
if err != nil { /* malformed client-echoed state */ }
title, err := flow.PromptText(ctx, "title", "Enter the issue title", "title")
if errors.Is(err, elicitation.ErrInputPending) {
    return flow.InputRequiredResult(), nil, nil // the client answers and retries
}
```

On the multi-round-trip path the handler is re-invoked from the start on
every round. The answers gathered so far travel in the opaque `requestState`
the client echoes back, so a flow replays the prompts already answered from
state instead of asking again. The state is signed with a key generated once
per process and bound to the call it belongs to (a digest of the tool name and
its canonicalized arguments), and it expires after `tenancy.RequestStateTTL`
(register row `IDN-011`). A replica of the server that did not issue the state
refuses it, which is why a balanced deployment whose clients answer
elicitations needs affinity. A handler that reports its outcome through an
error return uses `flow.PendingError()` instead; the dispatchers unwrap the
`*elicitation.InputRequiredError` with `toolutil.InputRequiredResultFromError`
and return the result it carries.

### Client and Flow

`elicitation.FromRequest(req)` returns a `Client`, a zero-value-safe value
type like `progress.Tracker`; a zero `Client` reports elicitation unsupported.
`elicitation.FlowFromRequest(req)` returns a `*Flow`, the protocol-aware entry
point the guided flows and the destructive confirmation
(`toolutil.ConfirmDestructiveAction`) use. `Flow` mirrors each prompt method
of `Client` with one extra leading argument, a stable exchange ID, unique per
prompt within one tool call, which identifies the exchange across
re-invocations; on a legacy session each method delegates to the synchronous
`Client`.

| `Client` method                                 | Returns                   | Purpose                                                                                        |
| ----------------------------------------------- | ------------------------- | ---------------------------------------------------------------------------------------------- |
| `IsSupported()`                                 | `bool`                    | Whether the client has the elicitation capability                                              |
| `IsFormSupported()`                             | `bool`                    | Whether it supports form mode, which every schema-driven prompt below needs                    |
| `IsURLSupported()`                              | `bool`                    | Whether it supports URL mode                                                                   |
| `Confirm(ctx, message)`                         | `(bool, error)`           | A yes or no question                                                                           |
| `PromptText(ctx, message, field)`               | `(string, error)`         | Free-form text                                                                                 |
| `PromptNumber(ctx, message, field, min, max)`   | `(float64, error)`        | A number within bounds; NaN and infinities are refused                                         |
| `SelectOne(ctx, message, options)`              | `(string, error)`         | One choice from a list, re-validated against the list                                          |
| `SelectOneInt(ctx, message, options)`           | `(int, error)`            | One integer choice; NaN, infinities and fractions such as `2.5` are refused                    |
| `SelectMulti(ctx, message, options, min, max)`  | `([]string, error)`       | Several choices within cardinality bounds                                                      |
| `GatherData(ctx, message, schema)`              | `(map[string]any, error)` | Structured data against a JSON Schema                                                          |
| `ElicitURL(ctx, gitlabBaseURL, targetURL, msg)` | `error`                   | Open a GitLab URL in the client (URL mode); the target must match the instance's host and port |

`Flow` adds `UsesMultiRoundTrip()`, `InputRequiredResult()` and
`PendingError()`.

### Errors

| Error                            | Meaning                                           | What the handler does                                                                                                                                 |
| -------------------------------- | ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ErrElicitationNotSupported`     | The client does not support elicitation           | Return an informational result explaining the requirement                                                                                             |
| `ErrFormElicitationNotSupported` | The client declared elicitation without form mode | Raised by `Flow` on the multi-round-trip path, since a server must not send a mode the client did not declare; the guided flows return it as an error |
| `ErrURLElicitationNotSupported`  | The client does not support URL mode              | Fall back to a text-based workflow                                                                                                                    |
| `ErrMalformedAnswer`             | The answer did not satisfy the requested schema   | Treat it as a client fault; the value never reaches the handler                                                                                       |
| `ErrInputPending`                | A multi-round-trip answer is not available yet    | Return `flow.InputRequiredResult()`, or `flow.PendingError()`, so the client can answer and retry                                                     |
| `ErrDeclined`                    | The user declined                                 | Return a cancellation result                                                                                                                          |
| `ErrCancelled`                   | The user cancelled                                | Return a cancellation result                                                                                                                          |

A cancellation is returned with `elicitation.CancelledResult(message)` (or
`toolutil.CancelledResult`, which annotates it), a result with
`isError: true`. The error flag is deliberate: a cancelled flow produced none
of the output its `outputSchema` describes, and a tool that declares an output
schema must return structured results conforming to it. The message is what
tells the model the stop was a person's decision and not a fault.

A URL-mode elicitation carries a server-generated UUID v4 `elicitationId` on
sessions below 2026-07-28, the revision that removed the field. Form mode,
which is every flow this server runs, carries none: correlation for a form
request is the JSON-RPC request ID.

### When the client cannot prompt

Each guided flow is wired through `elicitationRoute` in
`internal/tools/elicitationtools/action_specs.go` with the canonical ID of the
action that does the same work with every field passed in the call
(`issue.create`, `merge_request.create`, `project.create`, `release.create`).
When the flow returns `ErrElicitationNotSupported`, the route answers with an
informational result naming that action, which is the same on every surface.

### Adding a guided flow

Write the handler in `internal/tools/elicitationtools` on `elicitation.Flow`,
in the order the existing flows follow: capability check, sequential prompts,
a final `Confirm`, then the GitLab call. Every flow confirms before it writes
anything, so the user always sees a summary and approves it. Expose the
handler through the package's `ActionSpecs` and the standalone surface specs
(`internal/tools/surfaces`), never through package-local registration; the
`surfaces` group carries the `elicitation` capability requirement.

---

## Completions (`internal/completions`)

```go
handler := completions.NewHandler(client)
// passed as mcp.ServerOptions.CompletionHandler
```

`Handler.Complete` dispatches a `completion/complete` request by its
reference type: `ref/prompt` goes to `completePromptArg`, whose `switch` names
the 18 argument names the server completes, and `ref/resource` to
`completeResourceArg`, which completes `project_id`, `group_id`,
`merge_request_iid` and `issue_iid`. A completer queries GitLab with the
partial input and returns at most `maxCompletionResults` (10) values, bare
values with no labels, filling `total` from GitLab's `X-Total` header when it
is sent and `hasMore` when more exist. There is no cache, so a suggestion is
never a branch that has since been deleted. A GitLab error answers an empty
list; a prompt reference naming a prompt this server does not serve is refused
with `-32602`. The handler reads the caller's client from the request
context, so a server shared by several credentials completes with the
caller's. Adding a completable argument means a case in `completePromptArg`
and a search function in `search.go`, and the argument count in the
documentation changes with it.

---

## Icons

Every tool, resource and prompt carries a `[]mcp.Icon` of three entries, an
SVG and a light and a dark WebP, in that order; the server identity carries
the brand mark. What the entries contain and how a client should choose
between them is on the site page; this section is how they are built.

### The helper

`internal/toolutil/icons.go` holds one `svg<Name>` constant per icon and builds
each exported `Icon<Name>` with `icon()`, which base64-encodes the SVG and
reads the two pre-generated WebP fallbacks from an embedded filesystem:

```go
//go:embed icons/webp/*.webp
var webpFS embed.FS

func icon(name, svg string) []mcp.Icon {
    encodedSVG := base64.StdEncoding.EncodeToString([]byte(svg))
    return []mcp.Icon{
        {
            Source:   "data:" + svgMIME + ";base64," + encodedSVG,
            MIMEType: svgMIME,
            Sizes:    []string{"any"},
        },
        webpIcon(name, "light", mcp.IconThemeLight),
        webpIcon(name, "dark", mcp.IconThemeDark),
    }
}
```

`webpIcon` panics during package initialization when an icon has no generated
pair, rather than shipping an icon whose two fallback entries are empty.

### Where an icon is attached

Icons are attached per catalog group, not per package:
`catalogGroupIconsByToolName` in `internal/tools/catalog_group_metadata.go`
maps each group name (`gitlab_branch`, `gitlab_issue`, ...) to its icon, and
`catalogGroupIcons` falls back to `IconServer` for a group the map does not
name. The fallback is reached only by a group added without an entry:
`TestCatalogGroupIcons_EveryGroupTheCatalogBuilds_HasAnEntry` builds the
catalog at Free, Premium and Ultimate on a self-managed instance and on
GitLab.com, fails on any group the map does not name or whose icon is not the
map's, and fails on an entry no build serves. Before it, `gitlab_achievement`
had no entry and was drawn as the server, the icon `gitlab_execute_action`
also carries ([#1176](https://github.com/jmrplens/gitlab-mcp-server/issues/1176)).
A new group therefore needs its line in the map, and a new icon its
`svg<Name>` constant, its `Icon<Name>` variable, its WebP pair and its row in
`allIcons()`. A meta-tool carries its group's icon and every individual tool
projected from the group's actions inherits it. The dynamic surface's two tools carry
`IconSearch` and `IconServer`. Resources and prompts name their icon where
they are registered (`Icons: toolutil.IconIssue` on the `mcp.Resource` or
`mcp.Prompt` literal), and `IconBrand` is attached to `Implementation.Icons` in
`newServerShell`.

### Regenerating

- **The WebP fallbacks** under `internal/toolutil/icons/webp/` are generated
  from the SVG constants by `cmd/gen_icon_webp` (`make gen-icon-webp`) and
  committed; builds only embed them. It needs `rsvg-convert` (librsvg) and
  `cwebp` (libwebp) on `PATH`, so it is a maintainer step, and
  `make check-icon-webp`, which needs the same tools, is not run in CI. It
  scans `brandmark_gen.go` beside `icons.go`, so the brand mark's fallbacks
  regenerate with the rest. Run it after adding or editing an icon.
- **The brand mark** is generated: `cmd/gen_brand` holds the parametric
  geometry and `make brand` writes the 24×24 `currentColor` constant
  `internal/toolutil/brandmark_gen.go` with the site logo, favicon and cards,
  so the in-binary mark cannot drift from them. `make brand-check` is the CI
  gate.

Both commands are in [Command-Line Utilities](cmd-utilities.md#gen_icon_webp).
WebP was chosen when the fallbacks were added: lossless WebP measured about
32% smaller than optimized PNG at 16×16, JPEG has no alpha channel, and GIF's
256-color palette with one-bit alpha bands antialiased edges.

### Tests

`internal/toolutil/icons_test.go` holds every icon to the contract:

| Test                                | Validates                                                                                                          |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `TestAllIcons_ThreeEntries`         | Every icon has exactly three entries (SVG, light WebP, dark WebP)                                                  |
| `TestAllIcons_ValidDataURI`         | Every entry's source starts with the matching `data:<MIMEType>;base64,`                                            |
| `TestAllIcons_CorrectMIMEType`      | Entry 0 is `image/svg+xml`; entries 1 and 2 are `image/webp`                                                       |
| `TestAllIcons_NonEmpty`             | No entry's source is empty                                                                                         |
| `TestAllIcons_DecodesToSVG`         | The SVG entry decodes to an `<svg>...</svg>` document                                                              |
| `TestAllIcons_SizesAny`             | The SVG entry's `Sizes` is `["any"]`                                                                               |
| `TestAllIcons_WebPFallbackTheme`    | The WebP entries declare `Theme` `light` and `dark` and `Sizes: ["16x16"]`                                         |
| `TestAllIcons_WebPFallbackDecodes`  | The WebP payloads decode to a 16×16 image through `golang.org/x/image/webp`                                        |
| `TestWebpIcon_PanicsOnMissingAsset` | A name with no generated WebP pair panics during initialization instead of shipping an icon with two empty entries |

The icons are self-contained SVGs with no script, no event handlers and no
external references, which is what the specification asks a client to be wary
of; keep a new one that way.
