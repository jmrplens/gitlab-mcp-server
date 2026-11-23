// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/netfixture"
)

// TestShown_EachValue_ReadsAsTheScriptPrintedIt verifies how a JSON value
// reads in the report: a string as its text, an escaped one unescaped, and any
// other value, an absent one included, as JSON writes it.
func TestShown_EachValue_ReadsAsTheScriptPrintedIt(t *testing.T) {
	t.Parallel()
	for value, want := range map[string]string{"": "null", "null": "null", `"OK"`: "OK", `"a\"b"`: `a"b`, `""`: "", "98.1": "98.1", "true": "true"} {
		t.Run(value, func(t *testing.T) {
			t.Parallel()
			if got := shown(json.RawMessage(value)); got != want {
				t.Errorf("shown(%s) = %q, want %q", value, got, want)
			}
		})
	}
}

// TestValue_AMeasure_IsReadWhereItIs verifies which of a measure's values is
// the one shown: its own, then its period's, then the first of its periods,
// one that is null read as absent.
func TestValue_AMeasure_IsReadWhereItIs(t *testing.T) {
	t.Parallel()
	for measure, want := range map[string]string{
		`{"value":"1","period":{"value":"2"},"periods":[{"value":"3"}]}`:    "1",
		`{"value":null,"period":{"value":"2"},"periods":[{"value":"3"}]}`:   "2",
		`{"period":{"value":null},"periods":[{"value":"3"},{"value":"4"}]}`: "3",
		`{"periods":[]}`: "null",
		`{}`:             "null",
	} {
		t.Run(measure, func(t *testing.T) {
			t.Parallel()
			var read measured
			if err := json.Unmarshal([]byte(measure), &read); err != nil {
				t.Fatal(err)
			}
			if got := read.value(); got != want {
				t.Errorf("value() of %s = %q, want %q", measure, got, want)
			}
		})
	}
}

// TestReportGate_AnAnswerItCannotRead_IsAnError verifies that a request
// refused at any status from 400 on, an answer that is no JSON, a host that
// does not answer and one that is no address are each an error rather than a
// gate that reads as failed, for the gate and for the measures alike.
func TestReportGate_AnAnswerItCannotRead_IsAnError(t *testing.T) {
	t.Parallel()
	dead := netfixture.DeadEndpoint(t)
	refusing := func(path string, status int, body string) func(b *bench) {
		return func(b *bench) {
			b.answer = func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == path {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(body))
					return
				}
				_, _ = w.Write(b.run.Gate)
			}
		}
	}
	cases := []struct {
		name    string
		prepare func(b *bench)
		want    string
	}{
		{"the gate is refused", refusing("/api/qualitygates/project_status", http.StatusBadRequest, "{}"), "ERROR: /api/qualitygates/project_status answered 400 Bad Request\n"},
		{"the measures are refused", refusing("/api/measures/component", http.StatusForbidden, "{}"), "ERROR: /api/measures/component answered 403 Forbidden\n"},
		{"the gate is no JSON", refusing("/api/qualitygates/project_status", http.StatusOK, "<html>"), "ERROR: /api/qualitygates/project_status answered no JSON this reads: "},
		{"the host does not answer", func(b *bench) { b.session.environ = append(b.session.environ, "SONAR_HOST_URL="+dead) }, "ERROR: Get "},
		{"the host is no address", func(b *bench) { b.session.environ = append(b.session.environ, "SONAR_HOST_URL=\x7f") }, "ERROR: parse "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBench(t, usual(t))
			tc.prepare(b)
			if status := b.session.main(t.Context(), []string{"-no-scan"}); status != 1 || !strings.HasPrefix(b.stderr.String(), tc.want) {
				t.Errorf("status = %d, stderr = %q; want 1 and %q", status, b.stderr.String(), tc.want)
			}
			if b.stdout.String() != "==> [4/4] Fetching quality gate result\n" {
				t.Errorf("stdout = %q, want no report", b.stdout.String())
			}
		})
	}
}
