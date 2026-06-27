package releases

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).
//
// This file covers the release sub-objects surfaced on the canonical json keys
// (author, commit, assets, _links, milestones, evidences).

func assetsOptions(in *AssetsInput) *gl.ReleaseAssetsOptions {
	if in == nil || len(in.Links) == 0 {
		return nil
	}
	links := make([]*gl.ReleaseAssetLinkOptions, 0, len(in.Links))
	for i := range in.Links {
		links = append(links, assetLinkOption(in.Links[i]))
	}
	return &gl.ReleaseAssetsOptions{Links: links}
}

// assetLinkOption converts a single AssetLinkInput into the SDK option type,
// setting only the fields the caller supplied.
func assetLinkOption(l AssetLinkInput) *gl.ReleaseAssetLinkOptions {
	opt := &gl.ReleaseAssetLinkOptions{}
	if l.Name != "" {
		opt.Name = new(l.Name)
	}
	if l.URL != "" {
		opt.URL = new(l.URL)
	}
	if l.FilePath != "" {
		opt.FilePath = new(l.FilePath)
	}
	if l.DirectAssetPath != "" {
		opt.DirectAssetPath = new(l.DirectAssetPath)
	}
	if l.LinkType != "" {
		opt.LinkType = new(gl.LinkTypeValue(l.LinkType))
	}
	return opt
}

// AuthorOutput is the documented reference subset of the release author object
// per doc/api/releases/_index.md (the documented JSON shows id, name, username,
// state, avatar_url, web_url; created_at is not part of the release author
// subset and is intentionally omitted).


// CommitOutput is the documented reference subset of the commit the release tag
// points to per doc/api/releases/_index.md. The documented release commit JSON
// surfaces id, short_id, title, created_at, parent_ids, message, author_name,
// author_email, authored_date, committer_name, committer_email, committed_date.
// SDK-only commit fields (stats, status, project_id, web_url) are not part of
// the documented release commit subset and are intentionally omitted.
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
	}
}

// releaseAuthorOutput wraps toolutil.NewAuthorOutputFromBasicUser for the
// release author. Kept as a package-local helper so the call site reads
// consistently with the other release converters and to allow per-package
// variations later without touching the shared helper.
func releaseAuthorOutput(u gl.BasicUser) *toolutil.AuthorOutput {
	a := toolutil.NewAuthorOutputFromBasicUser(u)
	return &a
}


