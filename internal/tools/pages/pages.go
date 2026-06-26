package pages

import (
	"context"
	"fmt"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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
// It supports offset and keyset pagination (page_token/pagination) plus
// order_by/sort to mirror the keyset-capable [gl.ListPagesDomainsOptions].
type ListDomainsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Column to order keyset-paginated results by"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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

// GetPages retrieves the Pages settings (URL, unique domain, force
// HTTPS, deployments) for a project via the GitLab Pages API
// (GET /projects/:id/pages).
func GetPages(ctx context.Context, client *gitlabclient.Client, input GetPagesInput) (Output, error) {
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}

	pages, _, err := client.GL().Pages.GetPages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_pages_get", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; Pages may not be configured for this project")
	}

	return toPagesOutput(pages), nil
}

// UpdatePages updates the Pages settings for a project via the GitLab
// Pages API (PUT /projects/:id/pages). Only non-nil fields are
// applied; pages_primary_domain must reference a domain previously
// added via [CreateDomain].
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
		return Output{}, toolutil.WrapErrWithStatusHint("gitlab_pages_update", err, http.StatusForbidden,
			"updating Pages settings requires Maintainer role; pages_primary_domain must be a domain previously added via gitlab_pages_domain_create")
	}

	return toPagesOutput(pages), nil
}

// UnpublishPages removes the published Pages site for a project via
// the GitLab Pages API (DELETE /projects/:id/pages). Requires
// Maintainer or Owner role.
func UnpublishPages(ctx context.Context, client *gitlabclient.Client, input UnpublishPagesInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}

	_, err := client.GL().Pages.UnpublishPages(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_pages_unpublish", err, http.StatusForbidden,
			"unpublishing Pages requires Maintainer role; verify project_id")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Handlers — PagesDomainsService
// ---------------------------------------------------------------------------.

// ListAllDomains returns every Pages domain across the entire
// instance via the GitLab Pages domains admin API
// (GET /pages/domains). Requires administrator access.
func ListAllDomains(ctx context.Context, client *gitlabclient.Client, _ ListAllDomainsInput) (ListAllDomainsOutput, error) {
	domains, _, err := client.GL().PagesDomains.ListAllPagesDomains(gl.WithContext(ctx))
	if err != nil {
		return ListAllDomainsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_pages_domain_list_all", err, http.StatusForbidden,
			"listing all Pages domains requires admin token")
	}

	out := ListAllDomainsOutput{Domains: make([]DomainOutput, 0, len(domains))}
	for _, d := range domains {
		out.Domains = append(out.Domains, toDomainOutput(d))
	}

	return out, nil
}

// ListDomains returns the custom Pages domains configured for a
// project via the GitLab Pages domains API
// (GET /projects/:id/pages/domains). Supports pagination.
func ListDomains(ctx context.Context, client *gitlabclient.Client, input ListDomainsInput) (ListDomainsOutput, error) {
	if input.ProjectID == "" {
		return ListDomainsOutput{}, toolutil.ErrFieldRequired("project_id")
	}

	opts := &gl.ListPagesDomainsOptions{}
	toolutil.ApplyListOptions(&opts.ListOptions, input.PaginationInput, input.KeysetPaginationInput)
	if input.OrderBy != "" {
		opts.OrderBy = input.OrderBy
	}
	if input.Sort != "" {
		opts.Sort = input.Sort
	}

	domains, resp, err := client.GL().PagesDomains.ListPagesDomains(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListDomainsOutput{}, toolutil.WrapErrWithStatusHint("gitlab_pages_domain_list", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; the project may have no Pages domains configured")
	}

	out := ListDomainsOutput{
		Domains:    make([]DomainOutput, 0, len(domains)),
		Pagination: toolutil.PaginationFromResponse(resp),
	}
	for _, d := range domains {
		out.Domains = append(out.Domains, toDomainOutput(d))
	}

	return out, nil
}

// GetDomain retrieves a single Pages domain by name via the GitLab
// Pages domains API (GET /projects/:id/pages/domains/:domain).
func GetDomain(ctx context.Context, client *gitlabclient.Client, input GetDomainInput) (DomainOutput, error) {
	if input.ProjectID == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return DomainOutput{}, toolutil.ErrFieldRequired("domain")
	}

	domain, _, err := client.GL().PagesDomains.GetPagesDomain(string(input.ProjectID), input.Domain, gl.WithContext(ctx))
	if err != nil {
		return DomainOutput{}, toolutil.WrapErrWithStatusHint("gitlab_pages_domain_get", err, http.StatusNotFound,
			"verify domain with gitlab_pages_domain_list; the domain may have been removed")
	}

	return toDomainOutput(domain), nil
}

// CreateDomain adds a new custom domain to a project's Pages
// configuration via the GitLab Pages domains API
// (POST /projects/:id/pages/domains). Optional AutoSslEnabled,
// Certificate, and Key fields are forwarded to the API.
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
		return DomainOutput{}, toolutil.WrapErrWithStatusHint("gitlab_pages_domain_create", err, http.StatusBadRequest,
			"domain must be a valid FQDN and not in use by another project; certificate and key must be PEM-encoded matching pair when provided; auto_ssl_enabled requires DNS A/AAAA record pointing to GitLab Pages; requires Maintainer role")
	}

	return toDomainOutput(domain), nil
}

// UpdateDomain updates the auto-SSL flag and/or custom certificate
// of an existing Pages domain via the GitLab Pages domains API
// (PUT /projects/:id/pages/domains/:domain).
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
		return DomainOutput{}, toolutil.WrapErrWithStatusHint("gitlab_pages_domain_update", err, http.StatusBadRequest,
			"certificate and key must be PEM-encoded matching pair when provided; cannot set both auto_ssl_enabled and a custom certificate; requires Maintainer role")
	}

	return toDomainOutput(domain), nil
}

// DeleteDomain removes a custom Pages domain from a project via the
// GitLab Pages domains API
// (DELETE /projects/:id/pages/domains/:domain). Requires Maintainer
// or Owner role.
func DeleteDomain(ctx context.Context, client *gitlabclient.Client, input DeleteDomainInput) error {
	if input.ProjectID == "" {
		return toolutil.ErrFieldRequired("project_id")
	}
	if input.Domain == "" {
		return toolutil.ErrFieldRequired("domain")
	}

	_, err := client.GL().PagesDomains.DeletePagesDomain(string(input.ProjectID), input.Domain, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_pages_domain_delete", err, http.StatusForbidden,
			"deleting Pages domains requires Maintainer role; verify domain with gitlab_pages_domain_list")
	}

	return nil
}

// ---------------------------------------------------------------------------
// Converters
// ---------------------------------------------------------------------------.

// toPagesOutput converts a [gl.Pages] response into the package's
// [Output], formatting each deployment timestamp with
// [toolutil.DateTimeFormat].
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

// toDomainOutput converts a [gl.PagesDomain] response into the
// package's [DomainOutput], formatting the optional EnabledUntil
// and certificate expiration timestamps with
// [toolutil.DateTimeFormat].
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

// projectDisplay returns a human-readable project identifier from the
// numeric project ID returned by the GitLab Pages domains API. Used by
// the Markdown formatters.
func projectDisplay(id int64) string {
	return fmt.Sprintf("#%d", id)
}

// ---------------------------------------------------------------------------
// Markdown formatters
// ---------------------------------------------------------------------------.
