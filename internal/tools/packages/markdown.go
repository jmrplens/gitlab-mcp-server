package packages

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// shortDigestLength is how much of a SHA-256 digest a table column shows
// before the ellipsis; a digest of exactly this length is left whole, so the
// ellipsis always means something was cut.
const shortDigestLength = 12

// packageNotFoundOutput is the answer to a package GitLab answered 404 for,
// naming the package and its project as the caller gave them.
type packageNotFoundOutput struct {
	Identifier string
}

// formatPackageNotFound renders a package GitLab could not find as the
// structured not-found result, with the two ways a package_id goes stale.
func formatPackageNotFound(out packageNotFoundOutput) *mcp.CallToolResult {
	return toolutil.NotFoundResult(
		"Package", out.Identifier,
		"Use package.list with project_id to list the project's packages and their package_id",
		"Each version of a package has its own package_id, and deleting that version retires it",
	)
}

// FormatGetMarkdown renders one package as the card of one object: its own
// fields, the pipeline that last built it, and the package's other versions
// as a nested table, which only this read is sent.
func FormatGetMarkdown(out GetOutput) string {
	p := out.Package
	var b strings.Builder
	c := toolutil.NewCard(&b, "Package: "+p.Name)
	c.Int("ID", p.ID)
	c.Field("Version", p.Version)
	c.Field("Type", p.PackageType)
	c.Field("Status", p.Status)
	c.Field("Conan Package", p.ConanPackageName)
	c.Count("Creator ID", p.CreatorID)
	c.Time("Created", p.CreatedAt)
	c.Time("Last Downloaded", p.LastDownloadedAt)
	if p.Links != nil {
		c.Code("Web Path", p.Links.WebPath)
	}
	// The summary arrives rendered: a link when GitLab gave the pipeline's
	// page, the escaped text otherwise, which is what pipelineItemSummary
	// writes for the listing's cell too.
	c.Markdown("Pipeline", pipelineSummary(p))
	c.Field("Tags", listTagNames(p.Tags))
	writeVersionsTable(c, p.Versions)
	c.End(
		toolutil.HintAction(actionPackageFileList, "list the files inside this package"),
		toolutil.HintAction("package.download", "download one of its files"),
		toolutil.HintAction(actionPackageDelete, "delete this version of the package"),
	)
	return b.String()
}

// listTagNames joins the names of the tags pointing at the package, for the
// one card row that lists them; the row escapes what it writes.
func listTagNames(tags []TagItem) string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, ", ")
}

// versionTagNames joins the names of the tags pointing at one other version.
func versionTagNames(tags []toolutil.PackageTagOutput) string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, ", ")
}

// writeVersionsTable writes the package's other versions as the nested
// collection they are, under a heading of the card's own: each version's id,
// the tags pointing at it, the pipeline that built it and when it was
// published. A package with no other version writes nothing.
func writeVersionsTable(c *toolutil.Card, versions []toolutil.PackageVersionOutput) {
	if len(versions) == 0 {
		return
	}
	t := c.Table("Other Versions", "ID", "Version", "Tags", "Pipeline", "Created")
	for _, v := range versions {
		t.Row(
			strconv.FormatInt(v.ID, 10),
			toolutil.EscapeMdTableCell(v.Version),
			toolutil.EscapeMdTableCell(versionTagNames(v.Tags)),
			versionPipelineSummary(v.Pipeline),
			toolutil.FormatTime(toolutil.RFC3339Ptr(v.CreatedAt)),
		)
	}
}

// versionPipelineSummary renders the pipeline that built one other version as
// the listing renders a package's own pipeline, or nothing when GitLab sent
// none.
func versionPipelineSummary(pipeline *toolutil.PackagePipelineOutput) string {
	if pipeline == nil {
		return ""
	}
	return pipelineItemSummary(PipelineItem{ID: pipeline.ID, Status: pipeline.Status, Ref: pipeline.Ref, WebURL: pipeline.WebURL})
}

// FormatPublishMarkdown renders a published package file as the card of one
// object.
func FormatPublishMarkdown(out PublishOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Package Published")
	c.Int("Package File ID", out.PackageFileID)
	c.Int("Package ID", out.PackageID)
	// The only validation on a package file name refuses a space and a leading
	// tilde or at-sign, so a pipe and a '<' both survive.
	c.Field("File Name", out.FileName)
	c.Field("Size", fmt.Sprintf("%d bytes", out.Size))
	// A digest is a value a reader compares character by character, so it is a
	// code span sized to its content rather than a raw interpolation.
	c.Code("SHA256", out.SHA256)
	c.URL(out.URL)
	c.End(
		"Use action 'publish_and_link' to also create a release asset link in one step",
		"Use action 'publish_directory' to batch-upload all files from a directory",
		"Use action 'list' to see all packages in this project",
	)
	return b.String()
}

// FormatDownloadMarkdown renders a downloaded package file as the card of one
// object.
func FormatDownloadMarkdown(out DownloadOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Package Downloaded")
	// The output path is the caller's own argument echoed back, before any
	// canonicalization.
	c.Field("Output Path", out.OutputPath)
	c.Field("Size", fmt.Sprintf("%d bytes", out.Size))
	c.Code("SHA256", out.SHA256)
	c.End(
		"Use action 'file_list' to see all files in this package",
		"Use action 'list' to browse other packages in the project",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of packages as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	if len(out.Packages) == 0 {
		return toolutil.EmptyMessage("packages")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Packages", len(out.Packages), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Version", "Type", "Status", "Creator", "Pipeline"))
	for _, p := range out.Packages {
		writePackageRow(&b, p, pipelineSummary(p))
	}
	// The pipeline column carries a link, so the footer keeps the instruction
	// to preserve it.
	toolutil.WriteListFooter(&b, out.Pagination, true,
		"Use action 'file_list' with a package_id to see individual files",
		"Use action 'delete' to remove a package",
		"Use action 'publish' or 'publish_directory' to upload new packages",
	)
	return b.String()
}

// writePackageRow writes one package table row, sharing the common
// ID/name/version/type/status/creator columns between the project and group
// package list renderers and appending the caller-supplied final column.
//
// The final column arrives rendered: it is a link for a pipeline, and the
// cell escaper turns a finished link back into text, so each summary escapes
// the GitLab-authored text it holds and this row writes the column as given.
func writePackageRow(b *strings.Builder, p ListItem, lastColumn string) {
	b.WriteString(toolutil.MarkdownTableRow(
		strconv.FormatInt(p.ID, 10),
		toolutil.EscapeMdTableCell(p.Name),
		toolutil.EscapeMdTableCell(versionSummary(p)),
		toolutil.EscapeMdTableCell(p.PackageType),
		toolutil.EscapeMdTableCell(p.Status),
		creatorSummary(p),
		lastColumn,
	))
}

// versionSummary names the package's version and how many others GitLab sent
// beside it, which it does when one package is asked for rather than a page.
func versionSummary(pkg ListItem) string {
	if len(pkg.Versions) == 0 {
		return pkg.Version
	}
	return fmt.Sprintf("%s (+%d)", pkg.Version, len(pkg.Versions))
}

// creatorSummary names the user who published the package, and nothing when
// GitLab attributes it to no one.
func creatorSummary(pkg ListItem) string {
	if pkg.CreatorID == 0 {
		return ""
	}
	return strconv.FormatInt(pkg.CreatorID, 10)
}

func pipelineSummary(pkg ListItem) string {
	if pkg.Pipeline != nil {
		return pipelineItemSummary(*pkg.Pipeline)
	}
	if len(pkg.Pipelines) > 0 {
		latest := pkg.Pipelines[0]
		if len(pkg.Pipelines) == 1 {
			return pipelineItemSummary(latest)
		}
		return fmt.Sprintf("%s (+%d)", pipelineItemSummary(latest), len(pkg.Pipelines)-1)
	}
	return ""
}

// pipelineItemSummary renders one pipeline as a cell: its id, status and ref,
// linked to its page when GitLab gave one. The ref is a branch or tag name, so
// the text is escaped here on the path with no link, and by the link helper on
// the other.
func pipelineItemSummary(pipeline PipelineItem) string {
	summary := strings.TrimSpace(fmt.Sprintf("%d %s %s", pipeline.ID, pipeline.Status, pipeline.Ref))
	if pipeline.WebURL == "" {
		return toolutil.EscapeMdTableCell(summary)
	}
	return toolutil.MdTitleLink(summary, pipeline.WebURL)
}

// FormatGroupListMarkdown renders a paginated list of group packages as a Markdown table.
func FormatGroupListMarkdown(out GroupListOutput) string {
	if len(out.Packages) == 0 {
		return toolutil.EmptyMessage("packages")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Packages", len(out.Packages), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Name", "Version", "Type", "Status", "Creator", "Project"))
	for _, p := range out.Packages {
		writePackageRow(&b, p.ListItem, groupProjectSummary(p))
	}
	// Unlike the project list, this table's last column is the owning project's
	// path rather than a pipeline link, so no column here carries one and the
	// footer drops the instruction to preserve them.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'list' to scope packages to a single project",
		"Use action 'file_list' with a package_id to see individual files",
		"Use action 'delete' to remove a package",
	)
	return b.String()
}

// groupProjectSummary renders the owning project for a group package
// row, preferring the human-readable project path over the numeric ID.
func groupProjectSummary(pkg GroupListItem) string {
	if pkg.ProjectPath != "" {
		return toolutil.EscapeMdTableCell(pkg.ProjectPath)
	}
	if pkg.ProjectID != 0 {
		return strconv.FormatInt(pkg.ProjectID, 10)
	}
	return ""
}

// FormatFileListMarkdown renders a paginated list of package files as a Markdown table.
func FormatFileListMarkdown(out FileListOutput) string {
	if len(out.Files) == 0 {
		return toolutil.EmptyMessage("package files")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Package Files", len(out.Files), out.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "File Name", "Size (bytes)", "SHA256"))
	for _, f := range out.Files {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(f.PackageFileID, 10),
			toolutil.EscapeMdTableCell(f.FileName),
			strconv.FormatInt(f.Size, 10),
			toolutil.MdCodeSpanCell(shortDigest(f.SHA256)),
		))
	}
	// The table carries no link, so the footer carries no instruction to keep
	// the links of a table that has none.
	toolutil.WriteListFooter(&b, out.Pagination, false,
		"Use action 'download' to retrieve a specific file",
		"Use action 'file_delete' to remove a single file",
	)
	return b.String()
}

// shortDigest is the leading characters of a SHA-256 digest, with an ellipsis
// when anything was cut. A digest is hexadecimal, so slicing it cannot split a
// rune.
func shortDigest(sha string) string {
	if len(sha) <= shortDigestLength {
		return sha
	}
	return sha[:shortDigestLength] + "..."
}

// FormatPublishAndLinkMarkdown renders a publish-and-link result as one card
// with a section per object: the package file and the release link it now has.
func FormatPublishAndLinkMarkdown(out PublishAndLinkOutput) string {
	var b strings.Builder
	c := toolutil.NewCard(&b, "Package Published & Linked")
	pkg := c.Section("Package")
	pkg.Int("Package File ID", out.Package.PackageFileID)
	pkg.Field("File Name", out.Package.FileName)
	pkg.Field("Size", fmt.Sprintf("%d bytes", out.Package.Size))
	pkg.URL(out.Package.URL)
	link := c.Section("Release Link")
	link.Int("ID", out.ReleaseLink.ID)
	link.Field("Name", out.ReleaseLink.Name)
	link.URL(out.ReleaseLink.URL)
	c.End(
		toolutil.HintAction("package.publish_directory", "batch-upload a directory instead of repeating this for each file"),
		toolutil.HintAction("release.get", "verify the release links"),
	)
	return b.String()
}

// FormatPublishDirMarkdown renders a directory publish result as the card of
// one object.
//
// The heading names the outcome rather than always claiming success: a run
// where every file failed used to be headed "Directory Published" with an
// errors section under it, which is the opposite of what happened. The count
// is published out of the two lists too, because TotalFiles is the number of
// files that succeeded and reads as the number attempted.
func FormatPublishDirMarkdown(out PublishDirOutput) string {
	published, failed := len(out.Published), len(out.Errors)
	var b strings.Builder
	c := toolutil.NewCard(&b, publishDirHeading(published, failed))
	c.Field("Published", fmt.Sprintf("%d of %d files", published, published+failed))
	c.Field("Total Bytes", fmt.Sprintf("%d bytes", out.TotalBytes))
	if published > 0 {
		files := c.Table("Published Files", "File", "Size (bytes)", "SHA256")
		for _, p := range out.Published {
			files.Row(
				toolutil.EscapeMdTableCell(p.FileName),
				strconv.FormatInt(p.Size, 10),
				toolutil.MdCodeSpanCell(shortDigest(p.SHA256)),
			)
		}
	}
	if failed > 0 {
		errs := c.Table(fmt.Sprintf("Errors (%d)", failed), "Error")
		for _, e := range out.Errors {
			// Each is a local directory entry name plus a wrapped error whose
			// text carries GitLab's own message.
			errs.Row(toolutil.EscapeMdTableCell(e))
		}
	}
	c.End(
		toolutil.HintAction("package.publish_and_link", "also create a release asset link for each file"),
		toolutil.HintAction("release.link_create_batch", "link these packages to a release"),
		toolutil.HintAction("package.list", "verify the uploaded packages"),
	)
	return b.String()
}

// publishDirHeading names what the run actually did.
func publishDirHeading(published, failed int) string {
	switch {
	case failed == 0:
		return "Directory Published"
	case published == 0:
		return "Directory Publish Failed"
	default:
		return "Directory Partially Published"
	}
}

func init() {
	toolutil.RegisterMarkdownResult(formatPackageNotFound)
	toolutil.RegisterMarkdown(FormatGetMarkdown)
	toolutil.RegisterMarkdown(FormatPublishMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGroupListMarkdown)
	toolutil.RegisterMarkdown(FormatFileListMarkdown)
	toolutil.RegisterMarkdown(FormatPublishAndLinkMarkdown)
	toolutil.RegisterMarkdown(FormatPublishDirMarkdown)
}
