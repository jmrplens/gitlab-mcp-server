// listen.go binds the HTTP-mode listener, which is not always a TCP port.
//
// A deployment behind a reverse proxy on the same machine has two ways to
// make the hop between them unreadable. It can encrypt it — which is what
// --tls-cert/--tls-key are for, and what a proxy on a different machine
// needs. Or it can remove it: a unix socket has no network segment to read
// in the first place, no bridge, no docker-proxy hop, and no certificate to
// issue or rotate. The socket is the cheaper answer where it applies, so
// --http-addr accepts a filesystem path as well as host:port.
//
// A listener that does neither, a TCP port another machine can reach in plain
// HTTP, is still served by default, and startup warns about it unless the
// operator declared that something in front of it encrypts the hop; an
// operator who declares the opposite gets the hardened mode, which refuses to
// start it (checkPlaintextListener).

package main

import (
	"context"
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

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
)

// loadTLSKeyPair reads a certificate and its key from disk. It is a variable
// so a test can drive the failure branch without a filesystem.
var loadTLSKeyPair = tls.LoadX509KeyPair

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
//
// One test over both separators rather than one per separator: on every
// platform but Windows the native separator is the slash, so a second test for
// the slash could only be reached when the first had already said no, and
// would be a condition that never decides anything there.
func isUnixSocketAddr(addr string) bool {
	return strings.ContainsAny(addr, "/"+string(os.PathSeparator))
}

// listenHTTP binds the address the server serves on: a unix socket when the
// address is a path, a TCP port otherwise.
func listenHTTP(ctx context.Context, addr string, socketMode os.FileMode) (net.Listener, error) {
	if !isUnixSocketAddr(addr) {
		listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", addr)
		if err != nil {
			// Named, because the address is the whole content of this
			// failure: bare, the commonest one reads "address already in use"
			// without saying which address, and the operator has to go back to
			// the command line to find out what they asked for.
			return nil, fmt.Errorf("--http-addr %q: binding the TCP listener: %w", addr, err)
		}
		// The address that was bound, not the one that was asked for. They
		// differ whenever the port is 0, which is how a deployment asks the
		// kernel to choose one, and there is no other way to learn which port
		// it chose. The unix branch has logged what it bound since it existed.
		slog.InfoContext(ctx, "listening on tcp", "addr", listener.Addr().String())
		return listener, nil
	}
	return listenUnix(ctx, addr, socketMode)
}

// listenUnix binds a unix socket, clearing a stale one first and creating it
// under the permission mode the deployment asked for.
func listenUnix(ctx context.Context, path string, socketMode os.FileMode) (net.Listener, error) {
	if err := clearStaleSocket(ctx, path); err != nil {
		return nil, err
	}
	// filepath.Dir never answers an empty string (a bare name is in "."), so
	// every path has a directory to check.
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		return nil, fmt.Errorf("--http-addr %q: its directory is not usable: %w", path, err)
	}
	if socketMode == 0 {
		socketMode = config.DefaultSocketMode
	}
	// The socket carries its mode before it reaches this path at all: it is
	// assembled in a private directory and published with link(2). A chmod on
	// the operator's own path would be racing anybody who can write to that
	// directory, since they could swap the socket for a symlink in between and
	// have the mode land on whatever it points at (CWE-367). See
	// socket_mode_unix.go for the alternatives that do not work, the umask this
	// replaced among them.
	listener, err := bindUnixSocket(ctx, path, socketMode)
	if err != nil {
		return nil, err
	}
	slog.InfoContext(ctx, "listening on unix socket", "path", path, "mode", fmt.Sprintf("%#o", socketMode))
	return listener, nil
}

// clearStaleSocket removes a socket file left behind by a process that did
// not shut down cleanly, and refuses every other case.
//
// The distinction that matters is between a dead socket and a live one: bind
// on an existing path always fails, so an unconditional remove would let a
// second instance silently steal a socket the first is still serving. A
// successful connect proves someone is there.
func clearStaleSocket(ctx context.Context, path string) error {
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
	conn, dialErr := (&net.Dialer{Timeout: staleSocketDialTimeout}).DialContext(ctx, "unix", path)
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("--http-addr %q is already served by another process", path)
	}
	// Only ONE failure proves the socket is dead: the kernel answering that
	// nothing is listening. Anything else, a permission denial, a timeout, a
	// cancelled context, means the question went unanswered, and deleting on
	// an unanswered question is how a live server loses its socket to a
	// racing restart. Refusing costs an operator one manual `rm`; guessing
	// costs a running deployment its listener.
	//
	// Which code carries that answer is a per-platform question, which is why
	// it is asked through [isConnRefused] rather than compared here. Windows
	// answers with a Winsock code that is not [syscall.ECONNREFUSED], so this
	// used to treat every stale socket there as unprobed: a deployment whose
	// previous run exited uncleanly refused to start again until somebody
	// deleted the file by hand, forever, while the same situation self-healed
	// on Linux.
	if !isConnRefused(dialErr) {
		return fmt.Errorf(
			"--http-addr %q exists and could not be probed; refusing to replace it. Remove it by hand if you are sure no server is running: %w",
			path, dialErr,
		)
	}
	if removeErr := removeStaleSocket(path); removeErr != nil {
		return fmt.Errorf("removing stale socket %q: %w", path, removeErr)
	}
	slog.WarnContext(ctx, "removed a stale unix socket left by an earlier run", "path", path)
	return nil
}

// removeStaleSocket unlinks a socket the probe above proved dead. A variable
// because the tests run with every permission, so nothing on disk can make
// the unlink fail, and the branch reporting that failure is otherwise never
// run.
var removeStaleSocket = os.Remove

// repeatedFlag is a flag that may be given more than once, and whose single
// occurrence may itself be a comma-separated list.
//
// Both spellings are accepted because both reach the same flag by different
// routes: an operator writing a systemd unit repeats the flag, while the
// environment overlay has one string to give it. Order is preserved and
// meaningful — the first entry is the deployment's default instance.
type repeatedFlag []string

func (r *repeatedFlag) String() string {
	if r == nil {
		return ""
	}
	return strings.Join(*r, ",")
}

func (r *repeatedFlag) Set(value string) error {
	r.add(value)
	return nil
}

// add appends every non-blank comma-separated entry of value. It is what Set
// does, without the error flag.Value obliges Set to return and this parser
// never has: the environment overlay calls this one, so there is no error
// for it to drop.
func (r *repeatedFlag) add(value string) {
	for part := range strings.SplitSeq(value, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			*r = append(*r, trimmed)
		}
	}
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
//
// Written as a chain of returns rather than a tagless switch, whose case
// expressions carry no statement counter for the mutation gate to see.
func validateTLSFiles(cfg *config.Config) error {
	if cfg.TLSCertFile == "" && cfg.TLSKeyFile == "" {
		return nil
	}
	if cfg.TLSCertFile == "" {
		return errors.New("--tls-key requires --tls-cert")
	}
	if cfg.TLSKeyFile == "" {
		return errors.New("--tls-cert requires --tls-key")
	}
	if _, err := loadTLSKeyPair(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil {
		return fmt.Errorf("loading the TLS certificate and key: %w", err)
	}
	return nil
}

// plaintextHTTP is what the operator said about a TCP listener off loopback
// that serves plain HTTP, through --allow-plaintext-http or
// GITLAB_MCP_ALLOW_PLAINTEXT_HTTP, the flag winning over the variable. Its
// zero value is the default, which says nothing, so a configuration built
// without the setting serves such a listener as every release before the
// setting did.
type plaintextHTTP int

const (
	// plaintextHTTPUnset is the default: the listener is served, and startup
	// warns that every token crosses the network in the clear.
	plaintextHTTPUnset plaintextHTTP = iota
	// plaintextHTTPAllowed is the setting at true, the operator's statement
	// that something in front of the listener encrypts the hop: it is served,
	// and startup says so at INFO.
	plaintextHTTPAllowed
	// plaintextHTTPRefused is the setting at false, the hardened mode:
	// startup refuses such a listener.
	plaintextHTTPRefused
)

// setAllowed records a value the operator gave the setting.
func (p *plaintextHTTP) setAllowed(allowed bool) {
	*p = plaintextHTTPRefused
	if allowed {
		*p = plaintextHTTPAllowed
	}
}

// String is the setting's value as a boolean, true unless the operator chose
// the hardened mode, which is also what the flag package shows as the
// default.
func (p *plaintextHTTP) String() string {
	return strconv.FormatBool(p == nil || *p != plaintextHTTPRefused)
}

// Set reads the flag's value as the flag package reads a boolean flag's, so a
// value strconv.ParseBool does not read stops startup with the flag named.
func (p *plaintextHTTP) Set(value string) error {
	allowed, err := strconv.ParseBool(value)
	if err != nil {
		return err
	}
	p.setAllowed(allowed)
	return nil
}

// IsBoolFlag makes --allow-plaintext-http with no value mean true, as it does
// for every boolean flag.
func (*plaintextHTTP) IsBoolFlag() bool { return true }

// The two lines [checkPlaintextListener] writes. Each is one literal, so a
// log pipeline can match it whole, and test/e2e/http reads both off the real
// binary's output, spelled out there because it cannot import this package.
const (
	// plaintextListenerWarning is written at WARN, beside the address, when a
	// TCP listener another machine can reach serves plain HTTP and the
	// operator has said nothing about it.
	plaintextListenerWarning = "--http-addr serves plain HTTP on an address other machines can reach, so every request's GitLab token and every answer cross the network in the clear: serve TLS on the listener with --tls-cert and --tls-key, bind a loopback address or a unix socket for a proxy on this machine, or, where something in front of the listener already encrypts the hop, say so with --allow-plaintext-http or GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true. --allow-plaintext-http=false or GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false is the hardened mode, which refuses to start such a listener"
	// plaintextListenerAllowed is written at INFO in its place when the
	// operator set the opt-in, so the log of a deployment that declared the
	// hop still says what its listener serves.
	plaintextListenerAllowed = "--http-addr serves plain HTTP on an address other machines can reach, as --allow-plaintext-http declares: whatever is in front of the listener is trusted to encrypt the hop"
)

// checkPlaintextListener decides at startup what becomes of a listener that
// serves plain HTTP to the network. It is the one listener configuration of
// this server that puts a caller's token on a network in the clear, since
// every request carries one (an http:// instance does the same on the
// outbound side).
//
// It says nothing about the three listeners that carry nothing across a
// network in the clear: TLS on the listener (tlsCert names the pair, which
// runHTTP has loaded by now), and a loopback address or a unix socket, judged
// by [listenerIsHostLocal], the test --allow-any-gitlab-url is admitted on.
// Everything else is reachable from another machine in plain HTTP: a wildcard
// bind, which is what the default --http-addr of :8080 and the container
// image's command bind, a routable address, and a name that resolves to
// anything but loopback.
//
// Such a listener is served unless the operator chose the hardened mode,
// because most deployments of this shape are sound: a reverse proxy
// terminates TLS and reaches the listener over a private network or a
// container bridge, and nothing inside the process can tell that hop from the
// open network. A container published on loopback only (-p
// 127.0.0.1:8080:8080) is one of them: on a bridge network it binds every
// interface inside the container. Refusing it by default would stop every
// such deployment at the upgrade that brought the refusal, which
// docs/development/plaintext-http-refusal-analysis.md weighs and the
// maintainer decided against on 27 October 2026.
//
// What it does with one follows the setting. Unset, it is served and startup
// warns, naming the remedies and the hardened mode. Set to true, the
// operator's statement that something in front of the listener encrypts the
// hop, it is served and startup says so at INFO rather than going silent, so
// a declared plaintext listener stays on the record. Set to false, startup
// is refused (register row ADM-015) with an error naming the address and the
// ways to serve it.
func checkPlaintextListener(ctx context.Context, addr, tlsCert string, setting plaintextHTTP) error {
	if tlsCert != "" || listenerIsHostLocal(addr) {
		return nil
	}
	switch setting {
	case plaintextHTTPRefused:
		return fmt.Errorf(
			"--allow-plaintext-http=false, or GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=false, refuses a listener that serves plain HTTP "+
				"on an address other machines can reach, and --http-addr %q is one: every request's GitLab token and every "+
				"answer would cross the network in the clear. Serve TLS on the listener with --tls-cert and --tls-key, or bind "+
				"a loopback address or a unix socket for a proxy on this machine; where something in front of the listener "+
				"encrypts the hop, set --allow-plaintext-http or GITLAB_MCP_ALLOW_PLAINTEXT_HTTP=true instead",
			addr,
		)
	case plaintextHTTPAllowed:
		slog.InfoContext(ctx, plaintextListenerAllowed, "addr", addr)
	default:
		slog.WarnContext(ctx, plaintextListenerWarning, "addr", addr)
	}
	return nil
}
