// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package graphqlschema

import (
	"bytes"
	"strings"
	"testing"
)

// TestReleaseSchema_EmbeddedSDL_LoadsTheWholeReleaseSchema verifies that the
// committed release pin parses and carries the types the security documents
// name. A release pin that loads but is missing Vulnerability would accept
// every document the release refuses, which is the one thing it exists to
// catch.
func TestReleaseSchema_EmbeddedSDL_LoadsTheWholeReleaseSchema(t *testing.T) {
	schema, err := ReleaseSchema()
	if err != nil {
		t.Fatalf("ReleaseSchema() error = %v, want nil", err)
	}
	if schema.Query == nil || schema.Mutation == nil {
		t.Fatalf("ReleaseSchema() has query=%v mutation=%v, want both", schema.Query != nil, schema.Mutation != nil)
	}
	for _, name := range []string{"Vulnerability", "PipelineSecurityReportFinding", "VulnerabilityLocationCoverageFuzzing", "WorkItem"} {
		t.Run(name, func(t *testing.T) {
			if schema.Types[name] == nil {
				t.Errorf("the release pin has no type %q", name)
			}
		})
	}
}

// TestReleaseSchema_CalledTwice_ReturnsTheSameSchema verifies the release
// pin's sync.Once, for the reason the gitlab.com pin has one.
func TestReleaseSchema_CalledTwice_ReturnsTheSameSchema(t *testing.T) {
	first, err := ReleaseSchema()
	if err != nil {
		t.Fatalf("ReleaseSchema() error = %v", err)
	}
	second, err := ReleaseSchema()
	if err != nil {
		t.Fatalf("ReleaseSchema() second call error = %v", err)
	}
	if first != second {
		t.Error("ReleaseSchema() parsed the SDL twice; the sync.Once is not holding")
	}
}

// TestReleaseSchema_IsNotThePin verifies the two pins are two schemas of two
// GitLab versions. A release pin that was a copy of the gitlab.com one would
// judge nothing the pin does not, and gitlab.com always runs the pre-release
// of a minor no release has yet, so the two can never name one version.
func TestReleaseSchema_IsNotThePin(t *testing.T) {
	if bytes.Equal(releaseSDL, pinnedSDL) {
		t.Error("the release pin is byte for byte the gitlab.com pin")
	}
	release, err := ReleaseSourceInfo()
	if err != nil {
		t.Fatalf("ReleaseSourceInfo() error = %v", err)
	}
	pinned, err := SourceInfo()
	if err != nil {
		t.Fatalf("SourceInfo() error = %v", err)
	}
	if release.GitLabVersion == pinned.GitLabVersion {
		t.Errorf("both pins name GitLab %s, and the release pin is meant to be a release behind gitlab.com", release.GitLabVersion)
	}
}

// TestReleaseSourceInfo_EmbeddedRecord_NamesTheReleaseAndTheJob verifies that
// the committed release record decodes and is filled in, since a record that
// names no release cannot be held to the REST record's.
func TestReleaseSourceInfo_EmbeddedRecord_NamesTheReleaseAndTheJob(t *testing.T) {
	source, err := ReleaseSourceInfo()
	if err != nil {
		t.Fatalf("ReleaseSourceInfo() error = %v, want nil", err)
	}
	for _, testCase := range []struct{ name, value string }{
		{name: "gitlab_version", value: source.GitLabVersion},
		{name: "gitlab_revision", value: source.GitLabRevision},
		{name: "origin", value: source.Origin},
		{name: "sha256", value: source.SHA256},
		{name: "retrieved_at", value: source.RetrievedAt},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if strings.TrimSpace(testCase.value) == "" {
				t.Errorf("%s is empty in the committed release record", testCase.name)
			}
		})
	}
	release, err := ReleaseSchema()
	if err != nil {
		t.Fatalf("ReleaseSchema() error = %v", err)
	}
	if source.Types != len(release.Types) {
		t.Errorf("the record says %d types and the release pin loads with %d", source.Types, len(release.Types))
	}
}

// TestParseReleaseSource_MalformedRecord_ReportsTheFile verifies that a record
// which does not decode names its file.
func TestParseReleaseSource_MalformedRecord_ReportsTheFile(t *testing.T) {
	source, err := ParseReleaseSource([]byte("{not json"))
	if err == nil {
		t.Fatal("ParseReleaseSource() error = nil, want one")
	}
	if !strings.Contains(err.Error(), ReleaseSourceFileName) {
		t.Errorf("ParseReleaseSource() error = %q, want it to name %s", err, ReleaseSourceFileName)
	}
	if source != (ReleaseSource{}) {
		t.Errorf("ParseReleaseSource() source = %+v, want the zero value on failure", source)
	}
}

// TestParseReleaseSource_WellFormedRecord_ReadsEveryField verifies the decode
// field by field, under the names the generator writes.
func TestParseReleaseSource_WellFormedRecord_ReadsEveryField(t *testing.T) {
	source, err := ParseReleaseSource([]byte(`{
	  "gitlab_version": "19.5.0-ee",
	  "gitlab_revision": "883b289c40e",
	  "origin": "https://gitlab.com/gitlab-org/gitlab/-/jobs/1/artifacts/raw/tmp/tests/graphql/gitlab_schema.graphql",
	  "sha256": "abc",
	  "retrieved_at": "2026-10-30",
	  "types": 42
	}`))
	if err != nil {
		t.Fatalf("ParseReleaseSource() error = %v, want nil", err)
	}
	want := ReleaseSource{
		GitLabVersion:  "19.5.0-ee",
		GitLabRevision: "883b289c40e",
		Origin:         "https://gitlab.com/gitlab-org/gitlab/-/jobs/1/artifacts/raw/tmp/tests/graphql/gitlab_schema.graphql",
		SHA256:         "abc",
		RetrievedAt:    "2026-10-30",
		Types:          42,
	}
	if source != want {
		t.Errorf("ParseReleaseSource() = %+v, want %+v", source, want)
	}
}

// TestReleaseSource_String_ReadsAsOneReportLine verifies the line the document
// gate appends to its release summary.
func TestReleaseSource_String_ReadsAsOneReportLine(t *testing.T) {
	source := ReleaseSource{
		GitLabVersion:  "19.5.0-ee",
		GitLabRevision: "883b289c40e",
		Origin:         "https://gitlab.com/gitlab-org/gitlab/-/jobs/1/artifacts/raw/x.graphql",
		RetrievedAt:    "2026-10-30",
		Types:          4546,
	}
	const want = "4546 types of GitLab 19.5.0-ee (883b289c40e), from https://gitlab.com/gitlab-org/gitlab/-/jobs/1/artifacts/raw/x.graphql, retrieved 2026-10-30"
	if got := source.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
