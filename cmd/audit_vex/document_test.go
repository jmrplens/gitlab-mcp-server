// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// document_test.go covers reading the document as OpenVEX v0.2.0 and every
// rule a statement, a product and a subcomponent are held to, the
// repository's own document included.
package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fixtureModule is the module path every fixture document describes.
const fixtureModule = "example.com/fixture"

// fixtureImage is a product of the fixture module's image, pinned by digest.
const fixtureImage = "pkg:oci/fixture@sha256%3A" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" + "?repository_url=ghcr.io/example/fixture"

// one is the version every fixture document carries.
var one = 1

// fixtureStatement is a statement that validates: one advisory, not affecting
// the fixture module and its image, through the standard library.
func fixtureStatement(osv string) statement {
	return statement{
		Vulnerability: vulnerability{ID: advisoryPage + osv, Name: osv},
		Products: []component{
			{ID: "pkg:golang/" + fixtureModule, Subcomponents: []component{{ID: "pkg:golang/stdlib"}}},
			{ID: fixtureImage, Subcomponents: []component{{ID: "pkg:golang/stdlib@v1.27.0"}}},
		},
		Status:          statusNotAffected,
		Justification:   "vulnerable_code_not_present",
		ImpactStatement: "The fixture imports none of the advisory's packages.",
	}
}

// fixtureDocument is a document that validates, carrying one statement per
// advisory.
func fixtureDocument(osvs ...string) document {
	doc := document{
		Context:   openvexContext,
		ID:        "https://example.com/vex.json",
		Author:    "Fixture author",
		Timestamp: "2026-10-31T00:00:00Z",
		Version:   &one,
	}
	for _, osv := range osvs {
		doc.Statements = append(doc.Statements, fixtureStatement(osv))
	}
	return doc
}

// writeFile writes content to name under dir and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("creating %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}

// TestReadDocument_TheRepositoryDocumentValidates is the document the release
// publishes, read and validated against the module go.mod declares. The
// half that needs the vulnerability database is make check-vex.
func TestReadDocument_TheRepositoryDocumentValidates(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	mod, err := readGoMod(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("reading go.mod: %v", err)
	}
	doc, err := readDocument(filepath.Join(root, filepath.FromSlash(defaultDocument)))
	if err != nil {
		t.Fatalf("readDocument: %v", err)
	}
	if problems := validate(doc, mod.path); len(problems) != 0 {
		t.Errorf("the repository's document does not validate:\n%s", strings.Join(problems, "\n"))
	}
}

// TestReadDocument_RefusesWhatItWouldOtherwiseMisread covers the three
// refusals encoding/json would not make (a member twice, a member OpenVEX
// does not define, a member in another case), a document that is not JSON,
// and a file that is not there.
func TestReadDocument_RefusesWhatItWouldOtherwiseMisread(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, tc := range []struct {
		name, content, want string
	}{
		{name: "a member twice", content: `{"author": "a", "author": "b"}`, want: "duplicate"},
		{name: "an unknown member", content: `{"statements": [{"justificaton": "x"}]}`, want: "justificaton"},
		{name: "another case", content: `{"Author": "a"}`, want: "Author"},
		{name: "a component's identifiers", content: `{"statements": [{"products": [{"identifiers": {}}]}]}`, want: "identifiers"},
		{name: "not JSON", content: `{`, want: "does not read as OpenVEX"},
		{name: "a version that is not an integer", content: `{"version": 1.5}`, want: "version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := writeFile(t, dir, tc.name+".json", tc.content)
			if _, err := readDocument(path); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("readDocument(%s) error = %v, want one naming %q", tc.content, err, tc.want)
			}
		})
	}

	t.Run("absent", func(t *testing.T) {
		t.Parallel()

		if _, err := readDocument(filepath.Join(dir, "absent.json")); err == nil {
			t.Error("readDocument of a file that is not there returned no error")
		}
	})
}

// TestValidate_AFixtureThatHolds_HasNoProblem is the baseline every case of
// the next test changes one thing in, with every optional member the
// specification defines set to a value that holds.
func TestValidate_AFixtureThatHolds_HasNoProblem(t *testing.T) {
	t.Parallel()

	doc := fixtureDocument("GO-2099-0001")
	doc.LastUpdated = "2026-11-01T00:00:00Z"
	st := &doc.Statements[0]
	st.ID, st.Version = "https://example.com/vex.json#1", &one
	st.Timestamp, st.LastUpdated = "2026-10-31T00:00:00Z", "2026-11-01T00:00:00Z"
	if problems := validate(doc, fixtureModule); len(problems) != 0 {
		t.Errorf("validate = %q, want none", problems)
	}
	for _, status := range []string{statusAffected, statusUnderInvestigation} {
		t.Run(status, func(t *testing.T) {
			t.Parallel()

			other := fixtureDocument("GO-2099-0001")
			changed := &other.Statements[0]
			changed.Status, changed.Justification, changed.ImpactStatement = status, "", ""
			changed.ActionStatement, changed.ActionStatementTimestamp = "Update the module.", "2026-10-31T00:00:00Z"
			if problems := validate(other, fixtureModule); len(problems) != 0 {
				t.Errorf("validate = %q, want none", problems)
			}
		})
	}
}

// TestValidate_EachRuleIsReported changes one thing in the fixture per case
// and holds the report to naming it.
func TestValidate_EachRuleIsReported(t *testing.T) {
	t.Parallel()

	zero := 0
	for _, tc := range []struct {
		name   string
		change func(*document)
		want   string
	}{
		{name: "another context", change: func(d *document) { d.Context = "https://openvex.dev/ns/v0.1.0" }, want: "@context"},
		{name: "a document @id that is not https", change: func(d *document) { d.ID = "http://example.com/vex.json" }, want: "is not an https IRI"},
		{name: "a document @id that does not parse", change: func(d *document) { d.ID = "https://%zz" }, want: "is not an https IRI"},
		{name: "no author", change: func(d *document) { d.Author = " " }, want: "names no author"},
		{name: "no timestamp", change: func(d *document) { d.Timestamp = "" }, want: "the document: timestamp"},
		{name: "a last_updated that does not parse", change: func(d *document) { d.LastUpdated = "yesterday" }, want: "last_updated \"yesterday\""},
		{name: "a last_updated before the timestamp", change: func(d *document) { d.LastUpdated = "2026-01-01T00:00:00Z" }, want: "is before its timestamp"},
		{name: "no version", change: func(d *document) { d.Version = nil }, want: "version is not a positive integer"},
		{name: "version zero", change: func(d *document) { d.Version = &zero }, want: "version is not a positive integer"},
		{name: "no statement", change: func(d *document) { d.Statements = nil }, want: "carries no statement"},
		{name: "a statement @id that is not https", change: func(d *document) { d.Statements[0].ID = "urn:x" }, want: "@id \"urn:x\""},
		{name: "a statement version zero", change: func(d *document) { d.Statements[0].Version = &zero }, want: "statement 1 (GO-2099-0001): version"},
		{name: "a name that is not a Go id", change: func(d *document) { d.Statements[0].Vulnerability.Name = "CVE-2026-0001" }, want: "not a Go vulnerability database id"},
		{name: "a vulnerability @id of another advisory", change: func(d *document) { d.Statements[0].Vulnerability.ID = advisoryPage + "GO-2099-0002" }, want: "is not " + advisoryPage + "GO-2099-0001"},
		{name: "a statement timestamp that does not parse", change: func(d *document) { d.Statements[0].Timestamp = "now" }, want: "timestamp \"now\""},
		{name: "an action timestamp that does not parse", change: func(d *document) { d.Statements[0].ActionStatementTimestamp = "soon" }, want: "action_statement_timestamp"},
		{name: "an unknown justification", change: func(d *document) { d.Statements[0].Justification = "not_reachable" }, want: "justification \"not_reachable\""},
		{name: "no impact statement", change: func(d *document) { d.Statements[0].ImpactStatement = "" }, want: "no impact_statement"},
		{name: "affected with no action", change: func(d *document) {
			d.Statements[0].Status, d.Statements[0].Justification = statusAffected, ""
		}, want: "no action_statement"},
		{name: "fixed", change: func(d *document) {
			d.Statements[0].Status, d.Statements[0].Justification = statusFixed, ""
		}, want: "fixed, which this document does not state"},
		{name: "an unknown status", change: func(d *document) {
			d.Statements[0].Status, d.Statements[0].Justification = "safe", ""
		}, want: "the status \"safe\""},
		{name: "a justification beside another status", change: func(d *document) { d.Statements[0].Status = statusUnderInvestigation }, want: "belongs to a not_affected statement only"},
		{name: "no product", change: func(d *document) { d.Statements[0].Products = nil }, want: "names no product"},
		{name: "a product that is not a package URL", change: func(d *document) { d.Statements[0].Products[0].ID = "fixture" }, want: "product: \"fixture\" is not a package URL"},
		{name: "another module", change: func(d *document) { d.Statements[0].Products[0].ID = "pkg:golang/example.com/other" }, want: "is not pkg:golang/" + fixtureModule},
		{name: "the module at one version", change: func(d *document) { d.Statements[0].Products[0].ID = "pkg:golang/" + fixtureModule + "@v1.0.0" }, want: "this module at every version"},
		{name: "an image by tag", change: func(d *document) {
			d.Statements[0].Products[1].ID = "pkg:oci/fixture?repository_url=ghcr.io/example/fixture&tag=1.0.0"
		}, want: "pinned by the sha256 digest"},
		{name: "an image of another repository", change: func(d *document) {
			d.Statements[0].Products[1].ID = strings.Replace(fixtureImage, "example/fixture", "example/other", 1)
		}, want: "with the repository_url it was published to"},
		{name: "a product of another type", change: func(d *document) { d.Statements[0].Products[0].ID = "pkg:npm/fixture" }, want: "is a npm package"},
		{name: "a product with no subcomponent", change: func(d *document) { d.Statements[0].Products[0].Subcomponents = nil }, want: "names no subcomponent"},
		{name: "a subcomponent that is not a package URL", change: func(d *document) {
			d.Statements[0].Products[0].Subcomponents[0].ID = "stdlib"
		}, want: "a subcomponent of \"pkg:golang/" + fixtureModule + "\""},
		{name: "a subcomponent of another type", change: func(d *document) {
			d.Statements[0].Products[0].Subcomponents[0].ID = "pkg:npm/left-pad"
		}, want: "is not a Go module"},
		{name: "a subcomponent with qualifiers", change: func(d *document) {
			d.Statements[0].Products[0].Subcomponents[0].ID = "pkg:golang/stdlib?arch=amd64"
		}, want: "is not a Go module"},
		{name: "a subcomponent with subcomponents", change: func(d *document) {
			d.Statements[0].Products[0].Subcomponents[0].Subcomponents = []component{{ID: "pkg:golang/stdlib"}}
		}, want: "has subcomponents of its own"},
		{name: "an image's subcomponent with no version", change: func(d *document) {
			d.Statements[0].Products[1].Subcomponents[0].ID = "pkg:golang/stdlib"
		}, want: "of the image \"" + fixtureImage + "\" names no version"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc := fixtureDocument("GO-2099-0001")
			tc.change(&doc)
			problems := validate(doc, fixtureModule)
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Errorf("validate = %q, want exactly one problem naming %q", problems, tc.want)
			}
		})
	}
}

// TestOwnModules_ReadsThisModulesOwnProductOnly covers which subcomponents
// are held to the current tree: those of the module's own product, each
// once and sorted, and none of an image product's or another module's.
func TestOwnModules_ReadsThisModulesOwnProductOnly(t *testing.T) {
	t.Parallel()

	st := fixtureStatement("GO-2099-0001")
	st.Products[0].Subcomponents = append(st.Products[0].Subcomponents, component{ID: "pkg:golang/example.com/dep"}, component{ID: "pkg:golang/stdlib@v1.27.3"})
	st.Products = append(st.Products, component{ID: "pkg:golang/example.com/other", Subcomponents: []component{{ID: "pkg:golang/example.com/elsewhere"}}})
	if got := ownModules(st, fixtureModule); len(got) != 2 || got[0] != "example.com/dep" || got[1] != "stdlib" {
		t.Errorf("ownModules = %q, want example.com/dep and stdlib, once each", got)
	}
	st.Products = st.Products[1:2]
	if got := ownModules(st, fixtureModule); len(got) != 0 {
		t.Errorf("ownModules of a statement about an image alone = %q, want none", got)
	}
}

// TestImageModules_ReadsEveryImageProduct covers the subcomponents held to
// the version a published image carries: every one of every image product,
// in document order, and none of the module's own product.
func TestImageModules_ReadsEveryImageProduct(t *testing.T) {
	t.Parallel()

	second := strings.Replace(fixtureImage, "ghcr.io", "docker.io", 1)
	st := fixtureStatement("GO-2099-0001")
	st.Products = append(st.Products, component{ID: second, Subcomponents: []component{{ID: "pkg:golang/stdlib@v1.27.0"}, {ID: "pkg:golang/example.com/dep@v0.1.0"}}})
	want := []imageModule{
		{product: fixtureImage, path: "stdlib", version: "v1.27.0"},
		{product: second, path: "stdlib", version: "v1.27.0"},
		{product: second, path: "example.com/dep", version: "v0.1.0"},
	}
	if got := imageModules(st); !slices.Equal(got, want) {
		t.Errorf("imageModules = %+v, want %+v", got, want)
	}
}

// TestLevel_NamesEachLevel covers the words the report uses for each level.
func TestLevel_NamesEachLevel(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		level level
		want  string
	}{
		{levelModule, "against the module only"},
		{levelPackage, "vulnerable package imported"},
		{levelSymbol, "vulnerable function called"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := tc.level.String(); !strings.Contains(got, tc.want) {
				t.Errorf("level %d = %q, want %q", tc.level, got, tc.want)
			}
		})
	}
}
