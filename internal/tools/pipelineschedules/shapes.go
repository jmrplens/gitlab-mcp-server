package pipelineschedules

import (
	gitlab "gitlab.com/gitlab-org/api/client-go/v2"
)

// Canonical output shapes mirrored from client-go sub-objects. Per the 1:1
// audit policy (full nested objects) these surface every field of the SDK
// struct on the canonical json keys (owner, last_pipeline, variables, inputs)
// and are replicated here rather than imported from sibling packages to
// preserve the zero-import-cycle constraint (C-IMPORTS).

// OwnerOutput is a documented reference subset per
// doc/api/pipeline_schedules.md. It mirrors gl.User (the owner object embedded
// on a pipeline schedule) limited to the user-reference fields the official API
// documents for the `owner` reference: id, username, name, state, avatar_url,
// web_url. The full gl.User shape carries many unrelated account fields that the
// documented response does not include.
type OwnerOutput struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	State     string `json:"state"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}

// ownerOutput converts a gl.User to its output shape, returning nil when the
// SDK value is nil.
func ownerOutput(u *gitlab.User) *OwnerOutput {
	if u == nil {
		return nil
	}
	return &OwnerOutput{
		ID: u.ID, Username: u.Username, Name: u.Name, State: u.State,
		AvatarURL: u.AvatarURL, WebURL: u.WebURL,
	}
}

// LastPipelineOutput is a documented reference subset per
// doc/api/pipeline_schedules.md. It mirrors gl.LastPipeline (the last pipeline
// run by a schedule, returned only on the single-schedule get operation),
// limited to the fields the official API documents for the `last_pipeline`
// reference: id, sha, ref, status. The SDK's web_url field is intentionally
// omitted because the documented response does not include it.
type LastPipelineOutput struct {
	ID     int64  `json:"id"`
	SHA    string `json:"sha"`
	Ref    string `json:"ref"`
	Status string `json:"status"`
}

// lastPipelineOutput converts a gl.LastPipeline to its output shape, returning
// nil when the SDK value is nil.
func lastPipelineOutput(p *gitlab.LastPipeline) *LastPipelineOutput {
	if p == nil {
		return nil
	}
	return &LastPipelineOutput{
		ID: p.ID, SHA: p.SHA, Ref: p.Ref, Status: p.Status,
	}
}

// VariableObject is a documented reference subset per
// doc/api/pipeline_schedules.md. It mirrors gl.PipelineVariable, the variables
// embedded on a pipeline schedule payload (key, value, variable_type). The
// documented `variables[]` response also lists a `raw` boolean, but the SDK's
// gl.PipelineVariable does not expose it, so it cannot be surfaced without
// inventing a scalar.
type VariableObject struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
}

// variableObjects converts a slice of gl.PipelineVariable, skipping nil
// elements and returning nil for an empty or all-nil slice.
func variableObjects(vars []*gitlab.PipelineVariable) []VariableObject {
	if len(vars) == 0 {
		return nil
	}
	out := make([]VariableObject, 0, len(vars))
	for _, v := range vars {
		if v == nil {
			continue
		}
		out = append(out, VariableObject{
			Key: v.Key, Value: v.Value, VariableType: string(v.VariableType),
		})
	}
	return out
}

// InputObject mirrors gl.PipelineInput, the pipeline inputs embedded on a
// pipeline schedule payload and accepted on create/edit.
type InputObject struct {
	Name    string `json:"name"`
	Value   any    `json:"value,omitempty"`
	Destroy *bool  `json:"destroy,omitempty"`
}

// inputObjects converts a slice of gl.PipelineInput, skipping nil elements and
// returning nil for an empty or all-nil slice.
func inputObjects(inputs []*gitlab.PipelineInput) []InputObject {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]InputObject, 0, len(inputs))
	for _, in := range inputs {
		if in == nil {
			continue
		}
		out = append(out, InputObject{Name: in.Name, Value: in.Value, Destroy: in.Destroy})
	}
	return out
}

// toPipelineInputs converts the tool-facing InputObject slice to the SDK
// gl.PipelineInput slice for create/edit requests.
func toPipelineInputs(inputs []InputObject) []*gitlab.PipelineInput {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]*gitlab.PipelineInput, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, &gitlab.PipelineInput{
			Name: in.Name, Value: in.Value, Destroy: in.Destroy,
		})
	}
	return out
}
