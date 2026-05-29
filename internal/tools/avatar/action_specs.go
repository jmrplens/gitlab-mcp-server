package avatar

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ActionSpecs returns canonical specs for avatar lookup actions exposed through gitlab_user.
func ActionSpecs(client *gitlabclient.Client) []toolutil.ActionSpec {
	return []toolutil.ActionSpec{
		toolutil.NewReadActionSpec("avatar_get", toolutil.RouteAction(client, Get), avatarOptions()),
	}
}

func avatarOptions() toolutil.ActionSpecOptions {
	return toolutil.ActionSpecOptions{
		Aliases:        []string{"avatar", "user avatar", "avatar url", "avatar lookup", "lookup avatar by email"},
		Tags:           []string{"user", "avatar", "profile"},
		Usage:          "Resolve the avatar URL for a known email address. Use this when the task already provides an email; do not use it to search for users by name or username.",
		RelatedActions: []string{"user.me", "user.emails", "user.current_user_status"},
		ParameterGuidance: map[string]toolutil.ParameterGuidance{
			"email": {
				SemanticRole:     "email_address",
				ValueSource:      "Email address supplied by the task or returned by another GitLab user lookup.",
				ExampleBinding:   `params.email:"user@example.com"`,
				CommonConfusions: []string{"Do not send a username or display name as email."},
			},
			"size": {
				SemanticRole:     "image_size_pixels",
				ValueSource:      "Optional avatar size in pixels; omit it to let GitLab choose the default size.",
				ExampleBinding:   "params.size:128",
				CommonConfusions: []string{"Send a numeric pixel size such as 64 or 128, not CSS strings like 64px."},
			},
		},
		OpenWorld:    true,
		OwnerPackage: "avatar",
		IndividualTool: toolutil.IndividualToolSpec{
			Name:        "gitlab_get_avatar",
			Title:       toolutil.TitleFromName("gitlab_get_avatar"),
			Description: "Get the avatar URL for an email address. Returns: the resolved avatar URL for that email address. See also: gitlab_user_current, gitlab_get_user_status.",
		},
	}
}
