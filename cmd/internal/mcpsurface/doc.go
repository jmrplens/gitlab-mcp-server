// Package mcpsurface is the one reader of the MCP surface this server serves.
//
// Several commands under cmd/ need the same thing: the tools, prompts and
// resources the server actually registers, read over a real MCP round-trip
// rather than described by hand, and against a surface that does not depend on
// the ambient environment. The server chooses its catalog from TOOL_SURFACE and
// its client from GITLAB_URL/GITLAB_TOKEN, so a generator that read either would
// emit different files on a developer machine than in CI. Every constructor here
// pins the surface explicitly and talks to an in-process stub instead, which is
// what makes the committed artifacts reproducible.
//
// What it offers is one listing per surface — [IndividualTools], [MetaTools],
// [DynamicTools], [Resources] and [Prompts] — over the offline client
// [NewStubClient] builds, each registering exactly what cmd/server registers
// for that surface and tier.
//
// Three properties are the reason this is one package rather than a helper per
// command, and each of them was a defect in the readers it replaced:
//
//   - [Session] applies the served schema chain, LockdownInputSchemas then
//     EnrichPaginationConstraints, in the order cmd/server installs them. A
//     listing that applies neither describes a schema no client receives.
//   - [requireCompleteListing] stops a run whose listing came back truncated,
//     rather than letting a first page be published as the whole surface.
//   - Listings are memoized on (client, surface, tier, meta parameter-schema
//     mode), because registering a full surface costs seconds and every caller
//     only reads the result. A caller must not sort a returned slice in place.
package mcpsurface
