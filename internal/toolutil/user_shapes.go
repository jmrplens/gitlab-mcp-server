// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package toolutil

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// CustomAttributeOutput mirrors gl.CustomAttribute, a key/value custom
// attribute attached to a user.
type CustomAttributeOutput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// NewCustomAttributeOutputs converts a []*gl.CustomAttribute slice, skipping
// nil elements and returning nil when no attributes remain.
func NewCustomAttributeOutputs(attrs []*gl.CustomAttribute) []CustomAttributeOutput {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]CustomAttributeOutput, 0, len(attrs))
	for _, a := range attrs {
		if a == nil {
			continue
		}
		out = append(out, CustomAttributeOutput{Key: a.Key, Value: a.Value})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// UserRefOutput mirrors gl.BasicUser as surfaced on user resources (the
// created_by object): identity fields always present, avatar/web URL and
// created_at omitted when empty. It differs from BasicUserOutput (pipeline
// sub-objects), whose avatar_url and web_url are always serialized.
type UserRefOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url,omitempty"`
	WebURL    string `json:"web_url,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// NewUserRefOutput converts a *gl.BasicUser into the user-resource reference
// shape, returning nil when the SDK value is nil.
func NewUserRefOutput(u *gl.BasicUser) *UserRefOutput {
	if u == nil {
		return nil
	}
	o := &UserRefOutput{
		ID:        u.ID,
		Username:  u.Username,
		Name:      u.Name,
		State:     u.State,
		AvatarURL: u.AvatarURL,
		WebURL:    u.WebURL,
	}
	if u.CreatedAt != nil {
		o.CreatedAt = u.CreatedAt.Format(time.RFC3339)
	}
	return o
}

// ResolveProjectWebURLs fetches the web URL for each unique project ID.
// Failures are silently ignored — missing URLs simply produce no links.
func ResolveProjectWebURLs(ctx context.Context, projects gl.ProjectsServiceInterface, projectIDs []int64) map[int64]string {
	seen := make(map[int64]string, len(projectIDs))
	for _, id := range projectIDs {
		if _, ok := seen[id]; ok || id == 0 {
			continue
		}
		proj, _, err := projects.GetProject(id, &gl.GetProjectOptions{}, gl.WithContext(ctx))
		if err != nil || proj == nil {
			seen[id] = ""
			continue
		}
		seen[id] = proj.WebURL
	}
	return seen
}

// PersonalTokenOutput mirrors gl.PersonalAccessToken as surfaced by the
// impersonation-token and current-user PAT tools: identity, scopes, and
// RFC3339/ISO-date lifecycle timestamps. The token value is only present on
// creation responses. Its granular scopes, set when the token is granular and
// the endpoint asked for them, come from the SDK. It also carries what
// lib/api/entities/personal_access_token.rb sends and the SDK struct does not
// model, read from the captured response (ADR-0021): whether the token is
// granular, and the addresses it was last used from while the instance
// exposes them.
type PersonalTokenOutput struct {
	ID             int64                      `json:"id"`
	Name           string                     `json:"name"`
	Active         bool                       `json:"active"`
	Token          string                     `json:"token,omitempty"`
	Scopes         []string                   `json:"scopes"`
	Granular       bool                       `json:"granular"`
	GranularScopes []TokenGranularScopeOutput `json:"granular_scopes,omitempty"`
	Revoked        bool                       `json:"revoked"`
	Description    string                     `json:"description,omitempty"`
	UserID         int64                      `json:"user_id"`
	CreatedAt      string                     `json:"created_at,omitempty"`
	ExpiresAt      string                     `json:"expires_at,omitempty"`
	LastUsedAt     string                     `json:"last_used_at,omitempty"`
	LastUsedIPs    []string                   `json:"last_used_ips,omitempty"`
}

// NewPersonalTokenOutput converts a gl.PersonalAccessToken into the shared
// output shape, formatting created/last-used as RFC3339 and expiry as an ISO
// date, and takes the fields the capture read beside the SDK.
func NewPersonalTokenOutput(t *gl.PersonalAccessToken, extra TokenExtra) PersonalTokenOutput {
	o := PersonalTokenOutput{
		ID:             t.ID,
		Name:           t.Name,
		Active:         t.Active,
		Token:          t.Token,
		Scopes:         t.Scopes,
		Granular:       extra.Granular,
		GranularScopes: NewTokenGranularScopes(t.GranularScopes),
		Revoked:        t.Revoked,
		Description:    t.Description,
		UserID:         t.UserID,
		LastUsedIPs:    extra.LastUsedIPs,
	}
	if t.CreatedAt != nil {
		o.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	if t.ExpiresAt != nil {
		o.ExpiresAt = time.Time(*t.ExpiresAt).Format(DateFormatISO)
	}
	if t.LastUsedAt != nil {
		o.LastUsedAt = t.LastUsedAt.Format(time.RFC3339)
	}
	return o
}

// TokenGranularScopeOutput mirrors
// lib/api/entities/personal_access_token_granular_scope.rb, one entry of the
// granular_scopes array a granular token carries. project_id is set only when
// the scope's namespace is a project and group_id only when it is a group, so
// each is absent rather than zero on the other kind.
type TokenGranularScopeOutput struct {
	Access      string   `json:"access"`
	Permissions []string `json:"permissions"`
	ProjectID   int64    `json:"project_id,omitempty"`
	GroupID     int64    `json:"group_id,omitempty"`
}

// NewTokenGranularScopes converts the granular scopes client-go decodes on a
// token into the output shape, skipping a nil entry and reading an absent
// project or group id as zero, which the output omits. It returns nil for a
// token that carries none.
func NewTokenGranularScopes(scopes []*gl.GranularScope) []TokenGranularScopeOutput {
	if len(scopes) == 0 {
		return nil
	}
	out := make([]TokenGranularScopeOutput, 0, len(scopes))
	for _, s := range scopes {
		if s == nil {
			continue
		}
		out = append(out, TokenGranularScopeOutput{
			Access:      string(s.Access),
			Permissions: s.Permissions,
			ProjectID:   derefInt64(s.ProjectID),
			GroupID:     derefInt64(s.GroupID),
		})
	}
	return out
}

// derefInt64 reads an optional id, zero when it is absent.
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
