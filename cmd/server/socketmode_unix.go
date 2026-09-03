//go:build !windows

// socketmode_unix.go creates the unix socket with its permission mode already
// set, and publishes it at the operator's path only once that mode holds.
//
// The obvious implementation (bind, then os.Chmod(path, mode)) has a window
// between the two calls. On a directory an untrusted local account can write
// (/tmp being the obvious one), that account can replace the socket with a
// symlink in the gap and the chmod then lands on whatever the link points at:
// CWE-367, time-of-check to time-of-use.
//
// Two fixes look plausible and only one works. Applying the mode to the bound
// descriptor with fchmod(2) would close the window by removing the name from
// the operation, but on Linux fchmod on a socket descriptor RETURNS SUCCESS
// AND CHANGES NOTHING. Measured: asking for 0660 leaves 0755. A silent no-op
// is worse than the race it was meant to fix, because it reports a restriction
// that was never applied.
//
// The mode used to be set through the process umask around the bind instead.
// That produced a socket with the right bits from the start and had no second
// operation to race, but umask is process-global: for the length of the bind,
// every other goroutine in the process created its files under it too. A
// mutex can only serialize binds against each other, not against the rest of
// the program, and CI proved the difference by failing unrelated tests whose
// t.TempDir() came back without its execute bit.
//
// So the socket is now built somewhere nobody else can reach and published
// afterwards:
//
//  1. A staging directory is created beside the target path, mode 0700 set
//     explicitly through its descriptor. No other account can traverse it, so
//     nothing inside it can be swapped for a symlink.
//  2. The socket is bound inside it and chmod'ed there. The chmod is by path,
//     but the path is unreachable to anyone else, so there is no attacker to
//     win the race that made a chmod unsafe in the first place.
//  3. link(2) publishes it at the target path. link fails with EEXIST when
//     the target already exists, which is the no-clobber behavior bind gives
//     today, portably and without renameat2/RENAME_NOREPLACE. The staging
//     names are then removed; the socket survives on the listener's descriptor
//     and on the published link.
//
// Because the listener was bound under the staging name, its own
// unlink-on-close would remove that name rather than the published one. It is
// switched off and the published path is removed on Close instead, and only
// when it is still the same inode this process created.
package main

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// stagingDirPrefix names the staging directory bindUnixSocket creates beside
// the target path. It is short on purpose: a unix address is capped near 100
// bytes by the kernel, and the staged path is one directory deeper than the
// published one.
const stagingDirPrefix = ".s"

// stagingDirRandomBytes is how much randomness the staging directory's name
// carries, hex-encoded. Enough that a local account cannot pre-create the name
// this process is about to use and stall its startup, and short enough that
// the whole name is a fixed 10 bytes of the address budget. os.MkdirTemp is
// not used for exactly that reason: its suffix is a decimal number whose
// length varies with the value it drew.
const stagingDirRandomBytes = 4

// stagingDirAttempts bounds the retry when a staging name is already taken.
// Two collisions in a row on 32 random bits means something other than chance
// is producing them.
const stagingDirAttempts = 8

// stagedSocketName is the socket's name inside the staging directory, one
// character for the same reason.
const stagedSocketName = "s"

// stagingDirMode is the staging directory's permission mode: the owner, and
// nobody else. It is the whole reason the chmod inside it is safe.
const stagingDirMode os.FileMode = 0o700

// maxUnixPathLen is the longest path a unix address can carry: sun_path is 108
// bytes on Linux and 104 on the BSDs, one of which is the NUL terminator.
//
// Binding used to enforce this for free, because the kernel refused the
// address. It no longer does: the socket is bound under a short staging name
// and published with link(2), which has no such limit, so an over-long path
// would produce a listener that binds cleanly and that no client can ever
// connect to. The check has to be made here instead.
const maxUnixPathLen = len(syscall.RawSockaddrUnix{}.Path) - 1

// bindUnixSocket listens on path with the socket created under mode.
//
// The listener it returns publishes path as its address and removes path when
// closed, so callers see a listener on the address they asked for and never
// learn that it was assembled elsewhere.
func bindUnixSocket(ctx context.Context, path string, mode os.FileMode) (net.Listener, error) {
	if err := checkUnixPathLen(path, "the socket path"); err != nil {
		return nil, err
	}

	staging, err := newStagingDir(filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("preparing to bind unix socket %q: %w", path, err)
	}
	// Runs on every exit: on success it removes the staging name and the
	// directory, leaving the socket alive on the published link and the
	// listener's descriptor; on failure it removes a socket that was never
	// published.
	defer func() { _ = os.RemoveAll(staging) }()

	// The staging directory is one level deeper than the published path, so a
	// path that fits can still be staged into one that does not. Saying which
	// of the two ran out of room is the difference between an operator who
	// moves the socket and one who reads a kernel EINVAL about a temporary
	// directory they never created.
	staged := filepath.Join(staging, stagedSocketName)
	if lenErr := checkUnixPathLen(staged, "the staging path the socket is built under"); lenErr != nil {
		return nil, lenErr
	}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", staged)
	if err != nil {
		return nil, fmt.Errorf(
			"listening on unix socket %q (bound as %q first, see socketmode_unix.go): %w", path, staged, err,
		)
	}

	published, err := publishStagedSocket(listener, staged, path, mode)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	if probeErr := probeUnixSocket(ctx, path); probeErr != nil {
		// Closing the wrapper also removes the path it just published, so a
		// socket that failed its own probe leaves nothing behind.
		_ = published.Close()
		return nil, probeErr
	}
	return published, nil
}

// probeUnixSocket confirms that the published path reaches the listener. It is
// a variable so a test can drive the failure branch, which no filesystem this
// runs on is known to produce.
var probeUnixSocket = confirmReachable //nolint:gochecknoglobals // test seam

// The system calls this file makes on the staging path, as variables.
//
// Every one of them guards a property rather than merely reporting an error: a
// chmod that did not apply, a stat that cannot confirm what it applied to, a
// name reservation that did not reserve. None can be made to fail from a test
// by ordinary means, and leaving them uncovered would mean the checks that
// exist to catch a hostile or broken filesystem are themselves never exercised.
// The package already covers this class of branch the same way, through
// loadTLSKeyPair and buildServerCardFn.
var (
	chmodStagedSocket = os.Chmod        //nolint:gochecknoglobals // test seam
	lstatStagedSocket = os.Lstat        //nolint:gochecknoglobals // test seam
	mkdirStagingDir   = os.Mkdir        //nolint:gochecknoglobals // test seam
	readRandomName    = cryptorand.Read //nolint:gochecknoglobals // test seam
)

// confirmReachable connects to path and hangs up.
//
// Publishing with link(2) means the address a client dials is no longer the
// address the kernel was given at bind time, and one link resolving to a
// different in-kernel object than the other is not hypothetical: it is exactly
// how AF_UNIX sockets behaved over FUSE, where the link succeeded and every
// connect through it was refused.
//
// Apple's published sources settle the question for the syscall layer, which
// rejects only directories, and for HFS+, which routes sockets through the same
// hard-link machinery as regular files. APFS is closed source, so for the
// filesystem every current Mac actually uses the answer rests on inference. One
// connect() at startup is cheaper than that inference being wrong, and it also
// covers every other way a socket can end up published but unreachable.
//
// The probe's connection is left in the accept queue rather than accepted here:
// accepting would race a real client for the first connection, and the server
// that takes over the listener reads EOF from it and drops it.
func confirmReachable(ctx context.Context, path string) error {
	conn, err := (&net.Dialer{Timeout: staleSocketDialTimeout}).DialContext(ctx, "unix", path)
	if err != nil {
		return fmt.Errorf(
			"the unix socket published at %q cannot be connected to, so no client could reach it either: %w", path, err,
		)
	}
	return conn.Close()
}

// publishStagedSocket applies mode to the socket bound at staged and links it
// to path, returning a listener that owns path.
//
// The mode is applied and verified BEFORE the link, so a socket whose
// permissions could not be confirmed is never reachable at the operator's
// path at all, not even briefly.
func publishStagedSocket(listener net.Listener, staged, path string, mode os.FileMode) (net.Listener, error) {
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		return nil, fmt.Errorf("listening on unix socket %q: got a %T, which cannot own its path", path, listener)
	}
	// The listener would otherwise unlink the staging name on Close, which
	// this function is about to remove anyway, and never the published one.
	unixListener.SetUnlinkOnClose(false)

	if err := chmodStagedSocket(staged, mode.Perm()); err != nil {
		return nil, fmt.Errorf("setting the mode of unix socket %q: %w", path, err)
	}
	// Verified rather than assumed: a filesystem that ignores the chmod, or
	// honors only part of it, would leave the socket more open than the
	// operator asked for. Serving on a socket whose permissions nobody checked
	// is the failure this whole path exists to avoid.
	info, err := lstatStagedSocket(staged)
	if err != nil {
		return nil, fmt.Errorf("checking the mode of unix socket %q: %w", path, err)
	}
	if got := info.Mode().Perm(); got != mode.Perm() {
		return nil, fmt.Errorf(
			"unix socket %q was created with mode %#o, not the requested %#o; refusing to serve on it",
			path, got, mode.Perm(),
		)
	}

	// link(2), not rename(2): rename would replace whatever is at path,
	// while link refuses with EEXIST. That refusal is the guarantee bind
	// gives today, and it is what stops a second instance from silently
	// taking over a socket the first is still serving.
	if linkErr := os.Link(staged, path); linkErr != nil {
		return nil, fmt.Errorf("publishing unix socket at %q: %w", path, linkErr)
	}
	return &pathOwningListener{UnixListener: unixListener, path: path, bound: info}, nil
}

// checkUnixPathLen refuses a path no unix address can hold. what names which
// path it is, since one of the two is not the operator's.
func checkUnixPathLen(path, what string) error {
	if len(path) <= maxUnixPathLen {
		return nil
	}
	return fmt.Errorf(
		"unix socket %q: %s is %d bytes and a unix address holds at most %d; no client could reach it, so use a shorter --http-addr",
		path, what, len(path), maxUnixPathLen,
	)
}

// newStagingDir creates the private directory a socket is assembled in.
//
// parent is the target path's own directory because link(2) cannot cross
// filesystems, so the staging area has to live on the same one.
func newStagingDir(parent string) (string, error) {
	var lastErr error
	for range stagingDirAttempts {
		raw := make([]byte, stagingDirRandomBytes)
		if _, err := readRandomName(raw); err != nil {
			return "", fmt.Errorf("naming a staging directory: %w", err)
		}
		dir := filepath.Join(parent, stagingDirPrefix+hex.EncodeToString(raw))

		// mkdir is the reservation: it fails rather than reusing a name
		// somebody else already holds, so no two binds can share a directory.
		err := mkdirStagingDir(dir, stagingDirMode)
		if errors.Is(err, fs.ErrExist) {
			lastErr = err
			continue
		}
		if err != nil {
			return "", fmt.Errorf("creating a staging directory: %w", err)
		}
		if restrictErr := restrictDirToOwner(dir); restrictErr != nil {
			_ = os.RemoveAll(dir)
			return "", restrictErr
		}
		return dir, nil
	}
	return "", fmt.Errorf("creating a staging directory in %q: %w", parent, lastErr)
}

// restrictDirToOwner sets dir to [stagingDirMode] through its own descriptor
// and confirms the result.
//
// mkdir(2) subtracts the process umask from the mode it is given, so an
// unusual umask can hand back a directory this process cannot itself traverse. Repairing that by path would reintroduce exactly the race this
// file exists to avoid, so the mode is applied with fchmod(2) on a descriptor
// opened O_NOFOLLOW|O_DIRECTORY: if anything replaced the directory between
// the mkdir and the open, the open fails instead of the mode landing
// somewhere else.
func restrictDirToOwner(dir string) error {
	file, err := os.OpenFile(dir, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("opening the staging directory %q: %w", dir, err)
	}
	defer func() { _ = file.Close() }()

	if chmodErr := file.Chmod(stagingDirMode); chmodErr != nil {
		return fmt.Errorf("restricting the staging directory %q: %w", dir, chmodErr)
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("checking the staging directory %q: %w", dir, err)
	}
	if !info.IsDir() || info.Mode().Perm() != stagingDirMode {
		return fmt.Errorf(
			"the staging directory %q is %v, not a directory with mode %#o; refusing to build a socket in it",
			dir, info.Mode(), stagingDirMode,
		)
	}
	return nil
}

// pathOwningListener is a unix listener that reports and cleans up the path
// its socket was PUBLISHED at, rather than the staging name it was bound
// under.
type pathOwningListener struct {
	*net.UnixListener
	path      string
	bound     os.FileInfo
	unlinkOne sync.Once
}

// Addr reports the published path. The embedded listener would name the
// staging directory, which no longer exists and which no client was ever told
// about.
func (l *pathOwningListener) Addr() net.Addr {
	return &net.UnixAddr{Name: l.path, Net: "unix"}
}

// Close removes the published path and then closes the socket, the order the
// standard library uses for its own auto-unlink.
//
// The path is removed only while it still names the inode this process bound.
// net.UnixListener cannot make that check and documents the resulting race:
// after a slow shutdown, a successor process may already have bound its own
// socket at the same path, and an unconditional remove would delete it.
func (l *pathOwningListener) Close() error {
	l.unlinkOne.Do(func() {
		if info, err := os.Lstat(l.path); err == nil && os.SameFile(info, l.bound) {
			_ = os.Remove(l.path)
		}
	})
	return l.UnixListener.Close()
}
