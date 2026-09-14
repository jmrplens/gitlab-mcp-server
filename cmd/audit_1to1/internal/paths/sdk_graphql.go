package paths

import (
	"path/filepath"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
)

// SDKGraphQLCheck is the documents client-go builds inside its own module,
// read and judged against the same pinned schema the documents of this
// repository are.
//
// It exists because "does the document validate" was, until now, a question
// asked only of the source this repository writes. Every GraphQL operation the
// achievement, work item, saved view, security attribute, security category,
// scan profile, target branch rule and Terraform state services send is built
// inside the SDK, goes to GitLab through this server, and was judged by
// nothing except whichever of them a unit test happens to drive. A document
// GitLab refuses there breaks a registered tool exactly as completely as one
// written here.
//
// It reports and never gates, and the reason is not the usual one about an
// incomplete oracle. The pin is what GitLab serves, so a refusal here is real;
// what a refusal is not is something this repository can fix. The answer is an
// upstream merge request and a version bump, which is not a state to fail the
// build on, so it is published as a finding to act on rather than as a gate to
// pass. That also keeps the R-PATH gate's verdict a statement about this
// repository's own tree, which is what every other thing it fails on is.
type SDKGraphQLCheck struct {
	// Ran is whether the module was read. Everything below is empty when it is
	// false, and Error says why.
	Ran bool `json:"ran"`
	// Module is the directory the documents were read from, trimmed to the
	// module's own name and version so the report does not carry the reader's
	// module cache path.
	Module string `json:"module,omitempty"`
	// Error is what stopped the read. It is a note rather than a failure: this
	// reads a module cache whose state it does not own.
	Error string `json:"error,omitempty"`
	// Documents is how many were read, template shells included.
	Documents int `json:"documents"`
	// Templates are the documents assembled around a placeholder rather than
	// written as the text GitLab receives, named because they are the part of
	// this surface no schema can judge. See [graphqldocs.IsTemplate].
	Templates []SDKDocument `json:"template_documents,omitempty"`
	// Judged is how many were put to the schema, which is Documents less the
	// templates.
	Judged int `json:"judged"`
	// Refusals are the documents the pinned schema will not accept, in the
	// same shape the repository's own refusals are reported in.
	Refusals []GraphQLRefusal `json:"refusals"`
}

// SDKDocument names one document of the SDK for a list that is not a refusal.
//
// The position is what identifies it rather than the name: two thirds of these
// documents are written inline at the point of use and so are declared under
// no name at all, and a list of four rows all reading "an inline document" is
// a list a reader cannot act on.
type SDKDocument struct {
	Package  string `json:"package"`
	Document string `json:"document"`
	Position string `json:"position"`
}

// Seams for the two halves of this check the real tree resolves and a test
// cannot: the module directory comes out of a twenty-second load of the tool
// packages, and the documents come out of a module cache. Each is a variable a
// test restores.
var (
	readSDKDocuments = graphqldocs.SDKDocuments
	judgeDocuments   = graphqldocs.Judge
)

// sdkGraphQLCheck reads the client-go documents and judges each one.
//
// The directory is the one [structs.CollectOutputPairings] already resolved for
// the typed shape comparison, so this costs no second walk of the import graph
// to find it; what it does cost is type-checking client-go, which is the only
// way a document assembled from a shared field constant folds to the string
// GitLab receives.
func sdkGraphQLCheck(root string) SDKGraphQLCheck {
	pairings, err := collectPairings(root)
	if err != nil || pairings.ClientGoDir == "" {
		return SDKGraphQLCheck{}
	}

	check := SDKGraphQLCheck{Ran: true, Module: filepath.Base(pairings.ClientGoDir)}
	documents, err := readSDKDocuments(pairings.ClientGoDir)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	check.Documents = len(documents)

	judgeable := make([]graphqldocs.Document, 0, len(documents))
	for _, document := range documents {
		if graphqldocs.IsTemplate(document) {
			check.Templates = append(check.Templates, SDKDocument{
				Package:  document.Package,
				Document: document.Label(),
				Position: relativePosition(document, pairings.ClientGoDir),
			})
			continue
		}
		judgeable = append(judgeable, document)
	}
	check.Judged = len(judgeable)

	result, err := judgeDocuments(judgeable, graphqldocs.Options{})
	if err != nil {
		check.Error = err.Error()
		return check
	}
	check.Refusals = sdkRefusals(result, pairings.ClientGoDir)
	return check
}

// sdkRefusals renders the refused SDK documents the way the repository's own
// are.
func sdkRefusals(result graphqldocs.Result, moduleDir string) []GraphQLRefusal {
	found := make([]GraphQLRefusal, 0, len(result.Refusals))
	for _, refusal := range result.Refusals {
		found = append(found, GraphQLRefusal{
			Package:  refusal.Document.Package,
			Document: refusal.Document.Label(),
			Position: relativePosition(refusal.Document, moduleDir),
			Reasons:  refusal.Reasons,
		})
	}
	return found
}

// relativePosition names where a document is, relative to the module it came
// from.
//
// A position is trimmed here and left absolute for this repository's own
// documents on purpose: a path under internal/ names a file any reader can
// open, while a module cache path names one that exists only on the machine
// that ran the audit and at a different place on every other. A path the trim
// cannot make relative is left as it is rather than dropped, since a long
// position is more use than none.
func relativePosition(document graphqldocs.Document, moduleDir string) string {
	position := document.Position
	if relative, err := filepath.Rel(moduleDir, position.Filename); err == nil {
		position.Filename = filepath.ToSlash(relative)
	}
	return position.String()
}
