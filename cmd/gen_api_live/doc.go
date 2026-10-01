// Command gen_api_live pins what a booted GitLab says its own REST API is.
//
// It boots a released gitlab-ee image, runs one Ruby script inside it through
// gitlab-rails runner, and writes the answer to
// docs/development/gitlab-api-live.json with provenance. Every audit then
// reads that record offline. The boot is a generator and never a gate: an
// audit that needed Docker could not run in CI or on a contributor's machine,
// which is the same division cmd/gen_graphql_schema already keeps.
//
// # Why a boot rather than a read
//
// The two records this replaced were both readings of text: the OpenAPI
// document GitLab generates from its Grape definitions, fetched, and the Grape
// source itself, scanned for the condition each field is sent under. Both were
// downstream of the object that decides what a request returns, and both lost
// the same thing, a name that is not written down. Both are gone.
//
// Measured against this record on GitLab 19.3.1-ee, with the static record's
// inheritance resolved: 551 of 582 entities agree exactly and 31 do not, and
// in those the instance has 1935 fields the scanner never saw. GeoSiteStatus
// is 606 against 26, ApplicationSetting 680 against 81, MemberRole 50 against
// 5. None of that is a parser bug. GeoSiteStatus exposes its fields by
// iterating a constant assembled from two method calls, so the source says
// "expose the loop variable" and the names exist only after the class loads.
//
// The same holds for conditions. The instance carries 914 where the scan finds
// 388, and every one of them is recorded as what it tests: 873 block
// conditions with their text read back from the image, 32 symbol conditions
// with the option they name, and 9 hash conditions with their data. And for the
// licensed feature table, where the scan's own source concedes it cannot read
// the lists the table builds by concatenation.
//
// Until schema version 3 the 41 hash and symbol conditions arrived as their
// kind alone. The script asked for an instance variable grape-entity never
// sets and had no key for a symbol, and an unreadable condition was then
// dropped by the reader, so the 41 fields they gate, custom_attributes on
// seventeen entities among them, read downstream as sent on every response.
// The script now goes through each condition class's public reader, which
// raises rather than reads nil if grape-entity renames it, and this command
// refuses to write or to pass a record holding a condition that carries
// neither text, hash nor symbol.
//
// # Fine-grained authorization
//
// Since schema version 4 the same boot records what each route and each
// GraphQL element demands of a fine-grained personal access token, which is
// what issue 952 needs to say per action which permissions a token must be
// granted. GitLab's own permission tasks walk the same objects and are not
// loaded in a production image, so the script repeats their walk:
//
//   - every route's `route_setting :authorization`, key for key as
//     lib/api/helpers.rb reads it, a callable boundary located and quoted
//     like a block condition, and any key it does not know recorded by name;
//   - the permission vocabulary: every assignable permission, deprecated ones
//     kept, with the raw permissions it expands to and the words GitLab's
//     refusal prints, every raw permission, and for each raw one the first
//     assignable GitLab names and the first one a token can actually be
//     granted, which differ for seventeen at 19.4.1;
//   - the anonymous policy evaluated on an unsaved public project, with every
//     feature enabled and every licensed feature made available, and on an
//     unsaved public group, which is what GitLab serves a token beyond its
//     grant; the role file is a lower bound of it;
//   - every GraphQL object type with whether the granular check runs on it,
//     its abilities, its directives and the signature of every object-typed
//     field, every union and interface with its possible types, every
//     mutation and field-level directive, and the undeclared set computed with
//     GitLab's own todo rule beside the authorization_todo.txt the image ships.
//
// -check refuses a version 4 record that lacks either block, falls below the
// fine-grained floors, maps a raw permission to an assignable that is not its
// own or offers one no token can hold, declares a permission GitLab does not
// define or no assignable expands to, declares permissions with no boundary,
// names a boundary type GitLab does not resolve, carries a key or argument
// nothing reads, holds a directive with both a skip and permissions or
// neither, computes an undeclared set that differs from GitLab's list, leaves
// an abstract type without members or a field leading nowhere, or holds a
// public set that does not say how it was produced. The generator refuses to
// write the same record.
//
// # What it cannot give
//
// One released version and one edition. The static record is pinned to master,
// so a field merged after the latest release is in that record and not in this
// one. For a 1:1 surface that is the right direction, since an endpoint nobody
// can call yet is not a gap, but it is a difference and the record says which
// version it is.
//
// Nor where a hash or a symbol condition was written. grape-entity keeps a
// block condition's Proc, which knows its file, and for the other two kinds
// only the options they test, while an exposure records no location of its
// own. The record therefore quotes all three kinds and locates only blocks, so
// an edition read from a condition's file never answers ee for the other two:
// an epic's reference is gated by `if: { with_reference: true }` in
// ee/lib/api/entities/epic.rb and reads with no edition.
//
// And it cannot correct a wrong annotation. GET /api/v4/keys is annotated
// APIEntitiesUserWithAdmin and the endpoint presents an SSH key with a user
// under it; reading the annotation from the running router gives the same
// wrong answer as reading it from the document, because it is the same
// annotation. Only calling the endpoint settles that.
//
// # Usage
//
//	go run ./cmd/gen_api_live/                       # boot, introspect, write
//	go run ./cmd/gen_api_live/ -check                # gate the committed record, no Docker
//	go run ./cmd/gen_api_live/ -dump introspect.json # write from an existing dump
//	go run ./cmd/gen_api_live/ -keep                 # leave the container running
//
// The boot takes a few minutes the first time, while the image is pulled, and
// about forty seconds afterwards. It needs no license and no fixtures: the
// Enterprise classes are loaded whatever the license says, because a license
// gates feature_available? when a request is served and not when a class is
// defined.
package main
