package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// GraphQL queries for severity counts and pipeline security summary.

const querySeverityCount = `
query($projectPath: ID!) {
  project(fullPath: $projectPath) {
    vulnerabilitySeveritiesCount {
      critical
      high
      medium
      low
      info
      unknown
    }
  }
}
`

// summarySectionFields is what every scan type's section of a pipeline's
// security report summary is read with, decoded into [gqlScannerSummary]:
// the counts, the scans that ran for the type with what each reported going
// wrong, and the resources a DAST or API fuzzing scan requested. GitLab caps
// scannedResources at the first twenty, and the CSV path is where the rest are.
const summarySectionFields = `
          vulnerabilitiesCount
          scannedResourcesCount
          scannedResourcesCsvPath
          scans {
            nodes {
              name
              status
              errors
              warnings
            }
          }
          scannedResources {
            nodes {
              requestMethod
              url
            }
          }
`

const queryPipelineSecuritySummary = `
query($projectPath: ID!, $pipelineIID: ID!) {
  project(fullPath: $projectPath) {
    pipeline(iid: $pipelineIID) {
      securityReportSummary {
        sast {` + summarySectionFields + `}
        dast {` + summarySectionFields + `}
        dependencyScanning {` + summarySectionFields + `}
        containerScanning {` + summarySectionFields + `}
        secretDetection {` + summarySectionFields + `}
        coverageFuzzing {` + summarySectionFields + `}
        apiFuzzing {` + summarySectionFields + `}
        clusterImageScanning {` + summarySectionFields + `}
      }
    }
  }
}
`

// GraphQL response structs.

type gqlSeverityCount struct {
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Unknown  int `json:"unknown"`
}

type gqlScannerSummary struct {
	VulnerabilitiesCount    int                            `json:"vulnerabilitiesCount"`
	ScannedResourcesCount   int                            `json:"scannedResourcesCount"`
	ScannedResourcesCsvPath string                         `json:"scannedResourcesCsvPath"`
	Scans                   *gqlScanConnection             `json:"scans"`
	ScannedResources        *gqlScannedResourcesConnection `json:"scannedResources"`
}

// gqlScanConnection holds the scans that ran for one scan type.
type gqlScanConnection struct {
	Nodes []gqlScan `json:"nodes"`
}

// gqlScan is one security scan a pipeline ran: normally one per job that
// published a report of the type.
type gqlScan struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// gqlScannedResourcesConnection holds the resources a scan requested.
type gqlScannedResourcesConnection struct {
	Nodes []gqlScannedResource `json:"nodes"`
}

// gqlScannedResource is one resource a DAST or API fuzzing scan requested.
type gqlScannedResource struct {
	RequestMethod string `json:"requestMethod"`
	URL           string `json:"url"`
}

type gqlSecurityReportSummary struct {
	Sast                 *gqlScannerSummary `json:"sast"`
	Dast                 *gqlScannerSummary `json:"dast"`
	DependencyScanning   *gqlScannerSummary `json:"dependencyScanning"`
	ContainerScanning    *gqlScannerSummary `json:"containerScanning"`
	SecretDetection      *gqlScannerSummary `json:"secretDetection"`
	CoverageFuzzing      *gqlScannerSummary `json:"coverageFuzzing"`
	APIFuzzing           *gqlScannerSummary `json:"apiFuzzing"`
	ClusterImageScanning *gqlScannerSummary `json:"clusterImageScanning"`
}

// gqlSeverityCountProject wraps the severity count inside a project.
type gqlSeverityCountProject struct {
	VulnerabilitySeveritiesCount gqlSeverityCount `json:"vulnerabilitySeveritiesCount"`
}

// gqlSecurityPipeline wraps the security report summary inside a pipeline.
type gqlSecurityPipeline struct {
	SecurityReportSummary *gqlSecurityReportSummary `json:"securityReportSummary"`
}

// gqlSecurityProject wraps the pipeline inside a project for security summary.
type gqlSecurityProject struct {
	Pipeline *gqlSecurityPipeline `json:"pipeline"`
}

// Severity count types.

// SeverityCountInput is the input for retrieving vulnerability severity counts.
type SeverityCountInput struct {
	ProjectPath string `json:"project_path" jsonschema:"Full path of the project (e.g. my-group/my-project),required"`
}

// SeverityCountOutput contains vulnerability counts grouped by severity level.
type SeverityCountOutput struct {
	toolutil.HintableOutput
	Critical int `json:"critical"`
	High     int `json:"high"`
	Medium   int `json:"medium"`
	Low      int `json:"low"`
	Info     int `json:"info"`
	Unknown  int `json:"unknown"`
	Total    int `json:"total"`
}

// SeverityCount retrieves vulnerability severity counts for a project via GraphQL.
func SeverityCount(ctx context.Context, client *gitlabclient.Client, input SeverityCountInput) (SeverityCountOutput, error) {
	if input.ProjectPath == "" {
		return SeverityCountOutput{}, toolutil.ErrRequiredString("vulnerability_severity_count", "project_path")
	}

	var resp struct {
		Data struct {
			Project *gqlSeverityCountProject `json:"project"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     querySeverityCount,
		Variables: map[string]any{"projectPath": input.ProjectPath},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return SeverityCountOutput{}, toolutil.WrapErrWithHint("vulnerability_severity_count", err, "verify the project fullPath is correct and your token has access to security features")
	}

	if resp.Data.Project == nil {
		return SeverityCountOutput{}, fmt.Errorf("vulnerability_severity_count: project %q not found", input.ProjectPath)
	}

	c := resp.Data.Project.VulnerabilitySeveritiesCount
	return SeverityCountOutput{
		Critical: c.Critical,
		High:     c.High,
		Medium:   c.Medium,
		Low:      c.Low,
		Info:     c.Info,
		Unknown:  c.Unknown,
		Total:    c.Critical + c.High + c.Medium + c.Low + c.Info + c.Unknown,
	}, nil
}

// Pipeline security summary types.

// ScannerSummaryItem represents the security scan results from a single scanner type.
//
// Scans are what ran for the type and how each ended, with the errors and
// warnings the analyzer wrote into its report: a scan type that found nothing
// because its scan failed reads here as a failure rather than as a clean
// result. ScannedResources are the first twenty resources a DAST or API
// fuzzing scan requested, which is as many as GitLab sends; the CSV path
// downloads them all.
type ScannerSummaryItem struct {
	VulnerabilitiesCount    int                   `json:"vulnerabilities_count"`
	ScannedResourcesCount   int                   `json:"scanned_resources_count"`
	ScannedResourcesCsvPath string                `json:"scanned_resources_csv_path,omitempty"`
	Scans                   []ScanItem            `json:"scans,omitempty"`
	ScannedResources        []ScannedResourceItem `json:"scanned_resources,omitempty"`
}

// ScanItem is one security scan the pipeline ran for a scan type.
type ScanItem struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ScannedResourceItem is one resource a DAST or API fuzzing scan requested.
type ScannedResourceItem struct {
	RequestMethod string `json:"request_method,omitempty"`
	URL           string `json:"url"`
}

// PipelineSecuritySummaryInput is the input for retrieving a pipeline's security report summary.
type PipelineSecuritySummaryInput struct {
	ProjectPath string `json:"project_path" jsonschema:"Full path of the project (e.g. my-group/my-project),required"`
	PipelineIID string `json:"pipeline_iid" jsonschema:"Pipeline IID (internal ID within the project),required"`
}

// PipelineSecuritySummaryOutput contains scanner-level breakdown of a pipeline's security report.
type PipelineSecuritySummaryOutput struct {
	toolutil.HintableOutput
	Sast                 *ScannerSummaryItem `json:"sast,omitempty"`
	Dast                 *ScannerSummaryItem `json:"dast,omitempty"`
	DependencyScanning   *ScannerSummaryItem `json:"dependency_scanning,omitempty"`
	ContainerScanning    *ScannerSummaryItem `json:"container_scanning,omitempty"`
	SecretDetection      *ScannerSummaryItem `json:"secret_detection,omitempty"`
	CoverageFuzzing      *ScannerSummaryItem `json:"coverage_fuzzing,omitempty"`
	APIFuzzing           *ScannerSummaryItem `json:"api_fuzzing,omitempty"`
	ClusterImageScanning *ScannerSummaryItem `json:"cluster_image_scanning,omitempty"`
	TotalVulnerabilities int                 `json:"total_vulnerabilities"`
}

// PipelineSecuritySummary retrieves a pipeline's security report summary via GraphQL.
func PipelineSecuritySummary(ctx context.Context, client *gitlabclient.Client, input PipelineSecuritySummaryInput) (PipelineSecuritySummaryOutput, error) {
	if input.ProjectPath == "" {
		return PipelineSecuritySummaryOutput{}, toolutil.ErrRequiredString("pipeline_security_summary", "project_path")
	}
	if input.PipelineIID == "" {
		return PipelineSecuritySummaryOutput{}, toolutil.ErrRequiredString("pipeline_security_summary", "pipeline_iid")
	}

	var resp struct {
		Data struct {
			Project *gqlSecurityProject `json:"project"`
		} `json:"data"`
	}

	_, err := client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query: queryPipelineSecuritySummary,
		Variables: map[string]any{
			"projectPath": input.ProjectPath,
			"pipelineIID": input.PipelineIID,
		},
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return PipelineSecuritySummaryOutput{}, toolutil.WrapErrWithHint("pipeline_security_summary", err, "verify the project fullPath and pipeline_iid are correct")
	}

	if resp.Data.Project == nil {
		return PipelineSecuritySummaryOutput{}, fmt.Errorf("pipeline_security_summary: project %q not found", input.ProjectPath)
	}

	if resp.Data.Project.Pipeline == nil {
		return PipelineSecuritySummaryOutput{}, fmt.Errorf("pipeline_security_summary: pipeline %q not found in project %q", input.PipelineIID, input.ProjectPath)
	}

	summary := resp.Data.Project.Pipeline.SecurityReportSummary
	if summary == nil {
		return PipelineSecuritySummaryOutput{}, nil
	}

	out := PipelineSecuritySummaryOutput{}
	total := 0

	if summary.Sast != nil {
		out.Sast = gqlToScannerSummary(summary.Sast)
		total += summary.Sast.VulnerabilitiesCount
	}
	if summary.Dast != nil {
		out.Dast = gqlToScannerSummary(summary.Dast)
		total += summary.Dast.VulnerabilitiesCount
	}
	if summary.DependencyScanning != nil {
		out.DependencyScanning = gqlToScannerSummary(summary.DependencyScanning)
		total += summary.DependencyScanning.VulnerabilitiesCount
	}
	if summary.ContainerScanning != nil {
		out.ContainerScanning = gqlToScannerSummary(summary.ContainerScanning)
		total += summary.ContainerScanning.VulnerabilitiesCount
	}
	if summary.SecretDetection != nil {
		out.SecretDetection = gqlToScannerSummary(summary.SecretDetection)
		total += summary.SecretDetection.VulnerabilitiesCount
	}
	if summary.CoverageFuzzing != nil {
		out.CoverageFuzzing = gqlToScannerSummary(summary.CoverageFuzzing)
		total += summary.CoverageFuzzing.VulnerabilitiesCount
	}
	if summary.APIFuzzing != nil {
		out.APIFuzzing = gqlToScannerSummary(summary.APIFuzzing)
		total += summary.APIFuzzing.VulnerabilitiesCount
	}
	if summary.ClusterImageScanning != nil {
		out.ClusterImageScanning = gqlToScannerSummary(summary.ClusterImageScanning)
		total += summary.ClusterImageScanning.VulnerabilitiesCount
	}

	out.TotalVulnerabilities = total
	return out, nil
}

// gqlToScannerSummary converts a raw GraphQL scanner summary struct into a
// [ScannerSummaryItem] output struct.
func gqlToScannerSummary(s *gqlScannerSummary) *ScannerSummaryItem {
	item := &ScannerSummaryItem{
		VulnerabilitiesCount:    s.VulnerabilitiesCount,
		ScannedResourcesCount:   s.ScannedResourcesCount,
		ScannedResourcesCsvPath: s.ScannedResourcesCsvPath,
	}
	if s.Scans != nil {
		for _, scan := range s.Scans.Nodes {
			item.Scans = append(item.Scans, ScanItem(scan))
		}
	}
	if s.ScannedResources != nil {
		for _, resource := range s.ScannedResources.Nodes {
			item.ScannedResources = append(item.ScannedResources, ScannedResourceItem(resource))
		}
	}
	return item
}
