package cicatalog

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	StarrersPath        string   `json:"starrers_path,omitempty"`
	Last30DayUsageCount int      `json:"last_30_day_usage_count"`
	Archived            bool     `json:"archived"`
	Topics              []string `json:"topics,omitempty"`
	VerificationLevel   string   `json:"verification_level,omitempty"`
	VisibilityLevel     string   `json:"visibility_level,omitempty"`
	LatestReleasedAt    string   `json:"latest_released_at,omitempty"`
	LatestVersionName   string   `json:"latest_version_name,omitempty"`
}

// ResourceDetail extends ResourceItem with version and component information.
// The README is the latest version's, in its Markdown source and as GitLab
// renders it: GitLab resolves a version's README for one version per request,
// so the older versions listed here carry none.
type ResourceDetail struct {
	ResourceItem
	Readme     string          `json:"readme,omitempty"`
	ReadmeHTML string          `json:"readme_html,omitempty"`
	Versions   []VersionItem   `json:"versions,omitempty"`
	Components []ComponentItem `json:"components,omitempty"`
}

// VersionItem represents a released version of a catalog resource.
type VersionItem struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	ReleasedAt string          `json:"released_at,omitempty"`
	CreatedAt  string          `json:"created_at,omitempty"`
	Semver     string          `json:"semver,omitempty"`
	Path       string          `json:"path,omitempty"`
	Author     *VersionAuthor  `json:"author,omitempty"`
	Commit     *VersionCommit  `json:"commit,omitempty"`
	Components []ComponentItem `json:"components,omitempty"`
}

// VersionAuthor is the user who published a version. The avatar URL is the
// one GitLab sends, which may be a path relative to the instance.
type VersionAuthor struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	WebURL    string `json:"web_url"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// VersionCommit is the commit a version was released from.
type VersionCommit struct {
	SHA     string `json:"sha"`
	ShortID string `json:"short_id"`
	Title   string `json:"title,omitempty"`
	WebURL  string `json:"web_url"`
}

// ComponentItem represents a single CI/CD component within a catalog resource.
type ComponentItem struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Description         string      `json:"description,omitempty"`
	IncludePath         string      `json:"include_path"`
	Last30DayUsageCount *int        `json:"last_30_day_usage_count,omitempty"`
	Inputs              []InputItem `json:"inputs,omitempty"`
}

// InputItem represents an input parameter for a component.
//
// A default and the options are values of the input's own type, which GitLab
// sends as the JSON it is: a string, a number, a boolean or an array. They are
// published as that value rather than as its text, so a boolean input's false
// default reaches the caller as false.
type InputItem struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Type        string      `json:"type,omitempty"`
	Required    bool        `json:"required"`
	Default     any         `json:"default,omitempty"`
	Options     any         `json:"options,omitempty"`
	Regex       string      `json:"regex,omitempty"`
	Rules       []InputRule `json:"rules,omitempty"`
}

// InputRule is a conditional rule of an input: when its expression holds,
// the input takes this default and offers these options instead.
type InputRule struct {
	If      string `json:"if,omitempty"`
	Default any    `json:"default,omitempty"`
	Options []any  `json:"options,omitempty"`
}

// GraphQL queries.
//
// GitLab refuses a whole document that names a field it does not have, so the
// newest field a document selects is the oldest release it works on. Measured
// against GitLab's versioned GraphQL references, the get document needs GitLab
// 18.10 (a component's description), as it did before it read the README
// source, the author, the commit and an input's rules (18.6 the newest of
// them), and the listing now needs 18.1 (a resource's archived flag) where it
// needed 18.10 for the components it read and dropped.

// resourceSelection is what both documents select of a catalog resource
// itself.
const resourceSelection = `id
    name
    description
    icon
    fullPath
    webPath
    starCount
    starrersPath
    last30DayUsageCount
    archived
    topics
    verificationLevel
    visibilityLevel
    latestReleasedAt`

// queryListResources reads each resource and the name of its latest version,
// which is all a listing publishes of it. It used to select that version's
// rendered README and every component with its inputs as well, and drop them:
// measured on gitlab.com on 2026-09-26, a page of twenty resources was 3 MB
// and twenty seconds that way and is 13 KB and seven seconds this way, with
// the same latest versions.
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
      ` + resourceSelection + `
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

// queryGetResource reads one resource with its last ten versions, and the
// latest version a second time, under an alias, for its README: GitLab
// resolves CiCatalogResourceVersion.readme for one version per request and
// answers the rest with an error, so the README cannot be asked of the ten.
// Measured on gitlab.com on 2026-09-26 the document scores 83 against the
// complexity limit of 200 GitLab allows an anonymous caller.
const queryGetResource = `
query($id: CiCatalogResourceID, $fullPath: ID) {
  ciCatalogResource(id: $id, fullPath: $fullPath) {
    ` + resourceSelection + `
    versions(first: 10) {
      nodes {
        id
        name
        releasedAt
        createdAt
        semver {
          major
          minor
          patch
        }
        path
        author {
          id
          username
          name
          webUrl
          avatarUrl
        }
        commit {
          sha
          shortId
          title
          webUrl
        }
        components {
          nodes {
            id
            name
            description
            includePath
            last30DayUsageCount
            inputs {
              name
              description
              type
              required
              default
              options
              regex
              rules {
                if
                default
                options
              }
            }
          }
        }
      }
    }
    latestVersion: versions(first: 1) {
      nodes {
        readme
        readmeHtml
      }
    }
  }
}
`

// GraphQL response structs.

// gqlInputRule decodes a conditional rule of an input. Its values are
// arbitrary JSON, so they decode into any.
type gqlInputRule struct {
	If      string `json:"if"`
	Default any    `json:"default"`
	Options []any  `json:"options"`
}

// gqlInput decodes an input. The default and the options are CiInputsValue,
// arbitrary JSON: a string pointer failed the whole call on the first boolean
// or number default, which the catalog's own components/sast carries.
type gqlInput struct {
	Name        string         `json:"name"`
	Description *string        `json:"description"`
	Type        *string        `json:"type"`
	Required    bool           `json:"required"`
	Default     any            `json:"default"`
	Options     any            `json:"options"`
	Regex       string         `json:"regex"`
	Rules       []gqlInputRule `json:"rules"`
}

type gqlComponent struct {
	ID                  string     `json:"id"`
	Name                string     `json:"name"`
	Description         *string    `json:"description"`
	IncludePath         string     `json:"includePath"`
	Last30DayUsageCount *int       `json:"last30DayUsageCount"`
	Inputs              []gqlInput `json:"inputs"`
}

// gqlVersionAuthor decodes the user who published a version.
type gqlVersionAuthor struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	WebURL    string `json:"webUrl"`
	AvatarURL string `json:"avatarUrl"`
}

// gqlVersionCommit decodes the commit a version was released from.
type gqlVersionCommit struct {
	SHA     string `json:"sha"`
	ShortID string `json:"shortId"`
	Title   string `json:"title"`
	WebURL  string `json:"webUrl"`
}

type gqlVersion struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	ReleasedAt *string            `json:"releasedAt"`
	CreatedAt  *string            `json:"createdAt"`
	Semver     *gqlSemver         `json:"semver"`
	Path       *string            `json:"path"`
	Author     *gqlVersionAuthor  `json:"author"`
	Commit     *gqlVersionCommit  `json:"commit"`
	Components *gqlComponentNodes `json:"components"`
}

// gqlVersionName decodes a version as the listing reads it: its name.
type gqlVersionName struct {
	Name string `json:"name"`
}

// gqlVersionNameNodes holds the one version a listing reads per resource.
type gqlVersionNameNodes struct {
	Nodes []gqlVersionName `json:"nodes"`
}

// gqlVersionReadme decodes the latest version's README, read under an alias
// because GitLab answers it for one version per request.
type gqlVersionReadme struct {
	Readme     *string `json:"readme"`
	ReadmeHTML *string `json:"readmeHtml"`
}

// gqlVersionReadmeNodes holds the latest version the alias reads.
type gqlVersionReadmeNodes struct {
	Nodes []gqlVersionReadme `json:"nodes"`
}

// gqlSemver mirrors CiCatalogResourceSemver, which the schema models as a
// major/minor/patch object rather than a scalar. All three components are
// nullable in the schema, so they decode as pointers: a partial semver must
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

// gqlResourceFields are the fields of a catalog resource both documents
// select.
type gqlResourceFields struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         *string  `json:"description"`
	Icon                *string  `json:"icon"`
	FullPath            string   `json:"fullPath"`
	WebPath             string   `json:"webPath"`
	StarCount           int      `json:"starCount"`
	StarrersPath        string   `json:"starrersPath"`
	Last30DayUsageCount int      `json:"last30DayUsageCount"`
	Archived            bool     `json:"archived"`
	Topics              []string `json:"topics"`
	VerificationLevel   *string  `json:"verificationLevel"`
	VisibilityLevel     *string  `json:"visibilityLevel"`
	LatestReleasedAt    *string  `json:"latestReleasedAt"`
}

// gqlResourceListNode is a catalog resource as the listing selects it.
type gqlResourceListNode struct {
	gqlResourceFields
	Versions *gqlVersionNameNodes `json:"versions"`
}

// gqlResourceNode is a catalog resource as the get document selects it.
type gqlResourceNode struct {
	gqlResourceFields
	Versions      *gqlVersionNodes       `json:"versions"`
	LatestVersion *gqlVersionReadmeNodes `json:"latestVersion"`
}

// gqlVersionNodes holds a list of version nodes.
type gqlVersionNodes struct {
	Nodes []gqlVersion `json:"nodes"`
}

// gqlCatalogConnection holds the paginated list of CI catalog resource nodes.
type gqlCatalogConnection struct {
	Nodes    []gqlResourceListNode       `json:"nodes"`
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
}

// item converts the fields both documents select into a [ResourceItem],
// extracting optional fields only when present, with the latest version's
// name the caller read.
func (n gqlResourceFields) item(latestVersionName string) ResourceItem {
	item := ResourceItem{
		ID:                  n.ID,
		Name:                n.Name,
		FullPath:            n.FullPath,
		WebPath:             n.WebPath,
		StarCount:           n.StarCount,
		StarrersPath:        n.StarrersPath,
		Last30DayUsageCount: n.Last30DayUsageCount,
		Archived:            n.Archived,
		Topics:              n.Topics,
		LatestVersionName:   latestVersionName,
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
	return item
}

// nodeToResourceItem converts a listed catalog resource into a
// [ResourceItem], naming its latest version when it has one.
func nodeToResourceItem(n gqlResourceListNode) ResourceItem {
	var latest string
	if n.Versions != nil && len(n.Versions.Nodes) > 0 {
		latest = n.Versions.Nodes[0].Name
	}
	return n.item(latest)
}

// nodeToResourceDetail converts a catalog resource the get document read into
// a [ResourceDetail]: its versions, the latest version's components, and the
// latest version's README from the alias that reads it.
func nodeToResourceDetail(n gqlResourceNode) ResourceDetail {
	var versions []VersionItem
	if n.Versions != nil {
		for _, v := range n.Versions.Nodes {
			versions = append(versions, versionToItem(v))
		}
	}
	var latest string
	detail := ResourceDetail{Versions: versions}
	// The newest version carries the component set shown at detail level,
	// since the schema moved it from the resource to its versions.
	if len(versions) > 0 {
		latest = versions[0].Name
		detail.Components = versions[0].Components
	}
	detail.ResourceItem = n.item(latest)
	if n.LatestVersion != nil && len(n.LatestVersion.Nodes) > 0 {
		readme := n.LatestVersion.Nodes[0]
		if readme.Readme != nil {
			detail.Readme = *readme.Readme
		}
		if readme.ReadmeHTML != nil {
			detail.ReadmeHTML = *readme.ReadmeHTML
		}
	}
	return detail
}

// versionToItem converts a raw GraphQL version node into a [VersionItem],
// flattening the semver object and the component connection.
func versionToItem(v gqlVersion) VersionItem {
	item := VersionItem{
		ID:   v.ID,
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
	if v.Author != nil {
		item.Author = &VersionAuthor{
			ID:        v.Author.ID,
			Username:  v.Author.Username,
			Name:      v.Author.Name,
			WebURL:    v.Author.WebURL,
			AvatarURL: v.Author.AvatarURL,
		}
	}
	if v.Commit != nil {
		item.Commit = &VersionCommit{
			SHA:     v.Commit.SHA,
			ShortID: v.Commit.ShortID,
			Title:   v.Commit.Title,
			WebURL:  v.Commit.WebURL,
		}
	}
	return item
}

// convertComponents transforms a slice of raw GraphQL component structs into
// typed [ComponentItem] values, including nested input specifications.
func convertComponents(gqlComps []gqlComponent) []ComponentItem {
	items := make([]ComponentItem, 0, len(gqlComps))
	for _, c := range gqlComps {
		comp := ComponentItem{
			ID:                  c.ID,
			Name:                c.Name,
			IncludePath:         c.IncludePath,
			Last30DayUsageCount: c.Last30DayUsageCount,
		}
		if c.Description != nil {
			comp.Description = *c.Description
		}
		for _, inp := range c.Inputs {
			comp.Inputs = append(comp.Inputs, inputToItem(inp))
		}
		items = append(items, comp)
	}
	return items
}

// inputToItem converts one input with its conditional rules. The default and
// the options pass through as the JSON values GitLab sent.
func inputToItem(inp gqlInput) InputItem {
	item := InputItem{
		Name:     inp.Name,
		Required: inp.Required,
		Default:  inp.Default,
		Options:  inp.Options,
		Regex:    inp.Regex,
	}
	if inp.Description != nil {
		item.Description = *inp.Description
	}
	if inp.Type != nil {
		item.Type = *inp.Type
	}
	for _, rule := range inp.Rules {
		item.Rules = append(item.Rules, InputRule(rule))
	}
	return item
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
	ID       string `json:"id,omitempty" jsonschema:"Catalog resource GID (e.g. gid://gitlab/Ci::CatalogResource/1). Give exactly one of id or full_path. Sending both is refused."`
	FullPath string `json:"full_path,omitempty" jsonschema:"Full path of the project hosting the resource (e.g. my-group/my-components). Give exactly one of id or full_path. Sending both is refused."`
}

// GetOutput is the output for getting a single CI/CD Catalog resource.
type GetOutput struct {
	toolutil.HintableOutput
	Resource ResourceDetail `json:"resource"`
}

// Get retrieves a single CI/CD Catalog resource via the GitLab GraphQL API.
//
// The exclusion between id and full_path is enforced here because the schema
// cannot carry it: ciCatalogResource declares both arguments as nullable, and
// "exactly one of them" is a resolver rule GitLab answers with an error and no
// data. Sending both used to be reported to the caller as "catalog resource
// not found", with a suggestion that the resource was an unpublished draft,
// which is a false answer to a question this server could have refused itself.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ID == "" && input.FullPath == "" {
		return GetOutput{}, errors.New("get_catalog_resource: either id or full_path is required")
	}
	if input.ID != "" && input.FullPath != "" {
		return GetOutput{}, errors.New("get_catalog_resource: give exactly one of id or full_path, not both. GitLab refuses a query carrying both")
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
		// Errors is what a refused query carries instead of data. Without it a
		// GitLab sentence naming what was wrong with the request is discarded,
		// and the nil resource below becomes a not-found message the caller has
		// no reason to doubt.
		Errors []toolutil.GraphQLError `json:"errors"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryGetResource,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithHint("get_catalog_resource", err,
			"verify the resource exists with ci_catalog.list; id must be a GID (gid://gitlab/Ci::CatalogResource/N) or use full_path of the hosting project")
	}

	if resp.Data.CiCatalogResource == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError("get_catalog_resource", resp.Errors); graphQLErr != nil {
			return GetOutput{}, graphQLErr
		}
		lookup := input.ID
		if lookup == "" {
			lookup = input.FullPath
		}
		return GetOutput{}, fmt.Errorf("get_catalog_resource: catalog resource %q not found. Suggestion: a project marked as a catalog resource stays in draft and is not queryable until it publishes its first release. Create a release and retry, or use ci_catalog.list to see published resources", lookup)
	}

	return GetOutput{Resource: nodeToResourceDetail(*resp.Data.CiCatalogResource)}, nil
}
