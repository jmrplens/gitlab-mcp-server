package graphqldocs

import (
	"errors"
	"fmt"
	"os"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// Options is one configured audit: where to look, what to look at, and what to
// judge it against.
type Options struct {
	// Dir is the repository root to audit.
	Dir string
	// Patterns are the load patterns; empty means [DefaultPatterns].
	Patterns []string
	// SchemaPath names an SDL file to judge the documents against instead of
	// the pinned one. It is how the live re-probe works: cmd/gen_graphql_schema
	// writes today's schema into a temporary directory and this reads it, so a
	// field GitLab narrowed since the pin is reported as a failure rather than
	// waiting for the next re-pin. It is also how a document meant for a
	// particular self-managed release can be checked against that release.
	SchemaPath string
	// Schema is a schema the caller already has, judged in preference to both
	// the pin and SchemaPath. It exists for the live re-probe, which
	// introspects an instance itself rather than writing SDL to a file first,
	// because it also has to refuse an answer too short to be a GitLab schema
	// and report where the pin and that instance disagree, and both of those
	// need the schema as a value rather than as a path.
	Schema *ast.Schema
	// Provenance names what Schema is, for the line a reader of a refusal
	// needs. Required with Schema and ignored without it.
	Provenance string
	// Overlay supplies source that is not on disk, which is how a test hands
	// the audit a fixture package instead of the repository. Production passes
	// nil.
	Overlay map[string][]byte
}

// Refusal is one document the schema will not accept.
type Refusal struct {
	// Document is the document that was refused.
	Document Document
	// Reasons are the schema's objections, one per line a report prints.
	//
	// A failure that is not a refusal at all, such as a pin that will not
	// load, arrives here as its single message rather than as an empty list,
	// because a finding with nothing under it reads as a document nobody could
	// explain.
	Reasons []string
}

// Result is what one audit found.
type Result struct {
	// Provenance is the one line saying whose opinion judged these documents:
	// a schema pinned on a recorded day, or one fetched today.
	Provenance string
	// Documents are every document the audit read, in the order a reader walks
	// the repository.
	Documents []Document
	// Refusals are the documents the schema would not accept, a subset of
	// Documents in the same order.
	Refusals []Refusal
}

// ErrNoDocuments is returned when an audit found nothing to judge, which means
// it is looking at the wrong tree. A clean exit there would be the silence this
// package exists to remove, so it is an error rather than an empty result.
var ErrNoDocuments = errors.New("no GraphQL documents were found, which means this audit is looking at the wrong thing")

// Audit reads every document under the configured patterns and judges each one.
//
// A refused document is a finding in the result, not an error: the error return
// is for the audit failing to run at all, which is the case a caller must not
// report as a pass.
func Audit(opts Options) (Result, error) {
	validate, provenance, err := judge(opts.SchemaPath, opts.Schema, opts.Provenance)
	if err != nil {
		return Result{}, err
	}

	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = DefaultPatterns()
	}
	documents, err := Collect(opts.Dir, patterns, opts.Overlay)
	if err != nil {
		return Result{}, err
	}
	if len(documents) == 0 {
		return Result{}, ErrNoDocuments
	}

	result := Result{Provenance: provenance, Documents: documents}
	for _, found := range documents {
		if validationErr := validate(found.Text); validationErr != nil {
			result.Refusals = append(result.Refusals, Refusal{Document: found, Reasons: reasons(validationErr)})
		}
	}
	return result, nil
}

// judge returns the check each document is put through and the one line that
// says what judged it, so a reader of a failure knows whose opinion refused
// their document.
func judge(schemaPath string, probed *ast.Schema, probedProvenance string) (validate func(string) error, provenance string, err error) {
	if probed != nil {
		return func(document string) error {
			return graphqlschema.ValidateDocumentAgainst(probed, document)
		}, probedProvenance, nil
	}
	if schemaPath == "" {
		// The provenance record is embedded and its own gate
		// (make check-graphql-schema) refuses a build where it does not decode,
		// so a failure there is not something a caller could act on.
		return graphqlschema.ValidateDocument, cmdutil.Must(graphqlschema.SourceInfo()).String(), nil
	}

	sdl, err := os.ReadFile(schemaPath) //#nosec G304 -- the path is the operator's own -schema flag
	if err != nil {
		return nil, "", fmt.Errorf("read the schema to judge against: %w", err)
	}
	schema, err := graphqlschema.Load(sdl)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", schemaPath, err)
	}
	return func(document string) error {
			return graphqlschema.ValidateDocumentAgainst(schema, document)
		},
		fmt.Sprintf("%d types from %s, not the pinned schema", len(schema.Types), schemaPath),
		nil
}

// reasons renders a validation failure as the lines a report prints under the
// document it refused.
func reasons(err error) []string {
	var refusal *graphqlschema.ValidationError
	if errors.As(err, &refusal) && len(refusal.Reasons) > 0 {
		return refusal.Reasons
	}
	return []string{err.Error()}
}
