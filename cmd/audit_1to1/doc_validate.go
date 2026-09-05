// doc_validate.go implements the -validate-docs mode: it cross-checks the
// `doc/api/<area>.md` citations that justify the 1:1 adjudication tables against
// the live GitLab API reference docs (via internal/apidocs). The official API
// doc is the 1:1 ground truth, so a citation pointing at a doc that no longer
// exists (renamed/removed upstream) silently invalidates an adjudication — this
// gate surfaces it.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/audit_1to1/internal/shared"
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/internal/apidocs"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

// docCitationRE matches a `doc/api/<area>.md` reference in the auditor source.
var docCitationRE = regexp.MustCompile(`doc/api/([a-z0-9_]+)\.md`)

// docValidationReport is the JSON emitted by -validate-docs.
type docValidationReport struct {
	SchemaVersion int                `json:"schema_version"`
	Checked       int                `json:"checked"`
	OK            int                `json:"ok"`
	Stale         []docCitationIssue `json:"stale"`
}

// docCitationIssue is one cited doc area that could not be validated.
type docCitationIssue struct {
	Area  string `json:"area"`
	Error string `json:"error"`
}

// runValidateDocs scans the audit_1to1 source for doc/api citations and verifies
// each cited area is fetchable and non-empty. It returns the report JSON and a
// boolean that is true when every citation validated (callers exit non-zero
// otherwise so this can gate CI).
func runValidateDocs(ctx context.Context, root string, fetcher *apidocs.Fetcher) (report []byte, ok bool, err error) {
	areas, err := scanDocCitations(root)
	if err != nil {
		return nil, false, err
	}

	rep := docValidationReport{SchemaVersion: shared.SchemaVersion, Checked: len(areas)}
	cmdutil.Progressf("audit_1to1: validating %d cited API docs against the live source...", len(areas))
	for _, area := range areas {
		content, fetchErr := fetcher.Fetch(ctx, area)
		switch {
		case fetchErr != nil:
			rep.Stale = append(rep.Stale, docCitationIssue{Area: area, Error: fetchErr.Error()})
		case strings.TrimSpace(content) == "":
			rep.Stale = append(rep.Stale, docCitationIssue{Area: area, Error: "doc is empty"})
		default:
			rep.OK++
		}
	}

	out, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return nil, false, fmt.Errorf("marshal validation report: %w", err)
	}
	out = append(out, '\n')
	return out, len(rep.Stale) == 0, nil
}

// scanDocCitations walks the audit_1to1 command tree and returns the sorted,
// deduplicated set of `doc/api/<area>.md` areas cited anywhere in its source
// (the citations live in comments next to each adjudication entry). Scanning the
// source keeps the validation set in sync with the comments automatically.
func scanDocCitations(root string) ([]string, error) {
	srcDir := filepath.Join(root, "cmd", "audit_1to1")
	seen := map[string]struct{}{}
	err := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path) //#nosec G304,G122 -- scanning the repo's own audit_1to1 source tree, not user input
		if readErr != nil {
			return readErr
		}
		for _, m := range docCitationRE.FindAllStringSubmatch(string(data), -1) {
			seen[m[1]] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan doc citations: %w", err)
	}
	areas := make([]string, 0, len(seen))
	for a := range seen {
		areas = append(areas, a)
	}
	sort.Strings(areas)
	return areas, nil
}
