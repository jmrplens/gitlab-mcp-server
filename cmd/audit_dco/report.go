// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

// marks are the words a report line opens with, one per outcome.
var marks = map[outcome]string{
	signedOff:      "ok",
	unsigned:       "FAIL",
	signedByOthers: "FAIL",
	exempt:         "exempt",
	merge:          "merge",
}

// remedy is what a failing run prints after the count, with the base the
// branch is rebased on in place of its one verb.
const remedy = `
Each commit marked FAIL needs a Signed-off-by trailer naming its author, which
is how the author certifies the Developer Certificate of Origin
(https://developercertificate.org/) for it, as CONTRIBUTING.md sets out under
Developer Certificate of Origin. To sign off the commits you authored where
they stand, and push the branch again:

    git rebase --keep-base --signoff %s
    git push --force-with-lease

Then commit with git commit --signoff (-s). A commit somebody else authored is
signed off by that person. A rebase does not keep a merge commit, so a merge
marked FAIL, which brings a change of its own, is signed off with git commit
--amend --no-edit --signoff while it is the branch's last commit. In a stack,
run the rebase on the top layer with --update-refs as well, so the branch of
every layer below moves with it, and push each layer.
`

// annotationTitle is the title of the workflow command a failing commit gets
// under -annotate.
const annotationTitle = "DCO sign-off"

// report is what a run judged and how to word it: the range as the command
// line named it and as commits, the base the remedy rebases on, the login
// that opened the pull request, the verdicts in order, and whether to add a
// workflow command per failing commit.
type report struct {
	span     string
	base     string
	openedBy string
	verdicts []verdict
	annotate bool
}

// write prints the report and returns whether a commit failed: a line per
// commit with the reasons under it, the count, and for a failing run the
// remedy and, under annotate, one error command per failing commit. What a
// commit's author wrote is printed through printable, so none of it can end a
// line and start a workflow command of its own.
func (r report) write(w io.Writer) bool {
	if len(r.verdicts) == 0 {
		_, _ = fmt.Fprintf(w, "%s: no commits in %s, so nothing to judge\n", toolName, r.span)
		return false
	}
	_, _ = fmt.Fprintf(w, "%s: %d commit(s) in %s\n", toolName, len(r.verdicts), r.span)
	counts := map[outcome]int{}
	for _, v := range r.verdicts {
		counts[v.outcome]++
		_, _ = fmt.Fprintf(w, "  %-7s %s  %s  %s\n", marks[v.outcome], v.commit.short(), printable(v.commit.author.String()), printable(v.commit.subject))
		for _, reason := range reasons(v, r.openedBy) {
			_, _ = fmt.Fprintf(w, "          %s\n", reason)
		}
	}
	failing := counts[unsigned] + counts[signedByOthers]
	_, _ = fmt.Fprintf(w, "%s: %d signed off by the author, %d not, %d exempt, %d merge(s) not judged\n",
		toolName, counts[signedOff], failing, counts[exempt], counts[merge])
	if failing == 0 {
		return false
	}
	_, _ = fmt.Fprintf(w, remedy, printable(r.base))
	if r.annotate {
		for _, v := range r.verdicts {
			if v.failed() {
				message := v.commit.short() + " by " + printable(v.commit.author.String()) + " " + strings.Join(reasons(v, r.openedBy), "; ")
				_, _ = fmt.Fprintf(w, "::error title=%s::%s\n", annotationTitle, escapeData(message))
			}
		}
	}
	return true
}

// reasons words why a commit reached its outcome, one line each: nothing for
// a commit signed off by its author, and for a failing commit carrying a
// declared bot's identity, why the bot's exemption did not hold.
func reasons(v verdict, openedBy string) []string {
	var lines []string
	switch v.outcome {
	case unsigned:
		lines = append(lines, "carries no Signed-off-by trailer")
	case signedByOthers:
		lines = append(lines, "is signed off by "+printable(strings.Join(v.commit.signoffs, "; "))+", and not by its author")
	case exempt:
		return []string{fmt.Sprintf("a commit of %[1]s, declared in bots.go, in a pull request %[1]s opened", v.bot.login)}
	case merge:
		return []string{"a merge commit that brings no change of its own, which is not judged"}
	default:
		return nil
	}
	if v.bot != nil {
		opener := "no -opened-by names who opened this one"
		if openedBy != "" {
			opener = printable(openedBy) + " opened this one"
		}
		lines = append(lines, fmt.Sprintf("carries the identity of %[1]s, whose commits are exempt only in a pull request %[1]s opened, and %[2]s", v.bot.login, opener))
	}
	return lines
}

// printable replaces every control character of s with U+FFFD, so that text a
// commit's author chose can neither end a report line nor start one.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return unicode.ReplacementChar
		}
		return r
	}, s)
}

// workflowData escapes the characters GitHub Actions decodes in the message of
// a workflow command.
var workflowData = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")

// escapeData escapes s for the message of a workflow command.
func escapeData(s string) string { return workflowData.Replace(s) }
