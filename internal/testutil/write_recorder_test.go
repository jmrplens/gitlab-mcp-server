package testutil

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestRecordWrites_KeepsTheLastWriteAndAnswersReadsApart holds the recorder to
// its three promises: a read is answered with the read body and recorded as
// nothing, a write's members are kept exactly as they were encoded, null
// included, and a member the body did not carry reads as absent.
func TestRecordWrites_KeepsTheLastWriteAndAnswersReadsApart(t *testing.T) {
	client, recorder := RecordWrites(t, `[{"id":7,"title":"read"}]`, `{"id":1,"title":"written"}`)

	milestones, _, err := client.GL().Milestones.ListMilestones("42", nil, gl.WithContext(context.Background()))
	if err != nil || len(milestones) != 1 || milestones[0].Title != "read" {
		t.Fatalf("a read answered %+v, %v, want the read body", milestones, err)
	}
	if _, ok := recorder.Member("title"); ok {
		t.Fatal("a read was recorded as a write")
	}

	description := ""
	milestone, _, err := client.GL().Milestones.UpdateMilestone("42", 7, &gl.UpdateMilestoneOptions{Description: &description}, gl.WithContext(context.Background()))
	if err != nil || milestone.Title != "written" {
		t.Fatalf("a write answered %+v, %v, want the write body", milestone, err)
	}
	if got, ok := recorder.Member("description"); !ok || got != `""` {
		t.Errorf("Member(description) = %s, %v, want the empty string as sent", got, ok)
	}
	if got, ok := recorder.Member("title"); ok {
		t.Errorf("Member(title) = %s, want it absent from a body that did not carry it", got)
	}
}

// TestRecordWrites_AHeadIsAnsweredAsARead holds a HEAD request, which a
// handler sends to ask whether something exists, to the read's answer and to
// leaving the last write's members in place, as a GET does.
func TestRecordWrites_AHeadIsAnsweredAsARead(t *testing.T) {
	client, recorder := RecordWrites(t, `{}`, `{}`)
	send := func(method, body string) int {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), method, client.GL().BaseURL().String()+"projects", strings.NewReader(body))
		if err != nil {
			t.Fatalf("build the %s request: %v", method, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send the %s request: %v", method, err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	send(http.MethodPost, `{"name":"written"}`)
	if status := send(http.MethodHead, `{"name":"probed"}`); status != http.StatusOK {
		t.Errorf("a HEAD was answered %d, want the read's 200", status)
	}
	if got, ok := recorder.Member("name"); !ok || got != `"written"` {
		t.Errorf("Member(name) = %s, %v after a HEAD, want the write's name kept", got, ok)
	}
}

// TestWriteRecorder_AssertMembers_ReportsEachMismatchAndNothingElse holds the
// assertion to reporting a member encoded differently and a member the body did
// not carry, and to staying silent on a member that matches.
func TestWriteRecorder_AssertMembers_ReportsEachMismatchAndNothingElse(t *testing.T) {
	recorder := &WriteRecorder{members: map[string]json.RawMessage{"description": json.RawMessage(`""`), "labels": json.RawMessage(`"a"`)}}
	reporter := &recordingReporter{}
	recorder.AssertMembers(reporter, map[string]string{"description": `""`, "labels": `""`, "milestones": `[]`})
	if len(reporter.messages) != 2 {
		t.Fatalf("AssertMembers reported %q, want the labels mismatch and the missing milestones", reporter.messages)
	}
	for _, want := range []string{`labels = "a" (carried: true), want ""`, `milestones =  (carried: false), want []`} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(reporter.joined(), want) {
				t.Errorf("AssertMembers reported %q, want a report containing %q", reporter.messages, want)
			}
		})
	}
}

// TestRecordWrites_ABodyThatIsNotAnObject_KeepsNoMember holds a write whose
// body is not a JSON object to keeping no member, rather than the members of
// an earlier write.
func TestRecordWrites_ABodyThatIsNotAnObject_KeepsNoMember(t *testing.T) {
	client, recorder := RecordWrites(t, `{}`, `{}`)
	post := func(body string) {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, client.GL().BaseURL().String()+"projects", strings.NewReader(body))
		if err != nil {
			t.Fatalf("build the request: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("send the request: %v", err)
		}
		_ = resp.Body.Close()
	}

	post(`{"name":"first"}`)
	if got, ok := recorder.Member("name"); !ok || got != `"first"` {
		t.Fatalf("Member(name) = %s, %v, want the first write's name", got, ok)
	}
	post(`not json`)
	if got, ok := recorder.Member("name"); ok {
		t.Errorf("Member(name) = %s after a body that is not an object, want no member", got)
	}
}
