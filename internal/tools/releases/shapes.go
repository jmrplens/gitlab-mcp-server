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

// CommitOutput — the canonical commit shape for project releases lives in
// toolutil.CommitOutput. The project-release endpoint does not expose web_url
// on the embedded commit object, so the base type is used directly.

// releaseAuthorOutput wraps toolutil.NewAuthorOutputFromBasicUser for the
// release author. Kept as a package-local helper so the call site reads
// consistently with the other release converters and to allow per-package
// variations later without touching the shared helper.
func releaseAuthorOutput(u gl.BasicUser) *toolutil.AuthorOutput {
	a := toolutil.NewAuthorOutputFromBasicUser(u)
	return &a
}
