// Package groupsshcerts implements GitLab SSH certificate operations for groups
// including list, create, and delete.
package groupsshcerts

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds parameters for listing SSH certificates.
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// CreateInput holds parameters for creating an SSH certificate.
type CreateInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	Key     string               `json:"key"      jsonschema:"SSH public key content,required"`
	Title   string               `json:"title"    jsonschema:"Title for the SSH certificate,required"`
}

// DeleteInput holds parameters for deleting an SSH certificate.
type DeleteInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"       jsonschema:"Group ID or URL-encoded path,required"`
	CertificateID int64                `json:"certificate_id" jsonschema:"SSH certificate ID,required"`
}

// Output represents an SSH certificate.
type Output struct {
	toolutil.HintableOutput
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Key       string `json:"key"`
	CreatedAt string `json:"created_at,omitempty"`
}

// ListOutput holds the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Certificates []Output `json:"certificates"`
}

func toOutput(c *gl.GroupSSHCertificate) Output {
	if c == nil {
		return Output{}
	}
	o := Output{
		ID:    c.ID,
		Title: c.Title,
		Key:   c.Key,
	}
	if c.CreatedAt != nil {
		o.CreatedAt = c.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	return o
}

// List returns all SSH certificates for a group.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	certs, _, err := client.GL().GroupSSHCertificates.ListGroupSSHCertificates(in.GroupID.String())
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list group SSH certificates", err)
	}
	out := ListOutput{Certificates: make([]Output, 0, len(certs))}
	for _, c := range certs {
		out.Certificates = append(out.Certificates, toOutput(c))
	}
	return out, nil
}

// Create adds a new SSH certificate to a group.
func Create(ctx context.Context, client *gitlabclient.Client, in CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if in.GroupID.String() == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if in.Key == "" {
		return Output{}, toolutil.ErrFieldRequired("key")
	}
	if in.Title == "" {
		return Output{}, toolutil.ErrFieldRequired("title")
	}
	opts := &gl.CreateGroupSSHCertificateOptions{
		Key:   new(in.Key),
		Title: new(in.Title),
	}
	cert, _, err := client.GL().GroupSSHCertificates.CreateGroupSSHCertificate(in.GroupID.String(), opts)
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create group SSH certificate", err)
	}
	return toOutput(cert), nil
}

// Delete removes an SSH certificate from a group.
func Delete(ctx context.Context, client *gitlabclient.Client, in DeleteInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if in.GroupID.String() == "" {
		return toolutil.ErrFieldRequired("group_id")
	}
	if in.CertificateID == 0 {
		return toolutil.ErrFieldRequired("certificate_id")
	}
	_, err := client.GL().GroupSSHCertificates.DeleteGroupSSHCertificate(in.GroupID.String(), in.CertificateID)
	if err != nil {
		return toolutil.WrapErrWithMessage("delete group SSH certificate", err)
	}
	return nil
}
