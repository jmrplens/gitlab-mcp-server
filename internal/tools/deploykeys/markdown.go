// markdown.go provides Markdown formatting functions for deploy key MCP tool output.
package deploykeys

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FormatOutputMarkdown formats a single deploy key.
func FormatOutputMarkdown(o Output) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Deploy Key: %s (ID: %d)\n\n", o.Title, o.ID)
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&b, "| Title | %s |\n", o.Title)
	if o.Fingerprint != "" {
		fmt.Fprintf(&b, "| Fingerprint | %s |\n", o.Fingerprint)
	}
	if o.FingerprintSHA256 != "" {
		fmt.Fprintf(&b, "| SHA256 | %s |\n", o.FingerprintSHA256)
	}
	fmt.Fprintf(&b, "| Can Push | %t |\n", o.CanPush)
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(o.CreatedAt))
	}
	if o.ExpiresAt != "" {
		fmt.Fprintf(&b, "| Expires | %s |\n", toolutil.FormatTime(o.ExpiresAt))
	}
	toolutil.WriteHints(&b,
		"Use action 'enable' to grant this key to another project",
		"Use action 'delete' to remove this deploy key",
	)
	return b.String()
}

// FormatListMarkdown formats a list of project deploy keys.
func FormatListMarkdown(o ListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Deploy Keys (%d)\n\n", len(o.DeployKeys))
	toolutil.WriteListSummary(&b, len(o.DeployKeys), o.Pagination)
	if len(o.DeployKeys) == 0 {
		b.WriteString("No deploy keys found.\n")
		toolutil.WritePagination(&b, o.Pagination)
		return b.String()
	}
	b.WriteString("| ID | Title | Can Push | Fingerprint | Created |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, k := range o.DeployKeys {
		fmt.Fprintf(&b, "| %d | %s | %t | %s | %s |\n",
			k.ID, k.Title, k.CanPush, k.Fingerprint, k.CreatedAt)
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'get' with key_id for full details",
		"Use action 'add' to create a new deploy key",
	)
	return b.String()
}

// FormatInstanceOutputMarkdown formats a single instance deploy key.
func FormatInstanceOutputMarkdown(o InstanceOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Instance Deploy Key: %s (ID: %d)\n\n", o.Title, o.ID)
	fmt.Fprintf(&b, "| Field | Value |\n|---|---|\n")
	fmt.Fprintf(&b, "| ID | %d |\n", o.ID)
	fmt.Fprintf(&b, "| Title | %s |\n", o.Title)
	if o.Fingerprint != "" {
		fmt.Fprintf(&b, "| Fingerprint | %s |\n", o.Fingerprint)
	}
	if o.FingerprintSHA256 != "" {
		fmt.Fprintf(&b, "| SHA256 | %s |\n", o.FingerprintSHA256)
	}
	if o.CreatedAt != "" {
		fmt.Fprintf(&b, "| Created | %s |\n", toolutil.FormatTime(o.CreatedAt))
	}
	if o.ExpiresAt != "" {
		fmt.Fprintf(&b, "| Expires | %s |\n", toolutil.FormatTime(o.ExpiresAt))
	}
	if len(o.ProjectsWithWriteAccess) > 0 {
		b.WriteString("\n### Projects with Write Access\n\n")
		b.WriteString("| ID | Name | Path |\n|---|---|---|\n")
		for _, p := range o.ProjectsWithWriteAccess {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", p.ID, p.Name, p.PathWithNamespace)
		}
	}
	if len(o.ProjectsWithReadonlyAccess) > 0 {
		b.WriteString("\n### Projects with Readonly Access\n\n")
		b.WriteString("| ID | Name | Path |\n|---|---|---|\n")
		for _, p := range o.ProjectsWithReadonlyAccess {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", p.ID, p.Name, p.PathWithNamespace)
		}
	}
	toolutil.WriteHints(&b,
		"Use action 'instance_delete' to remove this instance key",
	)
	return b.String()
}

// FormatInstanceListMarkdown formats a list of instance deploy keys.
func FormatInstanceListMarkdown(o InstanceListOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## Instance Deploy Keys (%d)\n\n", len(o.DeployKeys))
	toolutil.WriteListSummary(&b, len(o.DeployKeys), o.Pagination)
	if len(o.DeployKeys) == 0 {
		b.WriteString("No instance deploy keys found.\n")
		toolutil.WritePagination(&b, o.Pagination)
		return b.String()
	}
	b.WriteString("| ID | Title | Fingerprint | Created | Expires |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, k := range o.DeployKeys {
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n",
			k.ID, k.Title, k.Fingerprint, k.CreatedAt, k.ExpiresAt)
	}
	toolutil.WritePagination(&b, o.Pagination)
	toolutil.WriteHints(&b,
		"Use action 'instance_get' with key_id for full details",
	)
	return b.String()
}

func init() {
	toolutil.RegisterMarkdown(FormatOutputMarkdown)
	toolutil.RegisterMarkdown(FormatListMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceOutputMarkdown)
	toolutil.RegisterMarkdown(FormatInstanceListMarkdown)
}
