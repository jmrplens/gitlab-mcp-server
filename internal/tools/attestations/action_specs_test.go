// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package attestations

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

const registerAttestationsJSON = `[{"iid":1,"bundle":{"mediaType":"application/vnd.in-toto+json","content":{"_type":"https://in-toto.io/Statement/v1"}}}]`

// TestActionSpecs_Metadata validates the Metadata route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_Metadata(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	specs := ActionSpecs(client)
	if len(specs) != 2 {
		t.Fatalf("len(ActionSpecs) = %d, want 2", len(specs))
	}
	for _, spec := range specs {
		if !spec.ReadOnly || !spec.Idempotent || spec.OwnerPackage != "attestations" {
			t.Fatalf("unexpected ActionSpec semantics: %+v", spec)
		}
		if spec.Usage == "" {
			t.Fatalf("Usage for %s is empty", spec.Name)
		}
		if len(spec.Aliases) == 0 {
			t.Fatalf("Aliases for %s are empty", spec.Name)
		}
	}
}

// TestActionSpecs_SubjectDigest_PublishesTheFormTheListTakes verifies that
// the list publishes subject_digest with subjectDigestPattern, so find's
// example fills it with a digest where it wrote "value" (issue 1272), and that
// the pattern takes exactly the digests the handler serves: every value
// subjectDigest brings to GitLab's form, and none it refuses. Blanks around a
// digest are left out, since the handler trims what the schema states
// without. The download takes no digest.
func TestActionSpecs_SubjectDigest_PublishesTheFormTheListTakes(t *testing.T) {
	for _, spec := range ActionSpecs(nil) {
		t.Run(spec.Name, func(t *testing.T) {
			properties, _ := spec.Route.InputSchema["properties"].(map[string]any)
			digest, has := properties["subject_digest"].(map[string]any)
			if has != (spec.Name == "list") {
				t.Fatalf("%s has a subject_digest property: %t, want it on the list alone", spec.Name, has)
			}
			if has && digest["pattern"] != subjectDigestPattern {
				t.Errorf("%s subject_digest pattern = %v, want subjectDigestPattern", spec.Name, digest["pattern"])
			}
		})
	}

	pattern := regexp.MustCompile(subjectDigestPattern)
	for _, raw := range []string{
		testDigest,
		"sha256:" + testDigest,
		strings.ToUpper(testDigest),
		"sha256:" + strings.ToUpper(testDigest),
		testDigest[1:],
		testDigest + "0",
		"g" + testDigest[1:],
		"sha256:",
		"sha512:" + testDigest,
		"SHA256:" + testDigest,
		"0123456789abcdef0123456789abcdef01234567",
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := subjectDigest(raw)
			if served := err == nil; pattern.MatchString(raw) != served {
				t.Errorf("subjectDigestPattern matches %q = %t, and the handler serves it = %t", raw, pattern.MatchString(raw), served)
			}
		})
	}
}

// TestActionSpecs_CallRoutes validates the CallRoutes route through the catalog surface.
// The test exercises the GET path of the underlying GitLab API call.
// It asserts the route returns the expected error or result.
func TestActionSpecs_CallRoutes(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/attestations/"+testDigest):
			testutil.RespondJSON(w, http.StatusOK, registerAttestationsJSON)
		case r.Method == http.MethodGet && strings.Contains(path, "/attestations/"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"_type":"attestation"}`))
		default:
			http.NotFound(w, r)
		}
	})
	client := testutil.NewTestClient(t, mux)
	specs := ActionSpecs(client)
	specByTool := make(map[string]toolutil.ActionSpec, len(specs))
	for _, spec := range specs {
		specByTool[spec.IndividualTool.Name] = spec
	}

	tools := []struct {
		name string
		args map[string]any
	}{
		{"gitlab_list_attestations", map[string]any{"project_id": "42", "subject_digest": testDigest}},
		{"gitlab_download_attestation", map[string]any{"project_id": "42", "attestation_iid": float64(1)}},
	}
	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			spec, ok := specByTool[tt.name]
			if !ok {
				t.Fatalf("missing ActionSpec for %s", tt.name)
			}
			result, err := spec.Route.Handler(t.Context(), tt.args)
			if err != nil {
				t.Fatalf("Route.Handler(%s) error: %v", tt.name, err)
			}
			if result == nil {
				t.Fatalf("Route.Handler(%s) returned nil", tt.name)
			}
		})
	}
}
