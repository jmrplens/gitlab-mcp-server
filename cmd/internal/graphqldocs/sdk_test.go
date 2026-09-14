package graphqldocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSDKDocuments_NoDirectory_IsRefusedRatherThanSearched pins the one input
// this reader must not guess at.
//
// The directory comes from a lookup that can legitimately find nothing, and
// loading "./..." from the empty string would load whatever the process's
// working directory happens to be — this repository, during an audit. The
// documents of the wrong module reported as the SDK's is the class of silent
// wrongness the whole audit exists to remove.
func TestSDKDocuments_NoDirectory_IsRefusedRatherThanSearched(t *testing.T) {
	t.Parallel()

	for _, dir := range []string{"", "   "} {
		t.Run("dir="+dir, func(t *testing.T) {
			t.Parallel()

			if _, err := SDKDocuments(dir); err == nil {
				t.Error("SDKDocuments() accepted a directory that names nothing")
			}
		})
	}
}

// TestSDKDocuments_DirectoryItCannotLoad_Fails verifies that a module the
// toolchain will not load ends the read rather than being reported as a module
// with no GraphQL in it.
//
// This reads a module cache whose state the process does not own, so the
// failure is expected and has to stay a failure: an empty answer would say
// client-go sends no GraphQL, which is the opposite of true.
func TestSDKDocuments_DirectoryItCannotLoad_Fails(t *testing.T) {
	t.Parallel()

	_, err := SDKDocuments(filepath.Join(t.TempDir(), "nowhere"))

	if err == nil {
		t.Fatal("SDKDocuments() reported no error for a directory that is not a module")
	}
	if !strings.Contains(err.Error(), "client-go") {
		t.Errorf("SDKDocuments() error = %v, want it to name what it was reading", err)
	}
}

// TestForeignModuleEnv_CarriesTheTwoSettingsAModuleCacheLoadNeeds pins the
// environment without which this read does not work at all.
//
// Both were found by a load that failed outright. GOWORK=off, because
// client-go ships a go.work naming a directory its module zip does not carry,
// and workspace mode then refuses to load anything there. -mod=readonly,
// because a module cache directory cannot be written and a -mod=mod inherited
// from the caller's environment asks the toolchain for permission to write a
// go.mod. They are appended to the caller's environment rather than replacing
// it, since the load still needs GOPATH, GOMODCACHE and the rest.
func TestForeignModuleEnv_CarriesTheTwoSettingsAModuleCacheLoadNeeds(t *testing.T) {
	t.Setenv("GOWORK", "/somewhere/go.work")
	t.Setenv("GOFLAGS", "-mod=mod")

	env := ForeignModuleEnv()

	cases := []struct {
		name string
		want string
	}{
		{name: "workspace mode is off", want: "GOWORK=off"},
		{name: "the module cache is read-only", want: "GOFLAGS=-mod=readonly"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// The last assignment of a variable is the one the toolchain
			// honors, so what matters is that the setting comes after whatever
			// the caller's environment already said about it.
			name, _, _ := strings.Cut(testCase.want, "=")
			last := -1
			for i, entry := range env {
				if strings.HasPrefix(entry, name+"=") {
					last = i
				}
			}
			if last < 0 {
				t.Fatalf("ForeignModuleEnv() carries no %s", testCase.want)
			}
			if env[last] != testCase.want {
				t.Errorf("ForeignModuleEnv() ends with %q, want %q", env[last], testCase.want)
			}
		})
	}
	if len(env) <= len(os.Environ()) {
		t.Error("ForeignModuleEnv() replaced the caller's environment rather than adding to it")
	}
}

// TestIsTemplate_TellsAShellFromSendableText covers both shapes client-go
// writes a document as something other than the text GitLab receives, and the
// documents that merely look like one.
//
// The point of the predicate is the refusal list: a shell is refused by any
// schema on every single run, and permanent entries teach a reader to skip the
// one list here that must never be skipped. Getting it too wide is just as bad
// in the other direction, since a document quietly excused is a document
// nobody judges, so a percent sign that is not a verb and a brace that is not
// an action both have to stay judgeable.
func TestIsTemplate_TellsAShellFromSendableText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			name: "a text/template shell",
			text: "query ListWorkItems($fullPath: ID!{{ if .Decls }}, {{ .Decls }}{{ end }}) { id }",
			want: true,
		},
		{
			name: "a template that only names another template",
			text: `query GetWorkItem($iid: String!) { workItem(iid: $iid) { {{ template "WorkItem" }} } }`,
			want: true,
		},
		{
			name: "a printf format string",
			text: `query { project(fullPath: %q) { terraformStates { nodes { name } } } }`,
			want: true,
		},
		{
			name: "an ordinary document",
			text: `query ListAchievements($fullPath: ID!) { group(fullPath: $fullPath) { id } }`,
		},
		{
			name: "a percent sign inside a value the document sends",
			text: `query { projects(search: "100% coverage") { nodes { id } } }`,
		},
		{
			name: "a selection set nested three deep",
			text: `query { project { issues { nodes { id } } } }`,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if got := IsTemplate(Document{Text: testCase.text}); got != testCase.want {
				t.Errorf("IsTemplate() = %t, want %t", got, testCase.want)
			}
		})
	}
}

// TestSDKPatterns_AreTheWholeModule pins that the read is not a list of files
// to keep current.
//
// client-go puts each service's operations beside the methods that send them,
// so a named-file list would drop a new service's documents the day it was
// added and say nothing about having done so.
func TestSDKPatterns_AreTheWholeModule(t *testing.T) {
	t.Parallel()

	if got := SDKPatterns(); len(got) != 1 || got[0] != "./..." {
		t.Errorf("SDKPatterns() = %v, want the whole module", got)
	}
}
