package securityfindings

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// FindingItem represents a single security report finding from a pipeline scan.
//
// The dismissal fields are the state transition GitLab last recorded when it
// dismissed the vulnerability this finding became, read through the finding
// so a pipeline's findings say who dismissed each one and why without a call
// per finding. The finding's issue links and merge request are its
// vulnerability's too, and vulnerability.get publishes them for the
// vulnerability_id given here.
type FindingItem struct {
	UUID             string                                   `json:"uuid"`
	Title            string                                   `json:"title"`
	Severity         string                                   `json:"severity"`
	OriginalSeverity string                                   `json:"original_severity,omitempty"`
	ReportType       string                                   `json:"report_type"`
	Scanner          *ScannerItem                             `json:"scanner,omitempty"`
	Description      string                                   `json:"description,omitempty"`
	Solution         string                                   `json:"solution,omitempty"`
	Identifiers      []IdentifierItem                         `json:"identifiers,omitempty"`
	Location         *LocationItem                            `json:"location,omitempty"`
	State            string                                   `json:"state"`
	StateComment     string                                   `json:"state_comment,omitempty"`
	DismissedAt      string                                   `json:"dismissed_at,omitempty"`
	DismissedBy      *toolutil.UserCoreRefOutput              `json:"dismissed_by,omitempty"`
	DismissalReason  string                                   `json:"dismissal_reason,omitempty"`
	FalsePositive    *bool                                    `json:"false_positive,omitempty"`
	Unverified       bool                                     `json:"unverified,omitempty"`
	Evidence         *EvidenceItem                            `json:"evidence,omitempty"`
	Remediations     []RemediationItem                        `json:"remediations,omitempty"`
	Links            []toolutil.VulnerabilityLinkOutput       `json:"links,omitempty"`
	Assets           []AssetItem                              `json:"assets,omitempty"`
	TokenStatus      *toolutil.VulnerabilityTokenStatusOutput `json:"token_status,omitempty"`
	VulnID           string                                   `json:"vulnerability_id,omitempty"`
	VulnState        string                                   `json:"vulnerability_state,omitempty"`
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

// LocationItem is where the scanner found the finding. It is the shape the
// vulnerabilities share, so a finding and the vulnerability it becomes name
// their location in the same words.
type LocationItem = toolutil.VulnerabilityLocationOutput

// EvidenceItem holds supporting evidence for a finding. The GraphQL
// VulnerabilityEvidence type is an object rather than a blob, so the summary,
// the named source it points at, and the HTTP exchange a DAST or API fuzzing
// scan recorded as proof are carried separately.
type EvidenceItem struct {
	Summary            string                  `json:"summary,omitempty"`
	Source             string                  `json:"source,omitempty"`
	SourceID           string                  `json:"source_id,omitempty"`
	SourceURL          string                  `json:"source_url,omitempty"`
	Request            *HTTPRequestItem        `json:"request,omitempty"`
	Response           *HTTPResponseItem       `json:"response,omitempty"`
	SupportingMessages []SupportingMessageItem `json:"supporting_messages,omitempty"`
}

// HTTPHeaderItem is one header of a request or a response the evidence
// recorded.
type HTTPHeaderItem struct {
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// HTTPRequestItem is the request a scan sent to show the vulnerability.
type HTTPRequestItem struct {
	Method  string           `json:"method,omitempty"`
	URL     string           `json:"url,omitempty"`
	Headers []HTTPHeaderItem `json:"headers,omitempty"`
	Body    string           `json:"body,omitempty"`
}

// HTTPResponseItem is what the application answered that request with.
type HTTPResponseItem struct {
	StatusCode   int              `json:"status_code,omitempty"`
	ReasonPhrase string           `json:"reason_phrase,omitempty"`
	Headers      []HTTPHeaderItem `json:"headers,omitempty"`
	Body         string           `json:"body,omitempty"`
}

// SupportingMessageItem is a further exchange the scan recorded beside the
// evidence, such as the unmodified request it compared the attack with.
type SupportingMessageItem struct {
	Name     string            `json:"name"`
	Request  *HTTPRequestItem  `json:"request,omitempty"`
	Response *HTTPResponseItem `json:"response,omitempty"`
}

// RemediationItem is a fix the scanner proposed, with the patch that applies
// it when the scanner produced one.
type RemediationItem struct {
	Summary string `json:"summary,omitempty"`
	Diff    string `json:"diff,omitempty"`
}

// AssetItem is an artifact the scan attached to the finding, such as the
// recording of a DAST session.
type AssetItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// httpRequestSelection and httpResponseSelection are what an evidence request
// and response are read with, at the evidence itself and at each supporting
// message alike, since both are decoded into the same structs.
const (
	httpRequestSelection = `
              method
              url
              body
              headers {
                name
                value
              }
            `
	httpResponseSelection = `
              statusCode
              reasonPhrase
              body
              headers {
                name
                value
              }
            `
)

// GraphQL query for pipeline security report findings.
const queryListFindings = `
query($projectPath: ID!, $pipelineIID: ID!, $first: Int, $after: String, $last: Int, $before: String, $severity: [String!], $scanner: [String!], $reportType: [String!], $state: [VulnerabilityState!], $sort: PipelineSecurityReportFindingSort) {
  project(fullPath: $projectPath) {
    pipeline(iid: $pipelineIID) {
      securityReportFindings(
        first: $first
        after: $after
        last: $last
        before: $before
        severity: $severity
        scanner: $scanner
        reportType: $reportType
        state: $state
        sort: $sort
      ) {
        nodes {
          uuid
          title
          severity
          originalSeverity
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
          location {` + toolutil.VulnerabilityLocationSelection + `}
          state
          stateComment
          dismissedAt
          dismissedBy {` + toolutil.UserCoreRefSelection + `}
          dismissalReason
          falsePositive
          unverified
          evidence {
            summary
            source {
              identifier
              name
              url
            }
            request {` + httpRequestSelection + `}
            response {` + httpResponseSelection + `}
            supportingMessages {
              name
              request {` + httpRequestSelection + `}
              response {` + httpResponseSelection + `}
            }
          }
          remediations {
            summary
            diff
          }
          links {` + toolutil.VulnerabilityLinkSelection + `}
          assets {
            name
            type
            url
          }
          findingTokenStatus {` + toolutil.VulnerabilityTokenStatusSelection + `}
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

// lowercased spells filter values the way GitLab's finder looks them up.
func lowercased(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(strings.TrimSpace(v))
	}
	return out
}

// gqlVulnerabilityRef holds a reference to a vulnerability.
type gqlVulnerabilityRef struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

type gqlFindingNode struct {
	UUID               string                                    `json:"uuid"`
	Title              string                                    `json:"title"`
	Severity           string                                    `json:"severity"`
	OriginalSeverity   string                                    `json:"originalSeverity"`
	ReportType         string                                    `json:"reportType"`
	Scanner            *gqlScanner                               `json:"scanner"`
	Description        string                                    `json:"description"`
	Solution           string                                    `json:"solution"`
	Identifiers        []gqlIdentifier                           `json:"identifiers"`
	Location           *toolutil.GraphQLVulnerabilityLocation    `json:"location"`
	State              string                                    `json:"state"`
	StateComment       string                                    `json:"stateComment"`
	DismissedAt        string                                    `json:"dismissedAt"`
	DismissedBy        *toolutil.GraphQLUserCoreRef              `json:"dismissedBy"`
	DismissalReason    string                                    `json:"dismissalReason"`
	FalsePositive      *bool                                     `json:"falsePositive"`
	Unverified         bool                                      `json:"unverified"`
	Evidence           *gqlEvidence                              `json:"evidence"`
	Remediations       []gqlRemediation                          `json:"remediations"`
	Links              []toolutil.GraphQLVulnerabilityLink       `json:"links"`
	Assets             []gqlAsset                                `json:"assets"`
	FindingTokenStatus *toolutil.GraphQLVulnerabilityTokenStatus `json:"findingTokenStatus"`
	Vulnerability      *gqlVulnerabilityRef                      `json:"vulnerability"`
}

// gqlEvidence holds the supporting evidence object returned for a finding.
type gqlEvidence struct {
	Summary            string                 `json:"summary"`
	Source             *gqlEvidenceSource     `json:"source"`
	Request            *gqlRequest            `json:"request"`
	Response           *gqlResponse           `json:"response"`
	SupportingMessages []gqlSupportingMessage `json:"supportingMessages"`
}

// gqlEvidenceSource names where a piece of evidence came from.
type gqlEvidenceSource struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	URL        string `json:"url"`
}

// gqlHeader is one header of a recorded request or response.
type gqlHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// gqlRequest is a recorded request, read with [httpRequestSelection].
type gqlRequest struct {
	Method  string      `json:"method"`
	URL     string      `json:"url"`
	Body    string      `json:"body"`
	Headers []gqlHeader `json:"headers"`
}

// gqlResponse is a recorded response, read with [httpResponseSelection].
type gqlResponse struct {
	StatusCode   int         `json:"statusCode"`
	ReasonPhrase string      `json:"reasonPhrase"`
	Body         string      `json:"body"`
	Headers      []gqlHeader `json:"headers"`
}

// gqlSupportingMessage is a further exchange recorded beside the evidence.
type gqlSupportingMessage struct {
	Name     string       `json:"name"`
	Request  *gqlRequest  `json:"request"`
	Response *gqlResponse `json:"response"`
}

// gqlRemediation is a fix the scanner proposed.
type gqlRemediation struct {
	Summary string `json:"summary"`
	Diff    string `json:"diff"`
}

// gqlAsset is an artifact the scan attached to the finding.
type gqlAsset struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
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
	Pipeline *gqlPipelineFindings `json:"pipeline"`
}

// nodeToItem converts a raw GraphQL security finding node into a [FindingItem]
// output struct, mapping scanner, identifiers, location, evidence, and
// vulnerability state.
func nodeToItem(n gqlFindingNode) FindingItem {
	item := FindingItem{
		UUID:             n.UUID,
		Title:            n.Title,
		Severity:         n.Severity,
		OriginalSeverity: n.OriginalSeverity,
		ReportType:       n.ReportType,
		Description:      n.Description,
		Solution:         n.Solution,
		Location:         n.Location.Output(),
		State:            n.State,
		StateComment:     n.StateComment,
		DismissedAt:      n.DismissedAt,
		DismissedBy:      n.DismissedBy.Output(),
		DismissalReason:  n.DismissalReason,
		FalsePositive:    n.FalsePositive,
		Unverified:       n.Unverified,
		Evidence:         evidenceToItem(n.Evidence),
		Links:            toolutil.VulnerabilityLinkOutputs(n.Links),
		TokenStatus:      n.FindingTokenStatus.Output(),
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
	for _, remediation := range n.Remediations {
		item.Remediations = append(item.Remediations, RemediationItem(remediation))
	}
	for _, asset := range n.Assets {
		item.Assets = append(item.Assets, AssetItem(asset))
	}
	if n.Vulnerability != nil {
		item.VulnID = n.Vulnerability.ID
		item.VulnState = n.Vulnerability.State
	}
	return item
}

// evidenceToItem converts the evidence GitLab sent, and answers nil for
// evidence that carries nothing, which GitLab sends for a finding whose report
// recorded none: an empty object would read as evidence that says nothing.
func evidenceToItem(e *gqlEvidence) *EvidenceItem {
	if e == nil {
		return nil
	}
	evidence := &EvidenceItem{
		Summary:  e.Summary,
		Request:  requestToItem(e.Request),
		Response: responseToItem(e.Response),
	}
	if e.Source != nil {
		evidence.Source = e.Source.Name
		evidence.SourceID = e.Source.Identifier
		evidence.SourceURL = e.Source.URL
	}
	for _, message := range e.SupportingMessages {
		evidence.SupportingMessages = append(evidence.SupportingMessages, SupportingMessageItem{
			Name:     message.Name,
			Request:  requestToItem(message.Request),
			Response: responseToItem(message.Response),
		})
	}
	if reflect.ValueOf(evidence).Elem().IsZero() {
		return nil
	}
	return evidence
}

// requestToItem converts a recorded request, and answers nil for none.
func requestToItem(r *gqlRequest) *HTTPRequestItem {
	if r == nil {
		return nil
	}
	return &HTTPRequestItem{Method: r.Method, URL: r.URL, Headers: headersToItems(r.Headers), Body: r.Body}
}

// responseToItem converts a recorded response, and answers nil for none.
func responseToItem(r *gqlResponse) *HTTPResponseItem {
	if r == nil {
		return nil
	}
	return &HTTPResponseItem{StatusCode: r.StatusCode, ReasonPhrase: r.ReasonPhrase, Headers: headersToItems(r.Headers), Body: r.Body}
}

// headersToItems converts recorded headers, and answers nil for none so the
// field is left out rather than written as an empty list.
func headersToItems(headers []gqlHeader) []HTTPHeaderItem {
	if len(headers) == 0 {
		return nil
	}
	out := make([]HTTPHeaderItem, 0, len(headers))
	for _, header := range headers {
		out = append(out, HTTPHeaderItem(header))
	}
	return out
}

// ListInput is the input for listing pipeline security report findings.
type ListInput struct {
	ProjectPath string   `json:"project_path" jsonschema:"Full path of the project (e.g. my-group/my-project),required"`
	PipelineIID string   `json:"pipeline_iid" jsonschema:"Pipeline IID within the project,required"`
	Severity    []string `json:"severity,omitempty" jsonschema:"Filter by severity: CRITICAL, HIGH, MEDIUM, LOW, INFO, UNKNOWN"`
	Scanner     []string `json:"scanner,omitempty" jsonschema:"Filter by scanner external IDs"`
	ReportType  []string `json:"report_type,omitempty" jsonschema:"Filter by report type: SAST, DAST, DEPENDENCY_SCANNING, CONTAINER_SCANNING, SECRET_DETECTION, COVERAGE_FUZZING, API_FUZZING, CLUSTER_IMAGE_SCANNING"`
	State       []string `json:"state,omitempty" jsonschema:"Filter by state: DETECTED, CONFIRMED, DISMISSED, RESOLVED"`
	Sort        string   `json:"sort,omitempty" jsonschema:"Sort order: severity_desc (default) or severity_asc"`
	toolutil.GraphQLCursorPaginationInput
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

	pageVars, err := input.Variables(queryListFindings)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_security_findings: %w", err)
	}
	vars := toolutil.MergeVariables(
		pageVars,
		map[string]any{
			"projectPath": input.ProjectPath,
			"pipelineIID": input.PipelineIID,
		},
	)
	// severity and reportType are plain String arguments on this field, not
	// the enums the vulnerability queries take, and GitLab looks the values up
	// in its own lowercase enums (Security::Finding.severities.fetch_values,
	// Security::Scan.by_scan_types): CRITICAL or SAST as spelled everywhere
	// else in this server is a KeyError there and comes back as a 500.
	if len(input.Severity) > 0 {
		vars["severity"] = lowercased(input.Severity)
	}
	if len(input.Scanner) > 0 {
		vars["scanner"] = input.Scanner
	}
	if len(input.ReportType) > 0 {
		vars["reportType"] = lowercased(input.ReportType)
	}
	if len(input.State) > 0 {
		vars["state"] = input.State
	}
	if input.Sort != "" {
		vars["sort"] = input.Sort
	}

	var resp struct {
		Data struct {
			Project *gqlProjectPipeline `json:"project"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}

	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListFindings,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_security_findings", err, "verify the project fullPath and pipeline_iid are correct. Requires Ultimate license")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error. Reporting it here
	// matters more than elsewhere: the branch below answers a project that
	// resolves over REST with an empty page, so a refused query would be read
	// as a pipeline with no findings.
	if resp.Data.Project == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError("list_security_findings", resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
		if _, _, projectErr := client.GL().Projects.GetProject(input.ProjectPath, nil, gl.WithContext(ctx)); projectErr == nil {
			return ListOutput{Findings: []FindingItem{}}, nil
		}
		return ListOutput{}, fmt.Errorf("list_security_findings: project %q not found", input.ProjectPath)
	}
	if resp.Data.Project.Pipeline == nil {
		return ListOutput{}, fmt.Errorf(
			"list_security_findings: pipeline_iid %q not found in project %q. Suggestion: verify pipeline_iid with pipeline.list or pipeline.latest; security findings require a pipeline with security scan report artifacts",
			input.PipelineIID,
			input.ProjectPath,
		)
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
