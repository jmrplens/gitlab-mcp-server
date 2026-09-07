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
// live instance and writes gitlab-schema.graphql beside source.json, which
// records the instance, the version it reported and the day it answered, so a
// reader can tell how old the pin is. The SDL is committed as text, close to a
// megabyte of it, because a re-pin is the one moment somebody needs to read
// what GitLab changed; git stores two revisions of the text in about half the
// space it needs for two gzip streams, which it can neither delta nor diff.
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
// cmd/audit_graphql_documents -schema is how that last gap is closed on
// demand, against a schema fetched rather than pinned.
//
// Three limits are inherent to validating at this layer rather than at
// GitLab's, and are worth knowing before assuming more of a green run than it
// offers. A fractional number passes where Int is declared, because JSON has
// already made every number a float64 by the time the value is seen. A custom
// scalar accepts anything, so a malformed global id passes as a VulnerabilityID.
// And no depth or complexity limit is enforced, so a query GitLab would refuse
// as too expensive is accepted here. The pin also carries no deprecation
// marks, so it reports a document that is already broken and never one that is
// about to be.
//
// Enum case is not among those limits, and used to be. gqlparser compares an
// enum value with strings.EqualFold, so it accepts "critical" for
// VulnerabilitySeverity, while GitLab answers that it expected one of INFO,
// UNKNOWN, LOW, MEDIUM, HIGH, CRITICAL and executes nothing. [Validate] walks
// the variables itself for that one reason.
package graphqlschema
