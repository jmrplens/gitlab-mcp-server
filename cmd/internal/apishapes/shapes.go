package apishapes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

const (
	// FileName is the committed extraction, beside the request inventory it is
	// compared with.
	FileName = "gitlab-api-shapes.json"
	// DefaultDir is where the record lives, repository relative. It is exported
	// so a reader joins on the same directory the generator writes to rather
	// than spelling it again.
	DefaultDir = "docs/development"
	// SchemaVersion is the artifact's shape. A reader that does not recognize
	// it must refuse rather than guess, since every field here is a list of
	// names a comparison acts on.
	//
	// Version 2 added [Operation.Nested]; version 3 added [Operation.Entity]
	// and [Operation.NestedEntity], the join into the conditions record.
	SchemaVersion = 3
	// SpecPath is where GitLab commits the generated document in its own
	// repository.
	SpecPath = "doc/api/openapi/openapi_v3.yaml"
	// RawURLTemplate builds the unauthenticated raw URL for a ref.
	RawURLTemplate = "https://gitlab.com/gitlab-org/gitlab/-/raw/%s/" + SpecPath
	// MinimumOperations is the floor an extraction has to clear to be one of
	// GitLab's whole API. The document carried 1847 operations on the day this
	// was written and grows release over release, so a count well under that is
	// a truncated download or a document that is not this one. The consequence
	// of accepting one is the same as everywhere else in this repository: a
	// comparison that reports every one of our endpoints as unknown to GitLab,
	// or none of them, and either way answers a question nobody asked.
	MinimumOperations = 1500
)

// Document is the committed extraction.
type Document struct {
	SchemaVersion int    `json:"schema_version"`
	Note          string `json:"note"`
	Source        Source `json:"source"`
	// Operations is keyed by "METHOD /path", with the path exactly as GitLab
	// spells it, /api/v4 prefix and {braces} included. Normalizing here would
	// bake one reader's convention into the record of what GitLab said.
	Operations map[string]Operation `json:"operations"`
}

// Source is where the extraction came from, so a reader can tell how old it is
// without asking git and can fetch the same bytes again.
type Source struct {
	URL         string `json:"url"`
	Ref         string `json:"ref"`
	RetrievedAt string `json:"retrieved_at"`
	// SHA256 is of the document that was extracted, not of this file: it is
	// what says two extractions read the same GitLab.
	SHA256 string `json:"sha256"`
	// OpenAPIVersion and APIVersion are what the document says it is.
	OpenAPIVersion string `json:"openapi_version"`
	APIVersion     string `json:"api_version"`
	Operations     int    `json:"operations"`
}

// Operation is what GitLab accepts and returns at one endpoint.
type Operation struct {
	// Response holds the property names of the success response's schema,
	// sorted. An operation that names no schema, which is 553 of them, carries
	// an empty list, and an empty list therefore means "GitLab does not say"
	// rather than "GitLab sends nothing".
	Response []string `json:"response,omitempty"`
	// Nested holds, for each property of the success response that carries an
	// object of its own, that object's property names, sorted. It is one level
	// deep and stays that way: the record is diffed by hand on every
	// regeneration, and each further level multiplies its size by the branching
	// of GitLab's schemas rather than adding to it.
	//
	// A property absent from this map either carries no object or carries one
	// the document does not describe, which is the same "GitLab does not say"
	// an empty [Operation.Response] means.
	Nested map[string][]string `json:"nested,omitempty"`
	// Entity names the component the success response resolves to, as the
	// document names it (APIEntitiesProject), for a response that is one
	// component or a list of one; "" for a response described inline or not
	// at all. It is the key into the conditions record, which says under what
	// condition each of the entity's fields is sent (cmd/internal/apiexposes).
	Entity string `json:"entity,omitempty"`
	// NestedEntity names, per property of the response carrying an object the
	// document reached through a component, that component, so a nested
	// object joins the conditions record the same way.
	NestedEntity map[string]string `json:"nested_entity,omitempty"`
	// Params holds the path and query parameter names, sorted.
	Params []string `json:"params,omitempty"`
	// Body holds the request body's property names, sorted.
	Body []string `json:"body,omitempty"`
}

// String renders the provenance as one reportable line.
func (s Source) String() string {
	return fmt.Sprintf("%d operations from %s at %s, retrieved %s",
		s.Operations, SpecPath, s.Ref, s.RetrievedAt)
}

// Key is how an operation is addressed in the artifact.
func Key(method, path string) string {
	return strings.ToUpper(method) + " " + path
}

// NormalizePath renders a path the way this repository's request inventory
// spells one, so the two artifacts can be compared: the /api/v4 prefix trimmed,
// and every {placeholder} replaced by the : form.
//
// The two conventions are kept apart deliberately. This record says what GitLab
// said; the inventory says what we sent; a comparison converts, and neither
// artifact is written in the other's dialect.
func NormalizePath(path string) string {
	path = strings.TrimPrefix(path, "/api/v4")
	path = strings.TrimPrefix(path, "/api")
	if path == "" {
		path = "/"
	}
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '{' {
			b.WriteByte(path[i])
			continue
		}
		end := strings.IndexByte(path[i:], '}')
		if end < 0 {
			b.WriteString(path[i:])
			break
		}
		b.WriteByte(':')
		b.WriteString(path[i+1 : i+end])
		i += end
	}
	return b.String()
}

// Write commits the document to dir.
func Write(dir string, doc Document) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	doc.SchemaVersion = SchemaVersion
	// Marshaling a struct of strings and string slices cannot fail, and the
	// repository's rule for that is to say so at the leaf rather than carry a
	// branch no test can reach.
	encoded := cmdutil.Must(json.MarshalIndent(doc, "", " "))
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Read loads the committed document from dir.
func Read(dir string) (Document, error) {
	path := filepath.Join(dir, FileName)
	raw, err := os.ReadFile(path) //#nosec G304 -- the directory is the caller's own flag
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	var doc Document
	if parseErr := json.Unmarshal(raw, &doc); parseErr != nil {
		return Document{}, fmt.Errorf("parse %s: %w", path, parseErr)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf("%s is schema version %d and this build reads %d: regenerate it with `make gen-api-shapes`",
			path, doc.SchemaVersion, SchemaVersion)
	}
	return doc, nil
}

// Keys returns every operation key, sorted, which is what a caller walking the
// document in a stable order needs.
func (d Document) Keys() []string {
	keys := make([]string, 0, len(d.Operations))
	for key := range d.Operations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
