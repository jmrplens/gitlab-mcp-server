// Package securityfindings implements MCP tool handlers for GitLab pipeline
// security report findings using the GraphQL API. This replaces the deprecated
// REST vulnerability_findings endpoint with the GraphQL Pipeline.securityReportFindings query.
package securityfindings

import (
	"context"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// FindingItem represents a single security report finding from a pipeline scan.
type FindingItem struct {
	UUID        string           `json:"uuid"`
	Name        string           `json:"name"`
	Title       string           `json:"title,omitempty"`
	Severity    string           `json:"severity"`
	Confidence  string           `json:"confidence,omitempty"`
	ReportType  string           `json:"report_type"`
	Scanner     *ScannerItem     `json:"scanner,omitempty"`
	Description string           `json:"description,omitempty"`
	Solution    string           `json:"solution,omitempty"`
	Identifiers []IdentifierItem `json:"identifiers,omitempty"`
	Location    *LocationItem    `json:"location,omitempty"`
	State       string           `json:"state"`
	Evidence    *EvidenceItem    `json:"evidence,omitempty"`
	VulnID      string           `json:"vulnerability_id,omitempty"`
	VulnState   string           `json:"vulnerability_state,omitempty"`
}

// ScannerItem represents the scanner that produced the finding.
type ScannerItem struct {
	Name       string `json:"name"`
	Vendor     string `json:"vendor,omitempty"`
	ExternalID string `json:"external_id,omitempty"`
}

// IdentifierItem represents a finding identifier (CVE, CWE, OWASP, etc.).
type IdentifierItem struct {
	Name         string `json:"name"`
	ExternalType string `json:"external_type,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	URL          string `json:"url,omitempty"`
}

// LocationItem represents the code location where the finding was detected.
type LocationItem struct {
	File      string `json:"file,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	BlobPath  string `json:"blob_path,omitempty"`
}

// EvidenceItem holds supporting evidence for a finding.
type EvidenceItem struct {
	Source string `json:"source,omitempty"`
	Data   string `json:"data,omitempty"`
}

// GraphQL query for pipeline security report findings.
const queryListFindings = `
query($projectPath: ID!, $pipelineIID: ID!, $first: Int!, $after: String, $severity: [String!], $confidence: [String!], $scanner: [String!], $reportType: [String!]) {
  project(fullPath: $projectPath) {
    pipeline(iid: $pipelineIID) {
      securityReportFindings(
        first: $first
        after: $after
        severity: $severity
        confidence: $confidence
        scanner: $scanner
        reportType: $reportType
      ) {
        nodes {
          uuid
          name
          title
          severity
          confidence
          reportType
          scanner {
            name
            vendor
            externalId
          }
          description
          solution
          identifiers {
            name
            externalType
            externalId
            url
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
          state
          evidence
          vulnerability {
            id
            state
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
}
`

// GraphQL response structs (camelCase to match API).

type gqlScanner struct {
	Name       string `json:"name"`
	Vendor     string `json:"vendor"`
	ExternalID string `json:"externalId"`
}

type gqlIdentifier struct {
	Name         string `json:"name"`
	ExternalType string `json:"externalType"`
	ExternalID   string `json:"externalId"`
	URL          string `json:"url"`
}

type gqlLocation struct {
	File      string `json:"file"`
	Path      string `json:"path"`
	Image     string `json:"image"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	BlobPath  string `json:"blobPath"`
}

// gqlVulnerabilityRef holds a reference to a vulnerability.
type gqlVulnerabilityRef struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type gqlFindingNode struct {
	UUID          string               `json:"uuid"`
	Name          string               `json:"name"`
	Title         string               `json:"title"`
	Severity      string               `json:"severity"`
	Confidence    string               `json:"confidence"`
	ReportType    string               `json:"reportType"`
	Scanner       *gqlScanner          `json:"scanner"`
	Description   string               `json:"description"`
	Solution      string               `json:"solution"`
	Identifiers   []gqlIdentifier      `json:"identifiers"`
	Location      *gqlLocation         `json:"location"`
	State         string               `json:"state"`
	Evidence      string               `json:"evidence"`
	Vulnerability *gqlVulnerabilityRef `json:"vulnerability"`
}

// gqlFindingsConnection holds the paginated list of security finding nodes.
type gqlFindingsConnection struct {
	Nodes    []gqlFindingNode            `json:"nodes"`
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
}

// gqlPipelineFindings wraps the security report findings inside a pipeline.
type gqlPipelineFindings struct {
	SecurityReportFindings gqlFindingsConnection `json:"securityReportFindings"`
}

// gqlProjectPipeline wraps the pipeline inside a project.
type gqlProjectPipeline struct {
	Pipeline gqlPipelineFindings `json:"pipeline"`
}

// nodeToItem converts a raw GraphQL security finding node into a [FindingItem]
// output struct, mapping scanner, identifiers, location, and vulnerability state.
func nodeToItem(n gqlFindingNode) FindingItem {
	item := FindingItem{
		UUID:        n.UUID,
		Name:        n.Name,
		Title:       n.Title,
		Severity:    n.Severity,
		Confidence:  n.Confidence,
		ReportType:  n.ReportType,
		Description: n.Description,
		Solution:    n.Solution,
		State:       n.State,
	}
	if n.Scanner != nil {
		item.Scanner = &ScannerItem{
			Name:       n.Scanner.Name,
			Vendor:     n.Scanner.Vendor,
			ExternalID: n.Scanner.ExternalID,
		}
	}
	for _, id := range n.Identifiers {
		item.Identifiers = append(item.Identifiers, IdentifierItem(id))
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
	if n.Evidence != "" {
		item.Evidence = &EvidenceItem{Data: n.Evidence}
	}
	if n.Vulnerability != nil {
		item.VulnID = n.Vulnerability.ID
		item.VulnState = n.Vulnerability.State
	}
	return item
}

// ListInput is the input for listing pipeline security report findings.
type ListInput struct {
	ProjectPath string   `json:"project_path" jsonschema:"Full path of the project (e.g. my-group/my-project),required"`
	PipelineIID string   `json:"pipeline_iid" jsonschema:"Pipeline IID within the project,required"`
	Severity    []string `json:"severity,omitempty" jsonschema:"Filter by severity: CRITICAL, HIGH, MEDIUM, LOW, INFO, UNKNOWN"`
	Confidence  []string `json:"confidence,omitempty" jsonschema:"Filter by confidence: CONFIRMED, MEDIUM, LOW"`
	Scanner     []string `json:"scanner,omitempty" jsonschema:"Filter by scanner external IDs"`
	ReportType  []string `json:"report_type,omitempty" jsonschema:"Filter by report type: SAST, DAST, DEPENDENCY_SCANNING, CONTAINER_SCANNING, SECRET_DETECTION, COVERAGE_FUZZING, API_FUZZING, CLUSTER_IMAGE_SCANNING"`
	toolutil.GraphQLPaginationInput
}

// ListOutput is the output for listing pipeline security report findings.
type ListOutput struct {
	toolutil.HintableOutput
	Findings   []FindingItem                    `json:"findings"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// List retrieves pipeline security report findings via the GitLab GraphQL API.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if input.ProjectPath == "" {
		return ListOutput{}, toolutil.ErrRequiredString("list_security_findings", "project_path")
	}
	if input.PipelineIID == "" {
		return ListOutput{}, toolutil.ErrRequiredString("list_security_findings", "pipeline_iid")
	}

	vars := toolutil.MergeVariables(
		input.GraphQLPaginationInput.Variables(),
		map[string]any{
			"projectPath": input.ProjectPath,
			"pipelineIID": input.PipelineIID,
		},
	)
	if len(input.Severity) > 0 {
		vars["severity"] = input.Severity
	}
	if len(input.Confidence) > 0 {
		vars["confidence"] = input.Confidence
	}
	if len(input.Scanner) > 0 {
		vars["scanner"] = input.Scanner
	}
	if len(input.ReportType) > 0 {
		vars["reportType"] = input.ReportType
	}

	var resp struct {
		Data struct {
			Project gqlProjectPipeline `json:"project"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListFindings,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_security_findings", err)
	}

	nodes := resp.Data.Project.Pipeline.SecurityReportFindings.Nodes
	items := make([]FindingItem, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, nodeToItem(n))
	}

	return ListOutput{
		Findings:   items,
		Pagination: toolutil.PageInfoToOutput(resp.Data.Project.Pipeline.SecurityReportFindings.PageInfo),
	}, nil
}
