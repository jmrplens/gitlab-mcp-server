package dependencyproxy

import (
	"context"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// PurgeInput contains parameters for purging the dependency proxy cache.
type PurgeInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// Purge clears the dependency proxy cache for a group.
func Purge(ctx context.Context, client *gitlabclient.Client, input PurgeInput) error {
	_, err := client.GL().DependencyProxy.PurgeGroupDependencyProxy(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("gitlab_purge_dependency_proxy", err, http.StatusForbidden,
			"purging the dependency proxy cache requires group Owner role; verify group_id with gitlab_group_get; the dependency proxy must be enabled at group level")
	}
	return nil
}
