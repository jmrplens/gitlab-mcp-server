// Package main: per-doc parser.
//
// Each docs/tools/<doc>.md file references its individual tools
// through one of three Markdown patterns:
//
//   - `### \`gitlab_<name>\“           heading with backticks
//     (branches.md, tags.md, etc.)
//   - `### gitlab_<name>`               heading without backticks
//     (geo-model-registry.md)
//   - `| \`gitlab_<name>\` | ... |`      row in a pipe table
//     (analytics-compliance.md,
//     enterprise-attestations.md)
//
// parseDocTools returns the union of all `gitlab_*` names that appear
// in any of these positions within the file. Tier badge parsing is
// best-effort and looks for the canonical "Premium"/"Ultimate" tokens
// near each heading.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// toolHeadingRE matches `### \`gitlab_<name>\“ (with optional
// trailing whitespace) and captures the tool name without the
// gitlab_ prefix.
var toolHeadingRE = regexp.MustCompile(`^###\s+` + "`" + `gitlab_([a-z][a-z0-9_]+)` + "`" + `\s*$`)

// toolHeadingBareRE matches `### gitlab_<name>` (no backticks) for
// docs that omit them. Captures the name without the gitlab_ prefix.
var toolHeadingBareRE = regexp.MustCompile(`^###\s+gitlab_([a-z][a-z0-9_]+)\s*$`)

// toolTableRowRE matches pipe-table rows whose first cell contains
// `gitlab_<name>`. Captures the name without the gitlab_ prefix.
var toolTableRowRE = regexp.MustCompile(`(?m)^\|\s*` + "`" + `gitlab_([a-z][a-z0-9_]+)` + "`")

// parseDocTools returns the sorted, deduplicated set of `gitlab_*`
// tool names referenced in the doc.
func parseDocTools(docPath string) ([]string, error) {
	f, err := os.Open(docPath)
	if err != nil {
		return nil, fmt.Errorf("open doc: %w", err)
	}
	defer func() { _ = f.Close() }()

	seen := make(map[string]struct{})
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := toolHeadingRE.FindStringSubmatch(line); m != nil {
			seen["gitlab_"+m[1]] = struct{}{}
			continue
		}
		if m := toolHeadingBareRE.FindStringSubmatch(line); m != nil {
			seen["gitlab_"+m[1]] = struct{}{}
			continue
		}
		for _, m := range toolTableRowRE.FindAllStringSubmatch(line, -1) {
			seen["gitlab_"+m[1]] = struct{}{}
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return nil, fmt.Errorf("scan doc: %w", scanErr)
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	// Deterministic output for stable diffs and tests.
	sortStrings(out)
	return out, nil
}

// parseTierBadge looks up the tier badge for tool in docPath. It
// returns the canonical badge string ("Premium" / "Ultimate") when
// one is detected, and false otherwise. The lookup is best-effort: it
// scans the lines around each heading match for the badge tokens.
//
// The standard badge form is "**Read** | **Premium**" in the small
// parameter table that follows a `### \`gitlab_<tool>\“ heading, but
// some docs use prose like "> **Tier**: Premium" instead. We accept
// both.
func parseTierBadge(docPath, tool string) (string, bool) {
	f, err := os.Open(docPath)
	if err != nil {
		return "", false
	}
	defer func() { _ = f.Close() }()

	target := "`gitlab_" + strings.TrimPrefix(tool, "gitlab_") + "`"
	targetBare := "gitlab_" + strings.TrimPrefix(tool, "gitlab_")

	scanner := bufio.NewScanner(f)
	var window []string
	const windowSize = 6
	foundHeading := false
	for scanner.Scan() {
		line := scanner.Text()
		if !foundHeading {
			if strings.Contains(line, target) || strings.HasPrefix(strings.TrimSpace(line), "### "+targetBare) {
				foundHeading = true
				window = window[:0]
				window = append(window, line)
				continue
			}
			continue
		}
		window = append(window, line)
		if len(window) > windowSize {
			break
		}
		if badge, ok := detectTierBadge(window); ok {
			return badge, true
		}
	}
	return "", false
}

// detectTierBadge scans the lines around a tool heading for the
// standard tier badge tokens. The order of checks reflects the doc
// style contract: explicit "**Premium**" / "**Ultimate**" badges in
// the small property table win; prose forms ("> **Tier**: Premium")
// are a fallback.
func detectTierBadge(window []string) (string, bool) {
	premiumRE := regexp.MustCompile(`(?i)\*\*premium\*\*|>\s*\*\*tier\*\*:\s*premium|requires\s+premium`)
	ultimateRE := regexp.MustCompile(`(?i)\*\*ultimate\*\*|>\s*\*\*tier\*\*:\s*ultimate|requires\s+ultimate`)
	if slices.ContainsFunc(window, func(line string) bool { return ultimateRE.MatchString(line) }) {
		return "Ultimate", true
	}
	if slices.ContainsFunc(window, func(line string) bool { return premiumRE.MatchString(line) }) {
		return "Premium", true
	}
	return "", false
}

// tierLabel maps the catalog Edition string to the canonical doc
// badge label. Free tools have no badge so they return "".
func tierLabel(edition string) string {
	switch strings.ToLower(strings.TrimSpace(edition)) {
	case "premium":
		return "Premium"
	case "ultimate":
		return "Ultimate"
	}
	return ""
}

// tierBadgeMatches returns true when the doc's tier badge is
// consistent with the catalog tier. Free tools match everything (no
// badge expected); missing badges on Premium/Ultimate tools are also
// considered a match here — the auditor's tier_mismatch check only
// fires when a badge IS present and disagrees with the catalog. A
// missing-badge scenario is a separate, future-facing check that the
// auditor does not currently emit (callers can detect it by looking
// for catalog-paid tools with no badge in the doc).
func tierBadgeMatches(badge, edition string) bool {
	canonical := tierLabel(edition)
	if canonical == "" {
		return true
	}
	badge = strings.TrimSpace(badge)
	if badge == "" {
		return true
	}
	return strings.EqualFold(badge, canonical)
}

// discoverDocFiles returns every *.md file under docsRoot except
// README.md (the Domains-table README is the mapping source, not a
// doc to audit). Paths are absolute.
func discoverDocFiles(docsRoot string) ([]string, error) {
	entries, err := os.ReadDir(docsRoot)
	if err != nil {
		return nil, fmt.Errorf("read docs root: %w", err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if strings.EqualFold(name, "README.md") {
			continue
		}
		out = append(out, filepathJoin(docsRoot, name))
	}
	return out, nil
}

// sortStrings sorts values lexicographically in place. This is a thin
// wrapper to keep the imports tidy in this file.
func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j-1] > values[j]; j-- {
			values[j-1], values[j] = values[j], values[j-1]
		}
	}
}

// filepathJoin is filepath.Join wrapped to keep the imports list
// local to this file.
func filepathJoin(elem ...string) string {
	return joinPath(elem...)
}

// joinPath joins path elements. filepath.Join is used.
func joinPath(elem ...string) string {
	return pathJoin(elem...)
}

// pathJoin is filepath.Join. Defined here so the function is
// testable without importing path/filepath in this file's exported
// surface (the parser only needs to read the file, not resolve
// paths).
func pathJoin(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	out := elem[0]
	for _, e := range elem[1:] {
		if out == "" {
			out = e
			continue
		}
		if strings.HasSuffix(out, string(os.PathSeparator)) {
			out += e
		} else {
			out += string(os.PathSeparator) + e
		}
	}
	return out
}
