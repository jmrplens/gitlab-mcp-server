package deploymentmergerequests

import (
	gl "gitlab.com/gitlab-org/api/client-go/v2"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct. The four pipeline-cluster shapes that are byte-identical across
// mergerequests / mergetrains / deploymentmergerequests (BasicUserOutput,
// PipelineOutput, PipelineDetailedStatusOutput,
// PipelineDetailedStatusIllustrationOutput) live in
// internal/toolutil since DEDUP-001 wave 3a; the rest remain local because
// they are package-specific.

// MilestoneOutput mirrors gl.Milestone (the merge-request milestone object).

// MilestoneOutput mirrors gl.Milestone (the merge-request milestone object).

// ReferencesOutput mirrors gl.IssueReferences (the merge-request references object).

// LabelDetailsOutput mirrors gl.LabelDetails.

// TaskCompletionStatusOutput mirrors gl.TasksCompletionStatus.

// TimeStatsOutput is the canonical pure time-tracking sub-object from toolutil.
// Aliased here so call sites within this package remain unchanged.
type TimeStatsOutput = toolutil.TimeStatsOutput

// timeStatsPtr converts a gl.TimeStats pointer to the canonical pure
// TimeStatsOutput, delegating to toolutil.NewTimeStatsOutput.
func timeStatsPtr(t *gl.TimeStats) *TimeStatsOutput {
	return toolutil.NewTimeStatsOutput(t)
}

// MergeRequestUserOutput mirrors gl.MergeRequestUser (the MR "user" object,
// describing the current user's relationship to the merge request).

// PipelineInfoOutput mirrors gl.PipelineInfo (the compact pipeline object on the
// merge-request "pipeline" key).

// PipelineDetailedStatusIllustrationOutput, PipelineDetailedStatusOutput,
// and PipelineOutput have moved to toolutil.* (DEDUP-001 wave 3a). The
// local converter helpers were also removed in favor of
// toolutil.NewPipelineDetailedStatusOutput, toolutil.NewPipelineOutput, and
// toolutil.NewBasicUserOutput.

// mrMilestoneOutputPtr converts a gl.Milestone pointer into the canonical
// MRMilestoneOutput. Package-local wrapper so the call site reads naturally.
func mrMilestoneOutputPtr(m *gl.Milestone) *toolutil.MRMilestoneOutput {
	out := toolutil.NewMRMilestoneOutputs([]*gl.Milestone{m})
	if len(out) == 0 {
		return nil
	}
	return out[0]
}

// mergeRequestUserOutputPtr converts a gl.MergeRequestUser value (passed
// by value in the SDK) into the canonical MergeRequestUserOutput pointer.
func mergeRequestUserOutputPtr(u gl.MergeRequestUser) *toolutil.MergeRequestUserOutput {
	return toolutil.NewMergeRequestUserOutput(&u)
}
