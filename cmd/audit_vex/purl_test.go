// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// purl_test.go covers how a package URL is read into the parts the products
// and subcomponents are compared by, and every spelling it refuses.
package main

import (
	"maps"
	"strings"
	"testing"
)

// TestParsePURL_ReadsTheProductsThisRepositoryWrites covers the three shapes
// the document uses: a Go module with no version, a Go module at a version,
// and an image pinned by a percent-encoded digest with its repository.
func TestParsePURL_ReadsTheProductsThisRepositoryWrites(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		in   string
		want purl
	}{
		{
			name: "module at every version",
			in:   "pkg:golang/github.com/jmrplens/gitlab-mcp-server/v3",
			want: purl{kind: "golang", path: "github.com/jmrplens/gitlab-mcp-server/v3", qualifiers: map[string]string{}},
		},
		{
			name: "module at a version",
			in:   "pkg:GOLANG/golang.org/x/crypto@v0.57.0",
			want: purl{kind: "golang", path: "golang.org/x/crypto", version: "v0.57.0", qualifiers: map[string]string{}},
		},
		{
			name: "image by digest",
			in:   "pkg:oci/gitlab-mcp-server@sha256%3Aabc?Repository_URL=ghcr.io/jmrplens/gitlab-mcp-server&arch=amd64",
			want: purl{kind: "oci", path: "gitlab-mcp-server", version: "sha256:abc", qualifiers: map[string]string{
				"repository_url": "ghcr.io/jmrplens/gitlab-mcp-server", "arch": "amd64",
			}},
		},
		{
			name: "standard library",
			in:   "pkg:golang/stdlib",
			want: purl{kind: "golang", path: "stdlib", qualifiers: map[string]string{}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePURL(tc.in)
			if err != nil {
				t.Fatalf("parsePURL(%q): %v", tc.in, err)
			}
			if got.kind != tc.want.kind || got.path != tc.want.path || got.version != tc.want.version || !maps.Equal(got.qualifiers, tc.want.qualifiers) {
				t.Errorf("parsePURL(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParsePURL_RefusesWhatItCannotRead covers each refusal, every one of
// which has to name the package URL it refused.
func TestParsePURL_RefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, in, want string
	}{
		{name: "not a package URL", in: "https://example.com/x", want: "is not a package URL"},
		{name: "a subpath", in: "pkg:golang/example.com/x#sub", want: "carries a subpath"},
		{name: "no name", in: "pkg:golang", want: "names no type and name"},
		{name: "no type", in: "pkg:/example.com/x", want: "names no type and name"},
		{name: "an empty version", in: "pkg:golang/example.com/x@", want: "names no version"},
		{name: "a version that does not decode", in: "pkg:golang/example.com/x@%zz", want: "names no version"},
		{name: "an empty segment", in: "pkg:golang/example.com//x", want: "segment that does not decode"},
		{name: "a segment that does not decode", in: "pkg:golang/example.com/%zz", want: "segment that does not decode"},
		{name: "a qualifier with no value", in: "pkg:oci/x?arch", want: "not a key and a value"},
		{name: "a qualifier with no key", in: "pkg:oci/x?=amd64", want: "not a key and a value"},
		{name: "a qualifier that does not decode", in: "pkg:oci/x?arch=%zz", want: "not a key and a value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := parsePURL(tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), tc.in) {
				t.Errorf("parsePURL(%q) error = %v, want one naming the URL and %q", tc.in, err, tc.want)
			}
		})
	}
}
