package graphqlschema

import (
	"strings"
	"testing"
)

// TestSourceInfo_EmbeddedRecord_NamesTheInstanceAndTheDay verifies that the
// committed provenance decodes and is filled in. A pin whose record says
// nothing is a pin nobody can judge the age of, which is the whole reason the
// file exists.
func TestSourceInfo_EmbeddedRecord_NamesTheInstanceAndTheDay(t *testing.T) {
	source, err := SourceInfo()
	if err != nil {
		t.Fatalf("SourceInfo() error = %v, want nil", err)
	}
	cases := []struct {
		name  string
		value string
	}{
		{name: "instance", value: source.Instance},
		{name: "gitlab_version", value: source.GitLabVersion},
		{name: "retrieved_at", value: source.RetrievedAt},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if strings.TrimSpace(testCase.value) == "" {
				t.Errorf("%s is empty in the committed provenance record", testCase.name)
			}
		})
	}
	if source.Types <= 0 {
		t.Errorf("types = %d, want the count the introspection carried", source.Types)
	}
}

// TestParseSource_MalformedRecord_ReportsTheFile verifies that a record which
// does not decode says which file to look at, since the error surfaces from
// commands whose other failures are about the schema.
func TestParseSource_MalformedRecord_ReportsTheFile(t *testing.T) {
	source, err := ParseSource([]byte("{not json"))

	if err == nil {
		t.Fatal("ParseSource() error = nil, want one")
	}
	if !strings.Contains(err.Error(), SourceFileName) {
		t.Errorf("ParseSource() error = %q, want it to name %s", err, SourceFileName)
	}
	if source != (Source{}) {
		t.Errorf("ParseSource() source = %+v, want the zero value on failure", source)
	}
}

// TestParseSource_WellFormedRecord_ReadsEveryField verifies the decode itself,
// including the revision that only a run with a token can fill in.
func TestParseSource_WellFormedRecord_ReadsEveryField(t *testing.T) {
	source, err := ParseSource([]byte(`{
	  "instance": "https://gitlab.example.com/api/graphql",
	  "gitlab_version": "19.4.0",
	  "gitlab_revision": "abc1234",
	  "retrieved_at": "2026-09-06",
	  "types": 42
	}`))
	if err != nil {
		t.Fatalf("ParseSource() error = %v, want nil", err)
	}

	want := Source{
		Instance:       "https://gitlab.example.com/api/graphql",
		GitLabVersion:  "19.4.0",
		GitLabRevision: "abc1234",
		RetrievedAt:    "2026-09-06",
		Types:          42,
	}
	if source != want {
		t.Errorf("ParseSource() = %+v, want %+v", source, want)
	}
}

// TestSource_String_ReadsAsOneReportLine verifies the rendering both commands
// append to their summary, so a reader is told the age of the pin by every
// gate that consults it.
func TestSource_String_ReadsAsOneReportLine(t *testing.T) {
	source := Source{
		Instance:      "https://gitlab.com/api/graphql",
		GitLabVersion: "19.4.0-pre",
		RetrievedAt:   "2026-09-06",
		Types:         4331,
	}

	const want = "4331 types from https://gitlab.com/api/graphql (GitLab 19.4.0-pre), retrieved 2026-09-06"
	if got := source.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
