// Tests for the seventh property: the release VERSION names opens with a
// written summary, kept in .github/release-notes, which carries the four
// sections a reader needs to decide whether to upgrade and nothing the
// release body may not publish.
package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// validReleaseNotes is the smallest summary the rule accepts: the four
// sections, in order, each with something under it.
const validReleaseNotes = "## Summary\n\nWhat changed, for whom.\n\n" +
	"## Breaking changes\n\nNone.\n\n" +
	"## Upgrading\n\nNone.\n\n" +
	"## Security\n\nNone.\n"

// The characters and commands the line rule refuses, built from their parts
// so that this file does not carry what it tests for.
var (
	emDash = string(rune(0x2014))
	enDash = string(rune(0x2013))
)

// skipCommandText is one of the bracketed commands GitHub reads as a request
// to skip every workflow, in the spelling given.
func skipCommandText(command string) string {
	return "[" + command + "]"
}

// writeReleaseNotes writes one file of .github/release-notes under root.
func writeReleaseNotes(t *testing.T, root, name, body string) {
	t.Helper()

	writeFile(t, root, releaseNotesDir+"/"+name, body)
}

// missingNotesFinding renders the finding a VERSION with no notes carries.
func missingNotesFinding(version string) string {
	return releaseNotesDir + "/v" + version + ".md: missing: every release opens with a written summary " +
		"of what changed, what breaks, how to upgrade and which vulnerabilities it fixes, and the release " +
		"workflow refuses a tag whose notes are not here (docs/development/release-process.md)"
}

// TestCheckReleaseNotes_CurrentVersion_MustHaveItsFile verifies that the
// release VERSION names is refused until its notes exist, whether the
// directory is missing altogether or holds only the notes of other releases.
//
// The file is what the release workflow passes GoReleaser as the release
// header, and GoReleaser reads it only outside a snapshot, so a rehearsal
// cannot be relied on to notice it is missing; this gate runs on every pull
// request, including the one that moves VERSION, which is where the notes are
// written.
func TestCheckReleaseNotes_CurrentVersion_MustHaveItsFile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{name: "no directory at all", want: []string{missingNotesFinding("3.4.0")}},
		{
			name:  "only the notes of earlier releases",
			files: map[string]string{"v3.3.0.md": validReleaseNotes},
			want:  []string{missingNotesFinding("3.4.0")},
		},
		{
			name:  "the notes of this release",
			files: map[string]string{"v3.3.0.md": validReleaseNotes, "v3.4.0.md": validReleaseNotes},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			for name, body := range testCase.files {
				writeReleaseNotes(t, root, name, body)
			}
			got, err := checkReleaseNotes(root, "3.4.0\n")
			if err != nil {
				t.Fatalf("checkReleaseNotes: %v", err)
			}
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkReleaseNotes() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// TestCheckReleaseNotes_EveryFile_IsJudged verifies that every file of the
// directory is held to the rule, not only the current release's, and that an
// entry which is not a release's notes is named.
//
// The notes of a published release are what its GitHub release opens with, so
// an edit to an older file is published too, and a stray file in the directory
// is one no release will ever read. The findings come in the directory's
// order, the current release's absence first.
func TestCheckReleaseNotes_EveryFile_IsJudged(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeReleaseNotes(t, root, "v3.4.0.md", validReleaseNotes)
	writeReleaseNotes(t, root, "v3.3.0.md", strings.Replace(validReleaseNotes, "## Upgrading\n\nNone.\n\n", "", 1))
	writeReleaseNotes(t, root, "draft.md", validReleaseNotes)
	writeReleaseNotes(t, root, "v3.5.md", validReleaseNotes)
	writeReleaseNotes(t, root, "v3.6.0/README.md", validReleaseNotes)
	writeReleaseNotes(t, root, "v3.7.0.md/README.md", validReleaseNotes)

	got, err := checkReleaseNotes(root, "3.4.0")
	if err != nil {
		t.Fatalf("checkReleaseNotes: %v", err)
	}
	stray := ": is not the notes of a release: the directory holds one vX.Y.Z.md per release and nothing else"
	want := []string{
		releaseNotesDir + "/draft.md" + stray,
		releaseNotesDir + "/v3.3.0.md: has no ## Upgrading section",
		releaseNotesDir + "/v3.5.md" + stray,
		releaseNotesDir + "/v3.6.0" + stray,
		releaseNotesDir + "/v3.7.0.md" + stray,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkReleaseNotes() = %#v, want %#v", got, want)
	}

	missing, missingErr := checkReleaseNotes(root, "3.5.0")
	if missingErr != nil || len(missing) == 0 || missing[0] != missingNotesFinding("3.5.0") {
		t.Errorf("checkReleaseNotes() for 3.5.0 = %#v (%v), want the missing finding first", missing, missingErr)
	}
}

// TestCheckReleaseNotes_UnreadableInput_IsAnError verifies that notes the
// audit cannot read stop it rather than passing: a directory that is a file,
// a path that cannot be stated, and a file name that resolves to nothing.
//
// A file in the directory's place is refused as errNotADirectory on every
// system. os.ReadDir alone does not refuse it the same way everywhere: on
// windows-latest the audit read such a file as no notes at all and passed.
func TestCheckReleaseNotes_UnreadableInput_IsAnError(t *testing.T) {
	t.Parallel()

	t.Run("the directory is a file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, root, releaseNotesDir, validReleaseNotes)
		if _, err := checkReleaseNotes(root, "3.4.0"); !errors.Is(err, errNotADirectory) {
			t.Errorf("checkReleaseNotes() error = %v, want %v when the directory is a file", err, errNotADirectory)
		}
	})

	t.Run("a path that cannot be stated", func(t *testing.T) {
		t.Parallel()

		// A NUL byte is refused by the system call on every platform, with
		// an error that is not a missing file's.
		if _, err := checkReleaseNotes(t.TempDir()+"\x00", "3.4.0"); err == nil {
			t.Error("checkReleaseNotes() = nil error, want a failure for a path that cannot be stated")
		}
	})

	t.Run("a file that does not read", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeReleaseNotes(t, root, "v3.3.0.md", validReleaseNotes)
		directory := filepath.Join(root, filepath.FromSlash(releaseNotesDir))
		if err := os.Symlink(filepath.Join(directory, "never-written.md"), filepath.Join(directory, "v3.4.0.md")); err != nil {
			t.Skipf("this platform will not create a symlink: %v", err)
		}
		if _, err := checkReleaseNotes(root, "3.4.0"); err == nil {
			t.Error("checkReleaseNotes() = nil error, want a failure for notes that cannot be read")
		}
	})
}

// TestReleaseNotesProblems_Sections_AreTheFourInOrder verifies that a summary
// carries Summary, Breaking changes, Upgrading and Security, once each, in
// that order, each with something under it, and opens with the first.
//
// The four are what a reader needs to decide whether to upgrade and what the
// upgrade costs, and the release body continues below them with GoReleaser's
// own commit list under ## Changelog, so a fifth section, or text above the
// first, would read as part of that list rather than of the summary. A heading
// inside a fenced code block is code, not a section.
func TestReleaseNotesProblems_Sections_AreTheFourInOrder(t *testing.T) {
	t.Parallel()

	const label = "notes.md"
	cases := []struct {
		name string
		text string
		want []string
	}{
		{name: "the four sections", text: validReleaseNotes},
		{
			name: "subsections and lists inside a section",
			text: strings.Replace(validReleaseNotes, "What changed, for whom.\n", "### Highlights\n\n- one\n- two\n", 1),
		},
		{name: "empty", text: " \n\n\t\n", want: []string{label + ": is empty: write the four sections, None. where there is nothing to say"}},
		{
			name: "a section missing",
			text: strings.Replace(validReleaseNotes, "## Security\n\nNone.\n", "", 1),
			want: []string{label + ": has no ## Security section"},
		},
		{
			name: "every section missing",
			text: "Just a paragraph.\n",
			want: []string{
				label + ":1: opens with text before its first section, where the release body has to open with ## Summary",
				label + ": has no ## Summary section",
				label + ": has no ## Breaking changes section",
				label + ": has no ## Upgrading section",
				label + ": has no ## Security section",
			},
		},
		{
			name: "a section the rule does not define",
			text: validReleaseNotes + "\n## Changelog\n\n- a commit\n",
			want: []string{label + ":17: ## Changelog is not one of the four sections (Summary, Breaking changes, Upgrading, Security)"},
		},
		{
			name: "a section twice",
			text: validReleaseNotes + "\n## Upgrading\n\nAgain.\n",
			want: []string{label + ":17: ## Upgrading appears a second time"},
		},
		{
			name: "an empty section",
			text: strings.Replace(validReleaseNotes, "## Breaking changes\n\nNone.\n\n", "## Breaking changes\n\n\n", 1),
			want: []string{label + ":5: ## Breaking changes is empty: write None. when there is nothing to say"},
		},
		{
			name: "the last section empty",
			text: strings.Replace(validReleaseNotes, "## Security\n\nNone.\n", "## Security\n", 1),
			want: []string{label + ":13: ## Security is empty: write None. when there is nothing to say"},
		},
		{
			name: "the sections out of order",
			text: "## Summary\n\nA.\n\n## Upgrading\n\nB.\n\n## Breaking changes\n\nC.\n\n## Security\n\nD.\n",
			want: []string{label + ": carries its sections in the order Summary, Upgrading, Breaking changes, Security, want Summary, Breaking changes, Upgrading, Security"},
		},
		{
			name: "the order of the sections present",
			text: "## Security\n\nD.\n\n## Summary\n\nA.\n",
			want: []string{
				label + ": has no ## Breaking changes section",
				label + ": has no ## Upgrading section",
				label + ": carries its sections in the order Security, Summary, want Summary, Breaking changes, Upgrading, Security",
			},
		},
		{
			name: "text before the first section",
			text: "\nRelease 3.4.0\nis out.\n\n" + validReleaseNotes,
			want: []string{label + ":2: opens with text before its first section, where the release body has to open with ## Summary"},
		},
		{
			name: "a heading inside a fenced block",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Upgrading", "```markdown\n## Changelog\n```\n\n## Upgrading", 1),
		},
		{
			name: "a heading inside a tilde fence",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Upgrading", "~~~\n## Changelog\n~~~\n\n## Upgrading", 1),
		},
		{
			name: "a shorter fence inside a longer one",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Upgrading", "````\n```\n## Changelog\n```\n````\n\n## Upgrading", 1),
		},
		{
			name: "a fence that a run of the other character does not close",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Upgrading", "```\n~~~\n## Changelog\n```\n\n## Upgrading", 1),
		},
		{
			name: "a fence closed by a longer run",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Security", "```\ncode\n`````\n\n## Changelog\n\nX.\n\n## Security", 1),
			want: []string{label + ":15: ## Changelog is not one of the four sections (Summary, Breaking changes, Upgrading, Security)"},
		},
		{
			name: "a fence line that is not a closing one",
			text: strings.Replace(validReleaseNotes, "None.\n\n## Upgrading", "```\n```go\n## Changelog\n```\n\n## Upgrading", 1),
		},
		{
			name: "a section whose only text is a fenced block",
			text: strings.Replace(validReleaseNotes, "## Upgrading\n\nNone.\n", "## Upgrading\n\n```\n```\n", 1),
		},
		{
			name: "a deeper heading is not a section",
			text: "### Summary\n\n" + validReleaseNotes,
			want: []string{label + ":1: opens with text before its first section, where the release body has to open with ## Summary"},
		},
		{
			name: "a heading with no space is not a section",
			text: strings.Replace(validReleaseNotes, "## Security", "##Security", 1),
			want: []string{label + ": has no ## Security section"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := releaseNotesProblems(label, testCase.text)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("releaseNotesProblems() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// TestReleaseNotesProblems_Lines_PublishNothingTheBodyMayNot verifies that a
// summary is refused for an em or en dash, for a command that makes GitHub
// skip workflows, and for a line crediting an assistant, each named by its
// line, and that the ordinary text around them passes.
//
// The release body is published under the maintainer's name, so it is held to
// the rules every other text this repository publishes is held to: no em or en
// dash, and no attribution of authorship to an assistant. A skip command is
// refused because the summary is the text most likely to be pasted into the
// pull request that moves VERSION, whose squash commit is the one the release
// tag is put on: the v3.1.0 tag went on such a commit (#1094) and ran no
// release.
func TestReleaseNotesProblems_Lines_PublishNothingTheBodyMayNot(t *testing.T) {
	t.Parallel()

	const label = "notes.md"
	dash := ": carries an em dash (U+2014): nothing published here uses one, so rewrite the line with a comma, a colon, parentheses or a full stop"
	en := ": carries an en dash (U+2013): nothing published here uses one, so rewrite the line with a comma, a colon, parentheses or a full stop"
	skip := ": carries a command that makes GitHub skip workflows: pasted into the pull request that moves VERSION, " +
		"it would run no CI on the commit the release tag goes on and no release from the tag"
	credit := func(what string) string {
		return ": carries " + what + ": the release body is published under the maintainer's name and credits no assistant with it"
	}
	cases := []struct {
		name string
		line string
		want []string
	}{
		{name: "plain prose", line: "Fixes a pagination bug in `issue.list` - see #12."},
		{name: "an em dash", line: "The guard " + emDash + " now stricter.", want: []string{label + ":3" + dash}},
		{name: "an en dash", line: "Versions 3.0" + enDash + "3.3.", want: []string{label + ":3" + en}},
		{name: "both dashes", line: emDash + enDash, want: []string{label + ":3" + dash, label + ":3" + en}},
		{name: "a bracketed skip command", line: "Merged as " + skipCommandText("skip ci") + ".", want: []string{label + ":3" + skip}},
		{name: "a skip command in another case and spacing", line: skipCommandText(" CI   Skip "), want: []string{label + ":3" + skip}},
		{name: "every spelling GitHub reads: no ci", line: skipCommandText("no ci"), want: []string{label + ":3" + skip}},
		{name: "every spelling GitHub reads: skip actions", line: skipCommandText("skip actions"), want: []string{label + ":3" + skip}},
		{name: "every spelling GitHub reads: actions skip", line: skipCommandText("actions skip"), want: []string{label + ":3" + skip}},
		{name: "the skip-checks trailer", line: "skip-checks: true", want: []string{label + ":3" + skip}},
		{name: "the skip-checks trailer indented in backticks", line: "  `skip-checks:true`", want: []string{label + ":3" + skip}},
		{name: "the skip-checks trailer named in prose", line: "A line reading skip-checks: true is refused."},
		{name: "the command named without brackets", line: "A skip ci command is refused."},
		{name: "a co-author trailer", line: "Co-Authored-By: Someone <someone@example.com>", want: []string{label + ":3" + credit("a Co-Authored-By trailer")}},
		{name: "a co-author trailer indented", line: "  co-authored-by:x", want: []string{label + ":3" + credit("a Co-Authored-By trailer")}},
		{name: "a generated-with footer", line: "Generated with [Claude Code](https://example.com)", want: []string{label + ":3" + credit("a credit to an assistant")}},
		{name: "a written-by credit", line: "These notes were written by an AI assistant.", want: []string{label + ":3" + credit("a credit to an assistant")}},
		{name: "a drafted-using credit", line: "Drafted using ChatGPT.", want: []string{label + ":3" + credit("a credit to an assistant")}},
		{name: "a created-by credit", line: "created by Copilot", want: []string{label + ":3" + credit("a credit to an assistant")}},
		{name: "a session link", line: "See https://claude.ai/code/session_123 for how.", want: []string{label + ":3" + credit("a link to an assistant's session")}},
		{name: "a client named as a client", line: "Recognize a Codex client on stateless HTTP by its User-Agent."},
		{name: "a generator named as one", line: "The tool reference is generated by make gen-tool-reference."},
		{name: "an origin in an example", line: "Trust `https://claude.ai` as a client origin."},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			text := "## Summary\n\n" + testCase.line + "\n\n## Breaking changes\n\nNone.\n\n## Upgrading\n\nNone.\n\n## Security\n\nNone.\n"
			got := releaseNotesProblems(label, text)
			if strings.Join(got, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("releaseNotesProblems() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// TestAudit_ReleaseNotes_AreJudged verifies that the audit holds the tree to
// the notes of the release VERSION names, so the rule is reached from the
// command and not only from its own function.
func TestAudit_ReleaseNotes_AreJudged(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(releaseNotesDir), "v2.7.5.md")); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	problems, err := audit(root, installerTables())
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	want := []string{
		".github/workflows/ci.yml:5: uses: actions/checkout@v7 is not pinned to a 40-character commit SHA",
		missingNotesFinding("2.7.5"),
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("audit() = %#v, want %#v", problems, want)
	}
}

// TestAudit_UnreadableReleaseNotes_IsAnError verifies that notes the audit
// cannot read stop the command rather than reading as notes that passed.
func TestAudit_UnreadableReleaseNotes_IsAnError(t *testing.T) {
	t.Parallel()

	root := brokenRepository(t)
	directory := filepath.Join(root, filepath.FromSlash(releaseNotesDir))
	if err := os.RemoveAll(directory); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	writeFile(t, root, releaseNotesDir, validReleaseNotes)
	if _, err := audit(root, installerTables()); !errors.Is(err, errNotADirectory) {
		t.Errorf("audit() error = %v, want %v when the notes directory is a file", err, errNotADirectory)
	}
}

// TestLoadWorkflows_ReleaseWorkflow_OpensWithTheWrittenSummary verifies that
// the release workflow refuses a release whose notes are missing or blank, in
// a rehearsal too, and hands the notes to GoReleaser as the release header on
// both sides of its command line.
//
// GoReleaser OSS reads a header file only from the --release-header flag (the
// release.header.from_file setting is GoReleaser Pro's), and only outside a
// snapshot: a rehearsal skips the changelog it would be read with, so without
// the preflight step a rehearsal would pass on a tree whose tag push then
// fails at GoReleaser, after the image is already pushed. The path is built
// from preflight's release_tag output by a workflow expression, since
// goreleaser-action splits args into words and expands no shell variable, and
// it names the directory this rule reads, so the two cannot drift apart.
func TestLoadWorkflows_ReleaseWorkflow_OpensWithTheWrittenSummary(t *testing.T) {
	t.Parallel()

	release := releaseWorkflow(t, repositoryRoot(t))

	t.Run("preflight refuses a release with no summary", func(t *testing.T) {
		t.Parallel()

		preflight := releaseJob(t, release, "preflight")
		steps, _ := preflight["steps"].([]any)
		tagStep, notesStep := -1, -1
		for index, raw := range steps {
			step, _ := raw.(map[string]any)
			if step["id"] == "tag" {
				tagStep = index
			}
			if step["name"] == "Hold the release to its written summary" {
				notesStep = index
			}
		}
		if notesStep < 0 || notesStep < tagStep {
			t.Fatalf("preflight's summary check is step %d and the tag is resolved at step %d, want the check after it", notesStep, tagStep)
		}
		notes, _ := steps[notesStep].(map[string]any)
		if condition, guarded := notes["if"]; guarded {
			t.Errorf("the summary check runs only if %v, want it in a rehearsal too", condition)
		}
		env, _ := notes["env"].(map[string]any)
		if got, want := pythonStr(env["RELEASE_TAG"]), "${{ steps.tag.outputs.release_tag }}"; got != want {
			t.Errorf("the summary check sets RELEASE_TAG to %q, want %q", got, want)
		}
		run := pythonStr(notes["run"])
		for _, want := range []string{
			`NOTES="` + releaseNotesDir + `/${RELEASE_TAG}.md"`,
			`if [ ! -f "${NOTES}" ] || ! grep -q '[^[:space:]]' "${NOTES}"; then`,
			"exit 1",
		} {
			t.Run(want, func(t *testing.T) {
				t.Parallel()

				if !strings.Contains(run, want) {
					t.Errorf("the summary check runs %q, want it to carry %q", run, want)
				}
			})
		}
	})

	t.Run("GoReleaser opens the release body with the summary", func(t *testing.T) {
		t.Parallel()

		step := releaseStep(t, releaseJob(t, release, "release"), func(step map[string]any) bool {
			name, _, _ := strings.Cut(pythonStr(step["uses"]), "@")
			return name == goreleaserAction
		})
		with, _ := step["with"].(map[string]any)
		header := " --release-header " + releaseNotesDir + "/{0}.md', needs.preflight.outputs.release_tag)"
		want := "${{ env.REHEARSAL == 'true' && format('release --clean --snapshot --skip=publish,sign,announce" + header +
			" || format('release --clean" + header + " }}"
		if got := pythonStr(with["args"]); got != want {
			t.Errorf("goreleaser-action args = %q, want %q", got, want)
		}
	})
}

// TestReleaseNotes_Repository_KeepThePublishedSummaries verifies that the
// tree holds the notes of v3.0.0 to v3.3.0, the four GitHub releases the
// summaries were written after, and those of the newest published release,
// the version the release workflow stamps into server.json.
//
// The gate asks only for the release VERSION names, and the pull request that
// prepares a release moves VERSION past the newest published one, so the notes
// of a published release could otherwise be deleted with every gate green.
// This keeps them in the tree, where an edit to one is reviewed before it is
// published again. A release older than the newest and later than v3.3.0 is
// kept by review alone, since nothing in the tree lists the releases cut.
func TestReleaseNotes_Repository_KeepThePublishedSummaries(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	manifest, err := os.ReadFile(filepath.Join(root, "server.json"))
	if err != nil {
		t.Fatalf("read server.json: %v", err)
	}
	var published struct {
		Version string `json:"version"`
	}
	if err = json.Unmarshal(manifest, &published); err != nil {
		t.Fatalf("decode server.json: %v", err)
	}
	if published.Version == "" {
		t.Fatal("server.json names no version, the newest published release")
	}
	tags := []string{"v3.0.0", "v3.1.0", "v3.2.0", "v3.3.0"}
	if newest := "v" + published.Version; !slices.Contains(tags, newest) {
		tags = append(tags, newest)
	}
	directory := filepath.Join(root, filepath.FromSlash(releaseNotesDir))
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			t.Parallel()

			if _, statErr := os.Stat(filepath.Join(directory, tag+".md")); statErr != nil {
				t.Errorf("the notes of %s: %v", tag, statErr)
			}
		})
	}
}
