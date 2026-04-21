// markdown.go provides Markdown formatters for all packages output types.
// Each formatter renders a human-friendly Markdown summary used both by
// individual tool handlers and the meta-tool dispatcher.

package packages

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const fmtSizeBytes = "- **Size**: %d bytes\n"

// FormatPublishMarkdown renders a published package file as Markdown.
func FormatPublishMarkdown(out PublishOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Published\n\n")
	fmt.Fprintf(&b, "- **Package File ID**: %d\n", out.PackageFileID)
	fmt.Fprintf(&b, "- **Package ID**: %d\n", out.PackageID)
	fmt.Fprintf(&b, "- **File Name**: %s\n", out.FileName)
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	if out.SHA256 != "" {
		fmt.Fprintf(&b, "- **SHA256**: %s\n", out.SHA256)
	}
	if out.URL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, out.URL)
	}
	toolutil.WriteHints(&b,
		"Use action 'publish_and_link' to also create a release asset link in one step",
		"Use action 'publish_directory' to batch-upload all files from a directory",
		"Use action 'list' to see all packages in this project",
	)
	return b.String()
}

// FormatDownloadMarkdown renders a downloaded package file as Markdown.
func FormatDownloadMarkdown(out DownloadOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Downloaded\n\n")
	fmt.Fprintf(&b, "- **Output Path**: %s\n", out.OutputPath)
	fmt.Fprintf(&b, fmtSizeBytes, out.Size)
	if out.SHA256 != "" {
		fmt.Fprintf(&b, "- **SHA256**: %s\n", out.SHA256)
	}
	toolutil.WriteHints(&b,
		"Use action 'file_list' to see all files in this package",
		"Use action 'list' to browse other packages in the project",
	)
	return b.String()
}

// FormatListMarkdown renders a paginated list of packages as a Markdown table.
func FormatListMarkdown(out ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Packages (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Packages), out.Pagination)
	if len(out.Packages) == 0 {
		b.WriteString("No packages found.\n")
		return b.String()
	}
	b.WriteString("| ID | Name | Version | Type | Status |\n")
	b.WriteString(toolutil.TblSep5Col)
	for _, p := range out.Packages {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			p.ID,
			toolutil.EscapeMdTableCell(p.Name),
			toolutil.EscapeMdTableCell(p.Version),
			p.PackageType,
			p.Status,
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'file_list' with a package_id to see individual files",
		"Use action 'delete' to remove a package",
		"Use action 'publish' or 'publish_directory' to upload new packages",
	)
	return b.String()
}

// FormatFileListMarkdown renders a paginated list of package files as a Markdown table.
func FormatFileListMarkdown(out FileListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Files (%d)\n\n", out.Pagination.TotalItems)
	toolutil.WriteListSummary(&b, len(out.Files), out.Pagination)
	if len(out.Files) == 0 {
		b.WriteString("No package files found.\n")
		return b.String()
	}
	b.WriteString("| ID | File Name | Size | SHA256 |\n")
	b.WriteString(toolutil.TblSep4Col)
	for _, f := range out.Files {
		sha := f.SHA256
		if len(sha) > 12 {
			sha = sha[:12] + "…"
		}
		fmt.Fprintf(&b, "| %d | %s | %d | %s |\n",
			f.PackageFileID,
			toolutil.EscapeMdTableCell(f.FileName),
			f.Size,
			sha,
		)
	}
	toolutil.WritePagination(&b, out.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'download' to retrieve a specific file",
		"Use action 'file_delete' to remove a single file",
	)
	return b.String()
}

// FormatPublishAndLinkMarkdown renders a publish-and-link result as Markdown.
func FormatPublishAndLinkMarkdown(out PublishAndLinkOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Package Published & Linked\n\n")
	fmt.Fprintf(&b, "### Package\n\n")
	fmt.Fprintf(&b, "- **Package File ID**: %d\n", out.Package.PackageFileID)
	fmt.Fprintf(&b, "- **File Name**: %s\n", out.Package.FileName)
	fmt.Fprintf(&b, fmtSizeBytes, out.Package.Size)
	if out.Package.URL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, out.Package.URL)
	}
	fmt.Fprintf(&b, "\n### Release Link\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, out.ReleaseLink.ID)
	fmt.Fprintf(&b, toolutil.FmtMdName, out.ReleaseLink.Name)
	if out.ReleaseLink.URL != "" {
		fmt.Fprintf(&b, toolutil.FmtMdURL, out.ReleaseLink.URL)
	}
	toolutil.WriteHints(&b,
		"Repeat for more files, or use 'publish_directory' to batch-upload a directory",
		"Use gitlab_release action 'get' to verify the release links",
	)
	return b.String()
}

// FormatPublishDirMarkdown renders a directory publish result as Markdown.
func FormatPublishDirMarkdown(out PublishDirOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Directory Published\n\n")
	fmt.Fprintf(&b, "- **Total Files**: %d\n", out.TotalFiles)
	fmt.Fprintf(&b, "- **Total Bytes**: %d\n", out.TotalBytes)
	if len(out.Published) > 0 {
		b.WriteString("\n| File | Size | SHA256 |\n")
		b.WriteString(toolutil.TblSep3Col)
		for _, p := range out.Published {
			sha := p.SHA256
			if len(sha) > 12 {
				sha = sha[:12] + "…"
			}
			fmt.Fprintf(&b, "| %s | %d | %s |\n",
				toolutil.EscapeMdTableCell(p.FileName),
				p.Size,
				sha,
			)
		}
	}
	if len(out.Errors) > 0 {
		fmt.Fprintf(&b, "\n### Errors (%d)\n\n", len(out.Errors))
		for _, e := range out.Errors {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	toolutil.WriteHints(&b,
		"Use 'publish_and_link' to also create release asset links for each file",
		"Use gitlab_release to create/manage releases and link these packages",
		"Use action 'list' to verify the uploaded packages",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatPublishMarkdown)
	toolutil.RegisterMarkdown(FormatDownloadMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatFileListMarkdown)
	toolutil.RegisterMarkdown(FormatPublishAndLinkMarkdown)
	toolutil.RegisterMarkdown(FormatPublishDirMarkdown)
}
