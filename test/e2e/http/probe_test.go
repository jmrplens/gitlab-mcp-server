//go:build httpe2e

// probe_test.go drives --probe, the image's HEALTHCHECK, against the real
// binary on each listener a deployment can choose.
//
// Unit tests cover the decision logic with injected processes. What they
// cannot show is that the flag is wired, that the probe's own process list
// finds a server started the way an operator starts one, and that a probe
// of a TLS or unix-socket listener really completes against this server.
package httpe2e

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// runProbe runs the binary with --probe and the given arguments (flags first,
// then the optional target, since flag parsing stops at the first positional),
// returning the exit code and what it said.
func runProbe(t *testing.T, probeArgs ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	args := append([]string{"--probe"}, probeArgs...)
	out, err := exec.CommandContext(ctx, serverBinary(t), args...).CombinedOutput() //nolint:gosec // the binary and arguments are the test's own
	if err == nil {
		return 0, string(out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running --probe: %v: %s", err, out)
	}
	return exitErr.ExitCode(), string(out)
}

// TestProbe_ExplicitTarget covers a target named on the command line: the
// server's own listener answers, a port nobody listens on does not, and the
// server answering something other than 200 is not healthy either.
func TestProbe_ExplicitTarget(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	t.Run("the listener answers", func(t *testing.T) {
		code, said := runProbe(t, srv.baseURL)
		if code != 0 {
			t.Fatalf("--probe %s exited %d, want 0: %s", srv.baseURL, code, said)
		}
	})
	t.Run("host:port is enough", func(t *testing.T) {
		code, said := runProbe(t, strings.TrimPrefix(srv.baseURL, "http://"))
		if code != 0 {
			t.Fatalf("--probe host:port exited %d, want 0: %s", code, said)
		}
	})
	t.Run("nothing listening", func(t *testing.T) {
		target := "http://127.0.0.1:" + strconv.Itoa(freePort(t))
		code, said := runProbe(t, target)
		if code != 1 {
			t.Fatalf("--probe %s exited %d, want 1: %s", target, code, said)
		}
	})
	t.Run("a path the server does not serve", func(t *testing.T) {
		code, said := runProbe(t, srv.baseURL+"/nope")
		if code != 1 {
			t.Fatalf("--probe of an unserved path exited %d, want 1: %s", code, said)
		}
		if !strings.Contains(said, "404") {
			t.Errorf("the refusal should carry the status: %s", said)
		}
	})
	t.Run("a target that is not one", func(t *testing.T) {
		code, _ := runProbe(t, "nope")
		if code != 2 {
			t.Fatalf("--probe nope exited %d, want 2", code)
		}
	})
}

// TestProbe_Discovery verifies the bare --probe, the form the image's
// HEALTHCHECK runs, finds a server started with --http on a chosen port
// without being told where.
//
// The address is what the assertion is about: the probe must report an HTTP
// listener it reached and answered on. A verdict alone would also be satisfied
// by a stdio instance, which is reported healthy without anything being
// probed, and the test would then pass without discovery reaching a listener
// at all.
//
// It cannot demand this test's own listener. Discovery answers on the first
// instance it meets, lowest pid first, and a sibling test's server is another
// instance of the same binary; measured, one of them answered here while this
// server was up. In the container the check is written for there is one
// instance, so the address is this one's.
func TestProbe_Discovery(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	srv := startServer(t, nil, "--gitlab-url="+gitlab.url)

	code, said := runProbe(t)
	if code != 0 {
		t.Fatalf("--probe exited %d with a server running on %s, want 0: %s", code, srv.baseURL, said)
	}
	if !strings.Contains(said, "answered") || !strings.Contains(said, "http://127.0.0.1:") {
		t.Errorf("--probe said %q, want it to name the HTTP listener it reached (this test's is %s)", said, srv.baseURL)
	}
}

// TestProbe_TLS verifies a listener that terminates TLS itself is probed over
// TLS, which the old wget of http://localhost:8080/health could never do.
func TestProbe_TLS(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	certFile, keyFile, pool := writeSelfSignedCert(t)
	port := freePort(t)
	addr := "127.0.0.1:" + strconv.Itoa(port)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}},
	}
	startServerWithClient(t, client, "https://"+addr,
		"--http-addr="+addr,
		"--gitlab-url="+gitlab.url,
		"--tls-cert="+certFile,
		"--tls-key="+keyFile,
	)

	// Pinned to the certificate the server was started with, which is how
	// the image's check reaches a self-signed listener.
	code, said := runProbe(t, "--tls-cert="+certFile, "https://"+addr)
	if code != 0 {
		t.Fatalf("--probe --tls-cert=... https://%s exited %d, want 0: %s", addr, code, said)
	}
	// Without the pin the standard verification applies, which a self-signed
	// certificate fails: the probe must not trust whatever answers.
	code, said = runProbe(t, "https://"+addr)
	if code != 1 {
		t.Errorf("--probe https://%s without the certificate exited %d, want 1: %s", addr, code, said)
	}
	// The plaintext spelling must not pass either: the listener speaks TLS.
	code, said = runProbe(t, "http://"+addr)
	if code != 1 {
		t.Errorf("--probe http://%s against a TLS listener exited %d, want 1: %s", addr, code, said)
	}
}

// TestProbe_UnixSocket verifies a socket listener is probed through the
// socket, in both spellings the flag documents.
func TestProbe_UnixSocket(t *testing.T) {
	gitlab := startFakeGitLab(t, http.StatusUnauthorized, "")
	// Not t.TempDir: a socket path is limited to about a hundred bytes and
	// the test name lands in that one.
	dir, err := os.MkdirTemp("", "probe") //nolint:usetesting // short path, see above
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socketPath := filepath.Join(dir, "mcp.sock")
	startServerOnUnixSocket(t, socketPath, "--gitlab-url="+gitlab.url)

	for _, target := range []string{socketPath, "unix:" + socketPath} {
		t.Run(target, func(t *testing.T) {
			code, said := runProbe(t, target)
			if code != 0 {
				t.Fatalf("--probe %s exited %d, want 0: %s", target, code, said)
			}
		})
	}
}
