// graphql_queries.go defines optimized GraphQL aggregation queries that fetch
// rich context in a single request, replacing multiple sequential REST calls
// used by sampling tools.

package samplingtools

// queryMRContext fetches merge request details, discussions, approval state,
// and head pipeline status in a single GraphQL request. This replaces 3
// sequential REST calls (MR details + discussions + approval state).
const queryMRContext = `
query($projectPath: ID!, $mrIID: String!) {
  project(fullPath: $projectPath) {
    mergeRequest(iid: $mrIID) {
      iid
      title
      description
      state
      sourceBranch
      targetBranch
      mergeStatusEnum
      diffStatsSummary {
        additions
        deletions
        fileCount
      }
      approvedBy {
        nodes {
          username
        }
      }
      approved
      approvalsRequired
      headPipeline {
        status
        detailedStatus {
          text
          label
        }
      }
      discussions(first: 100) {
        nodes {
          notes {
            nodes {
              author {
                username
              }
              body
              createdAt
              system
              resolvable
              resolved
            }
          }
        }
      }
    }
  }
}
`

// queryIssueContext fetches issue details, notes, participants, time tracking,
// labels, assignees, milestone, and related MRs in a single GraphQL request.
// This replaces up to 6 sequential REST calls.
const queryIssueContext = `
query($projectPath: ID!, $issueIID: String!) {
  project(fullPath: $projectPath) {
    issue(iid: $issueIID) {
      iid
      title
      description
      state
      author {
        username
      }
      createdAt
      dueDate
      weight
      labels(first: 50) {
        nodes {
          title
        }
      }
      assignees(first: 20) {
        nodes {
          username
        }
      }
      milestone {
        title
        dueDate
      }
      humanTimeEstimate
      humanTotalTimeSpent
      participants(first: 50) {
        nodes {
          username
        }
      }
      notes(first: 100) {
        nodes {
          author {
            username
          }
          body
          createdAt
          system
          internal
        }
      }
      relatedMergeRequests(first: 20) {
        nodes {
          iid
          title
          state
          author {
            username
          }
        }
      }
    }
  }
}
`

// queryPipelineContext fetches pipeline details with stages and jobs in a single
// GraphQL request. Job traces are NOT available via GraphQL and must still be
// fetched via REST. This replaces 2 sequential REST calls (pipeline + job list).
const queryPipelineContext = `
query($projectPath: ID!, $pipelineIID: ID!) {
  project(fullPath: $projectPath) {
    pipeline(iid: $pipelineIID) {
      iid
      status
      ref
      sha
      duration
      source
      yamlErrors
      stages(first: 20) {
        nodes {
          name
          status
          jobs(first: 50) {
            nodes {
              name
              status
              stage {
                name
              }
              duration
              failureMessage
              webPath
            }
          }
        }
      }
    }
  }
}
`

// GraphQL response types for MR context query.

type gqlMRContextResp struct {
	Project struct {
		MergeRequest *gqlMRContext `json:"mergeRequest"`
	} `json:"project"`
}

type gqlMRContext struct {
	IID              string             `json:"iid"`
	Title            string             `json:"title"`
	Description      string             `json:"description"`
	State            string             `json:"state"`
	SourceBranch     string             `json:"sourceBranch"`
	TargetBranch     string             `json:"targetBranch"`
	MergeStatusEnum  string             `json:"mergeStatusEnum"`
	DiffStatsSummary *gqlDiffStats      `json:"diffStatsSummary"`
	ApprovedBy       gqlUsernameNodes   `json:"approvedBy"`
	Approved         bool               `json:"approved"`
	ApprovalsReq     int                `json:"approvalsRequired"`
	HeadPipeline     *gqlHeadPipeline   `json:"headPipeline"`
	Discussions      gqlDiscussionNodes `json:"discussions"`
}

type gqlDiffStats struct {
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
	FileCount int `json:"fileCount"`
}

type gqlUsernameNodes struct {
	Nodes []gqlUsername `json:"nodes"`
}

type gqlUsername struct {
	Username string `json:"username"`
}

type gqlHeadPipeline struct {
	Status         string           `json:"status"`
	DetailedStatus *gqlDetailStatus `json:"detailedStatus"`
}

type gqlDetailStatus struct {
	Text  string `json:"text"`
	Label string `json:"label"`
}

type gqlDiscussionNodes struct {
	Nodes []gqlDiscussion `json:"nodes"`
}

type gqlDiscussion struct {
	Notes gqlNoteNodes `json:"notes"`
}

type gqlNoteNodes struct {
	Nodes []gqlNote `json:"nodes"`
}

type gqlNote struct {
	Author     gqlUsername `json:"author"`
	Body       string      `json:"body"`
	CreatedAt  string      `json:"createdAt"`
	System     bool        `json:"system"`
	Resolvable bool        `json:"resolvable"`
	Resolved   bool        `json:"resolved"`
	Internal   bool        `json:"internal"`
}

// GraphQL response types for Issue context query.

type gqlIssueContextResp struct {
	Project struct {
		Issue *gqlIssueContext `json:"issue"`
	} `json:"project"`
}

type gqlIssueContext struct {
	IID                  string           `json:"iid"`
	Title                string           `json:"title"`
	Description          string           `json:"description"`
	State                string           `json:"state"`
	Author               gqlUsername      `json:"author"`
	CreatedAt            string           `json:"createdAt"`
	DueDate              string           `json:"dueDate"`
	Weight               int              `json:"weight"`
	Labels               gqlLabelNodes    `json:"labels"`
	Assignees            gqlUsernameNodes `json:"assignees"`
	Milestone            *gqlMilestone    `json:"milestone"`
	HumanTimeEstimate    string           `json:"humanTimeEstimate"`
	HumanTotalTimeSpent  string           `json:"humanTotalTimeSpent"`
	Participants         gqlUsernameNodes `json:"participants"`
	Notes                gqlNoteNodes     `json:"notes"`
	RelatedMergeRequests gqlRelatedMRs    `json:"relatedMergeRequests"`
}

type gqlLabelNodes struct {
	Nodes []gqlLabel `json:"nodes"`
}

type gqlLabel struct {
	Title string `json:"title"`
}

type gqlMilestone struct {
	Title   string `json:"title"`
	DueDate string `json:"dueDate"`
}

type gqlRelatedMRs struct {
	Nodes []gqlRelatedMR `json:"nodes"`
}

type gqlRelatedMR struct {
	IID    string      `json:"iid"`
	Title  string      `json:"title"`
	State  string      `json:"state"`
	Author gqlUsername `json:"author"`
}

// GraphQL response types for Pipeline context query.

type gqlPipelineContextResp struct {
	Project struct {
		Pipeline *gqlPipelineContext `json:"pipeline"`
	} `json:"project"`
}

type gqlPipelineContext struct {
	IID        string        `json:"iid"`
	Status     string        `json:"status"`
	Ref        string        `json:"ref"`
	SHA        string        `json:"sha"`
	Duration   *float64      `json:"duration"`
	Source     string        `json:"source"`
	YamlErrors string        `json:"yamlErrors"`
	Stages     gqlStageNodes `json:"stages"`
}

type gqlStageNodes struct {
	Nodes []gqlStage `json:"nodes"`
}

type gqlStage struct {
	Name   string      `json:"name"`
	Status string      `json:"status"`
	Jobs   gqlJobNodes `json:"jobs"`
}

type gqlJobNodes struct {
	Nodes []gqlJob `json:"nodes"`
}

type gqlJob struct {
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	Stage          *gqlStage `json:"stage"`
	Duration       *float64  `json:"duration"`
	FailureMessage string    `json:"failureMessage"`
	WebPath        string    `json:"webPath"`
}
