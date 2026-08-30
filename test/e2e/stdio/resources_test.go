//go:build stdioe2e

package stdioe2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestResourceList_SaysHowMuchOfTheCollectionItReturned pins that a collection
// resource no longer overstates itself.
//
// The transport specification scopes pagination to the list operations and
// gives resources/read none: "Operations Supporting Pagination" names
// resources/list, resources/templates/list, prompts/list and tools/list, and
// the Resources page says of read only that it supports caching. So there is no
// cursor a client could follow and no partial-result shape in the schema. That
// makes disclosure the server's entire responsibility, and there was none.
//
// What these resources did was ask GitLab for a collection with no per_page,
// get its default of twenty, and return them as a bare JSON array under a
// description beginning "List all". On a real instance gitlab://groups answered
// 20 of 137 and said nothing about the other 117. The tool surface reports its
// pagination correctly on the same data, so a model that asked the same
// question two ways got two different answers and no way to tell which was
// whole.
//
// Two halves are checked here because either alone would be a false pass: the
// page size, since a fake honoring per_page returns a hundred only if a
// hundred was asked for, and the disclosure, since a complete-looking body is
// exactly the failure.
func TestResourceList_SaysHowMuchOfTheCollectionItReturned(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["CAPABILITY_SURFACE"] = "full"
	s := startSession(t, env)

	got := s.call(t, request(1, "resources/read", `{"uri":"gitlab://groups"}`))
	if got["error"] != nil {
		t.Fatalf("resources/read failed: %v", got["error"])
	}
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result: %v", got)
	}

	contents, ok := result["contents"].([]any)
	if !ok || len(contents) == 0 {
		t.Fatalf("no contents: %v", result)
	}
	first, ok := contents[0].(map[string]any)
	if !ok {
		t.Fatalf("contents[0] is not an object: %v", contents[0])
	}
	body, _ := first["text"].(string)

	var groups []map[string]any
	if err := json.Unmarshal([]byte(body), &groups); err != nil {
		t.Fatalf("the body is not a JSON array: %v (%.200s)", err, body)
	}
	if len(groups) != 100 {
		t.Errorf("%d groups returned, want 100; the read is asking GitLab for its default page of 20", len(groups))
	}

	meta, ok := result["_meta"].(map[string]any)
	if !ok {
		t.Fatalf("the read carries no _meta, so nothing says the collection is partial: %v", result)
	}
	page, ok := meta["io.github.jmrplens/pageInfo"].(map[string]any)
	if !ok {
		t.Fatalf("no pageInfo in _meta: %v", meta)
	}
	// Each field is required to be present and of the right type before its
	// value is read. A comma-ok assertion that falls through to the zero value
	// would let an absent "complete" read as false, which is the answer this
	// case is looking for: the test would pass on a server that disclosed
	// nothing at all.
	complete, ok := page["complete"].(bool)
	if !ok {
		t.Fatalf("no boolean \"complete\" in pageInfo, so nothing states whether the read is partial: %v", page)
	}
	if complete {
		t.Errorf("the read claims to be complete while returning %d of %d: %v", len(groups), fakeGroupTotal, page)
	}
	total, ok := page["total"].(float64)
	if !ok {
		t.Fatalf("no numeric \"total\" in pageInfo: %v", page)
	}
	if int(total) != fakeGroupTotal {
		t.Errorf("total = %v, want %d so a consumer knows what it is missing", total, fakeGroupTotal)
	}
	returned, ok := page["returned"].(float64)
	if !ok {
		t.Fatalf("no numeric \"returned\" in pageInfo: %v", page)
	}
	if int(returned) != len(groups) {
		t.Errorf("returned = %v, want %d", returned, len(groups))
	}
}

// TestCacheHints_ReachTheWire pins that the cache-hint middleware is actually
// registered.
//
// Its unit tests exercise the policy table directly, so deleting the
// registration in createServer broke no test at all: the table stayed correct
// and nothing checked that anything consulted it.
//
// What that costs is not "no hints". The SDK's setDefaultCacheableValues
// assigns "public" to any result that reaches it unstamped, so an unregistered
// middleware does not fail open to uncached: it fails open to publicly
// cacheable, on every catalog this server filters by the caller's token and
// tier. Removing the registration and rerunning this shows exactly that:
// tools/list comes back public. That is the failure mode worth a wire
// assertion, and it is invisible from inside the package.
//
// prompts/list is the one body that is genuinely public, and the claim behind
// it is that the prompt catalog does not vary per caller.
func TestCacheHints_ReachTheWire(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["CAPABILITY_SURFACE"] = "full"
	s := startSession(t, env)

	cases := []struct {
		method    string
		wantScope string
	}{
		{"tools/list", "private"},
		{"prompts/list", "public"},
		{"resources/list", "private"},
		{"resources/templates/list", "private"},
	}

	for id, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			got := s.call(t, request(id+1, tc.method, ""))
			if got["error"] != nil {
				t.Fatalf("%s failed: %v", tc.method, got["error"])
			}
			result, _ := got["result"].(map[string]any)

			scope, ok := result["cacheScope"].(string)
			if !ok {
				t.Fatalf("%s carries no cacheScope; the middleware is not registered: %v", tc.method, result)
			}
			if scope != tc.wantScope {
				t.Errorf("cacheScope = %q, want %q", scope, tc.wantScope)
			}
			ttlMs, ok := result["ttlMs"].(float64)
			if !ok {
				t.Fatalf("%s carries no numeric ttlMs, so a client has no freshness window to honor: %v", tc.method, result["ttlMs"])
			}
			if ttlMs < 0 {
				t.Errorf("ttlMs = %v, and the specification requires >= 0", ttlMs)
			}
		})
	}
}

// TestResourceList_DescriptionsDoNotPromiseEverything checks the other half of
// the same defect, which no code change would have fixed.
//
// Ten templates began "List all …" while returning one page. The description is
// what a model reads when it decides whether a resource answers its question,
// so an inaccurate one sends it to the wrong surface: it will read the resource
// and conclude the instance has twenty groups, rather than call the tool that
// paginates.
func TestResourceList_DescriptionsDoNotPromiseEverything(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["CAPABILITY_SURFACE"] = "full"
	s := startSession(t, env)

	templates := entriesOf(t, s, request(1, "resources/templates/list", ""), "resourceTemplates")
	assertNoCompletenessPromise(t, templates, "uriTemplate")

	// The same claim on the static resources, which resources/list carries.
	resources := entriesOf(t, s, request(2, "resources/list", ""), "resources")
	assertNoCompletenessPromise(t, resources, "uri")
}

// entriesOf calls a listing method and returns the named array from its result.
//
// It fails on anything unexpected rather than falling through to an empty
// slice: a listing that errored would otherwise leave nothing to iterate, and
// every assertion made over it would pass by not running.
func entriesOf(t *testing.T, s *session, req, field string) []any {
	t.Helper()

	got := s.call(t, req)
	if got["error"] != nil {
		t.Fatalf("listing failed: %v", got["error"])
	}
	result, ok := got["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result object: %v", got)
	}
	entries, ok := result[field].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("no %s listed: %v", field, result)
	}
	return entries
}

// assertNoCompletenessPromise fails for any entry whose description claims to
// return a whole collection.
func assertNoCompletenessPromise(t *testing.T, entries []any, idField string) {
	t.Helper()

	for _, entry := range entries {
		listing, ok := entry.(map[string]any)
		if !ok {
			t.Errorf("a listing entry is not an object: %v", entry)
			continue
		}
		description, _ := listing["description"].(string)
		if strings.Contains(description, "List all ") {
			t.Errorf("%v promises to list all of a collection it reads one page of: %q",
				listing[idField], description)
		}
	}
}
