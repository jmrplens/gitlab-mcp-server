// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/elicitation"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
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
// configuration that would force-overwrite the project's own branches. op is
// the operation label the refusals open with, the handler's own.
//
// Two actions can leave a project in that state, and both are judged here by
// the one rule: project.pull_mirror_configure, through PUT
// /projects/:id/mirror/pull, and project.update, which sends the same three
// settings to PUT /projects/:id under the names that route gives them, mirror
// for enabled and import_url for url. [UpdateInput.pullMirrorSettings]
// restates an update in the dedicated route's terms, so neither action is a
// way around the question the other asks.
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
// an unrelated setting on a mirror that already overwrites asks nothing, and
// neither does sending it the URL it already pulls from, since
// a prompt raised by every edit is a prompt a caller learns to pass through;
// disabling the mirror or clearing the flag asks nothing either, because both
// are the remedy.
//
// The one exception is a call that sets the flag while it leaves the mirror
// off, which is asked although nothing is overwritten yet (issue 1752). GitLab
// keeps the flag on a mirror that is off and describes no such mirror, so the
// later call that turns it back on would read nothing to ask about and every
// sync from then on would replace the diverged branches unasked. Its question
// says that the overwrite waits for the mirror to be turned back on. A flag
// stored while the mirror was off before this question existed, or through
// another client, stays out of sight all the same.
//
// Confirmation follows the same precedence as destructive actions: YOLO mode
// and an explicit confirm=true proceed immediately, otherwise the caller is
// elicited. Clients without elicitation get an actionable error telling them to
// resend with confirm=true, so the overwrite is never armed silently.
//
// It returns nil when the configuration may proceed.
func confirmDivergedOverwrite(ctx context.Context, client *gitlabclient.Client, op string, input ConfigurePullMirrorInput) error {
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

	question := pullMirrorQuestion(input)
	flow, flowErr := elicitation.FlowFromRequest(ctx, req)
	if flowErr != nil || !flow.IsSupported() {
		return fmt.Errorf(
			"%s: %s Re-send the same call with confirm=true once the user has approved it",
			op, question.English(),
		)
	}

	confirmed, err := flow.Confirm(ctx, pullMirrorOverwriteConfirmID, question.Text(ctx))
	// A chain of ifs rather than a switch: a switch case carries no statement
	// counter, so a mutation run reported the error check as not covered
	// whatever the tests did.
	if errors.Is(err, elicitation.ErrInputPending) {
		return flow.PendingError()
	}
	if err != nil {
		return fmt.Errorf("%s: confirmation failed: %w", op, err)
	}
	if !confirmed {
		return fmt.Errorf("%s: the user declined overwriting the project's diverged branches; send mirror_overwrites_diverged_branches=false to mirror without replacing them", op)
	}
	return nil
}

// pullMirrorQuestion is what [confirmDivergedOverwrite] asks about input.
//
// The question has two readers: the person it is asked of, in the request's
// language, and the model, which reads it in English as the refusal's opening
// sentence when the client cannot ask. A call that names no URL keeps the
// source the project has, which the question names in the person's language
// rather than reveals; one that names a URL is asked about that URL as the
// caller wrote it, redacted. A call that leaves the mirror off is asked
// whether to store the overwrite for when the mirror is turned back on, since
// nothing is pulled before that.
//
// Each of the four is written out in full because the catalog audit reads a
// question's key and the placeholders it fills from the call that renders it.
func pullMirrorQuestion(input ConfigurePullMirrorInput) i18n.Message {
	if enabled, sent := flagValue(input.Enabled); sent && !enabled {
		if input.URL == "" {
			return i18n.New(i18n.ConfirmPullMirrorOverwriteWhileOff,
				i18n.S("project", string(input.ProjectID)),
				i18n.M("source", i18n.New(i18n.ConfirmPullMirrorConfiguredSource)))
		}
		return i18n.New(i18n.ConfirmPullMirrorOverwriteWhileOff,
			i18n.S("project", string(input.ProjectID)),
			i18n.S("source", pullMirrorSourceForPrompt(input.URL)))
	}
	if input.URL == "" {
		return i18n.New(i18n.ConfirmPullMirrorOverwrite,
			i18n.S("project", string(input.ProjectID)),
			i18n.M("source", i18n.New(i18n.ConfirmPullMirrorConfiguredSource)))
	}
	return i18n.New(i18n.ConfirmPullMirrorOverwrite,
		i18n.S("project", string(input.ProjectID)),
		i18n.S("source", pullMirrorSourceForPrompt(input.URL)))
}

// pullMirrorSettings restates the pull-mirror settings a project update sends
// in the terms of the dedicated route, so [confirmDivergedOverwrite] judges the
// update by the rule it applies to project.pull_mirror_configure.
//
// PUT /projects/:id names the switch mirror and the source import_url, and the
// dedicated route's own handler in GitLab maps its enabled and url onto exactly
// those two project attributes (pull_mirror_update_attributes in
// ee/lib/api/project_mirror.rb), so the two calls write the same columns. The
// update's other mirror settings cannot arm the overwrite and are left out.
func (input UpdateInput) pullMirrorSettings() ConfigurePullMirrorInput {
	return ConfigurePullMirrorInput{
		ProjectID:                        input.ProjectID,
		Enabled:                          input.Mirror,
		URL:                              input.ImportURL,
		MirrorOverwritesDivergedBranches: input.MirrorOverwritesDivergedBranches,
	}
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
// both pulls and overwrites diverged branches, or would as soon as the mirror
// is turned back on.
//
// The overwrite flag is the call's own where the call sends it and the
// project's current one otherwise, so the answer describes the configuration
// GitLab will hold rather than the fields this particular call happens to name.
// The current one is read once and only when it is needed. Whether the mirror
// pulls takes no reading of its own: GET /projects/:id/mirror/pull describes
// only a mirror that is on (the route answers 400 unless the project mirrors,
// and the enabled it reports is the project's mirror column), so a mirror it
// describes is one this call leaves on, unless the call turns it off, which is
// decided before the read.
//
// Three calls say enough to be answered without a read. Clearing the overwrite
// never asks. Setting it beside enabled asks whichever way enabled is sent,
// also when the call turns the mirror off: GitLab keeps the flag on a mirror
// that is off and describes no such mirror, so the call that turns it back on
// would inherit nothing from the read, and the call that stores the flag is
// the last one that can be asked about it (issue 1752). Turning the mirror off
// without setting the flag asks nothing.
//
// A call that sends neither flag can arm nothing but by re-aiming the mirror,
// so its URL is held to the source the project already pulls from, and one
// naming that same repository is answered like the unrelated edit it is, since
// it changes neither what the mirror pulls nor whether it overwrites.
// [sameMirrorSource] says what counts as the same repository.
//
// A project with no mirror at all inherits nothing, so an unsent overwrite flag
// is off; an unsent enabled flag there is treated as on, because whether GitLab
// activates a mirror created without it is not something its API documents, and
// the guess that costs a prompt is better than the guess that skips one. GitLab
// answers the read of a mirror that is off the same way, 400 not mirrored, so a
// flag such a mirror stores is not inherited either, which is why the call
// that stores it is asked.
//
// A 404 is read the same way, as nothing to inherit. GitLab CE does not mount
// the pull mirror route at all, so there it is the answer to every read, and a
// project.update naming import_url or mirror, which the meta tool hands its
// handler although the Free schema prunes both, would otherwise be refused
// with a question about a mirror CE ignores. On a licensed instance the
// route's 404 means the project cannot be seen, which the write that follows
// is refused for as well. Any other read failure is answered by asking: a
// guard that cannot see the configuration must not assume the harmless one.
func overwritesAfterCall(ctx context.Context, client *gitlabclient.Client, input ConfigurePullMirrorInput) bool {
	overwrite, overwriteSent := flagValue(input.MirrorOverwritesDivergedBranches)
	enabled, enabledSent := flagValue(input.Enabled)
	if overwriteSent && !overwrite {
		return false
	}
	if overwriteSent && enabledSent {
		return true
	}
	if enabledSent && !enabled {
		return false
	}

	current, _, err := client.GL().Projects.GetProjectPullMirrorDetails(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if isNotMirrored(err) || toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			return overwriteSent
		}
		return true
	}
	if !overwriteSent && !enabledSent && sameMirrorSource(input.URL, current.URL) {
		return false
	}
	return overwriteSent || current.MirrorOverwritesDivergedBranches
}

// flagValue reports an optional flag's value and whether the caller sent it.
func flagValue(flag *bool) (value, sent bool) {
	if flag == nil {
		return false, false
	}
	return *flag, true
}

// sameMirrorSource reports whether sent names the repository current already
// names, current being the url GET /projects/:id/mirror/pull reports for the
// project's mirror.
//
// GitLab reports that url through Project#safe_import_url, which replaces the
// user and the password with ***** (Gitlab::UrlSanitizer#masked_url), so the
// two are compared without their credentials: those decide whom the mirror
// authenticates as, not which repository it pulls. The scheme and the host
// are compared without regard to case, as URLs define them, and the rest has
// to match as written. A spelling GitLab might normalize, such as a default
// port or a trailing slash, therefore reads as another source, which costs a
// question rather than skipping one.
func sameMirrorSource(sent, current string) bool {
	key := mirrorSourceKey(sent)
	return key != "" && key == mirrorSourceKey(current)
}

// mirrorSourceKey renders raw without its credentials and with its host in
// lower case, the form [sameMirrorSource] compares, or returns "" for a URL it
// does not compare.
//
// A URL net/url cannot parse is not compared, and neither is one with an @
// left once the credentials are gone. GitLab takes everything up to the last
// @ of the whole URL for credentials (Gitlab::UrlSanitizer#parse_url), so for
// https://host/a@b/repo it pulls from b while net/url reads host: the two
// parsers agree on such a URL's repository only by chance. GitLab reads every
// url it reports through that rule, so none it reports should carry one, and
// the comparison does not rely on that.
func mirrorSourceKey(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	parsed.User = nil
	parsed.Host = strings.ToLower(parsed.Host)
	if key := parsed.String(); !strings.Contains(key, "@") {
		return key
	}
	return ""
}

// isNotMirrored reports whether err is GitLab refusing to describe a pull
// mirror because the project has none, which is a configuration to inherit
// nothing from rather than a failure to read one.
func isNotMirrored(err error) bool {
	return toolutil.IsHTTPStatus(err, http.StatusBadRequest) && toolutil.ContainsAny(err, notMirroredHint)
}

// pullMirrorSourceForPrompt names the URL a call points the mirror at, for the
// confirmation.
//
// The URL is the caller's own parameter and may carry credentials in its
// userinfo despite the schema saying not to, so it is rendered through
// [toolutil.RedactURL], and anything that does not parse as an absolute URL is
// withheld rather than echoed. A call that sends no URL keeps the source the
// project already has, which [confirmDivergedOverwrite] names from the catalog
// rather than reveals: reading it would cost a round trip to tell the user
// something the project's own settings page already shows them.
func pullMirrorSourceForPrompt(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return toolutil.RedactedPlaceholder
	}
	return toolutil.RedactURL(parsed)
}
