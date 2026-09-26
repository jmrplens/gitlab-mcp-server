package vulnerabilities

import (
	"fmt"
	"slices"
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
	// Whether the footer asks for links to be kept is read off the rows that
	// were written, not decided in advance. The schema types webUrl non-null,
	// so in practice every row carries one; a response that sent none would
	// otherwise be answered with an instruction about links the table does not
	// have, which is what the table was written without a link to avoid.
	linked := false
	for _, v := range out.Vulnerabilities {
		linked = linked || toolutil.LinkableDestination(v.WebURL)
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.MdCodeSpanCell(v.ID),
			toolutil.SeverityBadge(v.Severity),
			// The title carries the vulnerability's own page, which is what a
			// reader of a triage list reaches for first. MdTitleLink answers
			// with the escaped title alone when GitLab sent no address, so a
			// row is never worse off than it was before the link existed.
			toolutil.MdTitleLink(listTitle(v), v.WebURL),
			toolutil.EscapeMdTableCell(v.State),
			toolutil.EscapeMdTableCell(scannerName(v.Scanner)),
			toolutil.EscapeMdTableCell(v.ReportType),
			toolutil.FormatTime(v.DetectedAt),
		))
	}
	toolutil.WriteGraphQLPagination(&b, out.Pagination, len(out.Vulnerabilities))
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, linked,
		toolutil.HintPreserveLinks,
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
	writeVulnerabilityCollections(c, v)
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
	c.Code("UUID", v.UUID)
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
	writeLocationRows(c, v.Location)
	c.Time("Detected", v.DetectedAt)
	c.Time("Updated", v.UpdatedAt)
	c.Time("Confirmed", v.ConfirmedAt)
	writeUser(c, "Confirmed By", v.ConfirmedBy)
	c.Time("Dismissed", v.DismissedAt)
	writeUser(c, "Dismissed By", v.DismissedBy)
	c.Time("Resolved", v.ResolvedAt)
	writeUser(c, "Resolved By", v.ResolvedBy)
	c.Field("Dismissal Reason", v.DismissalReason)
	c.Time("Due Date", v.DueDate)
	c.Bool("Present On Default Branch", v.PresentOnDefaultBranch)
	// The two signals that the code no longer carries the vulnerability are
	// stated only when they hold, the way GitLab's report badges them.
	c.Flag("", "No longer detected on the default branch", v.ResolvedOnDefaultBranch)
	c.Flag("", "Removed from the code", v.RemovedFromCode)
	c.BoolPtr("False Positive", v.FalsePositive)
	c.Warn("Unverified: detected without an identified source", v.Unverified)
	c.Bool("Has Issues", v.HasIssues)
	c.Bool("Has Merge Request", v.HasMR)
	if mr := v.MergeRequest; mr != nil {
		c.Link("Merge Request", referenceLabel("!", mr), mr.WebURL)
	}
	c.Bool("Has Remediations", v.HasRemediations)
	c.Count("User Notes", int64(v.UserNotesCount))
	writeRiskRows(c, v)
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
	// The state comment is what a person typed when they changed the state,
	// and the solution and the description are the scanner's own prose, which
	// a repository's CI job produced: quoted under their labels, none of them
	// can open a heading, a list item or a guidance section of the response.
	c.Text("State Comment", v.StateComment)
	c.Text("Solution", v.Solution)
	c.Text("Description", v.Description)
	if v.Location != nil {
		c.Text("Location Description", v.Location.Description)
	}
}

// writeLocationRows writes where the scanner found the vulnerability: the file
// and line range every scan type that names a file has, then whatever the
// scan type adds, which for DAST is the request and for dependency and
// container scanning the package.
func writeLocationRows(c *toolutil.Card, location *LocationItem) {
	if location == nil {
		return
	}
	c.Code("Location", formatVulnerabilityLocation(location))
	c.Code("Vulnerable Class", location.VulnerableClass)
	c.Code("Vulnerable Method", location.VulnerableMethod)
	c.Field("Request Method", location.RequestMethod)
	c.Field("Hostname", location.Hostname)
	c.Code("Parameter", location.Param)
	if d := location.Dependency; d != nil {
		c.Code("Dependency", dependencyLabel(d))
		c.Code("Dependency Path", d.PackagePath)
	}
	c.Field("Operating System", location.OperatingSystem)
	c.Code("Container Repository", location.ContainerRepositoryURL)
	if k := location.KubernetesResource; k != nil {
		workload := c.Sub("Kubernetes Resource")
		workload.Field("Kind", k.Kind)
		workload.Field("Namespace", k.Namespace)
		workload.Field("Name", k.Name)
		workload.Field("Container", k.ContainerName)
		workload.Field("Agent", k.AgentName)
		workload.Code("Cluster ID", k.ClusterID)
	}
	c.Field("Crash Type", location.CrashType)
	c.Code("Crash Address", location.CrashAddress)
}

// writeRiskRows writes what GitLab knows about how dangerous the vulnerability
// is beyond its severity: the CVE's exploitation data, and whether a leaked
// secret still works.
func writeRiskRows(c *toolutil.Card, v Item) {
	if e := v.CVEEnrichment; e != nil {
		enrichment := c.Sub("CVE Enrichment")
		enrichment.Code("CVE", e.CVE)
		enrichment.Field("EPSS Score", strconv.FormatFloat(e.EPSSScore, 'f', -1, 64))
		enrichment.Warn("Known exploited (CISA KEV)", e.IsKnownExploit)
	}
	if s := v.TokenStatus; s != nil {
		token := c.Sub("Token Status")
		token.Field("Status", s.Status)
		token.Time("Last Verified", s.LastVerifiedAt)
	}
}

// writeUser writes a person GitLab recorded against a state change, linked to
// their profile, and nothing when GitLab recorded nobody.
func writeUser(c *toolutil.Card, label string, user *toolutil.UserCoreRefOutput) {
	if user == nil {
		return
	}
	text := "@" + user.Username
	if user.Name != "" {
		text = user.Name + " (@" + user.Username + ")"
	}
	c.Link(label, text, user.WebURL)
}

// dependencyLabel names a vulnerable package the way a lockfile does, with the
// version after an at sign when the report named one.
func dependencyLabel(d *toolutil.VulnerableDependencyOutput) string {
	if d.Version == "" {
		return d.PackageName
	}
	return d.PackageName + "@" + d.Version
}

// referenceLabel names a linked issue or merge request by its reference and
// title, the way GitLab writes one in prose.
func referenceLabel(sigil string, r *ReferenceItem) string {
	label := sigil + strconv.FormatInt(r.IID, 10)
	if r.Title != "" {
		label += " " + r.Title
	}
	return label
}

// writeVulnerabilityCollections writes the nested collections of the detail
// view after its rows and its identifiers: the CVSS assessments, the report's
// links, the linked issues, and the fuzzer's stack trace.
func writeVulnerabilityCollections(c *toolutil.Card, v Item) {
	if len(v.CVSS) > 0 {
		table := c.Table("CVSS", "Vendor", "Version", "Base Score", "Overall Score", "Severity", "Vector")
		for _, cvss := range v.CVSS {
			// CVSS writes its versions and scores with one decimal, 4.0 and 9.8,
			// and a version printed as 4 names no version CVSS has.
			table.Row(
				toolutil.EscapeMdTableCell(cvss.Vendor),
				strconv.FormatFloat(cvss.Version, 'f', 1, 64),
				strconv.FormatFloat(cvss.BaseScore, 'f', 1, 64),
				strconv.FormatFloat(cvss.OverallScore, 'f', 1, 64),
				toolutil.EscapeMdTableCell(cvss.Severity),
				toolutil.MdCodeSpanCell(cvss.Vector),
			)
		}
	}
	if len(v.Links) > 0 {
		table := c.Table("Links", "Name", "URL")
		for _, link := range v.Links {
			table.Row(toolutil.MdTitleLink(linkName(link), link.URL), toolutil.EscapeMdTableCell(link.URL))
		}
	}
	if len(v.IssueLinks) > 0 {
		table := c.Table("Linked Issues", "Issue", "State", "Link Type")
		for _, link := range v.IssueLinks {
			issue, state, url := "", "", ""
			if link.Issue != nil {
				issue, state, url = referenceLabel("#", link.Issue), link.Issue.State, link.Issue.WebURL
			}
			table.Row(toolutil.MdTitleLink(issue, url), toolutil.EscapeMdTableCell(state), toolutil.EscapeMdTableCell(link.LinkType))
		}
	}
	if v.Location != nil {
		c.Fence("Stack Trace", "", v.Location.StacktraceSnippet)
	}
}

// linkName is what a report link is shown as: its name, or its address when
// the report gave it none.
func linkName(link toolutil.VulnerabilityLinkOutput) string {
	if link.Name == "" {
		return link.URL
	}
	return link.Name
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

	// The names are this server's own, and are still escaped where they land
	// in a cell: the escaping gate cannot follow one through a range over this
	// table, and the escaper leaves every one of them as it is.
	scanners := []scannerSection{
		{name: "SAST", summary: out.Sast},
		{name: "DAST", summary: out.Dast},
		{name: "Dependency Scanning", summary: out.DependencyScanning},
		{name: "Container Scanning", summary: out.ContainerScanning},
		{name: "Secret Detection", summary: out.SecretDetection},
		{name: "Coverage Fuzzing", summary: out.CoverageFuzzing},
		{name: "API Fuzzing", summary: out.APIFuzzing},
		{name: "Cluster Image Scanning", summary: out.ClusterImageScanning},
	}

	ran := slices.ContainsFunc(scanners, func(s scannerSection) bool { return s.summary != nil })
	if !ran && out.TotalVulnerabilities == 0 {
		b.WriteString("No security scans ran in this pipeline.\n")
		return b.String()
	}

	b.WriteString(toolutil.MarkdownTableHeader("Scanner", "Vulnerabilities", "Scanned Resources", "Scans"))
	for _, s := range scanners {
		if s.summary == nil {
			continue
		}
		b.WriteString(toolutil.MarkdownTableRow(
			toolutil.EscapeMdTableCell(s.name),
			strconv.Itoa(s.summary.VulnerabilitiesCount),
			strconv.Itoa(s.summary.ScannedResourcesCount),
			toolutil.EscapeMdTableCell(scanStatuses(s.summary.Scans)),
		))
	}

	fmt.Fprintf(&b, "\n**Total Vulnerabilities: %d**\n", out.TotalVulnerabilities)
	writeScanMessages(&b, scanners)
	writeScannedResources(&b, scanners)
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, toolutil.PaginationOutput{}, false,
		toolutil.HintAction(actionSecurityFindingList, "read the findings these scanners reported"),
		toolutil.HintAction(actionVulnSeverityCount, "see the project's counts by severity"),
	)
	return b.String()
}

// scannerSection is one scan type's section of the summary, under the name the
// table shows it by.
type scannerSection struct {
	name    string
	summary *ScannerSummaryItem
}

// scanStatuses names the scans that ran for a scan type and how each ended.
func scanStatuses(scans []ScanItem) string {
	parts := make([]string, 0, len(scans))
	for _, scan := range scans {
		parts = append(parts, scan.Name+" ("+scan.Status+")")
	}
	return strings.Join(parts, ", ")
}

// writeScanMessages writes the errors and warnings the analyzers wrote into
// their reports, one row per message, and nothing when no scan reported any:
// a scan type that found nothing because its scan failed is told apart here
// from one that found nothing to report.
func writeScanMessages(b *strings.Builder, scanners []scannerSection) {
	var rows []string
	for _, s := range scanners {
		if s.summary == nil {
			continue
		}
		for _, scan := range s.summary.Scans {
			for _, message := range scan.Errors {
				rows = append(rows, toolutil.MarkdownTableRow(toolutil.EscapeMdTableCell(s.name), toolutil.EscapeMdTableCell(scan.Name), "error", toolutil.EscapeMdTableCell(message)))
			}
			for _, message := range scan.Warnings {
				rows = append(rows, toolutil.MarkdownTableRow(toolutil.EscapeMdTableCell(s.name), toolutil.EscapeMdTableCell(scan.Name), "warning", toolutil.EscapeMdTableCell(message)))
			}
		}
	}
	writeSummaryTable(b, "Scan Errors and Warnings", []string{"Scanner", "Scan", "Kind", "Message"}, rows)
}

// writeScannedResources writes the resources the DAST and API fuzzing scans
// requested, as many as GitLab sends, and nothing when no scan listed any.
func writeScannedResources(b *strings.Builder, scanners []scannerSection) {
	var rows []string
	for _, s := range scanners {
		if s.summary == nil {
			continue
		}
		for _, resource := range s.summary.ScannedResources {
			rows = append(rows, toolutil.MarkdownTableRow(toolutil.EscapeMdTableCell(s.name), toolutil.EscapeMdTableCell(resource.RequestMethod), toolutil.EscapeMdTableCell(resource.URL)))
		}
	}
	writeSummaryTable(b, "Scanned Resources", []string{"Scanner", "Method", "URL"}, rows)
}

// writeSummaryTable writes one of the summary's nested collections under its
// heading, and nothing at all when it has no rows.
func writeSummaryTable(b *strings.Builder, title string, columns, rows []string) {
	if len(rows) == 0 {
		return
	}
	fmt.Fprintf(b, "\n### %s\n\n", toolutil.EscapeMdHeading(title))
	b.WriteString(toolutil.MarkdownTableHeader(columns...))
	for _, row := range rows {
		b.WriteString(row)
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(func(v MutationOutput) string { return FormatMutationMarkdown(v, "updated") })
	toolutil.RegisterMarkdown(FormatSeverityCountMarkdown)
	toolutil.RegisterMarkdown(FormatPipelineSecuritySummaryMarkdown)
}
