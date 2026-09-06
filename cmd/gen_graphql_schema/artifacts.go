package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/graphqlschema"
)

// compress gzips the SDL at the best ratio. The SDL is close to a megabyte and
// compresses to a sixth of that, which is the difference between an artifact a
// reviewer can live with in a diff and one nobody wants in the repository.
//
// Nothing here can fail: the level is a constant, and a bytes.Buffer accepts
// every write, which is why the writes go through cmdutil.MustDo rather than
// growing a return path no caller could act on.
func compress(sdl string) []byte {
	var buffer bytes.Buffer
	writer := cmdutil.Must(gzip.NewWriterLevel(&buffer, gzip.BestCompression))
	cmdutil.MustDo(writeAll(writer, sdl))
	cmdutil.MustDo(writer.Close())
	return buffer.Bytes()
}

// writeAll writes the SDL and discards the byte count, which a bytes.Buffer
// always reports in full.
func writeAll(writer *gzip.Writer, sdl string) error {
	_, err := writer.Write([]byte(sdl))
	return err
}

// writeArtifacts writes the compressed schema and its provenance record into
// dir, creating it when it does not exist.
func writeArtifacts(dir string, compressed []byte, source graphqlschema.Source) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	schemaPath := filepath.Join(dir, graphqlschema.SDLFileName)
	if err := os.WriteFile(schemaPath, compressed, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", schemaPath, err)
	}

	// Marshaling a struct built one line earlier cannot fail, and the trailing
	// newline is what keeps the file from being the one text file in the
	// repository without one.
	record := append(cmdutil.Must(json.MarshalIndent(source, "", "  ")), '\n')
	recordPath := filepath.Join(dir, graphqlschema.SourceFileName)
	if err := os.WriteFile(recordPath, record, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", recordPath, err)
	}
	return nil
}

// readArtifacts loads the committed schema and record from dir. This is what
// --check asks, and it deliberately reads the files rather than the embedded
// copies: the embedded ones are compiled into this binary, so asking them
// whether the files on disk are sound would answer about the wrong thing after
// an interrupted write.
func readArtifacts(dir string) (int, graphqlschema.Source, error) {
	schemaPath := filepath.Join(dir, graphqlschema.SDLFileName)
	compressed, err := os.ReadFile(schemaPath)
	if err != nil {
		return 0, graphqlschema.Source{}, fmt.Errorf("read %s: %w", schemaPath, err)
	}
	schema, err := graphqlschema.Load(compressed)
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
