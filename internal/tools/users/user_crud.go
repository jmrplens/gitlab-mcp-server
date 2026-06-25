package users

import (
	"context"
	"errors"
	"net/http"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// createUserForbiddenHint is the hint returned when CreateUser fails with 403.
// The string mentions password policy as part of GitLab's documented user-creation
// requirements; it is operator guidance, not a credential. The trailing NOSONAR
// suppresses go:S2068 (hard-coded credential heuristic).
const createUserForbiddenHint = "creating users requires admin token; email and username must be unique; password must meet instance complexity policy or use reset_password=true; required fields: email, name, username" // NOSONAR

// CreateInput holds parameters for creating a new GitLab user (admin only).
type CreateInput struct {
	Email                          string `json:"email" jsonschema:"The user email address,required"`
	Name                           string `json:"name" jsonschema:"The user display name,required"`
	Username                       string `json:"username" jsonschema:"The username,required"`
	Password                       string `json:"password,omitempty" jsonschema:"The user password (required unless reset_password or force_random_password is set)"`
	ResetPassword                  *bool  `json:"reset_password,omitempty" jsonschema:"Send a password reset email instead of setting password"`
	ForceRandomPassword            *bool  `json:"force_random_password,omitempty" jsonschema:"Set a random password instead of requiring one"`
	SkipConfirmation               *bool  `json:"skip_confirmation,omitempty" jsonschema:"Skip confirmation email and activate user immediately"`
	Admin                          *bool  `json:"admin,omitempty" jsonschema:"Grant admin privileges"`
	Auditor                        *bool  `json:"auditor,omitempty" jsonschema:"Grant auditor privileges (Premium/Ultimate)"`
	External                       *bool  `json:"external,omitempty" jsonschema:"Mark user as external"`
	CanCreateGroup                 *bool  `json:"can_create_group,omitempty" jsonschema:"Allow the user to create groups"`
	PrivateProfile                 *bool  `json:"private_profile,omitempty" jsonschema:"Make the user's profile private"`
	ViewDiffsFileByFile            *bool  `json:"view_diffs_file_by_file,omitempty" jsonschema:"Show whitespace changes in diffs file by file"`
	Bio                            string `json:"bio,omitempty" jsonschema:"User bio text"`
	Location                       string `json:"location,omitempty" jsonschema:"User location"`
	JobTitle                       string `json:"job_title,omitempty" jsonschema:"User job title"`
	Organization                   string `json:"organization,omitempty" jsonschema:"User organization"`
	Pronouns                       string `json:"pronouns,omitempty" jsonschema:"User pronouns"`
	CommitEmail                    string `json:"commit_email,omitempty" jsonschema:"Email used for commits"`
	PublicEmail                    string `json:"public_email,omitempty" jsonschema:"Publicly visible email address"`
	WebsiteURL                     string `json:"website_url,omitempty" jsonschema:"User website URL"`
	Linkedin                       string `json:"linkedin,omitempty" jsonschema:"LinkedIn account"`
	Twitter                        string `json:"twitter,omitempty" jsonschema:"Twitter/X account"`
	Skype                          string `json:"skype,omitempty" jsonschema:"Skype account"`
	Discord                        string `json:"discord,omitempty" jsonschema:"Discord account"`
	Github                         string `json:"github,omitempty" jsonschema:"GitHub account"`
	Provider                       string `json:"provider,omitempty" jsonschema:"External provider name (use with extern_uid)"`
	ExternUID                      string `json:"extern_uid,omitempty" jsonschema:"External UID for the provider"`
	GroupIDForSAML                 *int64 `json:"group_id_for_saml,omitempty" jsonschema:"Group ID for SAML provisioning"`
	ProjectsLimit                  *int64 `json:"projects_limit,omitempty" jsonschema:"Maximum number of projects the user can create"`
	ThemeID                        *int64 `json:"theme_id,omitempty" jsonschema:"GitLab theme ID for the user's UI"`
	ColorSchemeID                  *int64 `json:"color_scheme_id,omitempty" jsonschema:"Syntax-highlighting color scheme ID"`
	SharedRunnersMinutesLimit      *int64 `json:"shared_runners_minutes_limit,omitempty" jsonschema:"Shared runners minutes limit (admin, Premium/Ultimate)"`
	ExtraSharedRunnersMinutesLimit *int64 `json:"extra_shared_runners_minutes_limit,omitempty" jsonschema:"Extra shared runners minutes (admin, Premium/Ultimate)"`
	Note                           string `json:"note,omitempty" jsonschema:"Admin note about the user"`
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

	opts := buildCreateUserOptions(input)

	u, _, err := client.GL().Users.CreateUser(opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("create_user", err, http.StatusForbidden,
			createUserForbiddenHint)
	}
	return toOutput(u), nil
}

// ModifyInput holds parameters for modifying an existing GitLab user (admin only).
type ModifyInput struct {
	UserID              int64  `json:"user_id" jsonschema:"The ID of the user to modify,required"`
	Email               string `json:"email,omitempty" jsonschema:"New email address"`
	Name                string `json:"name,omitempty" jsonschema:"New display name"`
	Username            string `json:"username,omitempty" jsonschema:"New username"`
	Password            string `json:"password,omitempty" jsonschema:"New password"`
	Admin               *bool  `json:"admin,omitempty" jsonschema:"Grant or revoke admin privileges"`
	External            *bool  `json:"external,omitempty" jsonschema:"Mark or unmark as external"`
	SkipReconfirmation  *bool  `json:"skip_reconfirmation,omitempty" jsonschema:"Skip reconfirmation on email change"`
	Bio                 string `json:"bio,omitempty" jsonschema:"New bio text"`
	Location            string `json:"location,omitempty" jsonschema:"New location"`
	JobTitle            string `json:"job_title,omitempty" jsonschema:"New job title"`
	Organization        string `json:"organization,omitempty" jsonschema:"New organization"`
	ProjectsLimit       *int64 `json:"projects_limit,omitempty" jsonschema:"New maximum projects limit"`
	Note                string `json:"note,omitempty" jsonschema:"New admin note"`
	PrivateProfile      *bool  `json:"private_profile,omitempty" jsonschema:"Set profile as private"`
	CanCreateGroup      *bool  `json:"can_create_group,omitempty" jsonschema:"Allow user to create groups"`
	ViewDiffsFileByFile *bool  `json:"view_diffs_file_by_file,omitempty" jsonschema:"Show whitespace changes in diffs file by file"`
	CommitEmail         string `json:"commit_email,omitempty" jsonschema:"Email used for commits"`
	PublicEmail         string `json:"public_email,omitempty" jsonschema:"Publicly visible email address"`
	WebsiteURL          string `json:"website_url,omitempty" jsonschema:"User website URL"`
	Linkedin            string `json:"linkedin,omitempty" jsonschema:"LinkedIn account"`
	Twitter             string `json:"twitter,omitempty" jsonschema:"Twitter/X account"`
	Skype               string `json:"skype,omitempty" jsonschema:"Skype account"`
	Provider            string `json:"provider,omitempty" jsonschema:"External provider name (use with extern_uid)"`
	ExternUID           string `json:"extern_uid,omitempty" jsonschema:"External UID for the provider"`
	ThemeID             *int64 `json:"theme_id,omitempty" jsonschema:"GitLab theme ID for the user's UI"`
	Locked              *bool  `json:"locked,omitempty" jsonschema:"Lock or unlock the user account"`
}

// Modify modifies an existing GitLab user (admin only).
func Modify(ctx context.Context, client *gitlabclient.Client, input ModifyInput) (Output, error) {
	if input.UserID == 0 {
		return Output{}, errors.New("modify_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}

	opts := buildModifyUserOptions(input)

	u, _, err := client.GL().Users.ModifyUser(input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, toolutil.WrapErrWithStatusHint("modify_user", err, http.StatusForbidden,
			"modifying users requires admin token; verify user_id with gitlab_get_user; email/username changes must remain unique")
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
		return DeleteOutput{}, toolutil.WrapErrWithStatusHint("delete_user", err, http.StatusForbidden,
			"deleting users requires admin token; user_id must be a positive integer; use hard_delete=true to remove all contributions (default soft-deletes)")
	}
	return DeleteOutput{UserID: input.UserID, Deleted: true}, nil
}

// strPtr returns a pointer to v when v is non-empty, else nil. It keeps the
// option builders flat by collapsing the empty-string guard into one call.
func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// intPtr converts an optional *int64 input to the *int the client-go user
// options expect. It returns nil when the input is nil.
func intPtr(v *int64) *int {
	if v == nil {
		return nil
	}
	i := int(*v)
	return &i
}

// buildCreateUserOptions maps a CreateInput onto the client-go CreateUserOptions.
// All optional fields use omitempty pointers, so nil values are simply omitted
// from the request.
func buildCreateUserOptions(input CreateInput) *gl.CreateUserOptions {
	return &gl.CreateUserOptions{
		Email:                          new(input.Email),
		Name:                           new(input.Name),
		Username:                       new(input.Username),
		Password:                       strPtr(input.Password),
		ResetPassword:                  input.ResetPassword,
		ForceRandomPassword:            input.ForceRandomPassword,
		SkipConfirmation:               input.SkipConfirmation,
		Admin:                          input.Admin,
		Auditor:                        input.Auditor,
		External:                       input.External,
		CanCreateGroup:                 input.CanCreateGroup,
		PrivateProfile:                 input.PrivateProfile,
		ViewDiffsFileByFile:            input.ViewDiffsFileByFile,
		Bio:                            strPtr(input.Bio),
		Location:                       strPtr(input.Location),
		JobTitle:                       strPtr(input.JobTitle),
		Organization:                   strPtr(input.Organization),
		Pronouns:                       strPtr(input.Pronouns),
		CommitEmail:                    strPtr(input.CommitEmail),
		PublicEmail:                    strPtr(input.PublicEmail),
		WebsiteURL:                     strPtr(input.WebsiteURL),
		Linkedin:                       strPtr(input.Linkedin),
		Twitter:                        strPtr(input.Twitter),
		Skype:                          strPtr(input.Skype),
		Discord:                        strPtr(input.Discord),
		Github:                         strPtr(input.Github),
		Provider:                       strPtr(input.Provider),
		ExternUID:                      strPtr(input.ExternUID),
		Note:                           strPtr(input.Note),
		ProjectsLimit:                  input.ProjectsLimit,
		ThemeID:                        input.ThemeID,
		ColorSchemeID:                  intPtr(input.ColorSchemeID),
		GroupIDForSAML:                 intPtr(input.GroupIDForSAML),
		SharedRunnersMinutesLimit:      intPtr(input.SharedRunnersMinutesLimit),
		ExtraSharedRunnersMinutesLimit: intPtr(input.ExtraSharedRunnersMinutesLimit),
	}
}

// buildModifyUserOptions maps a ModifyInput onto the client-go ModifyUserOptions.
// All optional fields use omitempty pointers, so nil values are simply omitted
// from the request.
func buildModifyUserOptions(input ModifyInput) *gl.ModifyUserOptions {
	return &gl.ModifyUserOptions{
		Email:               strPtr(input.Email),
		Name:                strPtr(input.Name),
		Username:            strPtr(input.Username),
		Password:            strPtr(input.Password),
		Admin:               input.Admin,
		External:            input.External,
		SkipReconfirmation:  input.SkipReconfirmation,
		Bio:                 strPtr(input.Bio),
		Location:            strPtr(input.Location),
		JobTitle:            strPtr(input.JobTitle),
		Organization:        strPtr(input.Organization),
		ProjectsLimit:       input.ProjectsLimit,
		Note:                strPtr(input.Note),
		PrivateProfile:      input.PrivateProfile,
		CanCreateGroup:      input.CanCreateGroup,
		ViewDiffsFileByFile: input.ViewDiffsFileByFile,
		CommitEmail:         strPtr(input.CommitEmail),
		PublicEmail:         strPtr(input.PublicEmail),
		WebsiteURL:          strPtr(input.WebsiteURL),
		Linkedin:            strPtr(input.Linkedin),
		Twitter:             strPtr(input.Twitter),
		Skype:               strPtr(input.Skype),
		Provider:            strPtr(input.Provider),
		ExternUID:           strPtr(input.ExternUID),
		ThemeID:             input.ThemeID,
	}
}
