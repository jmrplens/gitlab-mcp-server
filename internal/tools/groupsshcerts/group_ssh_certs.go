// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package groupsshcerts

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput holds parameters for listing SSH certificates. It mirrors the
// offset and keyset pagination supported by the GitLab group SSH certificates
// list endpoint (page/per_page plus pagination/page_token for keyset traversal).
type ListInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	toolutil.PaginationInput
	toolutil.KeysetPaginationInput
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
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Key   string `json:"key"`
	// Fingerprint is the SHA256 fingerprint of the certificate authority's
	// public key, prefixed SHA256:, which GitLab sends on every certificate
	// from 19.5. client-go's GroupSSHCertificate does not model it, so it is
	// read off the captured response (ADR-0021), and an older instance leaves
	// it out.
	Fingerprint string `json:"fingerprint,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// ListOutput holds the list response: one page of a group's certificates and
// the pagination GitLab sent with it.
type ListOutput struct {
	toolutil.HintableOutput
	Certificates []Output                  `json:"certificates"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(c *gl.GroupSSHCertificate, extra toolutil.SSHCertificateExtra) Output {
	if c == nil {
		return Output{}
	}
	o := Output{
		ID:          c.ID,
		Title:       c.Title,
		Key:         c.Key,
		Fingerprint: extra.Fingerprint,
	}
	if c.CreatedAt != nil {
		o.CreatedAt = c.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	return o
}

// listQueryParameters renders the offset and keyset pagination parameters as a
// gl.RequestOptionFunc. The ListGroupSSHCertificates SDK method has no options
// struct, so pagination is applied via query parameters mirrored from a
// gl.ListOptions wired with toolutil.ApplyListOptions.
func listQueryParameters(in ListInput) gl.RequestOptionFunc {
	opts := &gl.ListOptions{}
	toolutil.ApplyListOptions(opts, in.PaginationInput, in.KeysetPaginationInput)

	q := url.Values{}
	if opts.Page > 0 {
		q.Set("page", strconv.FormatInt(opts.Page, 10))
	}
	if opts.PerPage > 0 {
		q.Set("per_page", strconv.FormatInt(opts.PerPage, 10))
	}
	if opts.Pagination != "" {
		q.Set("pagination", opts.Pagination)
	}
	if opts.PageToken != "" {
		q.Set("page_token", opts.PageToken)
	}
	return gl.WithKeysetPaginationParameters("?" + q.Encode())
}

// List returns one page of a group's SSH certificates.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.GroupID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	certs, resp, err := client.GL().GroupSSHCertificates.ListGroupSSHCertificates(
		in.GroupID.String(),
		gl.WithContext(ctx),
		listQueryParameters(in),
		toolutil.KeysetContinuation(in.KeysetPaginationInput),
	)
	if err != nil {
		return ListOutput{}, wrapSSHCertificateErr("list group SSH certificates", err, http.StatusNotFound, "verify group_id. Requires Owner role or admin access")
	}
	extras, err := toolutil.CapturedSSHCertificates(captured, len(certs))
	if err != nil {
		return ListOutput{}, toolutil.WrapErr("list group SSH certificates", err)
	}
	out := ListOutput{
		Certificates: make([]Output, 0, len(certs)),
		Pagination:   toolutil.PaginationFromResponse(resp),
	}
	for i, c := range certs {
		out.Certificates = append(out.Certificates, toOutput(c, extras[i]))
	}
	return out, nil
}

// subgroupRefusal is the message GitLab answers every SSH certificate route
// of a subgroup with, by forbidden!('Group') in the helper all three routes
// share (check_ssh_certificate_available_to_group, ee/lib/ee/api/groups.rb).
// It runs only once the caller passed the owner check, so a caller who is not
// an owner gets the plain "403 Forbidden" there instead, which a role
// explains.
const subgroupRefusal = "403 Forbidden - Group"

// topLevelOnlyHint is what a subgroup refusal is described with (issue
// 1333): the generic 403 hint sent an owner to check a role or scope that was
// never the cause.
const topLevelOnlyHint = "GitLab keeps SSH certificates only on top-level groups, and this group is a subgroup. Use group.get on it to read its full_path and pass the top-level group, the part before the first slash, as group_id"

// wrapSSHCertificateErr wraps err from an SSH certificate route: a subgroup
// refusal with topLevelOnlyHint, any other answer with hint when GitLab
// answered status, as [toolutil.WrapErrWithStatusHint] does. The message is
// read from the error's text rather than through
// [toolutil.ExtractGitLabMessage], which leaves out a message that opens
// with the status, as this one does.
func wrapSSHCertificateErr(op string, err error, status int, hint string) error {
	if toolutil.IsHTTPStatus(err, http.StatusForbidden) && toolutil.ContainsAny(err, subgroupRefusal) {
		return toolutil.WrapErrWithHint(op, err, topLevelOnlyHint)
	}
	return toolutil.WrapErrWithStatusHint(op, err, status, hint)
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
	ctx, captured := gitlabclient.WithResponseCapture(ctx)
	cert, _, err := client.GL().GroupSSHCertificates.CreateGroupSSHCertificate(in.GroupID.String(), opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapSSHCertificateErr("create group SSH certificate", err, http.StatusBadRequest, "verify the SSH certificate key is valid PEM format")
	}
	extra, err := toolutil.CapturedSSHCertificate(captured)
	if err != nil {
		return Output{}, toolutil.WrapErr("create group SSH certificate", err)
	}
	return toOutput(cert, extra), nil
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
	_, err := client.GL().GroupSSHCertificates.DeleteGroupSSHCertificate(in.GroupID.String(), in.CertificateID, gl.WithContext(ctx))
	if err != nil {
		return wrapSSHCertificateErr("delete group SSH certificate", err, http.StatusNotFound, "verify cert_id with group.ssh_cert_list")
	}
	return nil
}
