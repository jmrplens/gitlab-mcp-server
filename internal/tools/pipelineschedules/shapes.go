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
// embedded on a pipeline schedule payload (key, value, variable_type), plus the
// documented `raw` boolean that gl.PipelineVariable does not expose. The `raw`
// field is surfaced via a raw REST superset fetch (see rawScheduleAPI) rather
// than the SDK wrapper, and is omitted when the instance does not report it.
type VariableObject struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
	Raw          bool   `json:"raw,omitempty"`
}

// variableObjects converts a slice of gl.PipelineVariable, skipping nil
// elements and returning nil for an empty or all-nil slice. The SDK wrapper does
// not carry the documented `raw` field, so the raw-fetch path (variableObjectsAPI)
// is preferred wherever the documented `variables[]` response is returned.
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

// scheduleVariableAPI mirrors a single entry of the documented pipeline schedule
// `variables[]` response, including the `raw` boolean that the SDK
// gl.PipelineVariable omits. It is decoded from a raw REST superset fetch.
type scheduleVariableAPI struct {
	Key          string `json:"key"`
	Value        string `json:"value"`
	VariableType string `json:"variable_type"`
	Raw          bool   `json:"raw"`
}

// variableObjectsAPI converts the raw-fetch schedule variables, preserving the
// documented `raw` field. It skips null array elements and returns nil for an
// empty or all-null slice, mirroring the SDK-wrapper converter.
func variableObjectsAPI(vars []*scheduleVariableAPI) []VariableObject {
	if len(vars) == 0 {
		return nil
	}
	out := make([]VariableObject, 0, len(vars))
	for _, v := range vars {
		if v == nil {
			continue
		}
		out = append(out, VariableObject{
			Key: v.Key, Value: v.Value, VariableType: v.VariableType, Raw: v.Raw,
		})
	}
	return out
}

// InputObject mirrors gl.PipelineInput, the pipeline inputs embedded on a
// pipeline schedule payload and accepted on create/edit.
type InputObject struct {
	Name    string `json:"name"             jsonschema:"Pipeline input name (must match a defined input on the target pipeline)"`
	Value   any    `json:"value,omitempty"  jsonschema:"Pipeline input value; type depends on the input definition"`
	Destroy *bool  `json:"destroy,omitempty" jsonschema:"Set true to delete this input from the schedule (only honored on update)"`
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

// scheduleInputAPI mirrors a single documented `inputs[]` entry on a pipeline
// schedule response. It is decoded from the raw REST superset fetch alongside
// scheduleVariableAPI; the documented response carries no per-input `destroy`
// field, so only name and value are decoded.
type scheduleInputAPI struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// rawScheduleAPI is the documented superset of a single pipeline schedule
// response (doc/api/pipeline_schedules.md). It is decoded from a raw REST GET so
// that the `variables[].raw` boolean the SDK gl.PipelineSchedule omits can be
// surfaced. Timestamps are decoded as strings to avoid a redundant time parse:
// the canonical Output already carries them as RFC3339 strings.
type rawScheduleAPI struct {
	ID           int64                  `json:"id"`
	Description  string                 `json:"description"`
	Ref          string                 `json:"ref"`
	Cron         string                 `json:"cron"`
	CronTimezone string                 `json:"cron_timezone"`
	NextRunAt    string                 `json:"next_run_at"`
	Active       bool                   `json:"active"`
	CreatedAt    string                 `json:"created_at"`
	UpdatedAt    string                 `json:"updated_at"`
	Owner        *OwnerOutput           `json:"owner"`
	LastPipeline *LastPipelineOutput    `json:"last_pipeline"`
	Variables    []*scheduleVariableAPI `json:"variables"`
	Inputs       []*scheduleInputAPI    `json:"inputs"`
}

// inputObjectsAPI converts the raw-fetch schedule inputs into the canonical
// InputObject slice. It skips null array elements and returns nil for an empty
// or all-null slice, mirroring the SDK-wrapper converter.
func inputObjectsAPI(inputs []*scheduleInputAPI) []InputObject {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]InputObject, 0, len(inputs))
	for _, in := range inputs {
		if in == nil {
			continue
		}
		out = append(out, InputObject{Name: in.Name, Value: in.Value})
	}
	return out
}

// toOutputAPI maps a raw-fetch schedule superset into the canonical Output,
// preserving the documented `variables[].raw` field. Owner and last_pipeline are
// decoded directly into their canonical reference shapes.
func toOutputAPI(s *rawScheduleAPI) Output {
	return Output{
		ID:           int(s.ID),
		Description:  s.Description,
		Ref:          s.Ref,
		Cron:         s.Cron,
		CronTimezone: s.CronTimezone,
		NextRunAt:    s.NextRunAt,
		Active:       s.Active,
		CreatedAt:    s.CreatedAt,
		UpdatedAt:    s.UpdatedAt,
		Owner:        s.Owner,
		LastPipeline: s.LastPipeline,
		Variables:    variableObjectsAPI(s.Variables),
		Inputs:       inputObjectsAPI(s.Inputs),
	}
}
