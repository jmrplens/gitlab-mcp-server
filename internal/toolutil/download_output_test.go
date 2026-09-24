// download_output_test.go covers WriteDownloadOutputFile: that a download
// reaches its destination only when every byte of it did, and that anything
// short of that leaves the destination as it was and takes the temporary file
// with it.
package toolutil

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// errWriteCutShort stands in for whatever ends a download partway: an error
// answer, a dropped connection, a cancelled call.
var errWriteCutShort = errors.New("the body ended early")

// destinationState is what a destination held before a download began: its
// content, or absent when there was none.
type destinationState struct {
	path    string
	content []byte
	existed bool
}

// prepareDestination creates path holding content when existed is true, and
// records what it holds, so a test can assert later that it still does.
func prepareDestination(t *testing.T, path string, existed bool) destinationState {
	t.Helper()
	state := destinationState{path: path, existed: existed}
	if existed {
		state.content = []byte("the previous release, which must survive a failed download\n")
		if err := os.WriteFile(path, state.content, 0o600); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", path, err)
		}
	}
	return state
}

// assertUnchanged fails when the destination no longer holds what it held
// before the download, or exists when it did not.
func (s destinationState) assertUnchanged(t *testing.T) {
	t.Helper()
	got, err := os.ReadFile(s.path)
	if !s.existed {
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("os.ReadFile(%q) = %q, %v, want the destination still absent", s.path, got, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error = %v, want the previous content still there", s.path, err)
	}
	if string(got) != string(s.content) {
		t.Errorf("os.ReadFile(%q) = %q, want the previous content %q untouched", s.path, got, s.content)
	}
}

// assertNoPartialDownloads fails when dir still holds a temporary file a
// download was written to.
func assertNoPartialDownloads(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("os.ReadDir(%q) error = %v", dir, err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), partialDownloadPrefix) {
			t.Errorf("os.ReadDir(%q) holds %q, want no partial download left behind", dir, entry.Name())
		}
	}
}

// partialOf returns the temporary file behind the writer a download's write
// callback receives, so a test can stage what happens to it on disk.
func partialOf(t *testing.T, w io.Writer) *partialDownload {
	t.Helper()
	partial, ok := w.(*partialDownload)
	if !ok {
		t.Fatalf("write received %T, want the *partialDownload the helper creates", w)
	}
	return partial
}

// useMkdirAll replaces the directory creation for the duration of the test,
// which is where a race with another local principal would fall.
func useMkdirAll(t *testing.T, fn func(string, fs.FileMode) error) {
	t.Helper()
	original := mkdirAll
	mkdirAll = fn
	t.Cleanup(func() { mkdirAll = original })
}

// useSyncDownloadDirectory replaces the directory sync that follows a
// download's rename for the duration of the test, since no directory a test
// can make refuses to be synced.
func useSyncDownloadDirectory(t *testing.T, fn func(string) error) {
	t.Helper()
	original := syncDownloadDirectory
	syncDownloadDirectory = fn
	t.Cleanup(func() { syncDownloadDirectory = original })
}

// captureDebugLog redirects slog to a buffer at debug level for the duration
// of the test, which is the level a directory that cannot be synced is
// reported at.
func captureDebugLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return &buf
}

// directoryNotSyncedLog is the part of the log line a download writes when it
// is in place but its directory could not be synced.
const directoryNotSyncedLog = "its directory could not be synced"

// TestWriteDownloadOutputFile_CompleteWrite_PutsEveryByteAtTheDestination
// verifies that a write that returns nil ends with the destination holding
// exactly what was written, whether it was absent (with directories still to
// make) or held a longer file, and that the size reported is the count of
// bytes written across several writes.
//
// The longer previous file is the case that tells a replacement from an
// in-place write: a write through the old file without truncation would leave
// its tail after the new bytes.
func TestWriteDownloadOutputFile_CompleteWrite_PutsEveryByteAtTheDestination(t *testing.T) {
	tests := []struct {
		name    string
		nested  bool
		existed bool
	}{
		{name: "absent destination under directories not yet made", nested: true},
		{name: "destination already holding a longer file", existed: true},
	}
	chunks := []string{"first chunk, ", "second"}
	want := strings.Join(chunks, "")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.nested {
				dir = filepath.Join(dir, "releases", "v1")
			}
			destination := filepath.Join(dir, "artifact.bin")
			if tt.existed {
				prepareDestination(t, destination, true)
			}

			size, err := WriteDownloadOutputFile(destination, writeChunks(chunks))
			if err != nil {
				t.Fatalf("WriteDownloadOutputFile(%q) error = %v, want nil", destination, err)
			}
			if size != int64(len(want)) {
				t.Errorf("WriteDownloadOutputFile(%q) size = %d, want %d", destination, size, len(want))
			}
			if got, readErr := os.ReadFile(destination); readErr != nil || string(got) != want {
				t.Errorf("os.ReadFile(%q) = %q, %v, want %q", destination, got, readErr, want)
			}
			assertNoPartialDownloads(t, dir)
		})
	}
}

// writeChunks returns a write callback that writes each chunk in turn, so the
// size reported has to be the sum of several writes rather than the last one.
func writeChunks(chunks []string) func(io.Writer) error {
	return func(w io.Writer) error {
		for _, chunk := range chunks {
			if _, err := io.WriteString(w, chunk); err != nil {
				return err
			}
		}
		return nil
	}
}

// TestWriteDownloadOutputFile_WriteFails_LeavesTheDestinationAsItWas verifies
// the guarantee the helper exists for: a write that fails after some bytes
// arrived leaves the destination absent if it was absent and untouched if it
// held a file, takes its temporary file with it, and returns the write's own
// error unchanged, so a caller's classification of it still holds.
func TestWriteDownloadOutputFile_WriteFails_LeavesTheDestinationAsItWas(t *testing.T) {
	for _, existed := range []bool{false, true} {
		name := "destination absent"
		if existed {
			name = "destination held a file"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			before := prepareDestination(t, filepath.Join(dir, "artifact.bin"), existed)

			_, err := WriteDownloadOutputFile(before.path, func(w io.Writer) error {
				if _, writeErr := io.WriteString(w, "half of the"); writeErr != nil {
					return writeErr
				}
				return errWriteCutShort
			})
			if !errors.Is(err, errWriteCutShort) || err.Error() != errWriteCutShort.Error() {
				t.Errorf("WriteDownloadOutputFile() error = %v, want exactly %v", err, errWriteCutShort)
			}
			before.assertUnchanged(t)
			assertNoPartialDownloads(t, dir)
		})
	}
}

// TestWriteDownloadOutputFile_WritePanics_TakesItsTemporaryFileWithIt verifies
// that the temporary file is removed on the way out of a write that panics,
// which is why the cleanup is deferred rather than written on each error path:
// a panic a server recovers from would otherwise leave the file behind.
func TestWriteDownloadOutputFile_WritePanics_TakesItsTemporaryFileWithIt(t *testing.T) {
	dir := t.TempDir()
	before := prepareDestination(t, filepath.Join(dir, "artifact.bin"), true)

	recovered := func() (value any) {
		defer func() { value = recover() }()
		_, _ = WriteDownloadOutputFile(before.path, func(w io.Writer) error {
			_, _ = io.WriteString(w, "partial")
			panic("the writer broke")
		})
		return nil
	}()
	if recovered != "the writer broke" {
		t.Fatalf("recover() = %v, want the write's own panic", recovered)
	}
	before.assertUnchanged(t)
	assertNoPartialDownloads(t, dir)
}

// TestWriteDownloadOutputFile_CommitFails_LeavesTheDestinationAsItWas verifies
// the two ways the finished file can fail to be put in place: it cannot be
// flushed and closed, or the rename is refused because the destination turned
// into a directory while the bytes were being written. Either way the
// destination keeps what it had and the temporary file is gone.
func TestWriteDownloadOutputFile_CommitFails_LeavesTheDestinationAsItWas(t *testing.T) {
	tests := []struct {
		name    string
		stage   func(t *testing.T, partial *partialDownload)
		wantMsg string
	}{
		{
			name: "the file can no longer be flushed",
			stage: func(t *testing.T, partial *partialDownload) {
				t.Helper()
				if err := partial.file.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			},
			wantMsg: "write output file",
		},
		{
			name: "the destination became a directory",
			stage: func(t *testing.T, partial *partialDownload) {
				t.Helper()
				makeDirs(t, partial.destination)
				if err := os.WriteFile(filepath.Join(partial.destination, "kept.txt"), []byte("kept"), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			wantMsg: "move the download into place",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			destination := filepath.Join(dir, "artifact.bin")

			_, err := WriteDownloadOutputFile(destination, func(w io.Writer) error {
				if _, writeErr := io.WriteString(w, "every byte"); writeErr != nil {
					return writeErr
				}
				tt.stage(t, partialOf(t, w))
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("WriteDownloadOutputFile() error = %v, want one naming %q", err, tt.wantMsg)
			}
			if info, statErr := os.Lstat(destination); statErr == nil && !info.IsDir() {
				t.Errorf("os.Lstat(%q) = a %v, want no file put in place", destination, info.Mode())
			}
			assertNoPartialDownloads(t, dir)
		})
	}
}

// TestWriteDownloadOutputFile_TemporaryFileFate_DecidesWhatIsReported verifies
// what a failed download says about its temporary file. One that cannot be
// removed is the only outcome that leaves something behind, so it is reported
// beside the write's error rather than instead of it; one that is already
// gone leaves nothing behind, so the write's error comes back alone.
func TestWriteDownloadOutputFile_TemporaryFileFate_DecidesWhatIsReported(t *testing.T) {
	tests := []struct {
		name        string
		stage       func(t *testing.T, name string)
		wantRemoval bool
	}{
		{
			name: "replaced by a directory that is not empty",
			stage: func(t *testing.T, name string) {
				t.Helper()
				makeDirs(t, name)
				if err := os.WriteFile(filepath.Join(name, "occupant"), []byte("x"), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
			},
			wantRemoval: true,
		},
		{
			name:  "already removed by someone else",
			stage: func(*testing.T, string) {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "artifact.bin")

			_, err := WriteDownloadOutputFile(destination, func(w io.Writer) error {
				partial := partialOf(t, w)
				// Closed first because Windows cannot remove a file this
				// process still holds open.
				if closeErr := partial.file.Close(); closeErr != nil {
					t.Fatalf("Close() error = %v", closeErr)
				}
				if removeErr := os.Remove(partial.file.Name()); removeErr != nil {
					t.Fatalf("Remove() error = %v", removeErr)
				}
				tt.stage(t, partial.file.Name())
				return errWriteCutShort
			})
			if !errors.Is(err, errWriteCutShort) {
				t.Fatalf("WriteDownloadOutputFile() error = %v, want it to carry %v", err, errWriteCutShort)
			}
			if got := strings.Contains(err.Error(), "remove the partial download"); got != tt.wantRemoval {
				t.Errorf("WriteDownloadOutputFile() error = %q, reports the removal: %t, want %t", err, got, tt.wantRemoval)
			}
		})
	}
}

// TestWriteDownloadOutputFile_UnusableDestination_WritesNothing verifies that
// a destination refused while it is being prepared never reaches write and
// leaves nothing at the destination: one outside every allowed root, and one
// whose parent is a dangling symlink, which the first resolution cannot tell
// from a directory not yet made and the directory creation then refuses.
func TestWriteDownloadOutputFile_UnusableDestination_WritesNothing(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	dangling := filepath.Join(allowed, "dangling")
	symlinked := os.Symlink(filepath.Join(outside, "missing"), dangling) == nil
	confineLocalPathRoots(t, allowed)

	tests := []struct {
		name    string
		path    string
		wantMsg string
		skip    bool
	}{
		{name: "outside every allowed root", path: filepath.Join(outside, "artifact.bin"), wantMsg: "outside allowed directories"},
		{name: "parent is a dangling symlink", path: filepath.Join(dangling, "sub", "artifact.bin"), wantMsg: "create output directory", skip: !symlinked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			_, err := WriteDownloadOutputFile(tt.path, func(io.Writer) error {
				t.Errorf("write was called for a refused destination %q", tt.path)
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("WriteDownloadOutputFile(%q) error = %v, want one naming %q", tt.path, err, tt.wantMsg)
			}
			if _, statErr := os.Lstat(tt.path); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("os.Lstat(%q) error = %v, want nothing written", tt.path, statErr)
			}
		})
	}
}

// TestWriteDownloadOutputFile_DirectoryChangedAfterItWasMade_Refused verifies
// the second resolution and the exclusive create against what another local
// principal can do between the directory being made and the file being
// created in it: swap the new directory for a symlink out of the allowed
// roots, put a directory where the destination goes, or remove the directory
// again. None of them gets a byte written, and none reaches write.
func TestWriteDownloadOutputFile_DirectoryChangedAfterItWasMade_Refused(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	confineLocalPathRoots(t, allowed)
	probe := filepath.Join(root, "probe")
	canSymlink := os.Symlink(outside, probe) == nil

	tests := []struct {
		name         string
		after        func(dir, destination string) error
		wantMsg      string
		namesItsPath bool
		skip         bool
	}{
		{
			name: "the new directory swapped for a symlink out of the roots",
			after: func(dir, _ string) error {
				if err := os.Remove(dir); err != nil {
					return err
				}
				return os.Symlink(outside, dir)
			},
			wantMsg: "outside allowed directories",
			skip:    !canSymlink,
		},
		{
			name:    "a directory put where the destination goes",
			after:   func(_, destination string) error { return os.Mkdir(destination, 0o750) },
			wantMsg: "not a regular file",
		},
		{
			// Named after the file the destination resolves to rather than
			// the random temporary name the caller never saw.
			name:         "the new directory removed again",
			after:        func(dir, _ string) error { return os.Remove(dir) },
			wantMsg:      "create a temporary file beside output path ",
			namesItsPath: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("symlinks unsupported on this platform")
			}
			dir := filepath.Join(allowed, strings.ReplaceAll(tt.name, " ", "-"))
			destination := filepath.Join(dir, "artifact.bin")
			useMkdirAll(t, func(path string, perm fs.FileMode) error {
				if err := os.MkdirAll(path, perm); err != nil {
					return err
				}
				return tt.after(path, destination)
			})

			_, err := WriteDownloadOutputFile(destination, func(io.Writer) error {
				t.Errorf("write was called for a destination changed under it")
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("WriteDownloadOutputFile(%q) error = %v, want one naming %q", destination, err, tt.wantMsg)
			}
			// The tail the caller wrote, since the root above it may resolve
			// through a link to another spelling.
			if tail := filepath.Join(filepath.Base(dir), "artifact.bin") + ":"; tt.namesItsPath && (err == nil || !strings.Contains(err.Error(), tail)) {
				t.Errorf("WriteDownloadOutputFile(%q) error = %v, want it to name the destination %q", destination, err, tail)
			}
			if _, statErr := os.Stat(filepath.Join(outside, "artifact.bin")); !errors.Is(statErr, fs.ErrNotExist) {
				t.Errorf("os.Stat(outside) error = %v, want nothing written outside the roots", statErr)
			}
		})
	}
}

// TestWriteDownloadOutputFile_ReplacedDestination_IsANewOwnerOnlyFile verifies
// the two properties of a replacement the documentation states for Unix: the
// destination is a new file readable by the owner alone whatever mode the old
// one had, and a hard link to the old file keeps the old content, which is
// what tells a rename from a write through the old file. On Windows the new
// file takes the directory's inheritable ACL instead, which no mode bits
// describe, so there is nothing here to compare.
func TestWriteDownloadOutputFile_ReplacedDestination_IsANewOwnerOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no Unix permission bits to compare")
	}
	dir := t.TempDir()
	destination := filepath.Join(dir, "artifact.bin")
	if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	// Set apart from the write so the process umask cannot narrow it.
	if err := os.Chmod(destination, 0o644); err != nil { //nolint:gosec // The world-readable mode is the fixture: the replacement must not inherit it.
		t.Fatalf("Chmod() error = %v", err)
	}
	link := filepath.Join(dir, "old-link.bin")
	if err := os.Link(destination, link); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}

	if _, err := WriteDownloadOutputFile(destination, func(w io.Writer) error {
		_, err := io.WriteString(w, "new")
		return err
	}); err != nil {
		t.Fatalf("WriteDownloadOutputFile() error = %v", err)
	}

	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatalf("os.Lstat(%q) error = %v", destination, err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("os.Lstat(%q) permissions = %#o, want %#o", destination, got, 0o600)
	}
	if got, readErr := os.ReadFile(link); readErr != nil || string(got) != "old" {
		t.Errorf("os.ReadFile(%q) = %q, %v, want the old content kept by the hard link", link, got, readErr)
	}
}

// TestWriteDownloadOutputFile_LinkToAFileInTheRoots_ReplacesTheFileAndKeepsTheLink
// verifies what the documentation says of a destination named through a
// symlink to a regular file inside the allowed roots: the link is resolved,
// the file it names is what gets replaced, and the link itself stays a link,
// now naming the new file. Nothing refuses it and nothing tells the caller,
// which is why the documentation has to.
func TestWriteDownloadOutputFile_LinkToAFileInTheRoots_ReplacesTheFileAndKeepsTheLink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.bin")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	link := filepath.Join(dir, "artifact.bin")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	if _, err := WriteDownloadOutputFile(link, writeChunks([]string{"new"})); err != nil {
		t.Fatalf("WriteDownloadOutputFile(%q) error = %v, want nil", link, err)
	}

	if info, err := os.Lstat(link); err != nil || info.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("os.Lstat(%q) = %v, %v, want the link kept", link, info, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Errorf("os.ReadFile(%q) = %q, %v, want the file the link names replaced", target, got, err)
	}
	assertNoPartialDownloads(t, dir)
}

// TestWriteDownloadOutputFile_ALinkPlantedDuringTheWrite_IsReplacedNotWrittenThrough
// verifies the property the leaf race rests on: a symlink planted at the
// destination after the containment check, while the body is still arriving,
// is replaced by the rename and never written through, wherever it points.
// A link already there before the check is resolved to the file it names,
// and one planted after it is not, which is the difference the second row
// holds: its target is inside the roots and is still left as it was.
//
// Nothing else pins this. Every other symlink test puts the link in place
// before the check, and a commit that resolved the destination again before
// renaming passed them all while writing the new bytes into whatever the
// planted link named.
func TestWriteDownloadOutputFile_ALinkPlantedDuringTheWrite_IsReplacedNotWrittenThrough(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "workspace")
	outside := filepath.Join(root, "home")
	makeDirs(t, allowed, outside)
	confineLocalPathRoots(t, allowed)
	if os.Symlink(outside, filepath.Join(root, "probe")) != nil {
		t.Skip("symlinks unsupported on this platform")
	}

	tests := []struct {
		name      string
		targetDir string
	}{
		{name: "a link to a file outside the roots", targetDir: outside},
		{name: "a link to a file inside the roots", targetDir: allowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := strings.ReplaceAll(tt.name, " ", "-")
			destination := filepath.Join(allowed, base+".bin")
			target := filepath.Join(tt.targetDir, base+"-target.bin")
			if err := os.WriteFile(destination, []byte("old"), 0o600); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", destination, err)
			}
			if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
				t.Fatalf("WriteFile(%q) error = %v", target, err)
			}

			if _, err := WriteDownloadOutputFile(destination, plantLinkThenWrite(destination, target, "new")); err != nil {
				t.Fatalf("WriteDownloadOutputFile(%q) error = %v, want nil", destination, err)
			}

			assertFileHolds(t, target, "target")
			if info, lstatErr := os.Lstat(destination); lstatErr != nil || !info.Mode().IsRegular() {
				t.Errorf("os.Lstat(%q) = %v, %v, want a regular file in the link's place", destination, info, lstatErr)
			}
			assertFileHolds(t, destination, "new")
			assertNoPartialDownloads(t, allowed)
		})
	}
}

// plantLinkThenWrite is a download's write that, before it writes body,
// replaces the file at destination with a symlink to target: a link planted
// after the containment check, while the body is still arriving.
func plantLinkThenWrite(destination, target, body string) func(io.Writer) error {
	return func(w io.Writer) error {
		if err := os.Remove(destination); err != nil {
			return err
		}
		if err := os.Symlink(target, destination); err != nil {
			return err
		}
		_, err := w.Write([]byte(body))
		return err
	}
}

// assertFileHolds fails the test when path cannot be read or does not hold
// want.
func assertFileHolds(t *testing.T, path, want string) {
	t.Helper()
	if got, err := os.ReadFile(path); err != nil || string(got) != want {
		t.Errorf("os.ReadFile(%q) = %q, %v, want %q", path, got, err, want)
	}
}

// TestWriteDownloadOutputFile_Committed_SyncsTheDirectoryAfterTheRename
// verifies that a download ends by syncing the directory it was renamed into,
// once, and only after the rename: syncing the file makes its bytes durable
// but not the name now pointing at them, so without this a crash soon after
// the tool reports success can bring back the file the rename replaced.
func TestWriteDownloadOutputFile_Committed_SyncsTheDirectoryAfterTheRename(t *testing.T) {
	logs := captureDebugLog(t)
	dir := t.TempDir()
	destination := prepareDestination(t, filepath.Join(dir, "artifact.bin"), true).path
	canonicalDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q) error = %v", dir, err)
	}
	var synced []string
	var heldAtSync string
	useSyncDownloadDirectory(t, func(d string) error {
		synced = append(synced, d)
		got, readErr := os.ReadFile(destination)
		if readErr != nil {
			t.Errorf("os.ReadFile(%q) at the directory sync error = %v", destination, readErr)
		}
		heldAtSync = string(got)
		return syncDirectory(d)
	})

	if _, err = WriteDownloadOutputFile(destination, writeChunks([]string{"new"})); err != nil {
		t.Fatalf("WriteDownloadOutputFile(%q) error = %v, want nil", destination, err)
	}

	if len(synced) != 1 || synced[0] != canonicalDir {
		t.Errorf("directories synced = %q, want exactly [%q]", synced, canonicalDir)
	}
	if heldAtSync != "new" {
		t.Errorf("destination held %q when its directory was synced, want %q: the sync must follow the rename", heldAtSync, "new")
	}
	if strings.Contains(logs.String(), directoryNotSyncedLog) {
		t.Errorf("log = %s, want no report of a directory that could not be synced", logs)
	}
}

// TestWriteDownloadOutputFile_DirectorySyncFails_TheDownloadStillSucceeds
// verifies that a directory the filesystem refuses to sync does not turn a
// download that is already in place into a failure, which would tell the
// caller the file is absent while it sits at the destination, and that the
// refusal is logged rather than swallowed.
func TestWriteDownloadOutputFile_DirectorySyncFails_TheDownloadStillSucceeds(t *testing.T) {
	logs := captureDebugLog(t)
	dir := t.TempDir()
	destination := filepath.Join(dir, "artifact.bin")
	errDirectorySync := errors.New("this filesystem cannot sync a directory")
	useSyncDownloadDirectory(t, func(string) error { return errDirectorySync })

	size, err := WriteDownloadOutputFile(destination, writeChunks([]string{"new"}))
	if err != nil || size != int64(len("new")) {
		t.Fatalf("WriteDownloadOutputFile(%q) = %d, %v, want %d, nil", destination, size, err, len("new"))
	}
	if got, readErr := os.ReadFile(destination); readErr != nil || string(got) != "new" {
		t.Errorf("os.ReadFile(%q) = %q, %v, want the download in place", destination, got, readErr)
	}
	assertNoPartialDownloads(t, dir)
	if got := logs.String(); !strings.Contains(got, directoryNotSyncedLog) || !strings.Contains(got, errDirectorySync.Error()) {
		t.Errorf("log = %s, want it to carry %q and the refusal %q", got, directoryNotSyncedLog, errDirectorySync)
	}
}
