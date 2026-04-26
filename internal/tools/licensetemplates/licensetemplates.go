// Package licensetemplates implements MCP tools for GitLab License Templates API.
package licensetemplates

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput is the input for listing license templates.
type ListInput struct {
	Popular *bool `json:"popular,omitempty" jsonschema:"Filter by popular licenses"`
	Page    int64 `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int64 `json:"per_page,omitempty" jsonschema:"Items per page"`
}

// LicenseItem represents a license template.
type LicenseItem struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Nickname    string   `json:"nickname,omitempty"`
	Featured    bool     `json:"featured"`
	HTMLURL     string   `json:"html_url,omitempty"`
	SourceURL   string   `json:"source_url,omitempty"`
	Description string   `json:"description,omitempty"`
	Conditions  []string `json:"conditions,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
	Content     string   `json:"content,omitempty"`
}

// ListOutput is the output for listing license templates.
type ListOutput struct {
	toolutil.HintableOutput
	Licenses   []LicenseItem             `json:"licenses"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// List lists all available license templates.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	opts := &gl.ListLicenseTemplatesOptions{
		ListOptions: gl.ListOptions{Page: input.Page, PerPage: input.PerPage},
	}
	if input.Popular != nil {
		opts.Popular = input.Popular
	}
	items, resp, err := client.GL().LicenseTemplates.ListLicenseTemplates(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list_license_templates", err, http.StatusForbidden, "verify your token has read_api scope")
	}
	licenses := make([]LicenseItem, 0, len(items))
	for _, l := range items {
		licenses = append(licenses, licenseFromGL(l))
	}
	return ListOutput{
		Licenses:   licenses,
		Pagination: toolutil.PaginationFromResponse(resp),
	}, nil
}

// GetInput is the input for getting a license template.
type GetInput struct {
	Key      string  `json:"key" jsonschema:"License template key (e.g. mit, apache-2.0),required"`
	Project  *string `json:"project,omitempty" jsonschema:"Project name to replace in license"`
	Fullname *string `json:"fullname,omitempty" jsonschema:"Full name to replace in license"`
}

// GetOutput is the output for getting a license template.
type GetOutput struct {
	toolutil.HintableOutput
	LicenseItem
}

// Get gets a single license template by key.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.Key == "" {
		return GetOutput{}, errors.New("get_license_template: key is required. Use list action to see available template keys")
	}
	opts := &gl.GetLicenseTemplateOptions{}
	if input.Project != nil {
		opts.Project = input.Project
	}
	if input.Fullname != nil {
		opts.Fullname = input.Fullname
	}
	l, _, err := client.GL().LicenseTemplates.GetLicenseTemplate(input.Key, opts, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithStatusHint("get_license_template", err, http.StatusNotFound, "verify key with gitlab_list_license_templates")
	}
	return GetOutput{LicenseItem: licenseFromGL(l)}, nil
}

// licenseFromGL is an internal helper for the licensetemplates package.
func licenseFromGL(l *gl.LicenseTemplate) LicenseItem {
	return LicenseItem{
		Key:         l.Key,
		Name:        l.Name,
		Nickname:    l.Nickname,
		Featured:    l.Featured,
		HTMLURL:     l.HTMLURL,
		SourceURL:   l.SourceURL,
		Description: l.Description,
		Conditions:  l.Conditions,
		Permissions: l.Permissions,
		Limitations: l.Limitations,
		Content:     l.Content,
	}
}
