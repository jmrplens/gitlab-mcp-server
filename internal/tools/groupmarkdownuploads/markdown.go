// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupmarkdownuploads

import (
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/uploads"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

func init() {
	toolutil.RegisterMarkdownResult(UploadToolResult)
	toolutil.RegisterMarkdown(FormatListMarkdownString)
}

// UploadToolResult renders a group upload as the uploads package renders a
// project's, which carries the same keys: the card of the file, with an
// image embed for a person to look at when the file is an image whose
// address can be linked.
func UploadToolResult(u UploadOutput) *mcp.CallToolResult {
	return uploads.UploadToolResult(uploads.UploadOutput(u))
}

// FormatListMarkdownString renders a list of group markdown uploads.
func FormatListMarkdownString(o ListOutput) string {
	if len(o.Uploads) == 0 {
		return toolutil.EmptyMessage("group markdown uploads")
	}
	var b strings.Builder
	toolutil.WriteListHeading(&b, "Group Markdown Uploads", len(o.Uploads), o.Pagination)
	b.WriteString(toolutil.MarkdownTableHeader("ID", "Filename", "Size (bytes)", "Created", "Uploaded By"))
	for _, u := range o.Uploads {
		b.WriteString(toolutil.MarkdownTableRow(
			strconv.FormatInt(u.ID, 10),
			toolutil.EscapeMdTableCell(u.Filename),
			strconv.FormatInt(u.Size, 10),
			toolutil.FormatTime(u.CreatedAt),
			toolutil.EscapeMdTableCell(uploadedByLabel(u.UploadedBy)),
		))
	}
	toolutil.WriteListFooter(&b, o.Pagination, false,
		toolutil.HintAction(idGroupUploadDeleteByID, "remove one of these uploads"),
	)
	return b.String()
}
