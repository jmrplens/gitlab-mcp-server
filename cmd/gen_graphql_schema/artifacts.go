package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/docgen"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// writeArtifacts writes the schema and its provenance record into dir,
// creating it when it does not exist.
//
// The SDL is committed as text rather than compressed. It is close to a
// megabyte, which is the whole argument for gzipping it, and gzipping loses
// more than it saves: git zlib-compresses and deltas a text blob and can do
// neither to a gzip stream, so two revisions of the compressed form cost about
// twice the history of two revisions of the text. It also costs the review. A
// re-pin is the one moment somebody needs to see what GitLab changed, and a
// binary blob renders as "Bin 0 -> 155364 bytes", cannot be merged, and cannot
// be grepped by anybody verifying a repair. Nothing weighs against that: the
// schema never reaches a released binary, since cmd/server does not depend on
// internal/graphqlschema at all.
// The directory, the mode and the trailing newline both files end with are
// docgen.WriteOrCheck's decisions, taken in write mode: this pin's freshness is
// judged by its own age window rather than by comparing it with a fresh
// introspection, which needs an instance.
func writeArtifacts(dir, sdl string, source graphqlschema.Source) error {
	if err := docgen.WriteOrCheck(filepath.Join(dir, graphqlschema.SDLFileName),
		[]byte(sdl), false, "make gen-graphql-schema"); err != nil {
		return err
	}
	// Marshaling a struct built one line earlier cannot fail.
	record := cmdutil.Must(json.MarshalIndent(source, "", "  "))
	return docgen.WriteOrCheck(filepath.Join(dir, graphqlschema.SourceFileName),
		record, false, "make gen-graphql-schema")
}

// readArtifacts loads the committed schema and record from dir. This is what
// --check asks, and it deliberately reads the files rather than the embedded
// copies: the embedded ones are compiled into this binary, so asking them
// whether the files on disk are sound would answer about the wrong thing after
// an interrupted write.
func readArtifacts(dir string) (int, graphqlschema.Source, error) {
	schemaPath := filepath.Join(dir, graphqlschema.SDLFileName)
	sdl, err := os.ReadFile(schemaPath)
	if err != nil {
		return 0, graphqlschema.Source{}, fmt.Errorf("read %s: %w", schemaPath, err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return 0, graphqlschema.Source{}, fmt.Errorf("%s: %w", schemaPath, err)
	}

	recordPath := filepath.Join(dir, graphqlschema.SourceFileName)
	record, err := os.ReadFile(recordPath)
	if err != nil {
		return 0, graphqlschema.Source{}, fmt.Errorf("read %s: %w", recordPath, err)
	}
	source, err := graphqlschema.ParseSource(record)
	if err != nil {
		return 0, graphqlschema.Source{}, fmt.Errorf("%s: %w", recordPath, err)
	}
	return len(schema.Types), source, nil
}
