// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package graphqlschema

import (
	_ "embed" // registers the go:embed directives that carry the release pin
	"encoding/json"
	"fmt"
	"sync"

	"github.com/vektah/gqlparser/v2/ast"
)

// ReleaseSDLFileName is the release pin's schema on disk: the GraphQL schema
// of the GitLab release the REST record (docs/development/gitlab-api-live.json)
// and the fine-grained table are taken from, rendered the way the gitlab.com
// pin is. Hyphenated for the reason [SDLFileName] gives.
const ReleaseSDLFileName = "gitlab-release-schema.graphql"

// ReleaseSourceFileName is the release pin's provenance record, written by
// cmd/gen_graphql_schema -release beside the schema text.
const ReleaseSourceFileName = "release-source.json"

//go:embed gitlab-release-schema.graphql
var releaseSDL []byte

//go:embed release-source.json
var releaseSourceRecord []byte

// releaseOnce guards the one parse of the release pin a process pays for, for
// the reason schemaOnce gives about the gitlab.com pin.
var (
	releaseOnce sync.Once
	release     struct {
		schema *ast.Schema
		err    error
	}
)

// ReleaseSchema returns the schema of the released GitLab this server is held
// to, parsing the embedded SDL on first call.
//
// It exists because the gitlab.com pin is a pre-release ahead of every
// self-managed instance. A field the pre-release adds passes every gate that
// reads that pin and is refused by every release, and GitLab refuses the whole
// document naming it: the re-pin of 2026-10-29 let crashState into the
// location selection seven security actions share, the gates stayed green, and
// GitLab 19.5.3 answered them with "Field 'crashState' doesn't exist on type
// 'VulnerabilityLocationCoverageFuzzing'". cmd/audit_graphql_documents judges
// every document against this schema as well as the pin, so a field the
// release lacks is a failure on the commit that selects it.
//
// The returned schema is read-only and shared, as [Schema]'s is.
func ReleaseSchema() (*ast.Schema, error) {
	releaseOnce.Do(func() { release.schema, release.err = Load(releaseSDL) })
	return release.schema, release.err
}

// ReleaseSource describes where the release pin came from.
//
// It is a record of its own rather than a [Source] because it answers other
// questions. A [Source] names an instance that was introspected, and the
// release pin is not taken from one: it is the schema GitLab's own CI dumped
// for the release's tag, so what a reader needs is which release, which commit
// and which job, and a digest of the bytes the job published.
type ReleaseSource struct {
	// GitLabVersion is the release, spelled as the REST record spells its own
	// version ("19.5.0-ee"). cmd/gen_graphql_schema -release --check holds the
	// two to one another, which is what makes this the release the rest of the
	// repository is pinned to rather than one somebody happened to fetch.
	GitLabVersion string `json:"gitlab_version"`
	// GitLabRevision is the commit the release's tag points at, which the REST
	// record names as the revision its image reported.
	GitLabRevision string `json:"gitlab_revision"`
	// Origin is where the schema text was read from: the graphql-schema-dump
	// job of the tag's pipeline, by the URL that names the job, or the file a
	// re-pin was handed instead.
	Origin string `json:"origin"`
	// SHA256 is of the schema text as it was read, before it was rendered
	// into the form committed here, so a reader can compare it with the job's
	// artifact without trusting the renderer.
	SHA256 string `json:"sha256"`
	// RetrievedAt is the UTC day the schema text was read, as YYYY-MM-DD.
	RetrievedAt string `json:"retrieved_at"`
	// Types is how many types the schema beside this record loads with, which
	// the check recomputes from the file and holds the record to, as it does
	// for the gitlab.com pin.
	Types int `json:"types"`
}

// ReleaseSourceInfo returns the provenance of the embedded release pin.
func ReleaseSourceInfo() (ReleaseSource, error) {
	return ParseReleaseSource(releaseSourceRecord)
}

// ParseReleaseSource decodes a release provenance record. It is exported so
// the generator can check a file on disk rather than the embedded copy.
func ParseReleaseSource(record []byte) (ReleaseSource, error) {
	var source ReleaseSource
	if err := json.Unmarshal(record, &source); err != nil {
		return ReleaseSource{}, fmt.Errorf("parse %s: %w", ReleaseSourceFileName, err)
	}
	return source, nil
}

// String renders the provenance as one reportable line.
//
//gitlab:allow-raw s.RetrievedAt: a date the committed record carries as text, printed for a log rather than for a card
func (s ReleaseSource) String() string {
	return fmt.Sprintf("%d types of GitLab %s (%s), from %s, retrieved %s",
		s.Types, s.GitLabVersion, s.GitLabRevision, s.Origin, s.RetrievedAt)
}
