package toolutil

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// AssetSourceOutput mirrors gl.ReleaseAssetsSource (the per-source
// object inside release.assets.sources). Format is a short string
// (e.g. "package", "image", "url", "other").
type AssetSourceOutput struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

// AssetLinkOutput mirrors gl.ReleaseLink (the per-link object inside
// release.assets.links). The DirectAssetURL field is omitempty because
// the official assets-by-link endpoint only populates it for package-link
// types.
type AssetLinkOutput struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url,omitempty"`
	External       bool   `json:"external"`
	LinkType       string `json:"link_type"`
}

// AssetsOutput mirrors gl.ReleaseAssets (the count / sources / links /
// evidence_file_path object).
type AssetsOutput struct {
	Count            int64               `json:"count"`
	Sources          []AssetSourceOutput `json:"sources,omitempty"`
	Links            []AssetLinkOutput   `json:"links,omitempty"`
	EvidenceFilePath string              `json:"evidence_file_path,omitempty"`
}

// NewAssetsOutput converts a gl.ReleaseAssets into the canonical-key
// assets object, returning nil when the SDK value has every field zero.
func NewAssetsOutput(a gl.ReleaseAssets) *AssetsOutput {
	out := &AssetsOutput{
		Count:            a.Count,
		EvidenceFilePath: a.EvidenceFilePath,
	}
	if len(a.Sources) > 0 {
		out.Sources = make([]AssetSourceOutput, 0, len(a.Sources))
		for _, s := range a.Sources {
			out.Sources = append(out.Sources, AssetSourceOutput{Format: s.Format, URL: s.URL})
		}
	}
	if len(a.Links) > 0 {
		out.Links = make([]AssetLinkOutput, 0, len(a.Links))
		for _, l := range a.Links {
			if l == nil {
				continue
			}
			out.Links = append(out.Links, AssetLinkOutput{
				ID: l.ID, Name: l.Name, URL: l.URL,
				DirectAssetURL: l.DirectAssetURL, External: l.External,
				LinkType: string(l.LinkType),
			})
		}
	}
	if out.Count == 0 && len(out.Sources) == 0 && len(out.Links) == 0 && out.EvidenceFilePath == "" {
		return nil
	}
	return out
}

// LinksOutput mirrors gl.ReleaseLinks (the _links object on a release).
// Per the documented release-by-tag response GitLab returns URL strings
// for self, edit, opened_issues, opened_merge_requests, merged_merge_requests,
// and closed_merge_requests. The JSON tags follow the SDK's snake_case
// pluralisation (opened_issues_url, etc.) which differs from the older
// opened_issues / merged_issues / closed_issues names used in some docs.
type LinksOutput struct {
	Self                   string `json:"self,omitempty"`
	EditURL                string `json:"edit_url,omitempty"`
	OpenedIssuesURL        string `json:"opened_issues_url,omitempty"`
	OpenedMergeRequestsURL string `json:"opened_merge_requests_url,omitempty"`
	MergedMergeRequestsURL string `json:"merged_merge_requests_url,omitempty"`
	ClosedIssuesURL        string `json:"closed_issues_url,omitempty"`
	ClosedMergeRequestsURL string `json:"closed_merge_requests_url,omitempty"`
}

// NewLinksOutput converts a gl.ReleaseLinks into the canonical-key links
// object, returning nil when the SDK value has every URL field empty.
func NewLinksOutput(l gl.ReleaseLinks) *LinksOutput {
	out := &LinksOutput{
		Self:                   l.Self,
		EditURL:                l.EditURL,
		OpenedIssuesURL:        l.OpenedIssues,
		OpenedMergeRequestsURL: l.OpenedMergeRequest,
		MergedMergeRequestsURL: l.MergedMergeRequest,
		ClosedIssuesURL:        l.ClosedIssueURL,
		ClosedMergeRequestsURL: l.ClosedMergeRequest,
	}
	if out.Self == "" && out.EditURL == "" && out.OpenedIssuesURL == "" &&
		out.OpenedMergeRequestsURL == "" && out.MergedMergeRequestsURL == "" &&
		out.ClosedIssuesURL == "" && out.ClosedMergeRequestsURL == "" {
		return nil
	}
	return out
}

// MilestoneIssueStatsOutput mirrors gl.ReleaseMilestoneIssueStats
// (the total / closed counts object surfaced under each associated
// milestone on a release).
type MilestoneIssueStatsOutput struct {
	Total  int64 `json:"total,omitempty"`
	Closed int64 `json:"closed,omitempty"`
}

// NewMilestoneIssueStatsOutput converts a gl.ReleaseMilestoneIssueStats
// into the canonical-key stats object.
func NewMilestoneIssueStatsOutput(s *gl.ReleaseMilestoneIssueStats) *MilestoneIssueStatsOutput {
	if s == nil {
		return nil
	}
	return &MilestoneIssueStatsOutput{Total: s.Total, Closed: s.Closed}
}

// MilestoneOutput mirrors gl.ReleaseMilestone (the milestone object
// surfaced on the release's milestones array). Timestamps are surfaced
// as RFC 3339 strings via toolutil.FormatTimePtr / FormatISOTimePtr.
type MilestoneOutput struct {
	ID          int64                      `json:"id,omitempty"`
	IID         int64                      `json:"iid,omitempty"`
	ProjectID   int64                      `json:"project_id,omitempty"`
	Title       string                     `json:"title"`
	Description string                     `json:"description,omitempty"`
	State       string                     `json:"state,omitempty"`
	WebURL      string                     `json:"web_url,omitempty"`
	CreatedAt   string                     `json:"created_at,omitempty"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
	StartDate   string                     `json:"start_date,omitempty"`
	DueDate     string                     `json:"due_date,omitempty"`
	IssueStats  *MilestoneIssueStatsOutput `json:"issue_stats,omitempty"`
}

// NewMilestoneOutputs converts a slice of gl.ReleaseMilestone, skipping
// nil elements and returning nil for an empty input. Timestamps are
// formatted as RFC 3339 (or YYYY-MM-DD for ISOTime).
func NewMilestoneOutputs(ms []*gl.ReleaseMilestone) []*MilestoneOutput {
	if len(ms) == 0 {
		return nil
	}
	out := make([]*MilestoneOutput, 0, len(ms))
	for _, m := range ms {
		if m == nil {
			continue
		}
		out = append(out, &MilestoneOutput{
			ID:          m.ID,
			IID:         m.IID,
			ProjectID:   m.ProjectID,
			Title:       m.Title,
			Description: m.Description,
			State:       m.State,
			WebURL:      m.WebURL,
			CreatedAt:   FormatTimePtr(m.CreatedAt),
			UpdatedAt:   FormatTimePtr(m.UpdatedAt),
			StartDate:   FormatISOTimePtr(m.StartDate),
			DueDate:     FormatISOTimePtr(m.DueDate),
			IssueStats:  NewMilestoneIssueStatsOutput(m.IssueStats),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// EvidenceOutput mirrors gl.ReleaseEvidence (the per-evidence object
// surfaced on the release's evidences array).
type EvidenceOutput struct {
	SHA         string `json:"sha,omitempty"`
	Filepath    string `json:"filepath,omitempty"`
	CollectedAt string `json:"collected_at,omitempty"`
}

// NewEvidenceOutputs converts a slice of gl.ReleaseEvidence, skipping
// nil elements and returning nil for an empty input.
func NewEvidenceOutputs(evs []*gl.ReleaseEvidence) []*EvidenceOutput {
	if len(evs) == 0 {
		return nil
	}
	out := make([]*EvidenceOutput, 0, len(evs))
	for _, e := range evs {
		if e == nil {
			continue
		}
		out = append(out, &EvidenceOutput{
			SHA:         e.SHA,
			Filepath:    e.Filepath,
			CollectedAt: timeToRFC3339(e.CollectedAt),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// timeToRFC3339 formats a *time.Time as RFC 3339, returning "" for nil.
func timeToRFC3339(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// AuthorOutput mirrors gl.BasicUser for the release author (per the
// documented release-by-tag response; the author object is the compact
// BasicUser shape, not the full User shape).
type AuthorOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state,omitempty"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// NewAuthorOutputFromBasicUser converts a gl.BasicUser into the
// canonical-key author object.
func NewAuthorOutputFromBasicUser(u gl.BasicUser) AuthorOutput {
	return AuthorOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// CommitOutput is the documented reference subset of the commit object
// surfaced on a release (per doc/api/releases/_index.md). The documented
// release commit JSON surfaces id, short_id, title, created_at, parent_ids,
// message, author_name, author_email, authored_date, committer_name,
// committer_email, committed_date. SDK-only commit fields (stats, status,
// project_id, web_url) are not part of the documented project-release commit
// subset and are intentionally omitted from this base type. The group-releases
// endpoint adds web_url; consumers that need it embed this type and add the
// extra field.
type CommitOutput struct {
	ID             string   `json:"id"`
	ShortID        string   `json:"short_id"`
	Title          string   `json:"title"`
	AuthorName     string   `json:"author_name"`
	AuthorEmail    string   `json:"author_email"`
	AuthoredDate   string   `json:"authored_date,omitempty"`
	CommitterName  string   `json:"committer_name"`
	CommitterEmail string   `json:"committer_email"`
	CommittedDate  string   `json:"committed_date,omitempty"`
	CreatedAt      string   `json:"created_at,omitempty"`
	Message        string   `json:"message"`
	ParentIDs      []string `json:"parent_ids,omitempty"`
}

// NewCommitOutput converts a gl.Commit into the canonical-key commit object,
// returning nil when the commit ID is empty (no commit associated with the
// release). Timestamps are formatted as RFC 3339 strings via FormatTimePtr.
func NewCommitOutput(c gl.Commit) *CommitOutput {
	if c.ID == "" {
		return nil
	}
	return &CommitOutput{
		ID: c.ID, ShortID: c.ShortID, Title: c.Title,
		AuthorName: c.AuthorName, AuthorEmail: c.AuthorEmail,
		AuthoredDate:   FormatTimePtr(c.AuthoredDate),
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedDate:  FormatTimePtr(c.CommittedDate),
		CreatedAt:      FormatTimePtr(c.CreatedAt),
		Message:        c.Message,
		ParentIDs:      c.ParentIDs,
	}
}
