// list_test.go contains unit tests for the repository submodule MCP tool handlers.
// Tests use httptest to mock GitLab API responses and verify success, error,
// and edge-case paths.
package repositorysubmodules

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
)

// sampleGitmodules identifies the sample gitmodules constant used by this package.
const sampleGitmodules = `[submodule "dsp_up_spi_common_module"]
	path = dsp_up_spi_common_module
	url = git@gitlab.example.com:hardware/dsp/common-modules.git
[submodule "bootloader"]
	path = bootloader
	url = git@gitlab.example.com:firmware/cloud-bootloader.git
[submodule "libs/core-module"]
	path = libs/core-module
	url = git@gitlab.example.com:engineering/embedded/firmware/module-v3.git
[submodule ".gitlab/gitlab-templates"]
	path = .gitlab/gitlab-templates
	url = git@gitlab.example.com:engineering/docs/gitlab-templates.git
`

// parseGitmodules unit tests.

// testResolver resolves as an instance at https://gitlab.example.com whose
// SSH clone URLs start with git@gitlab.example.com:, for the superproject
// group/app.
func testResolver(t *testing.T) submoduleResolver {
	t.Helper()
	base, err := url.Parse("https://gitlab.example.com/api/v4/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	return newResolver(base, "group/app", "git@gitlab.example.com:")
}

// parseAndResolve parses content and resolves it with testResolver, as List
// does once it has read the superproject.
func parseAndResolve(t *testing.T, content string) []SubmoduleEntry {
	t.Helper()
	entries := parseGitmodules(content)
	testResolver(t).resolveAll(entries)
	return entries
}

// TestSubmoduleResolver_ResolvesTheProjectTheRemoteNames holds the resolver
// to the project a remote names on this instance, case by case: the
// superproject itself, relative URLs and where they may land, the instance's
// web URL with its relative root, its SSH clone URL in both of the forms git
// reads, and every remote that names no project here.
//
// It follows GitLab's SubmoduleHelper except where that helper compares text
// and git does not: a namespace whose name only begins with the
// superproject's ("../appx/lib" from group/app) is a sibling, not a path
// inside the superproject, and the ssh:// spelling of an SCP clone URL, a
// host written in capitals or the default port written out all name the same
// instance (see "SubmoduleHelper compares submodule URLs as text" in
// docs/development/upstream-bugs.md).
func TestSubmoduleResolver_ResolvesTheProjectTheRemoteNames(t *testing.T) {
	base, err := url.Parse("https://example.com/gitlab/api/v4/")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	r := newResolver(base, "group/app", "git@example.com:")
	cases := []struct{ remote, want string }{
		{".", "group/app"},
		{"./", "group/app"},
		{"../lib", "group/lib"},
		{"../lib.git", "group/lib"},
		{"../sub/lib.git", "group/sub/lib"},
		{"../../other/lib.git", "other/lib"},
		{"../app-tools", "group/app-tools"},
		{"../app-tools/lib.git", "group/app-tools/lib"},
		{"../appx/lib", "group/appx/lib"},
		{"./nested", ""},
		{"./nested/lib", ""},
		{"../app/nested", ""},
		{"../app/sub/lib.git", ""},
		{"../../lib", ""},
		{"../../../../lib", ""},
		{"../lib/", ""},
		{"https://example.com/gitlab/team/lib.git", "team/lib"},
		{"https://EXAMPLE.com:443/gitlab/team/lib.git", "team/lib"},
		{"https://example.com/gitlab/a/b/lib", "a/b/lib"},
		{"https://example.com/gitlab/lib.git", ""},
		{"https://example.com/gitlab/team/lib.git/", ""},
		{"https://example.com/team/lib.git", ""},
		{"git@example.com:team/lib.git", "team/lib"},
		{"git@EXAMPLE.com:team/lib.git", "team/lib"},
		{"git@example.com:/team/lib.git", ""},
		{"git@example.com:lib.git", ""},
		{"ssh://git@example.com/team/lib.git", "team/lib"},
		{"ssh://git@Example.com:22/team/lib.git", "team/lib"},
		{"git+ssh://git@example.com/team/lib.git", "team/lib"},
		{"ssh://git@example.com//team/lib.git", ""},
		{"ssh://git@example.com:2222/team/lib.git", ""},
		{"ssh://deploy@example.com/team/lib.git", ""},
		{"deploy@example.com:team/lib.git", ""},
		{"https://github.com/rev-misc/lib.git", ""},
		{"git@github.com:rev-misc/lib.git", ""},
		{"lib", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.remote, func(t *testing.T) {
			if got := r.resolve(c.remote); got != c.want {
				t.Errorf("resolve(%q) = %q, want %q", c.remote, got, c.want)
			}
		})
	}
	t.Run("no ssh prefix known", func(t *testing.T) {
		unknown := newResolver(base, "group/app", "")
		if got := unknown.resolve("git@example.com:team/lib.git"); got != "" {
			t.Errorf("resolve with no SSH prefix = %q, want nothing", got)
		}
		for _, remote := range []string{"git@example.com:team/lib.git", "ssh://example.com/team/lib.git", "lib", ""} {
			t.Run(remote, func(t *testing.T) {
				if below, ok := unknown.pathBelowSSH(remote); ok || below != "" {
					t.Errorf("pathBelowSSH(%q) with no SSH prefix = %q, %v; want nothing, false", remote, below, ok)
				}
			})
		}
	})
	t.Run("an instance serving SSH on its own port", func(t *testing.T) {
		port := newResolver(base, "group/app", "ssh://git@example.com:2222/scm/")
		for remote, want := range map[string]string{
			"ssh://git@example.com:2222/scm/team/lib.git": "team/lib",
			"ssh://git@example.com:2222/team/lib.git":     "",
			"git@example.com:team/lib.git":                "",
			"ssh://git@example.com/scm/team/lib.git":      "",
		} {
			t.Run(remote, func(t *testing.T) {
				if got := port.resolve(remote); got != want {
					t.Errorf("resolve(%q) = %q, want %q", remote, got, want)
				}
			})
		}
	})
}

// TestParseSSHRemote_ReadsBothFormsGitTakes verifies the reading of an SSH
// remote the resolver compares with the instance's SSH clone URLs: the SCP
// form with its implicit port, the URL form under each scheme git accepts for
// it with its port written or not, and every remote that is not an SSH one,
// including a URL that does not parse.
func TestParseSSHRemote_ReadsBothFormsGitTakes(t *testing.T) {
	cases := []struct {
		remote string
		want   sshRemote
		ok     bool
	}{
		{"git@Example.com:team/lib.git", sshRemote{"git", "example.com", "22", "team/lib.git"}, true},
		{"example.com:team/lib.git", sshRemote{"", "example.com", "22", "team/lib.git"}, true},
		{"git@example.com:", sshRemote{"git", "example.com", "22", ""}, true},
		{"ssh://git@example.com:2222/team/lib.git", sshRemote{"git", "example.com", "2222", "team/lib.git"}, true},
		{"ssh://git@example.com:2222/", sshRemote{"git", "example.com", "2222", ""}, true},
		{"git+ssh://git@example.com/team/lib.git", sshRemote{"git", "example.com", "22", "team/lib.git"}, true},
		{"ssh+git://ciuser:pw@example.com/team/lib.git", sshRemote{"ciuser", "example.com", "22", "team/lib.git"}, true},
		{"ssh://git@exa mple.com/team/lib.git", sshRemote{}, false},
		{"https://example.com/team/lib.git", sshRemote{}, false},
		{"lib", sshRemote{}, false},
		{":team/lib.git", sshRemote{}, false},
		{"dir/sub:lib", sshRemote{}, false},
		{"", sshRemote{}, false},
	}
	for _, c := range cases {
		t.Run(c.remote, func(t *testing.T) {
			got, ok := parseSSHRemote(c.remote)
			if got != c.want || ok != c.ok {
				t.Errorf("parseSSHRemote(%q) = %+v, %v; want %+v, %v", c.remote, got, ok, c.want, c.ok)
			}
		})
	}
}

// TestSubmoduleResolver_ResolveAll_TellsAnotherHostFromARemoteThatNamesNothing
// verifies that resolveAll marks a remote that names no project on this
// instance one of two ways, and clears the remote each was read from, which
// may carry credentials: external when the remote names another host, where
// the submodule can be read from that host, and unresolved otherwise, a
// relative URL landing inside the superproject or at the root, a remote on
// this instance's host in a form that is not a project's clone URL, or one
// naming no host at all. Every one of them used to be called "external, not
// on this instance", which three of them are not.
func TestSubmoduleResolver_ResolveAll_TellsAnotherHostFromARemoteThatNamesNothing(t *testing.T) {
	cases := []struct {
		remote               string
		external, unresolved bool
	}{
		{"https://u:pw@github.com/x/y.git", true, false},
		{"git@github.com:x/y.git", true, false},
		{"ssh://git@github.com/x/y.git", true, false},
		{"../lib", false, false},
		{"./nested", false, true},
		{"../../lib", false, true},
		{"ssh://git@gitlab.example.com:2222/team/lib.git", false, true},
		{"https://GitLab.example.com:8443/team/lib.git", false, true},
		{"ciuser:s3cretpw@gitlab.example.com:group/project.git", false, true},
		{"lib", false, true},
		{"file:///srv/git/lib.git", false, true},
		{"://invalid", false, true},
	}
	for _, c := range cases {
		t.Run(c.remote, func(t *testing.T) {
			entries := []SubmoduleEntry{{Path: "a", remote: c.remote}}
			testResolver(t).resolveAll(entries)
			e := entries[0]
			if e.External != c.external || e.Unresolved != c.unresolved {
				t.Errorf("entry = %+v, want external %v and unresolved %v", e, c.external, c.unresolved)
			}
			if (e.ResolvedProject != "") == (c.external || c.unresolved) {
				t.Errorf("entry = %+v, want a resolved project exactly when neither mark is set", e)
			}
			if e.remote != "" {
				t.Errorf("entry kept its remote %q", e.remote)
			}
		})
	}
	t.Run("the host of the instance's SSH clone URLs", func(t *testing.T) {
		base, err := url.Parse("https://gitlab.example.com/api/v4/")
		if err != nil {
			t.Fatalf("url.Parse: %v", err)
		}
		entries := []SubmoduleEntry{{Path: "a", remote: "ssh://git@ssh.example.com:2222/team/lib.git"}}
		newResolver(base, "group/app", "git@ssh.example.com:").resolveAll(entries)
		if entries[0].External || !entries[0].Unresolved {
			t.Errorf("entry = %+v, want unresolved on the instance's SSH host", entries[0])
		}
	})
}

// TestSSHPathPrefix_IsWhatPrecedesTheProjectPath verifies the SSH prefix is
// read off the superproject's own clone URL, in either of the forms GitLab
// writes, and that nothing is read off a clone URL that does not end in the
// project's path.
func TestSSHPathPrefix_IsWhatPrecedesTheProjectPath(t *testing.T) {
	cases := []struct{ clone, project, want string }{
		{"git@example.com:group/app.git", "group/app", "git@example.com:"},
		{"ssh://git@example.com:2222/group/app.git", "group/app", "ssh://git@example.com:2222/"},
		{"git@example.com:other/app.git", "group/app", ""},
		{"", "group/app", ""},
		{"group/app.git", "group/app", ""},
	}
	for _, c := range cases {
		t.Run(c.clone, func(t *testing.T) {
			if got := sshPathPrefix(c.clone, c.project); got != c.want {
				t.Errorf("sshPathPrefix(%q, %q) = %q, want %q", c.clone, c.project, got, c.want)
			}
		})
	}
}

// TestList_SuperprojectReadFails_IsAnError verifies that List reports the
// superproject read GitLab refused, rather than resolving nothing.
func TestList_SuperprojectReadFails_IsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, gitmodulesHandler(sampleGitmodules))
	_, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err == nil || !strings.Contains(err.Error(), "could not read project 42") {
		t.Fatalf("List error = %v, want the superproject read refused", err)
	}
}

// TestRead_SuperprojectReadFails_IsAnError verifies the same of Read.
func TestRead_SuperprojectReadFails_IsAnError(t *testing.T) {
	client := testutil.NewTestClient(t, gitmodulesHandler(sampleGitmodules))
	_, err := Read(t.Context(), client, ReadInput{ProjectID: "42", SubmodulePath: "bootloader", FilePath: "f"})
	if err == nil || !strings.Contains(err.Error(), "could not read project 42") {
		t.Fatalf("Read error = %v, want the superproject read refused", err)
	}
}

// TestParseGitmodules_Success covers ParseGitmodules with table-driven subtests for success.
func TestParseGitmodules_Success(t *testing.T) {
	entries := parseAndResolve(t, sampleGitmodules)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	tests := []struct {
		name            string
		path            string
		url             string
		resolvedProject string
	}{
		{"dsp_up_spi_common_module", "dsp_up_spi_common_module", "git@gitlab.example.com:hardware/dsp/common-modules.git", "hardware/dsp/common-modules"},
		{"bootloader", "bootloader", "git@gitlab.example.com:firmware/cloud-bootloader.git", "firmware/cloud-bootloader"},
		{"libs/core-module", "libs/core-module", "git@gitlab.example.com:engineering/embedded/firmware/module-v3.git", "engineering/embedded/firmware/module-v3"},
		{".gitlab/gitlab-templates", ".gitlab/gitlab-templates", "git@gitlab.example.com:engineering/docs/gitlab-templates.git", "engineering/docs/gitlab-templates"},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entries[i]
			if e.Name != tt.name {
				t.Errorf("entry[%d] name: got %q, want %q", i, e.Name, tt.name)
			}
			if e.Path != tt.path {
				t.Errorf("entry[%d] path: got %q, want %q", i, e.Path, tt.path)
			}
			if e.URL != tt.url {
				t.Errorf("entry[%d] url: got %q, want %q", i, e.URL, tt.url)
			}
			if e.ResolvedProject != tt.resolvedProject {
				t.Errorf("entry[%d] resolved: got %q, want %q", i, e.ResolvedProject, tt.resolvedProject)
			}
		})
	}
}

// TestParseGitmodules_Empty verifies ParseGitmodules when empty.
func TestParseGitmodules_Empty(t *testing.T) {
	entries := parseGitmodules("")
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// TestParseGitmodules_HTTPSUrl verifies that an https remote on the
// instance's web URL resolves to the project below it.
func TestParseGitmodules_HTTPSUrl(t *testing.T) {
	content := `[submodule "lib"]
	path = lib
	url = https://gitlab.example.com/group/project.git
`
	entries := parseAndResolve(t, content)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ResolvedProject != "group/project" || entries[0].External {
		t.Errorf("entry = %+v, want group/project on this instance", entries[0])
	}
}

// TestParseGitmodules_SSHUrl verifies that an SSH remote on this instance's
// SSH account, host and port resolves in either form git reads: the SCP form
// the superproject's clone URL shows, and the ssh:// spelling of it, which
// GitLab's own helper leaves unlinked because it compares the text.
func TestParseGitmodules_SSHUrl(t *testing.T) {
	content := `[submodule "scp"]
	path = scp
	url = git@gitlab.example.com:group/subgroup/project.git
[submodule "scheme"]
	path = scheme
	url = ssh://git@gitlab.example.com/group/subgroup/project.git
`
	entries := parseAndResolve(t, content)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, e := range entries {
		if e.ResolvedProject != "group/subgroup/project" || e.External || e.Unresolved {
			t.Errorf("%s entry = %+v, want group/subgroup/project on this instance", e.Path, e)
		}
	}
}

// TestParseGitmodules_RemoteWithCredentials_PublishesNeither verifies that a
// submodule remote carrying a password reaches neither the resolved project
// path nor the url field the tool publishes.
//
// It matters because .gitmodules is part of the repository, so whoever can
// push chooses its text, and the old resolver took its SCP branch on the
// presence of an "@" and a ":" -- which every credentialed https remote also
// has. "https://user:password@host/group/project.git" split on the colon of
// its own scheme and resolved to "/user:password@host/group/project", which
// the submodule table then printed as the project name.
func TestParseGitmodules_RemoteWithCredentials_PublishesNeither(t *testing.T) {
	cases := []struct {
		name         string
		remote       string
		wantResolved string
		wantURL      string
	}{
		{
			name:         "https with password",
			remote:       "https://ciuser:s3cretpw@gitlab.example.com/group/project.git",
			wantResolved: "group/project",
			wantURL:      "https://gitlab.example.com/group/project.git",
		},
		{
			name:         "https with username only",
			remote:       "https://ciuser@gitlab.example.com/group/project.git",
			wantResolved: "group/project",
			wantURL:      "https://gitlab.example.com/group/project.git",
		},
		{
			// Neither of these is the SSH account the instance's clone URLs
			// name, so neither reaches GitLab's shell and nothing resolves.
			name:         "ssh scheme with password",
			remote:       "ssh://ciuser:s3cretpw@gitlab.example.com/group/project.git",
			wantResolved: "",
			wantURL:      "ssh://gitlab.example.com/group/project.git",
		},
		{
			name:         "scp style with password",
			remote:       "ciuser:s3cretpw@gitlab.example.com:group/project.git",
			wantResolved: "",
			wantURL:      "gitlab.example.com:group/project.git",
		},
		{
			name:         "scp style ssh account is kept",
			remote:       "git@gitlab.example.com:group/project.git",
			wantResolved: "group/project",
			wantURL:      "git@gitlab.example.com:group/project.git",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			entries := parseAndResolve(t, "[submodule \"lib\"]\n\tpath = lib\n\turl = "+tc.remote+"\n")
			if len(entries) != 1 {
				t.Fatalf("expected 1 entry, got %d", len(entries))
			}
			got := entries[0]
			if got.ResolvedProject != tc.wantResolved {
				t.Errorf("ResolvedProject = %q, want %q", got.ResolvedProject, tc.wantResolved)
			}
			if got.URL != tc.wantURL {
				t.Errorf("URL = %q, want %q", got.URL, tc.wantURL)
			}
			for _, secret := range []string{"s3cretpw", "ciuser"} {
				if strings.Contains(got.ResolvedProject, secret) {
					t.Errorf("ResolvedProject %q leaked %q", got.ResolvedProject, secret)
				}
				if strings.Contains(got.URL, secret) {
					t.Errorf("URL %q leaked %q", got.URL, secret)
				}
			}
		})
	}
}

// TestFormatListMarkdown_RemoteWithCredentials_RendersNoPassword pins the
// whole rendered submodule table for a .gitmodules remote carrying a user name
// and a password, which is the text a model reads: neither half of the
// credential appears anywhere in it.
//
// The expectation is the whole render rather than two "does not contain"
// checks, because an absence assertion passes just as happily over a table
// that stopped rendering: this one fails if any byte of the row moves.
func TestFormatListMarkdown_RemoteWithCredentials_RendersNoPassword(t *testing.T) {
	entries := parseAndResolve(t, "[submodule \"lib\"]\n\tpath = lib\n\turl = https://ciuser:s3cretpw@gitlab.example.com/group/project.git\n")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := renderedText(t, FormatListMarkdown(ListOutput{Submodules: entries, Count: len(entries)}))

	want := "## Repository Submodules (1)\n\n" +
		"| Name | Path | Commit SHA | Resolved Project |\n" +
		"| --- | --- | --- | --- |\n" +
		"| lib | `lib` |  | group/project |\n" +
		submoduleListHints

	if got != want {
		t.Errorf("table mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
	for _, secret := range []string{"s3cretpw", "ciuser"} {
		t.Run("withholds "+secret, func(t *testing.T) {
			if strings.Contains(got, secret) {
				t.Errorf("submodule table leaked %q:\n%s", secret, got)
			}
		})
	}
}

// parentDir unit tests.

// TestParentDir covers ParentDir with table-driven subtests.
func TestParentDir(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"libs/core-module", "libs"},
		{".gitlab/gitlab-templates", ".gitlab"},
		{"bootloader", ""},
		{"a/b/c", "a/b"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := parentDir(tt.path)
			if got != tt.want {
				t.Errorf("parentDir(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// List integration tests.

// superprojectJSON is the superproject every submodule test's mock answers
// GET /projects/42 with: group/app, whose SSH clone URL shows the prefix
// this instance writes before a project path.
const superprojectJSON = `{"id":42,"path_with_namespace":"group/app","ssh_url_to_repo":"git@gitlab.example.com:group/app.git","http_url_to_repo":"https://gitlab.example.com/group/app.git"}`

// withSuperproject answers GET /api/v4/projects/42, the superproject the
// submodule resolver reads, and hands every other request to next.
func withSuperproject(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects/42" {
			testutil.RespondJSON(w, http.StatusOK, superprojectJSON)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// newSubmoduleClient is testutil.NewTestClient with the superproject read
// answered, which every List and Read that finds a submodule makes.
func newSubmoduleClient(t *testing.T, handler http.Handler) *gitlabclient.Client {
	t.Helper()
	return testutil.NewTestClient(t, withSuperproject(handler))
}

// gitmodulesHandler answers the .gitmodules read with content and every tree
// listing with no entry.
func gitmodulesHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/repository/files/.gitmodules"):
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{"file_name":".gitmodules","file_path":".gitmodules","encoding":"text","content":%q,"ref":"main"}`, content))
		case strings.Contains(r.URL.Path, "/repository/tree"):
			testutil.RespondJSON(w, http.StatusOK, `[]`)
		default:
			http.NotFound(w, r)
		}
	}
}

// TestList_SubmoduleURLs_ResolveToTheProjectTheyName verifies that List names
// a project on this instance only for a submodule whose remote names one: a
// relative URL resolves against the superproject group/app, a sibling
// namespace whose name only begins with "app" included, and an SCP or ssh://
// remote on this instance keeps its path, while a remote on another host is
// marked external and a relative URL inside the superproject unresolved. The
// resolver used to discard the host and keep the path, so github.com's
// rev-misc/lib was published as the local project rev-misc/lib, and "../lib"
// as itself.
func TestList_SubmoduleURLs_ResolveToTheProjectTheyName(t *testing.T) {
	const content = `[submodule "github"]
	path = github
	url = https://github.com/rev-misc/lib.git
[submodule "github-scp"]
	path = github-scp
	url = git@github.com:rev-misc/lib.git
[submodule "sibling"]
	path = sibling
	url = ../lib.git
[submodule "sibling-namespace"]
	path = sibling-namespace
	url = ../app-tools/lib.git
[submodule "cousin"]
	path = cousin
	url = ../../other/tools
[submodule "inside"]
	path = inside
	url = ./nested
[submodule "local"]
	path = local
	url = git@gitlab.example.com:team/lib.git
[submodule "local-ssh"]
	path = local-ssh
	url = ssh://git@gitlab.example.com/team/tools.git
`
	client := newSubmoduleClient(t, gitmodulesHandler(content))
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	type want struct {
		project              string
		external, unresolved bool
	}
	wants := map[string]want{
		"github": {external: true}, "github-scp": {external: true},
		"sibling": {project: "group/lib"}, "sibling-namespace": {project: "group/app-tools/lib"},
		"cousin": {project: "other/tools"}, "inside": {unresolved: true},
		"local": {project: "team/lib"}, "local-ssh": {project: "team/tools"},
	}
	if len(out.Submodules) != len(wants) {
		t.Fatalf("listed %d submodules, want %d", len(out.Submodules), len(wants))
	}
	for _, s := range out.Submodules {
		t.Run(s.Path, func(t *testing.T) {
			if got := (want{s.ResolvedProject, s.External, s.Unresolved}); got != wants[s.Path] {
				t.Errorf("resolved %+v, want %+v", got, wants[s.Path])
			}
		})
	}
}

// TestList_Success verifies List when success.
func TestList_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// .gitmodules request
		if strings.Contains(path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"file_path": ".gitmodules",
				"size": 200,
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, sampleGitmodules))
			return
		}

		// Tree requests: return commit-type entries
		if strings.Contains(path, "/repository/tree") {
			treePath := r.URL.Query().Get("path")
			switch treePath {
			case "libs":
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "aaa111", "name": "core-module", "type": "commit", "path": "libs/core-module", "mode": "160000"}
				]`)
			case ".gitlab":
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "bbb222", "name": "gitlab-templates", "type": "commit", "path": ".gitlab/gitlab-templates", "mode": "160000"}
				]`)
			default:
				// Root-level submodules
				testutil.RespondJSON(w, http.StatusOK, `[
					{"id": "ccc333", "name": "dsp_up_spi_common_module", "type": "commit", "path": "dsp_up_spi_common_module", "mode": "160000"},
					{"id": "ddd444", "name": "bootloader", "type": "commit", "path": "bootloader", "mode": "160000"},
					{"id": "eee555", "name": "src", "type": "tree", "path": "src", "mode": "040000"}
				]`)
			}
			return
		}

		http.NotFound(w, r)
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}

	// Verify commit SHAs were enriched
	shaMap := make(map[string]string)
	for _, s := range out.Submodules {
		shaMap[s.Path] = s.CommitSHA
	}
	if shaMap["dsp_up_spi_common_module"] != "ccc333" {
		t.Errorf("dsp_up_spi_common_module SHA: got %q", shaMap["dsp_up_spi_common_module"])
	}
	if shaMap["bootloader"] != "ddd444" {
		t.Errorf("bootloader SHA: got %q", shaMap["bootloader"])
	}
	if shaMap["libs/core-module"] != "aaa111" {
		t.Errorf("libs/core-module SHA: got %q", shaMap["libs/core-module"])
	}
	if shaMap[".gitlab/gitlab-templates"] != "bbb222" {
		t.Errorf(".gitlab/gitlab-templates SHA: got %q", shaMap[".gitlab/gitlab-templates"])
	}
}

// TestList_NoGitmodules verifies List when no gitmodules.
func TestList_NoGitmodules(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"404 File Not Found"}`)
	})

	client := newSubmoduleClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for missing .gitmodules, got nil")
	}
}

// TestList_EmptyProjectID verifies List when empty project ID.
func TestList_EmptyProjectID(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	_, err := List(t.Context(), client, ListInput{})
	if err == nil {
		t.Fatal("expected error for empty project_id")
	}
}

// TestList_CancelledContext verifies List when cancelled context.
func TestList_CancelledContext(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testutil.RespondJSON(w, http.StatusOK, `{}`)
	}))
	ctx := testutil.CancelledCtx(t)
	_, err := List(ctx, client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// FormatListMarkdown tests.

// TestFormatListMarkdown_SubmoduleNamingNoProject_SaysWhy pins the rows of
// the submodules whose remote names no project on this instance: the project
// cell says which of the two it is instead of standing empty, which read the
// same as a project nobody had resolved yet. A submodule on another host is
// external; one whose remote is on this instance, or names no host, and
// still names no project is unresolved, and is not said to be elsewhere.
func TestFormatListMarkdown_SubmoduleNamingNoProject_SaysWhy(t *testing.T) {
	entries := parseAndResolve(t, "[submodule \"lib\"]\n\tpath = lib\n\turl = https://github.com/rev-misc/lib.git\n"+
		"[submodule \"inner\"]\n\tpath = inner\n\turl = ./inner\n")
	got := renderedText(t, FormatListMarkdown(ListOutput{Submodules: entries, Count: len(entries)}))
	want := "## Repository Submodules (2)\n\n" +
		"| Name | Path | Commit SHA | Resolved Project |\n" +
		"| --- | --- | --- | --- |\n" +
		"| lib | `lib` |  | external, on another host |\n" +
		"| inner | `inner` |  | unresolved, names no project here |\n" +
		submoduleListHints
	if got != want {
		t.Errorf("table mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_WithEntries pins the whole submodule table: the path
// and the abbreviated commit are code spans a reader copies, and a pipe typed
// into a .gitmodules name stays inside its own cell.
func TestFormatListMarkdown_WithEntries(t *testing.T) {
	got := renderedText(t, FormatListMarkdown(ListOutput{
		Submodules: []SubmoduleEntry{
			{Name: "lib", Path: "lib", CommitSHA: "abc123def456", ResolvedProject: "group/lib"},
			{Name: "a | b", Path: "vendor/x", CommitSHA: "0123456", ResolvedProject: "group/x"},
		},
		Count: 2,
	}))

	want := "## Repository Submodules (2)\n\n" +
		"| Name | Path | Commit SHA | Resolved Project |\n" +
		"| --- | --- | --- | --- |\n" +
		"| lib | `lib` | `abc123de` | group/lib |\n" +
		"| a &#124; b | `vendor/x` | `0123456` | group/x |\n" +
		submoduleListHints

	if got != want {
		t.Errorf("table mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestFormatListMarkdown_Empty pins the whole response of a repository with no
// submodules: one sentence, and no heading counting zero above it.
func TestFormatListMarkdown_Empty(t *testing.T) {
	got := renderedText(t, FormatListMarkdown(ListOutput{Submodules: []SubmoduleEntry{}, Count: 0}))

	want := "No submodules found.\n"

	if got != want {
		t.Errorf("empty list mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// ActionSpecs includes all repository submodule tools.

// TestActionSpecs_IncludesTools verifies that ActionSpecs exposes all repository submodule tools.
func TestActionSpecs_IncludesTools(t *testing.T) {
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	byTool := repositorySubmoduleSpecsByTool(t, ActionSpecs(client))

	for _, name := range []string{
		"gitlab_list_repository_submodules",
		"gitlab_read_repository_submodule_file",
		"gitlab_update_repository_submodule",
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := byTool[name]; !ok {
				t.Errorf("expected tool %q in ActionSpecs", name)
			}
		})
	}
}

// TestList_Base64EncodedWithRef verifies that [List] correctly decodes
// base64-encoded .gitmodules content and passes the Ref parameter to the
// GitLab API (covers the base64 decode + Ref branches in List).
func TestList_Base64EncodedWithRef(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(sampleGitmodules))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			// Verify ref parameter is passed
			if r.URL.Query().Get("ref") != "develop" {
				t.Errorf("expected ref=develop, got ref=%s", r.URL.Query().Get("ref"))
			}
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": %q,
				"ref": "develop"
			}`, encoded))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusOK, `[]`)
			return
		}
		http.NotFound(w, r)
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42", Ref: "develop"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}
}

// TestList_EmptyGitmodulesAfterBase64 verifies that [List] returns an empty
// result when .gitmodules is base64-encoded but contains no submodule sections.
func TestList_EmptyGitmodulesAfterBase64(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("# empty file\n"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": %q,
				"ref": "main"
			}`, encoded))
			return
		}
		http.NotFound(w, r)
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 0 {
		t.Errorf("expected 0 submodules, got %d", out.Count)
	}
}

// TestList_Base64DecodeError verifies that [List] returns a meaningful error
// when the base64 content in .gitmodules is malformed.
func TestList_Base64DecodeError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": ".gitmodules",
				"encoding": "base64",
				"content": "!!!invalid-base64!!!",
				"ref": "main"
			}`)
			return
		}
		http.NotFound(w, r)
	})

	client := newSubmoduleClient(t, handler)
	_, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err == nil {
		t.Fatal("expected error for invalid base64 content")
	}
	if !strings.Contains(err.Error(), "decode .gitmodules") {
		t.Errorf("error = %v, want 'decode .gitmodules' message", err)
	}
}

// TestList_TreeRequestError verifies that [enrichSubmoduleCommitSHAs] handles
// a failed tree request gracefully (entries still returned, just without SHAs).
func TestList_TreeRequestError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules",
				"encoding": "text",
				"content": %q,
				"ref": "main"
			}`, sampleGitmodules))
			return
		}
		if strings.Contains(r.URL.Path, "/repository/tree") {
			testutil.RespondJSON(w, http.StatusForbidden, `{"message":"internal error"}`)
			return
		}
		http.NotFound(w, r)
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Entries returned but without SHAs
	if out.Count != 4 {
		t.Errorf("expected 4 submodules, got %d", out.Count)
	}
}

// TestList_TreeListing_AsksEachParentDirectoryAtTheRequestedRef pins the two
// query parameters the commit-SHA lookup depends on: the tree is listed once
// per parent directory a submodule sits in, and every listing names the ref the
// caller asked for.
//
// Both matter and neither was asserted. The listing is not recursive, so a
// request that lost its path answers with the repository root and a submodule
// under libs/ is never found; a request that lost its ref answers at the
// default branch, so the SHAs reported are not the ones the caller's ref pins.
func TestList_TreeListing_AsksEachParentDirectoryAtTheRequestedRef(t *testing.T) {
	var (
		mu    sync.Mutex
		asked []string
		refs  []string
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, fmt.Sprintf(`{
				"file_name": ".gitmodules", "encoding": "text", "content": %q, "ref": "release-3-1"
			}`, sampleGitmodules))
			return
		}
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("path"))
		refs = append(refs, r.URL.Query().Get("ref"))
		mu.Unlock()
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := newSubmoduleClient(t, handler)
	if _, err := List(t.Context(), client, ListInput{ProjectID: "42", Ref: "release-3-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	slices.Sort(asked)
	want := []string{"", ".gitlab", "libs"}
	if !slices.Equal(asked, want) {
		t.Errorf("tree listings asked for paths %q, want %q", asked, want)
	}
	for i, ref := range refs {
		if ref != "release-3-1" {
			t.Errorf("tree listing %d asked at ref %q, want release-3-1", i, ref)
		}
	}
}

// TestList_TreeNodeThatIsNotACommit_LeavesTheSubmoduleUnpinned verifies that a
// submodule keeps an empty commit SHA when the node at its path is an ordinary
// file, and that a commit node at some other path is not attributed to it.
//
// A submodule pointer is a tree node of type "commit" (mode 160000) at the
// submodule's own path, and both halves of that are load-bearing: publishing a
// blob's object id as the pinned commit would send a reader looking up a SHA
// that names a file, and publishing another path's would name the wrong commit
// with no sign that anything is wrong.
func TestList_TreeNodeThatIsNotACommit_LeavesTheSubmoduleUnpinned(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, `{
				"file_name": ".gitmodules", "encoding": "text",
				"content": "[submodule \"lib\"]\n\tpath = lib\n\turl = git@host:group/lib.git\n", "ref": "main"
			}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[
			{"id": "b10bb10b", "name": "lib", "type": "blob", "path": "lib", "mode": "100644"},
			{"id": "f0re1gn0", "name": "other", "type": "commit", "path": "other", "mode": "160000"}
		]`)
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("expected 1 submodule, got %d", out.Count)
	}
	if got := out.Submodules[0].CommitSHA; got != "" {
		t.Errorf("CommitSHA = %q, want empty: the node at lib is a blob and the commit node is another path", got)
	}
}

// oneSubmoduleGitmodules is a .gitmodules naming one submodule at lib.
const oneSubmoduleGitmodules = `{"file_name": ".gitmodules", "encoding": "text",
	"content": "[submodule \"lib\"]\n\tpath = lib\n\turl = git@gitlab.example.com:group/lib.git\n", "ref": "main"}`

// TestList_TreeListing_FollowsEveryPageOfTheDirectory verifies that a submodule
// GitLab lists on the second page of its directory is still found, and that
// the second page is asked for by the number the first announced.
//
// The listing asked for one page of a hundred entries and never looked at the
// pagination headers, so a submodule in a directory with more entries than
// that was reported with no commit, exactly as one that pins nothing.
func TestList_TreeListing_FollowsEveryPageOfTheDirectory(t *testing.T) {
	var (
		mu    sync.Mutex
		pages []string
	)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			testutil.RespondJSON(w, http.StatusOK, oneSubmoduleGitmodules)
			return
		}
		page := r.URL.Query().Get("page")
		mu.Lock()
		pages = append(pages, page)
		mu.Unlock()
		if page == "" {
			testutil.RespondJSONWithPagination(w, http.StatusOK,
				`[{"id": "b10bb10b", "name": "README", "type": "blob", "path": "README", "mode": "100644"}]`,
				testutil.PaginationHeaders{Page: "1", NextPage: "2"})
			return
		}
		testutil.RespondJSONWithPagination(w, http.StatusOK,
			`[{"id": "c0ffee00", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}]`,
			testutil.PaginationHeaders{Page: "2"})
	})

	client := newSubmoduleClient(t, handler)
	out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 1 || out.Submodules[0].CommitSHA != "c0ffee00" {
		t.Errorf("submodules = %+v, want lib pinned at c0ffee00 from the second page", out.Submodules)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"", "2"}; !slices.Equal(pages, want) {
		t.Errorf("tree pages asked = %q, want %q", pages, want)
	}
}

// twoSubmodulesGitmodules is a .gitmodules naming two submodules in the
// repository root, lib and vendor.
const twoSubmodulesGitmodules = `{"file_name": ".gitmodules", "encoding": "text",
	"content": "[submodule \"lib\"]\n\tpath = lib\n\turl = git@host:group/lib.git\n` +
	`[submodule \"vendor\"]\n\tpath = vendor\n\turl = git@host:group/vendor.git\n", "ref": "main"}`

// TestList_TreeListing_StopsAtThePageHoldingTheLastSubmodule verifies that a
// directory is listed only until every submodule it holds has been matched: a
// root of thousands of entries whose submodules sit on its first pages costs
// those pages and not all of them. The first case holds the listing to the
// one page its only submodule is on, although GitLab announces a second; the
// second holds it to reading on while a submodule of the directory is still
// unmatched, one on each page.
func TestList_TreeListing_StopsAtThePageHoldingTheLastSubmodule(t *testing.T) {
	const (
		libNode    = `{"id": "c0ffee00", "name": "lib", "type": "commit", "path": "lib", "mode": "160000"}`
		vendorNode = `{"id": "beef0000", "name": "vendor", "type": "commit", "path": "vendor", "mode": "160000"}`
		blobNode   = `{"id": "b10bb10b", "name": "README", "type": "blob", "path": "README", "mode": "100644"}`
	)
	cases := []struct {
		name       string
		gitmodules string
		firstPage  string
		secondPage string
		wantPages  []string
		wantSHAs   map[string]string
	}{
		{
			name: "one submodule, found on the first page", gitmodules: oneSubmoduleGitmodules,
			firstPage: "[" + libNode + "," + blobNode + "]", secondPage: "[" + blobNode + "]",
			wantPages: []string{""}, wantSHAs: map[string]string{"lib": "c0ffee00"},
		},
		{
			name: "two submodules, one on each page", gitmodules: twoSubmodulesGitmodules,
			firstPage: "[" + libNode + "," + blobNode + "]", secondPage: "[" + vendorNode + "]",
			wantPages: []string{"", "2"}, wantSHAs: map[string]string{"lib": "c0ffee00", "vendor": "beef0000"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mu    sync.Mutex
				pages []string
			)
			client := newSubmoduleClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/repository/files/") {
					testutil.RespondJSON(w, http.StatusOK, tc.gitmodules)
					return
				}
				page := r.URL.Query().Get("page")
				mu.Lock()
				pages = append(pages, page)
				mu.Unlock()
				if page == "" {
					testutil.RespondJSONWithPagination(w, http.StatusOK, tc.firstPage,
						testutil.PaginationHeaders{Page: "1", NextPage: "2"})
					return
				}
				testutil.RespondJSONWithPagination(w, http.StatusOK, tc.secondPage, testutil.PaginationHeaders{Page: "2"})
			}))

			out, err := List(t.Context(), client, ListInput{ProjectID: "42"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got := map[string]string{}
			for _, s := range out.Submodules {
				got[s.Path] = s.CommitSHA
			}
			if !maps.Equal(got, tc.wantSHAs) {
				t.Errorf("pinned commits = %v, want %v", got, tc.wantSHAs)
			}
			mu.Lock()
			defer mu.Unlock()
			if !slices.Equal(pages, tc.wantPages) {
				t.Errorf("tree pages asked = %q, want %q", pages, tc.wantPages)
			}
		})
	}
}

// TestParseGitmodules_ExtraKeysAndMalformedLines_AreIgnored verifies that a
// .gitmodules section carrying keys beyond path and url, and a line with no
// separator at all, leaves the entry those two keys built.
//
// Real files carry branch, shallow and update lines, and the parser reaches
// them through the same switch and the same ok test that the fixtures above
// only ever drive down the path and url arms.
func TestParseGitmodules_ExtraKeysAndMalformedLines_AreIgnored(t *testing.T) {
	entries := parseAndResolve(t, `[submodule "lib"]
	path = lib
	url = git@gitlab.example.com:group/lib.git
	branch = main
	shallow = true
	this line has no separator
`)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	want := SubmoduleEntry{
		Name:            "lib",
		Path:            "lib",
		URL:             "git@gitlab.example.com:group/lib.git",
		ResolvedProject: "group/lib",
	}
	if entries[0] != want {
		t.Errorf("entry = %+v, want %+v", entries[0], want)
	}
}

// TestParseKeyValue_MalformedLine verifies that [parseKeyValue] returns empty
// strings for lines without '=' separator.
func TestParseKeyValue_MalformedLine(t *testing.T) {
	k, v, ok := parseKeyValue("no-equals-sign-here")
	if ok || k != "" || v != "" {
		t.Errorf("parseKeyValue(no =) = (%q, %q, %v), want empty + false", k, v, ok)
	}
}

// TestEnrichSubmoduleCommitSHAs_CancelledContext verifies that an already
// cancelled context leaves every entry unenriched and sends GitLab nothing.
//
// The comment here used to say the loop "stops iterating directories", which
// the body never established: the mock answered every listing with an empty
// tree, so the entries would have come back with no SHA whether the loop ran or
// not. Counting the requests is the part a reader can act on, and it is the
// half that would notice a cancelled call being made anyway.
func TestEnrichSubmoduleCommitSHAs_CancelledContext(t *testing.T) {
	var requests atomic.Int64
	client := testutil.NewTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	}))

	entries := []SubmoduleEntry{
		{Name: "a", Path: "dir1/a"},
		{Name: "b", Path: "dir2/b"},
	}

	ctx := testutil.CancelledCtx(t)

	enrichSubmoduleCommitSHAs(ctx, client, "42", "main", entries)

	if got := requests.Load(); got != 0 {
		t.Errorf("%d tree listings reached GitLab under a cancelled context, want 0", got)
	}
	for _, e := range entries {
		t.Run(e.Name, func(t *testing.T) {
			if e.CommitSHA != "" {
				t.Errorf("expected empty CommitSHA for entry %q after cancelled context, got %q", e.Name, e.CommitSHA)
			}
		})
	}
}

// TestList_DefaultsRefToHEAD verifies List sends the HEAD ref alias when the
// input omits ref, honoring the documented default-branch fallback that the
// repository files API cannot resolve on its own.
func TestList_DefaultsRefToHEAD(t *testing.T) {
	var capturedRef string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/repository/files/") {
			capturedRef = r.URL.Query().Get("ref")
			testutil.RespondJSON(w, http.StatusOK, `{"file_name": ".gitmodules", "encoding": "text", "content": ""}`)
			return
		}
		testutil.RespondJSON(w, http.StatusOK, `[]`)
	})

	client := newSubmoduleClient(t, handler)
	if _, err := List(t.Context(), client, ListInput{ProjectID: "42"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRef != "HEAD" {
		t.Errorf("GetFile ref = %q, want HEAD", capturedRef)
	}
}
