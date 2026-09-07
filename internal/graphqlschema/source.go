package graphqlschema

import (
	_ "embed" // registers the go:embed directive that carries the pin's provenance
	"encoding/json"
	"fmt"
)

// SourceFileName is the provenance record's name on disk, written by
// cmd/gen_graphql_schema beside the compressed schema.
const SourceFileName = "source.json"

//go:embed source.json
var sourceRecord []byte

// Source describes where the pinned schema came from, so a reader can tell how
// old the pin is without asking git. A schema that parses says nothing about
// whether the instance it was taken from still answers this way.
type Source struct {
	// Instance is the GraphQL endpoint that was introspected.
	Instance string `json:"instance"`
	// GitLabVersion is what that instance reported for itself, or "unknown"
	// when the introspection ran without a token: GitLab answers introspection
	// to anyone and refuses the metadata query to an anonymous caller.
	GitLabVersion string `json:"gitlab_version"`
	// GitLabRevision is the commit that instance was running, when known.
	GitLabRevision string `json:"gitlab_revision,omitempty"`
	// RetrievedAt is the UTC day the instance answered, as YYYY-MM-DD.
	RetrievedAt string `json:"retrieved_at"`
	// Types is how many types the introspection carried, which is the one
	// number that says at a glance whether a regeneration got the whole
	// schema or a truncated answer.
	Types int `json:"types"`
}

// SourceInfo returns the provenance of the embedded schema.
func SourceInfo() (Source, error) {
	return ParseSource(sourceRecord)
}

// ParseSource decodes a provenance record. It is exported so the generator can
// check a file on disk rather than the embedded copy.
func ParseSource(record []byte) (Source, error) {
	var source Source
	if err := json.Unmarshal(record, &source); err != nil {
		return Source{}, fmt.Errorf("parse %s: %w", SourceFileName, err)
	}
	return source, nil
}

// String renders the provenance as one reportable line.
func (s Source) String() string {
	return fmt.Sprintf("%d types from %s (GitLab %s), retrieved %s",
		s.Types, s.Instance, s.GitLabVersion, s.RetrievedAt)
}
