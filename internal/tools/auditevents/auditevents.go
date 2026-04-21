// Package auditevents implements MCP tool handlers for GitLab audit event
// operations including list and get at instance, group, and project levels.
// It wraps the AuditEvents service from client-go v2.
package auditevents

import (
	"context"
	"fmt"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// ListInstanceInput defines parameters for listing instance-level audit events.
type ListInstanceInput struct {
	CreatedAfter  string `json:"created_after,omitempty"  jsonschema:"Return events created after this date (ISO 8601 YYYY-MM-DD)"`
	CreatedBefore string `json:"created_before,omitempty" jsonschema:"Return events created before this date (ISO 8601 YYYY-MM-DD)"`
	toolutil.PaginationInput
}

// GetInstanceInput defines parameters for retrieving a single instance audit event.
type GetInstanceInput struct {
	EventID int64 `json:"event_id" jsonschema:"Audit event ID,required"`
}

// ListGroupInput defines parameters for listing group-level audit events.
type ListGroupInput struct {
	GroupID       toolutil.StringOrInt `json:"group_id"                 jsonschema:"Group ID or URL-encoded path,required"`
	CreatedAfter  string               `json:"created_after,omitempty"  jsonschema:"Return events created after this date (ISO 8601 YYYY-MM-DD)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return events created before this date (ISO 8601 YYYY-MM-DD)"`
	toolutil.PaginationInput
}

// GetGroupInput defines parameters for retrieving a single group audit event.
type GetGroupInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	EventID int64                `json:"event_id" jsonschema:"Audit event ID,required"`
}

// ListProjectInput defines parameters for listing project-level audit events.
type ListProjectInput struct {
	ProjectID     toolutil.StringOrInt `json:"project_id"               jsonschema:"Project ID or URL-encoded path,required"`
	CreatedAfter  string               `json:"created_after,omitempty"  jsonschema:"Return events created after this date (ISO 8601 YYYY-MM-DD)"`
	CreatedBefore string               `json:"created_before,omitempty" jsonschema:"Return events created before this date (ISO 8601 YYYY-MM-DD)"`
	toolutil.PaginationInput
}

// GetProjectInput defines parameters for retrieving a single project audit event.
type GetProjectInput struct {
	ProjectID toolutil.StringOrInt `json:"project_id" jsonschema:"Project ID or URL-encoded path,required"`
	EventID   int64                `json:"event_id"   jsonschema:"Audit event ID,required"`
}

// DetailsOutput represents the details of an audit event.
type DetailsOutput struct {
	CustomMessage string `json:"custom_message,omitempty"`
	AuthorName    string `json:"author_name,omitempty"`
	TargetID      string `json:"target_id,omitempty"`
	TargetType    string `json:"target_type,omitempty"`
	TargetDetails string `json:"target_details,omitempty"`
	IPAddress     string `json:"ip_address,omitempty"`
	EntityPath    string `json:"entity_path,omitempty"`
}

// Output represents a single audit event.
type Output struct {
	toolutil.HintableOutput
	ID         int64         `json:"id"`
	AuthorID   int64         `json:"author_id"`
	EntityID   int64         `json:"entity_id"`
	EntityType string        `json:"entity_type"`
	EventName  string        `json:"event_name"`
	EventType  string        `json:"event_type"`
	Details    DetailsOutput `json:"details"`
	CreatedAt  string        `json:"created_at"`
}

// ListOutput holds a paginated list of audit events.
type ListOutput struct {
	toolutil.HintableOutput
	AuditEvents []Output                  `json:"audit_events"`
	Pagination  toolutil.PaginationOutput `json:"pagination"`
}

func toOutput(e *gl.AuditEvent) Output {
	o := Output{
		ID:         e.ID,
		AuthorID:   e.AuthorID,
		EntityID:   e.EntityID,
		EntityType: e.EntityType,
		EventName:  e.EventName,
		EventType:  e.EventType,
	}
	if e.CreatedAt != nil {
		o.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	if e.Details.AuthorName != "" {
		o.Details.AuthorName = e.Details.AuthorName
	}
	if e.Details.TargetType != "" {
		o.Details.TargetType = e.Details.TargetType
	}
	if e.Details.TargetDetails != "" {
		o.Details.TargetDetails = e.Details.TargetDetails
	}
	if e.Details.CustomMessage != "" {
		o.Details.CustomMessage = e.Details.CustomMessage
	}
	if e.Details.IPAddress != "" {
		o.Details.IPAddress = e.Details.IPAddress
	}
	if e.Details.EntityPath != "" {
		o.Details.EntityPath = e.Details.EntityPath
	}
	// TargetID is any in the SDK
	if e.Details.TargetID != nil {
		o.Details.TargetID = fmt.Sprintf("%v", e.Details.TargetID)
	}
	return o
}

func buildListOpts(after, before string, pag toolutil.PaginationInput) *gl.ListAuditEventsOptions {
	opts := &gl.ListAuditEventsOptions{}
	if after != "" {
		if t, err := time.Parse("2006-01-02", after); err == nil {
			opts.CreatedAfter = &t
		}
	}
	if before != "" {
		if t, err := time.Parse("2006-01-02", before); err == nil {
			opts.CreatedBefore = &t
		}
	}
	if pag.Page > 0 {
		opts.Page = int64(pag.Page)
	}
	if pag.PerPage > 0 {
		opts.PerPage = int64(pag.PerPage)
	}
	return opts
}

// ListInstance lists instance-level audit events (admin only).
func ListInstance(ctx context.Context, client *gitlabclient.Client, input ListInstanceInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	opts := buildListOpts(input.CreatedAfter, input.CreatedBefore, input.PaginationInput)
	events, resp, err := client.GL().AuditEvents.ListInstanceAuditEvents(opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("auditListInstance", err)
	}
	out := make([]Output, len(events))
	for i, e := range events {
		out[i] = toOutput(e)
	}
	return ListOutput{AuditEvents: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetInstance retrieves a single instance-level audit event.
func GetInstance(ctx context.Context, client *gitlabclient.Client, input GetInstanceInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.EventID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("auditGetInstance", "event_id")
	}
	e, _, err := client.GL().AuditEvents.GetInstanceAuditEvent(input.EventID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("auditGetInstance", err)
	}
	return toOutput(e), nil
}

// ListGroup lists group-level audit events.
func ListGroup(ctx context.Context, client *gitlabclient.Client, input ListGroupInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.GroupID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("group_id")
	}
	opts := buildListOpts(input.CreatedAfter, input.CreatedBefore, input.PaginationInput)
	events, resp, err := client.GL().AuditEvents.ListGroupAuditEvents(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("auditListGroup", err)
	}
	out := make([]Output, len(events))
	for i, e := range events {
		out[i] = toOutput(e)
	}
	return ListOutput{AuditEvents: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetGroup retrieves a single group-level audit event.
func GetGroup(ctx context.Context, client *gitlabclient.Client, input GetGroupInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.GroupID == "" {
		return Output{}, toolutil.ErrFieldRequired("group_id")
	}
	if input.EventID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("auditGetGroup", "event_id")
	}
	e, _, err := client.GL().AuditEvents.GetGroupAuditEvent(string(input.GroupID), input.EventID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("auditGetGroup", err)
	}
	return toOutput(e), nil
}

// ListProject lists project-level audit events.
func ListProject(ctx context.Context, client *gitlabclient.Client, input ListProjectInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	if input.ProjectID == "" {
		return ListOutput{}, toolutil.ErrFieldRequired("project_id")
	}
	opts := buildListOpts(input.CreatedAfter, input.CreatedBefore, input.PaginationInput)
	events, resp, err := client.GL().AuditEvents.ListProjectAuditEvents(string(input.ProjectID), opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("auditListProject", err)
	}
	out := make([]Output, len(events))
	for i, e := range events {
		out[i] = toOutput(e)
	}
	return ListOutput{AuditEvents: out, Pagination: toolutil.PaginationFromResponse(resp)}, nil
}

// GetProject retrieves a single project-level audit event.
func GetProject(ctx context.Context, client *gitlabclient.Client, input GetProjectInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.ProjectID == "" {
		return Output{}, toolutil.ErrFieldRequired("project_id")
	}
	if input.EventID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64("auditGetProject", "event_id")
	}
	e, _, err := client.GL().AuditEvents.GetProjectAuditEvent(string(input.ProjectID), input.EventID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("auditGetProject", err)
	}
	return toOutput(e), nil
}
