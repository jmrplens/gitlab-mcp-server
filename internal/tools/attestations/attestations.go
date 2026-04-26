// Package attestations implements GitLab build attestation operations for projects
// including list and download.
package attestations

import (
	"context"
	"encoding/base64"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInput holds parameters for listing attestations.
type ListInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	SubjectDigest string               `json:"subject_digest"  jsonschema:"Subject digest (hash) to filter attestations,required"`
}

// DownloadInput holds parameters for downloading a single attestation.
type DownloadInput struct {
	ProjectID      toolutil.StringOrInt `json:"project_id"       jsonschema:"Project ID or URL-encoded path,required"`
	AttestationIID int64                `json:"attestation_iid"  jsonschema:"Attestation IID (project-scoped),required"`
}

// Output represents a single attestation.
type Output struct {
	ID            int64  `json:"id"`
	IID           int64  `json:"iid"`
	ProjectID     int64  `json:"project_id"`
	BuildID       int64  `json:"build_id"`
	Status        string `json:"status"`
	PredicateKind string `json:"predicate_kind,omitempty"`
	PredicateType string `json:"predicate_type,omitempty"`
	SubjectDigest string `json:"subject_digest,omitempty"`
	DownloadURL   string `json:"download_url,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	ExpireAt      string `json:"expire_at,omitempty"`
}

// ListOutput holds the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Attestations []Output `json:"attestations"`
}

// DownloadOutput holds the downloaded attestation content.
type DownloadOutput struct {
	toolutil.HintableOutput
	AttestationIID int64  `json:"attestation_iid"`
	Size           int    `json:"size"`
	ContentBase64  string `json:"content_base64"`
}

func toOutput(a *gl.Attestation) Output {
	if a == nil {
		return Output{}
	}
	o := Output{
		ID:            a.ID,
		IID:           a.IID,
		ProjectID:     a.ProjectID,
		BuildID:       a.BuildID,
		Status:        a.Status,
		PredicateKind: a.PredicateKind,
		PredicateType: a.PredicateType,
		SubjectDigest: a.SubjectDigest,
		DownloadURL:   a.DownloadURL,
	}
	if a.CreatedAt != nil {
		o.CreatedAt = a.CreatedAt.Format(time.RFC3339)
	}
	if a.UpdatedAt != nil {
		o.UpdatedAt = a.UpdatedAt.Format(time.RFC3339)
	}
	if a.ExpireAt != nil {
		o.ExpireAt = a.ExpireAt.Format(time.RFC3339)
	}
	return o
}

// List returns all attestations for a project matching a subject digest.
func List(ctx context.Context, client *gitlabclient.Client, in ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.SubjectDigest == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("subject_digest")
	}
	atts, _, err := client.GL().Attestations.ListAttestations(in.ProjectID.String(), in.SubjectDigest, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list attestations", err, http.StatusNotFound, "verify project_id with gitlab_project_get \u2014 attestations require Ultimate license")
	}
	out := ListOutput{Attestations: make([]Output, 0, len(atts))}
	for _, a := range atts {
		out.Attestations = append(out.Attestations, toOutput(a))
	}
	return out, nil
}

// Download retrieves the binary content of an attestation.
func Download(ctx context.Context, client *gitlabclient.Client, in DownloadInput) (DownloadOutput, error) {
	if err := ctx.Err(); err != nil {
		return DownloadOutput{}, err
	}
	if in.ProjectID.String() == "" {
		return DownloadOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	if in.AttestationIID == 0 {
		return DownloadOutput{}, toolutil.ErrFieldRequired("attestation_iid")
	}
	data, _, err := client.GL().Attestations.DownloadAttestation(in.ProjectID.String(), in.AttestationIID, gl.WithContext(ctx))
	if err != nil {
		return DownloadOutput{}, toolutil.WrapErrWithStatusHint("download attestation", err, http.StatusNotFound, "verify attestation_iid and project_id are valid — use gitlab_list_project_attestations to find valid IIDs")
	}
	return DownloadOutput{
		AttestationIID: in.AttestationIID,
		Size:           len(data),
		ContentBase64:  base64.StdEncoding.EncodeToString(data),
	}, nil
}
