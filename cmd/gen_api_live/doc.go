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
// The two records this replaces are both readings of text. gen_api_shapes
// fetches the OpenAPI document GitLab generates from its Grape definitions;
// gen_api_exposes scans the Grape source itself for the condition each field
// is sent under. Both are downstream of the object that decides what a request
// returns, and both lose the same thing: a name that is not written down.
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
// 388, and for 873 of them the text can be read back. And for the licensed
// feature table, where the scan's own source concedes it cannot read the lists
// the table builds by concatenation.
//
// # What it cannot give
//
// One released version and one edition. The static record is pinned to master,
// so a field merged after the latest release is in that record and not in this
// one. For a 1:1 surface that is the right direction, since an endpoint nobody
// can call yet is not a gap, but it is a difference and the record says which
// version it is.
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
