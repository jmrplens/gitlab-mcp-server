package groupreleases

import (
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages
// (notably internal/tools/releases) to preserve the zero-import-cycle
// constraint (C-IMPORTS).
//
// This file covers the release sub-objects surfaced on the canonical json keys
// (author, commit, assets, _links, milestones, evidences) returned by the group
// releases endpoint, which returns the same gl.Release type as project releases.

// formatTimePtr renders an optional timestamp as RFC 3339, or "" when nil.
func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

// formatISOTimePtr renders an optional ISO date (gl.ISOTime) as YYYY-MM-DD,
// or "" when nil.
func formatISOTimePtr(t *gl.ISOTime) string {
	if t == nil {
		return ""
	}
	return time.Time(*t).Format("2006-01-02")
}

// AuthorOutput is the documented reference subset of the release author object
// per doc/api/group_releases.md (the documented JSON shows id, name, username,
// state, avatar_url, web_url; created_at is not part of the release author
// subset and is intentionally omitted).
type AuthorOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

func authorOutput(u gl.BasicUser) *AuthorOutput {
	return &AuthorOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// CommitOutput is the documented reference subset of the commit the release tag
// points to per doc/api/group_releases.md. The documented group-release commit
// JSON surfaces id, short_id, created_at, parent_ids, title, message,
// author_name, author_email, authored_date, committer_name, committer_email,
// committed_date, web_url. SDK-only commit fields (stats, status, project_id)
// are not part of the documented group-release commit subset and are
// intentionally omitted.
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
	WebURL         string   `json:"web_url,omitempty"`
}

func commitOutput(c gl.Commit) *CommitOutput {
	if c.ID == "" {
		return nil
	}
	return &CommitOutput{
		ID: c.ID, ShortID: c.ShortID, Title: c.Title,
		AuthorName: c.AuthorName, AuthorEmail: c.AuthorEmail,
		AuthoredDate:   formatTimePtr(c.AuthoredDate),
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedDate:  formatTimePtr(c.CommittedDate),
		CreatedAt:      formatTimePtr(c.CreatedAt),
		Message:        c.Message,
		ParentIDs:      c.ParentIDs,
		WebURL:         c.WebURL,
	}
}

// AssetSourceOutput mirrors gl.ReleaseAssetsSource (an auto-generated archive).
type AssetSourceOutput struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

// AssetLinkOutput mirrors gl.ReleaseLink (a release asset link).
type AssetLinkOutput struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	DirectAssetURL string `json:"direct_asset_url,omitempty"`
	External       bool   `json:"external"`
	LinkType       string `json:"link_type"`
}

// AssetsOutput mirrors gl.ReleaseAssets (the assets object on a release).
type AssetsOutput struct {
	Count            int64               `json:"count"`
	Sources          []AssetSourceOutput `json:"sources,omitempty"`
	Links            []AssetLinkOutput   `json:"links,omitempty"`
	EvidenceFilePath string              `json:"evidence_file_path,omitempty"`
}

func assetsOutput(a gl.ReleaseAssets) *AssetsOutput {
	out := &AssetsOutput{Count: a.Count, EvidenceFilePath: a.EvidenceFilePath}
	if len(a.Sources) > 0 {
		sources := make([]AssetSourceOutput, len(a.Sources))
		for i, s := range a.Sources {
			sources[i] = AssetSourceOutput{Format: s.Format, URL: s.URL}
		}
		out.Sources = sources
	}
	if len(a.Links) > 0 {
		links := make([]AssetLinkOutput, 0, len(a.Links))
		for _, l := range a.Links {
			if l == nil {
				continue
			}
			links = append(links, AssetLinkOutput{
				ID: l.ID, Name: l.Name, URL: l.URL,
				DirectAssetURL: l.DirectAssetURL, External: l.External,
				LinkType: string(l.LinkType),
			})
		}
		out.Links = links
	}
	return out
}

// LinksOutput mirrors gl.ReleaseLinks (the release _links object).
type LinksOutput struct {
	ClosedIssuesURL        string `json:"closed_issues_url,omitempty"`
	ClosedMergeRequestsURL string `json:"closed_merge_requests_url,omitempty"`
	EditURL                string `json:"edit_url,omitempty"`
	MergedMergeRequestsURL string `json:"merged_merge_requests_url,omitempty"`
	OpenedIssuesURL        string `json:"opened_issues_url,omitempty"`
	OpenedMergeRequestsURL string `json:"opened_merge_requests_url,omitempty"`
	Self                   string `json:"self,omitempty"`
}

func linksOutput(l gl.ReleaseLinks) *LinksOutput {
	out := &LinksOutput{
		ClosedIssuesURL:        l.ClosedIssueURL,
		ClosedMergeRequestsURL: l.ClosedMergeRequest,
		EditURL:                l.EditURL,
		MergedMergeRequestsURL: l.MergedMergeRequest,
		OpenedIssuesURL:        l.OpenedIssues,
		OpenedMergeRequestsURL: l.OpenedMergeRequest,
		Self:                   l.Self,
	}
	if *out == (LinksOutput{}) {
		return nil
	}
	return out
}

// MilestoneIssueStatsOutput mirrors gl.ReleaseMilestoneIssueStats.
type MilestoneIssueStatsOutput struct {
	Total  int64 `json:"total"`
	Closed int64 `json:"closed"`
}

// MilestoneOutput mirrors gl.ReleaseMilestone (a milestone associated with a
// release).
type MilestoneOutput struct {
	ID          int64                      `json:"id"`
	IID         int64                      `json:"iid"`
	ProjectID   int64                      `json:"project_id"`
	Title       string                     `json:"title"`
	Description string                     `json:"description"`
	State       string                     `json:"state"`
	CreatedAt   string                     `json:"created_at,omitempty"`
	UpdatedAt   string                     `json:"updated_at,omitempty"`
	DueDate     string                     `json:"due_date,omitempty"`
	StartDate   string                     `json:"start_date,omitempty"`
	WebURL      string                     `json:"web_url"`
	IssueStats  *MilestoneIssueStatsOutput `json:"issue_stats,omitempty"`
}

func milestoneOutputs(ms []*gl.ReleaseMilestone) []*MilestoneOutput {
	if len(ms) == 0 {
		return nil
	}
	out := make([]*MilestoneOutput, 0, len(ms))
	for _, m := range ms {
		if m == nil {
			continue
		}
		mo := &MilestoneOutput{
			ID: m.ID, IID: m.IID, ProjectID: m.ProjectID,
			Title: m.Title, Description: m.Description, State: m.State,
			CreatedAt: formatTimePtr(m.CreatedAt), UpdatedAt: formatTimePtr(m.UpdatedAt),
			DueDate: formatISOTimePtr(m.DueDate), StartDate: formatISOTimePtr(m.StartDate),
			WebURL: m.WebURL,
		}
		if m.IssueStats != nil {
			mo.IssueStats = &MilestoneIssueStatsOutput{
				Total: m.IssueStats.Total, Closed: m.IssueStats.Closed,
			}
		}
		out = append(out, mo)
	}
	return out
}

// EvidenceOutput mirrors gl.ReleaseEvidence (a release evidence record).
type EvidenceOutput struct {
	SHA         string `json:"sha"`
	Filepath    string `json:"filepath"`
	CollectedAt string `json:"collected_at,omitempty"`
}

func evidenceOutputs(evs []*gl.ReleaseEvidence) []*EvidenceOutput {
	if len(evs) == 0 {
		return nil
	}
	out := make([]*EvidenceOutput, 0, len(evs))
	for _, e := range evs {
		if e == nil {
			continue
		}
		out = append(out, &EvidenceOutput{
			SHA: e.SHA, Filepath: e.Filepath, CollectedAt: formatTimePtr(e.CollectedAt),
		})
	}
	return out
}
