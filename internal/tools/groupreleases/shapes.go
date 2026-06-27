package groupreleases

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// AuthorOutput is the documented reference subset of the release author object
// per doc/api/group_releases.md (the documented JSON shows id, name, username,
// state, avatar_url, web_url; created_at is not part of the release author
// subset and is intentionally omitted).

func authorOutput(u gl.BasicUser) *toolutil.AuthorOutput {
	a := toolutil.NewAuthorOutputFromBasicUser(u)
	return &a
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
		AuthoredDate:   toolutil.FormatTimePtr(c.AuthoredDate),
		CommitterName:  c.CommitterName,
		CommitterEmail: c.CommitterEmail,
		CommittedDate:  toolutil.FormatTimePtr(c.CommittedDate),
		CreatedAt:      toolutil.FormatTimePtr(c.CreatedAt),
		Message:        c.Message,
		ParentIDs:      c.ParentIDs,
		WebURL:         c.WebURL,
	}
}

// AssetSourceOutput mirrors gl.ReleaseAssetsSource (an auto-generated archive).

// AssetLinkOutput mirrors gl.ReleaseLink (a release asset link).

// AssetsOutput mirrors gl.ReleaseAssets (the assets object on a release).


// LinksOutput mirrors gl.ReleaseLinks (the release _links object).


// MilestoneIssueStatsOutput mirrors gl.ReleaseMilestoneIssueStats.

// MilestoneOutput mirrors gl.ReleaseMilestone (a milestone associated with a
// release).


// EvidenceOutput mirrors gl.ReleaseEvidence (a release evidence record).

