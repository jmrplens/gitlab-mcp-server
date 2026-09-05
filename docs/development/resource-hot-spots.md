# Resource Hot Spots

> **Diátaxis type**: Explanation
> **Audience**: Contributors changing the catalog, the tool surfaces, or the HTTP pool

What a pooled credential used to cost, where the memory went, what was
changed so that most of it is now shared between credentials, and what is
left. The numbers come from the concurrency series of the
[resource benchmark](../reference/resource-benchmark.md), which steps one HTTP
process through credential counts and writes a CPU and a heap profile at each
step; this page reads those profiles. The measurements marked as taken on the
TrueNAS host were taken before the change and will be joined by an after
series from the same host when it is re-run there; the before and after pairs
below were taken on the development machine described with them.

## What a pool entry was made of

In HTTP mode the pool builds one MCP server per credential, and until this
work every one of those servers built everything it needed from scratch: the
tool catalog, the schemas the tools carry, the search index of the dynamic
surface, the `gitlab://tools` manifest. The resident set therefore grew as a
straight line in the number of pooled credentials, and the slope was the size
of one of those builds. Measured on an i5-14400 with 62 GiB (kernel
6.12, Go 1.27.1), one HTTP process per surface, four requests in flight per
credential:

| Surface      | Resident set per credential | Where the series stopped                      |
| ------------ | --------------------------: | --------------------------------------------- |
| `dynamic`    |                     130 MiB | 13.2 GiB at 100 credentials, budget exhausted |
| `meta`       |                      63 MiB | 12.9 GiB at 200 credentials, budget exhausted |
| `individual` |                      90 MiB | 9.4 GiB at 100 credentials, budget exhausted  |

Processor time per call was flat across the series, about 8 ms on `dynamic`
and `meta` and 150 ms on `individual`, the latter dominated by marshalling its
3 MB `tools/list`; latency grew with the total load. About a fifth of the CPU
at a hundred credentials was the garbage collector scanning the heap
(`scanObjectsSmall`, `tryDeferToSpanScan`, `scanSpan`), which is memory
showing up as time.

The heap profile at the top step named one function on every surface:

| Surface                 | In-use heap | `toolutil.cloneSchemaMap` | Its callers, cumulative                                                                                                                                                                                            |
| ----------------------- | ----------: | ------------------------: | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `dynamic`, 100 creds    |    5.83 GiB |            48% (2.82 GiB) | `dynamic.newRegistryFromCatalog` 62%: `Catalog.Groups` and `Group.ActionMap` deep-copying the catalog, then the search documents and the search index built over the copy                                          |
| `meta`, 200 creds       |    5.59 GiB |            53% (2.95 GiB) | `toolutil.CloneMetaSchemaRoutes` 49%, through `Group.ActionMap` in `RegisterMetaCatalog`; `LookupMetaActionSchema` 16%, one enriched copy per manifest entry; `enrichParameterGuidanceSchema`                      |
| `individual`, 100 creds |    3.76 GiB |            33% (1.25 GiB) | `Catalog.Groups` in `RegisterIndividualCatalogTools`; then `jsonschema.Schema.MarshalJSON` 20% and `decodeMapStringInterface` 10%: the lockdown middleware turning each compiled schema back into a per-server map |

Two facts explain the shape. The base schemas were already shared: a route's
`InputSchema` comes from `inputSchemaForType`, cached per reflected type for
the whole process, so the catalog build itself referenced process-wide maps.
The copies came afterwards, and they came from the catalog's own accessors:
`cloneAction` and `cloneGroup` deep-copied every route's schema maps on every
`Groups()`, `Actions()`, `ActionMap()` and `ActionMaps()` call, and each
surface called several of those per server. The copies existed to be mutated:
`LockdownInputSchemas` and `EnrichPaginationConstraints` ran once per server on
its first `tools/list` and rewrote the schemas in place, `NewActionSpec` applied
overrides and canonical enums in place, `IndividualToolFromActionSpec` applied
the required list and the lockdown in place, and the meta and dynamic schema
readers added the confirm property and the guidance in place. Nothing could
share a map that anything might write to, so everything copied.

The dynamic surface added its own layer on top: `newRegistryFromCatalog`
rebuilt the search documents, the alias tables and the inverted index for
every entry, from its own deep copy of the catalog, and the `gitlab://tools`
manifest snapshot was built per entry on every surface.

## The options, per hot spot

**The schema copies.** Three ways to stop copying were weighed. Keeping the
copies but making them cheaper (interning strings, compacting maps) leaves
the slope where it is and only changes its coefficient. Copy-on-write per
server, where a server copies a schema the first time it wants to change it,
still ends with one copy per server for every schema the middlewares touch,
which is all of them. The third, chosen, is to freeze the schemas and make
every transform a pure function of its input that is memoized process-wide:
lockdown of schema X is one map, however many servers list X. The memo key is
the identity of the input map, its address, which is free to compute and
exact, and which is only valid for an object that can never be collected; a
content hash would be valid for any input but costs a walk over a thousand
schemas per server, which is the work being removed. So the identity key is
used, and only for schemas registered as process-lived: a schema nobody
registered is transformed privately, exactly as before, and the memo cannot
leak because it never holds a key whose object could be freed.

**The handlers.** A catalog shared between credentials must not share the
handlers, which capture the GitLab client. Two designs were considered. One
resolves the client from the request context at call time, leaving every
handler shared and stateless; it is elegant and fragile at once, since every
path that registers catalog tools must install the middleware that injects
the client, a shared catalog built with anyone's real client would keep that
credential reachable process-wide, and tests that invoke handlers directly
would have to know. The other, chosen, keeps a handler bound to one client
and records beside it the function that builds the handler for any client:
`ActionRoute.Bind`. The shared catalog is built once with a client that
carries no credential and refuses every request, and `Catalog.BindTo` gives
each server a copy of the descriptors with the handlers rebuilt for its
client. The cost is one closure per action per server, which the profile
shows is small, and the hazard is a domain that replaces `Route.Handler`
directly, since binding would then rebuild the constructor's plain handler
and drop the replacement. Twenty such sites existed, nineteen turning a 404
into a structured not-found result and one binding the elicitation flows;
all now go through `WrapHandler` or `WithBoundHandler`, `ValidateRouteBinding`
refuses a catalog whose handler is not the one its binder installed, and a
source-level test refuses the assignment in any package under
`internal/tools`. The validation compares closure identity rather than code,
because the compiler gives an inlined copy of a function literal a closure of
its own.

**The dynamic registry.** Its shape (entries, search documents, alias tables,
inverted index) depends on the catalog alone; its handlers dispatch through
the bound routes. The shape is cached per shared catalog, keyed by the
catalog's identity rather than by a content key such as the sorted action
IDs, because the search documents are built from schema property names,
descriptions and enum values, which the tier changes; identity is exact and
free. The shape is built from the shared, unbound origin, so the routes the
entries carry refuse every call, and `Execute` dispatches only through the
handlers built per registry over the bound catalog.

**The manifest.** `gitlab://tools` and `gitlab://tools/{id}` serve a snapshot
that depends on the surface, the shared catalog, the narrowing that decides
which registered tools are visible, the meta parameter-schema mode and the
capability surface. The snapshot is cached under a key naming all of those,
computed where the configuration is known, in `cmd/server`.

**The meta compact and full envelopes.** These embed every action's params
schema and are served as maps. Compiling them into `*jsonschema.Schema`, as
the opaque envelope already is, would have shared them through the compile
cache, and was tried first; it changes the wire, since a boolean sub-schema
serializes differently from a map through the two paths. The envelope map is
shared instead, memoized under the identities of the schemas it embeds.

## What was built

The direction is the one the maintainer set, and it is worth stating plainly
because every piece follows from it: everything in the binary is immutable.
The complete schema is, and every tool and action carries its own properties,
so what a user is served is a filter over that one immutable catalog at
request time, never a copy built per user. One frozen catalog per
configuration, views filtered per credential.

| Component                                                                                                                                                                         | What is shared, and by what key                                                                                                                                                                                                                                                                                                                                                                                                  |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `toolutil.ShareSchema`, `toolutil.DeriveSchema` (`internal/toolutil/schema_share.go`)                                                                                             | The registry of process-lived schemas (the reflected type schemas, the compiled tool schemas, every route schema of a shared catalog, and every derivation of one) and the memo of named transforms over them, keyed by input address and transform name. Every transform is idempotent and its output is registered, so reapplying it to its output returns the output; a clone chain in the catalog build collapses to one map |
| `NewActionSpec`, tier pruning, `IndividualToolFromActionSpec`, `MetaActionSchema`, the dynamic and manifest schema readers, `LockdownInputSchemas`, `EnrichPaginationConstraints` | Each derives its result through `DeriveSchema` instead of mutating a copy; the transform names carry every parameter the result depends on (overrides, tier, type, destructive flag, guidance identity)                                                                                                                                                                                                                          |
| `ActionRoute.Bind`, `BindTo`, `WrapHandler`, `WithBoundHandler`, `ValidateRouteBinding`                                                                                           | The seam between shared metadata and a per-client handler, set by the eight typed constructors and by `WithSafeModePreviews`                                                                                                                                                                                                                                                                                                     |
| `actioncatalog.Catalog.MarkShared`, `SharedOrigin`, `BindTo`; accessors no longer deep-copy                                                                                       | A catalog cached for the process is marked shared and its schemas registered; a bound catalog remembers its origin, which keys every client-independent derivation                                                                                                                                                                                                                                                               |
| `tools.ShareCatalog`, `SharedBaseCatalog`, `SharedMetaCatalog`, `SharedIndividualCatalog`, `dynamiccatalog.Build`                                                                 | One catalog per key, built with `tools.UnboundClient` for the instance class. Base key: tier, GitLab.com or self-managed, maintenance group. Surface keys add the narrowing: exclusions, token scopes sorted and whether they were detected, read-only mode and its cause, safe mode. `BuildActionCatalog(client, opts)` returns the shared base bound to the client; only a call carrying spec-group overrides builds privately |
| `dynamic.registryShapeFor`; the find and execute input schemas built once                                                                                                         | One registry shape per shared catalog origin; handlers per registry over the bound catalog                                                                                                                                                                                                                                                                                                                                       |
| `resources.ToolSurfaceResourceOptions.ShareKey`, `cmd/server.manifestShareKey`                                                                                                    | One manifest snapshot per surface, origin, narrowing, parameter-schema mode and capability surface                                                                                                                                                                                                                                                                                                                               |
| `tools.NewCallIdentifier`                                                                                                                                                         | One telemetry resolver per shared origin and surface                                                                                                                                                                                                                                                                                                                                                                             |
| `gitlabclient.NewUnboundClient`                                                                                                                                                   | The credential-less client a shared catalog is built with: it answers the instance class and refuses every request with `ErrUnboundClient`, so a handler served from the shared copy by mistake fails closed instead of running under someone else's token                                                                                                                                                                       |

What is deliberately still private to a server: its tool table (the SDK's one
entry per registered tool, with the closures that dispatch), the bound catalog
descriptors (one `Action` value per action, sharing every map), the sessions,
the GitLab client, the meta dispatchers' descriptions (built per tool from the
routes, a few kilobytes each), and, on the compact and full parameter-schema
modes only, the schema the SDK resolves for validation at `AddTool`, since the
SDK caches resolution by pointer and those envelopes are served as maps.

## What this branch measured

The same driver, on the development machine the change was made on: an AMD
Ryzen 5 3550H with 8 threads and 60.8 GiB, kernel 6.1, Go 1.27.1, a slower
host than the one above, so its per-call figures are higher and only its
before and after pairs compare. One HTTP process per surface, credential
counts 1, 2, 5, 10 and 20, ten seconds per step, four requests in flight per
credential on `dynamic` and `meta` and two on `individual`; before is the base
commit's binary, after is this branch. The slope is a least-squares line
through the five steps. Beside the resident set, the in-use heap total of the
profile at each step gives the live state per credential, which is the part
of the slope that sharing can reach: the rest is what the requests in flight
allocate while they are served.

| Surface      | Peak resident set per credential, before |    after | Ratio | Live heap per credential, before |  after | Ratio | Peak at 20 credentials, before |   after |
| ------------ | ---------------------------------------: | -------: | ----: | -------------------------------: | -----: | ----: | -----------------------------: | ------: |
| `dynamic`    |                                133.4 MiB |  6.6 MiB |   20x |                          62.4 MB | 3.1 MB |   20x |                       2784 MiB | 319 MiB |
| `meta`       |                                 78.0 MiB |  7.6 MiB |   10x |                          29.7 MB | 2.4 MB |   12x |                       1687 MiB | 320 MiB |
| `individual` |                                107.4 MiB | 27.7 MiB |  3.9x |                          42.6 MB | 5.5 MB |  7.7x |                       2374 MiB | 853 MiB |

Processor time per call did not move, which is expected: 19.5 ms before and
after on `dynamic` and `meta`, 348 and 342 ms on `individual`, the per-call
work being the SDK's and the subject of the next section. What moved with the
memory is latency under load, since the collector has less to scan:
`tools/call` p50 at 20 credentials went from 571 to 346 ms on `dynamic`, 281
to 267 ms on `meta` and 156 to 120 ms on `individual`.

The top of the after heap profile at 20 credentials on the dynamic surface,
114.8 MB in use against 1260.9 MB before, by flat allocation:

| Rank | Function                                                   |    Flat | What it is                                                                                                                 |
| ---: | ---------------------------------------------------------- | ------: | -------------------------------------------------------------------------------------------------------------------------- |
|    1 | `segmentio/encoding/json.decoder.decodeMapStringInterface` | 14.5 MB | the SDK decoding the arguments and results of the calls in flight                                                          |
|    2 | `reflect.mapassign_faststr0`                               | 10.5 MB | the maps those decodes build                                                                                               |
|    3 | `encoding/json/jsontext.(*encoderState).reformatObject`    | 10.5 MB | response encoding buffers of the calls in flight                                                                           |
|    4 | `bytes.Clone`                                              |  8.8 MB | request bodies in flight                                                                                                   |
|    5 | `bytes.growSlice`                                          |  7.1 MB | encoder growth                                                                                                             |
|    6 | `segmentio/encoding/json.decoder.decodeInterface`          |  7.0 MB | the same decodes                                                                                                           |
|    7 | `toolutil.cloneSchemaMap`                                  |  6.0 MB | the shared derivations, built once for the process: the type caches, the spec and individual rewrites, the lockdown copies |
|    8 | `encoding/json/jsontext.(*encoderState).reformatValue`     |  5.0 MB | response encoding                                                                                                          |
|    9 | `dynamic.dedupeStrings`                                    |  4.0 MB | the one registry shape                                                                                                     |
|   10 | `actioncatalog.(*Group).ActionMap`                         |  4.0 MB | per entry: the route maps the dynamic handlers dispatch through, 0.3 MB per credential                                     |

Below those, `dynamic.buildRegistryShape` at 12.5 MB cumulative is the one
search index, `actioncatalog.(*Catalog).AddGroup` at 3.5 MB is the shared
catalog build plus the bound copies, and `toolutil.cloneRouteStrings` at
2.5 MB is the slices of the bound routes, per entry. Nothing in the profile
copies a schema per server. The live residue per credential on this surface,
about 3 MB, is the bound catalog descriptors, the handlers' route maps, the
closures, and the server with its sessions.

The individual surface is where the resident slope stayed furthest from an
order of magnitude, and its profile says why that is not something to share.
`registerIndividualCatalogAction` accounts for 68 MB cumulative at 20
credentials, of which 33.5 MB under `CompileToolSchemas` is the compiled
schema cache the first entry built for the whole process, 12 MB under
`mcp.AddTool` is the SDK's tool table (0.6 MB per credential), 8 MB is the
`mcp.Tool` structs (0.4 MB) and 2 MB the handler closures: the tool table and
closures the direction leaves per server, about 1.1 MB. The rest of the
profile is `jsontext.reformatObject` and `reformatValue` at 51 MB and
`reflect.New` at 28 MB, both under `encoding/json/v2` marshalling the 3 MB
`tools/list` responses of the calls in flight, forty of them at once at this
step. The slope that remains on this surface is the cost of the responses
each credential keeps in flight, not of the credential, and it shrinks only
by shrinking what a `tools/list` allocates, which is the pre-encoded list
below.

At the previous slopes the TrueNAS series stopped at 100 credentials on
`dynamic` and `individual` and 200 on `meta` against a 16 GiB budget; at the
new ones a thousand credentials project to about 7 GiB on `dynamic`, 8 GiB on
`meta` and 28 GiB on `individual` under the same load, which is the series
the host will be asked to run.

## What remains, per credential and per call

Per credential, the residue is what the direction says it should be: the
server's own tool table and closures, its sessions, its client, and a bound
copy of the catalog's descriptors. The heap profile after the change is
listed above; nothing in it copies a schema.

Per call, the cost is processor time, and it was not this work's target. The
CPU profile of the before run at a hundred credentials on the dynamic surface
says where the 8 ms per call go, which is what makes a few dozen credentials
saturate a sixteen-thread host at about 1,500 calls a second. Inside
`mcp.(*Server).callTool` (68.5% of all samples), `mcp.applySchema` is 42% of
the total: the go-sdk validates the structured output of every call by
marshalling the handler's result to JSON (`encoding/json/v2.marshalEncode`,
35%) and decoding that JSON into `map[string]any`
(`segmentio/encoding/json.Decoder.Decode`, 30%) before checking it against
the output schema; the validation itself is not in the top of the profile, the
two conversions are. `dynamic.(*Registry).Find`, the search scoring behind
`gitlab_find_action`, is 13.6%, and GC marking 12.6%. On the individual
surface 84% of the CPU is `tools/list` marshalling the 3 MB response through
`encoding/json/v2` reflection over the tool structs and their schema maps.

Reading `applySchema` and `toolForErr` in go-sdk v1.7.0
(`$(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.7.0/mcp/`)
fixes what the options are:

- A typed handler (`mcp.AddTool[In, Out]`) always marshals its result: that
  JSON becomes `StructuredContent` and the `TextContent` fallback. The decode
  and the validation run only when the tool has an output schema
  (`resolved == nil` returns the bytes untouched), and the bytes are
  re-marshalled only when defaults were applied. So the first conversion is
  the price of returning structured content through a typed handler; the
  second is the price of declaring an output schema. Handing the SDK an
  already-decoded `map[string]any` as `StructuredContent` does not help, since
  the SDK marshals whatever the handler returns; a raw `mcp.ToolHandler`
  registered through `Server.AddTool` skips both conversions and both
  validations, input included, so it would have to validate the input itself.
- `gitlab_execute_action` declares a permissive output envelope
  (`ActionDispatchOutputSchema`). Dropping it removes the decode and the
  validation, about 30% of the per-call CPU on that surface, at the cost of
  the `outputSchema` in `tools/list`, which is a wire change and so a
  deliberate one. Validating each action's output shape once per action type
  off the hot path, and serving the envelope without a schema, keeps the
  guarantee without paying it per call.
- For the individual surface, the SDK accepts `json.RawMessage` as a tool's
  `InputSchema` and lists it as provided, so a pre-encoded schema per shared
  shape is possible: the bytes a map marshals to, marshalled once. Two things
  keep it from being a small change. The SDK resolves a raw schema by
  re-marshalling it, bypassing the pointer-keyed resolve cache, so the
  compiled pointer would have to stay for registration and the raw bytes be
  swapped in by the `tools/list` middleware; and `encoding/json/v2` re-scans a
  raw value on output (`jsontext.reformatValue`), cheaper than reflection but
  not free. A whole pre-encoded `tools/list` is not available: the SDK's
  `Result` is an interface with an unexported marker, so a middleware cannot
  substitute a type that marshals itself.

None of these is implemented here. They are phase three, with the expected
effect recorded: dropping the execute output schema or validating off the hot
path should take roughly a third off the dynamic surface's per-call CPU, and a
pre-encoded individual list should take most of that surface's 150 ms per
`tools/list`, which is what an agent client pays on every reconnect.

## How it is known that nothing changed on the wire

- The golden snapshots `internal/tools/testdata/tools_individual.json` and
  `tools_meta.json` (`TestToolSnapshots_Individual`, `TestToolSnapshots_Meta`)
  pin every tool's name, description, annotations and both schemas on the two
  surfaces with a golden, and pass unchanged.
- `TestSharedIndividualCatalog_TwoServersListOneCatalogUnchanged` registers
  two servers from one shared catalog, checks that every action's schema maps
  are the same objects in both, that their `tools/list` responses are
  byte-identical, and that no schema map in the catalog changed between the
  build and the listing, by hashing every one before and after.
  `TestSharedMetaCatalog_ConcurrentEntriesBuildSafely` builds several entries
  at once from one fresh configuration under the race detector.
- The two transport end-to-end modules (`make test-e2e-http`,
  `make test-e2e-stdio`) drive the real binary.
- Before merging, the base binary and the changed binary were driven over
  stdio through the same twenty-two requests on nine configurations (the
  three surfaces, the three parameter-schema modes, read-only, safe mode with
  exclusions, the minimal capability surface), covering `tools/list`, the
  manifest and its details, the meta schema resources, `gitlab_find_action`,
  one call on each surface and the resource, template and prompt listings.
  The responses were byte-identical apart from the version string in
  `initialize`. That is how the compact and full envelopes were found to
  change under compilation, and why they are shared as maps.
