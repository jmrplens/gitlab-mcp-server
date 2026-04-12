// Package cicatalog implements MCP tool handlers for GitLab CI/CD Catalog
// resource discovery and retrieval using the GraphQL API. The CI/CD Catalog
// is a GraphQL-only feature with no REST equivalent.
package cicatalog

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ResourceItem represents a CI/CD Catalog resource summary.
type ResourceItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	Icon              string `json:"icon,omitempty"`
	FullPath          string `json:"full_path"`
	WebURL            string `json:"web_url"`
	StarCount         int    `json:"star_count"`
	ForksCount        int    `json:"forks_count"`
	OpenIssuesCount   int    `json:"open_issues_count"`
	OpenMRsCount      int    `json:"open_merge_requests_count"`
	LatestReleasedAt  string `json:"latest_released_at,omitempty"`
	LatestVersionName string `json:"latest_version_name,omitempty"`
}

// ResourceDetail extends ResourceItem with version and component information.
type ResourceDetail struct {
	ResourceItem
	ReadmeHTML string          `json:"readme_html,omitempty"`
	Versions   []VersionItem   `json:"versions,omitempty"`
	Components []ComponentItem `json:"components,omitempty"`
}

// VersionItem represents a released version of a catalog resource.
type VersionItem struct {
	Name       string          `json:"name"`
	ReleasedAt string          `json:"released_at"`
	Components []ComponentItem `json:"components,omitempty"`
}

// ComponentItem represents a single CI/CD component within a catalog resource.
type ComponentItem struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	IncludePath string      `json:"include_path"`
	Inputs      []InputItem `json:"inputs,omitempty"`
}

// InputItem represents an input parameter for a component.
type InputItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required"`
	Default     string `json:"default,omitempty"`
}

// GraphQL queries.

const queryListResources = `
query($search: String, $scope: CiCatalogResourceScope, $sort: CiCatalogResourceSort, $first: Int!, $after: String) {
  ciCatalogResources(
    search: $search
    scope: $scope
    sort: $sort
    first: $first
    after: $after
  ) {
    nodes {
      id
      name
      description
      icon
      fullPath
      webUrl
      starCount
      forksCount
      openIssuesCount
      openMergeRequestsCount
      latestReleasedAt
      latestVersion {
        name
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      endCursor
      startCursor
    }
  }
}
`

const queryGetResource = `
query($id: CiCatalogResourceID, $fullPath: ID) {
  ciCatalogResource(id: $id, fullPath: $fullPath) {
    id
    name
    description
    icon
    fullPath
    webUrl
    starCount
    forksCount
    openIssuesCount
    openMergeRequestsCount
    latestReleasedAt
    readmeHtml
    latestVersion {
      name
      releasedAt
      components {
        name
        description
        includePath
        inputs {
          name
          description
          type
          required
          default
        }
      }
    }
    versions(first: 10) {
      nodes {
        name
        releasedAt
        components {
          name
          description
          includePath
          inputs {
            name
            description
            type
            required
            default
          }
        }
      }
    }
  }
}
`

// GraphQL response structs.

type gqlInput struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Type        *string `json:"type"`
	Required    bool    `json:"required"`
	Default     *string `json:"default"`
}

type gqlComponent struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	IncludePath string     `json:"includePath"`
	Inputs      []gqlInput `json:"inputs"`
}

type gqlVersion struct {
	Name       string         `json:"name"`
	ReleasedAt string         `json:"releasedAt"`
	Components []gqlComponent `json:"components"`
}

type gqlResourceNode struct {
	ID                     string      `json:"id"`
	Name                   string      `json:"name"`
	Description            *string     `json:"description"`
	Icon                   *string     `json:"icon"`
	FullPath               string      `json:"fullPath"`
	WebURL                 string      `json:"webUrl"`
	StarCount              int         `json:"starCount"`
	ForksCount             int         `json:"forksCount"`
	OpenIssuesCount        int         `json:"openIssuesCount"`
	OpenMergeRequestsCount int         `json:"openMergeRequestsCount"`
	LatestReleasedAt       *string     `json:"latestReleasedAt"`
	ReadmeHTML             *string     `json:"readmeHtml"`
	LatestVersion          *gqlVersion `json:"latestVersion"`
	Versions               *struct {
		Nodes []gqlVersion `json:"nodes"`
	} `json:"versions"`
}

// nodeToResourceItem converts a raw GraphQL CI catalog resource node into a
// [ResourceItem] output struct, extracting optional fields only when present.
func nodeToResourceItem(n gqlResourceNode) ResourceItem {
	item := ResourceItem{
		ID:              n.ID,
		Name:            n.Name,
		FullPath:        n.FullPath,
		WebURL:          n.WebURL,
		StarCount:       n.StarCount,
		ForksCount:      n.ForksCount,
		OpenIssuesCount: n.OpenIssuesCount,
		OpenMRsCount:    n.OpenMergeRequestsCount,
	}
	if n.Description != nil {
		item.Description = *n.Description
	}
	if n.Icon != nil {
		item.Icon = *n.Icon
	}
	if n.LatestReleasedAt != nil {
		item.LatestReleasedAt = *n.LatestReleasedAt
	}
	if n.LatestVersion != nil {
		item.LatestVersionName = n.LatestVersion.Name
	}
	return item
}

// nodeToResourceDetail converts a raw GraphQL CI catalog resource node into a
// [ResourceDetail] output struct, including README HTML, components, and version history.
func nodeToResourceDetail(n gqlResourceNode) ResourceDetail {
	detail := ResourceDetail{
		ResourceItem: nodeToResourceItem(n),
	}
	if n.ReadmeHTML != nil {
		detail.ReadmeHTML = *n.ReadmeHTML
	}
	if n.LatestVersion != nil {
		detail.Components = convertComponents(n.LatestVersion.Components)
	}
	if n.Versions != nil {
		for _, v := range n.Versions.Nodes {
			detail.Versions = append(detail.Versions, VersionItem{
				Name:       v.Name,
				ReleasedAt: v.ReleasedAt,
				Components: convertComponents(v.Components),
			})
		}
	}
	return detail
}

// convertComponents transforms a slice of raw GraphQL component structs into
// typed [ComponentItem] values, including nested input specifications.
func convertComponents(gqlComps []gqlComponent) []ComponentItem {
	items := make([]ComponentItem, 0, len(gqlComps))
	for _, c := range gqlComps {
		comp := ComponentItem{
			Name:        c.Name,
			IncludePath: c.IncludePath,
		}
		if c.Description != nil {
			comp.Description = *c.Description
		}
		for _, inp := range c.Inputs {
			item := InputItem{
				Name:     inp.Name,
				Required: inp.Required,
			}
			if inp.Description != nil {
				item.Description = *inp.Description
			}
			if inp.Type != nil {
				item.Type = *inp.Type
			}
			if inp.Default != nil {
				item.Default = *inp.Default
			}
			comp.Inputs = append(comp.Inputs, item)
		}
		items = append(items, comp)
	}
	return items
}

// List.

// ListInput is the input for listing CI/CD Catalog resources.
type ListInput struct {
	Search string `json:"search,omitempty" jsonschema:"Search resources by name or description"`
	Scope  string `json:"scope,omitempty" jsonschema:"Filter scope: ALL (default) or NAMESPACED"`
	Sort   string `json:"sort,omitempty" jsonschema:"Sort order: NAME_ASC (default), NAME_DESC, LATEST_RELEASED_AT_ASC, LATEST_RELEASED_AT_DESC, STAR_COUNT_ASC, STAR_COUNT_DESC"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing CI/CD Catalog resources.
type ListOutput struct {
	toolutil.HintableOutput
	Resources  []ResourceItem                   `json:"resources"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves CI/CD Catalog resources via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	vars := input.GraphQLPaginationInput.Variables()
	if input.Search != "" {
		vars["search"] = input.Search
	}
	if input.Scope != "" {
		vars["scope"] = input.Scope
	}
	if input.Sort != "" {
		vars["sort"] = input.Sort
	}

	var resp struct {
		Data struct {
			CiCatalogResources struct {
				Nodes    []gqlResourceNode           `json:"nodes"`
				PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
			} `json:"ciCatalogResources"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListResources,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_catalog_resources", err)
	}

	items := make([]ResourceItem, 0, len(resp.Data.CiCatalogResources.Nodes))
	for _, n := range resp.Data.CiCatalogResources.Nodes {
		items = append(items, nodeToResourceItem(n))
	}

	return ListOutput{
		Resources:  items,
		Pagination: toolutil.PageInfoToOutput(resp.Data.CiCatalogResources.PageInfo),
	}, nil
}

// Get.

// GetInput is the input for getting a single CI/CD Catalog resource.
type GetInput struct {
	ID       string `json:"id,omitempty" jsonschema:"Catalog resource GID (e.g. gid://gitlab/Ci::CatalogResource/1). Use either id or full_path."`
	FullPath string `json:"full_path,omitempty" jsonschema:"Full path of the project hosting the resource (e.g. my-group/my-components). Use either id or full_path."`
}

// GetOutput is the output for getting a single CI/CD Catalog resource.
type GetOutput struct {
	toolutil.HintableOutput
	Resource ResourceDetail `json:"resource"`
}

// Get retrieves a single CI/CD Catalog resource via the GitLab GraphQL API.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ID == "" && input.FullPath == "" {
		return GetOutput{}, errors.New("get_catalog_resource: either id or full_path is required")
	}

	vars := make(map[string]any)
	if input.ID != "" {
		vars["id"] = input.ID
	}
	if input.FullPath != "" {
		vars["fullPath"] = input.FullPath
	}

	var resp struct {
		Data struct {
			CiCatalogResource *gqlResourceNode `json:"ciCatalogResource"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryGetResource,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_catalog_resource", err)
	}

	if resp.Data.CiCatalogResource == nil {
		lookup := input.ID
		if lookup == "" {
			lookup = input.FullPath
		}
		return GetOutput{}, fmt.Errorf("get_catalog_resource: catalog resource %q not found", lookup)
	}

	return GetOutput{Resource: nodeToResourceDetail(*resp.Data.CiCatalogResource)}, nil
}
