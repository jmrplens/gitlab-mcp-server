// Command gen_api_shapes pins what GitLab says its own REST API accepts and
// returns, and gates the committed record.
//
// GitLab generates an OpenAPI 3 document from the Grape definitions that render
// its responses and commits it to its own repository, where it is served
// unauthenticated. This fetches that document, extracts the three lists a
// comparison with our types needs per operation, and writes
// docs/development/gitlab-api-shapes.json.
//
// Two things make it the oracle the audit lacked. It needs no running instance,
// no license and no image, so the check that reads it is a gate rather than a
// scheduled job. And gitlab-org/gitlab is the Enterprise codebase, so the
// document covers the Premium and Ultimate surface, which is precisely the half
// a Community Edition instance would never show and where every broken tool
// this repository has found so far lived.
//
// `--check` reads the committed record without the network and refuses one that
// is not what it claims: a schema version this build does not read, an
// extraction too short to be GitLab's whole API, or a record older than the
// window. It says nothing about whether the record still matches GitLab, which
// only a regeneration can answer, and which is why the window exists at all.
// The window itself, and the verdict passed on the retrieval date, are
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance]'s, shared
// with the two other records this repository pins; the floor, the identity
// checks and the record's own dialect stay here.
package main
