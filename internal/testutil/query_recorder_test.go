// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package testutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestRecordQueries_KeepsTheLastQueryAndCountsRequests holds the recorder to
// answering every request with its body, keeping the query of the last one,
// reading a parameter the query did not carry as absent, and counting what
// reached it.
func TestRecordQueries_KeepsTheLastQueryAndCountsRequests(t *testing.T) {
	client, recorder := RecordQueries(t, `[{"id":7,"title":"m"}]`)
	if recorder.Requests() != 0 {
		t.Fatalf("Requests() = %d before any request, want 0", recorder.Requests())
	}
	milestones, _, err := client.GL().Milestones.ListMilestones("42", &gl.ListMilestonesOptions{Search: new("first")}, gl.WithContext(context.Background()))
	if err != nil || len(milestones) != 1 {
		t.Fatalf("ListMilestones() = %+v, %v, want the answer body", milestones, err)
	}
	if _, _, err = client.GL().Milestones.ListMilestones("42", &gl.ListMilestonesOptions{State: new("active")}, gl.WithContext(context.Background())); err != nil {
		t.Fatalf("ListMilestones() error: %v", err)
	}
	if got, ok := recorder.Param("state"); !ok || got != "active" {
		t.Errorf("Param(state) = %q, %v, want the last request's active", got, ok)
	}
	if got, ok := recorder.Param("search"); ok {
		t.Errorf("Param(search) = %q, want it absent from the last request", got)
	}
	if recorder.Requests() != 2 {
		t.Errorf("Requests() = %d, want 2", recorder.Requests())
	}
}

// TestRecordQueries_Path_KeepsTheLastPathAsSent holds Path to reading empty
// before any request and to keeping the path of the last request as it was
// written on the wire: an escaped "/" of a full-path project stays "%2F", and
// the query is left out.
func TestRecordQueries_Path_KeepsTheLastPathAsSent(t *testing.T) {
	client, recorder := RecordQueries(t, `[]`)
	if got := recorder.Path(); got != "" {
		t.Fatalf("Path() = %q before any request, want empty", got)
	}
	if _, _, err := client.GL().Milestones.ListMilestones("1", nil, gl.WithContext(context.Background())); err != nil {
		t.Fatalf("ListMilestones() error: %v", err)
	}
	if _, _, err := client.GL().Milestones.ListMilestones("group/proj", &gl.ListMilestonesOptions{State: new("active")}, gl.WithContext(context.Background())); err != nil {
		t.Fatalf("ListMilestones() error: %v", err)
	}
	if got, want := recorder.Path(), "/api/v4/projects/group%2Fproj/milestones"; got != want {
		t.Errorf("Path() = %q, want the last request's %q", got, want)
	}
}

// TestUserIDFilterCase_Params_CopiesTheBaseAndAddsTheValue holds Params to
// leaving the base map untouched, adding the case's value under the name, and
// adding nothing for a filter left out, an empty base included.
func TestUserIDFilterCase_Params_CopiesTheBaseAndAddsTheValue(t *testing.T) {
	base := map[string]any{"project_id": "42"}
	params := UserIDFilterCase{Value: "None"}.Params(base, "assignee_id")
	if params["assignee_id"] != "None" || params["project_id"] != "42" {
		t.Errorf("Params() = %v, want the base and assignee_id None", params)
	}
	if _, ok := base["assignee_id"]; ok {
		t.Errorf("Params() changed the base: %v", base)
	}
	if params = (UserIDFilterCase{}).Params(nil, "assignee_id"); len(params) != 0 {
		t.Errorf("Params() for a filter left out on no base = %v, want an empty map", params)
	}
	cases := UserIDFilterCases()
	if len(cases) != 5 || !cases[3].Refused || cases[4].Value != nil {
		t.Errorf("UserIDFilterCases() = %+v, want the five cases ending in zero refused and the filter left out", cases)
	}
}

// TestQueryRecorder_AssertUserIDFilter_ReportsEachWayACallMissesTheCase holds
// the assertion to every report it makes and to staying silent when the call
// matches: a refusal that did not happen or does not name None, a refusal
// after a request, an unexpected error, and a query that carries the wrong
// value or none.
func TestQueryRecorder_AssertUserIDFilter_ReportsEachWayACallMissesTheCase(t *testing.T) {
	refused := UserIDFilterCase{Value: 0, Refused: true}
	sent := UserIDFilterCase{Value: 5, Want: "5", Sent: true}
	tests := []struct {
		name     string
		recorder *QueryRecorder
		c        UserIDFilterCase
		err      error
		want     []string
	}{
		{name: "refused before any request", recorder: &QueryRecorder{}, c: refused, err: errors.New("pass None")},
		{name: "not refused", recorder: &QueryRecorder{}, c: refused, want: []string{"want a refusal naming None"}},
		{name: "refused without naming None", recorder: &QueryRecorder{}, c: refused, err: errors.New("bad"), want: []string{"want a refusal naming None"}},
		{name: "refused after a request", recorder: &QueryRecorder{requests: 1}, c: refused, err: errors.New("None"), want: []string{"1 request(s) reached GitLab"}},
		{name: "unexpected error", recorder: &QueryRecorder{}, c: sent, err: errors.New("boom"), want: []string{"unexpected error: boom"}},
		{name: "the value sent", recorder: &QueryRecorder{query: map[string][]string{"assignee_id": {"5"}}}, c: sent},
		{name: "another value", recorder: &QueryRecorder{query: map[string][]string{"assignee_id": {"6"}}}, c: sent, want: []string{`query carries "6" (sent true), want "5" (sent true)`}},
		{name: "nothing sent", recorder: &QueryRecorder{}, c: sent, want: []string{`query carries "" (sent false)`}},
		{name: "an empty parameter", recorder: &QueryRecorder{query: map[string][]string{"assignee_id": {}}}, c: UserIDFilterCase{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reporter := &recordingReporter{}
			tt.recorder.AssertUserIDFilter(reporter, "assignee_id", tt.c, tt.err)
			if len(reporter.messages) != len(tt.want) {
				t.Fatalf("AssertUserIDFilter reported %q, want %d report(s)", reporter.messages, len(tt.want))
			}
			for _, want := range tt.want {
				if !strings.Contains(reporter.joined(), want) {
					t.Errorf("AssertUserIDFilter reported %q, want a report containing %q", reporter.messages, want)
				}
			}
		})
	}
}
