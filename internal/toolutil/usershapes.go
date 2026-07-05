package toolutil

import (
	"context"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
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
