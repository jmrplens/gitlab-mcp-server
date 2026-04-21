// user_crud.go implements admin user CRUD operations: create, modify, delete.

package users

import (
	"context"
	"errors"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// CreateInput holds parameters for creating a new GitLab user (admin only).
type CreateInput struct {
	Email               string `json:"email" jsonschema:"The user email address,required"`
	Name                string `json:"name" jsonschema:"The user display name,required"`
	Username            string `json:"username" jsonschema:"The username,required"`
	Password            string `json:"password,omitempty" jsonschema:"The user password (required unless reset_password or force_random_password is set)"`
	ResetPassword       *bool  `json:"reset_password,omitempty" jsonschema:"Send a password reset email instead of setting password"`
	ForceRandomPassword *bool  `json:"force_random_password,omitempty" jsonschema:"Set a random password instead of requiring one"`
	SkipConfirmation    *bool  `json:"skip_confirmation,omitempty" jsonschema:"Skip confirmation email and activate user immediately"`
	Admin               *bool  `json:"admin,omitempty" jsonschema:"Grant admin privileges"`
	External            *bool  `json:"external,omitempty" jsonschema:"Mark user as external"`
	Bio                 string `json:"bio,omitempty" jsonschema:"User bio text"`
	Location            string `json:"location,omitempty" jsonschema:"User location"`
	JobTitle            string `json:"job_title,omitempty" jsonschema:"User job title"`
	Organization        string `json:"organization,omitempty" jsonschema:"User organization"`
	ProjectsLimit       *int64 `json:"projects_limit,omitempty" jsonschema:"Maximum number of projects the user can create"`
	Note                string `json:"note,omitempty" jsonschema:"Admin note about the user"`
}

// Create creates a new GitLab user (admin only).
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if input.Email == "" {
		return Output{}, errors.New("create_user: email is required")
	}
	if input.Name == "" {
		return Output{}, errors.New("create_user: name is required")
	}
	if input.Username == "" {
		return Output{}, errors.New("create_user: username is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	opts := &gl.CreateUserOptions{
		Email:    new(input.Email),
		Name:     new(input.Name),
		Username: new(input.Username),
	}
	if input.Password != "" {
		opts.Password = new(input.Password)
	}
	if input.ResetPassword != nil {
		opts.ResetPassword = input.ResetPassword
	}
	if input.ForceRandomPassword != nil {
		opts.ForceRandomPassword = input.ForceRandomPassword
	}
	if input.SkipConfirmation != nil {
		opts.SkipConfirmation = input.SkipConfirmation
	}
	if input.Admin != nil {
		opts.Admin = input.Admin
	}
	if input.External != nil {
		opts.External = input.External
	}
	if input.Bio != "" {
		opts.Bio = new(input.Bio)
	}
	if input.Location != "" {
		opts.Location = new(input.Location)
	}
	if input.JobTitle != "" {
		opts.JobTitle = new(input.JobTitle)
	}
	if input.Organization != "" {
		opts.Organization = new(input.Organization)
	}
	if input.ProjectsLimit != nil {
		opts.ProjectsLimit = input.ProjectsLimit
	}
	if input.Note != "" {
		opts.Note = new(input.Note)
	}

	u, _, err := client.GL().Users.CreateUser(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("create_user", err)
	}
	return toOutput(u), nil
}

// ModifyInput holds parameters for modifying an existing GitLab user (admin only).
type ModifyInput struct {
	UserID             int64  `json:"user_id" jsonschema:"The ID of the user to modify,required"`
	Email              string `json:"email,omitempty" jsonschema:"New email address"`
	Name               string `json:"name,omitempty" jsonschema:"New display name"`
	Username           string `json:"username,omitempty" jsonschema:"New username"`
	Password           string `json:"password,omitempty" jsonschema:"New password"`
	Admin              *bool  `json:"admin,omitempty" jsonschema:"Grant or revoke admin privileges"`
	External           *bool  `json:"external,omitempty" jsonschema:"Mark or unmark as external"`
	SkipReconfirmation *bool  `json:"skip_reconfirmation,omitempty" jsonschema:"Skip reconfirmation on email change"`
	Bio                string `json:"bio,omitempty" jsonschema:"New bio text"`
	Location           string `json:"location,omitempty" jsonschema:"New location"`
	JobTitle           string `json:"job_title,omitempty" jsonschema:"New job title"`
	Organization       string `json:"organization,omitempty" jsonschema:"New organization"`
	ProjectsLimit      *int64 `json:"projects_limit,omitempty" jsonschema:"New maximum projects limit"`
	Note               string `json:"note,omitempty" jsonschema:"New admin note"`
	PrivateProfile     *bool  `json:"private_profile,omitempty" jsonschema:"Set profile as private"`
	CanCreateGroup     *bool  `json:"can_create_group,omitempty" jsonschema:"Allow user to create groups"`
	Locked             *bool  `json:"locked,omitempty" jsonschema:"Lock or unlock the user account"`
}

// Modify modifies an existing GitLab user (admin only).
func Modify(ctx context.Context, client *gitlabclient.Client, input ModifyInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("modify_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	opts := &gl.ModifyUserOptions{}
	if input.Email != "" {
		opts.Email = new(input.Email)
	}
	if input.Name != "" {
		opts.Name = new(input.Name)
	}
	if input.Username != "" {
		opts.Username = new(input.Username)
	}
	if input.Password != "" {
		opts.Password = new(input.Password)
	}
	if input.Admin != nil {
		opts.Admin = input.Admin
	}
	if input.External != nil {
		opts.External = input.External
	}
	if input.SkipReconfirmation != nil {
		opts.SkipReconfirmation = input.SkipReconfirmation
	}
	if input.Bio != "" {
		opts.Bio = new(input.Bio)
	}
	if input.Location != "" {
		opts.Location = new(input.Location)
	}
	if input.JobTitle != "" {
		opts.JobTitle = new(input.JobTitle)
	}
	if input.Organization != "" {
		opts.Organization = new(input.Organization)
	}
	if input.ProjectsLimit != nil {
		opts.ProjectsLimit = input.ProjectsLimit
	}
	if input.Note != "" {
		opts.Note = new(input.Note)
	}
	if input.PrivateProfile != nil {
		opts.PrivateProfile = input.PrivateProfile
	}
	if input.CanCreateGroup != nil {
		opts.CanCreateGroup = input.CanCreateGroup
	}

	u, _, err := client.GL().Users.ModifyUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithMessage("modify_user", err)
	}
	return toOutput(u), nil
}

// DeleteInput holds parameters for deleting a GitLab user (admin only).
type DeleteInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user to delete,required"`
}

// DeleteOutput represents the result of deleting a user.
type DeleteOutput struct {
	UserID  int64 `json:"user_id"`
	Deleted bool  `json:"deleted"`
}

// Delete deletes a GitLab user (admin only).
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if input.UserID == 0 {
		return DeleteOutput{}, errors.New("delete_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}

	_, err := client.GL().Users.DeleteUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, toolutil.WrapErrWithMessage("delete_user", err)
	}
	return DeleteOutput{UserID: input.UserID, Deleted: true}, nil
}
