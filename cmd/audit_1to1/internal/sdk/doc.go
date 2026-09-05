// Package sdk holds the two 1:1 audit rules whose universe comes from the SDK
// rather than from this repository's own call sites (R-SERVICE, R-GRAPHQL).
//
// The action-coverage scope answers "which methods of a service we call are
// uncovered". It cannot answer "is there a service we call nothing on", because
// its universe is built by walking our call expressions: a service nothing
// references never enters the map, so an entire upstream service can be added
// and the audit still report zero gaps. That is exactly what happened when
// WorkItemSavedViewsService arrived with seven methods.
//
// So this scope enumerates the services from the client-go Client struct and
// holds each one to a decision: covered by a call, or declared an exception
// with a category and a reason. A service that is neither is a finding.
//
// It carries a second rule for the same reason. ADR-0006 admits raw
// GraphQL.Do() for domains WITHOUT a client-go service wrapper. The wrapper
// appearing later is what retires that exemption, and nothing was checking, so
// the exemption was permanent in practice. Every raw GraphQL operation whose
// package maps to a client-go service is therefore held to a decision too,
// per operation rather than per package: packages that use GraphQL for one
// operation and the SDK for the rest are the norm, so a package-level verdict
// would be mostly noise.
//
// Unlike the three candidate-backlog scopes, this one is a gate: Run reports
// whether the tree is clean, and the command exits non-zero when it is not.
package sdk
