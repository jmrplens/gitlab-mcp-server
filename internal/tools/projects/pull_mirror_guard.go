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
	"strconv"
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
// /projects/:id/mirror/pull, and project.update, which sends the same settings
// to PUT /projects/:id, the switch and the source under the names that route
// gives them, mirror for enabled and import_url for url.
// [UpdateInput.pullMirrorSettings] restates an update in the dedicated route's
// terms, so neither action is a way around the question the other asks.
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
// the flag, by enabling the mirror, by pointing it at another URL, or by
// letting it pull branches it skipped. GitLab replaces a diverged branch only
// among the ones the mirror does not skip, which only_mirror_protected_branches
// and mirror_branch_regex decide (Gitlab::Repositories::Mirror::BranchSkipFilter,
// applied in Projects::UpdateMirrorService#update_branches before
// handle_diverged_branch), so widening that filter aims the overwrite at
// branches it did not reach, as [branchFilter] describes. Editing an unrelated
// setting on a mirror that already overwrites asks nothing, and neither does
// sending it the URL it already pulls from or a filter that reaches no branch
// it did not reach, since a prompt raised by every edit is a prompt a caller
// learns to pass through; disabling the mirror or clearing the flag asks
// nothing either, because both are the remedy.
//
// The one exception is a call that sets the flag while it leaves the mirror
// off, which is asked although nothing is overwritten yet (issue 1752). A call
// leaves the mirror off by turning it off, or by not sending enabled to a
// mirror that is off, since GitLab assigns the mirror's switch only when the
// call sends it. GitLab keeps the flag on a mirror that is off and describes
// no such mirror, so the later call that turns it on would read nothing to ask
// about and every sync from then on would replace the diverged branches
// unasked. Its question says that the overwrite waits for the mirror to be
// turned on. A flag stored while the mirror was off before this question
// existed, or through another client, stays out of sight all the same.
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
	ask, branches := overwritesAfterCall(ctx, client, input)
	if ask == askNothing {
		return nil
	}

	question := pullMirrorQuestion(input, ask, branches)
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
// caller wrote it, redacted. ask is the question [overwritesAfterCall] chose.
// A call that leaves the mirror off is asked whether to store the overwrite
// for when the mirror is turned on, since nothing is pulled before that. A
// call that only widens the branches a mirror that overwrites pulls is asked
// about those branches, which are what changes, named as branches, the filter
// the mirror pulls through after the call.
//
// Each of the six is written out in full because the catalog audit reads a
// question's key and the placeholders it fills from the call that renders it.
func pullMirrorQuestion(input ConfigurePullMirrorInput, ask mirrorAsk, branches branchFilter) i18n.Message {
	if ask == askOverwriteWhileOff {
		if input.URL == "" {
			return i18n.New(i18n.ConfirmPullMirrorOverwriteWhileOff,
				i18n.S("project", string(input.ProjectID)),
				i18n.M("source", i18n.New(i18n.ConfirmPullMirrorConfiguredSource)))
		}
		return i18n.New(i18n.ConfirmPullMirrorOverwriteWhileOff,
			i18n.S("project", string(input.ProjectID)),
			i18n.S("source", pullMirrorSourceForPrompt(input.URL)))
	}
	if ask == askWidening {
		if input.URL == "" {
			return i18n.New(i18n.ConfirmPullMirrorOverwriteWiden,
				i18n.S("project", string(input.ProjectID)),
				i18n.M("branches", branchesPulled(branches)),
				i18n.M("source", i18n.New(i18n.ConfirmPullMirrorConfiguredSource)))
		}
		return i18n.New(i18n.ConfirmPullMirrorOverwriteWiden,
			i18n.S("project", string(input.ProjectID)),
			i18n.M("branches", branchesPulled(branches)),
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
// branch filter, only_mirror_protected_branches and mirror_branch_regex, has
// the same name on both routes, and both assign it the same way, as
// [branchFilter.after] says. The update's other mirror settings, the trigger
// of builds and the mirror user, change neither which branches are pulled nor
// whether they are overwritten, and are left out.
func (input UpdateInput) pullMirrorSettings() ConfigurePullMirrorInput {
	return ConfigurePullMirrorInput{
		ProjectID:                        input.ProjectID,
		Enabled:                          input.Mirror,
		URL:                              input.ImportURL,
		MirrorOverwritesDivergedBranches: input.MirrorOverwritesDivergedBranches,
		OnlyMirrorProtectedBranches:      input.OnlyMirrorProtectedBranches,
		MirrorBranchRegex:                input.MirrorBranchRegex,
	}
}

// armsOverwrite reports whether this call could put the project into the
// overwriting state, or widen the branches it reaches, which is what makes
// reading the current configuration worth a round trip: it sets the flag,
// enables the mirror, aims it somewhere else, or sends the branch filter.
// Which of the filter's changes widen it takes the read to say.
func armsOverwrite(input ConfigurePullMirrorInput) bool {
	return isTrue(input.MirrorOverwritesDivergedBranches) || isTrue(input.Enabled) || input.URL != "" ||
		input.OnlyMirrorProtectedBranches != nil || input.MirrorBranchRegex != ""
}

// mirrorAsk is the question, if any, [overwritesAfterCall] decides a call is
// asked, which [pullMirrorQuestion] renders.
type mirrorAsk uint8

const (
	// askNothing lets the call through.
	askNothing mirrorAsk = iota
	// askOverwrite asks about a call that leaves the mirror pulling with the
	// overwrite armed, and is the call that arms or re-aims it.
	askOverwrite
	// askOverwriteWhileOff asks about a call that stores the overwrite on a
	// mirror it leaves off.
	askOverwriteWhileOff
	// askWidening asks about a call that changes nothing of a mirror that
	// overwrites but the branches it pulls, and lets it pull ones it skipped.
	askWidening
)

// branchFilter is the part of a pull mirror's configuration that decides which
// of the source's branches it pulls, and so which ones its overwrite reaches.
// GitLab skips an unprotected branch while protectedOnly is set, and a branch
// whose name regex does not match while regex is not empty
// (Gitlab::Repositories::Mirror::BranchSkipFilter), and replaces a diverged
// branch only among the ones it does not skip. The zero value pulls every
// branch.
type branchFilter struct {
	protectedOnly bool
	regex         string
}

// after returns the filter GitLab holds once input is applied to current.
//
// A regex the call sends turns protected-only off, and protected-only sent
// true clears the regex: both routes assign the pair that way
// (Repositories::PullMirrors::Attributes#allowed for the dedicated one and
// EE::Projects::UpdateService#mirror_branch_setting for the project update),
// so GitLab never holds both. Protected-only sent false leaves the regex as it
// is. Both routes refuse a call that sends the two together, so a regex winning
// here only decides the question asked before that refusal.
func (current branchFilter) after(input ConfigurePullMirrorInput) branchFilter {
	if input.MirrorBranchRegex != "" {
		return branchFilter{regex: input.MirrorBranchRegex}
	}
	if isTrue(input.OnlyMirrorProtectedBranches) {
		return branchFilter{protectedOnly: true}
	}
	if input.OnlyMirrorProtectedBranches != nil {
		return branchFilter{regex: current.regex}
	}
	return current
}

// widenedBy reports whether a mirror pulling through current would pull a
// branch it skips once it pulls through next.
//
// A filter that passes every branch cannot be widened, and one left as it is
// widens nothing. Any other change counts as widening, although some narrow in
// practice: whether a regex matches more than another, or than the protected
// branches, turns on the branches the source holds and on which of them are
// protected, which this server does not read. Like the URL comparison, that
// costs a question rather than skipping one.
func (current branchFilter) widenedBy(next branchFilter) bool {
	return next != current && (current.protectedOnly || current.regex != "")
}

// branchesPulled names the branches a mirror pulling through filter pulls, for
// the widening question. A filter [branchFilter.after] returns holds at most
// one of its two settings. A regex is quoted, since it is the caller's own
// text and the person has to see where it ends.
func branchesPulled(filter branchFilter) i18n.Message {
	if filter.regex != "" {
		return i18n.New(i18n.ConfirmPullMirrorBranchesMatching, i18n.S("regex", strconv.Quote(filter.regex)))
	}
	if filter.protectedOnly {
		return i18n.New(i18n.ConfirmPullMirrorBranchesProtected)
	}
	return i18n.New(i18n.ConfirmPullMirrorBranchesAll)
}

// isTrue reports whether an optional flag was sent with the value true.
func isTrue(flag *bool) bool {
	return flag != nil && *flag
}

// overwritesAfterCall reports which question, if any, this call is asked: none
// unless the configuration it leaves behind both pulls and overwrites diverged
// branches, or would as soon as the mirror is turned on, and the call is the
// one that arms, re-aims or widens the overwrite. askOverwriteWhileOff is the
// answer for a call that leaves the mirror off, and askWidening for one whose
// only change to a mirror that overwrites is the branches it pulls, which are
// then returned as the filter the mirror pulls through after the call.
//
// The overwrite flag is the call's own where the call sends it and the
// project's current one otherwise, so the answer describes the configuration
// GitLab will hold rather than the fields this particular call happens to name.
// The current configuration is read once, and only when the call leaves unsaid
// something the answer turns on: the flag, where the call does not send it,
// whether the mirror is on, where it sets the flag without sending enabled,
// and the source and branch filter the mirror has, where the call sends one of
// its own.
// Whether the mirror pulls takes no reading of its own: GET
// /projects/:id/mirror/pull describes only a mirror that is on (the route
// answers 400 unless the project mirrors, and the enabled it reports is the
// project's mirror column), so a mirror it describes is one this call leaves
// on, unless the call turns it off, which is decided before the read.
//
// Three calls say enough to be answered without a read. Clearing the overwrite
// never asks. Setting it beside enabled asks whichever way enabled is sent,
// also when the call turns the mirror off: GitLab keeps the flag on a mirror
// that is off and describes no such mirror, so the call that turns it back on
// would inherit nothing from the read, and the call that stores the flag is
// the last one that can be asked about it (issue 1752). Turning the mirror off
// without setting the flag asks nothing.
//
// A call that sends neither flag can arm nothing but by re-aiming the mirror
// or widening the branches it pulls, so its URL is held to the source the
// project already pulls from, and its branch filter to the one the mirror
// pulls through, both of which the read reports. One that names that same
// repository and reaches no branch the mirror skipped is answered like the
// unrelated edit it is, since it changes neither what the mirror pulls nor
// whether it overwrites. [sameMirrorSource] says what counts as the same
// repository and [branchFilter.widenedBy] what counts as widening.
//
// A project with no mirror at all inherits nothing, so an unsent overwrite flag
// is off, and GitLab answers the read of a mirror that is off the same way, 400
// not mirrored, so a flag such a mirror stores is not inherited either, which
// is why the call that stores it is asked. A call asked on that answer sets
// the flag without sending enabled, since one sending both is decided before
// the read, and it leaves the mirror off: GitLab assigns the mirror's switch
// only when the call sends it (both routes hand on only the params the call
// declared, and Repositories::PullMirrors::Attributes and
// Projects::UpdateService assign only those). So it is asked whether to store
// the overwrite for when the mirror is turned on.
//
// A 404 is read the same way, as nothing to inherit and a mirror left off.
// GitLab CE does not mount the pull mirror route at all, so there it is the
// answer to every read, and a project.update naming import_url or mirror, which
// the meta tool hands its handler although the Free schema prunes both, would
// otherwise be refused with a question about a mirror CE ignores. On a licensed
// instance the route's 404 means the project cannot be seen, which the write
// that follows is refused for as well. Any other read failure is answered by
// asking, with the question of a mirror that pulls: a guard that cannot see
// the configuration must not assume the harmless one.
func overwritesAfterCall(ctx context.Context, client *gitlabclient.Client, input ConfigurePullMirrorInput) (mirrorAsk, branchFilter) {
	overwrite, overwriteSent := flagValue(input.MirrorOverwritesDivergedBranches)
	enabled, enabledSent := flagValue(input.Enabled)
	if overwriteSent && !overwrite {
		return askNothing, branchFilter{}
	}
	if overwriteSent && enabledSent {
		if enabled {
			return askOverwrite, branchFilter{}
		}
		return askOverwriteWhileOff, branchFilter{}
	}
	if enabledSent && !enabled {
		return askNothing, branchFilter{}
	}

	current, _, err := client.GL().Projects.GetProjectPullMirrorDetails(string(input.ProjectID), gl.WithContext(ctx))
	if err != nil {
		if isNotMirrored(err) || toolutil.IsHTTPStatus(err, http.StatusNotFound) {
			if overwriteSent {
				return askOverwriteWhileOff, branchFilter{}
			}
			return askNothing, branchFilter{}
		}
		return askOverwrite, branchFilter{}
	}
	if !overwriteSent && !current.MirrorOverwritesDivergedBranches {
		return askNothing, branchFilter{}
	}
	if overwriteSent || enabledSent || (input.URL != "" && !sameMirrorSource(input.URL, current.URL)) {
		return askOverwrite, branchFilter{}
	}
	held := branchFilter{protectedOnly: current.OnlyMirrorProtectedBranches, regex: current.MirrorBranchRegex}
	if next := held.after(input); held.widenedBy(next) {
		return askWidening, next
	}
	return askNothing, branchFilter{}
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
// A URL net/url cannot parse is not compared, and neither is one GitLab reads
// past its userinfo, as [readsPastUserinfo] says: the two parsers agree on
// such a URL's repository only by chance. GitLab reads every url it reports
// through that rule, so none it reports should carry one, and the comparison
// does not rely on that.
//
// Nor is a URL whose path starts with ~ compared, which [homeRelative] says
// is one where the user picks the repository rather than the credentials.
// GitLab reports that user masked, so the url it reports cannot tell this
// server which repository the mirror pulls.
func mirrorSourceKey(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || homeRelative(parsed.Path) || readsPastUserinfo(parsed) {
		return ""
	}
	parsed.User = nil
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String()
}

// readsPastUserinfo reports whether GitLab takes more of parsed for
// credentials than net/url does.
//
// GitLab takes everything up to the last @ of the whole URL for credentials
// (Gitlab::UrlSanitizer#parse_url, whose comment says a password may hold #,
// ? and /), and net/url only what comes before the host. The two agree only
// when no @ is left once the userinfo is gone, in the path, the query or the
// fragment. In https://host/a@b/repo GitLab pulls from b while net/url reads
// host, and so it does for https://host/repo?ref=x@b/repo, whose @ sits in a
// query [toolutil.RedactURL] drops. An @ written escaped, %40, is not one
// GitLab splits at, and net/url keeps it escaped.
func readsPastUserinfo(parsed *url.URL) bool {
	bare := *parsed
	bare.User = nil
	return strings.Contains(bare.String(), "@")
}

// homeRelative reports whether a mirror URL's path is one git resolves in the
// home directory of the account it logs in as. Over SSH,
// ssh://alice@host/~/repo.git and ssh://bob@host/~/repo.git name two
// repositories, and a URL that names no user logs in as the default one.
func homeRelative(path string) bool {
	return strings.HasPrefix(path, "/~")
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
// withheld rather than echoed. So is a URL GitLab reads past its userinfo, as
// [readsPastUserinfo] says, wherever its @ is: in
// https://alice:123/secret@mirror.example/repo.git GitLab reads the password
// 123/secret where net/url reads a host and a path, and in
// https://github.com/trusted/repo.git?ref=x@evil.example/repo.git it pulls
// from evil.example, which sits in the query the rendering drops. Either
// rendering would name a host GitLab does not pull from, and the first would
// show that password too.
//
// The one user shown is the login of an SSH URL whose path [homeRelative]
// says the login resolves, without its password: there the login names the
// repository, so dropping it would ask about ssh://host/~/repo.git whichever
// account's home the call points the mirror at, the masked form GitLab's own
// settings page shows for the source the mirror has. Over HTTPS the same path
// names one repository whoever authenticates, and a user there can be a token,
// so it is dropped like any other credential.
//
// A call that sends no URL keeps the source the project already has, which
// [confirmDivergedOverwrite] names from the catalog rather than reveals:
// reading it would cost a round trip to tell the user something the project's
// own settings page already shows them.
func pullMirrorSourceForPrompt(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || readsPastUserinfo(parsed) {
		return toolutil.RedactedPlaceholder
	}
	if login := parsed.User.Username(); login != "" && parsed.Scheme == "ssh" && homeRelative(parsed.Path) {
		return parsed.Scheme + "://" + url.User(login).String() + "@" + parsed.Host + parsed.EscapedPath()
	}
	return toolutil.RedactURL(parsed)
}
