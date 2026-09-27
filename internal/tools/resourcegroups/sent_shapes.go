package resourcegroups

import (
	"fmt"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// upcomingJobExtra is what a job waiting on a resource group carries that
// client-go does not decode, read from the captured response beside the SDK's
// own decode (ADR-0021).
//
// lib/api/entities/ci/job_basic.rb exposes the job's pipeline with
// Entities::Ci::PipelineBasic, which sends ten keys with no condition, and
// client-go's JobPipeline models five of them: the ID, project, ref, SHA and
// status. The other five are below. The gap is recorded in
// docs/development/upstream-bugs.md.
//
// The pipeline is a value rather than a pointer because it is only read for a
// job whose SDK decode carries a pipeline, and a pipeline the SDK decoded is
// an object in the same bytes, so this decode has it too.
type upcomingJobExtra struct {
	Pipeline pipelineExtra `json:"pipeline"`
}

// pipelineExtra is the half of Entities::Ci::PipelineBasic that JobPipeline
// does not model: the pipeline's project-scoped number, what started it, when
// it was created and last changed, and its page.
type pipelineExtra struct {
	IID       int64      `json:"iid"`
	Source    string     `json:"source"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	WebURL    string     `json:"web_url"`
}

// capturedUpcomingJobs reads the extras off the answer to a request for a
// resource group's queue, one per job in order, the count held to what the SDK
// decoded so that no extra is ever paired with another job's row.
func capturedUpcomingJobs(capture *gitlabclient.ResponseCapture, decoded int) ([]upcomingJobExtra, error) {
	var extras []upcomingJobExtra
	if err := capture.Decode(&extras); err != nil {
		return nil, err
	}
	if len(extras) != decoded {
		return nil, fmt.Errorf("the captured answer holds %d jobs and the SDK decoded %d", len(extras), decoded)
	}
	return extras, nil
}
