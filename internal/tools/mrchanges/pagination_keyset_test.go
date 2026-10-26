package mrchanges

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

func TestGet_KeysetContinuation_AdvancesAndPreservesListing(t *testing.T) {
	var paths []string
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		// sequential: these are fields of one received request, not separate scenarios.
		for name, want := range map[string]string{"pagination": "keyset", "order_by": "id", "sort": "asc", "per_page": "100", "unidiff": "true"} {
			if query.Get(name) != want {
				t.Errorf("%s = %q, want %q", name, query.Get(name), want)
			}
		}
		if query.Has("page_token") {
			t.Error("the continuation link was sent as a raw page_token")
		}
		path := "first.go"
		// The synthetic Link exercises client continuation handling; it does not
		// establish GitLab support for keyset on this endpoint.
		if query.Get("id_after") == "" {
			query.Set("id_after", "100")
			w.Header().Set("Link", fmt.Sprintf("<http://%s%s?%s>; rel=\"next\"", r.Host, r.URL.Path, query.Encode()))
		} else {
			if query.Get("id_after") != "100" {
				t.Errorf("id_after = %q, want 100", query.Get("id_after"))
			}
			path = "second.go"
		}
		paths = append(paths, path)
		testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`[{"new_path":%q}]`, path))
	}))

	first, err := Get(t.Context(), client, GetInput{
		ProjectID: "42", MRIID: 1, Unidiff: true, OrderBy: "id", Sort: "asc",
		PerPage:    100,
		Pagination: "keyset",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.Pagination.HasMore || first.Pagination.NextPage != 0 || first.Pagination.NextPageToken == "" {
		t.Fatalf("first continuation = %+v, want keyset next_page_token", first.Pagination)
	}

	second, err := Get(t.Context(), client, GetInput{
		ProjectID: "42", MRIID: 1, Sort: "desc",
		PageToken: first.Pagination.NextPageToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Pagination.HasMore || second.Pagination.NextPageToken != "" {
		t.Errorf("last continuation = %+v, want no successor", second.Pagination)
	}
	if len(first.Changes) != 1 || len(second.Changes) != 1 ||
		first.Changes[0].NewPath != "first.go" || second.Changes[0].NewPath != "second.go" ||
		!slices.Equal(paths, []string{"first.go", "second.go"}) {
		t.Fatalf("pages = %+v, %+v; requests = %v", first.Changes, second.Changes, paths)
	}
}

func TestGet_KeysetCredentialParameter_RefusesBeforeRequest(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a credential-bearing continuation reached GitLab")
		w.WriteHeader(http.StatusBadRequest)
	}))

	_, err := Get(t.Context(), client, GetInput{
		ProjectID: "42", MRIID: 1,
		// This synthetic token tests query rejection; no request is sent.
		PageToken: "http://127.0.0.1/diffs?private_token=synthetic-test-credential",
	})

	if err == nil || !strings.Contains(err.Error(), "private_token") {
		t.Fatalf("error = %v, want rejected credential parameter", err)
	}
}

func TestActionSpecs_ChangesGetPagination_DescribesOffsetContinuation(t *testing.T) {
	specs := ActionSpecs(nil)
	usage := specs[0].Usage
	for _, name := range []string{"next_page as page", "has_more", "per_page: 100"} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(usage, name) {
				t.Errorf("usage lacks %s: %s", name, usage)
			}
		})
	}
	for _, name := range []string{"keyset", "page_token"} {
		t.Run("unadvertised_"+name, func(t *testing.T) {
			if strings.Contains(strings.ToLower(usage), name) {
				t.Errorf("usage advertises %s: %s", name, usage)
			}
		})
	}
	for _, name := range []string{"page", "per_page"} {
		t.Run("guidance_"+name, func(t *testing.T) {
			if _, ok := specs[0].ParameterGuidance[name]; !ok {
				t.Errorf("parameter guidance lacks %s", name)
			}
		})
	}
	if _, ok := specs[0].ParameterGuidance["page_token"]; ok {
		t.Error("parameter guidance advertises page_token for MR diffs")
	}
}
