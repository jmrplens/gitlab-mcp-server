// Package epicworkitems contains shared GraphQL helpers for epic-backed work items.
package epicworkitems

import (
	"context"
	"fmt"
	"strconv"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

const queryResolveWorkItemGID = `
query($fullPath: ID!, $iid: String!) {
  namespace(fullPath: $fullPath) {
    workItem(iid: $iid) {
      id
    }
  }
}
`

// ResolveEpicGID resolves an epic-backed work item GID by group path and IID.
func ResolveEpicGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return resolveGID(ctx, client, fullPath, iid, func() error {
		return fmt.Errorf("epic not found in group %q with IID %d", fullPath, iid)
	})
}

// ResolveWorkItemGID resolves a generic work item GID by namespace path and IID.
func ResolveWorkItemGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64) (string, error) {
	return resolveGID(ctx, client, fullPath, iid, func() error {
		return fmt.Errorf("work item not found in %q with IID %d", fullPath, iid)
	})
}

type gqlWorkItemID struct {
	ID string `json:"id"`
}

type gqlNamespaceWorkItemID struct {
	WorkItem *gqlWorkItemID `json:"workItem"`
}

func resolveGID(ctx context.Context, client *gitlabclient.Client, fullPath string, iid int64, notFound func() error) (string, error) {
	var resp struct {
		Data struct {
			Namespace *gqlNamespaceWorkItemID `json:"namespace"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryResolveWorkItemGID,
		Variables: map[string]any{
			"fullPath": fullPath,
			"iid":      strconv.FormatInt(iid, 10),
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return "", err
	}

	if resp.Data.Namespace == nil || resp.Data.Namespace.WorkItem == nil {
		return "", notFound()
	}

	return resp.Data.Namespace.WorkItem.ID, nil
}
