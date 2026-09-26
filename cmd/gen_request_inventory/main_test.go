// main_test.go covers the command's entry point: writing the artifact, and
// the check mode that gates it in CI.
package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

const sampleRecord = `{"package":"internal/tools/issues","test":"TestList","kind":"rest","method":"GET","path":"/projects/:id/issues","query":["state"]}`

// stubCatalog replaces the catalog build for the duration of a test, so a
// command test does not pay for the real catalog and the summary line stays
// out of the way of what is being asserted.
func stubCatalog(t *testing.T) {
	t.Helper()
	original := catalogActions
	catalogActions = func() ([]requestinventory.Action, error) { return nil, nil }
	t.Cleanup(func() { catalogActions = original })
}

// prepareRoot lays out a repository root with one shard in it and returns the
// root, the shard directory relative to it, and the artifact path relative to
// it.
func prepareRoot(t *testing.T) (root, shardDir, outputPath string) {
	t.Helper()
	root = t.TempDir()
	shardDir = filepath.Join("dist", "request-inventory")
	outputPath = filepath.Join("docs", "request-inventory.json")
	if err := os.MkdirAll(filepath.Join(root, shardDir), 0o750); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, shardDir, "requests-1.jsonl"), []byte(sampleRecord+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
	return root, shardDir, outputPath
}

// TestRun_Write_ProducesTheInventoryTwiceOver verifies the property the gate
// depends on: the same shards produce the same bytes, so a diff in the
// committed artifact means a request changed and never that the generator did.
func TestRun_Write_ProducesTheInventoryTwiceOver(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)
	var progress bytes.Buffer

	if err := run(&progress, root, options{shardDir: shardDir, outputPath: outputPath}); err != nil {
		t.Fatalf("run error = %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, outputPath))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if err = run(&progress, root, options{shardDir: shardDir, outputPath: outputPath}); err != nil {
		t.Fatalf("second run error = %v", err)
	}
	second, err := os.ReadFile(filepath.Join(root, outputPath))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Error("two runs over the same shards wrote different bytes")
	}
	if !strings.Contains(string(first), "/projects/:id/issues") {
		t.Errorf("the inventory does not hold the recorded endpoint:\n%s", first)
	}
	if !strings.Contains(progress.String(), "1 rows") {
		t.Errorf("progress = %q, want it to count the rows", progress.String())
	}
}

// TestRun_Check_PassesOnACurrentInventory verifies the gate says nothing when
// the committed artifact is what the suite records.
func TestRun_Check_PassesOnACurrentInventory(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)
	if err := run(&bytes.Buffer{}, root, options{shardDir: shardDir, outputPath: outputPath}); err != nil {
		t.Fatalf("run error = %v", err)
	}

	if err := run(&bytes.Buffer{}, root, options{shardDir: shardDir, outputPath: outputPath, check: true}); err != nil {
		t.Errorf("check error = %v, want nil", err)
	}
}

// TestRun_Check_FailsOnADriftedInventory verifies the gate's whole purpose: a
// handler that starts calling a different endpoint changes the artifact, and
// an artifact that no longer matches is a failure naming how to fix it. Each
// failure names the artifact it judged, which is how a reader running -out
// against another file learns which one the gate read.
func TestRun_Check_FailsOnADriftedInventory(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)
	target := filepath.Join(root, outputPath)

	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{
			name:  "an artifact that was never written",
			setup: func(*testing.T) {},
			want:  "read " + target + ": ",
		},
		{
			name: "an artifact holding something else",
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
					t.Fatalf("WriteFile error = %v", err)
				}
			},
			want: target + " is out of date: run `make gen-request-inventory`",
		},
		{
			name: "an artifact naming a request that is no longer issued",
			setup: func(t *testing.T) {
				t.Helper()
				stale := render([]row{{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/moved"}})
				if err := os.WriteFile(target, stale, 0o600); err != nil {
					t.Fatalf("WriteFile error = %v", err)
				}
			},
			want: "/projects/:id/moved",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			err := run(&bytes.Buffer{}, root, options{shardDir: shardDir, outputPath: outputPath, check: true})
			if err == nil {
				t.Fatal("check succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// TestRun_AbsoluteShardDirectory_IsReadWhereItWasNamed verifies the two ends
// of the pipeline agree about what a path means.
//
// The recorder refuses a relative directory, because a test binary runs in its
// own package directory and would scatter one shard under each of them, so the
// suite always records into an absolute path. filepath.Join treats an absolute
// second element as relative, so resolving -shards against the repository root
// unconditionally made the directory the suite wrote to unreadable by the
// merge that has to read it.
func TestRun_AbsoluteShardDirectory_IsReadWhereItWasNamed(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)

	err := run(&bytes.Buffer{}, root, options{shardDir: filepath.Join(root, shardDir), outputPath: outputPath})
	if err != nil {
		t.Fatalf("run error = %v, want the absolute shard directory read", err)
	}
	written, err := os.ReadFile(filepath.Join(root, outputPath))
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !strings.Contains(string(written), "/projects/:id/issues") {
		t.Errorf("the inventory does not hold the recorded endpoint:\n%s", written)
	}
}

// prepareCheckout lays out a repository root the command can be run from: a
// go.mod for the root lookup to find, a shard directory holding one recording,
// and a docs directory for the artifact. It chdirs into the root and returns
// the flags naming those two paths.
//
// The command resolves its own root from the working directory, so a test that
// drives it must give it one that is not this repository: an artifact written
// here would be the committed one.
func prepareCheckout(t *testing.T) (artifact string, args []string) {
	t.Helper()
	root, shardDir, outputPath := prepareRoot(t)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	t.Chdir(root)
	return filepath.Join(root, outputPath), []string{"-shards", shardDir, "-out", outputPath}
}

// TestRunMain_FlagParsing_ReturnsTheExitCode verifies that a parse failure is
// an exit code this function returns rather than an os.Exit inside the flag
// package: an unknown flag is the usage exit, 2, and -h is the one parse
// failure that exits clean, which is what ExitOnError would have done for both.
// Neither reaches the merge, which the absent artifact witnesses.
func TestRunMain_FlagParsing_ReturnsTheExitCode(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
		text string
	}{
		{name: "an unknown flag is a usage error", args: []string{"-bogus"}, want: 2, text: "flag provided but not defined: -bogus"},
		{name: "asking for help exits clean", args: []string{"-h"}, want: 0, text: "-check"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCatalog(t)
			artifact, args := prepareCheckout(t)

			var stderr bytes.Buffer
			if code := runMain(append(tt.args, args...), &stderr); code != tt.want {
				t.Fatalf("runMain(%v) = %d, want %d (stderr %q)", tt.args, code, tt.want, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.text) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.text)
			}
			if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("os.Stat(artifact) = %v, want nothing written past a failed parse", err)
			}
		})
	}
}

// TestRunMain_WriteRunFromACheckout_WritesTheArtifactAndExitsZero verifies the
// ordinary invocation end to end: the root is found from the working
// directory, the shards are merged, the artifact lands where -out named it, and
// the exit code is 0. The progress the run prints goes to the stderr it was
// handed, so a caller redirecting stdout still gets pure output.
func TestRunMain_WriteRunFromACheckout_WritesTheArtifactAndExitsZero(t *testing.T) {
	stubCatalog(t)
	artifact, args := prepareCheckout(t)

	var stderr bytes.Buffer
	if code := runMain(args, &stderr); code != 0 {
		t.Fatalf("runMain() = %d, want 0 (stderr %q)", code, stderr.String())
	}

	written, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatalf("ReadFile error = %v", err)
	}
	if !strings.Contains(string(written), "/projects/:id/issues") {
		t.Errorf("the artifact does not hold the recorded endpoint:\n%s", written)
	}
	if !strings.Contains(stderr.String(), "1 rows") {
		t.Errorf("stderr = %q, want the progress summary on it", stderr.String())
	}
}

// TestRunMain_AFailingStage_ExitsOneAndNamesIt verifies the two failures that
// share the exit code 1 still say which they are, since they are fixed by
// different things: one means this is not a checkout, the other that the merge
// or the comparison refused.
func TestRunMain_AFailingStage_ExitsOneAndNamesIt(t *testing.T) {
	tests := []struct {
		name string
		args func(t *testing.T, args []string) []string
		want string
	}{
		{
			name: "no repository above the working directory",
			args: func(t *testing.T, args []string) []string {
				t.Helper()
				t.Chdir(t.TempDir())
				return args
			},
			want: "find repository root: go.mod not found",
		},
		{
			name: "nothing has recorded a run",
			args: func(t *testing.T, _ []string) []string {
				t.Helper()
				return []string{"-shards", filepath.Join("dist", "absent")}
			},
			want: "make record-request-inventory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCatalog(t)
			_, args := prepareCheckout(t)

			var stderr bytes.Buffer
			if code := runMain(tt.args(t, args), &stderr); code != 1 {
				t.Fatalf("runMain() = %d, want 1 (stderr %q)", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.want)
			}
		})
	}
}

// TestRunMain_CheckAndVerbose_EachReachTheOptionTheyName verifies the two
// boolean flags are not exchanged on their way into the options.
//
// They are the one pair here no other test can tell apart: every run above
// leaves both false, and a block of flags has no fixture where no two values
// agree, so each is driven on its own against the effect only it has. -check
// refuses to write the artifact, which the run without it always writes, and -v
// names the owners behind the coverage count, which the run without it only
// scores.
func TestRunMain_CheckAndVerbose_EachReachTheOptionTheyName(t *testing.T) {
	t.Run("-check writes nothing and reports the artifact it could not read", func(t *testing.T) {
		stubCatalog(t)
		artifact, args := prepareCheckout(t)

		var stderr bytes.Buffer
		if code := runMain(append([]string{"-check"}, args...), &stderr); code != 1 {
			t.Fatalf("runMain(-check) = %d, want 1 (stderr %q)", code, stderr.String())
		}
		if !strings.Contains(stderr.String(), "read ") {
			t.Errorf("stderr = %q, want it to name the artifact it could not read", stderr.String())
		}
		if _, err := os.Stat(artifact); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("os.Stat(artifact) = %v, want -check to have written nothing", err)
		}
	})

	t.Run("-v names the owner behind the coverage count", func(t *testing.T) {
		original := catalogActions
		catalogActions = func() ([]requestinventory.Action, error) {
			return []requestinventory.Action{{ID: "ghost.list", Owner: "nowhere"}}, nil
		}
		t.Cleanup(func() { catalogActions = original })
		_, args := prepareCheckout(t)

		var quiet, verbose bytes.Buffer
		if code := runMain(args, &quiet); code != 0 {
			t.Fatalf("runMain() = %d, want 0 (stderr %q)", code, quiet.String())
		}
		if code := runMain(append([]string{"-v"}, args...), &verbose); code != 0 {
			t.Fatalf("runMain(-v) = %d, want 0 (stderr %q)", code, verbose.String())
		}

		if strings.Contains(quiet.String(), "nowhere") {
			t.Errorf("stderr = %q, want no owner named without -v", quiet.String())
		}
		if !strings.Contains(verbose.String(), "no package of that name: nowhere") {
			t.Errorf("stderr = %q, want -v to name the owner", verbose.String())
		}
	})
}

// TestMain_HandsTheExitCodeToTheSeam verifies main wires runMain's result to
// the exit seam and reads its flags from os.Args, which is the only thing main
// does and the one line no other test here reaches.
func TestMain_HandsTheExitCodeToTheSeam(t *testing.T) {
	stubCatalog(t)
	_, args := prepareCheckout(t)
	oldArgs := os.Args
	os.Args = append([]string{"gen_request_inventory"}, args...)
	t.Cleanup(func() { os.Args = oldArgs })
	code := -1
	osExit = func(got int) { code = got }
	t.Cleanup(func() { osExit = os.Exit })

	main()

	if code != 0 {
		t.Errorf("main() exited %d, want 0", code)
	}
}

// TestRun_UnusableInputOrOutput_NamesTheStage verifies that each failure says
// which half of the run it came from, since the two are fixed differently: one
// means the suite was not recorded, the other that the artifact cannot be
// written where it was asked for.
func TestRun_UnusableInputOrOutput_NamesTheStage(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)

	tests := []struct {
		name string
		opts options
		want string
	}{
		{
			name: "no shards to merge",
			opts: options{shardDir: filepath.Join("dist", "absent"), outputPath: outputPath},
			want: "make record-request-inventory",
		},
		{
			name: "an artifact path that is not writable",
			opts: options{shardDir: shardDir, outputPath: filepath.Join("absent", "request-inventory.json")},
			want: "write ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := run(&bytes.Buffer{}, root, tt.opts)
			if err == nil {
				t.Fatal("run succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}
