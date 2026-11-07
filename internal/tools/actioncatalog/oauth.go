// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actioncatalog

import "github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/actiongrants"

// OAuthRefused reports whether GitLab refuses the action to an OAuth access
// token on every way it can run, whatever scopes the token carries: each way
// sends a route the generated table marks as answered only for a personal
// access token (issue 1238). The calling token's own revocation and
// rotations are, and the model registry's package file download; the
// personal token read is not, since a call naming the token by id reaches a
// route GitLab serves an OAuth token.
func (a Action) OAuthRefused() bool {
	_, everyWay := actiongrants.Table().OAuthRefusal(a.FineGrained)
	return everyWay
}

// FilterServedToOAuth returns a catalog without the actions GitLab refuses to
// an OAuth access token on every way ([Action.OAuthRefused]), with the groups
// left empty dropped. A kept group is read-only when every action it keeps
// is, as [Catalog.FilterReachableWith] reads it.
func (c *Catalog) FilterServedToOAuth() *Catalog {
	if c == nil {
		return nil
	}
	filtered := NewCatalog()
	for _, group := range c.Groups() {
		served := NewGroup(GroupOptions{
			ToolName:               group.ToolName,
			Title:                  group.Title,
			Description:            group.Description,
			Icons:                  group.Icons,
			ReadOnly:               true,
			FormatResult:           group.FormatResult,
			BaseDomain:             group.BaseDomain,
			EnterpriseOnly:         group.EnterpriseOnly,
			GitLabDotComOnly:       group.GitLabDotComOnly,
			CapabilityRequirements: group.CapabilityRequirements,
			OwnerPackage:           group.OwnerPackage,
			SurfaceKind:            group.SurfaceKind,
		})
		for _, action := range group.ActionsInOrder() {
			if action.OAuthRefused() {
				continue
			}
			served.ReadOnly = served.ReadOnly && action.ReadOnly
			served.SetAction(action)
		}
		if len(served.Actions) == 0 {
			continue
		}
		mustAddCatalogGroup(filtered, served, "filter actions served to an OAuth access token")
	}
	return filtered
}
