package vulnerabilities

import (
	"context"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// IdentifierItem represents a vulnerability identifier (CVE, CWE, etc.).
type IdentifierItem struct {
	Name         string `json:"name"`
	ExternalType string `json:"external_type,omitempty"`
	ExternalID   string `json:"external_id,omitempty"`
	URL          string `json:"url,omitempty"`
}

// ScannerItem represents the scanner that detected the vulnerability.
// ScannerID is the id the report gives the scanner ("semgrep", "zap"), which
// is GitLab's externalId and what the list action's scanner filter takes.
type ScannerItem struct {
	Name      string `json:"name"`
	Vendor    string `json:"vendor,omitempty"`
	ScannerID string `json:"scanner_id,omitempty"`
}

// LocationItem is where the scanner found the vulnerability. It is the shape
// the security findings share, so a finding and the vulnerability it becomes
// name their location in the same words.
type LocationItem = toolutil.VulnerabilityLocationOutput

// ProjectItem represents a minimal project reference on a vulnerability.
type ProjectItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"full_path"`
}

// CVSSItem is one CVSS assessment of the vulnerability, as the vendor that
// scored it published it. A vulnerability can carry several, one per vendor
// and CVSS version.
type CVSSItem struct {
	Vendor       string  `json:"vendor"`
	Version      float64 `json:"version"`
	Vector       string  `json:"vector"`
	BaseScore    float64 `json:"base_score"`
	OverallScore float64 `json:"overall_score"`
	Severity     string  `json:"severity"`
}

// CVEEnrichmentItem is what GitLab knows about the CVE beyond the scanner's
// report: the EPSS probability of exploitation in the next thirty days, and
// whether CISA lists it as exploited in the wild.
type CVEEnrichmentItem struct {
	CVE            string  `json:"cve"`
	EPSSScore      float64 `json:"epss_score"`
	IsKnownExploit bool    `json:"is_known_exploit"`
}

// ReferenceItem names an issue or a merge request GitLab links to a
// vulnerability: enough to recognize it and to open it. Its own fields are the
// issue and merge request actions' to publish.
type ReferenceItem struct {
	IID    int64  `json:"iid"`
	Title  string `json:"title,omitempty"`
	State  string `json:"state,omitempty"`
	WebURL string `json:"web_url,omitempty"`
}

// IssueLinkItem is one issue linked to the vulnerability, and whether the issue
// was created from it (CREATED) or linked to it afterwards (RELATED).
type IssueLinkItem struct {
	LinkType string         `json:"link_type"`
	Issue    *ReferenceItem `json:"issue,omitempty"`
}

// Item is a summary of a vulnerability.
//
// The three people GitLab records against a state change are published beside
// the time of it, and the triage signals it computes (present on the default
// branch, resolved there, removed from the code) beside the state.
// present_on_default_branch is written at false as well as true, since GitLab
// always sends it and false is the answer that matters: the vulnerability was
// found only on another branch.
type Item struct {
	ID                      string                                   `json:"id"`
	UUID                    string                                   `json:"uuid,omitempty"`
	Title                   string                                   `json:"title"`
	Severity                string                                   `json:"severity"`
	State                   string                                   `json:"state"`
	StateComment            string                                   `json:"state_comment,omitempty"`
	Description             string                                   `json:"description,omitempty"`
	ReportType              string                                   `json:"report_type,omitempty"`
	Scanner                 *ScannerItem                             `json:"scanner,omitempty"`
	Location                *LocationItem                            `json:"location,omitempty"`
	Identifiers             []IdentifierItem                         `json:"identifiers,omitempty"`
	CVSS                    []CVSSItem                               `json:"cvss,omitempty"`
	CVEEnrichment           *CVEEnrichmentItem                       `json:"cve_enrichment,omitempty"`
	Links                   []toolutil.VulnerabilityLinkOutput       `json:"links,omitempty"`
	TokenStatus             *toolutil.VulnerabilityTokenStatusOutput `json:"token_status,omitempty"`
	DetectedAt              string                                   `json:"detected_at,omitempty"`
	UpdatedAt               string                                   `json:"updated_at,omitempty"`
	DismissedAt             string                                   `json:"dismissed_at,omitempty"`
	DismissedBy             *toolutil.UserCoreRefOutput              `json:"dismissed_by,omitempty"`
	ResolvedAt              string                                   `json:"resolved_at,omitempty"`
	ResolvedBy              *toolutil.UserCoreRefOutput              `json:"resolved_by,omitempty"`
	ConfirmedAt             string                                   `json:"confirmed_at,omitempty"`
	ConfirmedBy             *toolutil.UserCoreRefOutput              `json:"confirmed_by,omitempty"`
	Project                 *ProjectItem                             `json:"project,omitempty"`
	WebURL                  string                                   `json:"web_url,omitempty"`
	PrimaryID               *IdentifierItem                          `json:"primary_identifier,omitempty"`
	Solution                string                                   `json:"solution,omitempty"`
	HasRemediations         bool                                     `json:"has_remediations,omitempty"`
	FalsePositive           *bool                                    `json:"false_positive,omitempty"`
	PresentOnDefaultBranch  bool                                     `json:"present_on_default_branch"`
	ResolvedOnDefaultBranch bool                                     `json:"resolved_on_default_branch,omitempty"`
	RemovedFromCode         bool                                     `json:"removed_from_code,omitempty"`
	UserNotesCount          int                                      `json:"user_notes_count,omitempty"`
	HasIssues               bool                                     `json:"has_issues,omitempty"`
	IssueLinks              []IssueLinkItem                          `json:"issue_links,omitempty"`
	HasMR                   bool                                     `json:"has_merge_request,omitempty"`
	MergeRequest            *ReferenceItem                           `json:"merge_request,omitempty"`
	DismissalReason         string                                   `json:"dismissal_reason,omitempty"`
}

// GraphQL queries.

// referenceSelection is what an issue or a merge request linked to a
// vulnerability is read with, decoded into [gqlReference].
const referenceSelection = `
        iid
        title
        state
        webUrl
      `

// vulnFields is the selection every vulnerability document shares. The list,
// the get and the four state mutations all answer with a vulnerability node
// decoded into [gqlVulnerabilityNode], so the one selection is written once:
// a document selecting less left the struct's other fields empty, and the
// output claimed an empty description and no identifiers for a vulnerability
// that has both. make check-graphql-shapes refuses that shape.
//
// GitLab refuses a whole document that names a field it does not have, so the
// fields here decide which GitLab releases these six actions work on at all,
// and two kinds are left out on purpose, each named in
// cmd/audit_graphql_shapes/sent_declarations_security.go:
//
//   - a field GitLab's GraphQL reference marks Status: Experiment, which GitLab
//     may change or remove without notice (dueDate among them);
//   - a field newer than GitLab 18.10, the oldest release this selection is
//     served by. The newest field selected is removedFromCode, added in 18.10;
//     unverified, added in 18.11, would stop all six actions on every 18.10
//     instance for one boolean. Measured against GitLab's versioned GraphQL
//     references, the selection before issue 967 was served back to 16.11 at
//     least, and what moved the floor is, newest first: removedFromCode
//     (18.10), findingTokenStatus.lastVerifiedAt (18.5), the location's
//     dependency package path (18.3), findingTokenStatus (18.1),
//     cveEnrichment.isKnownExploit (17.7), cveEnrichment (17.6) and
//     containerRepositoryUrl (17.4).
//
// The list sends this selection for up to 100 nodes, and GitLab refuses a
// query whose complexity exceeds 250 for an authenticated caller, which every
// call this server makes is (AUTHENTICATED_MAX_COMPLEXITY in GitLab's
// app/graphql/gitlab_schema.rb). Measured on GitLab.com (19.5-pre),
// the list at first=100 costs 235, against at most 200 before issue 967, so a
// field added here is measured against that ceiling first: nothing in the
// unit suite can see it.
const vulnFields = `
    id
    uuid
    title
    severity
    state
    stateComment
    webUrl
    description
    reportType
    detectedAt
    updatedAt
    dismissedAt
    dismissedBy {` + toolutil.UserCoreRefSelection + `}
    resolvedAt
    resolvedBy {` + toolutil.UserCoreRefSelection + `}
    confirmedAt
    confirmedBy {` + toolutil.UserCoreRefSelection + `}
    solution
    hasRemediations
    dismissalReason
    falsePositive
    presentOnDefaultBranch
    resolvedOnDefaultBranch
    removedFromCode
    userNotesCount
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
    cvss {
      vendor
      version
      vector
      baseScore
      overallScore
      severity
    }
    cveEnrichment {
      cve
      epssScore
      isKnownExploit
    }
    links {` + toolutil.VulnerabilityLinkSelection + `}
    findingTokenStatus {` + toolutil.VulnerabilityTokenStatusSelection + `}
    scanner {
      name
      vendor
      externalId
    }
    location {` + toolutil.VulnerabilityLocationSelection + `}
    project {
      id
      name
      fullPath
    }
    issueLinks {
      nodes {
        linkType
        issue {` + referenceSelection + `}
      }
    }
    mergeRequest {` + referenceSelection + `}
`

const queryListVulnerabilities = `
query($projectPath: ID!, $first: Int, $after: String, $last: Int, $before: String, $severity: [VulnerabilitySeverity!], $state: [VulnerabilityState!], $scanner: [String!], $reportType: [VulnerabilityReportType!], $hasIssues: Boolean, $hasResolution: Boolean, $sort: VulnerabilitySort) {
  project(fullPath: $projectPath) {
    vulnerabilities(first: $first, after: $after, last: $last, before: $before, severity: $severity, state: $state, scanner: $scanner, reportType: $reportType, hasIssues: $hasIssues, hasResolution: $hasResolution, sort: $sort) {
      nodes {` + vulnFields + `
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
  vulnerability(id: $id) {` + vulnFields + `
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

// gqlScanner reads the scanner GitLab reports the finding under. externalId is
// selected because [ScannerItem] publishes it as scanner_id and no document
// asked for it, so the field was declared and always empty.
type gqlScanner struct {
	Name       string `json:"name"`
	Vendor     string `json:"vendor"`
	ExternalID string `json:"externalId"`
}

type gqlProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	FullPath string `json:"fullPath"`
}

// gqlCVSS reads one CvssType. GitLab types the scores and the version as
// Float, which a CVSS version of 3.1 and a score of 9.8 need.
type gqlCVSS struct {
	Vendor       string  `json:"vendor"`
	Version      float64 `json:"version"`
	Vector       string  `json:"vector"`
	BaseScore    float64 `json:"baseScore"`
	OverallScore float64 `json:"overallScore"`
	Severity     string  `json:"severity"`
}

// gqlCVEEnrichment reads the CveEnrichmentType GitLab attaches to a
// vulnerability identified by a CVE, and sends null for any other.
type gqlCVEEnrichment struct {
	CVE            string  `json:"cve"`
	EPSSScore      float64 `json:"epssScore"`
	IsKnownExploit bool    `json:"isKnownExploit"`
}

// gqlReference reads an issue or a merge request [referenceSelection] names.
// GitLab types iid as String on both.
type gqlReference struct {
	IID    string `json:"iid"`
	Title  string `json:"title"`
	State  string `json:"state"`
	WebURL string `json:"webUrl"`
}

// gqlIssueLinkNode holds one link between the vulnerability and an issue.
type gqlIssueLinkNode struct {
	LinkType string        `json:"linkType"`
	Issue    *gqlReference `json:"issue"`
}

// gqlIssueLinksConnection holds a list of issue link nodes.
type gqlIssueLinksConnection struct {
	Nodes []gqlIssueLinkNode `json:"nodes"`
}

type gqlVulnerabilityNode struct {
	ID                      string                                    `json:"id"`
	UUID                    string                                    `json:"uuid"`
	Title                   string                                    `json:"title"`
	Severity                string                                    `json:"severity"`
	State                   string                                    `json:"state"`
	StateComment            string                                    `json:"stateComment"`
	Description             string                                    `json:"description"`
	ReportType              string                                    `json:"reportType"`
	WebURL                  string                                    `json:"webUrl"`
	DetectedAt              string                                    `json:"detectedAt"`
	UpdatedAt               string                                    `json:"updatedAt"`
	DismissedAt             string                                    `json:"dismissedAt"`
	DismissedBy             *toolutil.GraphQLUserCoreRef              `json:"dismissedBy"`
	ResolvedAt              string                                    `json:"resolvedAt"`
	ResolvedBy              *toolutil.GraphQLUserCoreRef              `json:"resolvedBy"`
	ConfirmedAt             string                                    `json:"confirmedAt"`
	ConfirmedBy             *toolutil.GraphQLUserCoreRef              `json:"confirmedBy"`
	Solution                string                                    `json:"solution"`
	HasRemediations         bool                                      `json:"hasRemediations"`
	DismissalReason         string                                    `json:"dismissalReason"`
	FalsePositive           *bool                                     `json:"falsePositive"`
	PresentOnDefaultBranch  bool                                      `json:"presentOnDefaultBranch"`
	ResolvedOnDefaultBranch bool                                      `json:"resolvedOnDefaultBranch"`
	RemovedFromCode         bool                                      `json:"removedFromCode"`
	UserNotesCount          int                                       `json:"userNotesCount"`
	PrimaryIdentifier       *gqlIdentifier                            `json:"primaryIdentifier"`
	Identifiers             []gqlIdentifier                           `json:"identifiers"`
	CVSS                    []gqlCVSS                                 `json:"cvss"`
	CVEEnrichment           *gqlCVEEnrichment                         `json:"cveEnrichment"`
	Links                   []toolutil.GraphQLVulnerabilityLink       `json:"links"`
	FindingTokenStatus      *toolutil.GraphQLVulnerabilityTokenStatus `json:"findingTokenStatus"`
	Scanner                 *gqlScanner                               `json:"scanner"`
	Location                *toolutil.GraphQLVulnerabilityLocation    `json:"location"`
	Project                 *gqlProject                               `json:"project"`
	IssueLinks              *gqlIssueLinksConnection                  `json:"issueLinks"`
	MergeRequest            *gqlReference                             `json:"mergeRequest"`
}

// gqlVulnerabilitiesConnection holds the paginated list of vulnerability nodes.
type gqlVulnerabilitiesConnection struct {
	Nodes    []gqlVulnerabilityNode      `json:"nodes"`
	PageInfo toolutil.GraphQLRawPageInfo `json:"pageInfo"`
}

// gqlProjectVulnerabilities wraps the vulnerabilities connection inside a project.
type gqlProjectVulnerabilities struct {
	Vulnerabilities gqlVulnerabilitiesConnection `json:"vulnerabilities"`
}

// nodeToItem converts a raw GraphQL vulnerability node into an [Item] output
// struct, mapping identifiers, scanner, location, project, issues, and MR fields.
func nodeToItem(n gqlVulnerabilityNode) Item {
	item := Item{
		ID:                      n.ID,
		UUID:                    n.UUID,
		Title:                   n.Title,
		Severity:                n.Severity,
		State:                   n.State,
		StateComment:            n.StateComment,
		Description:             n.Description,
		ReportType:              n.ReportType,
		WebURL:                  n.WebURL,
		DetectedAt:              n.DetectedAt,
		UpdatedAt:               n.UpdatedAt,
		DismissedAt:             n.DismissedAt,
		DismissedBy:             n.DismissedBy.Output(),
		ResolvedAt:              n.ResolvedAt,
		ResolvedBy:              n.ResolvedBy.Output(),
		ConfirmedAt:             n.ConfirmedAt,
		ConfirmedBy:             n.ConfirmedBy.Output(),
		Solution:                n.Solution,
		HasRemediations:         n.HasRemediations,
		DismissalReason:         n.DismissalReason,
		FalsePositive:           n.FalsePositive,
		PresentOnDefaultBranch:  n.PresentOnDefaultBranch,
		ResolvedOnDefaultBranch: n.ResolvedOnDefaultBranch,
		RemovedFromCode:         n.RemovedFromCode,
		UserNotesCount:          n.UserNotesCount,
		CVSS:                    cvssToItems(n.CVSS),
		CVEEnrichment:           cveEnrichmentToItem(n.CVEEnrichment),
		Links:                   toolutil.VulnerabilityLinkOutputs(n.Links),
		TokenStatus:             n.FindingTokenStatus.Output(),
		Location:                n.Location.Output(),
		MergeRequest:            referenceToItem(n.MergeRequest),
		HasMR:                   n.MergeRequest != nil,
	}
	if n.PrimaryIdentifier != nil {
		item.PrimaryID = identifierToItem(n.PrimaryIdentifier)
	}
	for _, id := range n.Identifiers {
		item.Identifiers = append(item.Identifiers, *identifierToItem(&id))
	}
	if n.Scanner != nil {
		item.Scanner = &ScannerItem{Name: n.Scanner.Name, Vendor: n.Scanner.Vendor, ScannerID: n.Scanner.ExternalID}
	}
	if n.Project != nil {
		item.Project = &ProjectItem{ID: n.Project.ID, Name: n.Project.Name, FullPath: n.Project.FullPath}
	}
	if n.IssueLinks != nil && len(n.IssueLinks.Nodes) > 0 {
		item.HasIssues = true
		item.IssueLinks = issueLinksToItems(n.IssueLinks.Nodes)
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

// cvssToItems converts the CVSS assessments, and answers nil for none so the
// field is left out rather than written as an empty list.
func cvssToItems(assessments []gqlCVSS) []CVSSItem {
	if len(assessments) == 0 {
		return nil
	}
	out := make([]CVSSItem, 0, len(assessments))
	for _, a := range assessments {
		out = append(out, CVSSItem(a))
	}
	return out
}

// cveEnrichmentToItem converts the CVE enrichment, and answers nil for a
// vulnerability GitLab has none for.
func cveEnrichmentToItem(e *gqlCVEEnrichment) *CVEEnrichmentItem {
	if e == nil {
		return nil
	}
	item := CVEEnrichmentItem(*e)
	return &item
}

// referenceToItem converts a linked issue or merge request, and answers nil
// for none.
func referenceToItem(r *gqlReference) *ReferenceItem {
	if r == nil {
		return nil
	}
	return &ReferenceItem{
		IID:    toolutil.GraphQLNumber(r.IID),
		Title:  r.Title,
		State:  r.State,
		WebURL: r.WebURL,
	}
}

// issueLinksToItems converts the issue links GitLab sent, one item per link
// whether or not the issue behind it is one the caller may read: GitLab sends
// such a link with a null issue, and the link still exists.
func issueLinksToItems(nodes []gqlIssueLinkNode) []IssueLinkItem {
	out := make([]IssueLinkItem, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, IssueLinkItem{LinkType: node.LinkType, Issue: referenceToItem(node.Issue)})
	}
	return out
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
	toolutil.GraphQLCursorPaginationInput
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

	pageVars, err := input.Variables(queryListVulnerabilities)
	if err != nil {
		return ListOutput{}, fmt.Errorf("list_vulnerabilities: %w", err)
	}
	vars := toolutil.MergeVariables(
		pageVars,
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
			Project *gqlProjectVulnerabilities `json:"project"`
		} `json:"data"`
		Errors []toolutil.GraphQLError `json:"errors"`
	}

	_, err = client.GL().GraphQL.Do(gl.GraphQLQuery{
		Query:     queryListVulnerabilities,
		Variables: vars,
	}, &resp, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithHint("list_vulnerabilities", err, "verify the project fullPath is correct and your token has access to security features")
	}

	// GitLab answers a rejected document with HTTP 200 and a top-level errors
	// array, which client-go does not turn into an error. Reporting it here
	// matters more than elsewhere: the branch below answers a project that
	// resolves over REST with an empty page, so a refused query would be read
	// as a project with no vulnerabilities.
	if resp.Data.Project == nil {
		if graphQLErr := toolutil.GraphQLTopLevelError("list_vulnerabilities", resp.Errors); graphQLErr != nil {
			return ListOutput{}, graphQLErr
		}
		if _, _, projectErr := client.GL().Projects.GetProject(input.ProjectPath, nil, gl.WithContext(ctx)); projectErr == nil {
			return ListOutput{Vulnerabilities: []Item{}}, nil
		}
		return ListOutput{}, fmt.Errorf("list_vulnerabilities: project %q not found", input.ProjectPath)
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
		return GetOutput{}, toolutil.WrapErrWithHint("get_vulnerability", err, "verify the vulnerability GID format: gid://gitlab/Vulnerability/<id>")
	}

	if resp.Data.Vulnerability.ID == "" {
		return GetOutput{}, fmt.Errorf("get_vulnerability: vulnerability %q not found", input.ID)
	}

	return GetOutput{Vulnerability: nodeToItem(resp.Data.Vulnerability)}, nil
}
