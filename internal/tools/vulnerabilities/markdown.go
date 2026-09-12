package vulnerabilities

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// The states GitLab's VulnerabilityState enum takes. They decide which
// transitions a reader can still make, which is why the hints read them: a
// dismissed vulnerability cannot be dismissed again, and a detected one has
// nothing to revert to.
const (
	stateDetected  = "DETECTED"
	stateConfirmed = "CONFIRMED"
	stateDismissed = "DISMISSED"
	stateResolved  = "RESOLVED"
)

// FormatListMarkdown renders a page of vulnerabilities as a Markdown table: a
// collection of objects that share columns. The ID is a column because it is
// what every other vulnerability action takes, and the list used to be a page
// a model could read and not act on.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Vulnerabilities) == 0 {
		return toolutil.EmptyMessage("vulnerabilities")
	}
	var b strings.Builder
	// A cursor-paginated connection sends no total, so the heading counts what
	// is shown and says whether more follows, which is all the response knows.
	toolutil.WriteListHeading(&b, "Vulnerabilities", len(out.Vulnerabilities),
		toolutil.PaginationOutput{HasMore: out.Pagination.HasNextPage})
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Severity", "Title", "State", "Scanner", "Report Type", "Detected"))
	for _, v := range out.Vulnerabilities {
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(v.ID),
			toolutil.SeverityBadge(v.Severity),
			toolutil.EscapeMdTableCell(listTitle(v)),
			toolutil.EscapeMdTableCell(v.State),
			toolutil.EscapeMdTableCell(scannerName(v.Scanner)),
			toolutil.EscapeMdTableCell(v.ReportType),
			toolutil.FormatTime(v.DetectedAt),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Vulnerabilities))
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionVulnGet, "read one vulnerability in full, naming the ID above"),
		toolutil.HintAction(actionVulnSeverityCount, "see how many vulnerabilities the project has at each severity"),
		toolutil.HintAction(actionSecurityFindingList, "read the findings one pipeline's scanners reported"),
	)
	return b.String()
}

// listTitle names a vulnerability in a list row: its title, and the primary
// identifier beside it when GitLab sent one that is not the title itself.
func listTitle(v Item) string {
	if v.PrimaryID == nil || v.PrimaryID.Name == "" || v.PrimaryID.Name == v.Title {
		return v.Title
	}
	return fmt.Sprintf("%s (%s)", v.Title, v.PrimaryID.Name)
}

// scannerName is the scanner's name, or "" when GitLab sent no scanner.
func scannerName(s *ScannerItem) string {
	if s == nil {
		return ""
	}
	return s.Name
}

// FormatGetMarkdown renders one vulnerability as the card of one object: what
// it is, where it was found, what has happened to it, its identifiers as a
// nested collection, and the scanner's own prose as quoted text.
func FormatGetMarkdown(out GetOutput) string {
	v := out.Vulnerability
	var b strings.Builder
	// The title comes out of the security report artifact, which a repository's
	// own CI job writes.
	c := toolutil.NewCard(&b, vulnerabilityHeading(v.Title))
	writeVulnerabilityRows(c, v)
	writeVulnerabilityIdentifiers(c, v.Identifiers)
	c.End(append(
		stateTransitionHints(v.State),
		toolutil.HintAction(actionVulnList, "see the project's other vulnerabilities"),
	)...)
	return b.String()
}

// writeVulnerabilityRows writes the rows one vulnerability renders with, so
// the detail view and the mutation confirmation cannot drift apart.
func writeVulnerabilityRows(c *toolutil.Card, v Item) {
	c.Code("ID", v.ID)
	c.Field("Title", v.Title)
	c.Field("Severity", toolutil.SeverityBadge(v.Severity))
	c.Field("State", v.State)
	c.Field("Report Type", v.ReportType)
	c.Field("Scanner", scannerLabel(v.Scanner))
	if v.PrimaryID != nil {
		// Both halves come out of the report's identifier object, so the name
		// can close the label and the URL can close the destination.
		c.Link("Primary Identifier", v.PrimaryID.Name, v.PrimaryID.URL)
	}
	if v.Location != nil {
		c.Code("Location", formatVulnerabilityLocation(v.Location))
	}
	c.Time("Detected", v.DetectedAt)
	c.Time("Confirmed", v.ConfirmedAt)
	c.Time("Dismissed", v.DismissedAt)
	c.Time("Resolved", v.ResolvedAt)
	c.Field("Dismissal Reason", v.DismissalReason)
	c.Bool("Has Issues", v.HasIssues)
	c.Bool("Has Merge Request", v.HasMR)
	c.Bool("Has Remediations", v.HasRemediations)
	// The nested label is opened only when something goes under it: a Sub
	// writes its row whatever its card writes, and a label with nothing after
	// it is what the card rule exists to prevent.
	if p := v.Project; p != nil && (p.ID != "" || p.Name != "" || p.FullPath != "") {
		project := c.Sub("Project")
		project.Code("ID", p.ID)
		project.Field("Name", p.Name)
		project.Field("Full Path", p.FullPath)
	}
	c.URL(v.WebURL)
	// The solution and the description are the scanner's own prose, which a
	// repository's CI job produced: quoted under their labels, they can open no
	// heading, no list item and no guidance section of the response.
	c.Text("Solution", v.Solution)
	c.Text("Description", v.Description)
}

// scannerLabel names the scanner and, when GitLab sent one, its vendor.
func scannerLabel(s *ScannerItem) string {
	if s == nil {
		return ""
	}
	if s.Vendor == "" {
		return s.Name
	}
	return s.Name + " (" + s.Vendor + ")"
}

// vulnerabilityHeading names the vulnerability in the card's heading, or opens
// the generic one when the report carried no title.
func vulnerabilityHeading(title string) string {
	if strings.TrimSpace(title) == "" {
		return "Vulnerability"
	}
	return "Vulnerability: " + title
}

// formatVulnerabilityLocation renders where the scanner found the finding, as
// the file and the line range the report named.
func formatVulnerabilityLocation(location *LocationItem) string {
	loc := location.File
	if location.StartLine > 0 {
		loc += fmt.Sprintf(":%d", location.StartLine)
		if location.EndLine > 0 && location.EndLine != location.StartLine {
			loc += fmt.Sprintf("-%d", location.EndLine)
		}
	}
	return loc
}

// writeVulnerabilityIdentifiers renders the report's identifiers as the nested
// collection they are, and nothing when it carried none.
func writeVulnerabilityIdentifiers(c *toolutil.Card, identifiers []IdentifierItem) {
	if len(identifiers) == 0 {
		return
	}
	table := c.Table("Identifiers", "Name", "Type", "External ID", "URL")
	for _, id := range identifiers {
		// The cell escaper neutralizes the pipe and the angle bracket and leaves
		// ']' alone, so an identifier named "CWE-89](http://attacker.invalid/x)"
		// closed the label and retargeted the link. MdTitleLink escapes both halves.
		table.Row(
			toolutil.MdTitleLink(id.Name, id.URL),
			toolutil.EscapeMdTableCell(id.ExternalType),
			toolutil.EscapeMdTableCell(id.ExternalID),
			toolutil.EscapeMdTableCell(id.URL),
		)
	}
}

// stateTransitionHints returns the transitions the vulnerability's current
// state still allows. Offering to dismiss a dismissed vulnerability, or to
// revert a detected one, names a call GitLab refuses; a state this server has
// not heard of offers all four and lets GitLab answer.
func stateTransitionHints(state string) []string {
	current := strings.ToUpper(strings.TrimSpace(state))
	var hints []string
	if current != stateDismissed {
		hints = append(hints, toolutil.HintAction(actionVulnDismiss, "dismiss it as an acceptable risk or a false positive"))
	}
	if current != stateConfirmed {
		hints = append(hints, toolutil.HintAction(actionVulnConfirm, "confirm it as a real vulnerability"))
	}
	if current != stateResolved {
		hints = append(hints, toolutil.HintAction(actionVulnResolve, "mark it resolved"))
	}
	if current != stateDetected {
		hints = append(hints, toolutil.HintAction(actionVulnRevert, "revert it to detected"))
	}
	return hints
}

// FormatMutationMarkdown renders the vulnerability a state change answered
// with, as the card of one object. The action names what GitLab did, and the
// hints name what its new state still allows.
func FormatMutationMarkdown(out MutationOutput, action string) string {
	v := out.Vulnerability
	var b strings.Builder
	c := toolutil.NewCard(&b, "Vulnerability "+action)
	writeVulnerabilityRows(c, v)
	c.End(append(
		stateTransitionHints(v.State),
		toolutil.HintAction(actionVulnList, "see the project's other vulnerabilities"),
	)...)
	return b.String()
}

// FormatSeverityCountMarkdown renders the project's vulnerability counts as
// the card of one object: one row per severity, each labeled with the badge
// the security domains share, and the total last. Every count is written, zero
// included: "no criticals" is the answer a reader came for.
func FormatSeverityCountMarkdown(out SeverityCountOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Vulnerability Severity Counts")
	c.Int(toolutil.SeverityBadge("CRITICAL"), int64(out.Critical))
	c.Int(toolutil.SeverityBadge("HIGH"), int64(out.High))
	c.Int(toolutil.SeverityBadge("MEDIUM"), int64(out.Medium))
	c.Int(toolutil.SeverityBadge("LOW"), int64(out.Low))
	c.Int(toolutil.SeverityBadge("INFO"), int64(out.Info))
	c.Int(toolutil.SeverityBadge("UNKNOWN"), int64(out.Unknown))
	c.Int("Total", int64(out.Total))
	c.End(
		toolutil.HintAction(actionVulnList, "read the vulnerabilities behind these counts"),
		toolutil.HintAction(actionVulnPipelineSummary, "see what one pipeline's scanners reported"),
	)
	return b.String()
}

// FormatPipelineSecuritySummaryMarkdown renders one pipeline's security report
// summary as a table of the scanners that ran: a collection of objects that
// share columns.
func FormatPipelineSecuritySummaryMarkdown(out PipelineSecuritySummaryOutput) string {
	var b strings.Builder
	b.WriteString("## Pipeline Security Report Summary\n\n")

	scanners := []struct {
		name    string
		summary *ScannerSummaryItem
	}{
		{"SAST", out.Sast},
		{"DAST", out.Dast},
		{"Dependency Scanning", out.DependencyScanning},
		{"Container Scanning", out.ContainerScanning},
		{"Secret Detection", out.SecretDetection},
		{"Coverage Fuzzing", out.CoverageFuzzing},
		{"API Fuzzing", out.APIFuzzing},
		{"Cluster Image Scanning", out.ClusterImageScanning},
	}

	ran := 0
	for _, s := range scanners {
		if s.summary != nil {
			ran++
		}
	}
	if ran == 0 && out.TotalVulnerabilities == 0 {
		b.WriteString("No security scans ran in this pipeline.\n")
		return b.String()
	}

	b.WriteString(toolutil.MarkdownTableHeader("Scanner", "Vulnerabilities", "Scanned Resources"))
	for _, s := range scanners {
		if s.summary == nil {
			continue
		}
		b.WriteString(toolutil.MarkdownTableRow(
			s.name,
			strconv.Itoa(s.summary.VulnerabilitiesCount),
			strconv.Itoa(s.summary.ScannedResourcesCount),
		))
	}

	fmt.Fprintf(&b, "\n**Total Vulnerabilities: %d**\n", out.TotalVulnerabilities)
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionSecurityFindingList, "read the findings these scanners reported"),
		toolutil.HintAction(actionVulnSeverityCount, "see the project's counts by severity"),
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(func(v MutationOutput) string { return FormatMutationMarkdown(v, "updated") })
	toolutil.RegisterMarkdown(FormatSeverityCountMarkdown)
	toolutil.RegisterMarkdown(FormatPipelineSecuritySummaryMarkdown)
}
