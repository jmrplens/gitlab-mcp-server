package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// wholeEnough builds an introspection big enough to clear every floor, so a
// test about one rule is not also a test about the floors.
//
// The floors exist for a boot that half ran, and a fixture that trips them by
// being a fixture would make every case here fail for the same uninteresting
// reason.
func wholeEnough() dumped {
	payload := dumped{
		SchemaVersion: apilive.SchemaVersion,
		Version:       "19.3.1-ee",
		Revision:      "9d55961fa80",
		Entities:      map[string]apilive.Entity{},
		Features:      map[string]string{},
	}
	for i := range minEntities + 1 {
		fields := make([]apilive.Field, 0, 13)
		for j := range 13 {
			fields = append(fields, apilive.Field{Name: fieldName(i, j)})
		}
		payload.Entities[entityName(i)] = apilive.Entity{Fields: fields}
	}
	for i := range minRoutes + 1 {
		payload.Routes = append(payload.Routes, apilive.Route{
			Method: "GET", Path: routePath(i), Entity: entityName(i % 7),
		})
	}
	for i := range minFeatures + 1 {
		payload.Features[featureName(i)] = apilive.TierPremium
	}
	return payload
}

func entityName(i int) string   { return "API::Entities::Fixture" + itoa(i) }
func fieldName(i, j int) string { return "field_" + itoa(i) + "_" + itoa(j) }
func routePath(i int) string    { return "/api/:version/fixture/" + itoa(i) }
func featureName(i int) string  { return "fixture_feature_" + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// writeDump stages an introspection on disk the way the container would leave
// it, and returns its path.
func writeDump(t *testing.T, payload dumped) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "introspect.json")
	if writeErr := os.WriteFile(path, encoded, 0o600); writeErr != nil {
		t.Fatalf("write the fixture: %v", writeErr)
	}
	return path
}

// writeRecordAt commits a record made from payload, dated as given.
//
// It writes the document directly rather than through runGenerate, which
// stamps today and refuses a short record: the cases that use it need a
// retrieval date of their own, a record that is deliberately incomplete, or
// both at once.
func writeRecordAt(t *testing.T, dir string, payload dumped, retrievedAt string) {
	t.Helper()
	doc := apilive.Document{
		SchemaVersion: apilive.SchemaVersion,
		Source: apilive.Source{
			Image: "gitlab/gitlab-ee:latest", Version: payload.Version,
			RetrievedAt: retrievedAt,
		},
		Entities: payload.Entities, Routes: payload.Routes, Features: payload.Features,
	}
	if err := apilive.Write(dir, doc); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}
}

// today is the retrieval date a record taken now would carry, spelled the way
// the record spells it.
func today() string { return time.Now().UTC().Format(time.DateOnly) }

// quietStderr points os.Stderr at the null device for the length of the test.
//
// The flag set writes its usage there when a flag will not parse, and runMain
// writes the refusal there before returning 1. A case about the exit code is
// about neither, and both would otherwise be printed by a passing run.
func quietStderr(t *testing.T) {
	t.Helper()
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	previous := os.Stderr
	os.Stderr = devNull
	t.Cleanup(func() {
		os.Stderr = previous
		_ = devNull.Close()
	})
}

// captureStderr redirects os.Stderr into a file for the test and returns what
// was written to it.
//
// A file rather than a pipe, because a pipe's buffer is finite and a writer
// that fills it blocks until somebody reads: the test would then have to read
// concurrently with the code it is driving, which is a second thing to get
// right in every test that only wanted to read a message.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	sink, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("create the stderr sink: %v", err)
	}
	previous := os.Stderr
	os.Stderr = sink
	t.Cleanup(func() {
		os.Stderr = previous
		_ = sink.Close()
	})
	return func() string {
		said, readErr := os.ReadFile(sink.Name())
		if readErr != nil {
			t.Fatalf("read the stderr sink: %v", readErr)
		}
		return string(said)
	}
}

// TestRunGenerate_ADumpOnDisk_BecomesARecordWithProvenance verifies the half of
// this command that has rules in it, without the half that needs Docker.
//
// The separation is the point of the -dump flag: a boot is slow and needs an
// image, and none of the judgement lives there.
func TestRunGenerate_ADumpOnDisk_BecomesARecordWithProvenance(t *testing.T) {
	dir := t.TempDir()
	payload := wholeEnough()

	if err := runGenerate(dir, dumpFrom{path: writeDump(t, payload)}, "gitlab/gitlab-ee:latest", false); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	doc, err := apilive.Read(dir)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, testCase := range []struct {
		name string
		got  any
		want any
	}{
		{name: "the schema version is this build's", got: doc.SchemaVersion, want: apilive.SchemaVersion},
		{name: "the version comes from the instance", got: doc.Source.Version, want: "19.3.1-ee"},
		{name: "the revision comes from the instance", got: doc.Source.Revision, want: "9d55961fa80"},
		{name: "the image is recorded", got: doc.Source.Image, want: "gitlab/gitlab-ee:latest"},
		{name: "the entity count is counted, not copied", got: doc.Source.Entities, want: len(payload.Entities)},
		{name: "the field count spans every entity", got: doc.Source.Fields, want: len(payload.Entities) * 13},
		{name: "the route count is counted", got: doc.Source.Routes, want: len(payload.Routes)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}

	t.Run("the digest is of what the container produced", func(t *testing.T) {
		if len(doc.Source.SHA256) != 64 {
			t.Errorf("sha256 = %q, want 64 hex characters", doc.Source.SHA256)
		}
	})
	t.Run("the retrieval date is today", func(t *testing.T) {
		if doc.Source.RetrievedAt != time.Now().UTC().Format(time.DateOnly) {
			t.Errorf("retrieved_at = %q, want today", doc.Source.RetrievedAt)
		}
	})
}

// TestRunGenerate_ARecordThatIsNotAGitLab_IsRefusedRatherThanWritten verifies
// the floors at the moment they matter most.
//
// An introspection that half ran does not fail: it returns less. Written, that
// record says GitLab stopped sending things, and every audit downstream
// reports the difference as a gap in this server. Refusing to write it is the
// only place that can be caught, because afterwards it looks like data.
func TestRunGenerate_ARecordThatIsNotAGitLab_IsRefusedRatherThanWritten(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		mutate  func(*dumped)
		wantsIn string
	}{
		{
			name:    "too few entities",
			mutate:  func(d *dumped) { d.Entities = map[string]apilive.Entity{"API::Entities::One": {}} },
			wantsIn: "entities",
		},
		{
			name: "the entities are there and empty",
			mutate: func(d *dumped) {
				for name := range d.Entities {
					d.Entities[name] = apilive.Entity{}
				}
			},
			wantsIn: "exposed fields",
		},
		{
			name:    "too few routes",
			mutate:  func(d *dumped) { d.Routes = d.Routes[:3] },
			wantsIn: "routes",
		},
		{
			name:    "too few licensed features",
			mutate:  func(d *dumped) { d.Features = map[string]string{"one": apilive.TierPremium} },
			wantsIn: "licensed features",
		},
		{
			name: "an entity refused to describe itself",
			mutate: func(d *dumped) {
				d.Entities["API::Entities::Fixture1"] = apilive.Entity{Error: "NoMethodError"}
			},
			wantsIn: "refused to describe themselves",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			payload := wholeEnough()
			testCase.mutate(&payload)

			err := runGenerate(dir, dumpFrom{path: writeDump(t, payload)}, "gitlab/gitlab-ee:latest", false)
			if err == nil {
				t.Fatal("the record was written, want a refusal")
			}
			if !strings.Contains(err.Error(), testCase.wantsIn) {
				t.Errorf("error = %q, want it to name %q", err, testCase.wantsIn)
			}
			if _, statErr := os.Stat(apilive.Path(dir)); statErr == nil {
				t.Error("a refused record was written anyway")
			}
		})
	}
}

// TestRunGenerate_AnIntrospectionFromAnotherSchema_IsRefused verifies that the
// script and the command are held to one version of the shape between them.
//
// They are separate files and a maintainer edits the Ruby without rebuilding
// the Go. Decoding a shape that moved would populate the fields that still
// match and silently drop the rest, which is the one failure a reader could
// not detect afterwards.
func TestRunGenerate_AnIntrospectionFromAnotherSchema_IsRefused(t *testing.T) {
	payload := wholeEnough()
	payload.SchemaVersion = apilive.SchemaVersion + 1

	err := runGenerate(t.TempDir(), dumpFrom{path: writeDump(t, payload)}, "gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("an introspection from another schema was accepted")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("error = %q, want it to name the schema version", err)
	}
}

// TestRunCheck_AStaleOrTruncatedRecord_IsRefused verifies the gate every audit
// rests on: it reads one file, asks nothing of Docker or the network, and
// fails on a record that cannot answer for a current GitLab.
func TestRunCheck_AStaleOrTruncatedRecord_IsRefused(t *testing.T) {
	fresh := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	for _, testCase := range []struct {
		name        string
		retrievedAt string
		now         time.Time
		mutate      func(*dumped)
		wantsIn     string
	}{
		{name: "a whole record taken today", retrievedAt: "2026-09-09", now: fresh},
		{
			name: "a record older than the window", retrievedAt: "2025-01-01", now: fresh,
			wantsIn: "days old",
		},
		{
			name: "a record taken in the future", retrievedAt: "2027-01-01", now: fresh,
			wantsIn: "has not happened yet",
		},
		{
			name: "a retrieval date nothing can read", retrievedAt: "last Tuesday", now: fresh,
			wantsIn: "not a date",
		},
		{
			name: "a record that lost its routes", retrievedAt: "2026-09-09", now: fresh,
			mutate:  func(d *dumped) { d.Routes = nil },
			wantsIn: "routes",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			payload := wholeEnough()
			if testCase.mutate != nil {
				testCase.mutate(&payload)
			}
			writeRecordAt(t, dir, payload, testCase.retrievedAt)

			err := runCheck(dir, testCase.now)

			if testCase.wantsIn == "" {
				if err != nil {
					t.Fatalf("runCheck: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("the record passed, want a refusal")
			}
			if !strings.Contains(err.Error(), testCase.wantsIn) {
				t.Errorf("error = %q, want it to name %q", err, testCase.wantsIn)
			}
		})
	}
}

// TestIntrospection_WithoutADump_GoesToTheRunner verifies the seam that keeps
// Docker out of every test above.
func TestIntrospection_WithoutADump_GoesToTheRunner(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })

	var askedFor string
	var askedKeep bool
	runner = func(image string, keep bool) ([]byte, origin, error) {
		askedFor, askedKeep = image, keep
		return []byte("{}"), origin{image: image, digest: "sha256:abc"}, nil
	}

	raw, from, err := introspection(dumpFrom{}, "gitlab/gitlab-ee:17.0.0", true)
	if err != nil {
		t.Fatalf("introspection: %v", err)
	}
	t.Run("the image is the one asked for", func(t *testing.T) {
		if askedFor != "gitlab/gitlab-ee:17.0.0" {
			t.Errorf("image = %q, want the one passed", askedFor)
		}
	})
	t.Run("keep is passed through", func(t *testing.T) {
		if !askedKeep {
			t.Error("keep was not passed to the runner")
		}
	})
	t.Run("the digest travels with the output", func(t *testing.T) {
		if from.digest != "sha256:abc" || string(raw) != "{}" {
			t.Errorf("got %q and %+v, want the runner's own answer", raw, from)
		}
	})
}

// TestIntrospection_WithADump_NeverBootsAnything verifies that -dump is a
// genuine offline path and not a boot with a shortcut.
func TestIntrospection_WithADump_NeverBootsAnything(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })
	runner = func(string, bool) ([]byte, origin, error) {
		t.Error("the runner was called for a dump that was already on disk")
		return nil, origin{}, nil
	}

	path := writeDump(t, wholeEnough())
	raw, from, err := introspection(dumpFrom{path: path, digest: "sha256:abc"}, "gitlab/gitlab-ee:latest", false)
	if err != nil {
		t.Fatalf("introspection: %v", err)
	}
	if len(raw) == 0 || from.image != "gitlab/gitlab-ee:latest" {
		t.Errorf("got %d bytes from %+v, want the dump read back", len(raw), from)
	}
	// The digest travels with the dump because the boot that could ask Docker
	// for it may have happened on another machine.
	if from.digest != "sha256:abc" {
		t.Errorf("digest = %q, want the one given beside the dump", from.digest)
	}
}

// stubDocker writes a stand-in for the docker binary and returns the path this
// command would run it through.
//
// The stand-in is a shell script, so the cases that need one skip on Windows
// and say so: what is being tested is how this command reads docker's answers,
// and a platform that cannot host the stand-in cannot answer them.
func stubDocker(t *testing.T, script string) dockerPath {
	t.Helper()
	if runtime.GOOS == windowsGOOS {
		t.Skip("the docker stand-in is a shell script, which Windows will not execute for a file with no extension")
	}
	path := filepath.Join(t.TempDir(), "docker")
	// #nosec G306 -- the stand-in has to be executable to stand in for
	// anything, and it lives in a directory this test owns for its own run.
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o700); err != nil {
		t.Fatalf("writing the docker stand-in: %v", err)
	}
	return dockerPath(path)
}

// windowsGOOS is spelled once so the skip above reads as one decision.
const windowsGOOS = "windows"

// TestLookUpDocker_WithNothingOnPATH_SaysWhatIsMissing verifies the one place
// this command reads PATH reports its own failure, since every later call is
// built from what it resolved.
func TestLookUpDocker_WithNothingOnPATH_SaysWhatIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := lookUpDocker(); err == nil {
		t.Error("lookUpDocker found a docker on an empty PATH")
	}
}

// TestDockerCommand_RunsTheResolvedBinary verifies that a call names the
// absolute path the lookup returned rather than the word docker, which is the
// whole point of resolving it once.
func TestDockerCommand_RunsTheResolvedBinary(t *testing.T) {
	docker := dockerPath(filepath.Join("opt", "bin", "docker"))
	cmd := docker.command(context.Background(), "inspect", "-f", "{{.State.Running}}", "box")

	t.Run("the program is the resolved path", func(t *testing.T) {
		if cmd.Path != string(docker) {
			t.Errorf("Path = %q, want %q", cmd.Path, docker)
		}
	})
	t.Run("the arguments follow it in order", func(t *testing.T) {
		want := []string{string(docker), "inspect", "-f", "{{.State.Running}}", "box"}
		if strings.Join(cmd.Args, " ") != strings.Join(want, " ") {
			t.Errorf("Args = %v, want %v", cmd.Args, want)
		}
	})
}

// TestRunning_ReadsDockersAnswerRatherThanItsExitCode verifies both halves of
// the check: a container docker calls running, and one it does not.
func TestRunning_ReadsDockersAnswerRatherThanItsExitCode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer string
		want   bool
	}{
		{name: "up", answer: "true", want: true},
		{name: "stopped", answer: "false", want: false},
		{name: "not a boolean", answer: "<no value>", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docker := stubDocker(t, "echo "+tc.answer+"\n")
			if got := running(context.Background(), docker); got != tc.want {
				t.Errorf("running = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRunning_WhenDockerItselfFails_IsFalse verifies that a docker that cannot
// answer is not read as a running container.
func TestRunning_WhenDockerItselfFails_IsFalse(t *testing.T) {
	docker := stubDocker(t, "echo true\nexit 1\n")
	if running(context.Background(), docker) {
		t.Error("running = true for a docker that exited non-zero")
	}
}

// TestImageDigest_TakesWhatFollowsTheAt verifies the digest is read out of the
// repository digest rather than reported whole, and that an image without one
// reports none instead of failing the run.
func TestImageDigest_TakesWhatFollowsTheAt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "a repository digest",
			script: "echo gitlab/gitlab-ee@sha256:abc123\n",
			want:   "sha256:abc123",
		},
		{
			name:   "an image with no repository digest",
			script: "echo gitlab/gitlab-ee\n",
			want:   "",
		},
		{
			name:   "docker refuses to answer",
			script: "exit 1\n",
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			docker := stubDocker(t, tc.script)
			if got := imageDigest(context.Background(), docker, "gitlab/gitlab-ee:latest"); got != tc.want {
				t.Errorf("imageDigest = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestWaitDefaults_AsShipped_SleepBetweenAttemptsAndHoldManyOfThem verifies the
// two values waitForRails runs on when nothing has moved them.
//
// Every other case here moves both, because a real one is 20 minutes and 10
// seconds, so what actually ships is asserted nowhere else. Neither value is
// reachable by mutation testing either: a package-level var initializer carries
// no statement counter, so gremlins reports both NOT COVERED and never runs
// them. Measured by hand, a bootTimeout the tests do not override is caught by
// four cases; a pollInterval of zero was caught by nothing, and it is the worse
// of the two — the wait would fork a `docker exec` as fast as the machine
// allows for the whole twenty minutes, against a container that is still
// unpacking, which is the one thing this wait exists not to do.
//
// The relation is asserted rather than the literals: how long to wait for a
// GitLab is a judgement that may be revised, and a poll that does not sleep or
// a deadline that holds one attempt are wrong at any setting.
func TestWaitDefaults_AsShipped_SleepBetweenAttemptsAndHoldManyOfThem(t *testing.T) {
	t.Run("the poll sleeps rather than spinning", func(t *testing.T) {
		if pollInterval <= 0 {
			t.Errorf("pollInterval = %v, want a positive delay between two attempts", pollInterval)
		}
	})
	t.Run("the deadline holds many polls", func(t *testing.T) {
		if bootTimeout <= 10*pollInterval {
			t.Errorf("bootTimeout = %v and pollInterval = %v, want a deadline that outlasts a handful of attempts",
				bootTimeout, pollInterval)
		}
	})
}

// TestWaitForRails_WhenTheRunnerAnswers_ReturnsAtOnce verifies the readiness
// check is the runner answering and not the container being up.
func TestWaitForRails_WhenTheRunnerAnswers_ReturnsAtOnce(t *testing.T) {
	docker := stubDocker(t, "echo "+readyAnswer+"\n")
	if err := waitForRails(context.Background(), docker); err != nil {
		t.Errorf("waitForRails: %v", err)
	}
}

// TestWaitForRails_WhileTheDatabaseIsMigrating_KeepsWaiting verifies that an
// application which loads is not yet ready, which is the state that used to
// pass this wait and then break the introspection.
//
// A GitLab boots its Rails environment before its migrations have created the
// tables, so a probe that only proves the environment loaded says ready to a
// container whose first query will fail. Two full boots were spent on that
// before the probe asked the database a question.
func TestWaitForRails_WhileTheDatabaseIsMigrating_KeepsWaiting(t *testing.T) {
	previousTimeout, previousInterval := bootTimeout, pollInterval
	t.Cleanup(func() { bootTimeout, pollInterval = previousTimeout, previousInterval })
	bootTimeout, pollInterval = 40*time.Millisecond, time.Millisecond

	docker := stubDocker(t, `case "$1" in
  inspect) echo true ;;
  *) echo migrating ;;
esac
`)
	err := waitForRails(context.Background(), docker)
	if err == nil {
		t.Fatal("a container still migrating was reported ready")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("error = %v, want the wait to report it timed out", err)
	}
}

// TestWaitForRails_WhenTheContainerDied_ReportsItsLogs verifies a boot that
// died is reported with what docker said rather than waited out for twenty
// minutes, which is the difference between a diagnosis and a timeout.
func TestWaitForRails_WhenTheContainerDied_ReportsItsLogs(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  exec) exit 1 ;;
  inspect) echo false ;;
  logs) echo "the reconfigure failed" ;;
esac
`)
	err := waitForRails(context.Background(), docker)
	if err == nil {
		t.Fatal("waitForRails returned no error for a container that stopped")
	}
	if !strings.Contains(err.Error(), "the reconfigure failed") {
		t.Errorf("error = %q, want the container's own logs in it", err)
	}
}

// TestDockerRun_WithNoDockerOnPATH_NamesTheImageItCouldNotBoot verifies the
// early refusal carries what was being attempted, since this is the failure a
// maintainer without docker installed will actually see.
func TestDockerRun_WithNoDockerOnPATH_NamesTheImageItCouldNotBoot(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, _, err := dockerRun("gitlab/gitlab-ee:19.3.1-ee.0", false)
	if err == nil {
		t.Fatal("dockerRun booted something with no docker on PATH")
	}
	if !strings.Contains(err.Error(), "gitlab/gitlab-ee:19.3.1-ee.0") {
		t.Errorf("error = %q, want the image named in it", err)
	}
}

// TestDockerRun_DrivesOneBootAndCleansUpAfterIt verifies the whole sequence
// this command puts a container through, against a stand-in that records what
// it was asked for: the stale container is removed, the image is booted, the
// script is copied in, the introspection is run, and the container is removed
// again when -keep was not passed.
//
// Driving it end to end is the only way to see the order, and the order is the
// part that breaks: a copy before the application is up, or a teardown that
// does not run because the function returned early, are both invisible to a
// test of any one step.
func TestDockerRun_DrivesOneBootAndCleansUpAfterIt(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	docker := stubDocker(t, `echo "$@" >> `+log+`
case "$1" in
  exec)
    case "$5" in
      /tmp/introspect.rb) echo '{"schema_version":1}' ;;
      *) echo ready ;;
    esac
    ;;
  inspect) echo "gitlab/gitlab-ee@sha256:deadbeef" ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	raw, from, err := dockerRun("gitlab/gitlab-ee:19.3.1-ee.0", false)
	if err != nil {
		t.Fatalf("dockerRun: %v", err)
	}

	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("reading what the stand-in was asked: %v", readErr)
	}
	recorded := string(calls)

	t.Run("the introspection is what comes back", func(t *testing.T) {
		if strings.TrimSpace(string(raw)) != `{"schema_version":1}` {
			t.Errorf("output = %q, want the runner's own stdout", raw)
		}
	})
	t.Run("the digest travels with it", func(t *testing.T) {
		if from.digest != "sha256:deadbeef" || from.image != "gitlab/gitlab-ee:19.3.1-ee.0" {
			t.Errorf("origin = %+v, want the image and its repository digest", from)
		}
	})
	t.Run("a stale container is removed before the boot", func(t *testing.T) {
		if !strings.HasPrefix(recorded, "rm -f "+containerName) {
			t.Errorf("first call was %q, want the stale container removed first", firstLine(recorded))
		}
	})
	t.Run("the image asked for is the image booted", func(t *testing.T) {
		if !strings.Contains(recorded, "run -d --name "+containerName) ||
			!strings.Contains(recorded, "gitlab/gitlab-ee:19.3.1-ee.0") {
			t.Errorf("calls were:\n%s\nwant the named image booted detached", recorded)
		}
	})
	t.Run("the script is copied in before it is run", func(t *testing.T) {
		copied := strings.Index(recorded, "cp ")
		ran := strings.Index(recorded, "/tmp/introspect.rb\n")
		if copied < 0 || ran < 0 || copied > ran {
			t.Errorf("calls were:\n%s\nwant the copy before the run", recorded)
		}
	})
	t.Run("the container is removed again", func(t *testing.T) {
		if strings.Count(recorded, "rm -f "+containerName) != 2 {
			t.Errorf("calls were:\n%s\nwant the container removed before and after", recorded)
		}
	})
}

// TestDockerRun_WithKeep_LeavesTheContainerUp verifies -keep is honored, which
// is what makes a failed introspection debuggable.
func TestDockerRun_WithKeep_LeavesTheContainerUp(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	docker := stubDocker(t, `echo "$@" >> `+log+`
case "$1" in
  exec)
    case "$5" in
      /tmp/introspect.rb) echo '{}' ;;
      *) echo ready ;;
    esac
    ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	if _, _, err := dockerRun("gitlab/gitlab-ee:latest", true); err != nil {
		t.Fatalf("dockerRun: %v", err)
	}
	calls, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("reading what the stand-in was asked: %v", err)
	}
	if strings.Count(string(calls), "rm -f "+containerName) != 1 {
		t.Errorf("calls were:\n%s\nwant only the removal that precedes the boot", calls)
	}
}

// TestDockerRun_WhenTheBootFails_SaysWhatDockerSaid verifies the refusal
// carries docker's own message, since a boot that fails on the image name and
// one that fails on a daemon that is not running read identically otherwise.
func TestDockerRun_WhenTheBootFails_SaysWhatDockerSaid(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  run) echo "no such image" >&2; exit 125 ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	_, _, err := dockerRun("gitlab/gitlab-ee:nope", false)
	if err == nil {
		t.Fatal("dockerRun returned no error for a boot that failed")
	}
	if !strings.Contains(err.Error(), "no such image") {
		t.Errorf("error = %q, want docker's own message in it", err)
	}
}

// firstLine names the first call a stand-in recorded, for a failure message
// that shows what happened instead of the whole log.
func firstLine(recorded string) string {
	line, _, _ := strings.Cut(recorded, "\n")
	return line
}

// TestWaitForRails_WhenTheApplicationNeverAnswers_SaysTheContainerIsUp
// verifies the message the twenty-minute expiry carries, which is the one a
// maintainer reads when a boot went wrong in a way docker cannot see: the
// container is running and gitlab-rails runner is not answering, which is a
// different problem from a container that died.
func TestWaitForRails_WhenTheApplicationNeverAnswers_SaysTheContainerIsUp(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  exec) exit 1 ;;
  inspect) echo true ;;
esac
`)
	previous := bootTimeout
	t.Cleanup(func() { bootTimeout = previous })
	bootTimeout = -time.Second

	err := waitForRails(context.Background(), docker)

	if err == nil {
		t.Fatal("waitForRails returned no error past its deadline")
	}
	if !strings.Contains(err.Error(), "does not answer") {
		t.Errorf("error = %q, want it to say the application never answered", err)
	}
}

// TestWaitForRails_WhenTheRunIsCancelled_SaysSoRatherThanBlamingTheContainer
// verifies the interrupt path, and that it is not mistaken for the other way a
// wait ends.
//
// Every question this loop asks goes through docker with the run's own context,
// so a cancelled run makes the readiness check and the is-it-up check fail
// alike. Read in that order, an interrupt looks exactly like a container that
// died, and a reader is sent to the logs of a container that is fine. The
// cancellation is therefore reported before the container is asked about, and
// this test cancels up front, which is the case that used to be misreported.
func TestWaitForRails_WhenTheRunIsCancelled_SaysSoRatherThanBlamingTheContainer(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  exec) exit 1 ;;
  inspect) echo true ;;
  logs) echo "a container that is perfectly fine" ;;
esac
`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForRails(ctx, docker)

	if err == nil {
		t.Fatal("waitForRails returned no error for a cancelled run")
	}
	if !strings.Contains(err.Error(), "waiting for the application") {
		t.Errorf("error = %q, want it to name what it was waiting for", err)
	}
	if strings.Contains(err.Error(), "perfectly fine") {
		t.Errorf("error = %q, want it not to blame the container for an interrupt", err)
	}
}

// TestWaitForRails_WhenTheRunIsCancelledWhileItSleeps_StopsWaiting verifies the
// other moment an interrupt can arrive: not before an attempt but during the
// pause between two, which is where a twenty-minute wait spends nearly all of
// its time and so where a real interrupt almost always lands.
func TestWaitForRails_WhenTheRunIsCancelledWhileItSleeps_StopsWaiting(t *testing.T) {
	// The inspect sleeps past the deadline on purpose, so the cancellation
	// lands WHILE the running() check is in flight rather than before it.
	// That is the window a guard placed only ahead of the check leaves open:
	// running() asks docker through this same context, so the cancelled
	// inspect fails and reads as a container that died. Without the sleep
	// this test only reaches that interleaving when process spawning happens
	// to lose the race, which is what made it pass on Linux and fail on
	// macOS.
	docker := stubDocker(t, `case "$1" in
  exec) exit 1 ;;
  inspect) sleep 0.3; echo true ;;
esac
`)
	previousPoll := pollInterval
	t.Cleanup(func() { pollInterval = previousPoll })
	pollInterval = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := waitForRails(ctx, docker)

	if err == nil {
		t.Fatal("waitForRails returned no error for a run cancelled mid-wait")
	}
	if !strings.Contains(err.Error(), "waiting for the application") {
		t.Errorf("error = %q, want it to name what it was waiting for", err)
	}
	if strings.Contains(err.Error(), "the container stopped") {
		t.Errorf("error = %q, want it not to blame the container for an interrupt", err)
	}
}

// TestRunnerDetail_ReportsTheStderrThatNamesTheFailure verifies the detail a
// failed runner contributes to the error: nothing at all when the failure was
// not the command's own or it said nothing, the whole of a short complaint, and
// the last lines of a long one, since a Rails backtrace is long and the failure
// is named at its end rather than at its start.
func TestRunnerDetail_ReportsTheStderrThatNamesTheFailure(t *testing.T) {
	t.Parallel()

	longStderr := make([]string, 0, runnerStderrLines+5)
	for i := range cap(longStderr) {
		longStderr = append(longStderr, fmt.Sprintf("line %d", i))
	}

	tests := []struct {
		name        string
		err         error
		want        string
		wantMissing string
	}{
		{
			name: "a failure that is not the command's own contributes nothing",
			err:  errors.New("dial tcp: connection refused"),
			want: "",
		},
		{
			name: "a command that said nothing on stderr contributes nothing",
			err:  &exec.ExitError{Stderr: nil},
			want: "",
		},
		{
			name: "a short complaint is reported whole",
			err:  &exec.ExitError{Stderr: []byte("  PG::UndefinedTable: relation \"application_settings\" does not exist\n")},
			want: ": PG::UndefinedTable: relation \"application_settings\" does not exist",
		},
		{
			name:        "a long one keeps its last lines",
			err:         &exec.ExitError{Stderr: []byte(strings.Join(longStderr, "\n"))},
			want:        fmt.Sprintf("line %d", cap(longStderr)-1),
			wantMissing: "line 0\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			got := runnerDetail(testCase.err)
			if testCase.want == "" {
				if got != "" {
					t.Fatalf("runnerDetail() = %q, want no detail", got)
				}
				return
			}
			if !strings.Contains(got, testCase.want) {
				t.Errorf("runnerDetail() = %q, want it to carry %q", got, testCase.want)
			}
			if testCase.wantMissing != "" && strings.Contains(got, testCase.wantMissing) {
				t.Errorf("runnerDetail() kept %q, want only the last %d lines", testCase.wantMissing, runnerStderrLines)
			}
			if lines := strings.Count(got, "\n") + 1; lines > runnerStderrLines {
				t.Errorf("runnerDetail() reported %d lines, want at most %d", lines, runnerStderrLines)
			}
		})
	}
}

// TestRunCheck_WithNoRecordInTheDirectory_SaysItCouldNotReadIt verifies the
// gate keeps apart the two ways it refuses.
//
// A record it could not read and a record it read and judged too small want
// different things done: generate one, or regenerate this one. Reporting the
// first through the second's sentence would send a contributor who has never
// run the generator off to look for what shrank.
func TestRunCheck_WithNoRecordInTheDirectory_SaysItCouldNotReadIt(t *testing.T) {
	err := runCheck(t.TempDir(), time.Now())

	if err == nil {
		t.Fatal("runCheck passed a directory holding no record")
	}
	if !strings.Contains(err.Error(), "reading the live API record") {
		t.Errorf("error = %q, want it to say it could not read the record", err)
	}
	if strings.Contains(err.Error(), "cannot be rested on") {
		t.Errorf("error = %q, want a read failure rather than a verdict on a record", err)
	}
}

// TestRunGenerate_WhenTheBootFails_ReportsItAndWritesNothing verifies a boot
// that never produced an introspection stops the run where it is.
//
// The record is what every audit reads offline, and nothing downstream can
// tell one written from half an answer from one written from a whole GitLab.
// A failure that reached the write at all would therefore be permanent, so it
// has to end here, carrying what the boot said.
func TestRunGenerate_WhenTheBootFails_ReportsItAndWritesNothing(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })
	runner = func(string, bool) ([]byte, origin, error) {
		return nil, origin{}, errors.New("booting gitlab/gitlab-ee:latest: no such image")
	}

	dir := t.TempDir()
	err := runGenerate(dir, dumpFrom{}, "gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("runGenerate returned no error for a boot that failed")
	}
	if !strings.Contains(err.Error(), "no such image") {
		t.Errorf("error = %q, want the boot's own message in it", err)
	}
	if _, statErr := os.Stat(apilive.Path(dir)); statErr == nil {
		t.Error("a record was written although nothing was introspected")
	}
}

// TestRunGenerate_AnIntrospectionThatIsNotJSON_IsRefused verifies that what
// comes back from the runner is decoded or refused, never partly read.
//
// Only stdout is taken as the answer, and stdout is not always the answer: a
// script that dies before it prints, or a Rails that writes something of its
// own there first, leaves bytes no decoder will accept. Decoding those
// leniently would produce an empty record that passes for a small one.
func TestRunGenerate_AnIntrospectionThatIsNotJSON_IsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "introspect.json")
	if err := os.WriteFile(path, []byte("Ruby died before it printed anything\n"), 0o600); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}

	dir := t.TempDir()
	err := runGenerate(dir, dumpFrom{path: path}, "gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("runGenerate accepted an introspection that is not JSON")
	}
	if !strings.Contains(err.Error(), "decoding the introspection") {
		t.Errorf("error = %q, want it to name what it could not decode", err)
	}
	if _, statErr := os.Stat(apilive.Path(dir)); statErr == nil {
		t.Error("a record was written from an introspection that did not decode")
	}
}

// TestRunGenerate_WhenTheRecordCannotBeWritten_SaysSo verifies the last step
// reports its own failure instead of returning the success message.
//
// runGenerate prints the record's provenance when it finishes, and that line
// is what a maintainer reads as "the record is on disk". A write that failed
// silently would print it over a directory that has no record in it.
func TestRunGenerate_WhenTheRecordCannotBeWritten_SaysSo(t *testing.T) {
	root := t.TempDir()
	obstruction := filepath.Join(root, "docs")
	if err := os.WriteFile(obstruction, []byte("a file where the directory should be\n"), 0o600); err != nil {
		t.Fatalf("stage the obstruction: %v", err)
	}

	err := runGenerate(
		filepath.Join(obstruction, "development"),
		dumpFrom{path: writeDump(t, wholeEnough())},
		"gitlab/gitlab-ee:latest", false,
	)

	if err == nil {
		t.Fatal("runGenerate reported success for a record it could not write")
	}
	if !strings.Contains(err.Error(), "live API record") {
		t.Errorf("error = %q, want it to name what could not be written", err)
	}
}

// TestIntrospection_WithADumpThatIsNotThere_NeverFallsBackToABoot verifies
// that naming a dump commits the run to it.
//
// -dump is what a person passes when the boot happened on another machine, so
// falling back would start a twenty-minute pull on the machine chosen
// precisely because it should not have to do one, and would then write a
// record provenanced with the digest given beside a dump it never read.
func TestIntrospection_WithADumpThatIsNotThere_NeverFallsBackToABoot(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })
	runner = func(string, bool) ([]byte, origin, error) {
		t.Error("a dump that could not be read fell back to booting a container")
		return nil, origin{}, nil
	}

	absent := filepath.Join(t.TempDir(), "never-written.json")
	_, _, err := introspection(dumpFrom{path: absent, digest: "sha256:abc"}, "gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("introspection returned no error for a dump that is not there")
	}
	if !strings.Contains(err.Error(), "reading the introspection dump") {
		t.Errorf("error = %q, want it to say which file it could not read", err)
	}
}

// TestDockerRun_WhenTheApplicationNeverComesUp_ReportsTheWaitAndStillTearsDown
// verifies that a failure between the boot and the introspection is reported
// with what the container said, and that the teardown runs anyway.
//
// The teardown half is the part a test of waitForRails alone cannot see: the
// removal is deferred inside dockerRun, so a failure returned before the
// introspection is exactly the shape that would leave three gigabytes running
// if the defer were ever moved below the wait.
func TestDockerRun_WhenTheApplicationNeverComesUp_ReportsTheWaitAndStillTearsDown(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	docker := stubDocker(t, `echo "$@" >> `+log+`
case "$1" in
  exec) exit 1 ;;
  inspect) echo false ;;
  logs) echo "the reconfigure failed" ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)
	if err == nil {
		t.Fatal("dockerRun returned no error for an application that never came up")
	}
	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("reading what the stand-in was asked: %v", readErr)
	}

	t.Run("the container's own logs are the diagnosis", func(t *testing.T) {
		if !strings.Contains(err.Error(), "the reconfigure failed") {
			t.Errorf("error = %q, want the container's logs in it", err)
		}
	})
	t.Run("nothing was run inside the container", func(t *testing.T) {
		if strings.Contains(string(calls), "cp ") {
			t.Errorf("calls were:\n%s\nwant nothing copied into a container that never came up", calls)
		}
	})
	t.Run("the container is torn down anyway", func(t *testing.T) {
		if strings.Count(string(calls), "rm -f "+containerName) != 2 {
			t.Errorf("calls were:\n%s\nwant the container removed after the failure too", calls)
		}
	})
}

// TestDockerRun_WhenTheScriptCannotBeStaged_NeverTouchesTheContainer verifies
// the one step of this sequence that happens on the host rather than in the
// container, and that failing it stops the run before docker is asked to copy
// anything.
//
// It is staged through os.CreateTemp, so a temp directory that is not there is
// the way the host refuses: os.TempDir reads TMPDIR, which is what this test
// moves.
func TestDockerRun_WhenTheScriptCannotBeStaged_NeverTouchesTheContainer(t *testing.T) {
	log := filepath.Join(t.TempDir(), "calls.log")
	docker := stubDocker(t, `echo "$@" >> `+log+`
case "$1" in
  exec) echo `+readyAnswer+` ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))
	// Last, because every t.TempDir above resolves through the same variable.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "not-a-directory"))

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)
	if err == nil {
		t.Fatal("dockerRun returned no error for a script it could not stage")
	}
	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("reading what the stand-in was asked: %v", readErr)
	}

	t.Run("the failure names the step", func(t *testing.T) {
		if !strings.Contains(err.Error(), "staging the introspection script") {
			t.Errorf("error = %q, want it to name the step that failed", err)
		}
	})
	t.Run("nothing was copied into the container", func(t *testing.T) {
		if strings.Contains(string(calls), "cp ") {
			t.Errorf("calls were:\n%s\nwant no copy of a script that was never written", calls)
		}
	})
	t.Run("the container is torn down anyway", func(t *testing.T) {
		if strings.Count(string(calls), "rm -f "+containerName) != 2 {
			t.Errorf("calls were:\n%s\nwant the container removed after the failure too", calls)
		}
	})
}

// staleScriptFile is a staged script file that reports a failure where a full
// or dying filesystem reports one: a write that stopped short, and a close that
// reports a write the kernel had deferred.
//
// It exists because neither can be provoked through a temp directory a test can
// build — the directory is either there, and every write to a file this small
// succeeds, or it is not, and the creation fails first, which is the case the
// test above already drives.
type staleScriptFile struct {
	name     string
	writeErr error
	closeErr error
}

func (f staleScriptFile) Name() string { return f.name }

func (f staleScriptFile) WriteString(s string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(s), nil
}

func (f staleScriptFile) Close() error { return f.closeErr }

// stagesInto makes the staging hand back the given file, and puts the real
// constructor back afterwards so no later case stages against a stand-in.
func stagesInto(t *testing.T, file scriptFile) {
	t.Helper()
	previous := createScriptFile
	createScriptFile = func() (scriptFile, error) { return file, nil }
	t.Cleanup(func() { createScriptFile = previous })
}

// dockerAnsweringReady is a stand-in that logs every call it is given and
// answers the readiness probe, so a case about staging reaches the staging and
// what happened around it can be read back.
func dockerAnsweringReady(t *testing.T) (docker dockerPath, log string) {
	t.Helper()
	log = filepath.Join(t.TempDir(), "calls.log")
	docker = stubDocker(t, `echo "$@" >> `+log+`
case "$1" in
  exec) echo `+readyAnswer+` ;;
  *) exit 0 ;;
esac
`)
	return docker, log
}

// TestDockerRun_WhenTheStagedScriptCannotBeWritten_NeverTouchesTheContainer
// verifies that a write which stopped short ends the run: the error names the
// step and carries the filesystem's own reason, and no copy of a script that
// was never written reaches the container.
//
// The write is the one step between creating the file and copying it in, so
// without this the guard could be deleted and every test here would still pass
// while a truncated script ran inside a GitLab and produced a record missing
// whatever the lost bytes asked for.
func TestDockerRun_WhenTheStagedScriptCannotBeWritten_NeverTouchesTheContainer(t *testing.T) {
	docker, log := dockerAnsweringReady(t)
	t.Setenv("PATH", filepath.Dir(string(docker)))
	shortWrite := errors.New("no space left on device")
	stagesInto(t, staleScriptFile{name: filepath.Join(t.TempDir(), "introspect.rb"), writeErr: shortWrite})

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("dockerRun returned no error for a script it could not write")
	}
	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("reading what the stand-in was asked: %v", readErr)
	}

	t.Run("the failure names the step", func(t *testing.T) {
		if !strings.Contains(err.Error(), "staging the introspection script") {
			t.Errorf("error = %q, want it to name the step that failed", err)
		}
	})
	t.Run("the filesystem's own reason is carried", func(t *testing.T) {
		if !errors.Is(err, shortWrite) {
			t.Errorf("error = %q, want it to wrap %v", err, shortWrite)
		}
	})
	t.Run("nothing was copied into the container", func(t *testing.T) {
		if strings.Contains(string(calls), "cp ") {
			t.Errorf("calls were:\n%s\nwant no copy of a script that was never written", calls)
		}
	})
	t.Run("the container is torn down anyway", func(t *testing.T) {
		if strings.Count(string(calls), "rm -f "+containerName) != 2 {
			t.Errorf("calls were:\n%s\nwant the container removed after the failure too", calls)
		}
	})
}

// TestDockerRun_WhenTheStagedScriptCannotBeClosed_NeverTouchesTheContainer
// verifies that a close which reports a failure ends the run the same way.
//
// A close is where a filesystem reports a write it had deferred, so a script
// whose every WriteString returned success can still be incomplete on disk at
// this point. Copying it in regardless is the one outcome this guard exists to
// prevent, and the write case above cannot stand in for it: the two read
// different variables and the close is the later of the two.
func TestDockerRun_WhenTheStagedScriptCannotBeClosed_NeverTouchesTheContainer(t *testing.T) {
	docker, log := dockerAnsweringReady(t)
	t.Setenv("PATH", filepath.Dir(string(docker)))
	deferredWrite := errors.New("input/output error")
	stagesInto(t, staleScriptFile{name: filepath.Join(t.TempDir(), "introspect.rb"), closeErr: deferredWrite})

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("dockerRun returned no error for a script it could not close")
	}
	calls, readErr := os.ReadFile(log)
	if readErr != nil {
		t.Fatalf("reading what the stand-in was asked: %v", readErr)
	}

	t.Run("the failure names the step", func(t *testing.T) {
		if !strings.Contains(err.Error(), "staging the introspection script") {
			t.Errorf("error = %q, want it to name the step that failed", err)
		}
	})
	t.Run("the filesystem's own reason is carried", func(t *testing.T) {
		if !errors.Is(err, deferredWrite) {
			t.Errorf("error = %q, want it to wrap %v", err, deferredWrite)
		}
	})
	t.Run("nothing was copied into the container", func(t *testing.T) {
		if strings.Contains(string(calls), "cp ") {
			t.Errorf("calls were:\n%s\nwant no copy of a script that was never closed", calls)
		}
	})
	t.Run("the container is torn down anyway", func(t *testing.T) {
		if strings.Count(string(calls), "rm -f "+containerName) != 2 {
			t.Errorf("calls were:\n%s\nwant the container removed after the failure too", calls)
		}
	})
}

// TestDockerRun_WhenTheScriptCannotBeCopiedIn_SaysWhatDockerSaid verifies the
// copy reports docker's own message.
//
// A copy fails for reasons this command cannot distinguish on its own — a
// container that died between the wait and the copy, a daemon that went away,
// a path docker will not write — and the only account of which it was is on
// docker's stderr.
func TestDockerRun_WhenTheScriptCannotBeCopiedIn_SaysWhatDockerSaid(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  exec) echo `+readyAnswer+` ;;
  cp) echo "no such container: `+containerName+`" >&2; exit 1 ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("dockerRun returned no error for a copy that failed")
	}
	if !strings.Contains(err.Error(), "copying the introspection script in") {
		t.Errorf("error = %q, want it to name the step that failed", err)
	}
	if !strings.Contains(err.Error(), "no such container") {
		t.Errorf("error = %q, want docker's own message in it", err)
	}
}

// TestDockerRun_WhenTheIntrospectionRaises_CarriesTheRunnersOwnStderr verifies
// that a script which raised inside the container reports why, and that the
// stderr runnerDetail formats is really reaching the error a caller sees.
//
// Only stdout is the answer, so stderr is discarded on every good run. On this
// one it holds the only account of what happened, and without it a maintainer
// is left with an exit status and a twenty-minute boot to repeat.
func TestDockerRun_WhenTheIntrospectionRaises_CarriesTheRunnersOwnStderr(t *testing.T) {
	docker := stubDocker(t, `case "$1" in
  exec)
    case "$5" in
      /tmp/introspect.rb) echo "NoMethodError: undefined method exposures" >&2; exit 1 ;;
      *) echo `+readyAnswer+` ;;
    esac
    ;;
  *) exit 0 ;;
esac
`)
	t.Setenv("PATH", filepath.Dir(string(docker)))

	_, _, err := dockerRun("gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("dockerRun returned no error for an introspection that raised")
	}
	if !strings.Contains(err.Error(), "running the introspection") {
		t.Errorf("error = %q, want it to name the step that failed", err)
	}
	if !strings.Contains(err.Error(), "NoMethodError: undefined method exposures") {
		t.Errorf("error = %q, want the runner's own stderr in it", err)
	}
}

// TestWaitForRails_WhenTheRunIsCancelledBetweenTwoPolls_StopsWaiting verifies
// the third place an interrupt can land, and the only one that is not a docker
// call failing under it: the pause between two attempts.
//
// A twenty-minute wait spends nearly all of its time asleep there, so this is
// where a real interrupt almost always arrives. A loop that noticed a
// cancellation only through a failing docker call would sleep out the whole
// poll interval first, which is what the poll being an hour long here would
// turn into a hang rather than a pass.
func TestWaitForRails_WhenTheRunIsCancelledBetweenTwoPolls_StopsWaiting(t *testing.T) {
	asked := filepath.Join(t.TempDir(), "inspected")
	docker := stubDocker(t, `case "$1" in
  exec) exit 1 ;;
  inspect) echo true; echo asked >> `+asked+` ;;
esac
`)
	previous := pollInterval
	t.Cleanup(func() { pollInterval = previous })
	pollInterval = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		// Cancelled once the container has been asked about, which is the last
		// thing the loop does before it sleeps, and after a settle far longer
		// than the moment between that answer and the sleep. Nothing is
		// asserted here: the test goroutine checks afterwards that the stand-in
		// really was asked, so a cancellation that arrived for any other reason
		// cannot pass for this one.
		for range 2000 {
			if _, err := os.Stat(asked); err == nil {
				break
			}
			time.Sleep(time.Millisecond)
		}
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := waitForRails(ctx, docker)

	if err == nil {
		t.Fatal("waitForRails returned no error for a run cancelled between polls")
	}
	if _, statErr := os.Stat(asked); statErr != nil {
		t.Fatalf("the wait never reached the sleep: the container was never inspected (%v)", statErr)
	}
	if !strings.Contains(err.Error(), "waiting for the application") {
		t.Errorf("error = %q, want it to name what it was waiting for", err)
	}
	if strings.Contains(err.Error(), "the container stopped") {
		t.Errorf("error = %q, want it not to blame a container docker called running", err)
	}
}

// TestRunMain_DispatchesOnItsFlagsAndReturnsTheExitCode verifies every way
// this command ends, which is the one thing main itself cannot be asked about
// once it has handed the answer to os.Exit.
//
// The record on disk is checked in every case, not only the generating ones:
// it is what says a parse that failed, or a check, left the directory exactly
// as it found it.
func TestRunMain_DispatchesOnItsFlagsAndReturnsTheExitCode(t *testing.T) {
	quietStderr(t)

	for _, testCase := range []struct {
		name string
		// stage returns the command line and the directory the case is about.
		stage func(t *testing.T) (args []string, dir string)
		want  int
		// wantRecord is whether the directory holds a record afterwards.
		wantRecord bool
	}{
		{
			name: "a whole record passes -check",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				writeRecordAt(t, dir, wholeEnough(), today())
				return []string{"gen_api_live", "-check", "-dir", dir}, dir
			},
			want: 0, wantRecord: true,
		},
		{
			name: "a directory holding no record fails -check",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				return []string{"gen_api_live", "-check", "-dir", dir}, dir
			},
			want: 1,
		},
		{
			name: "a dump on disk becomes the record",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				return []string{"gen_api_live", "-dump", writeDump(t, wholeEnough()), "-dir", dir}, dir
			},
			want: 0, wantRecord: true,
		},
		{
			name: "a dump that is not there writes nothing",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				return []string{"gen_api_live", "-dump", filepath.Join(dir, "absent.json"), "-dir", dir}, dir
			},
			want: 1,
		},
		{
			name: "a flag nobody can parse is the usage exit",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				return []string{"gen_api_live", "-bogus", "-dir", dir}, dir
			},
			want: 2,
		},
		{
			name: "asking for help ends clean",
			stage: func(t *testing.T) ([]string, string) {
				t.Helper()
				dir := t.TempDir()
				return []string{"gen_api_live", "-h", "-dir", dir}, dir
			},
			want: 0,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			args, dir := testCase.stage(t)

			got := runMain(args)

			if got != testCase.want {
				t.Errorf("runMain(%v) = %d, want %d", args, got, testCase.want)
			}
			_, statErr := os.Stat(apilive.Path(dir))
			if onDisk := statErr == nil; onDisk != testCase.wantRecord {
				t.Errorf("a record in %s = %v, want %v", dir, onDisk, testCase.wantRecord)
			}
		})
	}
}

// TestRunMain_WithNoDirectory_AsksForTheCheckoutsOwn verifies that -dir is the
// only thing that moves the record, and that passing one really does keep this
// command out of the checkout it is running in.
func TestRunMain_WithNoDirectory_AsksForTheCheckoutsOwn(t *testing.T) {
	quietStderr(t)
	previous := defaultRecordDir
	t.Cleanup(func() { defaultRecordDir = previous })

	dir := t.TempDir()
	writeRecordAt(t, dir, wholeEnough(), today())
	asked := 0
	defaultRecordDir = func() (string, error) {
		asked++
		return dir, nil
	}

	t.Run("with no -dir the checkout's own directory is read", func(t *testing.T) {
		if got := runMain([]string{"gen_api_live", "-check"}); got != 0 {
			t.Errorf("runMain = %d, want the record in the checkout's own directory to be read", got)
		}
		if asked != 1 {
			t.Errorf("the checkout's directory was asked for %d times, want once", asked)
		}
	})
	t.Run("with -dir the checkout is never consulted", func(t *testing.T) {
		asked = 0
		if got := runMain([]string{"gen_api_live", "-check", "-dir", dir}); got != 0 {
			t.Errorf("runMain = %d, want 0", got)
		}
		if asked != 0 {
			t.Error("the checkout's directory was asked for although -dir named one")
		}
	})
}

// TestRunMain_WithNoCheckoutToResolve_ReportsAndExitsOne holds the ending a
// command run from outside a checkout gets.
//
// Resolving the record's home is the one thing here that fails on something
// the person running it acts on, by passing -dir or by running it from the
// repository, so it is reported and exited on like every other such failure in
// this command. It used to panic through cmdutil.Must, whose own documentation
// excludes exactly this class, and a stack trace buries the one sentence that
// says what to do.
func TestRunMain_WithNoCheckoutToResolve_ReportsAndExitsOne(t *testing.T) {
	stderr := captureStderr(t)
	previous := defaultRecordDir
	t.Cleanup(func() { defaultRecordDir = previous })
	defaultRecordDir = func() (string, error) {
		return "", errors.New("no checkout here")
	}

	if got := runMain([]string{"gen_api_live", "-check"}); got != 1 {
		t.Errorf("runMain = %d, want 1", got)
	}
	if said := stderr(); !strings.Contains(said, "no checkout here") {
		t.Errorf("stderr = %q, want it to carry the reason", said)
	}
}

// TestRepositoryRecordDir_OutsideACheckout_SaysHowToNameOne verifies the
// resolver's own failure, which is what the exit above reports: the error
// names the flag that answers it, since a reader who is outside a checkout
// cannot act on "go.mod not found" alone.
func TestRepositoryRecordDir_OutsideACheckout_SaysHowToNameOne(t *testing.T) {
	t.Chdir(t.TempDir())

	got, err := repositoryRecordDir()
	if err == nil {
		t.Fatalf("repositoryRecordDir() = %q, want a refusal outside a checkout", got)
	}
	if !strings.Contains(err.Error(), "-dir") {
		t.Errorf("repositoryRecordDir error = %q, want it to name -dir", err)
	}
}

// TestRepositoryRecordDir_IsTheCheckoutsDocsDevelopment verifies what -dir
// defaults to: the record beside the other pinned records of the checkout this
// command was run from, rather than a path relative to whatever working
// directory it happened to be started in.
func TestRepositoryRecordDir_IsTheCheckoutsDocsDevelopment(t *testing.T) {
	got, err := repositoryRecordDir()
	if err != nil {
		t.Fatalf("repositoryRecordDir: %v", err)
	}

	root, err := cmdutil.RepositoryRoot(".")
	if err != nil {
		t.Fatalf("RepositoryRoot: %v", err)
	}
	if want := filepath.Join(root, apilive.DefaultDir); got != want {
		t.Errorf("repositoryRecordDir() = %q, want %q", got, want)
	}
	if _, statErr := os.Stat(apilive.Path(got)); statErr != nil {
		t.Errorf("the committed record is not at %s: %v", apilive.Path(got), statErr)
	}
}

// TestMain_HandsTheExitCodeToTheProcess verifies main is the one line it looks
// like: whatever runMain decided becomes the process's status, failures
// included.
//
// os.Args is replaced because the flag set would otherwise be handed the test
// binary's own flags, and the case is a failing one so that a main which
// exited zero unconditionally could not pass it.
func TestMain_HandsTheExitCodeToTheProcess(t *testing.T) {
	quietStderr(t)
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })

	os.Args = []string{"gen_api_live", "-check", "-dir", t.TempDir()}
	got := -1
	osExit = func(code int) { got = code }

	main()

	if got != 1 {
		t.Errorf("main() exited %d, want 1 for a directory holding no record", got)
	}
}
