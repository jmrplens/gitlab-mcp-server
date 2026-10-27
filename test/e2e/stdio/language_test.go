// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build stdioe2e

// language_test.go drives a stdio server through the language setting
// (issue 1547): what a deployment that sets nothing, or sets English, is
// served, what a machine's own locale does to it, and the refusal of a
// language the server has no catalog for, through the variable and through
// the flag that writes it.
package stdioe2e

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"
)

// languageRefusal is what the server refuses a language without a catalog
// with, spelled here because this module cannot import internal/config.
const languageRefusal = `invalid GITLAB_MCP_LANGUAGE value: expected one of en, got "fr"`

// listedTools starts a session under env and args, has it serve one tool call
// that reaches GitLab, and returns the tools/list response as it crossed the
// pipe.
func listedTools(t *testing.T, env map[string]string, args ...string) (string, *session) {
	t.Helper()
	s := startSessionWithArgs(t, env, args...)
	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("the server did not serve a tool call: %v\nstderr: %s", got, s.stderrText())
	}
	line, got := s.callRaw(t, request(2, "tools/list", ""))
	if got["error"] != nil {
		t.Fatalf("tools/list was refused: %v", got)
	}
	return line, s
}

// TestLanguage_UnsetOrEnglish_IsServedWhatWasServedBefore pins the promise
// the setting makes to every deployment that does not use it: unset, blank,
// en in any case and with a region, and en written through the flag, the
// server starts, serves, and lists the same tools byte for byte.
func TestLanguage_UnsetOrEnglish_IsServedWhatWasServedBefore(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	baseline, s := listedTools(t, baseEnv(gitlabURL))
	s.terminate(t, 10*time.Second)

	for _, tc := range []struct {
		name  string
		value string
		args  []string
	}{
		{name: "blank", value: "   "},
		{name: "en", value: "en"},
		{name: "en in upper case with a region", value: "EN-GB"},
		{name: "the flag", args: []string{"--language=en"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(gitlabURL)
			if tc.value != "" {
				env["GITLAB_MCP_LANGUAGE"] = tc.value
			}
			got, configured := listedTools(t, env, tc.args...)
			if got != baseline {
				t.Errorf("tools/list differs from the one served with no language set:\n got: %.300s\nwant: %.300s", got, baseline)
			}
			configured.terminate(t, 10*time.Second)
		})
	}
}

// TestLanguage_AMachinesSpanishLocale_IsNeverRead pins that the language is
// the operator's setting and nothing else. A Linux desktop exports gettext's
// LANGUAGE (es_ES:es) beside LANG, LC_ALL and LC_MESSAGES, and a server
// started there with no GITLAB_MCP_LANGUAGE starts, serves the same tools as
// one started anywhere else, and warns about none of the four, neither as a
// retired spelling nor as anything else: read bare, the gettext list would
// have refused the start. The warnings the fake GitLab causes on every start
// (it answers no scope probe) are not about the locale and are let through.
func TestLanguage_AMachinesSpanishLocale_IsNeverRead(t *testing.T) {
	gitlabURL := startFakeGitLab(t).URL
	baseline, s := listedTools(t, baseEnv(gitlabURL))
	s.terminate(t, 10*time.Second)

	env := baseEnv(gitlabURL)
	env["LANGUAGE"] = "es_ES:es"
	env["LANG"] = "es_ES.UTF-8"
	env["LC_ALL"] = "es_ES.UTF-8"
	env["LC_MESSAGES"] = "es_ES.UTF-8"
	got, s := listedTools(t, env)
	if got != baseline {
		t.Errorf("tools/list under a Spanish locale differs from the one served without it:\n got: %.300s\nwant: %.300s", got, baseline)
	}
	if about := localeWarnings(s.stderrText()); len(about) != 0 {
		t.Errorf("a server under a Spanish locale warned about it:\n%s", strings.Join(about, "\n"))
	}
	s.terminate(t, 10*time.Second)
}

// localeNames matches what a warning about the machine's locale would name:
// one of its four variables, or the value they carry here.
var localeNames = regexp.MustCompile(`LANGUAGE|LANG|LC_ALL|LC_MESSAGES|es_ES`)

// localeWarnings returns the warnings in logs that name the locale.
func localeWarnings(logs string) []string {
	var about []string
	for _, line := range warnedLines(logs) {
		if localeNames.MatchString(line) {
			about = append(about, line)
		}
	}
	return about
}

// warnedLines returns the JSON log records written at WARN or above, each as
// the line it was written as.
func warnedLines(logs string) []string {
	var warned []string
	for line := range strings.SplitSeq(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if level, _ := record["level"].(string); level == "WARN" || level == "ERROR" {
			warned = append(warned, line)
		}
	}
	return warned
}

// TestLanguage_ALanguageWithoutACatalog_RefusesToStart pins the refusal: a
// language the server has no catalog for ends the start at ERROR, naming the
// variable and the languages the server accepts, before anything is written
// on stdout, whether it came from the variable or from the flag that writes
// it.
func TestLanguage_ALanguageWithoutACatalog_RefusesToStart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		args  []string
	}{
		{name: "the variable", value: "fr"},
		{name: "the flag", args: []string{"--language=fr"}},
		{name: "the flag over an English variable", value: "en", args: []string{"--language=fr"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := baseEnv(startFakeGitLab(t).URL)
			if tc.value != "" {
				env["GITLAB_MCP_LANGUAGE"] = tc.value
			}
			s := startSessionWithArgs(t, env, tc.args...)
			code, exited := s.waitExit(t, 20*time.Second)
			if !exited {
				t.Fatalf("a stdio server given a language without a catalog started instead of refusing\nstderr: %s", s.stderrText())
			}
			if code == 0 {
				t.Errorf("the refusal exited 0\nstderr: %s", s.stderrText())
			}
			refused := awaitLogRecord(t, s, "server exited with error", 5*time.Second)
			if level, _ := refused["level"].(string); level != "ERROR" {
				t.Errorf("the refusal is at %s, want ERROR", level)
			}
			if said, _ := refused["error"].(string); !strings.Contains(said, languageRefusal) {
				t.Errorf("the refusal says %q, want it to carry %q", said, languageRefusal)
			}
			if logs := s.stderrText(); strings.Contains(logs, stdioStartedLine) {
				t.Errorf("the refused run started the server:\n%s", logs)
			}
			if b, err := s.stdout.ReadByte(); err == nil {
				t.Errorf("the refused run wrote %q on stdout, which only JSON-RPC may use", b)
			}
		})
	}
}

// TestLanguage_TheFlagBeatsTheVariable pins the precedence the flag shares
// with every setting: a --language the operator typed wins over an exported
// GITLAB_MCP_LANGUAGE, so a value the server would refuse in the environment
// is overridden rather than stopping the start.
func TestLanguage_TheFlagBeatsTheVariable(t *testing.T) {
	env := baseEnv(startFakeGitLab(t).URL)
	env["GITLAB_MCP_LANGUAGE"] = "fr"
	s := startSessionWithArgs(t, env, "--language=en")
	if got := s.call(t, currentUserCall(1)); !served(got) {
		t.Fatalf("a stdio server given --language=en over GITLAB_MCP_LANGUAGE=fr did not serve: %v\nstderr: %s", got, s.stderrText())
	}
	s.terminate(t, 10*time.Second)
}
