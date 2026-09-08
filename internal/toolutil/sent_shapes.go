package toolutil

import (
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// The shapes in this file are what GitLab sends on an object that the
// client-go struct decoding it does not carry, each read from the captured
// response beside the SDK's own decode (ADR-0021) and named after the Grape
// entity that renders it. A reader takes the capture of one request and
// decodes it into the shape; a list reader holds its count to the SDK's.
// Every gap here is recorded in docs/development/upstream-bugs.md.

// UserBasicOutput mirrors lib/api/entities/user_basic.rb, the user object
// GitLab renders on a note's author and resolver and on a runner's creator.
type UserBasicOutput struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	PublicEmail string `json:"public_email,omitempty"`
	Name        string `json:"name"`
	State       string `json:"state,omitempty"`
	Locked      bool   `json:"locked"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	WebURL      string `json:"web_url,omitempty"`
}

// KeyExtra is what lib/api/entities/ssh_key.rb sends on a key that
// client-go's Key does not carry: the expiry, the last use and the usage
// type, all sent on every key.
type KeyExtra struct {
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	UsageType  string     `json:"usage_type"`
}

// CapturedKey reads, off the captured answer to a request for one key, the
// fields client-go's Key does not model.
func CapturedKey(capture *gitlabclient.ResponseCapture) (KeyExtra, error) {
	var extra KeyExtra
	if err := capture.Decode(&extra); err != nil {
		return KeyExtra{}, err
	}
	return extra, nil
}

// RunnerExtra is what lib/api/entities/ci/runner.rb sends on a runner that
// client-go's Runner and RunnerDetails do not carry: when it was created,
// who created it, sent to a caller allowed to read that user, and the job
// execution status.
type RunnerExtra struct {
	CreatedAt          *time.Time       `json:"created_at"`
	CreatedBy          *UserBasicOutput `json:"created_by"`
	JobExecutionStatus string           `json:"job_execution_status"`
}

// CapturedRunner reads, off the captured answer to a request for one
// runner, the fields client-go's runner structs do not model.
func CapturedRunner(capture *gitlabclient.ResponseCapture) (RunnerExtra, error) {
	var extra RunnerExtra
	if err := capture.Decode(&extra); err != nil {
		return RunnerExtra{}, err
	}
	return extra, nil
}

// CapturedRunners reads the same off a list answer, one extra per runner in
// order, the count held to what the SDK decoded.
func CapturedRunners(capture *gitlabclient.ResponseCapture, decoded int) ([]RunnerExtra, error) {
	return capturedList[RunnerExtra](capture, decoded, "runners")
}

// LintJobOutput is one job of the jobs array a CI lint answer carries when
// include_jobs is set, built by lib/gitlab/ci/lint.rb from the processed
// configuration; client-go's ProjectLintResult does not carry the array. A
// static check carries only, except and needs as the configuration spelled
// them, so those three keep the JSON shape GitLab sent, and environment is
// the name on a pipeline simulation and the configuration on a static check.
type LintJobOutput struct {
	Name         string   `json:"name"`
	Stage        string   `json:"stage"`
	BeforeScript []string `json:"before_script"`
	Script       []string `json:"script"`
	AfterScript  []string `json:"after_script"`
	TagList      []string `json:"tag_list"`
	Only         any      `json:"only,omitempty"`
	Except       any      `json:"except,omitempty"`
	Environment  any      `json:"environment,omitempty"`
	When         string   `json:"when"`
	AllowFailure bool     `json:"allow_failure"`
	Needs        any      `json:"needs,omitempty"`
}

// LintExtra is what lib/api/entities/ci/lint/result.rb sends that
// client-go's ProjectLintResult does not carry.
type LintExtra struct {
	Jobs []LintJobOutput `json:"jobs"`
}

// CapturedLint reads, off the captured answer to a lint request, the jobs
// array client-go does not model.
func CapturedLint(capture *gitlabclient.ResponseCapture) (LintExtra, error) {
	var extra LintExtra
	if err := capture.Decode(&extra); err != nil {
		return LintExtra{}, err
	}
	return extra, nil
}

// LabelExtra is what lib/api/entities/label.rb sends on a label that
// client-go's Label and GroupLabel do not carry: the description rendered
// as HTML, sent on every label.
type LabelExtra struct {
	DescriptionHTML string `json:"description_html"`
}

// CapturedLabel reads, off the captured answer to a request for one label,
// the field client-go's label structs do not model.
func CapturedLabel(capture *gitlabclient.ResponseCapture) (LabelExtra, error) {
	var extra LabelExtra
	if err := capture.Decode(&extra); err != nil {
		return LabelExtra{}, err
	}
	return extra, nil
}

// CapturedLabels reads the same off a list answer, one extra per label in
// order, the count held to what the SDK decoded.
func CapturedLabels(capture *gitlabclient.ResponseCapture, decoded int) ([]LabelExtra, error) {
	return capturedList[LabelExtra](capture, decoded, "labels")
}

// PipelineExtra is what lib/api/entities/ci/pipeline.rb sends on a pipeline
// that client-go's Pipeline does not carry: whether it is archived, sent on
// every pipeline rendered whole.
type PipelineExtra struct {
	Archived bool `json:"archived"`
}

// CapturedPipeline reads, off the captured answer to a request for one
// pipeline, the field client-go's Pipeline does not model.
func CapturedPipeline(capture *gitlabclient.ResponseCapture) (PipelineExtra, error) {
	var extra PipelineExtra
	if err := capture.Decode(&extra); err != nil {
		return PipelineExtra{}, err
	}
	return extra, nil
}
