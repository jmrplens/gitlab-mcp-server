// push_rules.go implements the group push-rule MCP tools: get, add, edit, and
// delete the singleton push-rule configuration of a group. Group push rules are
// a GitLab Premium/Ultimate feature and mirror the project push-rule shape.
package groups

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// PushRuleOutput represents a group's push-rule configuration. It mirrors
// gl.GroupPushRules 1:1 (1:1 audit output-fidelity policy).
type PushRuleOutput struct {
	toolutil.HintableOutput
	ID                         int64  `json:"id"`
	CommitMessageRegex         string `json:"commit_message_regex,omitempty"`
	CommitMessageNegativeRegex string `json:"commit_message_negative_regex,omitempty"`
	BranchNameRegex            string `json:"branch_name_regex,omitempty"`
	DenyDeleteTag              bool   `json:"deny_delete_tag"`
	MemberCheck                bool   `json:"member_check"`
	PreventSecrets             bool   `json:"prevent_secrets"`
	AuthorEmailRegex           string `json:"author_email_regex,omitempty"`
	FileNameRegex              string `json:"file_name_regex,omitempty"`
	MaxFileSize                int64  `json:"max_file_size"`
	CommitCommitterCheck       bool   `json:"commit_committer_check"`
	CommitCommitterNameCheck   bool   `json:"commit_committer_name_check"`
	RejectUnsignedCommits      bool   `json:"reject_unsigned_commits"`
	RejectNonDCOCommits        bool   `json:"reject_non_dco_commits"`
	CreatedAt                  string `json:"created_at,omitempty"`
}

// pushRuleOutputFromGL maps a gl.GroupPushRules result to PushRuleOutput.
func pushRuleOutputFromGL(r *gl.GroupPushRules) PushRuleOutput {
	out := PushRuleOutput{
		ID:                         r.ID,
		CommitMessageRegex:         r.CommitMessageRegex,
		CommitMessageNegativeRegex: r.CommitMessageNegativeRegex,
		BranchNameRegex:            r.BranchNameRegex,
		DenyDeleteTag:              r.DenyDeleteTag,
		MemberCheck:                r.MemberCheck,
		PreventSecrets:             r.PreventSecrets,
		AuthorEmailRegex:           r.AuthorEmailRegex,
		FileNameRegex:              r.FileNameRegex,
		MaxFileSize:                r.MaxFileSize,
		CommitCommitterCheck:       r.CommitCommitterCheck,
		CommitCommitterNameCheck:   r.CommitCommitterNameCheck,
		RejectUnsignedCommits:      r.RejectUnsignedCommits,
		RejectNonDCOCommits:        r.RejectNonDCOCommits,
	}
	if r.CreatedAt != nil {
		out.CreatedAt = r.CreatedAt.Format(time.RFC3339)
	}
	return out
}

// GetPushRulesInput defines parameters for getting a group's push rules.
type GetPushRulesInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// GetPushRules retrieves the push-rule configuration for a group.
func GetPushRules(ctx context.Context, client *gitlabclient.Client, input GetPushRulesInput) (PushRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return PushRuleOutput{}, err
	}
	if input.GroupID == "" {
		return PushRuleOutput{}, errors.New("groupGetPushRules: group_id is required. Use gitlab_group_list to find the ID, then pass it as group_id")
	}
	rule, _, err := client.GL().Groups.GetGroupPushRules(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("groupGetPushRules", err,
				"no push rules configured on this group, or the feature requires GitLab Premium/Ultimate — use gitlab_group_add_push_rule to create one")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("groupGetPushRules", err, http.StatusForbidden,
			"reading group push rules requires Premium/Ultimate licensing and Owner role on the group")
	}
	return pushRuleOutputFromGL(rule), nil
}

// AddPushRuleInput defines parameters for adding push rules to a group.
type AddPushRuleInput struct {
	GroupID                    toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	AuthorEmailRegex           string               `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            string               `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a group member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex string               `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         string               `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              string               `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from group members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

func hasAddPushRuleSetting(input AddPushRuleInput) bool {
	return input.AuthorEmailRegex != "" ||
		input.BranchNameRegex != "" ||
		input.CommitCommitterCheck != nil ||
		input.CommitCommitterNameCheck != nil ||
		input.CommitMessageNegativeRegex != "" ||
		input.CommitMessageRegex != "" ||
		input.DenyDeleteTag != nil ||
		input.FileNameRegex != "" ||
		input.MaxFileSize != nil ||
		input.MemberCheck != nil ||
		input.PreventSecrets != nil ||
		input.RejectUnsignedCommits != nil ||
		input.RejectNonDCOCommits != nil
}

// applyAddPushRuleOptions copies the AddPushRuleInput fields onto the SDK options.
func applyAddPushRuleOptions(input AddPushRuleInput, opts *gl.AddGroupPushRuleOptions) {
	if input.AuthorEmailRegex != "" {
		opts.AuthorEmailRegex = new(input.AuthorEmailRegex)
	}
	if input.BranchNameRegex != "" {
		opts.BranchNameRegex = new(input.BranchNameRegex)
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != "" {
		opts.CommitMessageNegativeRegex = new(input.CommitMessageNegativeRegex)
	}
	if input.CommitMessageRegex != "" {
		opts.CommitMessageRegex = new(input.CommitMessageRegex)
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != "" {
		opts.FileNameRegex = new(input.FileNameRegex)
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
}

// AddPushRule adds push-rule configuration to a group.
func AddPushRule(ctx context.Context, client *gitlabclient.Client, input AddPushRuleInput) (PushRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return PushRuleOutput{}, err
	}
	if input.GroupID == "" {
		return PushRuleOutput{}, errors.New("groupAddPushRule: group_id is required. Use gitlab_group_list to find the ID, then pass it as group_id")
	}
	if !hasAddPushRuleSetting(input) {
		return PushRuleOutput{}, errors.New("groupAddPushRule: at least one push rule setting is required. Include params.commit_message_regex for commit message regex tasks, or another setting such as reject_unsigned_commits, prevent_secrets, branch_name_regex, deny_delete_tag, member_check, or max_file_size")
	}
	opts := &gl.AddGroupPushRuleOptions{}
	applyAddPushRuleOptions(input, opts)
	rule, _, err := client.GL().Groups.AddGroupPushRule(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("groupAddPushRule", err,
				"push rules already exist on this group (use gitlab_group_edit_push_rule to update), one of the regex patterns is invalid, or the feature requires GitLab Premium/Ultimate")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("groupAddPushRule", err, http.StatusForbidden,
			"adding group push rules requires Owner role and Premium/Ultimate licensing")
	}
	return pushRuleOutputFromGL(rule), nil
}

// EditPushRuleInput defines parameters for editing push rules on a group.
type EditPushRuleInput struct {
	GroupID                    toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
	AuthorEmailRegex           *string              `json:"author_email_regex,omitempty" jsonschema:"Regex to validate author email addresses"`
	BranchNameRegex            *string              `json:"branch_name_regex,omitempty" jsonschema:"Regex to validate branch names"`
	CommitCommitterCheck       *bool                `json:"commit_committer_check,omitempty" jsonschema:"Reject commits where committer is not a group member"`
	CommitCommitterNameCheck   *bool                `json:"commit_committer_name_check,omitempty" jsonschema:"Reject commits where committer name does not match user name"`
	CommitMessageNegativeRegex *string              `json:"commit_message_negative_regex,omitempty" jsonschema:"Regex that commit messages must NOT match"`
	CommitMessageRegex         *string              `json:"commit_message_regex,omitempty" jsonschema:"Regex that commit messages must match"`
	DenyDeleteTag              *bool                `json:"deny_delete_tag,omitempty" jsonschema:"Deny tag deletion"`
	FileNameRegex              *string              `json:"file_name_regex,omitempty" jsonschema:"Regex for disallowed file names"`
	MaxFileSize                *int64               `json:"max_file_size,omitempty" jsonschema:"Maximum file size (MB). 0 means unlimited"`
	MemberCheck                *bool                `json:"member_check,omitempty" jsonschema:"Only allow commits from group members"`
	PreventSecrets             *bool                `json:"prevent_secrets,omitempty" jsonschema:"Reject files that are likely to contain secrets"`
	RejectUnsignedCommits      *bool                `json:"reject_unsigned_commits,omitempty" jsonschema:"Reject commits that are not GPG signed"`
	RejectNonDCOCommits        *bool                `json:"reject_non_dco_commits,omitempty" jsonschema:"Reject commits without DCO certification"`
}

// applyEditPushRuleOptions copies the EditPushRuleInput fields onto the SDK options.
func applyEditPushRuleOptions(input EditPushRuleInput, opts *gl.EditGroupPushRuleOptions) {
	if input.AuthorEmailRegex != nil {
		opts.AuthorEmailRegex = input.AuthorEmailRegex
	}
	if input.BranchNameRegex != nil {
		opts.BranchNameRegex = input.BranchNameRegex
	}
	if input.CommitCommitterCheck != nil {
		opts.CommitCommitterCheck = input.CommitCommitterCheck
	}
	if input.CommitCommitterNameCheck != nil {
		opts.CommitCommitterNameCheck = input.CommitCommitterNameCheck
	}
	if input.CommitMessageNegativeRegex != nil {
		opts.CommitMessageNegativeRegex = input.CommitMessageNegativeRegex
	}
	if input.CommitMessageRegex != nil {
		opts.CommitMessageRegex = input.CommitMessageRegex
	}
	if input.DenyDeleteTag != nil {
		opts.DenyDeleteTag = input.DenyDeleteTag
	}
	if input.FileNameRegex != nil {
		opts.FileNameRegex = input.FileNameRegex
	}
	if input.MaxFileSize != nil {
		opts.MaxFileSize = input.MaxFileSize
	}
	if input.MemberCheck != nil {
		opts.MemberCheck = input.MemberCheck
	}
	if input.PreventSecrets != nil {
		opts.PreventSecrets = input.PreventSecrets
	}
	if input.RejectUnsignedCommits != nil {
		opts.RejectUnsignedCommits = input.RejectUnsignedCommits
	}
	if input.RejectNonDCOCommits != nil {
		opts.RejectNonDCOCommits = input.RejectNonDCOCommits
	}
}

// EditPushRule modifies the push-rule configuration for a group.
func EditPushRule(ctx context.Context, client *gitlabclient.Client, input EditPushRuleInput) (PushRuleOutput, error) {
	if err := ctx.Err(); err != nil {
		return PushRuleOutput{}, err
	}
	if input.GroupID == "" {
		return PushRuleOutput{}, errors.New("groupEditPushRule: group_id is required. Use gitlab_group_list to find the ID, then pass it as group_id")
	}
	opts := &gl.EditGroupPushRuleOptions{}
	applyEditPushRuleOptions(input, opts)
	rule, _, err := client.GL().Groups.EditGroupPushRule(string(input.GroupID), opts, gl.WithContext(ctx))
	if err != nil {
		if toolutil.IsHTTPStatus(err, http.StatusUnprocessableEntity) || toolutil.IsHTTPStatus(err, http.StatusBadRequest) {
			return PushRuleOutput{}, toolutil.WrapErrWithHint("groupEditPushRule", err,
				"one of the regex patterns is invalid (use a Go-compatible regex syntax), or the field requires Premium/Ultimate")
		}
		return PushRuleOutput{}, toolutil.WrapErrWithStatusHint("groupEditPushRule", err, http.StatusNotFound,
			"no push rules currently exist on this group — use gitlab_group_add_push_rule first")
	}
	return pushRuleOutputFromGL(rule), nil
}

// DeletePushRuleInput defines parameters for deleting push rules from a group.
type DeletePushRuleInput struct {
	GroupID toolutil.StringOrInt `json:"group_id" jsonschema:"Group ID or URL-encoded path,required"`
}

// DeletePushRule deletes the push-rule configuration from a group.
func DeletePushRule(ctx context.Context, client *gitlabclient.Client, input DeletePushRuleInput) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if input.GroupID == "" {
		return errors.New("groupDeletePushRule: group_id is required. Use gitlab_group_list to find the ID, then pass it as group_id")
	}
	_, err := client.GL().Groups.DeleteGroupPushRule(string(input.GroupID), gl.WithContext(ctx))
	if err != nil {
		return toolutil.WrapErrWithStatusHint("groupDeletePushRule", err, http.StatusNotFound,
			"no push rules currently configured on this group — nothing to delete")
	}
	return nil
}

// ---------------------------------------------------------------------------
// Push rule Markdown formatter
// ---------------------------------------------------------------------------.

// FormatPushRuleMarkdown renders a group's push-rule configuration as Markdown.
func FormatPushRuleMarkdown(r PushRuleOutput) string {
	var b strings.Builder
	b.WriteString("## Group Push Rules\n\n")
	fmt.Fprintf(&b, toolutil.FmtMdID, r.ID)
	writePushRuleRegex(&b, r)
	writePushRuleFlags(&b, r)
	if r.MaxFileSize > 0 {
		fmt.Fprintf(&b, "- **Max File Size**: %d MB\n", r.MaxFileSize)
	}
	if r.CreatedAt != "" {
		fmt.Fprintf(&b, toolutil.FmtMdCreated, toolutil.FormatTime(r.CreatedAt))
	}
	toolutil.WriteHints(
		&b,
		"Use `gitlab_group_edit_push_rule` to change these settings",
		"Use `gitlab_group_delete_push_rule` to remove them",
	)
	return b.String()
}

func writePushRuleRegex(b *strings.Builder, r PushRuleOutput) {
	regexes := []struct {
		label string
		value string
	}{
		{"Commit Message Regex", r.CommitMessageRegex},
		{"Commit Message Negative Regex", r.CommitMessageNegativeRegex},
		{"Branch Name Regex", r.BranchNameRegex},
		{"Author Email Regex", r.AuthorEmailRegex},
		{"File Name Regex", r.FileNameRegex},
	}
	for _, re := range regexes {
		if re.value != "" {
			fmt.Fprintf(b, "- **%s**: `%s`\n", re.label, re.value)
		}
	}
}

func writePushRuleFlags(b *strings.Builder, r PushRuleOutput) {
	flags := []struct {
		label string
		value bool
	}{
		{"Deny Delete Tag", r.DenyDeleteTag},
		{"Member Check", r.MemberCheck},
		{"Prevent Secrets", r.PreventSecrets},
		{"Commit Committer Check", r.CommitCommitterCheck},
		{"Commit Committer Name Check", r.CommitCommitterNameCheck},
		{"Reject Unsigned Commits", r.RejectUnsignedCommits},
		{"Reject Non-DCO Commits", r.RejectNonDCOCommits},
	}
	for _, f := range flags {
		fmt.Fprintf(b, "- **%s**: %v\n", f.label, f.value)
	}
}

func init() {
	toolutil.RegisterMarkdown(FormatPushRuleMarkdown)
}
