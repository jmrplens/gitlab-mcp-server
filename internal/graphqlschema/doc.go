// Package graphqlschema holds the pinned GitLab GraphQL schema and validates
// documents against it.
//
// Every GraphQL test in this repository answers the request from an httptest
// handler that returns whatever the test wrote, so the document itself used to
// be judged by nobody: our code and our fixture agreed with each other and
// both could be wrong about GitLab. Four registered tools shipped with green
// tests and documents no current instance accepts, and the same blindness let
// eight domains advertise a backward pagination no operation declared.
//
// GitLab is the only party that refuses a document, and no unit test may reach
// it, so the schema comes here instead. cmd/gen_graphql_schema introspects a
// live instance and writes gitlab-schema.graphql.gz beside source.json, which
// records the instance, the version it reported and the day it answered, so a
// reader can tell how old the pin is. The schema is embedded compressed
// because the SDL is close to a megabyte and the compressed form is a sixth of
// that.
//
// # Cost
//
// Parsing the schema takes around 200 ms for 4331 types, so it happens once
// per process behind a [sync.Once] and never per call. Validating one document
// against the loaded schema costs tens of microseconds, which is what makes
// running it inside the shared test transport affordable.
//
// # What it cannot see
//
// The pin is a snapshot, and GitLab narrows fields in place: the
// securityReportFindings confidence argument and the [String!] typing of
// Project.vulnerabilities.severity were both valid once. A document this
// package accepts is one the pinned instance accepted on the day recorded in
// source.json, which is a far stronger statement than the mocks used to make
// and still not the same as one a live instance accepts today.
package graphqlschema
