// Package pages implements MCP tool handlers for GitLab Pages and Pages Domains
// management. Covers PagesService (get, update, unpublish) and PagesDomainsService
// (list all, list project, get, create, update, delete).
package pages

import (
	"context"
	"fmt"
	"strings"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ---------------------------------------------------------------------------
// Input types
// ---------------------------------------------------------------------------.

// GetPagesInput defines parameters for getting Pages settings.
type GetPagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// UpdatePagesInput defines parameters for updating Pages settings.
type UpdatePagesInput struct {
	ProjectID                toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	PagesUniqueDomainEnabled *bool                `json:"pages_unique_domain_enabled,omitempty" jsonschema:"Enable unique domain for Pages"`
	PagesHTTPSOnly           *bool                `json:"pages_https_only,omitempty" jsonschema:"Enforce HTTPS for Pages"`
	PagesPrimaryDomain       string               `json:"pages_primary_domain,omitempty" jsonschema:"Primary domain for Pages"`
}

// UnpublishPagesInput defines parameters for unpublishing Pages.
type UnpublishPagesInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
}

// ListAllDomainsInput defines parameters for listing all Pages domains globally.
type ListAllDomainsInput struct{}

// ListDomainsInput defines parameters for listing Pages domains for a project.
type ListDomainsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// GetDomainInput defines parameters for getting a single Pages domain.
type GetDomainInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Domain    string               `json:"domain" jsonschema:"The Pages domain name,required"`
}

// CreateDomainInput defines parameters for creating a Pages domain.
type CreateDomainInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Domain         string               `json:"domain" jsonschema:"Custom domain name (e.g. example.com),required"`
	AutoSslEnabled *bool                `json:"auto_ssl_enabled,omitempty" jsonschema:"Enable automatic SSL certificate provisioning"`
	Certificate    string               `json:"certificate,omitempty" jsonschema:"PEM-encoded SSL certificate"`
	Key            string               `json:"key,omitempty" jsonschema:"PEM-encoded private key for the certificate"`
}

// UpdateDomainInput defines parameters for updating a Pages domain.
type UpdateDomainInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Domain         string               `json:"domain" jsonschema:"The Pages domain name to update,required"`
	AutoSslEnabled *bool                `json:"auto_ssl_enabled,omitempty" jsonschema:"Enable automatic SSL certificate provisioning"`
	Certificate    string               `json:"certificate,omitempty" jsonschema:"PEM-encoded SSL certificate"`
	Key            string               `json:"key,omitempty" jsonschema:"PEM-encoded private key for the certificate"`
}

// DeleteDomainInput defines parameters for deleting a Pages domain.
type DeleteDomainInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Domain    string               `json:"domain" jsonschema:"The Pages domain name to delete,required"`
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------.

// DeploymentOutput represents a Pages deployment.
type DeploymentOutput struct {
	CreatedAt     string `json:"created_at"`
	URL           string `json:"url"`
	PathPrefix    string `json:"path_prefix"`
	RootDirectory string `json:"root_directory"`
}

// Output represents Pages settings for a project.
type Output struct {
	toolutil.HintableOutput
	URL                   string             `json:"url"`
	IsUniqueDomainEnabled bool               `json:"is_unique_domain_enabled"`
	ForceHTTPS            bool               `json:"force_https"`
	Deployments           []DeploymentOutput `json:"deployments,omitempty"`
	PrimaryDomain         string             `json:"primary_domain"`
}

// CertificateOutput represents a Pages domain certificate.
type CertificateOutput struct {
	Subject         string `json:"subject"`
	Expired         bool   `json:"expired"`
	Expiration      string `json:"expiration,omitempty"`
	Certificate     string `json:"certificate,omitempty"`
	CertificateText string `json:"certificate_text,omitempty"`
}

// DomainOutput represents a Pages domain.
type DomainOutput struct {
	toolutil.HintableOutput
	Domain           string            `json:"domain"`
	AutoSslEnabled   bool              `json:"auto_ssl_enabled"`
	URL              string            `json:"url"`
	ProjectID        int64             `json:"project_id"`
	ProjectPath      string            `json:"project_path,omitempty"`
	Verified         bool              `json:"verified"`
	VerificationCode string            `json:"verification_code"`
	EnabledUntil     string            `json:"enabled_until,omitempty"`
	Certificate      CertificateOutput `json:"certificate"`
}

// ListDomainsOutput wraps a list of Pages domains with pagination.
type ListDomainsOutput struct {
	toolutil.HintableOutput
	Domains    []DomainOutput            `json:"domains"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// ListAllDomainsOutput wraps a list of all Pages domains.
type ListAllDomainsOutput struct {
	toolutil.HintableOutput
	Domains []DomainOutput `json:"domains"`
}

// ---------------------------------------------------------------------------
// Handlers — PagesService
// ---------------------------------------------------------------------------.

// GetPages retrieves Pages settings for a project.
func GetPages(ctx context.Context, client *gitlabclient.Client, input GetPagesInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}

	pages, _, err := client.GL().Pages.GetPages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("gitlab_pages_get", err)
	}

	return toPagesOutput(pages), nil
}

// UpdatePages updates Pages settings for a project.
func UpdatePages(ctx context.Context, client *gitlabclient.Client, input UpdatePagesInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := gl.UpdatePagesOptions{}
	if input.PagesUniqueDomainEnabled != nil {
		opts.PagesUniqueDomainEnabled = input.PagesUniqueDomainEnabled
	}
	if input.PagesHTTPSOnly != nil {
		opts.PagesHTTPSOnly = input.PagesHTTPSOnly
	}
	if input.PagesPrimaryDomain != "" {
		opts.PagesPrimaryDomain = new(input.PagesPrimaryDomain)
	}

	pages, _, err := client.GL().Pages.UpdatePages(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("gitlab_pages_update", err)
	}

	return toPagesOutput(pages), nil
}

// UnpublishPages unpublishes Pages for a project.
func UnpublishPages(ctx context.Context, client *gitlabclient.Client, input UnpublishPagesInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}

	_, err := client.GL().Pages.UnpublishPages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_pages_unpublish", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Handlers — PagesDomainsService
// ---------------------------------------------------------------------------.

// ListAllDomains returns all Pages domains across all projects.
func ListAllDomains(ctx context.Context, client *gitlabclient.Client, _ ListAllDomainsInput) (ListAllDomainsOutput, error) {
	domains, _, err := client.GL().PagesDomains.ListAllPagesDomains(gl.WithContext(ctx))
	if err != nil {
		return ListAllDomainsOutput{}, toolutil.WrapErrWithMessage("gitlab_pages_domain_list_all", err)
	}

	out := ListAllDomainsOutput{Domains: make([]DomainOutput, 0, len(domains))}
	for _, d := range domains {
		out.Domains = append(out.Domains, toDomainOutput(d))
	}

	return out, nil
}

// ListDomains returns Pages domains for a specific project.
func ListDomains(ctx context.Context, client *gitlabclient.Client, input ListDomainsInput) (ListDomainsOutput, error) {
	if input.ProjectID == "" {
		return ListDomainsOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := &gl.ListPagesDomainsOptions{
		ListOptions: gl.ListOptions{
			Page:    int64(input.Page),
			PerPage: int64(input.PerPage),
		},
	}

	domains, resp, err := client.GL().PagesDomains.ListPagesDomains(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListDomainsOutput{}, toolutil.WrapErrWithMessage("gitlab_pages_domain_list", err)
	}

	out := ListDomainsOutput{
		Domains:    make([]DomainOutput, 0, len(domains)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, d := range domains {
		do := toDomainOutput(d)
		setProjectPathFromInput(&do, input.ProjectID)
		out.Domains = append(out.Domains, do)
	}

	return out, nil
}

// GetDomain retrieves a single Pages domain.
func GetDomain(ctx context.Context, client *gitlabclient.Client, input GetDomainInput) (DomainOutput, error) {
	if input.ProjectID == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("domain")
	}

	domain, _, err := client.GL().PagesDomains.GetPagesDomain(string(input.ProjectID), input.Domain, gl.WithContext(ctx))
	if err != nil {
		return DomainOutput{}, toolutil.WrapErrWithMessage("gitlab_pages_domain_get", err)
	}

	out := toDomainOutput(domain)
	setProjectPathFromInput(&out, input.ProjectID)
	return out, nil
}

// CreateDomain creates a new Pages domain for a project.
func CreateDomain(ctx context.Context, client *gitlabclient.Client, input CreateDomainInput) (DomainOutput, error) {
	if input.ProjectID == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("domain")
	}

	opts := &gl.CreatePagesDomainOptions{
		Domain: new(input.Domain),
	}
	if input.AutoSslEnabled != nil {
		opts.AutoSslEnabled = input.AutoSslEnabled
	}
	if input.Certificate != "" {
		opts.Certificate = new(input.Certificate)
	}
	if input.Key != "" {
		opts.Key = new(input.Key)
	}

	domain, _, err := client.GL().PagesDomains.CreatePagesDomain(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return DomainOutput{}, toolutil.WrapErrWithMessage("gitlab_pages_domain_create", err)
	}

	out := toDomainOutput(domain)
	setProjectPathFromInput(&out, input.ProjectID)
	return out, nil
}

// UpdateDomain updates an existing Pages domain.
func UpdateDomain(ctx context.Context, client *gitlabclient.Client, input UpdateDomainInput) (DomainOutput, error) {
	if input.ProjectID == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("domain")
	}

	opts := &gl.UpdatePagesDomainOptions{}
	if input.AutoSslEnabled != nil {
		opts.AutoSslEnabled = input.AutoSslEnabled
	}
	if input.Certificate != "" {
		opts.Certificate = new(input.Certificate)
	}
	if input.Key != "" {
		opts.Key = new(input.Key)
	}

	domain, _, err := client.GL().PagesDomains.UpdatePagesDomain(string(input.ProjectID), input.Domain, opts, gl.WithContext(ctx))
	if err != nil {
		return DomainOutput{}, toolutil.WrapErrWithMessage("gitlab_pages_domain_update", err)
	}

	out := toDomainOutput(domain)
	setProjectPathFromInput(&out, input.ProjectID)
	return out, nil
}

// DeleteDomain deletes a Pages domain.
func DeleteDomain(ctx context.Context, client *gitlabclient.Client, input DeleteDomainInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return toolutil.ErrFieldRequired("domain")
	}

	_, err := client.GL().PagesDomains.DeletePagesDomain(string(input.ProjectID), input.Domain, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithMessage("gitlab_pages_domain_delete", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toPagesOutput converts the GitLab API response to the tool output format.
func toPagesOutput(p *gl.Pages) Output {
	if p == nil {
		return Output{}
	}
	out := Output{
		URL:                   p.URL,
		IsUniqueDomainEnabled: p.IsUniqueDomainEnabled,
		ForceHTTPS:            p.ForceHTTPS,
		PrimaryDomain:         p.PrimaryDomain,
	}
	for _, d := range p.Deployments {
		out.Deployments = append(out.Deployments, DeploymentOutput{
			CreatedAt:     d.CreatedAt.Format(toolutil.DateTimeFormat),
			URL:           d.URL,
			PathPrefix:    d.PathPrefix,
			RootDirectory: d.RootDirectory,
		})
	}
	return out
}

// toDomainOutput converts the GitLab API response to the tool output format.
func toDomainOutput(d *gl.PagesDomain) DomainOutput {
	if d == nil {
		return DomainOutput{}
	}
	out := DomainOutput{
		Domain:           d.Domain,
		AutoSslEnabled:   d.AutoSslEnabled,
		URL:              d.URL,
		ProjectID:        d.ProjectID,
		Verified:         d.Verified,
		VerificationCode: d.VerificationCode,
		Certificate: CertificateOutput{
			Subject:         d.Certificate.Subject,
			Expired:         d.Certificate.Expired,
			Certificate:     d.Certificate.Certificate,
			CertificateText: d.Certificate.CertificateText,
		},
	}
	if d.EnabledUntil != nil {
		out.EnabledUntil = d.EnabledUntil.Format(toolutil.DateTimeFormat)
	}
	if d.Certificate.Expiration != nil {
		out.Certificate.Expiration = d.Certificate.Expiration.Format(toolutil.DateTimeFormat)
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------.

// projectDisplay returns a human-readable project identifier, preferring
// the full path (e.g. "group/project") over a numeric ID.
func projectDisplay(path string, id int64) string {
	if path != "" {
		return path
	}
	return fmt.Sprintf("#%d", id)
}

// setProjectPathFromInput copies the caller-supplied project identifier into
// out.ProjectPath when it looks like a path (contains '/').
func setProjectPathFromInput(out *DomainOutput, input toolutil.StringOrInt) {
	if strings.Contains(string(input), "/") {
		out.ProjectPath = string(input)
	}
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
