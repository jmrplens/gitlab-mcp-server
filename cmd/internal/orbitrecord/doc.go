// Package orbitrecord is the committed record of what GitLab.com's Orbit
// routes answer, and the one reader of it.
//
// It exists because the six Orbit actions were outside every oracle the 1:1
// audit reads. The live API record (cmd/internal/apilive) is taken from a
// booted self-managed GitLab, and no self-managed instance serves Orbit; the
// routes it does list carry no response entity, because
// ee/lib/api/orbit/data.rb presents hashes that
// ee/lib/analytics/knowledge_graph/grpc_client.rb builds by hand from gRPC
// messages. GitLab's generated OpenAPI document gives no 200 schema for them
// either. A recorded answer is therefore the only oracle that covers every
// Orbit output type, and this is that recording, reduced to what a comparison
// needs and nothing a reader could mistake for data.
//
// What it keeps is a key tree per recorded call: every path a response
// carried, each with the JSON kinds seen there, and never a value. A subtree
// whose keys are chosen by data rather than by GitLab's code (a JSON Schema
// document, the rows a query asked for by column) is kept only as its root,
// marked verbatim, because the server passes it through as it came and its
// keys are not a response shape. Each call also names the canonical action
// that made it, the Go type that action returned, and the query and body
// parameter names the handler sent, which is what lets the audit join a
// recorded answer to an output type without inferring anything: the join is
// the handler the generator called.
//
// Its source names, beside the instance, the Orbit version, the fixture
// namespace and the day, the query DSL GitLab.com served when the record was
// taken: the $id and version of the JSON Schema orbit.dsl returns. That is
// the one part of the provenance [Diff] compares, because it is the language
// orbit.query teaches a model, and a new one is a change somebody has to read
// that guidance against (issue 1031).
//
// cmd/gen_orbit_record writes it from a run against GitLab.com, which needs a
// token and the network and so can never gate; its -check reads the committed
// file with neither, holds it to [Problems] and to its own canonical form,
// and cmd/audit_1to1's R-PATH scope compares the Orbit output types against
// it. The retrieval date is held to the window
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/provenance] records
// for every pinned truth.
package orbitrecord
