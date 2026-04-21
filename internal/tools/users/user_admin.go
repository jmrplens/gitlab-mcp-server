// user_admin.go implements admin user state management operations:
// block, unblock, ban, unban, activate, deactivate, approve, reject, disable 2FA.

package users

import (
	"context"
	"errors"
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/internal/toolutil"
)

// AdminActionInput holds the user_id parameter for admin state actions.
type AdminActionInput struct {
	UserID int64 `json:"user_id" jsonschema:"The ID of the user,required"`
}

// AdminActionOutput represents the result of an admin state action.
type AdminActionOutput struct {
	UserID  int64  `json:"user_id"`
	Action  string `json:"action"`
	Success bool   `json:"success"`
}

// BlockUser blocks a GitLab user (admin only).
func BlockUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("block_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.BlockUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("block_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "blocked", Success: true}, nil
}

// UnblockUser unblocks a previously blocked GitLab user (admin only).
func UnblockUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("unblock_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.UnblockUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("unblock_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "unblocked", Success: true}, nil
}

// BanUser bans a GitLab user (admin only).
func BanUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("ban_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.BanUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("ban_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "banned", Success: true}, nil
}

// UnbanUser unbans a previously banned GitLab user (admin only).
func UnbanUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("unban_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.UnbanUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("unban_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "unbanned", Success: true}, nil
}

// ActivateUser activates a deactivated GitLab user (admin only).
func ActivateUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("activate_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.ActivateUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("activate_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "activated", Success: true}, nil
}

// DeactivateUser deactivates an active GitLab user (admin only).
func DeactivateUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("deactivate_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.DeactivateUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("deactivate_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "deactivated", Success: true}, nil
}

// ApproveUser approves a pending GitLab user (admin only).
func ApproveUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("approve_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.ApproveUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("approve_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "approved", Success: true}, nil
}

// RejectUser rejects a pending GitLab user (admin only).
func RejectUser(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("reject_user: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.RejectUser(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("reject_user", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "rejected", Success: true}, nil
}

// DisableTwoFactor disables two-factor authentication for a GitLab user (admin only).
func DisableTwoFactor(ctx context.Context, client *gitlabclient.Client, input AdminActionInput) (AdminActionOutput, error) {
	if input.UserID == 0 {
		return AdminActionOutput{}, errors.New("disable_two_factor: user_id is required")
	}
	if err := ctx.Err(); err != nil {
		return AdminActionOutput{}, err
	}
	_, err := client.GL().Users.DisableTwoFactor(input.UserID, gl.WithContext(ctx))
	if err != nil {
		return AdminActionOutput{}, toolutil.WrapErrWithMessage("disable_two_factor", err)
	}
	return AdminActionOutput{UserID: input.UserID, Action: "two_factor_disabled", Success: true}, nil
}

// FormatAdminActionMarkdownString renders an admin action result as Markdown.
func FormatAdminActionMarkdownString(o AdminActionOutput) string {
	return fmt.Sprintf("## User Admin Action\n\n"+
		toolutil.FmtMdID+
		"- **Action**: %s\n"+
		"- **Success**: %s %v\n",
		o.UserID, o.Action, toolutil.EmojiSuccess, o.Success)
}
