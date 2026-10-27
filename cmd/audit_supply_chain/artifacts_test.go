// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Tests for the hand-over of a tool binary to a credentialed job: a job that
// runs what another job built with go downloads it from this run, from a job it
// needs, built from this run's commit, and checks it against the SHA-256 that
// job computed before anything runs it.
package main

import (
	"strings"
	"testing"
)

// toolWorkflow is a workflow handing a tool built in an uncredentialed job to
// a credentialed one the way rule 2 accepts: the builder checks out this run's
// commit, builds with go, publishes the SHA-256 it computed as an output and
// uploads the binary, and the publisher needs it, downloads it from this run
// and checks it in the very next step. edit changes the two jobs before the
// workflow is returned.
func toolWorkflow(edit func(builder, publisher map[string]any)) map[string]any {
	builder := map[string]any{
		"permissions": map[string]any{"contents": "read"},
		"outputs":     map[string]any{"sha256": "${{ steps.build.outputs.sha256 }}", "name": "stamp"},
		"steps": []any{
			map[string]any{"uses": "actions/checkout@" + testSHA, "with": map[string]any{"persist-credentials": false}},
			map[string]any{"id": "build", "run": "go build -o \"$RUNNER_TEMP/tools/stamp\" ./cmd/stamp\n" +
				"echo \"sha256=$(sha256sum \"$RUNNER_TEMP/tools/stamp\" | cut -d' ' -f1)\" >> \"$GITHUB_OUTPUT\""},
			map[string]any{"id": "named", "run": "echo \"name=stamp\" >> \"$GITHUB_OUTPUT\""},
			map[string]any{"uses": "actions/upload-artifact@" + testSHA, "with": map[string]any{"name": "tools", "path": "${{ runner.temp }}/tools"}},
		},
	}
	publisher := map[string]any{
		"needs":       []any{"lint", "tools"},
		"permissions": map[string]any{"id-token": "write"},
		"steps": []any{
			map[string]any{"uses": "actions/download-artifact@" + testSHA, "with": map[string]any{"name": "tools", "path": "${{ runner.temp }}/tools"}},
			map[string]any{
				"env": map[string]any{"TOOL_SHA256": "${{ needs.tools.outputs.sha256 }}"},
				"run": "echo \"${TOOL_SHA256}  $RUNNER_TEMP/tools/stamp\" | sha256sum -c -\n\"$RUNNER_TEMP/tools/stamp\" --write",
			},
		},
	}
	if edit != nil {
		edit(builder, publisher)
	}
	return map[string]any{"jobs": map[string]any{"tools": builder, "publish": publisher}}
}

// toolRoot is the module the hand-over fixtures build their tool from: the
// Go-command fixtures, a stamp command of the standard library alone, and a
// script that runs code resolved at run time.
func toolRoot(t *testing.T) string {
	t.Helper()

	root := goModuleRoot(t)
	writeFile(t, root, "cmd/stamp/main.go", "package main\n\nimport \"os\"\n\nfunc main() { os.Exit(0) }\n")
	writeFile(t, root, "scripts/prepare.sh", "npx --yes prepare\n")
	return root
}

// steps returns a job's steps as maps, for an edit to reach into.
func steps(job map[string]any) []map[string]any {
	var found []map[string]any
	for _, step := range job["steps"].([]any) {
		found = append(found, step.(map[string]any))
	}
	return found
}

// TestCheckWorkflowJobs_ToolArtifact_IsCheckedBeforeItRuns verifies the
// hand-over of a tool binary to a credentialed job.
//
// The ports of the release jobs hand those jobs a binary rather than a Go
// toolchain, and the job that runs it holds the OIDC identity the registries
// trust. So what it runs is held to having been built by this run from this
// run's commit, in a job the publisher needs, and to the SHA-256 that job
// computed and published as an output: the check sits in the step right after
// the download, against that output read directly or through an environment
// variable, and a download that takes nothing a job built with go is not held.
func TestCheckWorkflowJobs_ToolArtifact_IsCheckedBeforeItRuns(t *testing.T) {
	t.Parallel()

	const publish = "wf.yml: job publish: step 0 downloads what job tools builds with go"
	cases := []struct {
		edit func(builder, publisher map[string]any)
		name string
		want []string
	}{
		{name: "checked against the output it was handed"},
		{
			name: "the output read directly",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[1]["env"] = nil
				steps(publisher)[1]["run"] = "echo \"${{ needs.tools.outputs.sha256 }}  stamp\" | sha256sum --check"
			},
		},
		{
			name: "the output read through the job's environment",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[1]["env"] = nil
				publisher["env"] = map[string]any{"DIGEST": "${{ needs.tools.outputs.sha256 }}"}
				steps(publisher)[1]["run"] = "shasum -a 256 -c <<< \"$DIGEST  stamp\""
			},
		},
		{
			name: "a download by a pattern",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{"pattern": "tool*"}
			},
		},
		{
			name: "a download by a name only the run knows",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{"name": "${{ matrix.artifact }}"}
			},
		},
		{
			name: "a builder checking out this run's commit by name",
			edit: func(builder, _ map[string]any) {
				steps(builder)[0]["with"].(map[string]any)["ref"] = "${{ github.sha }}"
			},
		},
		{
			name: "a download of every artifact, needed by name",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{}
				publisher["needs"] = "tools"
			},
		},
		{
			name: "a download of every artifact takes the tool too",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{}
				publisher["needs"] = []any{"lint"}
			},
			want: []string{publish + " without needing that job, so the SHA-256 that job computed cannot be read from its outputs"},
		},
		{
			name: "a pattern the tool's artifact does not match",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{"pattern": "coverage-*"}
				steps(publisher)[1]["run"] = "cat coverage.out"
			},
		},
		{
			name: "an upload under the default name",
			edit: func(builder, publisher map[string]any) {
				steps(builder)[3]["with"] = map[string]any{"path": "out"}
				steps(publisher)[0]["with"] = map[string]any{"name": "artifact"}
			},
		},
		{
			name: "an artifact no job builds with go",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"] = map[string]any{"name": "coverage"}
				steps(publisher)[1]["run"] = "cat coverage.out"
			},
		},
		{
			name: "a download from another run",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[0]["with"].(map[string]any)["run-id"] = "${{ github.event.workflow_run.id }}"
				steps(publisher)[0]["with"].(map[string]any)["github-token"] = "${{ github.token }}"
			},
			want: []string{publish + " from another run (github-token, run-id), so it is not the build of this run's commit"},
		},
		{
			name: "a job it does not need",
			edit: func(_, publisher map[string]any) { publisher["needs"] = []any{"lint"} },
			want: []string{publish + " without needing that job, so the SHA-256 that job computed cannot be read from its outputs"},
		},
		{
			name: "a builder checking out another commit",
			edit: func(builder, _ map[string]any) {
				steps(builder)[0]["with"].(map[string]any)["ref"] = "main"
			},
			want: []string{"wf.yml: job tools builds with go a tool it hands over from a checkout of ref main, not of this run's commit"},
		},
		{
			name: "a builder checking out another repository",
			edit: func(builder, _ map[string]any) {
				steps(builder)[0]["with"].(map[string]any)["repository"] = "someone/else"
			},
			want: []string{"wf.yml: job tools builds with go a tool it hands over from a checkout of repository someone/else, not of this run's commit"},
		},
		{
			name: "no output a step hashed",
			edit: func(builder, _ map[string]any) {
				builder["outputs"] = map[string]any{
					"sha256": "${{ steps.named.outputs.sha256 }}",
					"other":  "${{ steps.gone.outputs.sha256 }}",
					"digest": "${{ steps.build.outputs.digest }}",
				}
			},
			want: []string{"wf.yml: job tools builds with go a tool it hands over with no output a step of it computed with sha256sum, " +
				"so the job running it has nothing to check it against"},
		},
		{
			name: "no outputs at all",
			edit: func(builder, _ map[string]any) { delete(builder, "outputs") },
			want: []string{"wf.yml: job tools builds with go a tool it hands over with no output a step of it computed with sha256sum, " +
				"so the job running it has nothing to check it against"},
		},
		{
			name: "a step after it that checks nothing",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[1]["run"] = "chmod +x \"$RUNNER_TEMP/tools/stamp\"\n\"$RUNNER_TEMP/tools/stamp\" --write"
			},
			want: []string{publish + ", and the step after it does not check it with sha256sum against that job's sha256 output, so whatever the artifact holds is what runs"},
		},
		{
			name: "a step after it that computes a digest rather than checks one",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[1]["run"] = "sha256sum \"$RUNNER_TEMP/tools/stamp\" > \"${TOOL_SHA256}.txt\""
			},
			want: []string{publish + ", and the step after it does not check it with sha256sum against that job's sha256 output, so whatever the artifact holds is what runs"},
		},
		{
			name: "a check against something else",
			edit: func(_, publisher map[string]any) {
				steps(publisher)[1]["env"] = map[string]any{"TOOL_SHA256": "0123"}
			},
			want: []string{publish + ", and the step after it does not check it with sha256sum against that job's sha256 output, so whatever the artifact holds is what runs"},
		},
		{
			name: "no step after it",
			edit: func(_, publisher map[string]any) { publisher["steps"] = publisher["steps"].([]any)[:1] },
			want: []string{publish + ", and the step after it does not check it with sha256sum against that job's sha256 output, so whatever the artifact holds is what runs"},
		},
		{
			name: "a uses: step after it",
			edit: func(_, publisher map[string]any) {
				publisher["steps"] = append([]any{}, publisher["steps"].([]any)[0], map[string]any{"uses": "./.github/actions/run-tool"})
			},
			want: []string{publish + ", and the step after it does not check it with sha256sum against that job's sha256 output, so whatever the artifact holds is what runs"},
		},
	}

	root := toolRoot(t)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := toolWorkflow(testCase.edit)
			problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, []string{"tools", "publish"})
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkWorkflowJobs() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}
}

// TestCheckWorkflowJobs_ToolArtifact_ReportsTheBuilderOnce verifies that a
// builder two credentialed jobs take a tool from is reported once for what it
// does wrong itself, and that a job holding no credential is not held to the
// hand-over at all.
func TestCheckWorkflowJobs_ToolArtifact_ReportsTheBuilderOnce(t *testing.T) {
	t.Parallel()

	doc := toolWorkflow(func(builder, _ map[string]any) {
		delete(builder, "outputs")
		steps(builder)[2]["run"] = "go run ./cmd/dirty"
	})
	jobs := doc["jobs"].(map[string]any)
	jobs["verify"] = jobs["publish"]
	jobs["read"] = map[string]any{"permissions": map[string]any{"contents": "read"}, "steps": []any{
		map[string]any{"uses": "actions/download-artifact@" + testSHA, "with": map[string]any{"name": "tools"}},
	}}

	problems := newAudit(toolRoot(t), tables{}).checkWorkflowJobs("wf.yml", doc, []string{"tools", "publish", "verify", "read"})
	want := []string{
		"wf.yml: job tools, which builds a tool job publish runs: step 2: go run ./cmd/dirty imports " + dirtyImports +
			", outside the standard library and this module: " + goGraphReason,
		"wf.yml: job tools builds with go a tool it hands over with no output a step of it computed with sha256sum, " +
			"so the job running it has nothing to check it against",
	}
	if strings.Join(problems, "\n") != strings.Join(want, "\n") {
		t.Errorf("checkWorkflowJobs() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
	}
}

// TestCheckWorkflowJobs_ToolBuilder_IsHeldLikeTheJobThatRunsIt verifies that
// a job building a tool a credentialed job runs is held to rule 2 as that job
// is: what it builds, what its run blocks and the files they reach run, and the
// cache its Go setup would restore all decide what the credentialed job runs,
// and the SHA-256 the builder publishes vouches for whatever it built. Its
// checkout alone is not held to persist-credentials, since the token it would
// leave there writes nothing, unless the builder holds a credential itself;
// and a tool no credentialed job runs holds its builder to nothing.
func TestCheckWorkflowJobs_ToolBuilder_IsHeldLikeTheJobThatRunsIt(t *testing.T) {
	t.Parallel()

	const held = "wf.yml: job tools, which builds a tool job publish runs: "
	const sha = "\necho \"sha256=$(sha256sum \"$RUNNER_TEMP/tools/stamp\" | cut -d' ' -f1)\" >> \"$GITHUB_OUTPUT\""
	outside := ", outside the standard library and this module: " + goGraphReason
	cases := []struct {
		edit func(builder, publisher map[string]any)
		name string
		want []string
	}{
		{name: "a tool of the module and the standard library"},
		{
			name: "a tool reaching other modules",
			edit: func(builder, _ map[string]any) {
				steps(builder)[1]["run"] = "go build -o \"$RUNNER_TEMP/tools/stamp\" ./cmd/dirty" + sha
			},
			want: []string{held + "step 1: go build ./cmd/dirty imports " + dirtyImports + outside},
		},
		{
			name: "a Go setup restoring a cache",
			edit: func(builder, _ map[string]any) {
				builder["steps"].([]any)[2] = map[string]any{"uses": "actions/setup-go@" + testSHA}
			},
			want: []string{held + "step 2: actions/setup-go must set cache: false: the module and build caches it would restore " +
				"were saved by whichever run wrote them last, and nothing this job checks covers what they hold"},
		},
		{
			name: "code resolved at run time",
			edit: func(builder, _ map[string]any) { steps(builder)[2]["run"] = "npx --yes prepare" },
			want: []string{held + "step 2: run block matches " + npxFinding},
		},
		{
			name: "a script the builder runs",
			edit: func(builder, _ map[string]any) { steps(builder)[2]["run"] = "bash scripts/prepare.sh" },
			want: []string{held + "scripts/prepare.sh matches " + npxFinding},
		},
		{
			name: "a checkout leaving its read token in place",
			edit: func(builder, _ map[string]any) { steps(builder)[0]["with"] = map[string]any{} },
		},
		{
			name: "a builder holding a credential itself",
			edit: func(builder, _ map[string]any) {
				builder["permissions"] = map[string]any{"contents": "write"}
				steps(builder)[0]["with"] = map[string]any{}
			},
			want: []string{"wf.yml: job tools: step 0: actions/checkout must set persist-credentials: false \U00002014 the job's write-capable " +
				"token would otherwise sit in .git/config while the rest of the steps run"},
		},
		{
			name: "a tool no credentialed job runs",
			edit: func(builder, publisher map[string]any) {
				publisher["permissions"] = map[string]any{"contents": "read"}
				steps(builder)[2]["run"] = "npx --yes prepare"
			},
		},
	}

	root := toolRoot(t)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			doc := toolWorkflow(testCase.edit)
			problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, []string{"tools", "publish"})
			if strings.Join(problems, "\n") != strings.Join(testCase.want, "\n") {
				t.Errorf("checkWorkflowJobs() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(testCase.want, "\n"))
			}
		})
	}

	t.Run("a tool built from a tool another job built", func(t *testing.T) {
		t.Parallel()

		doc := toolWorkflow(func(builder, _ map[string]any) {
			builder["needs"] = []any{"base"}
			builder["steps"] = append([]any{
				map[string]any{"uses": "actions/download-artifact@" + testSHA, "with": map[string]any{"name": "base"}},
				map[string]any{
					"env": map[string]any{"BASE_SHA256": "${{ needs.base.outputs.sha256 }}"},
					"run": "echo \"${BASE_SHA256}  base\" | sha256sum -c -",
				},
			}, builder["steps"].([]any)...)
		})
		doc["jobs"].(map[string]any)["base"] = map[string]any{
			"outputs": map[string]any{"sha256": "${{ steps.build.outputs.sha256 }}"},
			"steps": []any{
				map[string]any{"uses": "actions/checkout@" + testSHA, "with": map[string]any{"persist-credentials": false}},
				map[string]any{"id": "build", "run": "go build -o base ./cmd/dirty\necho \"sha256=$(sha256sum base | cut -d' ' -f1)\" >> \"$GITHUB_OUTPUT\""},
				map[string]any{"uses": "actions/upload-artifact@" + testSHA, "with": map[string]any{"name": "base", "path": "base"}},
			},
		}

		problems := newAudit(root, tables{}).checkWorkflowJobs("wf.yml", doc, []string{"base", "tools", "publish"})
		want := []string{"wf.yml: job base, which builds a tool job tools runs: step 1: go build ./cmd/dirty imports " + dirtyImports + outside}
		if strings.Join(problems, "\n") != strings.Join(want, "\n") {
			t.Errorf("checkWorkflowJobs() =\n%s\nwant\n%s", strings.Join(problems, "\n"), strings.Join(want, "\n"))
		}
	})
}
