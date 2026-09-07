// main_test.go covers the command's entry point: writing the artifact, and
// the check mode that gates it in CI.
package main

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleRecord = `{"package":"internal/tools/issues","test":"TestList","kind":"rest","method":"GET","path":"/projects/:id/issues","query":["state"]}`

// stubCatalog replaces the catalog build for the duration of a test, so a
// command test does not pay for the real catalog and the summary line stays
// out of the way of what is being asserted.
func stubCatalog(t *testing.T) {
	t.Helper()
	original := buildCatalog
	buildCatalog = func() ([]string, error) { return nil, nil }
	t.Cleanup(func() { buildCatalog = original })
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
// an artifact that no longer matches is a failure naming how to fix it.
func TestRun_Check_FailsOnADriftedInventory(t *testing.T) {
	stubCatalog(t)
	root, shardDir, outputPath := prepareRoot(t)

	tests := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{
			name:  "an artifact that was never written",
			setup: func(*testing.T) {},
			want:  "read ",
		},
		{
			name: "an artifact holding something else",
			setup: func(t *testing.T) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, outputPath), []byte("{}\n"), 0o600); err != nil {
					t.Fatalf("WriteFile error = %v", err)
				}
			},
			want: "out of date",
		},
		{
			name: "an artifact naming a request that is no longer issued",
			setup: func(t *testing.T) {
				t.Helper()
				stale := render([]row{{Package: "internal/tools/issues", Kind: "rest", Method: http.MethodGet, Path: "/projects/:id/moved"}})
				if err := os.WriteFile(filepath.Join(root, outputPath), stale, 0o600); err != nil {
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
			want: "read shard directory",
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
