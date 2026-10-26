// report_test.go covers what a run prints: one line per commit with the
// reason under each that is not simply signed off, the count, how to sign a
// failing commit off, the workflow commands of -annotate, and the escaping of
// what a commit's author chose to write.
package main

import (
	"bytes"
	"strings"
	"testing"
)

// hashOf is a forty-character hash made of one repeated digit, so a report
// line shows which commit it is.
func hashOf(digit string) string { return strings.Repeat(digit, 40) }

// everyOutcome is one verdict of each outcome, a failing commit carrying a
// bot's identity included.
func everyOutcome() []verdict {
	bot := declaredBots[0]
	return []verdict{
		{commit: commit{hash: hashOf("1"), author: jane, subject: "feat: one"}, outcome: signedOff},
		{commit: commit{hash: hashOf("2"), author: jane, subject: "fix: two"}, outcome: unsigned},
		{commit: commit{hash: hashOf("3"), author: john, subject: "docs: three", signoffs: []string{"Jane Doe <jane@example.org>", "Jane"}}, outcome: signedByOthers},
		{commit: commit{hash: hashOf("4"), author: dependabot, subject: "chore(deps): bump"}, outcome: exempt, bot: &bot},
		{commit: commit{hash: hashOf("5"), parents: 2, author: jane, subject: "Merge branch 'side'", cleanMerge: true}, outcome: merge},
		{commit: commit{hash: hashOf("6"), author: dependabot, subject: "chore(deps): copied"}, outcome: unsigned, bot: &bot},
	}
}

// TestReport_Write_EveryOutcome_WordsEachCommitAndSaysHowToSignOff pins the
// whole report of a run with one commit of each outcome: the header, each
// commit's line and reasons, the count, and the remedy naming the base.
func TestReport_Write_EveryOutcome_WordsEachCommitAndSaysHowToSignOff(t *testing.T) {
	var out bytes.Buffer
	r := report{span: "origin/main..HEAD (aaaaaaaaaaaa..bbbbbbbbbbbb)", base: "origin/main", openedBy: "jmrplens", verdicts: everyOutcome()}
	if !r.write(&out) {
		t.Error("write() = false, want true for a run with failing commits")
	}
	want := `audit_dco: 6 commit(s) in origin/main..HEAD (aaaaaaaaaaaa..bbbbbbbbbbbb)
  ok      111111111111  Jane Doe <jane@example.org>  feat: one
  FAIL    222222222222  Jane Doe <jane@example.org>  fix: two
          carries no Signed-off-by trailer
  FAIL    333333333333  John Roe <john@example.org>  docs: three
          is signed off by Jane Doe <jane@example.org>; Jane, and not by its author
  exempt  444444444444  dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>  chore(deps): bump
          a commit of dependabot[bot], declared in bots.go, in a pull request dependabot[bot] opened
  merge   555555555555  Jane Doe <jane@example.org>  Merge branch 'side'
          a merge commit that brings no change of its own, which is not judged
  FAIL    666666666666  dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>  chore(deps): copied
          carries no Signed-off-by trailer
          carries the identity of dependabot[bot], whose commits are exempt only in a pull request dependabot[bot] opened, and jmrplens opened this one
audit_dco: 1 signed off by the author, 3 not, 1 exempt, 1 merge(s) not judged

Each commit marked FAIL needs a Signed-off-by trailer naming its author, which
is how the author certifies the Developer Certificate of Origin
(https://developercertificate.org/) for it, as CONTRIBUTING.md sets out under
Developer Certificate of Origin. To sign off the commits you authored where
they stand, and push the branch again:

    git rebase --keep-base --signoff origin/main
    git push --force-with-lease

Then commit with git commit --signoff (-s). A commit somebody else authored is
signed off by that person. A rebase does not keep a merge commit, so a merge
marked FAIL, which brings a change of its own, is signed off with git commit
--amend --no-edit --signoff while it is the branch's last commit. In a stack,
run the rebase on the top layer with --update-refs as well, so the branch of
every layer below moves with it, and push each layer.
`
	if got := out.String(); got != want {
		t.Errorf("write() printed:\n%s\nwant:\n%s", got, want)
	}
}

// TestReport_Write_Annotate_AddsAnErrorCommandPerFailingCommit verifies that
// -annotate adds one GitHub Actions error command per failing commit, after
// the report, with every reason of the commit in it, and none for a commit
// that passes, is exempt or is a merge.
func TestReport_Write_Annotate_AddsAnErrorCommandPerFailingCommit(t *testing.T) {
	var out bytes.Buffer
	r := report{span: "a..b", base: "a", verdicts: everyOutcome(), annotate: true}
	r.write(&out)
	var commands []string
	for line := range strings.SplitSeq(out.String(), "\n") {
		if strings.HasPrefix(line, "::") {
			commands = append(commands, line)
		}
	}
	want := []string{
		"::error title=DCO sign-off::222222222222 by Jane Doe <jane@example.org> carries no Signed-off-by trailer",
		"::error title=DCO sign-off::333333333333 by John Roe <john@example.org> is signed off by Jane Doe <jane@example.org>; Jane, and not by its author",
		"::error title=DCO sign-off::666666666666 by dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com> carries no Signed-off-by trailer; carries the identity of dependabot[bot], whose commits are exempt only in a pull request dependabot[bot] opened, and no -opened-by names who opened this one",
	}
	if strings.Join(commands, "\n") != strings.Join(want, "\n") {
		t.Errorf("workflow commands:\n%s\nwant:\n%s", strings.Join(commands, "\n"), strings.Join(want, "\n"))
	}
	if !strings.HasSuffix(out.String(), want[len(want)-1]+"\n") {
		t.Errorf("the workflow commands do not come last:\n%s", out.String())
	}
}

// TestReport_Write_NoFailure_PrintsTheCountAndNoRemedy verifies that a run
// whose every commit holds says so with the count, and prints neither the
// remedy nor a workflow command even under -annotate.
func TestReport_Write_NoFailure_PrintsTheCountAndNoRemedy(t *testing.T) {
	var out bytes.Buffer
	verdicts := everyOutcome()
	r := report{span: "a..b", base: "a", verdicts: []verdict{verdicts[0], verdicts[3], verdicts[4]}, annotate: true}
	if r.write(&out) {
		t.Error("write() = true, want false when no commit fails")
	}
	want := `audit_dco: 3 commit(s) in a..b
  ok      111111111111  Jane Doe <jane@example.org>  feat: one
  exempt  444444444444  dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>  chore(deps): bump
          a commit of dependabot[bot], declared in bots.go, in a pull request dependabot[bot] opened
  merge   555555555555  Jane Doe <jane@example.org>  Merge branch 'side'
          a merge commit that brings no change of its own, which is not judged
audit_dco: 1 signed off by the author, 0 not, 1 exempt, 1 merge(s) not judged
`
	if got := out.String(); got != want {
		t.Errorf("write() printed:\n%s\nwant:\n%s", got, want)
	}
}

// TestReport_Write_OnlySignedByOthers_CountsThemAsNotSignedOff verifies the
// count of a run whose one failing commit is signed off by somebody else,
// which the count adds to the commits with no trailer.
func TestReport_Write_OnlySignedByOthers_CountsThemAsNotSignedOff(t *testing.T) {
	var out bytes.Buffer
	r := report{span: "a..b", base: "a", verdicts: []verdict{everyOutcome()[2]}}
	if !r.write(&out) {
		t.Error("write() = false, want true")
	}
	if !strings.Contains(out.String(), "audit_dco: 0 signed off by the author, 1 not, 0 exempt, 0 merge(s) not judged\n") {
		t.Errorf("write() printed:\n%s\nwant the count of one commit not signed off", out.String())
	}
}

// TestReport_Write_NoCommits_SaysThereIsNothingToJudge verifies the report of
// an empty range: one line, and no failure.
func TestReport_Write_NoCommits_SaysThereIsNothingToJudge(t *testing.T) {
	var out bytes.Buffer
	if (report{span: "a..a"}).write(&out) {
		t.Error("write() = true, want false for an empty range")
	}
	if got, want := out.String(), "audit_dco: no commits in a..a, so nothing to judge\n"; got != want {
		t.Errorf("write() printed %q, want %q", got, want)
	}
}

// TestReport_Write_AuthorText_CannotStartALineOrBreakOne verifies that what a
// commit's author wrote, the subject, the identity, a trailer value and the
// opener, reaches the report with its control characters replaced, so none of
// it can end a line and start a workflow command of its own, and that the
// annotation escapes the percent sign GitHub decodes.
func TestReport_Write_AuthorText_CannotStartALineOrBreakOne(t *testing.T) {
	bot := declaredBots[0]
	crafted := commit{
		hash:     hashOf("7"),
		author:   identity{name: "Jane\rDoe", email: "jane@example.org"},
		subject:  "fix: 100%\n::warning::forged",
		signoffs: []string{"John\x1bRoe 100% <john@example.org>"},
	}
	var out bytes.Buffer
	r := report{
		span: "a..b", base: "a\nb", openedBy: "x\ny", annotate: true,
		verdicts: []verdict{{commit: crafted, outcome: signedByOthers, bot: &bot}},
	}
	r.write(&out)
	for line := range strings.SplitSeq(out.String(), "\n") {
		if strings.HasPrefix(line, "::") && !strings.HasPrefix(line, "::error title=DCO sign-off::") {
			t.Errorf("a line starts a workflow command the report did not write: %q", line)
		}
	}
	tests := []struct{ name, want string }{
		{name: "author and subject", want: "Jane\uFFFDDoe <jane@example.org>  fix: 100%\uFFFD::warning::forged"},
		{name: "trailer value", want: "is signed off by John\uFFFDRoe 100% <john@example.org>, and not by its author"},
		{name: "opener", want: "and x\uFFFDy opened this one"},
		{name: "base", want: "git rebase --keep-base --signoff a\uFFFDb"},
		{name: "annotation", want: "::error title=DCO sign-off::777777777777 by Jane\uFFFDDoe <jane@example.org> is signed off by John\uFFFDRoe 100%25 <john@example.org>, and not by its author;"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("the report does not hold %q:\n%s", tc.want, out.String())
			}
		})
	}
}

// TestPrintable_ControlCharacters_AreReplaced verifies that every control
// character, the line breaks, the escape character and DEL among them, is
// replaced, and that ordinary text, accents included, is kept as it is.
func TestPrintable_ControlCharacters_AreReplaced(t *testing.T) {
	if got, want := printable("a\nb\rc\td\x1be\x7ff\x00g"), "a\uFFFDb\uFFFDc\uFFFDd\uFFFDe\uFFFDf\uFFFDg"; got != want {
		t.Errorf("printable() = %q, want %q", got, want)
	}
	if got, want := printable("José M. Requena Plens <mail@jmrp.io>"), "José M. Requena Plens <mail@jmrp.io>"; got != want {
		t.Errorf("printable() = %q, want %q", got, want)
	}
}

// TestEscapeData_WorkflowCommandMessage_EscapesWhatGitHubDecodes verifies the
// escaping GitHub Actions decodes in a workflow command's message: the
// percent sign, carriage return and line feed.
func TestEscapeData_WorkflowCommandMessage_EscapesWhatGitHubDecodes(t *testing.T) {
	if got, want := escapeData("100% done\r\nnext"), "100%25 done%0D%0Anext"; got != want {
		t.Errorf("escapeData() = %q, want %q", got, want)
	}
}
