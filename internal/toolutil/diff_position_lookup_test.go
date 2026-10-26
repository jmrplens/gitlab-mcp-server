package toolutil

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// positionCheckRefs identifies the synthetic snapshot all successful reads use.
var positionCheckRefs = gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "head"}

const positionCheckMRPath = "/api/v4/projects/42/merge_requests/7"

func TestMergeRequestDiffForPositionCheck_TargetPath_StopsAtMatchingPage(t *testing.T) {
	cases := []struct {
		name string
		page int
		path string
	}{
		{"FirstPageNewPath", 1, "new.go"},
		{"SecondPageOldPath", 2, "old.go"},
		{"ThirdPageNewPath", 3, "new.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var pages []int
			snapshots := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == positionCheckMRPath {
					snapshots++
					respondPositionCheckRefs(w, positionCheckRefs)
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				pages = append(pages, page)
				if r.URL.Query().Get("per_page") != "100" {
					t.Errorf("per_page = %q, want 100", r.URL.Query().Get("per_page"))
				}
				body := "[null,{\"new_path\":\"other.go\"}]"
				if page == tc.page {
					body = "[{\"new_path\":\"new.go\",\"old_path\":\"old.go\",\"diff\":\"patch\"}]"
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, body,
					testutil.PaginationHeaders{Page: strconv.Itoa(page), NextPage: strconv.Itoa(page + 1)})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, tc.path, positionCheckRefs)

			if !ok || diff == nil || diff.Diff != "patch" {
				t.Fatalf("lookup = (%+v, %t), want the target patch", diff, ok)
			}
			if pages[len(pages)-1] != tc.page || snapshots != 2 {
				t.Errorf("pages = %v, snapshots = %d; want stop at %d and two snapshots", pages, snapshots, tc.page)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_AbsentFile_ConfirmsCompleteSnapshot(t *testing.T) {
	snapshots := 0
	pages := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			snapshots++
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		pages++
		testutil.RespondJSON(w, http.StatusOK, "[]")
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "missing.go", positionCheckRefs)

	if diff != nil || !ok || snapshots != 2 || pages != 1 {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d, pages = %d", diff, ok, snapshots, pages)
	}
}

func TestMergeRequestDiffForPositionCheck_PageBudget_StopsAfterHundredRequests(t *testing.T) {
	cases := []struct {
		name, lastBody, lastNext string
		wantOK, wantFound        bool
	}{
		{"FoundOnLastPage", "[{\"new_path\":\"target.go\"}]", "101", true, true},
		{"CompleteOnLastPage", "[]", "", true, false},
		{"MorePagesRemain", "[]", "101", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages++
				if pages > 100 {
					t.Error("request 101 must not be sent")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				body, next := "[]", strconv.Itoa(pages+1)
				if pages == 100 {
					body, next = tc.lastBody, tc.lastNext
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, body,
					testutil.PaginationHeaders{Page: strconv.Itoa(pages), NextPage: next})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if ok != tc.wantOK || (diff != nil) != tc.wantFound || pages != 100 {
				t.Errorf("lookup = (%+v, %t), pages = %d; want found=%t, ok=%t, pages=100", diff, ok, pages, tc.wantFound, tc.wantOK)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_InvalidExpectedRefs_SkipsRequests(t *testing.T) {
	cases := []gl.MergeRequestDiffRefs{
		{StartSha: "start", HeadSha: "head"},
		{BaseSha: "base", HeadSha: "head"},
		{BaseSha: "base", StartSha: "start"},
	}
	for i, refs := range cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				t.Error("an incomplete expected snapshot must not send a request")
				w.WriteHeader(http.StatusBadRequest)
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", refs)

			if diff != nil || ok {
				t.Fatalf("lookup = (%+v, %t), want (nil, false)", diff, ok)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_ChangedSnapshot_DiscardsResult(t *testing.T) {
	cases := []struct {
		name, pageBody string
		refs           gl.MergeRequestDiffRefs
	}{
		{"BaseChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "changed", StartSha: "start", HeadSha: "head"}},
		{"StartChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "changed", HeadSha: "head"}},
		{"HeadChanged", "[{\"new_path\":\"target.go\"}]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "changed"}},
		{"AbsentFileHeadChanged", "[]", gl.MergeRequestDiffRefs{BaseSha: "base", StartSha: "start", HeadSha: "changed"}},
		{"RefsMissing", "[]", gl.MergeRequestDiffRefs{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshots := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == positionCheckMRPath {
					snapshots++
					refs := positionCheckRefs
					if snapshots == 2 {
						refs = tc.refs
					}
					respondPositionCheckRefs(w, refs)
					return
				}
				testutil.RespondJSON(w, http.StatusOK, tc.pageBody)
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || snapshots != 2 {
				t.Fatalf("lookup = (%+v, %t), snapshots = %d; want (nil, false), 2", diff, ok, snapshots)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_UnconfirmedInitialSnapshot_SkipsDiffs(t *testing.T) {
	cases := []struct {
		name, body string
		status     int
	}{
		{"PermissionRefused", "{\"message\":\"Forbidden\"}", http.StatusForbidden},
		{"DifferentRefs", "{\"diff_refs\":{\"base_sha\":\"other\",\"start_sha\":\"start\",\"head_sha\":\"head\"}}", http.StatusOK},
		{"MissingRefs", "{}", http.StatusOK},
		{"NullSnapshot", "null", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != positionCheckMRPath {
					t.Errorf("unexpected diff request %s", r.URL.Path)
				}
				testutil.RespondJSON(w, tc.status, tc.body)
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || calls != 1 {
				t.Fatalf("lookup = (%+v, %t), calls = %d; want (nil, false), 1", diff, ok, calls)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_FinalSnapshotError_DiscardsMatchingFile(t *testing.T) {
	snapshots := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			snapshots++
			if snapshots == 1 {
				respondPositionCheckRefs(w, positionCheckRefs)
				return
			}
			w.WriteHeader(http.StatusForbidden)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, "[{\"new_path\":\"target.go\",\"diff\":\"patch\"}]")
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || snapshots != 2 {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d; want (nil, false), 2", diff, ok, snapshots)
	}
}

func TestMergeRequestDiffForPositionCheck_LaterPageFails_DiscardsPartialEvidence(t *testing.T) {
	pages := 0
	client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		if pages == 1 {
			testutil.RespondJSONWithPagination(w, http.StatusOK, "[{\"new_path\":\"other.go\"}]",
				testutil.PaginationHeaders{Page: "1", NextPage: "2"})
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))

	diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || pages != 2 {
		t.Fatalf("lookup = (%+v, %t), pages = %d; want (nil, false), 2", diff, ok, pages)
	}
}

func TestMergeRequestDiffForPositionCheck_NonAdvancingPage_SkipsCheck(t *testing.T) {
	cases := []struct{ name, next string }{{"Repeated", "3"}, {"Backwards", "2"}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pages := 0
			client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				pages++
				next := "3"
				if r.URL.Query().Get("page") == "3" {
					next = tc.next
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, "[]",
					testutil.PaginationHeaders{Page: r.URL.Query().Get("page"), NextPage: next})
			}))

			diff, ok := MergeRequestDiffForPositionCheck(t.Context(), client, "42", 7, "target.go", positionCheckRefs)

			if diff != nil || ok || pages != 2 {
				t.Fatalf("lookup = (%+v, %t), pages = %d; want (nil, false), 2", diff, ok, pages)
			}
		})
	}
}

func TestMergeRequestDiffForPositionCheck_CancelledContext_SendsNoRequests(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("cancelled lookup sent a request")
		w.WriteHeader(http.StatusBadRequest)
	}))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok {
		t.Fatalf("lookup = (%+v, %t), want (nil, false)", diff, ok)
	}
}

func TestMergeRequestDiffForPositionCheck_CancelledBetweenPages_SendsNoNextPage(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	pages := 0
	client := newStablePositionCheckClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		pages++
		cancel()
		testutil.RespondJSONWithPagination(w, http.StatusOK, "[]",
			testutil.PaginationHeaders{Page: "1", NextPage: "2"})
	}))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || pages != 1 {
		t.Fatalf("lookup = (%+v, %t), pages = %d; want (nil, false), 1", diff, ok, pages)
	}
}

func TestMergeRequestDiffForPositionCheck_CancelledAtMatchingPage_SkipsSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	snapshots := 0
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			snapshots++
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		cancel()
		testutil.RespondJSON(w, http.StatusOK, "[{\"new_path\":\"target.go\"}]")
	}))

	diff, ok := MergeRequestDiffForPositionCheck(ctx, client, "42", 7, "target.go", positionCheckRefs)

	if diff != nil || ok || snapshots != 1 {
		t.Fatalf("lookup = (%+v, %t), snapshots = %d; want (nil, false), 1", diff, ok, snapshots)
	}
}

// newStablePositionCheckClient supplies only the mechanical snapshot endpoint;
// each test owns the diff endpoint and its behavioral assertions.
func newStablePositionCheckClient(t *testing.T, diffs http.Handler) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == positionCheckMRPath {
			respondPositionCheckRefs(w, positionCheckRefs)
			return
		}
		diffs.ServeHTTP(w, r)
	}))
}

// respondPositionCheckRefs writes the synthetic snapshot using GitLab's shape.
func respondPositionCheckRefs(w http.ResponseWriter, refs gl.MergeRequestDiffRefs) {
	body, err := json.Marshal(struct {
		Refs gl.MergeRequestDiffRefs `json:"diff_refs"`
	}{refs})
	if err != nil {
		http.Error(w, fmt.Sprintf("encode fixture: %v", err), http.StatusInternalServerError)
		return
	}
	testutil.RespondJSON(w, http.StatusOK, string(body))
}
