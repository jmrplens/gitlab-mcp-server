# Resource Hot Spots

> **Diátaxis type**: Explanation
> **Audience**: Contributors changing the catalog, the tool surfaces, or the HTTP pool

What a pooled credential used to cost, where the memory went, what was
changed so that most of it is now shared between credentials, and what is
left. The numbers come from the concurrency series of the
[resource benchmark](../reference/resource-benchmark.md), which steps one HTTP
process through credential counts and writes a CPU and a heap profile at each
step; this page reads those profiles.

Every measurement on this page is from one host: an Intel i5-14400 with 16
threads and 62 GiB, kernel 6.12, Go 1.27.1, each binary built from a clean tree.

Which trees, exactly, because it decides how far the figures can be trusted. The
before series was run from the commit this work branched from at the time, which
a later rebase has made unreachable; the branch point today is `5492f2b5` on
`main`, the commit that added the concurrency series this page reads. The after
series was run from `53eb2204`, which is reachable and can be checked out, and
which is **not** the last commit of the branch: nine commits follow it, five of
them functional, and at least three of those change what is allocated (the
shared-catalog key was narrowed to the scopes the filter reads, a route's
parameter guidance is now named by content rather than by address, and each
shared cache entry is built once under a startup burst). The published figures
therefore predate those five.

A fresh series from the current head is being measured and will replace the
tables below. When it lands, what changes is the commit named here and the
numbers in the tables; the reasoning around them does not depend on their
values, only on their order of magnitude.

## What a pool entry was made of

In HTTP mode the pool used to build one MCP server per credential, and until
this work every one of those servers built everything it needed from scratch: the
tool catalog, the schemas the tools carry, the search index of the dynamic
surface, the `gitlab://tools` manifest. The resident set therefore grew as a
straight line in the number of pooled credentials, and the slope was the size
of one of those builds. One HTTP process per surface, four requests in flight
per credential on `dynamic` and `meta` and two on `individual`, least-squares
slope through the peak resident set of every step:

| Surface      | Resident set per credential | Where the series stopped                                   |
| ------------ | --------------------------: | ---------------------------------------------------------- |
| `dynamic`    |                   130.9 MiB | 12.9 GiB at 100 credentials, the next step over the budget |
| `meta`       |                    63.5 MiB | 12.6 GiB at 200 credentials, the next step over the budget |
| `individual` |                    90.8 MiB | 9.2 GiB at 100 credentials, the next step over the budget  |

The budget was 15.8 GiB, and each series stopped when the next step's estimate
exceeded it rather than on a failure.

Processor time per call was flat on `dynamic` and `meta`, 7.5 to 8.1 ms and
8.3 to 7.9 ms from the first step to the last. On `individual` it was not: it
climbed 43%, from 106 ms at one credential to 152 ms at a hundred, dominated
by marshalling its 3 MB `tools/list`. Latency grew with the total load. About
a fifth of the CPU at a hundred credentials was the garbage collector scanning
the heap (`scanObjectsSmall`, `tryDeferToSpanScan`, `scanSpan`), which is
memory showing up as time.

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

The parameter guidance a transform name also depends on is the exception, and
it is named by a digest of its content. Nothing pins a guidance map for the
process the way `ShareSchema` pins a schema, so its address is exactly the
kind of key the paragraph above rules out: a route from a catalog nobody
retained would leave a permanent memo entry keyed by an address the allocator
can hand out again. The digest is over a handful of short strings per route,
not over a schema, so it does not bring back the walk that made a content key
wrong for the schemas.

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
refuses a catalog whose handler is not the one its binder installed,
`Catalog.Validate` refuses one whose action has no binder at all, and a
source-level test refuses the assignment, and any write into a route's
guidance or schema maps, in every package under `internal/tools`. The
validation compares closure identity rather than code,
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

| Component                                                                                                                                                                         | What is shared, and by what key                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `toolutil.ShareSchema`, `toolutil.DeriveSchema` (`internal/toolutil/schema_share.go`)                                                                                             | The registry of process-lived schemas (the reflected type schemas, the compiled tool schemas, every route schema of a shared catalog, and every derivation of one) and the memo of named transforms over them, keyed by input address and transform name. Every transform is idempotent and its output is registered, so reapplying it to its output returns the output; a clone chain in the catalog build collapses to one map                                                                                                                                                                                                                                                                                                                            |
| `NewActionSpec`, tier pruning, `IndividualToolFromActionSpec`, `MetaActionSchema`, the dynamic and manifest schema readers, `LockdownInputSchemas`, `EnrichPaginationConstraints` | Each derives its result through `DeriveSchema` instead of mutating a copy; the transform names carry every parameter the result depends on (overrides, tier, type, destructive flag, and a digest of the parameter guidance)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `ActionRoute.Bind`, `BindTo`, `WrapHandler`, `WithBoundHandler`, `ValidateRouteBinding`                                                                                           | The seam between shared metadata and a per-client handler, set by the seven typed constructors that take a client and by `WithSafeModePreviews`. A catalog route must carry it: `Catalog.Validate` refuses an action without one, the dynamic controllers excepted, since their handlers close over the registry rather than a client                                                                                                                                                                                                                                                                                                                                                                                                                       |
| `actioncatalog.Catalog.MarkShared`, `SharedOrigin`, `BindTo`; accessors no longer deep-copy                                                                                       | A catalog cached for the process is marked shared and its schemas registered; a bound catalog remembers its origin, which keys every client-independent derivation                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `tools.ShareCatalog`, `SharedBaseCatalog`, `SharedMetaCatalog`, `SharedIndividualCatalog`, `dynamiccatalog.Build`                                                                 | One catalog per key, built with `tools.UnboundClient` for the instance class. Base key: tier, GitLab.com or self-managed, maintenance group. The meta and dynamic keys add the whole narrowing (`CatalogFilterKey`): exclusions, the token scopes the scope filter can act on, read-only mode and its cause, safe mode. The individual key adds the exclusions and nothing else, because that surface applies the scopes, read-only mode and safe mode when it registers rather than when it builds; its manifest key adds the filter key back, since what a manifest lists is what the server ended up serving. `BuildActionCatalog(client, opts)` returns the shared base bound to the client; only a call carrying spec-group overrides builds privately |
| `dynamic.registryShapeFor`; the find and execute input schemas built once                                                                                                         | One registry shape per shared catalog origin; handlers per registry over the bound catalog                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `resources.ToolSurfaceResourceOptions.ShareKey`, `cmd/server.manifestShareKey`                                                                                                    | One manifest snapshot per surface, origin, narrowing, parameter-schema mode and capability surface                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `tools.NewCallIdentifier`                                                                                                                                                         | One telemetry resolver per shared origin and surface                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `gitlabclient.NewUnboundClient`                                                                                                                                                   | The credential-less client a shared catalog is built with: it answers the instance class and refuses every request with `ErrUnboundClient`, so a handler served from the shared copy by mistake fails closed instead of running under someone else's token                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

None of these caches evicts, and three of them (the registry shape, the
manifest snapshot and the telemetry resolver) name a catalog by its address,
so eviction cannot be added to the catalog cache either: a recycled address
would be served the previous catalog's shape, manifest and resolver. What
takes the place of eviction is a key space bounded by configuration, which is
why the scope component of `CatalogFilterKey` is not the token's scope list
but the part of it the scope filter can act on. The scopes of a token are
chosen by whoever minted it, and a user can mint personal access tokens with
arbitrary scope subsets; before the catalog was shared, that cost died with
the pool entry, which is bounded.

Every one of these caches builds each entry once, through
`toolutil.OnceMap`, rather than letting the callers that lose the race build a
copy each and drop all but one. That matters most where the loser's copy would
not have been dropped: `schemaForType` and `inputSchemaForType` register what
they build as process-lived, so two configurations reflecting one type at the
same moment would both register a map, and the loser's would still be embedded
in its catalog's routes.

What is deliberately still private to a server: its tool table (the SDK's one
entry per registered tool, with the closures that dispatch), the bound catalog
descriptors (one `Action` value per action, sharing every map), the sessions,
the GitLab client, the meta dispatchers' descriptions (built per tool from the
routes, a few kilobytes each), and, on the compact and full parameter-schema
modes only, the schema the SDK resolves for validation at `AddTool`, since the
SDK caches resolution by pointer and those envelopes are served as maps.

## What this branch measured

The same driver on the same host, from `53eb2204`. One HTTP process per
surface, credential counts 1, 2, 5, 10, 20, 50, 100, 200, 500 and 1000, ten
seconds per step, four requests in flight per credential on `dynamic` and
`meta` and two on `individual`. The slope is a least-squares line through the
peak resident set of every step, the same way the before figures above were
computed:

| Surface      | Resident set per credential, before |     after | Ratio | Where the before series stopped | The after series at 1000 credentials | Fitted intercept, after |
| ------------ | ----------------------------------: | --------: | ----: | ------------------------------- | ------------------------------------ | ----------------------: |
| `dynamic`    |                           130.9 MiB |  4.10 MiB |   32x | 100 credentials, 12.9 GiB       | 4.0 GiB                              |                 263 MiB |
| `meta`       |                            63.5 MiB |  5.44 MiB |   12x | 200 credentials, 12.6 GiB       | 5.7 GiB                              |                 139 MiB |
| `individual` |                            90.8 MiB | 11.28 MiB |  8.1x | 100 credentials, 9.2 GiB        | 11.2 GiB                             |                 756 MiB |

All three after series reached a thousand credentials, which no surface
reached before. The after run had a larger budget than the before run (32.3
against 15.8 GiB, the host having been freed up in between), so the stopping
points are not a like-for-like limit; what makes the comparison hold anyway is
that all three after series would have fit under the before run's budget too,
with the largest of them, `individual`, ending at 11.2 GiB.

At 100 credentials, a step both runs took: 12.9 GiB to 704 MiB on `dynamic`,
6.5 GiB to 698 MiB on `meta`, 9.2 GiB to 2.1 GiB on `individual`.

Processor time per call did not move, which is expected, the per-call work
being the SDK's and the subject of the next section: 7.5 ms at one credential
to 8.8 ms at a thousand on `dynamic`, 8.2 to 7.4 ms on `meta`, 106 to 130 ms
on `individual`. What the thousand-credential step does show is where the
remaining cost is: on `individual`, `tools/list` p50 is 25.6 s, one 3 MB
response marshalled per call, while `tools/call` p50 on the same step is
122 ms.

The heap profile of the after run at a thousand credentials on the dynamic
surface holds 936 MB in use, and its top by flat allocation is this:

| Rank | Function                                                   |     Flat | What it is                                                                          |
| ---: | ---------------------------------------------------------- | -------: | ----------------------------------------------------------------------------------- |
|    1 | `actioncatalog.(*Group).ActionMap`                         | 185.1 MB | per credential: the route maps its dynamic handlers dispatch through                |
|    2 | `segmentio/encoding/json.decoder.decodeMapStringInterface` | 114.1 MB | the SDK decoding the arguments and results of the calls in flight                   |
|    3 | `toolutil.cloneRouteStrings`                               | 112.5 MB | per credential: the alias, tag and related-action slices of those same bound routes |
|    4 | `segmentio/encoding/json.(*Decoder).readValue`             |  65.2 MB | the same decodes                                                                    |
|    5 | `bytes.Clone`                                              |  53.8 MB | request bodies in flight                                                            |
|    6 | `encoding/json/jsontext.(*encoderState).reformatObject`    |  46.2 MB | response encoding buffers of the calls in flight                                    |
|    7 | `bytes.growSlice`                                          |  40.3 MB | encoder growth                                                                      |
|    8 | `segmentio/encoding/json.decoder.decodeInterface`          |  40.0 MB | the same decodes                                                                    |
|    9 | `resources.(*recorder).AddResourceTemplate`                |  22.5 MB | per credential: the resource and template records of its server                     |
|   10 | `slices.Grow`                                              |  21.5 MB | encoder growth                                                                      |

`Group.ActionMap` is 297 MB cumulative, the flat 185 MB plus the 112 MB of
`cloneRouteStrings` under it: 0.3 MB per credential, which is what a bound
catalog's descriptors cost. `toolutil.cloneSchemaMap` is 7.5 MB, eight tenths
of one percent of the heap, and it is 7.5 MB in the profiles at 20 and at 100
credentials too: the process-wide derivations, the type caches and the spec,
individual and lockdown rewrites, built once for the process and not once per
server. `dynamic.buildRegistryShape` is 8.0 MB cumulative, the one search
index. Nothing in the profile copies a schema per server.

The individual surface is where the slope stayed furthest from an order of
magnitude, and its profile says why that is not something to share. At a
thousand credentials it holds 2.76 GB in use, of which
`registerIndividualCatalogAction` is 1.13 GB cumulative: the SDK's tool table,
the `mcp.Tool` structs and the handler closures, about 1.1 MB per credential,
which is exactly what the direction leaves per server. Most of the rest is the
responses in flight: `jsontext.reformatValue` and `reformatObject` at 561 MB
between them, `jsonschema.Schema.MarshalJSON` at 450 MB and `bytes.growSlice`
at 228 MB, all of it `encoding/json/v2` marshalling the 3 MB `tools/list`
responses of the calls in flight. What remains on this surface is the cost of
what each credential keeps in flight, not of the credential, and it shrinks
only by shrinking what a `tools/list` allocates, which is the pre-encoded list
below.

## What remains, per credential and per call

Per credential, the residue is what the direction says it should be: the
server's own tool table and closures, its sessions, its client, and a bound
copy of the catalog's descriptors. The heap profile after the change is
listed above; nothing in it copies a schema.

Per call, the cost is processor time, and it was not this work's target. The
CPU profile of the before run at a hundred credentials on the dynamic surface
says where the 8 ms per call go, which is what makes a few dozen credentials
saturate a sixteen-thread host at about 1,500 calls a second. Inside
`mcp.(*Server).callTool` (68.5% of all samples), the go-sdk marshals the
handler's result to JSON first, in `toolForErr` and before any validation
begins (`encoding/json.Marshal`, 12.4% of the total), and then `mcp.applySchema`
is 42.5% of the total: decoding that JSON into `map[string]any`
(`internal/json.Unmarshal`, 28.6%), re-marshalling it when defaults were
applied (10.6%), and the schema check itself
(`jsonschema.(*Resolved).Validate`, 3.2%). The two conversions inside
`applySchema` cost twelve times the check they serve.
`dynamic.(*Registry).Find`, the search scoring behind
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
  (`ActionDispatchOutputSchema`). Dropping it removes everything `applySchema`
  does, 42.5% of the per-call CPU in that profile, at the cost of
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
path should take about two fifths off the dynamic surface's per-call CPU, and
a pre-encoded individual list should take most of what that surface spends on
a `tools/list`, which is what an agent client pays on every reconnect.

## How it is known that nothing changed on the wire

Everything below runs from a checkout, and that is the point of listing it:
what is claimed here is what a reader can re-run.

- The golden snapshots under `internal/tools/testdata/` pin every tool's name,
  description, annotations and both schemas, and pass unchanged.
  `tools_individual.json` and `tools_meta.json`
  (`TestToolSnapshots_Individual`, `TestToolSnapshots_Meta`) cover the
  individual surface and the meta surface in its default opaque mode;
  `tools_meta_compact.json` and `tools_meta_full.json`
  (`TestToolSnapshots_MetaParamSchemaModes`) cover the two modes that publish
  the per-action envelopes, which the opaque golden cannot see. Regenerate any
  of them with `UPDATE_TOOLSNAPS=true`.
- `TestSharedIndividualCatalog_TwoServersListOneCatalogUnchanged` registers
  two servers from one shared catalog, under separate compiled-schema cache
  keys so the comparison is of the maps rather than of one cached compiled
  schema, and checks that their `tools/list` responses are byte-identical and
  that no schema map in the catalog changed between the build and the listing,
  by hashing every one before and after.
  `TestSharedMetaCatalog_TwoClientsShareOneCatalogBoundToEach` is the
  same-object check: every action of two catalogs bound from one origin
  carries the same input schema map, by address.
  `TestCatalog_BindTo_RebindsEveryHandlerAndSharesTheRest` is the same
  statement at the catalog level.
- `TestToolManifest_SharedSnapshotServesTheSameBytesAsAPrivateOne` reads
  `gitlab://tools` and `gitlab://tools/{id}` from two servers sharing a
  snapshot and from a third that built its own, and compares the bytes.
  `TestSharedMetaCatalog_SafeModePreviewsOnEveryServerOfOneKey` calls a
  mutating action on two servers of one shared safe-mode catalog and checks
  that both preview and neither instance is asked for anything.
  `TestBuild_ACacheHitStillCarriesTheScopeCause` checks that the second
  `dynamiccatalog.Build` for a narrowed credential, which is a cache hit,
  still reports what the credential withheld.
- `TestSharedMetaCatalog_ConcurrentEntriesBuildSafely` builds several pool
  entries at once from one fresh configuration. It is written for the race
  detector, which no CI job runs: `make test-race` runs the unit suite under
  it locally, and that is where this test earns its keep.
- The two transport end-to-end modules (`make test-e2e-http`,
  `make test-e2e-stdio`) drive the real binary.

One check behind this work is not reproducible and is recorded as what it was.
While the change was being made, the base binary and the changed binary were
driven over stdio through the same requests on several configurations and their
responses compared by hand. That exploratory comparison is what found the
compact and full envelopes changing under compilation, and why they are shared
as maps rather than compiled; the goldens above are what pins that finding from
now on.

## The server itself: one per configuration shape

The direction's last step is done: the pool no longer builds a server per
credential. It builds one per **configuration shape** and hands the same one to
every credential that hashes to it, with the pool entry reduced to credential
state. The decision, its evidence in the go-sdk source and the alternatives
weighed are recorded in
[ADR-0020](adr/adr-0020-one-server-per-configuration-shape.md); this is what the
code does.

**The shape** is what the catalog key already named plus what the shell reads:
tool surface, capability surface, meta parameter-schema mode, tier and whether
it was pinned, whether the instance is GitLab.com, read-only including the
token-scope narrowing, safe mode, the excluded tools, the token scopes and the
transport's statelessness (`serverShapeKey` in `cmd/server/shape.go`). The
instance URL is not in it, since two instances of one tier share a catalog and
the client is per credential either way. `shapeServers` builds each shape at
most once, and registration runs behind that server's readiness gate, so the
first credential of a shape still does not wait for the catalog and every later
one finds it built. That build was 1.8 s on the dynamic surface and 3.0 s on
individual, and it used to be paid per credential and again after every
eviction.

**The credential travels with the request.** A shape server is registered with
the credential-less client for its instance class, and every handler resolves
the caller's client from the request context through
`(*gitlab.Client).For(ctx)`, falling back to that unbound client, which refuses
everything. The fallback is what makes an unattributed request fail rather than
run under whoever happened to build the shape. The resolution happens in
`WrapAction` and its three siblings, which covers every catalog action on every
surface, and in the 38 resource closures, the 37 prompt closures, the completion
handler and the interactive flows.

The channel is the per-POST carrier in `cmd/server/carrier.go`, for the reason
already recorded there: context values reach a handler in stateless mode, where
the session is connected with the POST's own context, and do not in stateful
mode, where it is connected with the initialize POST's. The gate resolves the
pool entry, stamps its `credentialState` on the HTTP request context, and
`bindCredential` reads it back and installs it, along with the client, on the
handler context. It is added after the telemetry, rate-limit, listen-ceiling and
subscription middlewares so that it runs before them, which is what lets each of
them read the right tenant's bucket, ceiling and watchers.

**Session ownership is recorded rather than derived.**
`ServerOptions.GetSessionID` takes no request, so the per-server session tag
stopped meaning anything the moment one server served more than one credential.
`sessionOwners` writes down the session, its ID and the owner when a request of
that session arrives already bound, forgets a session when `ServerSession.Wait`
returns, and forgets every session of an entry the pool evicts. The gate's
ownership check reads that map instead of parsing a prefix. Only the two kinds
of session something asks about are recorded, one carrying an ID and one that
subscribes: on the default stateless transport every other POST is its own
session, and recording those would cost an entry and a parked goroutine per
request for a fact nothing reads.

**Subscriptions split in two.** A `subscriptionShape` per shape holds the
polling options, the listen-stream registry and the handler index registration
publishes. A `subscriptionRuntime` per pool entry holds the watchers, because a
watcher polls with a credential and ADR-0015 makes its first read the
authorization check. Two credentials watching one URI have one watcher each, and
each stream records its owner so a watch stopping closes only that credential's
streams.

Delivery is filtered, since the SDK exposes no per-session send. Each entry
carries an opaque owner token minted from `crypto/rand.Text`, never derived from
the credential; the notifier stamps it into the notification's `_meta`, and a
sending middleware on the shape server forwards the notification with that key
removed to the sessions of the same owner, dropping everything else, including
any notification with no tag and any session with no recorded owner.

Two details of that middleware are load-bearing and neither is obvious. It reads
the **params**, not the request: the SDK's two delivery paths instantiate the
request generic differently, so a type assertion on the request matches one and
drops the other in silence, which is precisely what the first version did. And
it restores the key after the send, because the legacy path hands one params
value to every subscriber in turn and a key stripped for good would make every
session after the first look untagged. The `_meta` map is never written to: the
stripped one is a new map, and the shared one is where the SDK stamps its own
subscription id.

The invariant, stated so it can be tested: two credentials listening to the same
resource URI each receive exactly their own watcher's notifications, with their
own watch state in `_meta`, and a credential whose access has been revoked
receives nothing, because its own watcher stopped and the others' notifications
are filtered away from its sessions. It is checked on the wire in
`test/e2e/http/` and over the middleware itself in `cmd/server/`.

### Why it had to be a filter

Resource subscriptions were the one part with no per-credential seam. The go-sdk
keeps the subscription table per server (`Server.resourceSubscriptions`, URI
to session to request id), and `Server.ResourceUpdated` is the only exported
delivery: it notifies every session on that server subscribed to the URI.
`ServerSession` exposes no per-session notification (its senders are
`NotifyProgress`, `Log`, `Ping`, `ListRoots`, `CreateMessage` and `Elicit`),
and the 2026-07-28 form needs the listen request id the SDK stamps into
`_meta` itself. On a shared server, credential A's watcher would notify B's
session: each change delivered twice, A's watch state in B's `_meta`, and
after B's access is revoked, when its own watcher has stopped on the 401 or
404 while the SDK's table still holds its session for as long as the listen
is open, B would keep learning that the resource changed from A's polling.
ADR-0015 makes the first read the authorization check because every session
on one manager shares one token; a shared server keeps that at the polling
end and would lose it at the delivery end.

What made the filter possible is that both of the SDK's delivery paths build the
notification per session and run it through the server's **sending middleware**:
`notifySessions` in `shared.go` and `notifySubscribedSessions` in `server.go`
each call `newRequest(sess, params)` and then `handleNotify`, which dispatches
through the handler `AddSendingMiddleware` wraps. The session and the params are
both in hand there, and a middleware that returns without calling the next
handler does not send. The context is not a channel: both functions create a
fresh `context.Background()` with a ten second timeout, so nothing the caller of
`ResourceUpdated` puts on its own context arrives. That is why the owner travels
in the params.

The ways through that were weighed, and what each would have cost:

| Way                                                                                  | What it delivers                                                                             | What it costs                                                                                                                                                                                                                                                                                                                      |
| ------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Route `subscriptions/listen` to a per-credential subscription server, stateless only | A flat heap on the default transport; delivery stays per credential without an SDK change    | A second shell (templates and manager, no tools) and a gate that reads the body's method itself, since the SDK checks `Mcp-Method` against a single message only and a batch skips it; `--stateless=false` keeps a full server per credential, because a session lives on one server and `resources/subscribe` arrives on it later |
| Per-session delivery in the go-sdk                                                   | The shared server needs nothing else; each manager hands its update to the sessions it knows | `ServerSession.ResourceUpdated(ctx, params)` upstream, reading the session's own request id from the table; nothing here can use it until it is released                                                                                                                                                                           |
| Accept cross-notification within a shape                                             | Nothing to build                                                                             | Duplicates, wrong watch state, and the post-revocation leak above                                                                                                                                                                                                                                                                  |
| Keep one server per credential and shrink it                                         | Some of the remainder                                                                        | The remainder is the SDK tool table and closures, 1.1 MB on individual, and the bound descriptors and route maps, about 3 MB on dynamic; smaller, never flat                                                                                                                                                                       |

The fifth way is the one taken, and it is not in the table because it was not
seen until the SDK's sending path was read: tag the notification with its owner
and filter delivery on the way out. It needs no second shell, no body parser in
the authentication path, and it works identically on both transports. When
per-session delivery is released upstream, the tag and the filter go away and
nothing else changes. The ask is recorded in
[upstream-bugs.md](upstream-bugs.md).

### What this branch measured

Same driver and the same machine as the section above (AMD Ryzen 5 3550H, 8
threads, 60.8 GiB, kernel 6.1, Go 1.27.1), one HTTP process per surface,
credential counts 1, 2, 5, 10 and 20, ten seconds per step. Before is the branch
point, which is the shared-catalog work already described; after is this branch.

The first thing the run says is that the resident set under load is the wrong
instrument for this change, and it is worth stating rather than hiding, because
the headline numbers barely move:

| Surface      | Peak resident set per credential, before |    after | Peak at 20 credentials, before |   after |
| ------------ | ---------------------------------------: | -------: | -----------------------------: | ------: |
| `dynamic`    |                                  8.6 MiB |  7.3 MiB |                        360 MiB | 331 MiB |
| `meta`       |                                  7.4 MiB |  2.7 MiB |                        317 MiB | 262 MiB |
| `individual` |                                 30.5 MiB | 26.3 MiB |                        921 MiB | 835 MiB |

Those slopes are least-squares lines through the five steps, and most of what
they measure is not the credential. The driver keeps four requests in flight per
credential on `dynamic` and `meta` and two on `individual`, so at the last step
the process is serving eighty concurrent calls, and what they allocate while
they are being served is the bulk of the line. The previous section said the
same thing about its own residue and it is more true here, now that the part
sharing can reach has been removed: what is left of the slope is the load, not
the tenancy.

The "before" column here does not reproduce the previous section's "after"
column, although it is the same binary: 8.6 against 4.10 MiB on `dynamic`, 7.4
against 5.44 on `meta`, 30.5 against 11.28 on `individual`. Two differences
account for that and neither is a disagreement about the code. The series are
different shapes: ten steps out to a thousand credentials there, five steps out
to twenty here, and a least-squares line through five short steps is dominated
by the intercept the previous section reports separately (263, 139 and 756 MiB).
And this run shared the machine with other work. So read every resident-set
slope on this page as an order of magnitude, and read the live-heap figure below
as the measurement: it is the one taken on an idle process, which is what a
credential costs when nobody is calling.

The credential's own cost is what an idle process holds, and that is what this
change was aimed at. Driving one `tools/list` per credential and reading
`HeapAlloc` from `/debug/pprof/heap?gc=1&debug=1` after a collection, on the
same two binaries, twenty credentials each:

| Surface      | Live heap per credential, before |  after | Ratio |
| ------------ | -------------------------------: | -----: | ----: |
| `dynamic`    |                          434 KiB | 17 KiB |   26x |
| `meta`       |                          815 KiB | 73 KiB |   11x |
| `individual` |                        1,487 KiB |  8 KiB |  186x |

In absolute terms the twentieth credential adds 0.3 MiB on `dynamic`, 1.4 MiB on
`meta` and 0.2 MiB on `individual` over the first, against 8, 15 and 27 MiB
before. That is the target: what a credential costs is now the pool entry (a
GitLab client, a rate-limit bucket, a listen counter, a watcher set) rather than
a registered tool surface, and the registered surface is paid once per
configuration.

The e2e test `TestSharedServer_LiveHeapDoesNotGrowWithTheNumberOfCredentials`
takes the same measurement on every push, on the `dynamic` and `individual`
surfaces, and fails when the growth over 1 to 20 credentials exceeds 2 MiB. That
budget is what makes it an assertion: the growth on this branch is under 0.2 MiB
on both surfaces, and a revert to a server per credential grows 8.1 MiB on
`dynamic` and 27.6 on `individual`, both measured by running the same test on
the parent commit. It carried a 32 MiB budget until this pass, which that same
revert passed on every surface, so what the test pinned was nothing.

Per-call processor cost is unchanged, which is expected: 20.0 ms before and
20.2 ms after on `dynamic`, 19.2 and 19.1 on `meta`, 357 and 342 on
`individual`. It was never this work's target, and the options for it are
enumerated at the end of
[What remains, per credential and per call](#what-remains-per-credential-and-per-call).
None of them is implemented.
