// Command gen_orbit_record records what GitLab.com's Orbit routes answer and
// commits the key tree of each answer to docs/development/orbit-responses.json.
//
// The six Orbit actions were outside every oracle the 1:1 audit reads: no
// self-managed GitLab serves Orbit, so the live API record has nothing to say
// about them, and GitLab's generated OpenAPI document gives their routes no
// response schema. A recorded answer is the one oracle that covers all six
// output types, so this command makes one, through the handlers themselves.
// It starts a recording proxy on the loopback interface, points a client of
// this repository's own at it, calls each Orbit handler with the inputs the
// record expects, and keeps what GitLab answered each request the handler
// built. The requests recorded are therefore the ones the server sends,
// parameter names included, and a handler that builds a request GitLab
// refuses fails the recording rather than recording a refusal.
//
// Only the shape is kept: every path an answer carried and the JSON kinds
// seen there, never a value, with a subtree whose keys are data (a JSON
// Schema document, the rows of a query, a component's metrics) kept as its
// root only. See
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord].
//
// Recording needs GITLAB_COM_TOKEN and the network, and every request it makes
// of GitLab.com is a read, so it is run by hand or by make test-e2e-gitlab-com
// and never gates. It refuses to record over a fixture namespace the indexer
// has not reached, whose answers would lack the rows and counts the record
// holds the shape of. It compares what it recorded with the record committed
// at HEAD, read through git rather than from the file it has just replaced,
// and prints every key added, dropped or changed, and a query DSL whose $id or
// version moved (read from the raw orbit.dsl answer into the record's source);
// when the key tree or the DSL differs it still writes the new record and
// exits 1, and every later recording exits 1
// too until somebody has read the change and committed the record. A -dir
// outside the repository, a record HEAD does not hold yet, or a machine with
// no git leaves nothing to compare with, which the run says and passes. -check
// is the offline half and the gate: it holds the committed record to
// [github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/orbitrecord.Problems]
// and to the form this command writes, with no token and no network.
//
// Usage:
//
//	go run ./cmd/gen_orbit_record/                 # record (GITLAB_COM_TOKEN set)
//	go run ./cmd/gen_orbit_record/ -namespace acme # another fixture namespace
//	go run ./cmd/gen_orbit_record/ -check          # the offline gate
package main
