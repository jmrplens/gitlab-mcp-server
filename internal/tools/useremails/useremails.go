// Package useremails implements GitLab email address management operations for users.
package useremails

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

const (
	errUserIDPositive  = "user_id must be a positive integer"
	errEmailIDPositive = "email_id must be a positive integer"
)

// Output represents an email address.
type Output struct {
	toolutil.HintableOutput
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
}

// ListOutput holds a list of emails.
type ListOutput struct {
	toolutil.HintableOutput
	Emails []Output `json:"emails"`
}

// DeleteOutput confirms an email deletion.
type DeleteOutput struct {
	EmailID int64 `json:"email_id"`
	Deleted bool  `json:"deleted"`
}

// --- Input types ---.

// ListForUserInput identifies a user for listing emails.
type ListForUserInput struct {
	UserID  int64 `json:"user_id" jsonschema:"GitLab user ID"`
	Page    int   `json:"page,omitempty" jsonschema:"Page number for pagination"`
	PerPage int   `json:"per_page,omitempty" jsonschema:"Items per page (max 100)"`
}

// GetInput identifies an email by ID.
type GetInput struct {
	EmailID int64 `json:"email_id" jsonschema:"Email ID to retrieve"`
}

// AddInput holds parameters for adding an email to the current user.
type AddInput struct {
	Email            string `json:"email" jsonschema:"Email address to add,required"`
	SkipConfirmation bool   `json:"skip_confirmation,omitempty" jsonschema:"Skip confirmation email (admin only)"`
}

// AddForUserInput holds parameters for adding an email to a specific user.
type AddForUserInput struct {
	UserID           int64  `json:"user_id" jsonschema:"GitLab user ID,required"`
	Email            string `json:"email" jsonschema:"Email address to add,required"`
	SkipConfirmation bool   `json:"skip_confirmation,omitempty" jsonschema:"Skip confirmation email (admin only)"`
}

// DeleteInput identifies an email to delete for the current user.
type DeleteInput struct {
	EmailID int64 `json:"email_id" jsonschema:"Email ID to delete,required"`
}

// DeleteForUserInput identifies a user's email to delete.
type DeleteForUserInput struct {
	UserID  int64 `json:"user_id" jsonschema:"GitLab user ID,required"`
	EmailID int64 `json:"email_id" jsonschema:"Email ID to delete,required"`
}

// --- Conversion helpers ---.

func toOutput(e *gl.Email) Output {
	o := Output{ID: e.ID, Email: e.Email}
	if e.ConfirmedAt != nil {
		o.ConfirmedAt = e.ConfirmedAt.Format("2006-01-02T15:04:05Z")
	}
	return o
}

func toOutputList(emails []*gl.Email) ListOutput {
	out := make([]Output, 0, len(emails))
	for _, e := range emails {
		out = append(out, toOutput(e))
	}
	return ListOutput{Emails: out}
}

// --- Handlers ---.

// ListForUser lists email addresses for a specific user.
func ListForUser(ctx context.Context, client *gitlabclient.Client, input ListForUserInput) (ListOutput, error) {
	if input.UserID <= 0 {
		return ListOutput{}, errors.New(errUserIDPositive)
	}
	opts := &gl.ListEmailsForUserOptions{}
	if input.Page > 0 {
		opts.Page = int64(input.Page)
	}
	if input.PerPage > 0 {
		opts.PerPage = int64(input.PerPage)
	}
	emails, _, err := client.GL().Users.ListEmailsForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, toolutil.WrapErrWithMessage("list_emails_for_user", err)
	}
	return toOutputList(emails), nil
}

// Get retrieves a single email by ID.
func Get(ctx context.Context, client *gitlabclient.Client, input GetInput) (Output, error) {
	if input.EmailID <= 0 {
		return Output{}, errors.New(errEmailIDPositive)
	}
	email, _, err := client.GL().Users.GetEmail(input.EmailID, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("get_email", err)
	}
	return toOutput(email), nil
}

// Add adds an email address to the current user.
func Add(ctx context.Context, client *gitlabclient.Client, input AddInput) (Output, error) {
	if input.Email == "" {
		return Output{}, toolutil.ErrFieldRequired("email")
	}
	opts := &gl.AddEmailOptions{
		Email: new(input.Email),
	}
	if input.SkipConfirmation {
		opts.SkipConfirmation = new(input.SkipConfirmation)
	}
	email, _, err := client.GL().Users.AddEmail(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("add_email", err)
	}
	return toOutput(email), nil
}

// AddForUser adds an email address to a specific user (admin only).
func AddForUser(ctx context.Context, client *gitlabclient.Client, input AddForUserInput) (Output, error) {
	if input.UserID <= 0 {
		return Output{}, errors.New(errUserIDPositive)
	}
	if input.Email == "" {
		return Output{}, toolutil.ErrFieldRequired("email")
	}
	opts := &gl.AddEmailOptions{
		Email: new(input.Email),
	}
	if input.SkipConfirmation {
		opts.SkipConfirmation = new(input.SkipConfirmation)
	}
	email, _, err := client.GL().Users.AddEmailForUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("add_email_for_user", err)
	}
	return toOutput(email), nil
}

// Delete deletes an email address from the current user.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if input.EmailID <= 0 {
		return DeleteOutput{}, errors.New(errEmailIDPositive)
	}
	_, err := client.GL().Users.DeleteEmail(input.EmailID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithMessage("delete_email", err)
	}
	return DeleteOutput{EmailID: input.EmailID, Deleted: true}, nil
}

// DeleteForUser deletes an email address from a specific user (admin only).
func DeleteForUser(ctx context.Context, client *gitlabclient.Client, input DeleteForUserInput) (DeleteOutput, error) {
	if input.UserID <= 0 {
		return DeleteOutput{}, errors.New(errUserIDPositive)
	}
	if input.EmailID <= 0 {
		return DeleteOutput{}, errors.New(errEmailIDPositive)
	}
	_, err := client.GL().Users.DeleteEmailForUser(input.UserID, input.EmailID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithMessage("delete_email_for_user", err)
	}
	return DeleteOutput{EmailID: input.EmailID, Deleted: true}, nil
}

// --- Markdown formatters ---.

// FormatListMarkdownString formats a list of emails as Markdown.
func FormatListMarkdownString(out ListOutput) string {
	if len(out.Emails) == 0 {
		return fmt.Sprintf("## Emails\n\n%s No emails found.\n", toolutil.EmojiWarning)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "## Emails (%d)\n\n", len(out.Emails))
	sb.WriteString("| ID | Email | Confirmed At |\n")
	sb.WriteString("|---|---|---|\n")
	for _, e := range out.Emails {
		confirmed := "-"
		if e.ConfirmedAt != "" {
			confirmed = e.ConfirmedAt
		}
		fmt.Fprintf(&sb, "| %d | %s | %s |\n", e.ID, e.Email, confirmed)
	}
	return sb.String()
}

// FormatMarkdownString formats a single email as Markdown.
func FormatMarkdownString(out Output) string {
	var sb strings.Builder
	sb.WriteString("## Email\n\n")
	fmt.Fprintf(&sb, toolutil.FmtMdID, out.ID)
	fmt.Fprintf(&sb, "- **Email**: %s\n", out.Email)
	if out.ConfirmedAt != "" {
		fmt.Fprintf(&sb, "- **Confirmed At**: %s\n", out.ConfirmedAt)
	}
	return sb.String()
}
