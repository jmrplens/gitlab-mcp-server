package main

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// TestValidatePprofAddr_AcceptsOnlyLoopback pins the one property the
// profile listener has to have: it is never reachable from the network.
//
// A heap profile is a copy of the process's memory, catalogs and credentials
// included, and the handlers are served without a credential, so the address
// is the whole access control. Every shape that could bind another interface
// is refused with a reason: the empty host that binds all of them, an address
// that is not loopback, and a name that could resolve anywhere.
func TestValidatePprofAddr_AcceptsOnlyLoopback(t *testing.T) {
	cases := []struct {
		name    string
		addr    string
		wantErr string
	}{
		{name: "ipv4 loopback", addr: "127.0.0.1:6060"},
		{name: "ipv4 loopback range", addr: "127.0.0.2:6060"},
		{name: "ipv6 loopback", addr: "[::1]:6060"},
		{name: "localhost by name", addr: "localhost:6060"},
		{name: "an ephemeral port", addr: "127.0.0.1:0"},
		{name: "every interface", addr: ":6060", wantErr: "every interface"},
		{name: "unspecified address", addr: "0.0.0.0:6060", wantErr: "not a loopback address"},
		{name: "a private address", addr: "192.168.1.5:6060", wantErr: "not a loopback address"},
		{name: "a name", addr: "profiles.example:6060", wantErr: "is a name, not an address"},
		{name: "no port", addr: "127.0.0.1", wantErr: "host:port"},
		{name: "nonsense", addr: "not an address at all", wantErr: "host:port"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePprofAddr(tc.addr)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("validatePprofAddr(%q) = %v, want it accepted", tc.addr, err)
			case tc.wantErr != "" && err == nil:
				t.Errorf("validatePprofAddr(%q) accepted an address the network can reach", tc.addr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Errorf("error %q does not say %q", err, tc.wantErr)
			}
		})
	}
}

// getPprof issues one GET against a running listener and returns the status
// and body, closing the response before returning.
func getPprof(t *testing.T, url string) (status int, body []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}
	return resp.StatusCode, body
}

// TestStartPprofListener_ServesProfilesOnLoopbackAndStops walks the whole
// life of a listener: it announces itself once at INFO, answers a heap
// profile in the compressed protobuf shape the pprof tool reads, answers the
// goroutine listing whose first line the benchmark counts, and after stop
// refuses connections.
func TestStartPprofListener_ServesProfilesOnLoopbackAndStops(t *testing.T) {
	logged := captureLogMessages(t)

	l, err := startPprofListener(t.Context(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startPprofListener: %v", err)
	}
	if !logged("pprof listener started") {
		t.Error("the listener did not announce itself at startup")
	}
	if host, _, splitErr := net.SplitHostPort(l.addr); splitErr != nil || host != "127.0.0.1" {
		t.Errorf("listener address %q, want a 127.0.0.1 port", l.addr)
	}

	t.Run("heap profile", func(t *testing.T) {
		status, body := getPprof(t, "http://"+l.addr+"/debug/pprof/heap")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		// A profile is gzip-compressed protobuf; the magic number is what
		// `go tool pprof` looks at first.
		if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
			t.Errorf("the heap profile does not start with the gzip magic number: % x", body[:min(len(body), 4)])
		}
	})

	t.Run("goroutine count", func(t *testing.T) {
		status, body := getPprof(t, "http://"+l.addr+"/debug/pprof/goroutine?debug=1")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}
		first, _ := bufio.NewReader(strings.NewReader(string(body))).ReadString('\n')
		if !strings.HasPrefix(first, "goroutine profile: total ") {
			t.Errorf("first line = %q, want the goroutine total", first)
		}
		total := strings.TrimSpace(strings.TrimPrefix(first, "goroutine profile: total "))
		if count, convErr := strconv.Atoi(total); convErr != nil || count < 1 {
			t.Errorf("total %q is not a positive count", total)
		}
	})

	t.Run("index", func(t *testing.T) {
		status, body := getPprof(t, "http://"+l.addr+"/debug/pprof/")
		if status != http.StatusOK || !strings.Contains(string(body), "heap") {
			t.Errorf("index answered %d with %d bytes, want the profile listing", status, len(body))
		}
	})

	l.stop()
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+l.addr+"/debug/pprof/heap", http.NoBody)
	if reqErr != nil {
		t.Fatalf("build request: %v", reqErr)
	}
	if resp, getErr := http.DefaultClient.Do(req); getErr == nil {
		_ = resp.Body.Close()
		t.Error("the listener still answered after stop")
	}
}

// TestStartPprofListener_NothingAskedFor_StartsNothing verifies the empty
// default serves nothing and hands back a listener whose stop is safe, since
// the caller defers it without looking.
func TestStartPprofListener_NothingAskedFor_StartsNothing(t *testing.T) {
	logged := captureLogMessages(t)
	l, err := startPprofListener(t.Context(), "")
	if err != nil {
		t.Fatalf("startPprofListener(\"\"): %v", err)
	}
	if l != nil {
		t.Errorf("an empty address started a listener on %s", l.addr)
	}
	if logged("pprof listener") {
		t.Error("nothing was asked for, yet something was logged about the listener")
	}
	l.stop()
}

// TestStartPprofListener_Refusals covers the two ways starting fails: an
// address the network could reach, and a loopback port something else holds.
func TestStartPprofListener_Refusals(t *testing.T) {
	t.Run("not loopback", func(t *testing.T) {
		if _, err := startPprofListener(t.Context(), "0.0.0.0:0"); err == nil {
			t.Error("startPprofListener bound an address the network can reach")
		}
	})
	t.Run("port in use", func(t *testing.T) {
		held, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("hold a port: %v", err)
		}
		defer func() { _ = held.Close() }()
		_, err = startPprofListener(t.Context(), held.Addr().String())
		if err == nil || !strings.Contains(err.Error(), "--pprof-addr") {
			t.Errorf("startPprofListener on a held port = %v, want a refusal naming the flag", err)
		}
	})
}

// TestStartPprofListener_AcceptLoopFailure_IsLogged drives the accept loop
// to fail, which no network condition does on demand, and checks the failure
// is logged rather than swallowed: a listener that silently died would leave
// the benchmark waiting on a port nothing serves.
func TestStartPprofListener_AcceptLoopFailure_IsLogged(t *testing.T) {
	logged := captureLogMessages(t)
	previous := pprofServe
	pprofServe = func(_ *http.Server, ln net.Listener) error {
		_ = ln.Close()
		return errors.New("accept loop failed")
	}
	t.Cleanup(func() { pprofServe = previous })

	l, err := startPprofListener(t.Context(), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("startPprofListener: %v", err)
	}
	l.stop()
	if !logged("pprof listener stopped") {
		t.Error("the accept loop's failure was not logged")
	}
}

// TestRunWithContext_PprofAddr_NotLoopback_RefusesStartup verifies the
// refusal happens at startup, before any transport, in both modes: a process
// that served MCP while its profile listener was on the network would be the
// defect, and one that only failed later would already have served.
func TestRunWithContext_PprofAddr_NotLoopback_RefusesStartup(t *testing.T) {
	t.Setenv(config.EnvPrefix+"PPROF_ADDR", "0.0.0.0:0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cases := []struct {
		name string
		hcfg *httpConfig
	}{
		{name: "stdio", hcfg: nil},
		{name: "http", hcfg: &httpConfig{addr: ":0", gitlabURL: "https://gitlab.example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runWithContext(ctx, tc.hcfg)
			if err == nil || !strings.Contains(err.Error(), "--pprof-addr") {
				t.Errorf("runWithContext = %v, want the --pprof-addr refusal", err)
			}
		})
	}
}

// TestRunWithContext_PprofAddr_ServesWhileTheServerRuns starts HTTP mode with
// a profile listener and reads a heap profile off it while the MCP server is
// up, then checks the listener goes away with the process.
func TestRunWithContext_PprofAddr_ServesWhileTheServerRuns(t *testing.T) {
	srv := newMockGitLabServer(t)
	reserved, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := reserved.Addr().String()
	_ = reserved.Close()
	t.Setenv(config.EnvPrefix+"PPROF_ADDR", addr)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- runWithContext(ctx, &httpConfig{
			addr:           ":0",
			gitlabURL:      srv.URL,
			maxHTTPClients: config.DefaultMaxHTTPClients,
			sessionTimeout: config.DefaultSessionTimeout,
		})
	}()

	answered := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !answered {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/debug/pprof/heap", http.NoBody)
		if reqErr != nil {
			t.Fatalf("build request: %v", reqErr)
		}
		if resp, getErr := http.DefaultClient.Do(req); getErr == nil {
			answered = resp.StatusCode == http.StatusOK
			_ = resp.Body.Close()
		}
		if !answered {
			time.Sleep(20 * time.Millisecond)
		}
	}
	cancel()

	select {
	case runErr := <-errCh:
		if runErr != nil {
			t.Errorf("runWithContext: %v", runErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runWithContext did not return after cancellation")
	}
	if !answered {
		t.Error("the profile listener never answered a heap profile while the server ran")
	}
	req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/debug/pprof/heap", http.NoBody)
	if reqErr != nil {
		t.Fatalf("build request: %v", reqErr)
	}
	if resp, getErr := http.DefaultClient.Do(req); getErr == nil {
		_ = resp.Body.Close()
		t.Error("the profile listener outlived the server")
	}
}
