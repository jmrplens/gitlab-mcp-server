// Package vulnerabilities implements MCP tool handlers for GitLab vulnerability
// management using the GraphQL API. It covers listing project vulnerabilities,
// retrieving individual vulnerability details, and state mutations (dismiss,
// confirm, resolve, revert).
package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// IdentifierItem represents a vulnerability identifier (CVE, CWE, etc.).
type IdentifierItem struct {
	Name         string `json:"name"`
	ExternalType string `json:"external_type,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	URL          string `json:"url,omitempty"`
}

// ScannerItem represents the scanner that detected the vulnerability.
type ScannerItem struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor,omitempty"`
	ScannerID string `json:"scanner_id,omitempty"`
}

// LocationItem represents the location where the vulnerability was found.
type LocationItem struct {
	File      string `json:"file,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	BlobPath  string `json:"blob_path,omitempty"`
}

// ProjectItem represents a minimal project reference on a vulnerability.
type ProjectItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
}

// Item is a summary of a vulnerability.
type Item struct {
	ID              string           `json:"id"`
	Title           string           `json:"title"`
	Severity        string           `json:"severity"`
	State           string           `json:"state"`
	Description     string           `json:"description,omitempty"`
	ReportType      string           `json:"report_type,omitempty"`
	Scanner         *ScannerItem     `json:"scanner,omitempty"`
	Location        *LocationItem    `json:"location,omitempty"`
	Identifiers     []IdentifierItem `json:"identifiers,omitempty"`
	DetectedAt      string           `json:"detected_at,omitempty"`
	DismissedAt     string           `json:"dismissed_at,omitempty"`
	ResolvedAt      string           `json:"resolved_at,omitempty"`
	ConfirmedAt     string           `json:"confirmed_at,omitempty"`
	Project         *ProjectItem     `json:"project,omitempty"`
	WebURL          string           `json:"web_url,omitempty"`
	PrimaryID       *IdentifierItem  `json:"primary_identifier,omitempty"`
	Solution        string           `json:"solution,omitempty"`
	HasSolutions    bool             `json:"has_solutions,omitempty"`
	HasIssues       bool             `json:"has_issues,omitempty"`
	HasMR           bool             `json:"has_merge_request,omitempty"`
	DismissalReason string           `json:"dismissal_reason,omitempty"`
}

// GraphQL queries.

const queryListVulnerabilities = `
query($projectPath: ID!, $first: Int!, $after: String, $severity: [String!], $state: [VulnerabilityState!], $scanner: [String!], $reportType: [VulnerabilityReportType!], $hasIssues: Boolean, $hasResolution: Boolean, $sort: VulnerabilitySort) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first, after: $after, severity: $severity, state: $state, scanner: $scanner, reportType: $reportType, hasIssues: $hasIssues, hasResolution: $hasResolution, sort: $sort) {
      nodes {
        id
        title
        severity
        state
        reportType
        detectedAt
        dismissedAt
        resolvedAt
        confirmedAt
        primaryIdentifier {
          name
          externalType
          externalId
          url
        }
        scanner {
          name
          vendor
        }
        location {
          ... on VulnerabilityLocationSast {
            file
            startLine
            endLine
            blobPath
          }
          ... on VulnerabilityLocationDast {
            path
          }
          ... on VulnerabilityLocationDependencyScanning {
            file
            blobPath
          }
          ... on VulnerabilityLocationContainerScanning {
            image
          }
          ... on VulnerabilityLocationSecretDetection {
            file
            startLine
            endLine
            blobPath
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
}
`

const queryGetVulnerability = `
query($id: VulnerabilityID!) {
  vulnerability(id: $id) {
    id
    title
    severity
    state
    description
    reportType
    detectedAt
    dismissedAt
    resolvedAt
    confirmedAt
    solution
    hasSolutions
    dismissalReason
    primaryIdentifier {
      name
      externalType
      externalId
      url
    }
    identifiers {
      name
      externalType
      externalId
      url
    }
    scanner {
      name
      vendor
    }
    location {
      ... on VulnerabilityLocationSast {
        file
        startLine
        endLine
        blobPath
      }
      ... on VulnerabilityLocationDast {
        path
      }
      ... on VulnerabilityLocationDependencyScanning {
        file
        blobPath
      }
      ... on VulnerabilityLocationContainerScanning {
        image
      }
      ... on VulnerabilityLocationSecretDetection {
        file
        startLine
        endLine
        blobPath
      }
    }
    project {
      id
      name
      fullPath
    }
    issueLinks {
      nodes {
        id
      }
    }
    mergeRequest {
      iid
    }
  }
}
`

// GraphQL response structs (camelCase to match API).

type gqlIdentifier struct {
	Name         string `json:"name"`
	ExternalType string `json:"externalType"`
	ExternalID   string `json:"externalId"`
	URL          string `json:"url"`
}

type gqlScanner struct {
	Name   string `json:"name"`
	Vendor string `json:"vendor"`
}

type gqlLocation struct {
	File      string `json:"file"`
	Path      string `json:"path"`
	Image     string `json:"image"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	BlobPath  string `json:"blobPath"`
}

type gqlProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"fullPath"`
}

type gqlVulnerabilityNode struct {
	ID                string          `json:"id"`
	Title             string          `json:"title"`
	Severity          string          `json:"severity"`
	State             string          `json:"state"`
	Description       string          `json:"description"`
	ReportType        string          `json:"reportType"`
	DetectedAt        string          `json:"detectedAt"`
	DismissedAt       string          `json:"dismissedAt"`
	ResolvedAt        string          `json:"resolvedAt"`
	ConfirmedAt       string          `json:"confirmedAt"`
	Solution          string          `json:"solution"`
	HasSolutions      bool            `json:"hasSolutions"`
	DismissalReason   string          `json:"dismissalReason"`
	PrimaryIdentifier *gqlIdentifier  `json:"primaryIdentifier"`
	Identifiers       []gqlIdentifier `json:"identifiers"`
	Scanner           *gqlScanner     `json:"scanner"`
	Location          *gqlLocation    `json:"location"`
	Project           *gqlProject     `json:"project"`
	IssueLinks        *struct {
		Nodes []struct {
			ID string `json:"id"`
		} `json:"nodes"`
	} `json:"issueLinks"`
	MergeRequest *struct {
		IID string `json:"iid"`
	} `json:"mergeRequest"`
}

// nodeToItem converts a raw GraphQL vulnerability node into an [Item] output
// struct, mapping identifiers, scanner, location, project, issues, and MR fields.
func nodeToItem(n gqlVulnerabilityNode) Item {
	item := Item{
		ID:              n.ID,
		Title:           n.Title,
		Severity:        n.Severity,
		State:           n.State,
		Description:     n.Description,
		ReportType:      n.ReportType,
		DetectedAt:      n.DetectedAt,
		DismissedAt:     n.DismissedAt,
		ResolvedAt:      n.ResolvedAt,
		ConfirmedAt:     n.ConfirmedAt,
		Solution:        n.Solution,
		HasSolutions:    n.HasSolutions,
		DismissalReason: n.DismissalReason,
	}
	if n.PrimaryIdentifier != nil {
		item.PrimaryID = identifierToItem(n.PrimaryIdentifier)
	}
	for _, id := range n.Identifiers {
		item.Identifiers = append(item.Identifiers, *identifierToItem(&id))
	}
	if n.Scanner != nil {
		item.Scanner = &ScannerItem{Name: n.Scanner.Name, Vendor: n.Scanner.Vendor}
	}
	if n.Location != nil {
		loc := &LocationItem{
			File:      n.Location.File,
			StartLine: n.Location.StartLine,
			EndLine:   n.Location.EndLine,
			BlobPath:  n.Location.BlobPath,
		}
		if loc.File == "" && n.Location.Path != "" {
			loc.File = n.Location.Path
		}
		if loc.File == "" && n.Location.Image != "" {
			loc.File = n.Location.Image
		}
		item.Location = loc
	}
	if n.Project != nil {
		item.Project = &ProjectItem{ID: n.Project.ID, Name: n.Project.Name, FullPath: n.Project.FullPath}
	}
	if n.IssueLinks != nil && len(n.IssueLinks.Nodes) > 0 {
		item.HasIssues = true
	}
	if n.MergeRequest != nil {
		item.HasMR = true
	}
	return item
}

// identifierToItem converts a raw GraphQL identifier struct into an
// [IdentifierItem] output struct. Returns nil if id is nil.
func identifierToItem(id *gqlIdentifier) *IdentifierItem {
	if id == nil {
		return nil
	}
	return &IdentifierItem{
		Name:         id.Name,
		ExternalType: id.ExternalType,
		ExternalID:   id.ExternalID,
		URL:          id.URL,
	}
}

// List.

// ListInput is the input for listing project vulnerabilities.
type ListInput struct {
	ProjectPath   string   `json:"project_path" jsonschema:"Full path of the project (e.g. my-group/my-project),required"`
	Severity      []string `json:"severity,omitempty" jsonschema:"Filter by severity: CRITICAL, HIGH, MEDIUM, LOW, INFO, UNKNOWN"`
	State         []string `json:"state,omitempty" jsonschema:"Filter by state: DETECTED, CONFIRMED, DISMISSED, RESOLVED"`
	Scanner       []string `json:"scanner,omitempty" jsonschema:"Filter by scanner external IDs"`
	ReportType    []string `json:"report_type,omitempty" jsonschema:"Filter by report type: SAST, DAST, DEPENDENCY_SCANNING, CONTAINER_SCANNING, SECRET_DETECTION, COVERAGE_FUZZING, API_FUZZING, CLUSTER_IMAGE_SCANNING"`
	HasIssues     *bool    `json:"has_issues,omitempty" jsonschema:"Filter by whether a linked issue exists"`
	HasResolution *bool    `json:"has_resolution,omitempty" jsonschema:"Filter by whether a resolution exists"`
	Sort          string   `json:"sort,omitempty" jsonschema:"Sort order: severity_desc, severity_asc, detected_desc, detected_asc"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing project vulnerabilities.
type ListOutput struct {
	toolutil.HintableOutput
	Vulnerabilities []Item                           `json:"vulnerabilities"`
	Pagination      toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves project vulnerabilities via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectPath == "" {
		return ListOutput{}, toolutil.ErrRequiredString("list_vulnerabilities", "project_path")
	}

	vars := toolutil.MergeVariables(
		input.GraphQLPaginationInput.Variables(),
		map[string]any{"projectPath": input.ProjectPath},
	)
	if len(input.Severity) > 0 {
		vars["severity"] = input.Severity
	}
	if len(input.State) > 0 {
		vars["state"] = input.State
	}
	if len(input.Scanner) > 0 {
		vars["scanner"] = input.Scanner
	}
	if len(input.ReportType) > 0 {
		vars["reportType"] = input.ReportType
	}
	if input.HasIssues != nil {
		vars["hasIssues"] = *input.HasIssues
	}
	if input.HasResolution != nil {
		vars["hasResolution"] = *input.HasResolution
	}
	if input.Sort != "" {
		vars["sort"] = input.Sort
	}

	var resp struct {
		Data struct {
			Project struct {
				Vulnerabilities struct {
					Nodes    []gqlVulnerabilityNode      `json:"nodes"`
					PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
				} `json:"vulnerabilities"`
			} `json:"project"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListVulnerabilities,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_vulnerabilities", err)
	}

	items := make([]Item, 0, len(resp.Data.Project.Vulnerabilities.Nodes))
	for _, n := range resp.Data.Project.Vulnerabilities.Nodes {
		items = append(items, nodeToItem(n))
	}

	return ListOutput{
		Vulnerabilities: items,
		Pagination:      toolutil.PageInfoToOutput(resp.Data.Project.Vulnerabilities.PageInfo),
	}, nil
}

// Get.

// GetInput is the input for getting a single vulnerability.
type GetInput struct {
	ID string `json:"id" jsonschema:"Vulnerability GID (e.g. gid://gitlab/Vulnerability/42),required"`
}

// GetOutput is the output for getting a single vulnerability.
type GetOutput struct {
	toolutil.HintableOutput
	Vulnerability Item `json:"vulnerability"`
}

// Get retrieves a single vulnerability by GID via the GitLab GraphQL API.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (GetOutput, error) {
	if input.ID == "" {
		return GetOutput{}, toolutil.ErrRequiredString("get_vulnerability", "id")
	}

	var resp struct {
		Data struct {
			Vulnerability gqlVulnerabilityNode `json:"vulnerability"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryGetVulnerability,
		Variables: map[string]any{"id": input.ID},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return GetOutput{}, toolutil.WrapErrWithMessage("get_vulnerability", err)
	}

	if resp.Data.Vulnerability.ID == "" {
		return GetOutput{}, fmt.Errorf("get_vulnerability: vulnerability %q not found", input.ID)
	}

	return GetOutput{Vulnerability: nodeToItem(resp.Data.Vulnerability)}, nil
}
