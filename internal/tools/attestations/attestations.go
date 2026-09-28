package attestations

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// ListInput holds parameters for listing attestations, and the page of them
// to list.
//
// GitLab pages the list: the route presents paginate(attestations), which
// reads page and per_page from the request although the route declares
// neither. client-go's ListAttestations takes no options struct, so the page
// travels as a request option.
type ListInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"      jsonschema:"Project ID or URL-encoded path,required"`
	SubjectDigest string               `json:"subject_digest"  jsonschema:"Hex-encoded SHA-256 digest of the attested artifact: its 64 hex characters, in either case. An OCI-style sha256: prefix is accepted and removed and the hex is lowered before the request,required"`
	toolutil.PaginationInput
}

// FeatureFlag is the GitLab feature flag both attestation routes are served
// behind. The routes' before block answers 404 for every project the flag is
// off for (lib/api/supply_chain/attestations.rb), and the flag is a
// gitlab_com_derisk flag that ships disabled (default_enabled: false), so on a
// self-managed instance where nobody enabled it every request here is a 404
// whatever the digest or IID.
const FeatureFlag = "slsa_provenance_statement"

// featureOffHint is the corrective hint for a 404 that only the feature flag
// explains: the project reads back and the digest is in the form the route
// matches, so the route itself was refused. It spells the flag rather than
// concatenating [FeatureFlag], since an operator in a constant initializer is
// a mutant no test can reach; the tests hold the two to each other.
const featureOffHint = "the project exists and the digest is well formed, so GitLab refused the attestations API itself: it serves the API only where the slsa_provenance_statement feature flag is enabled for the project, and the flag ships disabled. Ask an administrator to enable it, for instance with admin.feature_set; until then no attestation of this project can be listed or downloaded"

// sha256Prefix is the algorithm prefix an OCI digest carries and GitLab's
// route does not accept. OCI spells the algorithm in lower case.
const sha256Prefix = "sha256:"

// sha256HexLength is the length of a SHA-256 digest written in hex.
const sha256HexLength = 64

// hexDigits are the characters of a digest in the case GitLab stores it.
const hexDigits = "0123456789abcdef"

// subjectDigest returns the digest in the one form GitLab's route accepts and
// its lookup finds: the 64 hex characters of the artifact's SHA-256 hash, in
// lower case, with no algorithm prefix.
//
// The route declares /[A-Fa-f0-9]{64}/ as a requirement
// (lib/api/supply_chain/attestations.rb), so a digest in any other form,
// sha256:<hex> included, matches no route and GitLab answers 404. The OCI
// prefix is removed rather than refused, since it is the form a registry and a
// model both reach for first. The hex is brought to lower case because the
// route admits upper case and the lookup behind it does not: it is an exact
// where(subject_digest:) on a case-sensitive text column, and every digest
// GitLab stores is lower case (Digest::SHA256#hexdigest for an artifact, the
// OCI digest of an image), so an upper-case digest reaches the route and
// answers an empty list for an artifact that has attestations. Anything that
// is still not 64 hex characters is refused with the form named.
func subjectDigest(raw string) (string, error) {
	digest, _ := strings.CutPrefix(strings.TrimSpace(raw), sha256Prefix)
	digest = strings.ToLower(digest)
	if len(digest) != sha256HexLength || strings.Trim(digest, hexDigits) != "" {
		return "", fmt.Errorf("subject_digest %q is not a SHA-256 digest: GitLab takes the 64 hex characters of the artifact's hash, with or without a sha256: prefix", raw)
	}
	return digest, nil
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

// ListOutput holds one page of the list response.
type ListOutput struct {
	toolutil.HintableOutput
	Attestations []Output                  `json:"attestations"`
	Pagination   toolutil.PaginationOutput `json:"pagination"`
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

// List returns one page of the attestations for a project matching a subject
// digest.
//
// A 404 is never an empty list. With the digest brought to the form the route
// matches, a digest nothing was attested under answers 200 and an empty
// array, so a 404 means the project could not be read or the feature flag the
// routes are served behind is off for it. The project is read back to tell the
// two apart, and the second is reported as what it is: an empty list there
// would tell a model that an attested artifact has no attestations on every
// instance that has not enabled the flag, which is every self-managed one by
// default.
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
	digest, err := subjectDigest(in.SubjectDigest)
	if err != nil {
		return ListOutput{}, err
	}
	atts, resp, err := client.GL().Attestations.ListAttestations(in.ProjectID.String(), digest, gl.WithContext(ctx), toolutil.PaginationRequestOption(in.PaginationInput))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			if _, _, projectErr := client.GL().Projects.GetProject(in.ProjectID.String(), nil, gl.WithContext(ctx)); projectErr == nil {
				return ListOutput{}, toolutil.WrapErrWithHint("list attestations", err, featureOffHint)
			}
		}
		return ListOutput{}, toolutil.WrapErrWithStatusHint("list attestations", err, http.StatusNotFound, "verify project_id with project.get. Attestations require Ultimate license")
	}
	out := ListOutput{Attestations: make([]Output, 0, len(atts)), Pagination: toolutil.PaginationFromResponse(resp)}
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
		return DownloadOutput{}, toolutil.WrapErrWithStatusHint("download attestation", err, http.StatusNotFound,
			"verify attestation_iid and project_id are valid; use attestation.list to find valid IIDs. GitLab also answers 404 for every attestation of a project the "+
				FeatureFlag+" feature flag is off for, which is the default, and attestation.list says so when that is the cause")
	}
	return DownloadOutput{
		AttestationIID: in.AttestationIID,
		Size:           len(data),
		ContentBase64:  base64.StdEncoding.EncodeToString(data),
	}, nil
}
