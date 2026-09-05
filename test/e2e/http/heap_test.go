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
	"encoding/json"
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
// probe, a license lookup, a scope lookup and an identity lookup against the
// fake GitLab, plus an initialize and a tools/list.
const heapCredentials = 20

// heapGrowthBudget is how much the live heap may grow between the first
// credential and the twentieth.
//
// It is set from the two figures this test sits between, both measured on this
// same harness rather than taken from the analysis page. On this branch the
// growth over 1 to 20 credentials is under 0.2 MiB, on both surfaces and
// repeatably. On the parent commit, where the server is built per credential,
// the same run grows 8.1 MiB on the dynamic surface and 27.6 MiB on individual.
// Two mebibytes is therefore ten times what the shared server costs and four
// times under the smaller of the two regressions, which is what makes a revert
// of the change this guards fail here rather than ship green.
//
// The earlier budget was 32 MiB, chosen against a per-credential figure quoted
// from an alternatives table rather than measured, and it passed on the parent
// commit on every surface: the test asserted nothing at all.
const heapGrowthBudget = 2 << 20

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
// It runs on both ends of the surface range on purpose. The dynamic surface is
// the default and registers two tools, so it is the weakest signal this test
// can be given: the parent commit grows 8.1 MiB there against a 2 MiB budget.
// The individual surface registers a tool per action, which is where a server
// per credential costs the most and where the same run grows 27.6 MiB. A
// regression that somehow stayed inside the budget on dynamic has nowhere to
// hide on individual.
//
// The heap is read through --pprof-addr, which the binary serves on loopback
// only, with ?gc=1 so each reading is of the live set rather than of whatever
// had not been collected yet.
func TestSharedServer_LiveHeapDoesNotGrowWithTheNumberOfCredentials(t *testing.T) {
	surfaces := []string{"dynamic", "individual"}

	for _, surface := range surfaces {
		t.Run(surface, func(t *testing.T) {
			gitlab := startFakeGitLabServingAProject(t)
			pprofAddr := fmt.Sprintf("127.0.0.1:%d", freePort(t))
			srv := startServer(t, nil,
				"--gitlab-url="+gitlab.URL,
				"--tool-surface="+surface,
				"--pprof-addr="+pprofAddr,
			)

			driveCredential(t, srv, "glpat-heap-1")
			first := liveHeapBytes(t, pprofAddr)

			for i := 2; i <= heapCredentials; i++ {
				driveCredential(t, srv, "glpat-heap-"+strconv.Itoa(i))
			}
			last := liveHeapBytes(t, pprofAddr)

			// One shape for every credential is the structural claim; the heap
			// figure is its consequence. Asserting both means a failure says
			// which of the two went wrong.
			if builds := countShapeBuilds(srv.logs()); builds != 1 {
				t.Errorf("the server built %d configuration shapes for %d credentials of one configuration, want 1",
					builds, heapCredentials)
			}

			// Signed, because the growth is a difference and can be negative:
			// the readings are HeapAlloc, and a later one is often the smaller.
			// Neither can approach the range of an int64, being the live heap
			// of a process this test would have run out of memory to hold.
			growth := int64(last) - int64(first) //nolint:gosec // both figures are a live heap, orders below 2^63
			t.Logf("live heap on %s: %d bytes at 1 credential, %d at %d, growth %d bytes (%.1f MiB)",
				surface, first, last, heapCredentials, growth, float64(growth)/(1<<20))
			if growth > heapGrowthBudget {
				t.Errorf("the live heap grew by %.1f MiB between 1 and %d credentials on the %s surface, "+
					"budget %.1f MiB: a registered surface is being built per credential again",
					float64(growth)/(1<<20), heapCredentials, surface, float64(heapGrowthBudget)/(1<<20))
			}
		})
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
	if rpcErr := jsonRPCErrorIn(resp.body); rpcErr != "" {
		t.Fatalf("driving %s: %s", token, rpcErr)
	}
}

// jsonRPCErrorIn returns the error member of a JSON-RPC response, or "" when
// the response carries none.
//
// The envelope is decoded rather than searched for the word. A tools/list body
// on the individual surface contains "error" five times as ordinary catalog
// text (an enum value and a property name among them), so a substring probe
// aborts every run on that surface with a failure that is not there. It was
// what kept this test from being run on the surface where its signal is
// strongest.
//
// The body arrives either as a bare JSON object or as SSE frames, so the
// decode is per line and a line that is not an envelope is skipped rather than
// reported: this is a probe for a refusal, not a parser.
func jsonRPCErrorIn(body string) string {
	for line := range strings.SplitSeq(body, "\n") {
		payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "data: "))
		if !strings.HasPrefix(payload, "{") {
			continue
		}
		var envelope struct {
			Error json.RawMessage `json:"error"`
		}
		if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
			continue
		}
		if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
			return string(envelope.Error)
		}
	}
	return ""
}

// liveHeapBytes reads HeapAlloc from the profiling handlers, after a
// collection, so the figure is the live set rather than the allocation history.
//
// Two collections rather than one. The first reading of a process that has just
// registered a catalog still counts what that registration left behind for one
// more cycle (finalizers, the profile's own buffers, the response still on the
// wire), and on the individual surface that was worth several mebibytes, which
// is the same order as the budget this test asserts. Reading twice makes both
// ends of the comparison a settled heap.
func liveHeapBytes(t *testing.T, pprofAddr string) uint64 {
	t.Helper()

	_ = readHeapAlloc(t, pprofAddr)
	return readHeapAlloc(t, pprofAddr)
}

// readHeapAlloc performs one collection and returns the live heap it reports.
func readHeapAlloc(t *testing.T, pprofAddr string) uint64 {
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
