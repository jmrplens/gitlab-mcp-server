// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// quick_action_guard.go asks before a text is sent that carries a quick
// action GitLab runs with the effect of an action that asks. GitLab reads the
// body of a note and the description of an issue, a merge request, an epic or
// a work item for lines that begin with a command, and runs them when it
// saves the text, so a note is another way to move an issue or to merge a
// merge request, and the actions that send such a text ask here for the lines
// that do.

package toolutil

import (
	"context"
	"regexp"
	"strings"
	"unicode"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/i18n"
)

// QuickActionConfirmID is the question the actions that send a note body or
// a description ask before a quick action in it is run.
const QuickActionConfirmID = "quick_action_effect"

// Bounds of what the question shows of the text: the lines it quotes, and
// the length of each. A text can carry any number of lines of any length, and
// the question is read by a person in a dialog. quickActionLinesShown is the
// least the question quotes, not the most: every kind of effect is quoted at
// least once whatever the count, so the question quotes up to the 16 guarded
// commands, which the two pairs that share an effect bring to 13 kinds, and
// counts only the lines whose kind it already quotes.
const (
	quickActionLinesShown = 6
	quickActionLineRunes  = 120
)

// guardedQuickActions are the quick actions the guard asks about, each with
// the sentence fragment that says what it does. They are the commands whose
// effect is one the catalog asks about under its own action, or one that no
// call takes back or that hands the item to a party outside the instance:
//
//   - /move moves the item to another project or group, which issue.move asks
//     about,
//   - /merge merges the merge request, at once or when its checks pass, which
//     merge_request.merge asks about,
//   - /promote closes the issue and makes an epic of it, and no call makes
//     the epic an issue again, which is what promoting a label or a milestone
//     asks about,
//   - /convert_to_ticket makes the issue a Service Desk ticket whose author
//     is an outside email address, and no call restores the author,
//   - /add_email adds outside email addresses as participants, which receive
//     the comments of the issue from then on, and removing them recalls
//     nothing that was sent,
//   - /publish publishes the issue on the status page of the project, where
//     anybody reads it,
//   - /unapprove withdraws the caller's approval of the merge request, which
//     merge_request.unapprove asks about,
//   - /unlink removes the link between the item and another, which
//     issue.link_delete asks about,
//   - /remove_epic, /remove_parent and /remove_parent_epic take the item out
//     of its epic or its parent, and /remove_child and /remove_child_epic
//     take a child out of the item, which group.epic_issue_remove asks about,
//     and so do issue.update with epic_id 0 and issue.work_item_update and
//     group.epic_update with parent_id 0,
//   - /unassign removes every assignee of the item when it names nobody, and
//     on an item that takes a single assignee, as every issue and merge
//     request on a Free instance does, whatever it names; only on an item
//     that takes several does it remove just the users it names. Every
//     /unassign is guarded, because which kind the item is depends on the
//     instance's license, which the guard does not read. Removing every
//     assignee is what issue.update and merge_request.update with
//     assignee_ids [] or [0] or assignee_id 0, and issue.work_item_update
//     and group.epic_update with assignee_ids [], ask about,
//   - /reassign naming nobody, such as "/reassign" or "/reassign , and",
//     sets the assignees to none, the same removal,
//   - /remove_contacts removes CRM contacts from the item, which
//     issue.work_item_update with crm_contact_ids [] asks about.
//
// The groups from /unapprove on are undone by the opposite command. They are
// guarded all the same, because the rule is the catalog's and not this
// file's: the action that does the same asks, so the text that does it asks.
//
// The rest of GitLab's commands are left alone. Most do what an action here
// does unasked, and are undone the same way: /close and /reopen, /assign,
// /reassign followed by the users it sets, /label and /unlabel, /milestone,
// /due, /lock, /confidential, /approve, /spend and /remove_time_spent,
// /rebase, /add_contacts, and the conversions /type, /promote_to and
// /promote_to_incident, which change the type issue.update changes through
// issue_type. A few, such as /clone and /page, have no action here to
// compare them with.
//
// /award and /react are the one stated exception to the catalog's rule. They
// toggle a reaction on the item (AwardEmojis::ToggleService in GitLab), so on
// an item the caller has already reacted to with that emoji they remove the
// reaction, which is what issue.emoji_issue_delete and
// merge_request.emoji_mr_delete ask about. They are not guarded: removing a
// reaction is undone at once by adding it again, and a question before every
// reaction would be one nobody reads. The test of the action catalog declares
// the pair, so the exception is a decision and not an oversight.
//
// Of the 26 actions that send such a text, the guard is the only question
// 22 ask; issue.update, merge_request.update, issue.work_item_update and
// group.epic_update also ask before the fields that do what /remove_epic,
// /remove_parent, /unassign and /remove_contacts do.
var guardedQuickActions = []string{
	quickActionMove, quickActionMerge, quickActionPromote, quickActionTicket, quickActionAddEmail, quickActionPublish,
	quickActionUnapprove, quickActionUnlink,
	quickActionRemoveEpic, quickActionRemoveParent, quickActionRemoveParentEpic,
	quickActionRemoveChild, quickActionRemoveChildEpic,
	quickActionUnassign, quickActionReassign, quickActionRemoveContacts,
}

// Names of the guarded quick actions, as GitLab spells them after the slash.
const (
	quickActionMove             = "move"
	quickActionMerge            = "merge"
	quickActionPromote          = "promote"
	quickActionTicket           = "convert_to_ticket"
	quickActionAddEmail         = "add_email"
	quickActionPublish          = "publish"
	quickActionUnapprove        = "unapprove"
	quickActionUnlink           = "unlink"
	quickActionRemoveEpic       = "remove_epic"
	quickActionRemoveParent     = "remove_parent"
	quickActionRemoveParentEpic = "remove_parent_epic"
	quickActionRemoveChild      = "remove_child"
	quickActionRemoveChildEpic  = "remove_child_epic"
	quickActionUnassign         = "unassign"
	quickActionReassign         = "reassign"
	quickActionRemoveContacts   = "remove_contacts"
)

// quickActionEffect says what a guarded command does, quoting its line. Each
// key is written out where it is rendered, a chain of ifs rather than a table
// of keys, so the gates that read the message catalog see which template the
// line lands in, and each branch is its own statement in a coverage report.
func quickActionEffect(command, line string) i18n.Message {
	if command == quickActionMove {
		return i18n.New(i18n.ConfirmQuickActionMove, i18n.S("line", line))
	}
	if command == quickActionMerge {
		return i18n.New(i18n.ConfirmQuickActionMerge, i18n.S("line", line))
	}
	if command == quickActionPromote {
		return i18n.New(i18n.ConfirmQuickActionPromote, i18n.S("line", line))
	}
	if command == quickActionTicket {
		return i18n.New(i18n.ConfirmQuickActionTicket, i18n.S("line", line))
	}
	if command == quickActionAddEmail {
		return i18n.New(i18n.ConfirmQuickActionAddEmail, i18n.S("line", line))
	}
	if command == quickActionPublish {
		return i18n.New(i18n.ConfirmQuickActionPublish, i18n.S("line", line))
	}
	if command == quickActionUnapprove {
		return i18n.New(i18n.ConfirmQuickActionUnapprove, i18n.S("line", line))
	}
	if command == quickActionUnlink {
		return i18n.New(i18n.ConfirmQuickActionUnlink, i18n.S("line", line))
	}
	if command == quickActionRemoveChild || command == quickActionRemoveChildEpic {
		return i18n.New(i18n.ConfirmQuickActionRemoveChild, i18n.S("line", line))
	}
	if command == quickActionUnassign {
		return i18n.New(i18n.ConfirmQuickActionUnassign, i18n.S("line", line))
	}
	if command == quickActionReassign {
		return i18n.New(i18n.ConfirmQuickActionReassign, i18n.S("line", line))
	}
	if command == quickActionRemoveContacts {
		return i18n.New(i18n.ConfirmQuickActionRemoveContacts, i18n.S("line", line))
	}
	return i18n.New(i18n.ConfirmQuickActionRemoveParent, i18n.S("line", line))
}

// quickActionEffectKey names the kind of effect a guarded command has: the
// command itself, except for the two groups [quickActionEffect] says the same
// of, the commands that take the item out of its epic or its parent and the
// ones that take a child out of the item.
func quickActionEffectKey(command string) string {
	if command == quickActionRemoveParent || command == quickActionRemoveParentEpic {
		return quickActionRemoveEpic
	}
	if command == quickActionRemoveChildEpic {
		return quickActionRemoveChild
	}
	return command
}

// quotedQuickActionLines picks the lines the question quotes, in the order of
// the text: the first line of every kind of effect, so that no effect is
// left to a count, and then the lines that come first among the rest until
// [quickActionLinesShown] are quoted. Every line it leaves out has an effect
// of a kind a quoted line already has.
func quotedQuickActionLines(lines []QuickActionLine) []QuickActionLine {
	quoted := make([]bool, len(lines))
	kinds := map[string]struct{}{}
	count := 0
	for i, line := range lines {
		kind := quickActionEffectKey(line.Command)
		if _, twice := kinds[kind]; twice {
			continue
		}
		kinds[kind] = struct{}{}
		quoted[i] = true
		count++
	}
	for i := range lines {
		if count >= quickActionLinesShown {
			break
		}
		if !quoted[i] {
			quoted[i] = true
			count++
		}
	}
	shown := make([]QuickActionLine, 0, count)
	for i, line := range lines {
		if quoted[i] {
			shown = append(shown, line)
		}
	}
	return shown
}

// QuickActionLine is one line of a text that begins with a quick action the
// guard asks about: the command, in lower case and without its slash, and
// the line as the text wrote it, without the space around it.
type QuickActionLine struct {
	Command string
	Line    string
}

// GuardedQuickActionLines returns the lines of the texts that begin with a
// quick action the guard asks about, in the order they appear, each distinct
// line once.
//
// It reads a text the way GitLab does, and wider wherever being wider costs a
// question and being narrower would cost the guard. GitLab drops every
// carriage return before it looks, so they are dropped here, and a command
// split by one is found. A command is the slash at the start of a line, the
// name in any case, and then the end of the line or a space. GitLab compares
// the name under Unicode case folding, so the Kelvin sign is a k and the long
// s an s, and so does [strings.EqualFold]. Space before the slash is skipped,
// which GitLab does not do, and so is a NUL byte, which GitLab refuses the
// whole request for. A handler that passes its text through [NormalizeText]
// sends a line end where the call wrote a backslash and an n, so a text is
// read both as written and as rewritten. A line inside a code block, a quote
// block fenced with >>> or an HTML block is read as any other: GitLab does not
// run those, and telling them apart takes its Markdown parser, so such a line
// costs a question nobody needed rather than one nobody was asked. A line
// quoted with a > of its own does not begin with the slash, and is not a
// command here or in GitLab.
func GuardedQuickActionLines(texts ...string) []QuickActionLine {
	var found []QuickActionLine
	seen := map[string]struct{}{}
	dropped := strings.NewReplacer("\r", "", "\x00", "")
	for _, text := range texts {
		// Both spellings are read: the text as the call wrote it, and as
		// [NormalizeText] rewrites it, which is what most handlers send.
		for _, spelling := range []string{text, NormalizeText(text)} {
			for line := range strings.SplitSeq(dropped.Replace(spelling), "\n") {
				line = strings.TrimSpace(line)
				command, ok := guardedQuickAction(line)
				if !ok {
					continue
				}
				if _, twice := seen[line]; twice {
					continue
				}
				seen[line] = struct{}{}
				found = append(found, QuickActionLine{Command: command, Line: line})
			}
		}
	}
	return found
}

// guardedQuickAction reports the guarded command a trimmed line begins with:
// a slash, the command's name in any case, and nothing or a space after it.
//
// /reassign is guarded only when it names nobody, which is when GitLab sets
// the assignees to the empty list; followed by users it sets them, which is
// what issue.update does unasked with another list.
func guardedQuickAction(line string) (string, bool) {
	rest, slash := strings.CutPrefix(line, "/")
	if !slash {
		return "", false
	}
	name, args := rest, ""
	if end := strings.IndexFunc(rest, unicode.IsSpace); end >= 0 {
		name, args = rest[:end], rest[end:]
	}
	for _, command := range guardedQuickActions {
		if !strings.EqualFold(name, command) {
			continue
		}
		if command == quickActionReassign && namesUsers(args) {
			return "", false
		}
		return command, true
	}
	return "", false
}

// namesUsers reports whether the arguments of a quick action that takes
// users name any, read the way GitLab's users extractor reads them: split at
// every space and comma, with the empty pieces and the word "and" dropped.
// "/reassign , and" names nobody, and GitLab runs it as "/reassign" alone.
// The split is at every Unicode space, which GitLab's is not, but GitLab
// also drops a piece that is only such spaces, so no text names users here
// that names none there. Whether a name GitLab would resolve is not read: a
// name it does not resolve fails the command, which then changes nothing.
func namesUsers(args string) bool {
	for piece := range strings.FieldsFuncSeq(args, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		if piece != "and" {
			return true
		}
	}
	return false
}

// ConfirmQuickActions asks the caller to confirm a call whose text carries a
// quick action the guard asks about, and returns nil when the call may
// proceed. op is the operation label the refusals open with, the handler's
// own, and texts are the note body or the description the call sends, the
// empty string for one it does not send. A text without such a line asks
// nothing, which is every ordinary note and description.
//
// Sending a note or a description is not destructive, and is deliberately
// not classified as such: a prompt in front of every comment is one nobody
// reads. The guard asks only for the lines [GuardedQuickActionLines] finds,
// because GitLab runs them with the caller's permissions when it saves the
// text, and what they do is what an action here asks about before it does
// the same. A text is also the parameter a model most readily fills with
// something it read, so a command in it need not be one the user wrote.
//
// The arguments alone decide, so nothing is read: whether GitLab would run
// the command depends on the type of the item, its state and the caller's
// role, and a guard that asked only when it would has to know all three. The
// question quotes the lines, each with an address on it written by its origin
// alone, as the other guards write one, cut to a length a dialog shows and
// escaped so it cannot pass for the server's own words, and no other part of
// the text. It quotes every kind of effect the text carries and at least
// [quickActionLinesShown] lines, and counts the rest, each of a kind already
// quoted, so a long run of one command cannot keep another out of the
// question.
func ConfirmQuickActions(ctx context.Context, op string, texts ...string) error {
	lines := GuardedQuickActionLines(texts...)
	if len(lines) == 0 {
		return nil
	}
	shown := quotedQuickActionLines(lines)
	effects := make([]i18n.Message, 0, len(shown)+1)
	for _, line := range shown {
		effects = append(effects, quickActionEffect(line.Command, EscapeConsentValue(truncateRunes(redactLineURLs(line.Line), quickActionLineRunes))))
	}
	if more := len(lines) - len(shown); more > 0 {
		effects = append(effects, i18n.New(i18n.ConfirmQuickActionMore, i18n.N("count", int64(more))))
	}
	question := i18n.New(i18n.ConfirmQuickActions, i18n.L("effects", effects...))
	return ConfirmGuardedCall(ctx, op, QuickActionConfirmID, question,
		"the user declined running the quick actions in the text; send the text without the lines that begin with them")
}

// ConfirmQuickActionsOptional is [ConfirmQuickActions] for the one text of an
// edit that leaves a text out by sending no key: nil is a text the call does
// not send, which asks nothing.
func ConfirmQuickActionsOptional(ctx context.Context, op string, text *string) error {
	if text == nil {
		return nil
	}
	return ConfirmQuickActions(ctx, op, *text)
}

// lineURL matches an address on a quoted line: a scheme, in any case, its
// "://" and everything up to the next space, wherever the scheme starts, so
// an address glued to an underscore, a digit or Markdown emphasis is matched
// as well; or a scheme-relative "//" and what follows it up to the next
// space, which has no origin to write and is written as the placeholder.
var lineURL = regexp.MustCompile(`(?i)[a-z][a-z0-9+.-]*://\S+|//[^\s/]\S*`)

// redactLineURLs writes every address on a line by its origin alone, as
// [RedactURLToOrigin] reduces it, and leaves the rest of the line as it is.
// The guarded commands take a path, a reference or a name, so an address on
// one is never what a person needs to decide on, and its userinfo, path and
// query are where a credential sits. It runs before the line is cut, so a cut
// cannot leave the front of an address that no longer parses.
func redactLineURLs(line string) string {
	return lineURL.ReplaceAllStringFunc(line, RedactURLToOrigin)
}

// truncateRunes cuts s to at most limit runes, marking a cut with an
// ellipsis, and never splits a rune.
func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}
