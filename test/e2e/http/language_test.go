// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build httpe2e

// language_test.go drives the real binary in HTTP mode through the language
// setting (issue 1547): a deployment that sets nothing or sets English is
// served what it was served before the setting existed, a language the server
// has no catalog for refuses the start before the listener opens, as it
// refuses a stdio start, and the flag wins over the variable.
package httpe2e

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// listedToolsOverHTTP starts a server publishing a fake GitLab under env and
// flags and returns its stateless tools/list answer.
func listedToolsOverHTTP(t *testing.T, gitlabURL string, env map[string]string, flags ...string) string {
	t.Helper()
	srv := startServer(t, env, append([]string{"--gitlab-url=" + gitlabURL}, flags...)...)
	got := srv.do(t, request{
		body:    toolsListBody,
		headers: map[string]string{"PRIVATE-TOKEN": "glpat-language"},
	})
	if got.status != http.StatusOK {
		t.Fatalf("tools/list = %d: %s", got.status, truncate(got.body))
	}
	return jsonRPCPayload(t, got.body)
}

// TestLanguage_UnsetOrEnglish_IsServedWhatWasServedBeforeOverHTTP pins the
// promise on the transport a hosted deployment uses: unset, blank, en in any
// case and with a region, en through the flag, and en through the flag over a
// variable the server would refuse, each start serves and lists the same
// tools byte for byte.
func TestLanguage_UnsetOrEnglish_IsServedWhatWasServedBeforeOverHTTP(t *testing.T) {
	gitlab := startFakeGitLabServingProjects(t)
	baseline := listedToolsOverHTTP(t, gitlab.URL, nil)

	for _, tc := range []struct {
		name  string
		env   map[string]string
		flags []string
	}{
		{name: "blank", env: map[string]string{"GITLAB_MCP_LANGUAGE": "   "}},
		{name: "en", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}},
		{name: "en in upper case with a region", env: map[string]string{"GITLAB_MCP_LANGUAGE": "EN-GB"}},
		{name: "the flag", flags: []string{"--language=en"}},
		{name: "the flag beats the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "fr"}, flags: []string{"--language=en"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := listedToolsOverHTTP(t, gitlab.URL, tc.env, tc.flags...); got != baseline {
				t.Errorf("tools/list differs from the one served with no language set:\n got: %.300s\nwant: %.300s", got, baseline)
			}
		})
	}
}

// TestLanguage_ALanguageWithoutACatalog_RefusesToStartOverHTTP pins the
// refusal before the listener opens, through the variable and through the
// flag, which writes the variable, with the words a stdio start is refused
// with. The output is the JSON log, where the quotes around the value are
// escaped, so the refusal is matched up to the value.
func TestLanguage_ALanguageWithoutACatalog_RefusesToStartOverHTTP(t *testing.T) {
	base := []string{"--http", "--http-addr=127.0.0.1:0", "--gitlab-url=https://gitlab.example.com"}
	const says = "invalid GITLAB_MCP_LANGUAGE value: expected one of en, got "
	for _, tc := range []struct {
		name  string
		env   map[string]string
		flags []string
	}{
		{name: "the variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "fr"}},
		{name: "the flag", flags: []string{"--language=fr"}},
		{name: "the flag over an English variable", env: map[string]string{"GITLAB_MCP_LANGUAGE": "en"}, flags: []string{"--language=fr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runServerWithEnvExpectingExit(t, serverBinary(t), tc.env, append(slices.Clone(base), tc.flags...)...)
			if err == nil {
				t.Fatalf("the server started and exited cleanly; output:\n%s", out)
			}
			if !strings.Contains(out, says) || !strings.Contains(out, "fr") {
				t.Errorf("the start was not refused with %q; output:\n%s", says, out)
			}
			if strings.Contains(out, "listening on tcp") {
				t.Errorf("the server listened under a language it refuses; output:\n%s", out)
			}
		})
	}
}
