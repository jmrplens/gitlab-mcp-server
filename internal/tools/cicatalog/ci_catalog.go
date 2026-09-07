package cicatalog

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// ResourceItem represents a CI/CD Catalog resource summary.
type ResourceItem struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	Icon                string   `json:"icon,omitempty"`
	FullPath            string   `json:"full_path"`
	WebPath             string   `json:"web_path,omitempty"`
	StarCount           int      `json:"star_count"`
	Last30DayUsageCount int      `json:"last_30_day_usage_count"`
	Archived            bool     `json:"archived"`
	Topics              []string `json:"topics,omitempty"`
	VerificationLevel   string   `json:"verification_level,omitempty"`
	VisibilityLevel     string   `json:"visibility_level,omitempty"`
	LatestReleasedAt    string   `json:"latest_released_at,omitempty"`
	LatestVersionName   string   `json:"latest_version_name,omitempty"`
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
	ReleasedAt string          `json:"released_at,omitempty"`
	CreatedAt  string          `json:"created_at,omitempty"`
	Semver     string          `json:"semver,omitempty"`
	Path       string          `json:"path,omitempty"`
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
query($search: String, $scope: CiCatalogResourceScope, $sort: CiCatalogResourceSort, $first: Int, $after: String, $last: Int, $before: String) {
  ciCatalogResources(
    search: $search
    scope: $scope
    sort: $sort
    first: $first
    after: $after
    last: $last
    before: $before
  ) {
    nodes {
      id
      name
      description
      icon
      fullPath
      webPath
      starCount
      last30DayUsageCount
      archived
      topics
      verificationLevel
      visibilityLevel
      latestReleasedAt
      versions(first: 1) {
        nodes {
          name
        }
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
    webPath
    starCount
    last30DayUsageCount
    archived
    topics
    verificationLevel
    visibilityLevel
    latestReleasedAt
    versions(first: 10) {
      nodes {
        name
        releasedAt
        createdAt
        semver {
          major
          minor
          patch
        }
        path
        readmeHtml
        components {
          nodes {
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
	Name       string             `json:"name"`
	ReleasedAt *string            `json:"releasedAt"`
	CreatedAt  *string            `json:"createdAt"`
	Semver     *gqlSemver         `json:"semver"`
	Path       *string            `json:"path"`
	ReadmeHTML *string            `json:"readmeHtml"`
	Components *gqlComponentNodes `json:"components"`
}

// gqlSemver mirrors CiCatalogResourceSemver, which the schema models as a
// major/minor/patch object rather than a scalar. All three components are
// nullable in the schema, so they decode as pointers — a partial semver must
// not collapse into a misleading "0.0.0".
type gqlSemver struct {
	Major *int `json:"major"`
	Minor *int `json:"minor"`
	Patch *int `json:"patch"`
}

// gqlComponentNodes holds the component connection of a version.
type gqlComponentNodes struct {
	Nodes []gqlComponent `json:"nodes"`
}

type gqlResourceNode struct {
	ID                  string           `json:"id"`
	Name                string           `json:"name"`
	Description         *string          `json:"description"`
	Icon                *string          `json:"icon"`
	FullPath            string           `json:"fullPath"`
	WebPath             string           `json:"webPath"`
	StarCount           int              `json:"starCount"`
	Last30DayUsageCount int              `json:"last30DayUsageCount"`
	Archived            bool             `json:"archived"`
	Topics              []string         `json:"topics"`
	VerificationLevel   *string          `json:"verificationLevel"`
	VisibilityLevel     *string          `json:"visibilityLevel"`
	LatestReleasedAt    *string          `json:"latestReleasedAt"`
	Versions            *gqlVersionNodes `json:"versions"`
}

// gqlVersionNodes holds a list of version nodes.
type gqlVersionNodes struct {
	Nodes []gqlVersion `json:"nodes"`
}

// gqlCatalogConnection holds the paginated list of CI catalog resource nodes.
type gqlCatalogConnection struct {
	Nodes    []gqlResourceNode           `json:"nodes"`
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
}

// nodeToResourceItem converts a raw GraphQL CI catalog resource node into a
// [ResourceItem] output struct, extracting optional fields only when present.
func nodeToResourceItem(n gqlResourceNode) ResourceItem {
	item := ResourceItem{
		ID:                  n.ID,
		Name:                n.Name,
		FullPath:            n.FullPath,
		WebPath:             n.WebPath,
		StarCount:           n.StarCount,
		Last30DayUsageCount: n.Last30DayUsageCount,
		Archived:            n.Archived,
		Topics:              n.Topics,
	}
	if n.Description != nil {
		item.Description = *n.Description
	}
	if n.Icon != nil {
		item.Icon = *n.Icon
	}
	if n.VerificationLevel != nil {
		item.VerificationLevel = *n.VerificationLevel
	}
	if n.VisibilityLevel != nil {
		item.VisibilityLevel = *n.VisibilityLevel
	}
	if n.LatestReleasedAt != nil {
		item.LatestReleasedAt = *n.LatestReleasedAt
	}
	if n.Versions != nil && len(n.Versions.Nodes) > 0 {
		item.LatestVersionName = n.Versions.Nodes[0].Name
	}
	return item
}

// nodeToResourceDetail converts a raw GraphQL CI catalog resource node into a
// [ResourceDetail] output struct, including README HTML, components, and version history.
func nodeToResourceDetail(n gqlResourceNode) ResourceDetail {
	detail := ResourceDetail{
		ResourceItem: nodeToResourceItem(n),
	}
	if n.Versions == nil {
		return detail
	}
	for i, v := range n.Versions.Nodes {
		item := versionToItem(v)
		detail.Versions = append(detail.Versions, item)
		// The newest version carries the resource's README and the
		// component set shown at detail level — the schema moved both
		// from the resource to its versions.
		if i == 0 {
			if v.ReadmeHTML != nil {
				detail.ReadmeHTML = *v.ReadmeHTML
			}
			detail.Components = item.Components
		}
	}
	return detail
}

// versionToItem converts a raw GraphQL version node into a [VersionItem],
// flattening the semver object and the component connection.
func versionToItem(v gqlVersion) VersionItem {
	item := VersionItem{
		Name: v.Name,
	}
	if v.ReleasedAt != nil {
		item.ReleasedAt = *v.ReleasedAt
	}
	if v.Components != nil {
		item.Components = convertComponents(v.Components.Nodes)
	}
	if v.CreatedAt != nil {
		item.CreatedAt = *v.CreatedAt
	}
	if v.Semver != nil && v.Semver.Major != nil && v.Semver.Minor != nil && v.Semver.Patch != nil {
		item.Semver = fmt.Sprintf("%d.%d.%d", *v.Semver.Major, *v.Semver.Minor, *v.Semver.Patch)
	}
	if v.Path != nil {
		item.Path = *v.Path
	}
	return item
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
	Scope  string `json:"scope,omitempty" jsonschema:"Filter scope: ALL (default) or NAMESPACES"`
	Sort   string `json:"sort,omitempty" jsonschema:"Sort order: NAME_ASC (default), NAME_DESC, LATEST_RELEASED_AT_ASC, LATEST_RELEASED_AT_DESC, STAR_COUNT_ASC, STAR_COUNT_DESC, CREATED_ASC, CREATED_DESC, USAGE_COUNT_ASC, USAGE_COUNT_DESC"`
	toolutil.GraphQLCursorPaginationInput
}

// ListOutput is the output for listing CI/CD Catalog resources.
type ListOutput struct {
	toolutil.HintableOutput
	Resources  []ResourceItem                   `json:"resources"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves CI/CD Catalog resources via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	vars, err := input.Variables(queryListResources)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_catalog_resources: %w", err)
	}
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
			CiCatalogResources gqlCatalogConnection `json:"ciCatalogResources"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}

	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListResources,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_catalog_resources", err,
			"the CI/CD Catalog requires GitLab 16.7+; scope must be one of {ALL, NAMESPACES}; sort one of {NAME_ASC, NAME_DESC, LATEST_RELEASED_AT_ASC, LATEST_RELEASED_AT_DESC, STAR_COUNT_ASC, STAR_COUNT_DESC, CREATED_ASC, CREATED_DESC, USAGE_COUNT_ASC, USAGE_COUNT_DESC}")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error. This connection has
	// no container to come back missing, so the empty page is the only sign
	// anything went wrong, and the errors are what the caller needs instead.
	if len(resp.Data.CiCatalogResources.Nodes) == 0 {
		if graphQLErr := toolutil.GraphQLTopLevelError("list_catalog_resources", resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
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
		return GetOutput{}, toolutil.WrapErrWithHint("get_catalog_resource", err,
			"verify the resource exists with gitlab_list_catalog_resources; id must be a GID (gid://gitlab/Ci::CatalogResource/N) or use full_path of the hosting project")
	}

	if resp.Data.CiCatalogResource == nil {
		lookup := input.ID
		if lookup == "" {
			lookup = input.FullPath
		}
		return GetOutput{}, fmt.Errorf("get_catalog_resource: catalog resource %q not found. Suggestion: a project marked as a catalog resource stays in draft and is not queryable until it publishes its first release. Create a release and retry, or use gitlab_list_catalog_resources to see published resources", lookup)
	}

	return GetOutput{Resource: nodeToResourceDetail(*resp.Data.CiCatalogResource)}, nil
}
