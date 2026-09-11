package packages

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// shortDigestLength is how much of a SHA-256 digest a table column shows
// before the ellipsis; a digest of exactly this length is left whole, so the
// ellipsis always means something was cut.
const shortDigestLength = 12

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
		"Repeat for more files, or use 'publish_directory' to batch-upload a directory",
		"Use gitlab_release action 'get' to verify the release links",
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
		"Use 'publish_and_link' to also create release asset links for each file",
		"Use gitlab_release to create/manage releases and link these packages",
		"Use action 'list' to verify the uploaded packages",
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
	toolutil.RegisterMarkdown(FormatPublishMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatGroupListMarkdown)
	toolutil.RegisterMarkdown(FormatFileListMarkdown)
	toolutil.RegisterMarkdown(FormatPublishAndLinkMarkdown)
	toolutil.RegisterMarkdown(FormatPublishDirMarkdown)
}
