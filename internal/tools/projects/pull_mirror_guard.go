// pull_mirror_guard.go guards the one pull-mirror configuration that destroys
// commits: a mirror that overwrites diverged branches replaces the project's
// own history with whatever the source repository holds.

package projects

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// pullMirrorOverwriteConfirmID identifies this confirmation inside a multi
// round-trip elicitation flow, keeping it distinct from the destructive-action
// prompt a destructive route raises.
const pullMirrorOverwriteConfirmID = "project_pull_mirror_overwrite"

// notMirroredHint is the GitLab refusal that means the project has no pull
// mirror at all, rather than one this caller may not read.
const notMirroredHint = "not mirrored"

// confirmDivergedOverwrite asks the caller to confirm a pull-mirror
// configuration that would force-overwrite the project's own branches.
//
// Configuring a pull mirror is not destructive by itself, and is deliberately
// not classified as such: GitLab stops updating any branch that has diverged
// from the source, which is its own documented protection against data loss, so
// an ordinary mirror adds commits and branches and removes nothing. What
// removes something is mirror_overwrites_diverged_branches, which GitLab
// documents as always updating local branches with the remote versions "even if
// they have diverged", with "the loss of local changes". Those commits are
// unreachable afterwards and no call here brings them back.
//
// The guard therefore fires on the state rather than on the action: it asks
// when this call would leave the project pulling from its source with the
// overwrite armed, and when it is this call that arms or re-aims it, by setting
// the flag, by enabling the mirror, or by pointing it at another URL. Editing
// an unrelated setting on a mirror that already overwrites asks nothing, since
// a prompt raised by every edit is a prompt a caller learns to pass through;
// disabling the mirror or clearing the flag asks nothing either, because both
// are the remedy.
//
// Confirmation follows the same precedence as destructive actions: YOLO mode
// and an explicit confirm=true proceed immediately, otherwise the caller is
// elicited. Clients without elicitation get an actionable error telling them to
// resend with confirm=true, so the overwrite is never armed silently.
//
// It returns nil when the configuration may proceed.
func confirmDivergedOverwrite(ctx context.Context, client *gitlabclient.Client, input ConfigurePullMirrorInput) error {
	if !armsOverwrite(input) {
		return nil
	}
	// Check the bypasses before reading the mirror: a confirmed call does not
	// need to know what it is about to arm.
	req := toolutil.RequestFromContext(ctx)
	if toolutil.IsYOLOMode() || toolutil.ExplicitConfirmFromRequest(req) {
		return nil
	}
	if !overwritesAfterCall(ctx, client, input) {
		return nil
	}

	message := fmt.Sprintf(
		"Configure project %s to pull from %s and overwrite diverged branches?"+
			" Every branch of this project that has diverged from that repository is replaced by its version on the next sync,"+
			" and the replaced commits cannot be recovered through the API.",
		string(input.ProjectID), pullMirrorSourceForPrompt(input.URL),
	)

	flow, flowErr := elicitation.FlowFromRequest(req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"projectConfigurePullMirror: %s Re-send the same call with confirm=true once the user has approved it",
			message,
		)
	}

	confirmed, err := flow.Confirm(ctx, pullMirrorOverwriteConfirmID, message)
	switch {
	case errors.Is(err, elicitation.ErrInputPending):
		return flow.PendingError()
	case err != nil:
		return fmt.Errorf("projectConfigurePullMirror: confirmation failed: %w", err)
	case !confirmed:
		return errors.New("projectConfigurePullMirror: the user declined overwriting the project's diverged branches; send mirror_overwrites_diverged_branches=false to mirror without replacing them")
	}
	return nil
}

// armsOverwrite reports whether this call could put the project into the
// overwriting state, which is what makes reading the current configuration
// worth a round trip: it sets the flag, enables the mirror, or aims it
// somewhere else.
func armsOverwrite(input ConfigurePullMirrorInput) bool {
	return isTrue(input.MirrorOverwritesDivergedBranches) || isTrue(input.Enabled) || input.URL != ""
}

// isTrue reports whether an optional flag was sent with the value true.
func isTrue(flag *bool) bool {
	return flag != nil && *flag
}

// overwritesAfterCall reports whether the configuration this call leaves behind
// both pulls and overwrites diverged branches.
//
// Each of the two flags is the call's own where the call sends it and the
// project's current one otherwise, so the answer describes the configuration
// GitLab will hold rather than the fields this particular call happens to name.
// The current one is read once and only when it is needed.
//
// A project with no mirror at all inherits nothing, so an unsent overwrite flag
// is off; an unsent enabled flag there is treated as on, because whether GitLab
// activates a mirror created without it is not something its API documents, and
// the guess that costs a prompt is better than the guess that skips one. Any
// other read failure is answered the same way: a guard that cannot see the
// configuration must not assume the harmless one.
func overwritesAfterCall(ctx context.Context, client *gitlabclient.Client, input ConfigurePullMirrorInput) bool {
	overwrite, overwriteSent := flagValue(input.MirrorOverwritesDivergedBranches)
	enabled, enabledSent := flagValue(input.Enabled)
	if overwriteSent && !overwrite {
		return false
	}
	if enabledSent && !enabled {
		return false
	}
	if overwriteSent && enabledSent {
		return true
	}

	current, _, err := client.GL().Projects.GetProjectPullMirrorDetails(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if isNotMirrored(err) {
			return overwriteSent
		}
		return true
	}
	if !overwriteSent {
		overwrite = current.MirrorOverwritesDivergedBranches
	}
	if !enabledSent {
		enabled = current.Enabled
	}
	return overwrite && enabled
}

// flagValue reports an optional flag's value and whether the caller sent it.
func flagValue(flag *bool) (value, sent bool) {
	if flag == nil {
		return false, false
	}
	return *flag, true
}

// isNotMirrored reports whether err is GitLab refusing to describe a pull
// mirror because the project has none, which is a configuration to inherit
// nothing from rather than a failure to read one.
func isNotMirrored(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusBadRequest) && toolutil.ContainsAny(err, notMirroredHint)
}

// pullMirrorSourceForPrompt names the source the confirmation is about.
//
// The URL is the caller's own parameter and may carry credentials in its
// userinfo despite the schema saying not to, so it is rendered through
// [toolutil.RedactURL], and anything that does not parse as an absolute URL is
// withheld rather than echoed. A call that sends no URL keeps the source the
// project already has, which the prompt names rather than reveals: reading it
// would cost a round trip to tell the user something the project's own settings
// page already shows them.
func pullMirrorSourceForPrompt(raw string) string {
	if raw == "" {
		return "its configured mirror source"
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return toolutil.RedactedPlaceholder
	}
	return toolutil.RedactURL(parsed)
}
