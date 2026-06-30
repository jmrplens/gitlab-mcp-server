// Package main implements the audit_doc_coverage command.
//
// audit_doc_coverage reports per-doc-file gaps between
// docs/tools/<doc>.md and the canonical action catalog. It produces
// plan/docs-tools-backlog.json (gitignored) listing, per file:
//
//   - missing: catalog tools expected in the doc but not documented
//   - orphan: tool headings/rows in the doc that do not belong there
//   - tier_mismatch: best-effort detection of catalog tier vs doc tier badge
//   - count_doc vs count_catalog vs readme_count for triple-cross-check
//
// The auditor exits non-zero under -check when any file has missing,
// orphan, or tier_mismatch findings, providing a CI gate that mirrors
// audit-discovery-check / audit-action-spec-coverage.
//
// Usage:
//
//	go run ./cmd/audit_doc_coverage/                     # full report to stdout
//	go run ./cmd/audit_doc_coverage/ -gaps-only          # only files with findings
//	go run ./cmd/audit_doc_coverage/ -output plan/docs-tools-backlog.json
//	go run ./cmd/audit_doc_coverage/ -check              # CI gate
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	// schemaVersion of the backlog JSON. Bump on incompatible shape changes.
	schemaVersion = 1

	// defaultBacklogPath is the canonical output location. It lives under
	// plan/, which is gitignored, so the file never enters the repo.
	defaultBacklogPath = "plan/docs-tools-backlog.json"

	// defaultToolsReadmePath is the source-of-truth for the per-doc
	// expected counts and group→doc mapping.
	defaultToolsReadmePath = "docs/tools/README.md"

	// defaultDocsRoot is the directory holding the per-domain docs that the
	// auditor compares against the catalog.
	defaultDocsRoot = "docs/tools"
)

// fileFinding captures the audit findings for one docs/tools/*.md file.
//
// ExpectedCount is derived from the canonical action catalog using the
// group→doc mapping parsed from docs/tools/README.md plus the hardcoded
// overrides in mapping.go (branch-rules routed tools, project-discovery,
// capabilities, "various" rows, etc.). DocumentedCount is the number of
// distinct `gitlab_*` references found in the doc itself. ReadmeCount is
// the count column from the Domains table; mismatches with ExpectedCount
// indicate README drift relative to the live catalog.
type fileFinding struct {
	DocPath              string         `json:"doc_path"`
	Domain               string         `json:"domain"`
	ExpectedCount        int            `json:"expected_count"`
	DocumentedCount      int            `json:"documented_count"`
	ReadmeCount          int            `json:"readme_count"`
	Missing              []string       `json:"missing"`
	Orphan               []string       `json:"orphan"`
	TierMismatch         []tierMismatch `json:"tier_mismatch"`
	CountMatches         bool           `json:"count_matches"`
	ReadmeCountMatches   bool           `json:"readme_count_matches"`
	UnassignedExpected   []string       `json:"unassigned_expected,omitempty"`
	UnassignedDocumented []string       `json:"unassigned_documented,omitempty"`
}

// tierMismatch is one catalog-vs-doc tier badge discrepancy. The
// auditor's tier detection on the doc side is best-effort: it scans for
// the standard badge tokens next to the heading.
type tierMismatch struct {
	Tool       string `json:"tool"`
	Catalog    string `json:"catalog"`
	Documented string `json:"documented"`
}

// reportSummary is the global headline for the backlog.
type reportSummary struct {
	Docs                 int `json:"docs"`
	DocsWithFindings     int `json:"docs_with_findings"`
	MissingTotal         int `json:"missing_total"`
	OrphanTotal          int `json:"orphan_total"`
	TierMismatchTotal    int `json:"tier_mismatch_total"`
	UnassignedTotal      int `json:"unassigned_total"`
	CleanDocs            int `json:"clean_docs"`
	ReadmeCountDriftDocs int `json:"readme_count_drift_docs"`
}

// report is the top-level JSON document written to the backlog path.
type report struct {
	SchemaVersion int           `json:"schema_version"`
	Summary       reportSummary `json:"summary"`
	Files         []fileFinding `json:"files"`
}

// cmdlineFlags holds the parsed CLI options.
type cmdlineFlags struct {
	outputPath string
	gapsOnly   bool
	checkMode  bool
	docsRoot   string
	readmePath string
}

// unassignedDocPath is the pseudo DocPath used in the report's
// "(unassigned)" entry. Catalog tools that no README row claims
// (typically because the catalog gained a new group before the
// Domains table was updated) are bucketed under this path so the
// auditor surfaces them as one backlog row instead of silently
// dropping them.
const unassignedDocPath = "(unassigned)"

func main() {
	flags := parseFlags()

	repoRoot, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		cmdutil.Fatalf("find repository root: %v", err)
	}

	docsRoot := filepath.Join(repoRoot, flags.docsRoot)
	readmePath := filepath.Join(repoRoot, flags.readmePath)

	catalog, err := loadCatalog(repoRoot)
	if err != nil {
		cmdutil.Fatalf("load action catalog: %v", err)
	}

	rep, err := buildReport(repoRoot, docsRoot, readmePath, catalog)
	if err != nil {
		cmdutil.Fatalf("build doc coverage report: %v", err)
	}

	if flags.checkMode {
		if msg := rep.check(); msg != "" {
			fmt.Fprintln(os.Stderr, msg)
			os.Exit(1)
		}
		return
	}

	if flags.gapsOnly {
		rep = rep.filterGapsOnly()
	}

	content, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		cmdutil.Fatalf("marshal report: %v", err)
	}
	content = append(content, '\n')

	if writeErr := writeReport(resolveOutputPath(repoRoot, flags.outputPath), content); writeErr != nil {
		cmdutil.Fatalf("write report: %v", writeErr)
	}
}

// resolveOutputPath resolves the -output flag value. "-" (stdout) and absolute
// paths are used verbatim; relative paths resolve against the repo root so the
// default plan/ location works regardless of the caller's cwd. Without the
// IsAbs guard, an absolute -output path would be joined onto repoRoot and write
// a stray tree inside the repo.
func resolveOutputPath(repoRoot, outputPath string) string {
	if outputPath == "-" || filepath.IsAbs(outputPath) {
		return outputPath
	}
	return filepath.Join(repoRoot, outputPath)
}

// parseFlags parses CLI options. -output defaults to
// plan/docs-tools-backlog.json (under the repo root).
func parseFlags() cmdlineFlags {
	var flags cmdlineFlags
	flag.StringVar(&flags.outputPath, "output", defaultBacklogPath, "path to write JSON report (relative paths resolve against repo root; absolute paths used as-is; '-' for stdout)")
	flag.BoolVar(&flags.gapsOnly, "gaps-only", false, "only include files that have at least one finding")
	flag.BoolVar(&flags.checkMode, "check", false, "exit non-zero if any file has missing/orphan/tier_mismatch findings")
	flag.StringVar(&flags.docsRoot, "docs-root", defaultDocsRoot, "directory of per-domain docs (relative to repo root)")
	flag.StringVar(&flags.readmePath, "readme-path", defaultToolsReadmePath, "path to the Domains-table README (relative to repo root)")
	flag.Parse()
	return flags
}

// check returns a non-empty diagnostic when the report has any
// blocking findings. Empty string means the gate passes.
//
// UnassignedTotal blocks the gate too: when the catalog gains a new
// group (or a tool whose owning group isn't claimed by any README
// row), those tools would otherwise slip past DOC-002 silently. The
// -check exit-non-zero on UnassignedTotal forces the orchestrator
// to add explicit routing — either by extending docs/tools/doc-ownership.json,
// by adding a new README Domains row, or by ADR-routing
// the group into a parent doc's prefix allowlist.
func (r report) check() string {
	if r.Summary.MissingTotal == 0 && r.Summary.OrphanTotal == 0 && r.Summary.TierMismatchTotal == 0 && r.Summary.UnassignedTotal == 0 {
		return ""
	}
	return fmt.Sprintf(
		"doc coverage: %d missing, %d orphan, %d tier_mismatch, %d unassigned across %d/%d docs",
		r.Summary.MissingTotal,
		r.Summary.OrphanTotal,
		r.Summary.TierMismatchTotal,
		r.Summary.UnassignedTotal,
		r.Summary.DocsWithFindings,
		r.Summary.Docs,
	)
}

// filterGapsOnly returns a copy of the report containing only files
// that have at least one finding. The summary is recomputed so the
// filtered report remains internally consistent.
func (r report) filterGapsOnly() report {
	out := report{SchemaVersion: r.SchemaVersion}
	for _, f := range r.Files {
		if len(f.Missing) == 0 && len(f.Orphan) == 0 && len(f.TierMismatch) == 0 {
			continue
		}
		out.Files = append(out.Files, f)
	}
	out.Summary = summarize(out.Files)
	return out
}

// buildReport walks the catalog and the docs directory, cross-checks
// each doc file against the catalog mapping, and returns the resulting
// report.
func buildReport(repoRoot, docsRoot, readmePath string, catalog *catalogSnapshot) (report, error) {
	mapping, err := loadDocMapping(readmePath)
	if err != nil {
		return report{}, fmt.Errorf("load doc mapping from %s: %w", readmePath, err)
	}

	docPaths, err := discoverDocFiles(docsRoot)
	if err != nil {
		return report{}, fmt.Errorf("discover docs under %s: %w", docsRoot, err)
	}

	docEntries, absByRel, relByBase := buildDocEntries(repoRoot, docPaths)
	applyReadmeMapping(mapping, docEntries, relByBase)
	expectedByDoc, unassignedTools := resolveExpectedByDoc(mapping, catalog, docEntries)

	if docErr := fillPerDocFindings(docEntries, absByRel, expectedByDoc, catalog); docErr != nil {
		return report{}, docErr
	}

	files := assembleFileList(mapping, docEntries, unassignedTools)

	return report{
		SchemaVersion: schemaVersion,
		Summary:       summarize(files),
		Files:         files,
	}, nil
}

// buildDocEntries walks the filesystem, recording each *.md file as
// a fileFinding keyed by its repo-relative path. The parallel
// absByRel/relByBase maps let later phases open files by their
// canonical doc path even when the docsRoot layout differs from
// docs/tools/ (only possible in tests).
func buildDocEntries(repoRoot string, docPaths []string) (docEntries map[string]*fileFinding, absByRel, relByBase map[string]string) {
	docEntries = make(map[string]*fileFinding, len(docPaths))
	absByRel = make(map[string]string, len(docPaths))
	relByBase = make(map[string]string, len(docPaths))
	for _, p := range docPaths {
		rel, relErr := filepath.Rel(repoRoot, p)
		if relErr != nil {
			rel = p
		}
		docEntries[rel] = &fileFinding{DocPath: rel}
		absByRel[rel] = p
		relByBase[filepath.Base(p)] = rel
	}
	return docEntries, absByRel, relByBase
}

// applyReadmeMapping projects each README row's domain name and
// expected count into its corresponding fileFinding. README rows
// whose doc file does not exist on disk still get an entry so the
// auditor can flag the missing doc instead of silently dropping it.
// When the README's relative path differs from the filesystem
// layout, we look the file up by basename (only possible in tests).
func applyReadmeMapping(mapping *docMapping, docEntries map[string]*fileFinding, relByBase map[string]string) {
	for _, row := range mapping.Rows {
		rel := relativeDocPath(row.DocLink)
		entry := docEntries[rel]
		if entry == nil {
			if existingRel, ok := relByBase[filepath.Base(rel)]; ok {
				rel = existingRel
				entry = docEntries[rel]
			}
		}
		if entry == nil {
			entry = &fileFinding{DocPath: rel}
			docEntries[rel] = entry
		}
		entry.Domain = row.Domain
		entry.ReadmeCount = row.ExpectedCount
	}
}

// resolveExpectedByDoc walks the catalog under the README mapping
// and returns the expected tool set per doc (keyed by the same
// relative path used by docEntries). It also returns the list of
// catalog tools that no doc claims; these surface as the
// "(unassigned)" pseudo-entry.
func resolveExpectedByDoc(mapping *docMapping, catalog *catalogSnapshot, docEntries map[string]*fileFinding) (expectedByDoc map[string][]string, unassignedTools []string) {
	expectedByDocCanonical, unassignedTools := computeExpectedByDoc(mapping, catalog)
	canonicalByBase := make(map[string]string, len(expectedByDocCanonical))
	for canonicalPath := range expectedByDocCanonical {
		canonicalByBase[filepath.Base(canonicalPath)] = canonicalPath
	}
	expectedByDoc = make(map[string][]string, len(docEntries))
	for rel := range docEntries {
		if tools, ok := expectedByDocCanonical[rel]; ok {
			expectedByDoc[rel] = tools
			continue
		}
		if canonicalPath, ok := canonicalByBase[filepath.Base(rel)]; ok {
			expectedByDoc[rel] = expectedByDocCanonical[canonicalPath]
		}
	}
	return expectedByDoc, unassignedTools
}

// fillPerDocFindings computes the per-doc missing/orphan/tier_mismatch
// sets and the count invariants. Doc entries without a backing file
// on disk (e.g. README rows whose doc path doesn't exist yet) are
// reported as fully-missing rather than failing the entire audit;
// only genuine parse failures abort the run.
func fillPerDocFindings(docEntries map[string]*fileFinding, absByRel map[string]string, expectedByDoc map[string][]string, catalog *catalogSnapshot) error {
	for docPath, entry := range docEntries {
		entry.ExpectedCount = len(expectedByDoc[docPath])
		expectedSet := stringSet(expectedByDoc[docPath])

		absPath, hasFile := absByRel[docPath]
		if !hasFile {
			// Doc file missing on disk: every expected tool is
			// missing and there is nothing documented. Leave
			// Missing/Orphan populated by the set diff below.
			entry.DocumentedCount = 0
			entry.CountMatches = entry.DocumentedCount == entry.ExpectedCount
			entry.ReadmeCountMatches = entry.ExpectedCount == entry.ReadmeCount
			entry.Missing = sortedSetMinus(expectedSet, stringSet(nil))
			entry.Orphan = nil
			continue
		}

		documented, parseErr := parseDocTools(absPath)
		if parseErr != nil {
			return fmt.Errorf("parse %s: %w", docPath, parseErr)
		}
		entry.DocumentedCount = len(documented)
		entry.CountMatches = entry.DocumentedCount == entry.ExpectedCount
		entry.ReadmeCountMatches = entry.ExpectedCount == entry.ReadmeCount
		entry.Missing = sortedSetMinus(expectedSet, stringSet(documented))
		entry.Orphan = sortedSetMinus(stringSet(documented), expectedSet)
		entry.TierMismatch = computeTierMismatches(absByRel[docPath], documented, catalog)
	}
	return nil
}

// computeTierMismatches best-effort compares catalog Edition vs
// tier badge detected next to each tool heading. Tools absent from
// the catalog (e.g. standalone surface utilities) and tools with no
// badge are skipped; only explicit disagreements surface.
func computeTierMismatches(absPath string, documented []string, catalog *catalogSnapshot) []tierMismatch {
	var out []tierMismatch
	for _, tool := range documented {
		info, ok := catalog.Tools[tool]
		if !ok {
			continue
		}
		badge, found := parseTierBadge(absPath, tool)
		if !found {
			continue
		}
		if tierBadgeMatches(badge, info.Tier) {
			continue
		}
		out = append(out, tierMismatch{
			Tool:       tool,
			Catalog:    tierLabel(info.Tier),
			Documented: badge,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tool < out[j].Tool })
	return out
}

// assembleFileList orders the report's fileFinding slice
// deterministically: README rows first (in table order), then any
// stray filesystem entries not referenced by the README, then the
// "(unassigned)" pseudo-entry when there are catalog tools no doc
// claims.
func assembleFileList(mapping *docMapping, docEntries map[string]*fileFinding, unassignedTools []string) []fileFinding {
	files := make([]fileFinding, 0, len(docEntries)+1)
	for _, row := range mapping.Rows {
		rel := relativeDocPath(row.DocLink)
		if entry, ok := docEntries[rel]; ok {
			files = append(files, *entry)
			delete(docEntries, rel)
		}
	}
	for _, k := range sortedKeys(docEntries) {
		files = append(files, *docEntries[k])
	}
	if len(unassignedTools) > 0 {
		files = append(files, fileFinding{
			DocPath:              unassignedDocPath,
			Domain:               "tools not present in the Domains table",
			UnassignedExpected:   sortedStrings(unassignedTools),
			UnassignedDocumented: nil,
		})
	}
	return files
}

// summarize counts global findings across all files. Files with only
// clean counts (no missing/orphan/tier_mismatch) count toward
// CleanDocs; files with any blocking finding count toward
// DocsWithFindings. ReadmeCountDriftDocs is the number of files whose
// ExpectedCount diverges from ReadmeCount, which signals that the
// Domains table needs a regen after catalog changes. UnassignedTotal
// sums the (unassigned) pseudo-entry's unassigned_expected and counts
// catalog tools that no README row has been routed to.
func summarize(files []fileFinding) reportSummary {
	s := reportSummary{Docs: len(files)}
	for _, f := range files {
		s.MissingTotal += len(f.Missing)
		s.OrphanTotal += len(f.Orphan)
		s.TierMismatchTotal += len(f.TierMismatch)
		switch {
		case f.DocPath == unassignedDocPath:
			s.UnassignedTotal += len(f.UnassignedExpected) + len(f.UnassignedDocumented)
			// Unassigned is itself a "finding" — every unassigned
			// tool must be routed explicitly before merge.
			s.DocsWithFindings++
		case len(f.Missing) > 0 || len(f.Orphan) > 0 || len(f.TierMismatch) > 0:
			s.DocsWithFindings++
		default:
			s.CleanDocs++
		}
		if !f.ReadmeCountMatches && f.DocPath != unassignedDocPath {
			s.ReadmeCountDriftDocs++
		}
	}
	return s
}

// writeReport writes content to outputPath, creating parent
// directories as needed. The sentinel "-" writes to stdout, matching the
// -output convention of audit_1to1 and audit_discovery_completeness.
func writeReport(outputPath string, content []byte) error {
	if outputPath == "-" {
		_, err := os.Stdout.Write(content)
		return err
	}
	if dir := filepath.Dir(outputPath); dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return os.WriteFile(outputPath, content, 0o600)
}

// stringSet returns set membership for a slice of strings.
func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}

// sortedDiff returns the elements in want that are absent from got.
// Returns nil for a nil want so callers can distinguish "empty diff"
// from "no findings" when comparing against nil.
func sortedDiff(want, got []string) []string {
	if len(want) == 0 {
		return nil
	}
	gotSet := stringSet(got)
	out := make([]string, 0)
	for _, v := range want {
		if _, ok := gotSet[v]; !ok {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// sortedSetMinus returns the elements of a that are absent from b,
// in lex order. Both arguments are sets; the result is a sorted slice.
// Used symmetrically to compute both Missing (expected - documented)
// and Orphan (documented - expected) without reallocating an
// intermediate slice on each call.
func sortedSetMinus(a, b map[string]struct{}) []string {
	out := make([]string, 0, len(a))
	for v := range a {
		if _, ok := b[v]; !ok {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

// sortedKeys returns the keys of m in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedStrings returns a sorted copy of values.
func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
