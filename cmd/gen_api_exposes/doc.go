// Command gen_api_exposes records, for every field a GitLab REST entity
// exposes, the condition under which GitLab sends it, read out of GitLab's
// Ruby source.
//
// The OpenAPI record (make gen-api-shapes) is an upper bound: it lists a
// field an entity exposes under a condition as if it were always sent. This
// command reads the Grape entities the document was generated from, the
// Enterprise modules prepended into them, and GitLab's licensed-feature table,
// and writes docs/development/gitlab-api-exposes.json: entity by entity, the
// fields in declaration order, each with its `if:` and `unless:` as written,
// the licensed feature symbols the condition names and the tier the table
// puts them under, the entity a value is rendered with, and the fields a
// block nests. cmd/internal/apiexposes is what reads the source and what a
// consumer reads the record through; the reading's limits are documented
// there.
//
// The sources are fetched from gitlab-org/gitlab as three subtree archives
// (lib/api/entities, ee/lib/api/entities, ee/lib/ee/api/entities) and one
// file (ee/app/models/gitlab_subscriptions/features.rb), all at one ref. The
// archives name the commit the ref resolved to, and a set of archives that
// resolved to different commits, which a push to master between two downloads
// produces, is refused rather than recorded as one tree.
//
// Usage:
//
//	go run ./cmd/gen_api_exposes/                       # fetch master and write the record
//	go run ./cmd/gen_api_exposes/ -ref v19.4.0-ee       # a tag instead
//	go run ./cmd/gen_api_exposes/ -source ~/src/gitlab  # read a local checkout, no network
//	go run ./cmd/gen_api_exposes/ -check                # CI: the committed record is usable
//	go run ./cmd/gen_api_exposes/ -report APIEntitiesProject
package main
