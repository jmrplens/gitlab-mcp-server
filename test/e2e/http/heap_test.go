//go:build httpe2e

// heap_test.go measures what a pooled credential costs the process, through
// the profiling handlers the binary already serves.
//
// It is the regression test for ADR-0020's stated target: memory flat in the
// number of credentials. The claim is structural rather than statistical, so
// the assertion is a fixed budget on growth rather than a ratio between noisy
// numbers, and the budget is set far below what a server per credential costs
// and far above what the pool entries themselves do.
package httpe2e

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// heapCredentials is how many distinct credentials the measurement drives.
//
// Twenty is enough that a per-credential registered surface is unmissable and
// small enough that the run stays under a minute: each one costs a credential
// probe, a licence lookup, a scope lookup and an identity lookup against the
// fake GitLab, plus an initialize and a tools/list.
const heapCredentials = 20

// heapGrowthBudget is how much the live heap may grow between the first
// credential and the twentieth.
//
// The figure to compare it against is the one the benchmark measured for the
// arrangement this replaced: with a registered surface per credential, the live
// heap grew by about 3 MB per credential on the dynamic surface, so nineteen
// more would have added something near 57 MB. What remains per credential is a
// GitLab client, a rate-limit bucket, a counter and a watcher set. The budget
// sits between the two with room for the allocation noise of twenty sessions.
const heapGrowthBudget = 32 << 20

// TestSharedServer_LiveHeapDoesNotGrowWithTheNumberOfCredentials pins the
// target of ADR-0020.
//
// A server per credential made the resident set a straight line in the number
// of pooled credentials, and the line's slope was one registered tool catalog.
// Sharing the server per configuration shape is what flattens it, and nothing
// smaller than a real process with real pool entries can show that: the cost
// being removed is the SDK's tool table, the bound catalog descriptors and the
// closures that dispatch, none of which a unit test allocates.
//
// The heap is read through --pprof-addr, which the binary serves on loopback
// only, with ?gc=1 so each reading is of the live set rather than of whatever
// had not been collected yet.
func TestSharedServer_LiveHeapDoesNotGrowWithTheNumberOfCredentials(t *testing.T) {
	gitlab := startFakeGitLabServingAProject(t)
	pprofAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
	srv := startServer(t, nil,
		"--gitlab-url="+gitlab.URL,
		"--pprof-addr="+pprofAddr,
	)

	driveCredential(t, srv, "glpat-heap-1")
	first := liveHeapBytes(t, pprofAddr)

	for i := 2; i <= heapCredentials; i++ {
		driveCredential(t, srv, "glpat-heap-"+strconv.Itoa(i))
	}
	last := liveHeapBytes(t, pprofAddr)

	// One shape for every credential is the structural claim; the heap figure
	// is its consequence. Asserting both means a failure says which of the two
	// went wrong.
	if builds := countShapeBuilds(srv.logs()); builds != 1 {
		t.Errorf("the server built %d configuration shapes for %d credentials of one configuration, want 1",
			builds, heapCredentials)
	}

	growth := int64(last) - int64(first)
	t.Logf("live heap: %d bytes at 1 credential, %d at %d, growth %d bytes (%.1f MiB)",
		first, last, heapCredentials, growth, float64(growth)/(1<<20))
	if growth > heapGrowthBudget {
		t.Errorf("the live heap grew by %.1f MiB between 1 and %d credentials, budget %.1f MiB: "+
			"a registered surface is being built per credential again",
			float64(growth)/(1<<20), heapCredentials, float64(heapGrowthBudget)/(1<<20))
	}
}

// driveCredential runs one credential through a tools/list, which is what
// forces its pool entry to be built and its catalog to be ready.
//
// One self-contained POST is the whole exchange on the default stateless
// transport, and it is enough: the entry is built by the gate before the call
// is routed, and the readiness gate holds the listing until the catalog behind
// it exists.
func driveCredential(t *testing.T, srv *server, token string) {
	t.Helper()

	resp := srv.do(t, request{
		body:    toolsListBody,
		headers: map[string]string{"PRIVATE-TOKEN": token},
	})
	if resp.status != http.StatusOK {
		t.Fatalf("driving %s: status %d, body %s", token, resp.status, resp.body)
	}
	if strings.Contains(resp.body, `"error"`) {
		t.Fatalf("driving %s: %s", token, resp.body)
	}
}

// liveHeapBytes reads HeapAlloc from the profiling handlers, after a
// collection, so the figure is the live set rather than the allocation history.
func liveHeapBytes(t *testing.T, pprofAddr string) uint64 {
	t.Helper()

	url := "http://" + pprofAddr + "/debug/pprof/heap?gc=1&debug=1"
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, http.NoBody)
	if err != nil {
		t.Fatalf("building the heap profile request: %v", err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("reading the heap profile: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the heap profile answered %d, want 200", resp.StatusCode)
	}

	body := make([]byte, 0, 1<<20)
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		body = append(body, buf[:n]...)
		if readErr != nil {
			break
		}
	}

	// The text form of the heap profile ends with a runtime.MemStats dump, one
	// "# Field = value" per line.
	for line := range strings.SplitSeq(string(body), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "# HeapAlloc = ")
		if !ok {
			continue
		}
		value, parseErr := strconv.ParseUint(strings.TrimSpace(rest), 10, 64)
		if parseErr != nil {
			t.Fatalf("parsing HeapAlloc from %q: %v", line, parseErr)
		}
		return value
	}
	t.Fatalf("the heap profile carried no HeapAlloc line; got %d bytes", len(body))
	return 0
}
