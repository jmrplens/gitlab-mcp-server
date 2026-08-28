// listen.go binds the HTTP-mode listener, which is not always a TCP port.
//
// A deployment behind a reverse proxy on the same machine has two ways to
// make the hop between them unreadable. It can encrypt it — which is what
// --tls-cert/--tls-key are for, and what a proxy on a different machine
// needs. Or it can remove it: a unix socket has no network segment to read
// in the first place, no bridge, no docker-proxy hop, and no certificate to
// issue or rotate. The socket is the cheaper answer where it applies, so
// --http-addr accepts a filesystem path as well as host:port.
package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// loadTLSKeyPair reads a certificate and its key from disk. It is a variable
// so a test can drive the failure branch without a filesystem.
var loadTLSKeyPair = tls.LoadX509KeyPair //nolint:gochecknoglobals // test seam

// staleSocketDialTimeout bounds the probe that tells a socket left behind by
// a crashed process apart from one a live process is serving. It is a
// connect() to a local path: it either completes immediately or the path is
// dead.
const staleSocketDialTimeout = 200 * time.Millisecond

// isUnixSocketAddr reports whether a listen address names a filesystem path
// rather than a TCP address.
//
// A TCP listen address is host:port, and a host is a name or an IP — neither
// contains a path separator. Anything with one is a path, which also covers
// the relative "./mcp.sock" form. A bare "mcp.sock" is deliberately NOT a
// socket: it is indistinguishable from a hostname, and guessing wrong there
// would silently bind something other than what the operator meant.
func isUnixSocketAddr(addr string) bool {
	return strings.Contains(addr, string(os.PathSeparator)) || strings.Contains(addr, "/")
}

// listenHTTP binds the address the server serves on: a unix socket when the
// address is a path, a TCP port otherwise.
func listenHTTP(addr string, socketMode os.FileMode) (net.Listener, error) {
	if !isUnixSocketAddr(addr) {
		return net.Listen("tcp", addr)
	}
	return listenUnix(addr, socketMode)
}

// listenUnix binds a unix socket, clearing a stale one first and applying the
// permission mode the deployment asked for.
//
// The mode is applied after bind rather than through umask: umask can only
// remove bits, it is process-global, and it would make the resulting
// permissions depend on how the service manager happened to be configured.
func listenUnix(path string, socketMode os.FileMode) (net.Listener, error) {
	if err := clearStaleSocket(path); err != nil {
		return nil, err
	}
	if dir := filepath.Dir(path); dir != "" {
		if _, err := os.Stat(dir); err != nil {
			return nil, fmt.Errorf("--http-addr %q: its directory is not usable: %w", path, err)
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listening on unix socket %q: %w", path, err)
	}
	if socketMode == 0 {
		socketMode = config.DefaultSocketMode
	}
	// A socket bound but left at the umask's mercy is the failure mode this
	// whole path exists to avoid: either the proxy cannot reach it, or
	// everyone can. Failing loudly beats serving on a socket whose
	// permissions nobody checked.
	if err := os.Chmod(path, socketMode); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("setting mode %#o on unix socket %q: %w", socketMode, path, err)
	}
	slog.Info("listening on unix socket", "path", path, "mode", fmt.Sprintf("%#o", socketMode))
	return listener, nil
}

// clearStaleSocket removes a socket file left behind by a process that did
// not shut down cleanly, and refuses every other case.
//
// The distinction that matters is between a dead socket and a live one: bind
// on an existing path always fails, so an unconditional remove would let a
// second instance silently steal a socket the first is still serving. A
// successful connect proves someone is there.
func clearStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("--http-addr %q: %w", path, err)
	}
	if info.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("--http-addr %q exists and is not a socket; refusing to replace it", path)
	}
	conn, dialErr := net.DialTimeout("unix", path, staleSocketDialTimeout)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("--http-addr %q is already served by another process", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("removing stale socket %q: %w", path, err)
	}
	slog.Warn("removed a stale unix socket left by an earlier run", "path", path)
	return nil
}

// parseSocketMode resolves --http-socket-mode into permission bits.
//
// The value is read as octal whether or not it carries a leading 0, because
// "0660" and "660" mean the same thing to every operator who has ever used
// chmod, and reading the second as decimal would silently produce 0o1224.
func parseSocketMode(hcfg *httpConfig) error {
	if hcfg.socketMode == "" {
		hcfg.socketModeParsed = config.DefaultSocketMode
		return nil
	}
	mode, err := strconv.ParseUint(strings.TrimPrefix(hcfg.socketMode, "0o"), 8, 32)
	if err != nil {
		return fmt.Errorf("invalid --http-socket-mode %q: expected an octal mode such as 0660", hcfg.socketMode)
	}
	if mode == 0 || mode > 0o777 {
		return fmt.Errorf("invalid --http-socket-mode %q: expected a permission mode between 0001 and 0777", hcfg.socketMode)
	}
	hcfg.socketModeParsed = os.FileMode(mode)
	return nil
}

// validateTLSFiles checks the certificate pair before the server starts.
//
// Both files or neither: a cert without its key is a deployment that thinks
// it is encrypting and is not. Loading the pair here turns a typo into a
// startup error naming the file, instead of a TLS handshake failure on the
// first request that nobody sees until a client reports it.
func validateTLSFiles(cfg *config.Config) error {
	switch {
	case cfg.TLSCertFile == "" && cfg.TLSKeyFile == "":
		return nil
	case cfg.TLSCertFile == "":
		return errors.New("--tls-key requires --tls-cert")
	case cfg.TLSKeyFile == "":
		return errors.New("--tls-cert requires --tls-key")
	}
	if _, err := loadTLSKeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
		return fmt.Errorf("loading the TLS certificate and key: %w", err)
	}
	return nil
}
