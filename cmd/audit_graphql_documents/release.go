// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/graphqldocs"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/graphqlschema"
)

// The release pass judges every document this server sends against the schema
// of the GitLab release the REST record is taken from, as well as against the
// gitlab.com pin the rest of this command judges by.
//
// The pin is gitlab.com's, which runs the next minor's pre-release, so a field
// the pre-release adds passes the pin and is refused by every released
// instance. The re-pin of 2026-10-29 let crashState into the location
// selection seven security actions share; this gate, the shape gate and the
// whole unit suite stayed green, and the first thing to refuse the documents
// was the Enterprise end-to-end run against GitLab 19.5.3. The weekly
// ee-schema.yml re-probe would have said so too, a week after the commit. This
// pass says so on the commit, from a schema pinned beside the other one
// (internal/graphqlschema, ReleaseSchema), with no instance and no network.
//
// client-go's documents are judged here as well, unlike under the pin. Under
// the pin a document client-go builds that GitLab refuses is a defect of
// client-go's, fixed upstream and reported by R-PATH. Under the release it is
// a dependency this server took that stops working on the release the server
// is pinned to, which is a decision to take here, before the bump lands: hold
// the bump, stop sending that method on the release, or declare it.

// releaseDeclaration answers one document the release refuses on purpose: one
// that targets something no self-managed release serves.
type releaseDeclaration struct {
	// Package is the import path of the package that declares the document,
	// as [graphqldocs.Document.Package] spells it.
	Package string
	// Document is its name, as [graphqldocs.Document.Label] spells it.
	Document string
	// Reason says why the release refusing it is not a defect, in words a
	// reviewer can check.
	Reason string
}

// orbitPackage is where the Orbit documents live.
const orbitPackage = "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/orbit"

// orbitReason is why the release refuses every Orbit document and why that is
// right. TestReleaseDeclarations_OrbitDocuments_BelongToGitLabDotComOnlyActions
// holds the claim about the actions to the catalog.
func orbitReason(fields string) string {
	return "Orbit is served by GitLab.com alone: no self-managed release has " + fields + ", and every Orbit " +
		"action is declared GitLabDotComOnly, so the catalog offers none of them to a session on another " +
		"instance and this document is never sent to one. The gitlab.com pin, which is the schema of the " +
		"only instance that answers it, judges it."
}

// releaseDeclarations are the documents the release pin refuses on purpose.
func releaseDeclarations() []releaseDeclaration {
	return []releaseDeclaration{
		{Package: orbitPackage, Document: "queryServiceStatus", Reason: orbitReason("Query.orbit")},
		{Package: orbitPackage, Document: "querySchema", Reason: orbitReason("Query.orbit")},
		{Package: orbitPackage, Document: "queryGraphStatusByPath", Reason: orbitReason("OrbitNamespace.enabled, access, trial and indexing")},
		{Package: orbitPackage, Document: "queryGraphStatusByGroupID", Reason: orbitReason("OrbitNamespace.enabled, access, trial and indexing")},
		{Package: orbitPackage, Document: "queryGraphStatusByProjectID", Reason: orbitReason("OrbitNamespace.enabled, access, trial and indexing")},
	}
}

// releaseVerdict is what the release pass found.
type releaseVerdict struct {
	// judged and sdkJudged count the documents put through the release, this
	// repository's and client-go's, and ownRefusals and sdkRefusals how many
	// of each it refused, declared or not.
	judged, sdkJudged        int
	ownRefusals, sdkRefusals int
	// refused are the refusals no declaration answers, which fail the run.
	refused []graphqldocs.Refusal
	// declared are the refusals a declaration answers.
	declared []graphqldocs.Refusal
	// stale are the declarations that answer no refusal, which fail the run
	// too, so the table cannot outlive the documents it describes.
	stale []releaseDeclaration
	// unjudged names the client-go documents no schema can be asked about,
	// because the walk could not render the text they are sent as.
	unjudged []string
	// sdkErr is why client-go's documents were not read, which fails the run:
	// a pass that judged half of what this server sends is not a pass.
	sdkErr error
}

// failed reports whether the pass fails the run.
func (v releaseVerdict) failed() bool {
	return len(v.refused) > 0 || len(v.stale) > 0 || v.sdkErr != nil
}

// judgeRelease puts this repository's documents and client-go's through the
// release schema and sorts the refusals by the declarations.
func judgeRelease(schema *ast.Schema, documents []graphqldocs.Document, sdk sdkRead, declarations []releaseDeclaration) releaseVerdict {
	verdict := releaseVerdict{judged: len(documents), sdkErr: sdk.err}
	refusals := refusedBy(schema, documents)
	verdict.ownRefusals = len(refusals)

	sendable := make([]graphqldocs.Document, 0, len(sdk.documents))
	for _, document := range sdk.documents {
		if document.Assembly != nil && document.Assembly.Unrendered != "" {
			verdict.unjudged = append(verdict.unjudged, fmt.Sprintf("%s (%s:%d), assembled at run time: %s",
				document.Label(), filepath.Base(document.Position.Filename), document.Position.Line, document.Assembly.Unrendered))
			continue
		}
		sendable = append(sendable, document)
	}
	verdict.sdkJudged = len(sendable)
	sdkRefusals := refusedBy(schema, sendable)
	verdict.sdkRefusals = len(sdkRefusals)
	refusals = append(refusals, sdkRefusals...)

	answered := make([]bool, len(declarations))
	for _, refusal := range refusals {
		index := declarationFor(declarations, refusal.Document)
		if index < 0 {
			verdict.refused = append(verdict.refused, refusal)
			continue
		}
		answered[index] = true
		verdict.declared = append(verdict.declared, refusal)
	}
	for i, declaration := range declarations {
		if !answered[i] {
			verdict.stale = append(verdict.stale, declaration)
		}
	}
	return verdict
}

// refusedBy returns the documents the schema refuses, each with its reasons.
// An empty set refuses nothing, which [graphqldocs.Judge] would report as an
// audit looking at the wrong tree; the caller has already judged this
// repository's documents, so an empty set here is a module with no sendable
// document rather than that, and the only error Judge returns for a schema
// handed to it is that one.
func refusedBy(schema *ast.Schema, documents []graphqldocs.Document) []graphqldocs.Refusal {
	if len(documents) == 0 {
		return nil
	}
	return cmdutil.Must(graphqldocs.Judge(documents, graphqldocs.Options{Schema: schema, Provenance: "the release schema"})).Refusals
}

// declarationFor returns the index of the declaration answering document, or
// -1 when none does.
func declarationFor(declarations []releaseDeclaration, document graphqldocs.Document) int {
	for i, declaration := range declarations {
		if declaration.Package == document.Package && declaration.Document == document.Label() {
			return i
		}
	}
	return -1
}

// reportRelease writes what the pass found, the findings to errOut and the
// summary to out.
func reportRelease(verdict releaseVerdict, release graphqlschema.ReleaseSource, roots []string, out, errOut io.Writer) {
	if verdict.sdkErr != nil {
		fmt.Fprintf(errOut, "%s client-go's documents were not read, so the release cannot judge them: %v\n", prefix, verdict.sdkErr)
	}
	if len(verdict.refused) > 0 {
		fmt.Fprintf(errOut, "\n%s GitLab %s, the release this server is pinned to, refuses %d document(s) it sends. GitLab refuses a whole "+
			"document naming a field it does not have, so every action sending one fails on that release and every "+
			"release before it: select the field only where the release has it, or declare why the document is never "+
			"sent to a self-managed instance in cmd/audit_graphql_documents/release.go\n",
			prefix, release.GitLabVersion, len(verdict.refused))
		for _, refusal := range verdict.refused {
			fmt.Fprint(errOut, finding(roots, refusal))
		}
	}
	for _, declaration := range verdict.stale {
		fmt.Fprintf(errOut, "%s the release declaration for %s %s answers nothing: the document is gone, renamed or "+
			"accepted by GitLab %s now, so remove it from cmd/audit_graphql_documents/release.go\n",
			prefix, declaration.Package, declaration.Document, release.GitLabVersion)
	}
	for _, line := range verdict.unjudged {
		fmt.Fprintf(out, "    not judged by the release: %s\n", line)
	}

	declared := make([]string, 0, len(verdict.declared))
	for _, refusal := range verdict.declared {
		declared = append(declared, refusal.Document.Label())
	}
	fmt.Fprintf(out, "%s GitLab %s accepts %d of %d document(s) of this repository and %d of %d of client-go's, "+
		"and refuses %d declared to need GitLab.com (%s); %s\n",
		prefix, release.GitLabVersion,
		verdict.judged-verdict.ownRefusals, verdict.judged,
		verdict.sdkJudged-verdict.sdkRefusals, verdict.sdkJudged,
		len(verdict.declared), strings.Join(declared, ", "), release)
}
