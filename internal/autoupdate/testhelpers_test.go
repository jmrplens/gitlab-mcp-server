package autoupdate

import (
	"os"
	"path/filepath"
	"testing"
)

// stubExecutablePath overrides resolveExecutable to return a fake binary
// inside t.TempDir(), preventing tests from creating .old/.tmp/.new files
// next to the real production binary.
func stubExecutablePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fakeBin := filepath.Join(dir, "gitlab-mcp-server")
	if err := os.WriteFile(fakeBin, []byte("fake-binary"), 0o755); err != nil {
		t.Fatalf("cannot create fake binary: %v", err)
	}

	orig := resolveExecutable
	resolveExecutable = func() (string, error) { return fakeBin, nil }
	t.Cleanup(func() { resolveExecutable = orig })
	return fakeBin
}

// stubExecSelf overrides the execSelf function variable to return the
// given error, allowing tests to simulate exec success/failure without
// calling syscall.Exec. Returns the path from stubExecutablePath.
func stubExecSelf(t *testing.T, err error) string {
	t.Helper()
	exe := stubExecutablePath(t)
	orig := execSelf
	execSelf = func() error { return err }
	t.Cleanup(func() { execSelf = orig })
	return exe
}
