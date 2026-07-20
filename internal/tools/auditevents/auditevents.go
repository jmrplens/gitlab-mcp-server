package auditevents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
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

// ChangeEntry mirrors a single entry in the client-go AuditEventChange struct,
// i.e. one element of the plural "changes" array that some audit events emit.
type ChangeEntry struct {
	Change string `json:"change,omitempty" jsonschema:"The setting or attribute that was changed"`
	From   string `json:"from,omitempty"   jsonschema:"Previous value before the change"`
	To     string `json:"to,omitempty"     jsonschema:"New value after the change"`
}

// DetailsOutput represents the details of an audit event. The exact fields
// returned depend on the action being recorded; this mirrors every field of
// the client-go AuditEventDetails struct for 1:1 API fidelity.
type DetailsOutput struct {
	With          string        `json:"with,omitempty"`
	Add           string        `json:"add,omitempty"`
	As            string        `json:"as,omitempty"`
	Change        string        `json:"change,omitempty"`
	ChangeObject  any           `json:"change_object,omitempty" jsonschema:"Object form of the change field. Populated when the API returns change as a JSON object instead of a plain string (e.g. project_group_link_updated); holds the raw JSON object so the object-valued change is not lost"`
	Changes       []ChangeEntry `json:"changes,omitempty" jsonschema:"Plural list of changes emitted by some audit events; each entry records a changed setting with its previous (from) and new (to) values"`
	From          string        `json:"from,omitempty"`
	To            string        `json:"to,omitempty"`
	Remove        string        `json:"remove,omitempty"`
	CustomMessage string        `json:"custom_message,omitempty"`
	AuthorName    string        `json:"author_name,omitempty"`
	AuthorEmail   string        `json:"author_email,omitempty"`
	AuthorClass   string        `json:"author_class,omitempty"`
	TargetID      string        `json:"target_id,omitempty"`
	TargetType    string        `json:"target_type,omitempty"`
	TargetDetails string        `json:"target_details,omitempty"`
	IPAddress     string        `json:"ip_address,omitempty"`
	EntityPath    string        `json:"entity_path,omitempty"`
	FailedLogin   string        `json:"failed_login,omitempty"`
	EventName     string        `json:"event_name,omitempty"`
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
	o.Details.With = e.Details.With
	o.Details.Add = e.Details.Add
	o.Details.As = e.Details.As
	o.Details.Change = e.Details.Change
	if len(e.Details.ChangeObject) > 0 {
		// Decode the raw object-valued change into a generic value so the
		// model-facing schema is an open JSON value rather than a byte array.
		_ = json.Unmarshal(e.Details.ChangeObject, &o.Details.ChangeObject)
	}
	if len(e.Details.Changes) > 0 {
		o.Details.Changes = make([]ChangeEntry, len(e.Details.Changes))
		for i, c := range e.Details.Changes {
			o.Details.Changes[i] = ChangeEntry{Change: c.Change, From: c.From, To: c.To}
		}
	}
	o.Details.From = e.Details.From
	o.Details.To = e.Details.To
	o.Details.Remove = e.Details.Remove
	o.Details.CustomMessage = e.Details.CustomMessage
	o.Details.AuthorName = e.Details.AuthorName
	o.Details.AuthorEmail = e.Details.AuthorEmail
	o.Details.AuthorClass = e.Details.AuthorClass
	o.Details.TargetType = e.Details.TargetType
	o.Details.TargetDetails = e.Details.TargetDetails
	o.Details.IPAddress = e.Details.IPAddress
	o.Details.EntityPath = e.Details.EntityPath
	o.Details.FailedLogin = e.Details.FailedLogin
	o.Details.EventName = e.Details.EventName
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("auditListInstance", err, http.StatusForbidden,
			"requires administrator access; self-managed Premium/Ultimate only; created_after/created_before must be ISO 8601 dates; entity_type filters: User, Group, Project")
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
		return Output{}, toolutil.WrapErrWithStatusHint("auditGetInstance", err, http.StatusNotFound,
			"verify id with gitlab_list_instance_audit_events; admin-only on self-managed Premium/Ultimate")
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("auditListGroup", err, http.StatusForbidden,
			"requires Owner role + Premium/Ultimate; verify group_id with gitlab_group_list; created_after/created_before must be ISO 8601")
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
		return Output{}, toolutil.WrapErrWithStatusHint("auditGetGroup", err, http.StatusNotFound,
			"verify group_id + id combination with gitlab_list_group_audit_events; requires Owner + Premium/Ultimate")
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
		return ListOutput{}, toolutil.WrapErrWithStatusHint("auditListProject", err, http.StatusForbidden,
			"requires Maintainer role + Premium/Ultimate; verify project_id with gitlab_project_list; created_after/created_before must be ISO 8601")
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
		return Output{}, toolutil.WrapErrWithStatusHint("auditGetProject", err, http.StatusNotFound,
			"verify project_id + id combination with gitlab_list_project_audit_events; requires Maintainer + Premium/Ultimate")
	}
	return toOutput(e), nil
}
