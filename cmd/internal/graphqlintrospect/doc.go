// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Package graphqlintrospect asks a GitLab instance for its GraphQL schema and
// converts the answer to SDL.
//
// It exists because two commands need the same fetch. cmd/gen_graphql_schema
// pins gitlab.com's schema into the repository, and cmd/audit_graphql_documents
// judges every document this server sends against a schema fetched from a named
// instance right now. Those are the same three steps, and a second copy of them
// would be a second set of introspection quirks to keep in step: the canned
// answer every instance returns whatever is asked, the deprecation arguments
// that an older instance refuses, and the wrapper depth a type reference nests.
//
// [FromSDL] is the same conversion entered from the other side: it reads a
// schema written as SDL, the dump GitLab's own CI keeps for a release tag,
// into the introspection shape, so the release pin cmd/gen_graphql_schema
// -release writes is rendered exactly as the gitlab.com pin is.
//
// The conversion is deliberately lossy. Descriptions and deprecation reasons
// are dropped because validation never consults them and they would triple the
// file, and everything nameable is sorted so re-fetching from an instance that
// reorders its answer produces a diff of what changed rather than a reshuffle.
package graphqlintrospect
