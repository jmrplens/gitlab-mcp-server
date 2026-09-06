package achievements

import (
	"context"
	"errors"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// Operation names used in wrapped errors and shared with the action specs.
const (
	opCreate                 = "create_achievement"
	opUpdate                 = "update_achievement"
	opDelete                 = "delete_achievement"
	opAward                  = "award_achievement"
	opRevoke                 = "revoke_achievement"
	opUserAchievementUpdate  = "update_user_achievement"
	opUserAchievementDelete  = "delete_user_achievement"
	opUserAchievementReorder = "reorder_user_achievements"
	opUserList               = "list_user_achievements"
	opList                   = "list_achievements"
	opRecipients             = "list_achievement_recipients"
	opUniqueUsers            = "list_achievement_unique_users"
	hintAchievementID        = "achievement_id is the numeric ID from a prior achievement.list response, not the gid:// global ID string"
	hintUserAchievementID    = "user_achievement_id is the numeric ID of an award, from achievement.recipients or achievement.user_list, not the achievement's own ID"
	hintNamespacePath        = "full_path is the group or project path such as my-group or my-group/my-project, and the namespace must already exist"
	hintUsername             = "username is the account name without a leading @, as returned by user.list"
	hintFeatureAvailability  = "achievements are generally available from GitLab 19.3; on an older instance an administrator must enable the achievements feature flag"
)

// Achievement mirrors gl.Achievement, the badge a namespace defines.
type Achievement struct {
	ID          int64  `json:"id"`
	NamespaceID int64  `json:"namespace_id"`
	Name        string `json:"name"`
	AvatarURL   string `json:"avatar_url,omitempty"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// UserAchievement mirrors gl.UserAchievement, one award of an achievement to
// one user. It is a record in its own right with its own ID: revoking or
// deleting an award addresses this ID, never the achievement's.
type UserAchievement struct {
	ID              int64  `json:"id"`
	AchievementID   int64  `json:"achievement_id"`
	UserID          int64  `json:"user_id"`
	AwardedByUserID int64  `json:"awarded_by_user_id"`
	RevokedByUserID *int64 `json:"revoked_by_user_id,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	RevokedAt       string `json:"revoked_at,omitempty"`
	Priority        *int64 `json:"priority,omitempty"`
	ShowOnProfile   bool   `json:"show_on_profile"`
	AwardMessage    string `json:"award_message,omitempty"`
}

// Output carries one achievement definition.
type Output struct {
	toolutil.HintableOutput
	Achievement Achievement `json:"achievement"`
}

// DeleteOutput carries the achievement as it looked when it was removed, which
// is what the GraphQL mutation returns instead of a bare acknowledgement.
type DeleteOutput struct {
	toolutil.HintableOutput
	Status      string      `json:"status"`
	Message     string      `json:"message"`
	Achievement Achievement `json:"achievement"`
}

// UserAchievementOutput carries one award.
type UserAchievementOutput struct {
	toolutil.HintableOutput
	UserAchievement UserAchievement `json:"user_achievement"`
}

// UserAchievementMutationOutput carries an award whose removal was the point of
// the call, so the status line says which of the two removals happened.
type UserAchievementMutationOutput struct {
	toolutil.HintableOutput
	Status          string          `json:"status"`
	Message         string          `json:"message"`
	UserAchievement UserAchievement `json:"user_achievement"`
}

// ListOutput carries a page of achievement definitions.
type ListOutput struct {
	toolutil.HintableOutput
	Achievements []Achievement                    `json:"achievements"`
	Pagination   toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// UserAchievementListOutput carries a page of awards.
type UserAchievementListOutput struct {
	toolutil.HintableOutput
	UserAchievements []UserAchievement                `json:"user_achievements"`
	Pagination       toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// ReorderOutput carries the awards in their new priority order. The mutation
// returns the whole reordered set rather than a page, so there is no cursor.
type ReorderOutput struct {
	toolutil.HintableOutput
	Status           string            `json:"status"`
	Message          string            `json:"message"`
	UserAchievements []UserAchievement `json:"user_achievements"`
}

// UniqueUsersOutput carries a page of the distinct recipients of one
// achievement, deduplicated across repeat awards.
type UniqueUsersOutput struct {
	toolutil.HintableOutput
	Users      []*toolutil.BasicUserOutput      `json:"users"`
	Pagination toolutil.GraphQLPaginationOutput `json:"pagination"`
}

// AvatarInput is the dual file shape shared by create and update: either a
// local path the server reads (refused over HTTP, where the caller's disk is
// not the server's) or inline base64 bytes.
type AvatarInput struct {
	AvatarFilename      string `json:"avatar_filename,omitempty"      jsonschema:"File name for the avatar image such as badge.png. Required whenever an avatar is sent, because GitLab identifies the upload part by its file name"`
	AvatarContentType   string `json:"avatar_content_type,omitempty"  jsonschema:"MIME type of the avatar image such as image/png. Defaults to application/octet-stream when omitted"`
	AvatarFilePath      string `json:"avatar_file_path,omitempty"     jsonschema:"Absolute path to a local image file the MCP server reads. Alternative to avatar_content_base64 for files too large to base64-encode. Only one of the two should be provided, and neither is available when the server is reached over HTTP"`
	AvatarContentBase64 string `json:"avatar_content_base64,omitempty" jsonschema:"Base64-encoded avatar image bytes. Alternative to avatar_file_path. Only one of the two should be provided"`
}

// CursorInput is the connection-style pagination every list action accepts.
type CursorInput struct {
	After  string `json:"after,omitempty"  jsonschema:"Cursor for forward pagination, taken from a previous response pagination.end_cursor"`
	Before string `json:"before,omitempty" jsonschema:"Cursor for backward pagination, taken from a previous response pagination.start_cursor"`
	First  *int64 `json:"first,omitempty"  jsonschema:"Number of items to return from the start of the page (default 20, max 100)"`
	Last   *int64 `json:"last,omitempty"   jsonschema:"Number of items to return from the end of the page, for backward pagination"`
}

// CreateInput defines the input for creating an achievement in a namespace.
type CreateInput struct {
	NamespaceID int64  `json:"namespace_id" jsonschema:"Numeric ID of the group or project namespace that will own the achievement,required"`
	Name        string `json:"name" jsonschema:"Display name of the achievement such as First Contribution,required"`
	Description string `json:"description,omitempty" jsonschema:"Free-text explanation of what the achievement is awarded for"`
	AvatarInput
}

// UpdateInput defines the input for changing an existing achievement.
type UpdateInput struct {
	AchievementID int64  `json:"achievement_id" jsonschema:"Numeric ID of the achievement to change,required"`
	Name          string `json:"name,omitempty" jsonschema:"New display name. Omit to leave the current name unchanged"`
	Description   string `json:"description,omitempty" jsonschema:"New description. Omit to leave the current description unchanged"`
	AvatarInput
}

// DeleteInput defines the input for deleting an achievement definition.
type DeleteInput struct {
	AchievementID int64 `json:"achievement_id" jsonschema:"Numeric ID of the achievement to delete. Every award made from it is removed with it,required"`
}

// AwardInput defines the input for awarding an achievement to a user.
type AwardInput struct {
	AchievementID int64  `json:"achievement_id" jsonschema:"Numeric ID of the achievement to hand out,required"`
	UserID        int64  `json:"user_id" jsonschema:"Numeric ID of the user receiving the achievement,required"`
	AwardMessage  string `json:"award_message,omitempty" jsonschema:"Note shown alongside the award, up to 200 characters"`
}

// RevokeInput defines the input for revoking an award.
type RevokeInput struct {
	UserAchievementID int64 `json:"user_achievement_id" jsonschema:"Numeric ID of the award to revoke. The award record is kept and marked revoked,required"`
}

// UserAchievementUpdateInput defines the input for changing an award.
type UserAchievementUpdateInput struct {
	UserAchievementID int64 `json:"user_achievement_id" jsonschema:"Numeric ID of the award to change,required"`
	ShowOnProfile     *bool `json:"show_on_profile,omitempty" jsonschema:"Whether the recipient's profile displays this award. Omit to leave the current visibility unchanged"`
}

// UserAchievementDeleteInput defines the input for deleting an award.
type UserAchievementDeleteInput struct {
	UserAchievementID int64 `json:"user_achievement_id" jsonschema:"Numeric ID of the award to delete. The record is removed outright rather than marked revoked,required"`
}

// UserAchievementReorderInput defines the input for reordering a user's awards.
type UserAchievementReorderInput struct {
	UserAchievementIDs []int64 `json:"user_achievement_ids" jsonschema:"Numeric award IDs in the order they should appear, highest priority first. All of one user's awards should be listed,required"`
}

// UserListInput defines the input for listing the awards one user holds.
type UserListInput struct {
	Username      string `json:"username" jsonschema:"Account name of the user whose awards to list, without a leading at sign,required"`
	IncludeHidden *bool  `json:"include_hidden,omitempty" jsonschema:"Include awards the user hid from their profile. Only the user themself and namespace or instance maintainers and owners see these"`
	CursorInput
}

// ListInput defines the input for listing the achievements a namespace defines.
type ListInput struct {
	FullPath string  `json:"full_path" jsonschema:"Full path of the group or project namespace such as my-group or my-group/my-project,required"`
	IDs      []int64 `json:"ids,omitempty" jsonschema:"Numeric achievement IDs to restrict the result to. Omit to list every achievement in the namespace"`
	CursorInput
}

// RecipientsInput defines the input for listing the awards of one achievement.
type RecipientsInput struct {
	FullPath      string `json:"full_path" jsonschema:"Full path of the group or project namespace that owns the achievement,required"`
	AchievementID int64  `json:"achievement_id" jsonschema:"Numeric ID of the achievement whose awards to list,required"`
	CursorInput
}

// UniqueUsersInput defines the input for listing the distinct recipients of one
// achievement.
type UniqueUsersInput struct {
	FullPath      string `json:"full_path" jsonschema:"Full path of the group or project namespace that owns the achievement,required"`
	AchievementID int64  `json:"achievement_id" jsonschema:"Numeric ID of the achievement whose recipients to count,required"`
	CursorInput
}

// Create defines a new achievement in a namespace.
func Create(ctx context.Context, client *gitlabclient.Client, input CreateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.NamespaceID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(opCreate, "namespace_id")
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return Output{}, toolutil.ErrRequiredString(opCreate, "name")
	}

	opts := &gl.CreateAchievementOptions{Name: &name}
	if description := strings.TrimSpace(input.Description); description != "" {
		opts.Description = &description
	}
	avatar, cleanup, err := input.upload(opCreate)
	if err != nil {
		return Output{}, err
	}
	defer cleanup()
	opts.Avatar = avatar

	achievement, _, err := client.GL().Achievements.CreateAchievement(input.NamespaceID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapErr(opCreate, err, "verify namespace_id names an existing group or project namespace you can administer")
	}
	return Output{Achievement: toAchievement(achievement)}, nil
}

// Update changes an existing achievement. Fields left empty keep their
// current value, because the GraphQL mutation treats an absent input field as
// "unchanged" rather than as "clear".
func Update(ctx context.Context, client *gitlabclient.Client, input UpdateInput) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	if input.AchievementID <= 0 {
		return Output{}, toolutil.ErrRequiredInt64(opUpdate, "achievement_id")
	}

	opts := &gl.UpdateAchievementOptions{}
	if name := strings.TrimSpace(input.Name); name != "" {
		opts.Name = &name
	}
	if description := strings.TrimSpace(input.Description); description != "" {
		opts.Description = &description
	}
	avatar, cleanup, err := input.upload(opUpdate)
	if err != nil {
		return Output{}, err
	}
	defer cleanup()
	opts.Avatar = avatar

	achievement, _, err := client.GL().Achievements.UpdateAchievement(input.AchievementID, opts, gl.WithContext(ctx))
	if err != nil {
		return Output{}, wrapErr(opUpdate, err, hintAchievementID)
	}
	return Output{Achievement: toAchievement(achievement)}, nil
}

// Delete removes an achievement definition and every award made from it.
func Delete(ctx context.Context, client *gitlabclient.Client, input DeleteInput) (DeleteOutput, error) {
	if err := ctx.Err(); err != nil {
		return DeleteOutput{}, err
	}
	if input.AchievementID <= 0 {
		return DeleteOutput{}, toolutil.ErrRequiredInt64(opDelete, "achievement_id")
	}

	achievement, _, err := client.GL().Achievements.DeleteAchievement(input.AchievementID, gl.WithContext(ctx))
	if err != nil {
		return DeleteOutput{}, wrapErr(opDelete, err, hintAchievementID)
	}
	return DeleteOutput{
		Status:      "success",
		Message:     "Successfully deleted the achievement and every award made from it.",
		Achievement: toAchievement(achievement),
	}, nil
}

// Award hands an achievement to a user, creating an award record.
func Award(ctx context.Context, client *gitlabclient.Client, input AwardInput) (UserAchievementOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementOutput{}, err
	}
	if input.AchievementID <= 0 {
		return UserAchievementOutput{}, toolutil.ErrRequiredInt64(opAward, "achievement_id")
	}
	if input.UserID <= 0 {
		return UserAchievementOutput{}, toolutil.ErrRequiredInt64(opAward, "user_id")
	}

	opts := &gl.AwardAchievementOptions{}
	if message := strings.TrimSpace(input.AwardMessage); message != "" {
		opts.AwardMessage = &message
	}

	award, _, err := client.GL().Achievements.AwardAchievement(input.AchievementID, input.UserID, opts, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementOutput{}, wrapErr(opAward, err, "verify achievement_id and user_id both exist, and that award_message stays within 200 characters")
	}
	return UserAchievementOutput{UserAchievement: toUserAchievement(award)}, nil
}

// Revoke marks an award revoked while keeping the record and its history.
func Revoke(ctx context.Context, client *gitlabclient.Client, input RevokeInput) (UserAchievementMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementMutationOutput{}, err
	}
	if input.UserAchievementID <= 0 {
		return UserAchievementMutationOutput{}, toolutil.ErrRequiredInt64(opRevoke, "user_achievement_id")
	}

	award, _, err := client.GL().Achievements.RevokeAchievement(input.UserAchievementID, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementMutationOutput{}, wrapErr(opRevoke, err, hintUserAchievementID)
	}
	return UserAchievementMutationOutput{
		Status:          "success",
		Message:         "Successfully revoked the award. The record is kept and marked revoked.",
		UserAchievement: toUserAchievement(award),
	}, nil
}

// UserAchievementUpdate changes an award, which today means its profile
// visibility.
func UserAchievementUpdate(ctx context.Context, client *gitlabclient.Client, input UserAchievementUpdateInput) (UserAchievementOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementOutput{}, err
	}
	if input.UserAchievementID <= 0 {
		return UserAchievementOutput{}, toolutil.ErrRequiredInt64(opUserAchievementUpdate, "user_achievement_id")
	}

	opts := &gl.UpdateUserAchievementOptions{ShowOnProfile: input.ShowOnProfile}

	award, _, err := client.GL().Achievements.UpdateUserAchievement(input.UserAchievementID, opts, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementOutput{}, wrapErr(opUserAchievementUpdate, err, hintUserAchievementID)
	}
	return UserAchievementOutput{UserAchievement: toUserAchievement(award)}, nil
}

// UserAchievementDelete removes an award record outright.
func UserAchievementDelete(ctx context.Context, client *gitlabclient.Client, input UserAchievementDeleteInput) (UserAchievementMutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementMutationOutput{}, err
	}
	if input.UserAchievementID <= 0 {
		return UserAchievementMutationOutput{}, toolutil.ErrRequiredInt64(opUserAchievementDelete, "user_achievement_id")
	}

	award, _, err := client.GL().Achievements.DeleteUserAchievement(input.UserAchievementID, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementMutationOutput{}, wrapErr(opUserAchievementDelete, err, hintUserAchievementID)
	}
	return UserAchievementMutationOutput{
		Status:          "success",
		Message:         "Successfully deleted the award record.",
		UserAchievement: toUserAchievement(award),
	}, nil
}

// UserAchievementReorder sets the display order of one user's awards.
func UserAchievementReorder(ctx context.Context, client *gitlabclient.Client, input UserAchievementReorderInput) (ReorderOutput, error) {
	if err := ctx.Err(); err != nil {
		return ReorderOutput{}, err
	}
	if len(input.UserAchievementIDs) == 0 {
		return ReorderOutput{}, toolutil.ErrFieldRequired("user_achievement_ids")
	}
	for _, id := range input.UserAchievementIDs {
		if id <= 0 {
			return ReorderOutput{}, toolutil.ErrRequiredInt64(opUserAchievementReorder, "user_achievement_ids")
		}
	}

	awards, _, err := client.GL().Achievements.UpdateUserAchievementPriorities(input.UserAchievementIDs, gl.WithContext(ctx))
	if err != nil {
		return ReorderOutput{}, wrapErr(opUserAchievementReorder, err, "every ID must be an award of the same user, from a prior achievement.user_list response")
	}
	return ReorderOutput{
		Status:           "success",
		Message:          "Successfully reordered the awards, highest priority first.",
		UserAchievements: toUserAchievements(awards),
	}, nil
}

// UserList lists the awards one user holds.
func UserList(ctx context.Context, client *gitlabclient.Client, input UserListInput) (UserAchievementListOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementListOutput{}, err
	}
	username := strings.TrimSpace(input.Username)
	if username == "" {
		return UserAchievementListOutput{}, toolutil.ErrRequiredString(opUserList, "username")
	}

	opts := &gl.ListUserAchievementsOptions{IncludeHidden: input.IncludeHidden}
	opts.After, opts.Before, opts.First, opts.Last = input.resolve()

	awards, resp, err := client.GL().Achievements.ListUserAchievements(username, opts, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementListOutput{}, wrapErr(opUserList, err, hintUsername)
	}
	return UserAchievementListOutput{
		UserAchievements: toUserAchievements(awards),
		Pagination:       pagination(resp),
	}, nil
}

// List lists the achievements a namespace defines.
func List(ctx context.Context, client *gitlabclient.Client, input ListInput) (ListOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListOutput{}, err
	}
	fullPath := strings.TrimSpace(input.FullPath)
	if fullPath == "" {
		return ListOutput{}, toolutil.ErrRequiredString(opList, "full_path")
	}

	opts := &gl.ListAchievementsOptions{IDs: input.IDs}
	opts.After, opts.Before, opts.First, opts.Last = input.resolve()

	achievements, resp, err := client.GL().Achievements.ListAchievements(fullPath, opts, gl.WithContext(ctx))
	if err != nil {
		return ListOutput{}, wrapErr(opList, err, hintNamespacePath)
	}

	out := ListOutput{Achievements: make([]Achievement, 0, len(achievements)), Pagination: pagination(resp)}
	for _, achievement := range achievements {
		out.Achievements = append(out.Achievements, toAchievement(achievement))
	}
	return out, nil
}

// Recipients lists every award of one achievement, including repeat awards to
// the same user and awards that have since been revoked.
func Recipients(ctx context.Context, client *gitlabclient.Client, input RecipientsInput) (UserAchievementListOutput, error) {
	if err := ctx.Err(); err != nil {
		return UserAchievementListOutput{}, err
	}
	fullPath := strings.TrimSpace(input.FullPath)
	if fullPath == "" {
		return UserAchievementListOutput{}, toolutil.ErrRequiredString(opRecipients, "full_path")
	}
	if input.AchievementID <= 0 {
		return UserAchievementListOutput{}, toolutil.ErrRequiredInt64(opRecipients, "achievement_id")
	}

	opts := &gl.ListAchievementRecipientsOptions{}
	opts.After, opts.Before, opts.First, opts.Last = input.resolve()

	awards, resp, err := client.GL().Achievements.ListAchievementRecipients(fullPath, input.AchievementID, opts, gl.WithContext(ctx))
	if err != nil {
		return UserAchievementListOutput{}, wrapErr(opRecipients, err, hintAchievementID)
	}
	return UserAchievementListOutput{
		UserAchievements: toUserAchievements(awards),
		Pagination:       pagination(resp),
	}, nil
}

// UniqueUsers lists the distinct users who hold one achievement, counting a
// user once however many times they were awarded it.
func UniqueUsers(ctx context.Context, client *gitlabclient.Client, input UniqueUsersInput) (UniqueUsersOutput, error) {
	if err := ctx.Err(); err != nil {
		return UniqueUsersOutput{}, err
	}
	fullPath := strings.TrimSpace(input.FullPath)
	if fullPath == "" {
		return UniqueUsersOutput{}, toolutil.ErrRequiredString(opUniqueUsers, "full_path")
	}
	if input.AchievementID <= 0 {
		return UniqueUsersOutput{}, toolutil.ErrRequiredInt64(opUniqueUsers, "achievement_id")
	}

	opts := &gl.ListAchievementUniqueUsersOptions{}
	opts.After, opts.Before, opts.First, opts.Last = input.resolve()

	users, resp, err := client.GL().Achievements.ListAchievementUniqueUsers(fullPath, input.AchievementID, opts, gl.WithContext(ctx))
	if err != nil {
		return UniqueUsersOutput{}, wrapErr(opUniqueUsers, err, hintAchievementID)
	}
	return UniqueUsersOutput{
		Users:      toolutil.NewBasicUserOutputs(users),
		Pagination: pagination(resp),
	}, nil
}

// newGraphQLUpload is a package variable so the construction-failure branch is
// testable: the constructor only fails on a reader that errors mid-read, and
// both readers the dual file shape can produce are already-validated bytes.
var newGraphQLUpload = gl.NewGraphQLUpload

// upload turns the dual file shape into the SDK's GraphQL upload, returning a
// nil upload when the caller sent no avatar at all. NewGraphQLUpload buffers
// the whole reader before it returns, so the cleanup the caller defers runs
// well after the bytes have been taken.
func (a AvatarInput) upload(operation string) (*gl.GraphQLUpload, func(), error) {
	noCleanup := func() {
		// Nothing to release. The caller defers whatever this function returns
		// without asking which of the two shapes it got, so the shape that
		// opened no file still has to hand back something to defer.
	}
	if a.AvatarFilePath == "" && a.AvatarContentBase64 == "" {
		return nil, noCleanup, nil
	}
	filename := strings.TrimSpace(a.AvatarFilename)
	if filename == "" {
		return nil, noCleanup, toolutil.ErrRequiredString(operation, "avatar_filename")
	}

	reader, _, cleanup, err := toolutil.OpenFileOrBase64Source(operation, a.AvatarFilePath, a.AvatarContentBase64)
	if err != nil {
		return nil, noCleanup, err
	}
	upload, err := newGraphQLUpload(reader, filename, strings.TrimSpace(a.AvatarContentType))
	if err != nil {
		cleanup()
		return nil, noCleanup, toolutil.WrapErrWithHint(operation, err, "the avatar must be a readable image and avatar_filename must be set")
	}
	return upload, cleanup, nil
}

// resolve returns the cursor options in the SDK's own shape. Only one of first
// and last is ever set, because a GraphQL connection rejects both at once, and
// a page size is requested even when the caller named none so a list action
// never asks the instance for every award it holds.
func (c CursorInput) resolve() (after, before *string, first, last *int64) {
	if c.After != "" {
		after = &c.After
	}
	if c.Before != "" {
		before = &c.Before
	}
	switch {
	case c.First != nil:
		first = new(clampPageSize(*c.First))
	case c.Last != nil:
		last = new(clampPageSize(*c.Last))
	default:
		first = new(int64(toolutil.GraphQLDefaultFirst))
	}
	return after, before, first, last
}

// clampPageSize bounds a requested page size to what GitLab accepts on a
// connection, the way toolutil.GraphQLPaginationInput.EffectiveFirst does for
// the page-number surfaces.
func clampPageSize(n int64) int64 {
	if n < 1 {
		return 1
	}
	if n > toolutil.GraphQLMaxFirst {
		return toolutil.GraphQLMaxFirst
	}
	return n
}

// pagination reads the cursor metadata the SDK hangs off the response, which is
// absent on an error path and on the mutations.
func pagination(resp *gl.Response) toolutil.GraphQLPaginationOutput {
	if resp == nil || resp.PageInfo == nil {
		return toolutil.GraphQLPaginationOutput{}
	}
	return toolutil.GraphQLPaginationOutput{
		HasNextPage:     resp.PageInfo.HasNextPage,
		HasPreviousPage: resp.PageInfo.HasPreviousPage,
		EndCursor:       resp.PageInfo.EndCursor,
		StartCursor:     resp.PageInfo.StartCursor,
	}
}

// wrapErr attaches an actionable hint. These endpoints are GraphQL, so a
// missing record arrives as the SDK's own sentinel rather than an HTTP 404 the
// status helpers could classify, and an instance that has the feature switched
// off answers the same way as one where the ID is simply wrong.
func wrapErr(operation string, err error, hint string) error {
	if errors.Is(err, gl.ErrNotFound) {
		return toolutil.WrapErrWithHint(operation, err, hint+". "+hintFeatureAvailability)
	}
	return toolutil.WrapErrWithHint(operation, err, hint)
}

func toAchievement(a *gl.Achievement) Achievement {
	if a == nil {
		return Achievement{}
	}
	out := Achievement{
		ID:          a.ID,
		NamespaceID: a.NamespaceID,
		Name:        a.Name,
		CreatedAt:   toolutil.FormatTimePtr(&a.CreatedAt),
		UpdatedAt:   toolutil.FormatTimePtr(&a.UpdatedAt),
	}
	if a.AvatarURL != nil {
		out.AvatarURL = *a.AvatarURL
	}
	if a.Description != nil {
		out.Description = *a.Description
	}
	return out
}

func toUserAchievement(u *gl.UserAchievement) UserAchievement {
	if u == nil {
		return UserAchievement{}
	}
	out := UserAchievement{
		ID:              u.ID,
		AchievementID:   u.AchievementID,
		UserID:          u.UserID,
		AwardedByUserID: u.AwardedByUserID,
		RevokedByUserID: u.RevokedByUserID,
		CreatedAt:       toolutil.FormatTimePtr(&u.CreatedAt),
		UpdatedAt:       toolutil.FormatTimePtr(&u.UpdatedAt),
		RevokedAt:       toolutil.FormatTimePtr(u.RevokedAt),
		Priority:        u.Priority,
		ShowOnProfile:   u.ShowOnProfile,
	}
	if u.AwardMessage != nil {
		out.AwardMessage = *u.AwardMessage
	}
	return out
}

func toUserAchievements(awards []*gl.UserAchievement) []UserAchievement {
	out := make([]UserAchievement, 0, len(awards))
	for _, award := range awards {
		out = append(out, toUserAchievement(award))
	}
	return out
}
