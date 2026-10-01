// Package apilive is the committed record of what a booted GitLab says its
// own REST API is, and the one reader of it.
//
// Every other oracle this repository holds about GitLab's REST API is a
// reading of text: the OpenAPI document GitLab commits to its own repository,
// or a scan of the Grape source that document is generated from. Both are
// downstream of the object that decides what a request returns, which is the
// Rails application with its classes loaded, and both lose the same thing when
// a name is not written down. GeoSiteStatus exposes its fields by iterating a
// constant assembled from two method calls: the source says "expose the loop
// variable", a scanner reads 26 fields, and GitLab sends 606. That is not a
// hole a better parser closes.
//
// So this record is produced by asking the application. cmd/gen_api_live boots
// a released GitLab image, runs one script inside it, and writes what comes
// back here; every audit then reads this file with no Docker and no network,
// the way cmd/gen_graphql_schema's pin is read. The boot is a generator, never
// an audit: an audit that needed a container could not be a gate.
//
// One thing evaluation does not give and the record therefore carries from
// source: a block condition is a Proc, and a Proc knows where it was written
// but not what it says. The generator reads those lines back from inside the
// same image, so a block condition arrives here both located and quoted. A
// hash or a symbol condition is the other way round: it holds what it tests,
// which the record keeps as its data or its option, and grape-entity keeps no
// location for it, so it arrives quoted and never located.
//
// Since schema version 4 the record also says what a fine-grained personal
// access token needs: each route's authorization, the permission vocabulary a
// token is granted in, the anonymous policy on a public project and group,
// and what each GraphQL type, mutation and field demands (authorization.go).
package apilive
