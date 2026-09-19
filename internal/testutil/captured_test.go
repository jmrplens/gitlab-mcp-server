package testutil

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestReportsCapturedDecodeFailure_JudgesTheErrorAlone verifies the judgement
// the helper makes about one handler's answer: the capture's decode failure
// passes, and both no error at all and an error about anything else fail.
func TestReportsCapturedDecodeFailure_JudgesTheErrorAlone(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "the capture's decode failure", err: errors.New("op: decode the captured response: bad"), want: true},
		{name: "no error at all", err: nil, want: false},
		{name: "another error", err: errors.New("op: 404 Not Found"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := reportsCapturedDecodeFailure(testCase.err); got != testCase.want {
				t.Errorf("reportsCapturedDecodeFailure(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestAssertCapturedDecodeFailures_RunsACaseUnderItsOwnName verifies the
// helper drives every case it is given, under a subtest named after it.
func TestAssertCapturedDecodeFailures_RunsACaseUnderItsOwnName(t *testing.T) {
	var ran []string
	AssertCapturedDecodeFailures(t, []CapturedCase{
		{Name: "first", Call: func() error {
			ran = append(ran, "first")
			return errors.New("op: decode the captured response: bad")
		}},
		{Name: "second", Call: func() error {
			ran = append(ran, "second")
			return errors.New("op: decode the captured response: bad")
		}},
	})
	if len(ran) != 2 || ran[0] != "first" || ran[1] != "second" {
		t.Errorf("calls run = %v, want both in order", ran)
	}
}

// capturedCaseReporter stands in for [testing.T] so the failure one case
// reports can be observed rather than raised.
type capturedCaseReporter struct{ reported []string }

// Errorf records the message instead of failing a test.
func (r *capturedCaseReporter) Errorf(format string, args ...any) {
	r.reported = append(r.reported, fmt.Sprintf(format, args...))
}

// TestAssertCapturedDecodeFailure_ReportsAnythingElse verifies one case is
// reported when the handler answered successfully or with another error, and
// left alone when it reported the capture's decode failure.
func TestAssertCapturedDecodeFailure_ReportsAnythingElse(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReport bool
	}{
		{name: "no error at all", err: nil, wantReport: true},
		{name: "another error", err: errors.New("op: 404 Not Found"), wantReport: true},
		{name: "the capture's decode failure", err: errors.New("op: decode the captured response: bad"), wantReport: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reporter := &capturedCaseReporter{}

			assertCapturedDecodeFailure(reporter, CapturedCase{
				Name: "handler",
				Call: func() error { return testCase.err },
			})

			if (len(reporter.reported) == 1) != testCase.wantReport {
				t.Errorf("reported = %v, want %t", reporter.reported, testCase.wantReport)
			}
			if testCase.wantReport && !strings.Contains(reporter.reported[0], "want the capture's decode failure") {
				t.Errorf("report = %q, want it to name what was expected", reporter.reported[0])
			}
		})
	}
}

// TestCapturedDecodeFailure_IsTheFragmentTheCaptureWrites pins the fragment
// the helper looks for to the wording [gitlabclient.ResponseCapture.Decode]
// uses, so a reworded error is caught here rather than in every package.
func TestCapturedDecodeFailure_IsTheFragmentTheCaptureWrites(t *testing.T) {
	if !strings.Contains("decode the captured response: unexpected end of JSON input", capturedDecodeFailure) {
		t.Errorf("capturedDecodeFailure = %q, want the fragment Decode writes", capturedDecodeFailure)
	}
}

// TestSDKDecodeFailure_IsTheFragmentEncodingJSONWrites pins the fragment the
// looser helper looks for to the wording encoding/json uses for a value that
// does not fit the field reading it, which is what the SDK's own decoder
// reports wherever client-go models the field the capture reads.
func TestSDKDecodeFailure_IsTheFragmentEncodingJSONWrites(t *testing.T) {
	if !strings.Contains("json: cannot unmarshal number into Go struct field .site_name of type string", sdkDecodeFailure) {
		t.Errorf("sdkDecodeFailure = %q, want the fragment encoding/json writes", sdkDecodeFailure)
	}
}

// TestReportsUnreadableBody_AcceptsEitherDecoder verifies the judgement the
// looser helper makes: either reader's decode failure passes, and both no
// error at all and an error about anything else fail.
func TestReportsUnreadableBody_AcceptsEitherDecoder(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "the capture's decode failure", err: errors.New("op: decode the captured response: bad"), want: true},
		{name: "the SDK's own decode failure", err: errors.New("op: json: cannot unmarshal number into Go struct field .site_name of type string"), want: true},
		{name: "no error at all", err: nil, want: false},
		{name: "another error", err: errors.New("op: 404 Not Found"), want: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := reportsUnreadableBody(testCase.err); got != testCase.want {
				t.Errorf("reportsUnreadableBody(%v) = %t, want %t", testCase.err, got, testCase.want)
			}
		})
	}
}

// TestAssertUnreadableBodyRefused_RunsACaseUnderItsOwnName verifies the looser
// helper drives every case it is given, under a subtest named after it.
func TestAssertUnreadableBodyRefused_RunsACaseUnderItsOwnName(t *testing.T) {
	var ran []string
	AssertUnreadableBodyRefused(t, []CapturedCase{
		{Name: "first", Call: func() error {
			ran = append(ran, "first")
			return errors.New("op: json: cannot unmarshal number into Go struct field .color of type string")
		}},
		{Name: "second", Call: func() error {
			ran = append(ran, "second")
			return errors.New("op: decode the captured response: bad")
		}},
	})
	if len(ran) != 2 || ran[0] != "first" || ran[1] != "second" {
		t.Errorf("calls run = %v, want both in order", ran)
	}
}

// TestAssertUnreadableBodyRefused_ReportsAnythingElse verifies one case is
// reported when the handler answered successfully or with an error about
// something other than a body it could not read.
func TestAssertUnreadableBodyRefused_ReportsAnythingElse(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReport bool
	}{
		{name: "no error at all", err: nil, wantReport: true},
		{name: "another error", err: errors.New("op: 404 Not Found"), wantReport: true},
		{name: "the capture's decode failure", err: errors.New("op: decode the captured response: bad"), wantReport: false},
		{name: "the SDK's own decode failure", err: errors.New("op: json: cannot unmarshal string into Go struct field .popular of type bool"), wantReport: false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reporter := &capturedCaseReporter{}

			assertUnreadableBodyRefused(reporter, CapturedCase{
				Name: "handler",
				Call: func() error { return testCase.err },
			})

			if (len(reporter.reported) == 1) != testCase.wantReport {
				t.Errorf("reported = %v, want %t", reporter.reported, testCase.wantReport)
			}
			if testCase.wantReport && !strings.Contains(reporter.reported[0], "want a decode failure from either reader") {
				t.Errorf("report = %q, want it to name what was expected", reporter.reported[0])
			}
		})
	}
}
