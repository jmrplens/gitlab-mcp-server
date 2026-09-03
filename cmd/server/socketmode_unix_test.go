//go:build !windows

// socketmode_unix_test.go covers the two properties bindUnixSocket exists to
// hold at once: the socket carries the operator's permission mode before
// anybody can reach it, and getting it there disturbs nothing else in the
// process.
//
// The second one is not decoration. The implementation this replaced set the
// process-global umask around the bind, which every other goroutine's file
// creation inherited; CI caught it as unrelated tests whose t.TempDir() came
// back without its execute bit.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stagingLeftovers returns the staging directories bindUnixSocket created in
// dir and failed to clean up. It must always be empty: a leftover staging
// directory is a socket nobody can reach and a name nobody will remove.
func stagingLeftovers(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %q: %v", dir, err)
	}
	var leftovers []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), stagingDirPrefix) {
			leftovers = append(leftovers, entry.Name())
		}
	}
	return leftovers
}

// TestBindUnixSocket_CreatesTheSocketWithTheRequestedMode pins the mode on the
// published path for every value an operator can ask for.
//
// The mode is the difference between a socket only the proxy's group can open
// and one every local account on the machine can, so a mode that silently did
// not apply would be worse than a refusal.
func TestBindUnixSocket_CreatesTheSocketWithTheRequestedMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []os.FileMode{0o600, 0o640, 0o660, 0o666, 0o700} {
		t.Run(fmt.Sprintf("%#o", mode), func(t *testing.T) {
			t.Parallel()

			dir := socketDir(t)
			path := filepath.Join(dir, "m.sock")
			listener, err := bindUnixSocket(t.Context(), path, mode)
			if err != nil {
				t.Fatalf("bindUnixSocket(%#o) error = %v", mode, err)
			}
			defer func() { _ = listener.Close() }()

			info, statErr := os.Lstat(path)
			if statErr != nil {
				t.Fatalf("the socket was not published at %q: %v", path, statErr)
			}
			if info.Mode()&fs.ModeSocket == 0 {
				t.Fatalf("%q is %v, not a socket", path, info.Mode())
			}
			if got := info.Mode().Perm(); got != mode {
				t.Errorf("socket mode = %#o, want %#o", got, mode)
			}
			if leftovers := stagingLeftovers(t, dir); len(leftovers) != 0 {
				t.Errorf("staging directories left behind: %v", leftovers)
			}

			// A socket with the right bits that nobody can talk to would pass
			// every assertion above.
			conn, dialErr := (&net.Dialer{}).DialContext(t.Context(), "unix", path)
			if dialErr != nil {
				t.Fatalf("dialing the published socket: %v", dialErr)
			}
			_ = conn.Close()
		})
	}
}

// occupyWithRegularFile puts a plain file where the socket wants to go, the
// shape a typo in --http-addr produces.
func occupyWithRegularFile(t *testing.T, path string) func(*testing.T) {
	t.Helper()

	if err := os.WriteFile(path, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("writing the occupant: %v", err)
	}
	return func(t *testing.T) {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "not a socket" {
			t.Errorf("the occupying file was disturbed: %q, %v", content, err)
		}
	}
}

// occupyWithDirectory puts a directory at the path, which link(2) must refuse
// like anything else that already exists.
func occupyWithDirectory(t *testing.T, path string) func(*testing.T) {
	t.Helper()

	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("creating the occupant: %v", err)
	}
	return func(t *testing.T) {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() {
			t.Errorf("the occupying directory was disturbed: %v, %v", info, err)
		}
	}
}

// occupyWithLiveSocket stands in for the case the guarantee exists for: a
// second instance started by mistake while the first is still serving.
func occupyWithLiveSocket(t *testing.T, path string) func(*testing.T) {
	t.Helper()

	occupant := listenUnixForTest(t, path)
	t.Cleanup(func() { _ = occupant.Close() })
	return func(t *testing.T) {
		t.Helper()
		conn, err := (&net.Dialer{}).DialContext(t.Context(), "unix", path)
		if err != nil {
			t.Errorf("the socket that was already being served is no longer reachable: %v", err)
			return
		}
		_ = conn.Close()
	}
}

// occupyWithSymlink plants the attack this whole file is built around: a
// symlink at the target path, pointing at a file the socket mode must never
// be applied to (CWE-367).
func occupyWithSymlink(t *testing.T, path string) func(*testing.T) {
	t.Helper()

	target := filepath.Join(filepath.Dir(path), "secret")
	if err := os.WriteFile(target, []byte("private"), 0o600); err != nil {
		t.Fatalf("writing the symlink target: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("creating the occupant: %v", err)
	}
	return func(t *testing.T) {
		t.Helper()
		info, err := os.Lstat(target)
		if err != nil {
			t.Fatalf("stat the symlink target: %v", err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Errorf("the symlink target's mode became %#o; the socket mode landed on it", got)
		}
		link, linkErr := os.Lstat(path)
		if linkErr != nil || link.Mode()&fs.ModeSymlink == 0 {
			t.Errorf("the planted symlink was replaced: %v, %v", link, linkErr)
		}
	}
}

// TestBindUnixSocket_RefusesToClobberAnExistingPath verifies the no-clobber
// guarantee, which is the reason the socket is published with link(2) rather
// than moved into place with rename(2).
//
// rename would replace whatever is at the path, so a second instance started
// by mistake would take over the socket the first is still serving and the
// operator would see no error at all. link refuses.
func TestBindUnixSocket_RefusesToClobberAnExistingPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// occupy puts something at path and returns a check to run after the
		// refusal, proving the occupant survived untouched.
		occupy func(t *testing.T, path string) func(*testing.T)
	}{
		{name: "a regular file", occupy: occupyWithRegularFile},
		{name: "a directory", occupy: occupyWithDirectory},
		{name: "a live socket another process is serving", occupy: occupyWithLiveSocket},
		{name: "a symlink pointing at somebody else's file", occupy: occupyWithSymlink},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := socketDir(t)
			path := filepath.Join(dir, "s.sock")
			verify := tt.occupy(t, path)

			listener, err := bindUnixSocket(t.Context(), path, 0o660)
			if err == nil {
				_ = listener.Close()
				t.Fatalf("bindUnixSocket(%q) replaced an occupied path", path)
			}
			if !errors.Is(err, fs.ErrExist) {
				t.Errorf("error = %v, want it to report that the path already exists", err)
			}
			verify(t)
			if leftovers := stagingLeftovers(t, dir); len(leftovers) != 0 {
				t.Errorf("a refused bind left staging directories behind: %v", leftovers)
			}
		})
	}
}

// TestBindUnixSocket_LeavesConcurrentFileCreationAlone is the regression test
// for the reason this implementation exists.
//
// The previous one set the process-global umask for the length of the bind, so
// a directory any other goroutine created inside that window came back with
// bits cleared: mkdir asking for 0700 under the umask 0117 that a 0660 socket
// implies yields 0600, and a directory without its execute bit cannot be
// entered. This binds in a loop while creating directories and asserts they
// all come back with the mode the very first one got, which calibrates the
// expectation against whatever umask the machine running the test has.
//
// It cannot fail spuriously: the only way the modes diverge is if binding
// changed process-wide state. Against the umask implementation it caught 87 of
// 200 directories.
func TestBindUnixSocket_LeavesConcurrentFileCreationAlone(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	baseline := filepath.Join(dir, "baseline")
	if err := os.Mkdir(baseline, 0o700); err != nil {
		t.Fatalf("creating the baseline directory: %v", err)
	}
	baselineInfo, err := os.Stat(baseline)
	if err != nil {
		t.Fatalf("stat the baseline directory: %v", err)
	}
	want := baselineInfo.Mode().Perm()

	stop := make(chan struct{})
	binderErr := make(chan error, 1)
	go func() {
		// No assertions off the test goroutine: the first failure travels
		// back on the channel and the main goroutine reports it.
		for {
			select {
			case <-stop:
				binderErr <- nil
				return
			default:
			}
			listener, bindErr := bindUnixSocket(context.Background(), filepath.Join(dir, "b.sock"), 0o660)
			if bindErr != nil {
				binderErr <- bindErr
				return
			}
			if closeErr := listener.Close(); closeErr != nil {
				binderErr <- closeErr
				return
			}
		}
	}()

	var disturbed []string
	for i := range 200 {
		path := filepath.Join(dir, fmt.Sprintf("d%d", i))
		if mkErr := os.Mkdir(path, 0o700); mkErr != nil {
			disturbed = append(disturbed, fmt.Sprintf("%s: %v", path, mkErr))
			continue
		}
		info, statErr := os.Stat(path)
		if statErr != nil {
			disturbed = append(disturbed, fmt.Sprintf("%s: %v", path, statErr))
			continue
		}
		if got := info.Mode().Perm(); got != want {
			disturbed = append(disturbed, fmt.Sprintf("%s: mode %#o, want %#o", path, got, want))
		}
	}
	close(stop)

	if loopErr := <-binderErr; loopErr != nil {
		t.Fatalf("binding in a loop failed: %v", loopErr)
	}
	if len(disturbed) != 0 {
		t.Errorf("binding a unix socket changed how %d of 200 concurrent directories were created:\n%s",
			len(disturbed), strings.Join(disturbed, "\n"))
	}
}

// TestBindUnixSocket_ListenerOwnsThePublishedPath verifies that the listener
// hands callers the address they asked for and cleans that address up.
//
// The socket is bound under a staging name, so an unwrapped listener would
// report a directory that no longer exists and would unlink the wrong name on
// Close, leaving the operator's path behind forever.
func TestBindUnixSocket_ListenerOwnsThePublishedPath(t *testing.T) {
	t.Parallel()

	dir := socketDir(t)
	path := filepath.Join(dir, "owned.sock")
	listener, err := bindUnixSocket(t.Context(), path, 0o660)
	if err != nil {
		t.Fatalf("bindUnixSocket() error = %v", err)
	}

	if got := listener.Addr().String(); got != path {
		t.Errorf("Addr() = %q, want the published path %q", got, path)
	}
	if closeErr := listener.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("the published path survived Close(): %v", statErr)
	}
	if leftovers := stagingLeftovers(t, dir); len(leftovers) != 0 {
		t.Errorf("staging directories left behind: %v", leftovers)
	}
}

// TestBindUnixSocket_CloseKeepsASuccessorsSocket covers the shutdown race the
// standard library documents and cannot fix.
//
// A restart can bind the same path before the outgoing process finishes
// closing. net.UnixListener would unlink by name and delete the successor's
// socket; this one removes the path only while it still names the inode it
// published.
func TestBindUnixSocket_CloseKeepsASuccessorsSocket(t *testing.T) {
	t.Parallel()

	dir := socketDir(t)
	path := filepath.Join(dir, "handover.sock")
	outgoing, err := bindUnixSocket(t.Context(), path, 0o660)
	if err != nil {
		t.Fatalf("bindUnixSocket() error = %v", err)
	}

	// The successor takes the path over the way a restart does.
	if removeErr := os.Remove(path); removeErr != nil {
		t.Fatalf("removing the outgoing socket: %v", removeErr)
	}
	successor, successorErr := bindUnixSocket(t.Context(), path, 0o660)
	if successorErr != nil {
		t.Fatalf("the successor could not bind: %v", successorErr)
	}
	defer func() { _ = successor.Close() }()

	if closeErr := outgoing.Close(); closeErr != nil {
		t.Fatalf("closing the outgoing listener: %v", closeErr)
	}

	conn, dialErr := (&net.Dialer{}).DialContext(t.Context(), "unix", path)
	if dialErr != nil {
		t.Fatalf("the outgoing listener deleted the successor's socket: %v", dialErr)
	}
	_ = conn.Close()
}

// TestBindUnixSocket_UnwritableDirectoryIsRefused covers the failure an
// operator produces by pointing --http-addr into a directory the server does
// not own.
//
// It is the one listener failure whose outcome depends on who runs it:
// permission bits refuse an ordinary account and never refuse root, so it
// skips when the test process is privileged rather than asserting an error the
// kernel would not produce. CI runs unprivileged, so CI runs it.
func TestBindUnixSocket_UnwritableDirectoryIsRefused(t *testing.T) {
	t.Parallel()

	if os.Geteuid() == 0 {
		t.Skip("running as root: permission bits refuse nothing, so there is no refusal to assert")
	}

	dir := socketDir(t)
	closed := filepath.Join(dir, "closed")
	if err := os.Mkdir(closed, 0o700); err != nil {
		t.Fatalf("creating the directory: %v", err)
	}
	// Readable and searchable but not writable is the whole point, so the
	// mode cannot be narrowed to satisfy a permissions linter.
	if chmodErr := os.Chmod(closed, 0o500); chmodErr != nil { //nolint:gosec // see above
		t.Fatalf("making the directory unwritable: %v", chmodErr)
	}

	listener, err := bindUnixSocket(t.Context(), filepath.Join(closed, "m.sock"), 0o660)
	if err == nil {
		_ = listener.Close()
		t.Fatal("bindUnixSocket() bound a socket in a directory it cannot write in")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("error = %v, want it to report the permission denial", err)
	}
	if leftovers := stagingLeftovers(t, closed); len(leftovers) != 0 {
		t.Errorf("a refused bind left staging directories behind: %v", leftovers)
	}
}

// TestBindUnixSocket_UnreachablePublishedPathIsRefused covers the startup
// probe: a socket that cannot be connected to must stop the server rather than
// be served.
//
// The probe exists because publishing with link(2) means the address a client
// dials is not the address the kernel was given at bind time. Apple's sources
// settle that for the syscall layer and for HFS+, but APFS is closed, so the
// filesystem every current Mac uses is covered by inference. The probe replaces
// that inference with a connect. Its failure branch is stubbed because no
// filesystem this runs on is known to produce it.
func TestBindUnixSocket_UnreachablePublishedPathIsRefused(t *testing.T) {
	// Deliberately not parallel: it swaps a package-level seam.
	original := probeUnixSocket
	t.Cleanup(func() { probeUnixSocket = original })
	probeUnixSocket = func(context.Context, string) error {
		return errors.New("no client could reach it")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "unreachable.sock")
	listener, err := bindUnixSocket(t.Context(), path, 0o660)
	if err == nil {
		_ = listener.Close()
		t.Fatal("bindUnixSocket() served a socket that failed its own reachability probe")
	}
	if !strings.Contains(err.Error(), "no client could reach it") {
		t.Errorf("error = %v, want the probe's own reason", err)
	}
	// A socket left at the path would be a dead address the next start would
	// then have to decide whether to remove.
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("the failed socket was left at %q: %v", path, statErr)
	}
	if leftovers := stagingLeftovers(t, dir); len(leftovers) != 0 {
		t.Errorf("a refused bind left staging directories behind: %v", leftovers)
	}
}

// padDirTo creates a directory whose absolute path is exactly length bytes, so
// a test can sit on either side of the unix address limit deliberately rather
// than by guessing at a repeat count.
func padDirTo(t *testing.T, length int) string {
	t.Helper()

	// socketDir rather than t.TempDir for the reason given there: the test's
	// own name is longer than some of the budgets being measured here.
	base := socketDir(t)

	needed := length - len(base) - 1
	if needed < 1 {
		t.Skipf("the temporary directory %q is already %d bytes, too long to pad to %d", base, len(base), length)
	}
	dir := filepath.Join(base, strings.Repeat("p", needed))
	if mkErr := os.Mkdir(dir, 0o700); mkErr != nil {
		t.Fatalf("padding to %d bytes: %v", length, mkErr)
	}
	return dir
}

// TestBindUnixSocket_RefusesAPathNoAddressCanHold verifies that an over-long
// path is refused rather than bound.
//
// The kernel used to enforce this for free, because the address it was handed
// was the operator's path. It no longer sees that path: the socket is bound
// under a short staging name and published with link(2), which has no address
// limit at all. Without an explicit check the server would start, report
// itself listening, and no client would ever be able to connect.
//
// Both budgets are covered because they run out separately. The staging
// directory sits one level below the published path, so a path that fits can
// still be staged into one that does not, and the operator has to be told
// which of the two it was.
func TestBindUnixSocket_RefusesAPathNoAddressCanHold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    func(t *testing.T) string
		wantErr string
	}{
		{
			name: "the published path is too long",
			path: func(t *testing.T) string {
				t.Helper()
				return filepath.Join(padDirTo(t, maxUnixPathLen-4), "toolong.sock")
			},
			wantErr: "the socket path is",
		},
		{
			name: "the published path fits but the staging path does not",
			path: func(t *testing.T) string {
				t.Helper()
				// The staged name adds 13 bytes to the directory and the
				// published one adds 2, so this sits one byte above the limit
				// and ten below it.
				return filepath.Join(padDirTo(t, maxUnixPathLen-12), "s")
			},
			wantErr: "the staging path the socket is built under is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			path := tt.path(t)
			listener, err := bindUnixSocket(t.Context(), path, 0o660)
			if err == nil {
				_ = listener.Close()
				t.Fatalf("bindUnixSocket(%d-byte path) bound a socket no client could reach", len(path))
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to say %q", err, tt.wantErr)
			}
			if leftovers := stagingLeftovers(t, filepath.Dir(path)); len(leftovers) != 0 {
				t.Errorf("a refused bind left staging directories behind: %v", leftovers)
			}
		})
	}
}

// TestNewStagingDir_AParentThatCannotHoldIt_IsReported covers the reservation
// failing, which is the branch that decides whether a bind proceeds at all.
//
// It matters because the staging directory is what makes the chmod safe: the
// socket is assembled somewhere no other account can reach. If the directory
// could not be created and the code carried on regardless, the mode would be
// applied on a path anyone could have swapped underneath it, which is the race
// this whole file exists to avoid.
func TestNewStagingDir_AParentThatCannotHoldIt_IsReported(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	missing := filepath.Join(t.TempDir(), "no-such-directory")

	if _, err := newStagingDir(missing); err == nil {
		t.Fatal("newStagingDir() accepted a parent that does not exist")
	}
}

// TestRestrictDirToOwner_ANonDirectory_IsRefused verifies the O_DIRECTORY half
// of the descriptor-based mode application.
//
// The mode is applied through a descriptor rather than by path so that
// anything swapping the directory between the mkdir and the chmod makes the
// open fail rather than the mode land somewhere else. A regular file standing
// where a directory should be is the readable form of that swap.
func TestRestrictDirToOwner_ANonDirectory_IsRefused(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("writing the stand-in: %v", err)
	}

	if err := restrictDirToOwner(notADir); err == nil {
		t.Fatal("restrictDirToOwner() accepted something that is not a directory")
	}
}

// TestRestrictDirToOwner_ASymlink_IsRefused verifies the O_NOFOLLOW half.
//
// A symlink pointing at a directory this process may legitimately open is the
// exact shape the flag is for: without it the mode would be applied to the
// target, which is chosen by whoever planted the link rather than by this
// process.
func TestRestrictDirToOwner_ASymlink_IsRefused(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("creating the target directory: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("this filesystem does not support symlinks: %v", err)
	}

	if err := restrictDirToOwner(link); err == nil {
		t.Fatal("restrictDirToOwner() followed a symlink, so the mode could land on a directory it did not choose")
	}
}

// TestConfirmReachable_APathNothingIsServing_IsReported covers the check that
// turns an unverifiable assumption into a startup failure.
//
// The published path is reached through a hard link, and whether a hard link
// to a bound socket is connectable is a filesystem property this project
// cannot read on every platform it ships to. So it is proven once per start
// with a real connect rather than assumed, and this is the failing half of
// that proof.
func TestConfirmReachable_APathNothingIsServing_IsReported(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	absent := filepath.Join(t.TempDir(), "nothing.sock")

	if err := confirmReachable(t.Context(), absent); err == nil {
		t.Fatal("confirmReachable() reported a path nothing is serving as reachable")
	}
}

// TestPublishStagedSocket_AListenerThatIsNotUnix_IsRefused covers the type
// assertion, which is the one branch that cannot be reached through
// bindUnixSocket at all.
//
// It is worth a test rather than a panic because the function hands back a
// listener that owns a filesystem path: a TCP listener given the same job
// would report an address nothing published and unlink a path it never
// created.
func TestPublishStagedSocket_AListenerThatIsNotUnix_IsRefused(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	tcp, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listening on tcp: %v", err)
	}
	t.Cleanup(func() { _ = tcp.Close() })

	dir := t.TempDir()
	_, err = publishStagedSocket(tcp, filepath.Join(dir, "staged"), filepath.Join(dir, "published"), 0o660)
	if err == nil {
		t.Fatal("publishStagedSocket() accepted a listener that cannot own a path")
	}
	if !strings.Contains(err.Error(), "cannot own its path") {
		t.Errorf("error = %q, want it to say the listener cannot own a path", err)
	}
}

// TestPublishStagedSocket_AChmodThatFails_LeavesNothingPublished verifies that
// a socket whose mode could not be applied never reaches the operator's path.
//
// The order matters more than the error: the mode is applied and confirmed
// BEFORE the link, so a failure here means the path was never created at all,
// rather than created and then repaired.
func TestPublishStagedSocket_AChmodThatFails_LeavesNothingPublished(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	dir := t.TempDir()
	path := filepath.Join(dir, "published.sock")

	original := chmodStagedSocket
	t.Cleanup(func() { chmodStagedSocket = original })
	chmodStagedSocket = func(string, os.FileMode) error {
		return errors.New("the filesystem refused the mode")
	}

	_, err := bindUnixSocket(t.Context(), path, 0o660)
	if err == nil {
		t.Fatal("bindUnixSocket() published a socket whose mode was never applied")
	}
	if _, statErr := os.Lstat(path); !errors.Is(statErr, fs.ErrNotExist) {
		t.Errorf("the path exists after a failed chmod, so a socket with an unknown mode was published")
	}
}

// TestPublishStagedSocket_AModeThatDidNotApply_IsRefused verifies the check
// that exists for a filesystem which accepts a chmod and does not honor it.
//
// Reporting success while leaving the socket more open than the operator asked
// for is the failure this whole file is built to avoid, so the result is
// confirmed rather than assumed.
func TestPublishStagedSocket_AModeThatDidNotApply_IsRefused(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	dir := socketDir(t)
	path := filepath.Join(dir, "wrong-mode.sock")

	originalChmod := chmodStagedSocket
	originalLstat := lstatStagedSocket
	t.Cleanup(func() {
		chmodStagedSocket = originalChmod
		lstatStagedSocket = originalLstat
	})
	// A filesystem that takes the call and does nothing.
	chmodStagedSocket = func(string, os.FileMode) error { return nil }
	lstatStagedSocket = func(name string) (os.FileInfo, error) {
		info, err := os.Lstat(name)
		if err != nil {
			return nil, err
		}
		return wrongModeInfo{FileInfo: info}, nil
	}

	_, err := bindUnixSocket(t.Context(), path, 0o660)
	if err == nil {
		t.Fatal("bindUnixSocket() served on a socket whose mode it could not confirm")
	}
	if !strings.Contains(err.Error(), "refusing to serve on it") {
		t.Errorf("error = %q, want it to say it is refusing to serve", err)
	}
}

// wrongModeInfo reports a mode other than the one on disk, standing in for a
// filesystem that ignores a chmod.
type wrongModeInfo struct {
	os.FileInfo
}

func (w wrongModeInfo) Mode() os.FileMode {
	return (w.FileInfo.Mode() &^ os.ModePerm) | 0o777
}

// TestPublishStagedSocket_AStatThatFails_IsRefused verifies that a mode which
// cannot be read back is treated as a mode that was not applied.
func TestPublishStagedSocket_AStatThatFails_IsRefused(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	dir := t.TempDir()
	path := filepath.Join(dir, "unstattable.sock")

	original := lstatStagedSocket
	t.Cleanup(func() { lstatStagedSocket = original })
	lstatStagedSocket = func(string) (os.FileInfo, error) {
		return nil, errors.New("the filesystem would not answer")
	}

	if _, err := bindUnixSocket(t.Context(), path, 0o660); err == nil {
		t.Fatal("bindUnixSocket() published a socket whose mode it never read back")
	}
}

// TestNewStagingDir_NamesAlreadyTaken_AreRetriedAndThenReported verifies the
// reservation loop.
//
// mkdir is the reservation: it fails rather than reusing a name somebody else
// holds, which is what stops two concurrent binds sharing a directory. A
// collision must therefore be retried rather than treated as fatal, and a
// parent that collides every time must be reported rather than looped on.
func TestNewStagingDir_NamesAlreadyTaken_AreRetriedAndThenReported(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	original := mkdirStagingDir
	t.Cleanup(func() { mkdirStagingDir = original })
	attempts := 0
	mkdirStagingDir = func(string, os.FileMode) error {
		attempts++
		return fs.ErrExist
	}

	_, err := newStagingDir(t.TempDir())
	if err == nil {
		t.Fatal("newStagingDir() reported success without ever creating a directory")
	}
	if attempts < 2 {
		t.Errorf("mkdir was called %d time(s); a name collision must be retried, not fatal", attempts)
	}
}

// TestNewStagingDir_AnUnnameableDirectory_IsReported covers the one failure
// that happens before the filesystem is touched at all.
func TestNewStagingDir_AnUnnameableDirectory_IsReported(t *testing.T) {
	// Deliberately not parallel: see the note on the binding tests.
	original := readRandomName
	t.Cleanup(func() { readRandomName = original })
	readRandomName = func([]byte) (int, error) {
		return 0, errors.New("no entropy")
	}

	if _, err := newStagingDir(t.TempDir()); err == nil {
		t.Fatal("newStagingDir() named a directory without any randomness")
	}
}
