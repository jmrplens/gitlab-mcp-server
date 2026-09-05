// profiles.go reads profiles off the server's pprof listener and writes
// them beside the record.
//
// The listener is the one --pprof-addr starts: loopback, on a port this
// driver picks, serving net/http/pprof on an http.Server of its own. Reading
// it is the only way the series can look inside the process it measures, and
// it is also how the goroutine count is taken without the traceback signal
// the point scenarios use, which ends the process and so cannot be sent
// between steps.

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// profileGrace is how long past its own duration a profile request is given
// before the driver gives up on it: the listener answers the moment the
// profile is written, and a longer wait means the server is not answering
// at all.
const profileGrace = 30 * time.Second

// captured is one profile as it came off the listener, or the reason it did
// not.
type captured struct {
	data []byte
	err  error
}

// pprofClient talks to one server's profile listener.
type pprofClient struct {
	base   string
	client *http.Client
}

// newPprofClient targets a listener by its base URL.
func newPprofClient(base string) *pprofClient {
	return &pprofClient{base: base, client: &http.Client{}}
}

// fetch reads one handler's body, failing on any answer but 200.
func (p *pprofClient) fetch(ctx context.Context, route string, timeout time.Duration) ([]byte, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, p.base+route, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", route, err)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", route, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", route, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", route, resp.StatusCode, firstLine(body))
	}
	return body, nil
}

// cpuProfile collects a CPU profile over the given seconds. The call blocks
// for that long: it is meant to run beside the load, not before or after it.
func (p *pprofClient) cpuProfile(ctx context.Context, seconds int) captured {
	data, err := p.fetch(ctx, "/debug/pprof/profile?seconds="+strconv.Itoa(seconds), time.Duration(seconds)*time.Second+profileGrace)
	return captured{data: data, err: err}
}

// heapProfile collects the heap as it is now.
func (p *pprofClient) heapProfile(ctx context.Context) captured {
	data, err := p.fetch(ctx, "/debug/pprof/heap", profileGrace)
	return captured{data: data, err: err}
}

// goroutineCount reads the total off the goroutine listing's first line.
func (p *pprofClient) goroutineCount(ctx context.Context) (int, error) {
	body, err := p.fetch(ctx, "/debug/pprof/goroutine?debug=1", profileGrace)
	if err != nil {
		return 0, err
	}
	return parseGoroutineTotal(body)
}

// parseGoroutineTotal reads "goroutine profile: total N" off the first line
// of a debug=1 goroutine listing.
func parseGoroutineTotal(body []byte) (int, error) {
	first, _, _ := strings.Cut(string(body), "\n")
	rest, ok := strings.CutPrefix(first, "goroutine profile: total ")
	if !ok {
		return 0, fmt.Errorf("unexpected goroutine listing header %q", firstLine(body))
	}
	count, err := strconv.Atoi(strings.TrimSpace(rest))
	if err != nil {
		return 0, fmt.Errorf("parse goroutine total %q: %w", rest, err)
	}
	return count, nil
}

// writeProfiles stores a step's two profiles under the profiles directory as
// <scenario>/<clients>.<kind>.pb.gz, returning their paths relative to it
// and a note for each one that could not be taken or written.
func writeProfiles(dir, scenarioID string, clients int, cpu, heap captured) (profiles StepProfiles, notes []string) {
	store := func(kind string, c captured) string {
		if c.err != nil {
			notes = append(notes, kind+" profile unavailable: "+c.err.Error())
			return ""
		}
		relative, err := writeProfile(dir, scenarioID, clients, kind, c.data)
		if err != nil {
			notes = append(notes, kind+" profile not written: "+err.Error())
			return ""
		}
		return relative
	}
	return StepProfiles{CPU: store("cpu", cpu), Heap: store("heap", heap)}, notes
}

// writeProfile writes one profile and returns its path relative to dir,
// spelled with slashes so the record reads the same on every platform.
func writeProfile(dir, scenarioID string, clients int, kind string, data []byte) (string, error) {
	relative := path.Join(scenarioID, strconv.Itoa(clients)+"."+kind+".pb.gz")
	full := filepath.Join(dir, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", fmt.Errorf("create %s: %w", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", full, err)
	}
	return relative, nil
}
