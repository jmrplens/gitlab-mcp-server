package pipelines

import (
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// infoExtra is what lib/api/entities/ci/pipeline.rb sends on a pipeline that
// client-go's PipelineInfo does not carry, read from the captured response
// beside the SDK's own decode (ADR-0021). The gap is recorded in
// docs/development/upstream-bugs.md.
//
// Three routes fill [Output] and they do not send the same thing. The two
// pipeline lists present `API::Entities::Ci::PipelineBasic`, ten keys, which
// is what `gl.PipelineInfo` models; `POST /projects/:id/merge_requests/:iid/pipelines`
// presents `API::Entities::Ci::Pipeline`, which inherits that basic entity and
// adds these twelve. Each is exposed with no condition, so the created
// pipeline carries all twelve and a list row carries none, which is why every
// field of [Output] filled from here is omitted when empty.
//
// `user` is `API::Entities::UserBasic` and is read into the shape that mirrors
// exactly that entity rather than into the SDK's `BasicUser`, which carries a
// `created_at` GitLab does not send here and lacks the `public_email` and
// `locked` it does.
type infoExtra struct {
	BeforeSHA      string                    `json:"before_sha"`
	Tag            bool                      `json:"tag"`
	YamlErrors     string                    `json:"yaml_errors"`
	User           *toolutil.UserBasicOutput `json:"user"`
	StartedAt      *time.Time                `json:"started_at"`
	FinishedAt     *time.Time                `json:"finished_at"`
	CommittedAt    *time.Time                `json:"committed_at"`
	Duration       int64                     `json:"duration"`
	QueuedDuration int64                     `json:"queued_duration"`
	Coverage       string                    `json:"coverage"`
	DetailedStatus *StatusOutput             `json:"detailed_status"`
	Archived       bool                      `json:"archived"`
}

// capturedInfo reads the keys client-go's PipelineInfo does not model off the
// answer to a request that returned one pipeline.
func capturedInfo(capture *gitlabclient.ResponseCapture) (infoExtra, error) {
	var extra infoExtra
	if err := capture.Decode(&extra); err != nil {
		return infoExtra{}, err
	}
	return extra, nil
}
