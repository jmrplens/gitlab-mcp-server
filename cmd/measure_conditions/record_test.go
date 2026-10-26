package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	yaml "go.yaml.in/yaml/v3"
)

// writeRun writes one run's artifact into dir: its record and its raw output.
func writeRun(t *testing.T, dir, env, output string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, recordFile), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, outputFile), []byte(output), 0o600); err != nil {
		t.Fatal(err)
	}
}

// recordText is a record as the workflow writes it.
func recordText(pkg, system string, status int) string {
	return fmt.Sprintf("package=%s\nsystem=%s\nplatform=linux/amd64\nstatus=%d\n", pkg, system, status)
}

// TestReadRecords_ArtifactLayouts_ReadEveryRecord verifies that the records
// are found wherever the download put them: one directory per artifact, or
// the root itself when there was only one artifact, and nested below either.
func TestReadRecords_ArtifactLayouts_ReadEveryRecord(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRun(t, filepath.Join(root, "gobco-macos-latest-cmd-server"), recordText("./cmd/server", "macos-latest", 0), "mac")
	writeRun(t, filepath.Join(root, "deeper", "gobco-windows-latest-cmd-server"), recordText("./cmd/server", "windows-latest", 3), "win")
	records, problems := readRecords(root)
	if len(problems) != 0 {
		t.Fatalf("readRecords() problems = %q", problems)
	}
	want := []record{
		{pkg: "./cmd/server", system: "windows-latest", platform: "linux/amd64", status: 3, output: "win"},
		{pkg: "./cmd/server", system: "macos-latest", platform: "linux/amd64", status: 0, output: "mac"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Errorf("readRecords() = %+v, want %+v", records, want)
	}

	single := t.TempDir()
	writeRun(t, single, recordText("./cmd/server", "ubuntu-latest", 1), "one")
	records, problems = readRecords(single)
	if len(problems) != 0 || len(records) != 1 || records[0].output != "one" {
		t.Errorf("readRecords(single artifact) = %+v, %q; want the one record", records, problems)
	}
}

// TestReadRecords_Problems_AreNamedAndTheRestRead verifies that a record that
// cannot be read is a problem naming its directory below the root, written
// with slashes, while every other record is still read; that a root which
// does not exist holds no record and no problem; and that a root which cannot
// be walked at all is a problem.
func TestReadRecords_Problems_AreNamedAndTheRestRead(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeRun(t, filepath.Join(root, "good"), recordText("./cmd/server", "macos-latest", 0), "ok")
	writeRun(t, filepath.Join(root, "nested", "bad"), "package=./cmd/server\n", "")
	if err := os.MkdirAll(filepath.Join(root, "odd", recordFile), 0o750); err != nil {
		t.Fatal(err)
	}
	records, problems := readRecords(root)
	if len(records) != 1 || records[0].output != "ok" {
		t.Errorf("readRecords() records = %+v, want the good one", records)
	}
	if len(problems) != 2 ||
		problems[0] != `the record in nested/bad cannot be read: key "system" is missing` ||
		!strings.HasPrefix(problems[1], "the record in odd cannot be read: ") {
		t.Errorf("readRecords() problems = %q", problems)
	}

	records, problems = readRecords(filepath.Join(root, "absent"))
	if records != nil || problems != nil {
		t.Errorf("readRecords(absent root) = %+v, %q; want nothing", records, problems)
	}

	records, problems = readRecords("bad\x00root")
	if records != nil || len(problems) != 1 || !strings.HasPrefix(problems[0], "the records under bad\x00root cannot be walked: ") {
		t.Errorf("readRecords(unwalkable root) = %+v, %q; want one problem", records, problems)
	}
}

// TestReadRecord_Failures_SayWhatIsWrong verifies the two failures a record
// file that parses can still have: a status that is not a number, and no raw
// output beside it.
func TestReadRecord_Failures_SayWhatIsWrong(t *testing.T) {
	t.Parallel()
	bad := t.TempDir()
	writeRun(t, bad, "package=./a\nsystem=macos-latest\nplatform=darwin/arm64\nstatus=three\n", "")
	if _, err := readRecord(bad); err == nil || err.Error() != `status "three" is not an exit status` {
		t.Errorf("readRecord(bad status) error = %v", err)
	}
	noOutput := t.TempDir()
	writeRun(t, noOutput, recordText("./a", "macos-latest", 0), "")
	if err := os.Remove(filepath.Join(noOutput, outputFile)); err != nil {
		t.Fatal(err)
	}
	if _, err := readRecord(noOutput); err == nil || !os.IsNotExist(err) {
		t.Errorf("readRecord(no output) error = %v, want not exist", err)
	}
}

// TestParseRecordFields_Lines_AreReadOrRefused verifies the record reader:
// carriage returns and blank lines are dropped and a value may hold an equals
// sign, while a line that is not key=value, an unknown key, a repeated key
// and a missing key are each refused.
func TestParseRecordFields_Lines_AreReadOrRefused(t *testing.T) {
	t.Parallel()
	fields, err := parseRecordFields("\r\npackage=./a\r\nsystem=s\n\nplatform=p=q\nstatus=0")
	want := map[string]string{"package": "./a", "system": "s", "platform": "p=q", "status": "0"}
	if err != nil || !reflect.DeepEqual(fields, want) {
		t.Errorf("parseRecordFields() = %v, %v; want %v", fields, err, want)
	}
	cases := []struct {
		name string
		text string
		want string
	}{
		{name: "not key=value", text: "package\n", want: `line "package" is not key=value`},
		{name: "unknown key", text: "gate=all\n", want: `key "gate" is not one of package, system, platform, status`},
		{name: "repeated key", text: "status=0\nstatus=3\n", want: `key "status" is given twice`},
		{name: "missing key", text: "package=./a\nsystem=s\nstatus=0\n", want: `key "platform" is missing`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, refused := parseRecordFields(tc.text); refused == nil || refused.Error() != tc.want {
				t.Errorf("parseRecordFields(%q) error = %v, want %q", tc.text, refused, tc.want)
			}
		})
	}
}

// workflowStep is the part of a workflow step the contract below reads.
type workflowStep struct {
	Name  string         `yaml:"name"`
	Uses  string         `yaml:"uses"`
	Shell string         `yaml:"shell"`
	Run   string         `yaml:"run"`
	Env   map[string]any `yaml:"env"`
	With  map[string]any `yaml:"with"`
}

// conditionsWorkflow is .github/workflows/conditions.yml as the contract
// below reads it, with every named step keyed by its job and its name.
type conditionsWorkflow struct {
	On          map[string]any    `yaml:"on"`
	Permissions map[string]string `yaml:"permissions"`
	Jobs        map[string]struct {
		Name           string         `yaml:"name"`
		Permissions    any            `yaml:"permissions"`
		TimeoutMinutes any            `yaml:"timeout-minutes"`
		Steps          []workflowStep `yaml:"steps"`
	} `yaml:"jobs"`
	steps map[string]workflowStep
}

// loadConditionsWorkflow reads and parses the workflow this command serves.
func loadConditionsWorkflow(t *testing.T) conditionsWorkflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "conditions.yml"))
	if err != nil {
		t.Fatalf("read the workflow: %v", err)
	}
	var workflow conditionsWorkflow
	if err = yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatalf("parse the workflow: %v", err)
	}
	workflow.steps = map[string]workflowStep{}
	for name, job := range workflow.Jobs {
		for _, step := range job.Steps {
			workflow.steps[name+"/"+step.Name] = step
		}
	}
	return workflow
}

// TestConditionsWorkflow_Triggers_AreADispatchThatOnlyReads holds the workflow
// to being run by hand and by nothing else, and to the read-only token: no
// trigger beside workflow_dispatch, contents: read alone, and no job raising
// it for itself.
func TestConditionsWorkflow_Triggers_AreADispatchThatOnlyReads(t *testing.T) {
	t.Parallel()
	workflow := loadConditionsWorkflow(t)
	if len(workflow.On) != 1 || workflow.On["workflow_dispatch"] == nil {
		t.Errorf("on = %v, want workflow_dispatch alone", workflow.On)
	}
	if !reflect.DeepEqual(workflow.Permissions, map[string]string{"contents": "read"}) {
		t.Errorf("permissions = %v, want contents: read alone", workflow.Permissions)
	}
	if len(workflow.Jobs) != 3 {
		t.Errorf("jobs = %d, want plan, measure and summary", len(workflow.Jobs))
	}
	for name, job := range workflow.Jobs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if job.Permissions != nil {
				t.Errorf("job %s sets permissions of its own: %v", name, job.Permissions)
			}
		})
	}
}

// TestConditionsWorkflow_MeasureStep_WritesTheRecordThisCommandReads holds the
// measuring step to what summarize assumes of it: it runs this command's
// measure over the job's packages through bash, with the dispatch's gate in
// the script's environment and the system and artifact the plan chose, writes
// the records below the directory the upload step sends under the artifact
// name, and fails the job with an annotation when the command says a package
// was not measured. It also removes setup-go's problem matcher before
// anything is printed, since that matcher files every condition gobco names
// as an error annotation.
func TestConditionsWorkflow_MeasureStep_WritesTheRecordThisCommandReads(t *testing.T) {
	t.Parallel()
	workflow := loadConditionsWorkflow(t)
	measure := workflow.steps["measure/Measure"]
	wantEnv := map[string]any{
		"PACKAGES":         "${{ matrix.packages }}",
		"SYSTEM":           "${{ matrix.system }}",
		"ARTIFACT":         "${{ matrix.artifact }}",
		"GOBCO_GATE":       "${{ inputs.gate }}",
		"GOBCO_TEST_FLAGS": "-timeout=30m",
	}
	if measure.Shell != "bash" || !reflect.DeepEqual(measure.Env, wantEnv) {
		t.Errorf("the measure step has shell %q and env %v, want bash and %v", measure.Shell, measure.Env, wantEnv)
	}
	if first, _, _ := strings.Cut(measure.Run, "\n"); first != `echo "::remove-matcher owner=go::"` {
		t.Errorf("the measure step opens with %q, want the removal of setup-go's problem matcher", first)
	}
	wanted := []string{
		`go run ./cmd/measure_conditions measure -packages "$PACKAGES" -system "$SYSTEM" -records "${RUNNER_TEMP//\\//}/$ARTIFACT" || status=$?`,
		`if [ "$status" -ne 0 ]; then`,
		`echo "::error title=Not measured::`,
		`exit "$status"`,
	}
	for _, fragment := range wanted {
		t.Run(fragment, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(measure.Run, fragment) {
				t.Errorf("the measure step does not run %q:\n%s", fragment, measure.Run)
			}
		})
	}
	upload := workflow.steps["measure/Upload the record and the raw gobco output"]
	if upload.With["name"] != "${{ matrix.artifact }}" || upload.With["path"] != "${{ runner.temp }}/${{ matrix.artifact }}" {
		t.Errorf("the upload step takes %v", upload.With)
	}
}

// TestConditionsWorkflow_JobBounds_FollowWhatEachJobMeasures holds each job to
// its time bound: a measuring job of named packages measures one and keeps
// the hour it had before the tree existed, which on the Windows and macOS
// runners is billed at a multiple of the Linux rate, a shard of the tree
// measures fifteen or so packages one after another and gets two hours, and
// the plan and the summary, which run the command and nothing else, ten
// minutes each.
func TestConditionsWorkflow_JobBounds_FollowWhatEachJobMeasures(t *testing.T) {
	t.Parallel()
	workflow := loadConditionsWorkflow(t)
	want := map[string]any{"plan": 10, "measure": "${{ inputs.tree && 120 || 60 }}", "summary": 10}
	for name, bound := range want {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := workflow.Jobs[name].TimeoutMinutes; got != bound {
				t.Errorf("job %s has timeout-minutes %v (%T), want %v", name, got, got, bound)
			}
		})
	}
}

// TestConditionsWorkflow_PlanAndSummary_RunThisCommand holds the two ends to
// this command: the plan is handed every input of the dispatch, the tree
// one included, and prints its matrix into $GITHUB_OUTPUT; the jobs are
// named by the name a cell carries; and the summary downloads every artifact
// by the prefix the plan names them with, a shard's as much as a package's,
// and writes the table into the job summary through bash, so a failure of
// the command is not hidden by the pipe.
func TestConditionsWorkflow_PlanAndSummary_RunThisCommand(t *testing.T) {
	t.Parallel()
	workflow := loadConditionsWorkflow(t)
	plan := workflow.steps["plan/Plan the runs"]
	if want := `go run ./cmd/measure_conditions plan -packages "$PACKAGES" -systems "$SYSTEMS" -gate "$GATE" -tree="$TREE" >> "$GITHUB_OUTPUT"`; plan.Run != want {
		t.Errorf("the plan step runs %q, want %q", plan.Run, want)
	}
	if plan.Env["TREE"] != "${{ inputs.tree }}" {
		t.Errorf("the plan step's env is %v, want TREE from the tree input", plan.Env)
	}
	if name := workflow.Jobs["measure"].Name; name != "🔬 ${{ matrix.name }} on ${{ matrix.system }}" {
		t.Errorf("the measure job is named %q", name)
	}
	planned, err := buildMatrix([]string{"./cmd/server"}, []string{"macos-latest"})
	if err != nil {
		t.Fatal(err)
	}
	shards := buildShards([]string{"./cmd/server"}, []string{"macos-latest"}, treeShards)
	download := workflow.steps["summary/Download the records"]
	for _, artifact := range []string{planned.Include[0].Artifact, shards.Include[0].Artifact} {
		t.Run(artifact, func(t *testing.T) {
			t.Parallel()
			if pattern, isText := download.With["pattern"].(string); !isText || !strings.HasSuffix(pattern, "*") ||
				!strings.HasPrefix(artifact, strings.TrimSuffix(pattern, "*")) {
				t.Errorf("the download step takes %v, which does not match the artifact %s the plan names", download.With, artifact)
			}
		})
	}
	summary := workflow.steps["summary/Write the job summary"]
	if !strings.Contains(summary.Run, "go run ./cmd/measure_conditions summarize ") ||
		!strings.Contains(summary.Run, `tee -a "$GITHUB_STEP_SUMMARY"`) || summary.Shell != "bash" {
		t.Errorf("the summary step runs %q with shell %q", summary.Run, summary.Shell)
	}
}
