// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// work_item_widgets.go reads the inputs of a work item update that GitLab can
// be asked to remove, and makes a removal reach GitLab as the null its schema
// asks for, which client-go cannot send.

package toolutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/hashicorp/go-retryablehttp"
	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// WorkItemWidgetInputs are the inputs of a work item update that set a value
// GitLab can also be asked to remove: the milestone, parent and iteration a
// work item references, its start and due dates, and its health status.
//
// Each follows the clearing convention every update here follows (issue
// 1303): nil leaves the value alone, 0 removes a reference, "" removes a date
// or the health status, and anything else sets it.
type WorkItemWidgetInputs struct {
	MilestoneID  *int64
	ParentID     *int64
	IterationID  *int64
	StartDate    *string
	DueDate      *string
	HealthStatus *string
}

// workItemNull names one field of one widget of the workItemUpdate input that
// the update sends as null.
type workItemNull struct {
	widget string
	field  string
}

// ApplyWorkItemWidgets copies the inputs that set a value onto opts, refusing
// a negative reference and a date that does not parse, and returns the
// request option that sends each removal the inputs ask for, or nil when they
// ask for none.
//
// The option exists because client-go's UpdateWorkItemOptions cannot express
// a removal (register row 119): it wraps any reference, 0 included, into a
// global ID, so 0 reached GitLab as gid://gitlab/Milestone/0 and was refused
// as "No object found", and its widget fields are omitted when unset, so an
// empty date or health status sent nothing and the update answered success
// with the value still in place. GitLab removes each of these when the
// widget's field is null, which the option writes into the input client-go
// built, beside whatever else the update sets.
func ApplyWorkItemWidgets(opts *gl.UpdateWorkItemOptions, in WorkItemWidgetInputs) (gl.RequestOptionFunc, error) {
	var nulls []workItemNull
	references := []struct {
		input  string
		value  *int64
		dest   **int64
		remove workItemNull
	}{
		{"milestone_id", in.MilestoneID, &opts.MilestoneID, workItemNull{"milestoneWidget", "milestoneId"}},
		{"parent_id", in.ParentID, &opts.ParentID, workItemNull{"hierarchyWidget", "parentId"}},
		{"iteration_id", in.IterationID, &opts.IterationID, workItemNull{"iterationWidget", "iterationId"}},
	}
	// The checks below are separate ifs rather than tagless switches: a
	// coverage profile puts no block over a case expression, so mutation
	// testing could not tell whether a test reaches each condition.
	for _, ref := range references {
		if ref.value == nil {
			continue
		}
		if *ref.value < 0 {
			return nil, fmt.Errorf("%s must be the ID of the object to set, or 0 to remove it, got %d", ref.input, *ref.value)
		}
		if *ref.value == 0 {
			nulls = append(nulls, ref.remove)
			continue
		}
		*ref.dest = ref.value
	}
	dates := []struct {
		input  string
		value  *string
		dest   **gl.ISOTime
		remove workItemNull
	}{
		{"start_date", in.StartDate, &opts.StartDate, workItemNull{"startAndDueDateWidget", "startDate"}},
		{"due_date", in.DueDate, &opts.DueDate, workItemNull{"startAndDueDateWidget", "dueDate"}},
	}
	for _, date := range dates {
		if date.value == nil {
			continue
		}
		if *date.value == "" {
			nulls = append(nulls, date.remove)
			continue
		}
		parsed, err := ParseDateField(date.input, *date.value)
		if err != nil {
			return nil, err
		}
		*date.dest = parsed
	}
	if in.HealthStatus != nil {
		if *in.HealthStatus == "" {
			nulls = append(nulls, workItemNull{"healthStatusWidget", "healthStatus"})
		} else {
			opts.HealthStatus = in.HealthStatus
		}
	}
	if len(nulls) == 0 {
		return nil, nil //nolint:nilnil // no removal asked for: the update goes out as client-go builds it
	}
	return workItemNullOption(nulls), nil
}

// workItemNullOption returns the request option that writes null into each
// named widget field of the workItemUpdate input client-go has encoded.
//
// client-go hands the same options to the global-ID lookup it sends before
// the mutation, so a request whose variables carry no input object is left as
// it is. A body the option cannot read fails the request rather than letting
// the update go out without the removal it was asked for.
func workItemNullOption(nulls []workItemNull) gl.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		raw, err := req.BodyBytes()
		if err != nil {
			return fmt.Errorf("read the work item update to send its removals: %w", err)
		}
		if len(raw) == 0 {
			return nil
		}
		var body map[string]any
		decoder := json.NewDecoder(bytes.NewReader(raw))
		// Numbers stay as written, so a weight beside a removal is sent as
		// the caller gave it rather than through a float64.
		decoder.UseNumber()
		if err = decoder.Decode(&body); err != nil {
			return fmt.Errorf("decode the work item update to send its removals: %w", err)
		}
		variables, _ := body["variables"].(map[string]any)
		input, ok := variables["input"].(map[string]any)
		if !ok {
			return nil
		}
		for _, null := range nulls {
			widget, _ := input[null.widget].(map[string]any)
			if widget == nil {
				widget = map[string]any{}
				input[null.widget] = widget
			}
			widget[null.field] = nil
		}
		// A map of what encoding/json decoded marshals without error, and
		// SetBody takes a byte slice without one; both are joined rather than
		// branched on so that neither impossible failure is left untested.
		rewritten, marshalErr := json.Marshal(body)
		return errors.Join(marshalErr, req.SetBody(rewritten))
	}
}
