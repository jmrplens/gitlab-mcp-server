// Package tags implements GitLab tag and protected tag operations including
// create, delete, get, list, signature, protect, and unprotect.
//
// The package also registers MCP tools and renders Markdown summaries for tag
// responses.
package tags

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput defines parameters for creating a Git tag.
type CreateInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Name of the tag,required"`
	Ref       string               `json:"ref"        jsonschema:"Commit SHA, branch name, or another tag to create the tag from,required"`
	Message   string               `json:"message,omitempty" jsonschema:"Creates an annotated tag with this message"`
}

// Output represents a Git tag.
type Output struct {
	toolutil.HintableOutput
	Name          string `json:"name"`
	Target        string `json:"target"`
	Message       string `json:"message"`
	Protected     bool   `json:"protected"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
}

// DeleteInput defines parameters for deleting a tag.
type DeleteInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Name of the tag to delete,required"`
}

// ListInput defines parameters for listing tags.
type ListInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	Search    string               `json:"search,omitempty" jsonschema:"Search query to filter tags by name"`
	OrderBy   string               `json:"order_by,omitempty" jsonschema:"Order tags by field (name, updated)"`
	Sort      string               `json:"sort,omitempty"     jsonschema:"Sort direction (asc, desc)"`
	toolutil.PaginationInput
}

// ListOutput holds a list of tags.
type ListOutput struct {
	toolutil.HintableOutput
	Tags       []Output                  `json:"tags"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// toOutput converts a GitLab API [gl.Tag] to the MCP tool output format.
func toOutput(t *gl.Tag) Output {
	out := Output{
		Name:      t.Name,
		Target:    t.Target,
		Message:   t.Message,
		Protected: t.Protected,
	}
	if t.Commit != nil {
		out.CommitSHA = t.Commit.ID
		out.CommitMessage = t.Commit.Message
	}
	if t.CreatedAt != nil {
		out.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// Create creates a new Git tag in the specified GitLab project.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("tagCreate: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.CreateTagOptions{
		TagName: new(input.TagName),
		Ref:     new(input.Ref),
	}
	if input.Message != "" {
		opts.Message = new(toolutil.NormalizeText(input.Message))
	}
	tag, _, err := client.GL().Tags.CreateTag(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		switch {
		case toolutil.ContainsAny(err, "already exists"):
			return Output{}, toolutil.WrapErrWithHint("tagCreate", err, "a tag with this name already exists — use gitlab_tag_get to view it")
		case toolutil.ContainsAny(err, "Target", "is invalid"):
			return Output{}, toolutil.WrapErrWithHint("tagCreate", err, "the ref does not exist — use gitlab_branch_list or gitlab_tag_list to verify")
		default:
			return Output{}, toolutil.WrapErrWithMessage("tagCreate", err)
		}
	}
	return toOutput(tag), nil
}

// Delete removes a tag from the specified GitLab project.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("tagDelete: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	_, err := client.GL().Tags.DeleteTag(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("tagDelete", err, http.StatusForbidden,
			"deleting tags requires Maintainer or Owner role; protected tags cannot be deleted without unprotecting first")
	}
	return nil
}

// List retrieves a paginated list of tags for the specified GitLab project.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, errors.New("tagList: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}
	opts := &gl.ListTagsOptions{}
	if input.Search != "" {
		opts.Search = new(input.Search)
	}
	if input.OrderBy != "" {
		opts.OrderBy = new(input.OrderBy)
	}
	if input.Sort != "" {
		opts.Sort = new(input.Sort)
	}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	tags, resp, err := client.GL().Tags.ListTags(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("tagList", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get")
	}
	out := make([]Output, len(tags))
	for i, t := range tags {
		out[i] = toOutput(t)
	}
	return ListOutput{Tags: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInput defines parameters for retrieving a single tag.
type GetInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name to retrieve,required"`
}

// Get retrieves a single tag by name from a GitLab project.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, errors.New("tagGet: project_id is required. Use gitlab_project_list to find the ID first, then pass it as project_id")
	}

	t, _, err := client.GL().Tags.GetTag(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("tagGet", err, http.StatusNotFound,
			"verify tag_name with gitlab_tag_list; tag names are case-sensitive")
	}
	return toOutput(t), nil
}

// ---------------------------------------------------------------------------
// GetTagSignature
// ---------------------------------------------------------------------------.

// SignatureInput defines parameters for retrieving the X.509 signature of a tag.
type SignatureInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Tag name to retrieve signature for,required"`
}

// X509IssuerOutput represents an X.509 certificate issuer.
type X509IssuerOutput struct {
	ID                   int64  `json:"id"`
	Subject              string `json:"subject"`
	SubjectKeyIdentifier string `json:"subject_key_identifier"`
	CrlURL               string `json:"crl_url,omitempty"`
}

// X509CertificateOutput represents an X.509 certificate.
type X509CertificateOutput struct {
	ID                   int64            `json:"id"`
	Subject              string           `json:"subject"`
	SubjectKeyIdentifier string           `json:"subject_key_identifier"`
	Email                string           `json:"email"`
	SerialNumber         string           `json:"serial_number"`
	CertificateStatus    string           `json:"certificate_status"`
	X509Issuer           X509IssuerOutput `json:"x509_issuer"`
}

// SignatureOutput represents the X.509 signature of a tag.
type SignatureOutput struct {
	toolutil.HintableOutput
	SignatureType      string                `json:"signature_type"`
	VerificationStatus string                `json:"verification_status"`
	X509Certificate    X509CertificateOutput `json:"x509_certificate"`
}

// GetSignature retrieves the X.509 signature of a tag.
func GetSignature(ctx context.Context, client *gitlabclient.Client, input SignatureInput) (SignatureOutput, error) {
	if err := ctx.Err(); err != nil {
		return SignatureOutput{}, err
	}
	if input.ProjectID == "" {
		return SignatureOutput{}, errors.New("tagGetSignature: project_id is required")
	}
	if input.TagName == "" {
		return SignatureOutput{}, errors.New("tagGetSignature: tag_name is required")
	}
	sig, _, err := client.GL().Tags.GetTagSignature(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return SignatureOutput{}, toolutil.WrapErrWithStatusHint("tagGetSignature", err, http.StatusNotFound,
			"the tag may not be signed, or verify tag_name with gitlab_tag_list")
	}
	out := SignatureOutput{
		SignatureType:      sig.SignatureType,
		VerificationStatus: sig.VerificationStatus,
	}
	out.X509Certificate = X509CertificateOutput{
		ID:                   sig.X509Certificate.ID,
		Subject:              sig.X509Certificate.Subject,
		SubjectKeyIdentifier: sig.X509Certificate.SubjectKeyIdentifier,
		Email:                sig.X509Certificate.Email,
		CertificateStatus:    sig.X509Certificate.CertificateStatus,
	}
	if sig.X509Certificate.SerialNumber != nil {
		out.X509Certificate.SerialNumber = sig.X509Certificate.SerialNumber.String()
	}
	out.X509Certificate.X509Issuer = X509IssuerOutput{
		ID:                   sig.X509Certificate.X509Issuer.ID,
		Subject:              sig.X509Certificate.X509Issuer.Subject,
		SubjectKeyIdentifier: sig.X509Certificate.X509Issuer.SubjectKeyIdentifier,
		CrlURL:               sig.X509Certificate.X509Issuer.CrlURL,
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Protected Tags
// ---------------------------------------------------------------------------.

// TagAccessLevelOutput represents access level description for a protected tag.
type TagAccessLevelOutput struct {
	ID                     int64  `json:"id"`
	AccessLevel            int    `json:"access_level"`
	AccessLevelDescription string `json:"access_level_description"`
	UserID                 int64  `json:"user_id,omitempty"`
	GroupID                int64  `json:"group_id,omitempty"`
	DeployKeyID            int64  `json:"deploy_key_id,omitempty"`
}

// ProtectedTagOutput represents a protected tag.
type ProtectedTagOutput struct {
	toolutil.HintableOutput
	Name               string                 `json:"name"`
	CreateAccessLevels []TagAccessLevelOutput `json:"create_access_levels"`
}

// ListProtectedTagsInput defines parameters for listing protected tags.
type ListProtectedTagsInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	toolutil.PaginationInput
}

// ListProtectedTagsOutput holds a list of protected tags.
type ListProtectedTagsOutput struct {
	toolutil.HintableOutput
	Tags       []ProtectedTagOutput      `json:"tags"`
	Pagination toolutil.PaginationOutput `json:"pagination"`
}

// GetProtectedTagInput defines parameters for retrieving a single protected tag.
type GetProtectedTagInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Name of the protected tag,required"`
}

// ProtectTagInput defines parameters for protecting a tag.
type ProtectTagInput struct {
	ProjectID         toolutil.StringOrInt `json:"project_id"          jsonschema:"Project ID or URL-encoded path,required"`
	TagName           string               `json:"tag_name"             jsonschema:"Tag name or wildcard pattern (e.g. 'v*'),required"`
	CreateAccessLevel int                  `json:"create_access_level,omitempty" jsonschema:"Access level allowed to create (0=No access, 30=Developer, 40=Maintainer)"`
	AllowedToCreate   []TagPermission      `json:"allowed_to_create,omitempty"  jsonschema:"Granular create permissions (user_id, group_id, deploy_key_id, access_level)"`
}

// TagPermission represents a granular permission option for protected tags.
type TagPermission struct {
	UserID      int64 `json:"user_id,omitempty"      jsonschema:"User ID allowed to create"`
	GroupID     int64 `json:"group_id,omitempty"      jsonschema:"Group ID allowed to create"`
	DeployKeyID int64 `json:"deploy_key_id,omitempty" jsonschema:"Deploy key ID allowed to create"`
	AccessLevel int   `json:"access_level,omitempty"  jsonschema:"Access level (0, 30, 40)"`
}

// UnprotectTagInput defines parameters for unprotecting a tag.
type UnprotectTagInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	TagName   string               `json:"tag_name"   jsonschema:"Name of the protected tag to unprotect,required"`
}

// protectedTagOutputFromGL converts a GitLab [gl.ProtectedTag] to our output type.
func protectedTagOutputFromGL(pt *gl.ProtectedTag) ProtectedTagOutput {
	out := ProtectedTagOutput{Name: pt.Name}
	for _, al := range pt.CreateAccessLevels {
		out.CreateAccessLevels = append(out.CreateAccessLevels, TagAccessLevelOutput{
			ID:                     al.ID,
			AccessLevel:            int(al.AccessLevel),
			AccessLevelDescription: al.AccessLevelDescription,
			UserID:                 al.UserID,
			GroupID:                al.GroupID,
			DeployKeyID:            al.DeployKeyID,
		})
	}
	return out
}

// ListProtectedTags retrieves a paginated list of protected tags for a project.
func ListProtectedTags(ctx context.Context, client *gitlabclient.Client, input ListProtectedTagsInput) (ListProtectedTagsOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListProtectedTagsOutput{}, err
	}
	if input.ProjectID == "" {
		return ListProtectedTagsOutput{}, errors.New("tagListProtected: project_id is required")
	}
	opts := &gl.ListProtectedTagsOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	tags, resp, err := client.GL().ProtectedTags.ListProtectedTags(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListProtectedTagsOutput{}, toolutil.WrapErrWithStatusHint("tagListProtected", err, http.StatusNotFound,
			"verify project_id with gitlab_project_get; protected tags require Maintainer or Owner role to view")
	}
	out := make([]ProtectedTagOutput, len(tags))
	for i, pt := range tags {
		out[i] = protectedTagOutputFromGL(pt)
	}
	return ListProtectedTagsOutput{Tags: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetProtectedTag retrieves a single protected tag by name.
func GetProtectedTag(ctx context.Context, client *gitlabclient.Client, input GetProtectedTagInput) (ProtectedTagOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedTagOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedTagOutput{}, errors.New("tagGetProtected: project_id is required")
	}
	if input.TagName == "" {
		return ProtectedTagOutput{}, errors.New("tagGetProtected: tag_name is required")
	}
	pt, _, err := client.GL().ProtectedTags.GetProtectedTag(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return ProtectedTagOutput{}, toolutil.WrapErrWithStatusHint("tagGetProtected", err, http.StatusNotFound,
			"the tag may not be protected \u2014 use gitlab_tag_list_protected to verify")
	}
	return protectedTagOutputFromGL(pt), nil
}

// ProtectTag protects a repository tag or wildcard pattern.
// buildTagPermissions converts permission inputs into GitLab API permission options.
func buildTagPermissions(allowed []TagPermission) *[]*gl.TagsPermissionOptions {
	perms := make([]*gl.TagsPermissionOptions, len(allowed))
	for i, p := range allowed {
		perm := &gl.TagsPermissionOptions{}
		if p.UserID > 0 {
			perm.UserID = new(p.UserID)
		}
		if p.GroupID > 0 {
			perm.GroupID = new(p.GroupID)
		}
		if p.DeployKeyID > 0 {
			perm.DeployKeyID = new(p.DeployKeyID)
		}
		if p.AccessLevel > 0 {
			perm.AccessLevel = new(gl.AccessLevelValue(p.AccessLevel))
		}
		perms[i] = perm
	}
	return &perms
}

// ProtectTag protects tag for the tags package.
func ProtectTag(ctx context.Context, client *gitlabclient.Client, input ProtectTagInput) (ProtectedTagOutput, error) {
	if err := ctx.Err(); err != nil {
		return ProtectedTagOutput{}, err
	}
	if input.ProjectID == "" {
		return ProtectedTagOutput{}, errors.New("tagProtect: project_id is required")
	}
	if input.TagName == "" {
		return ProtectedTagOutput{}, errors.New("tagProtect: tag_name is required")
	}
	opts := &gl.ProtectRepositoryTagsOptions{
		Name: new(input.TagName),
	}
	if input.CreateAccessLevel > 0 {
		opts.CreateAccessLevel = new(gl.AccessLevelValue(input.CreateAccessLevel))
	}
	if len(input.AllowedToCreate) > 0 {
		opts.AllowedToCreate = buildTagPermissions(input.AllowedToCreate)
	}
	pt, _, err := client.GL().ProtectedTags.ProtectRepositoryTags(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusConflict) {
			return ProtectedTagOutput{}, toolutil.WrapErrWithHint("tagProtect", err, "a protected tag rule for this name already exists")
		}
		return ProtectedTagOutput{}, toolutil.WrapErrWithStatusHint("tagProtect", err, http.StatusForbidden,
			"protecting tags requires Maintainer or Owner role; create_access_level must be one of 0 (no access), 30 (Developer), 40 (Maintainer)")
	}
	return protectedTagOutputFromGL(pt), nil
}

// UnprotectTag removes protection from a repository tag.
func UnprotectTag(ctx context.Context, client *gitlabclient.Client, input UnprotectTagInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.ProjectID == "" {
		return errors.New("tagUnprotect: project_id is required")
	}
	if input.TagName == "" {
		return errors.New("tagUnprotect: tag_name is required")
	}
	_, err := client.GL().ProtectedTags.UnprotectRepositoryTags(string(input.ProjectID), input.TagName, gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("tagUnprotect", err, http.StatusForbidden,
			"unprotecting tags requires Maintainer or Owner role; the tag may not be protected \u2014 use gitlab_tag_list_protected to verify")
	}
	return nil
}

// formatIDCell renders an integer ID as a readable string for markdown tables.
// Zero values are shown as "-" to avoid misleading numeric output.
func formatIDCell(id int64) string {
	if id == 0 {
		return "-"
	}
	return strconv.FormatInt(id, 10)
}

// formatAccessLevelSummary renders a single access level entry as a human-readable
// string. It appends user/group/deploy-key context when the IDs are non-zero.
func formatAccessLevelSummary(al TagAccessLevelOutput) string {
	desc := al.AccessLevelDescription
	switch {
	case al.UserID != 0:
		return fmt.Sprintf("%s (User #%d)", desc, al.UserID)
	case al.GroupID != 0:
		return fmt.Sprintf("%s (Group #%d)", desc, al.GroupID)
	case al.DeployKeyID != 0:
		return fmt.Sprintf("%s (Deploy Key #%d)", desc, al.DeployKeyID)
	default:
		return desc
	}
}
