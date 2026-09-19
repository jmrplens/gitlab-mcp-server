package testutil

import (
	"strings"
	"testing"
)

// capturedDecodeFailure is the fragment [gitlabclient.ResponseCapture.Decode]
// puts in an error when GitLab's answer decoded for the SDK and not for the
// type reading fields beside it.
const capturedDecodeFailure = "decode the captured response"

// sdkDecodeFailure is the fragment encoding/json puts in an error when a
// value in GitLab's answer does not fit the Go field reading it. The SDK
// decodes the same bytes the capture does, and decodes them first, so this is
// the failure a handler reports wherever client-go models the field too.
const sdkDecodeFailure = "cannot unmarshal"

// CapturedCase is one handler call in a table asserting the failure a
// captured response adds: a body the output type cannot hold.
type CapturedCase struct {
	// Name says which handler the call drives, and names the subtest.
	Name string
	// Call runs the handler and returns the error it reported, if any.
	Call func() error
}

// AssertCapturedDecodeFailures runs each case under a subtest of its own and
// fails the ones whose handler answered successfully, or with an error that
// does not name the captured response's decode failure.
//
// Every package that reads a field client-go does not model asserts the same
// thing about every handler it converted (ADR-0021), so the table lives with
// the handlers and the assertion lives here.
func AssertCapturedDecodeFailures(t *testing.T, cases []CapturedCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			assertCapturedDecodeFailure(t, testCase)
		})
	}
}

// assertCapturedDecodeFailure runs one case and reports it when the handler
// answered with anything but the capture's decode failure. It takes the
// reporter as [errorReporter], the interface this package already uses to
// make a failure branch observable without failing the calling test.
func assertCapturedDecodeFailure(t errorReporter, testCase CapturedCase) {
	if err := testCase.Call(); !reportsCapturedDecodeFailure(err) {
		t.Errorf("%s error = %v, want the capture's decode failure", testCase.Name, err)
	}
}

// reportsCapturedDecodeFailure reports whether err is the one a handler
// returns when the captured answer does not fit the type reading it. It is
// the whole judgement [AssertCapturedDecodeFailures] makes, split out so it
// can be tested without a stand-in [testing.T].
func reportsCapturedDecodeFailure(err error) bool {
	return err != nil && strings.Contains(err.Error(), capturedDecodeFailure)
}

// AssertUnreadableBodyRefused runs each case under a subtest of its own and
// fails the ones whose handler answered successfully, or with an error that
// names neither decoder's failure.
//
// It is [AssertCapturedDecodeFailures] for a field client-go has since caught
// up with. Both decoders read the same bytes and the SDK's runs first, so
// once client-go models the field the capture reads, no body can fail the
// capture alone and the stricter assertion is unsatisfiable. What survives is
// the behavior these tests were written for: a handler refuses a body it
// cannot read rather than publishing a half-filled result.
func AssertUnreadableBodyRefused(t *testing.T, cases []CapturedCase) {
	t.Helper()
	for _, testCase := range cases {
		t.Run(testCase.Name, func(t *testing.T) {
			assertUnreadableBodyRefused(t, testCase)
		})
	}
}

// assertUnreadableBodyRefused runs one case and reports it when the handler
// answered with anything but a decode failure from one of the two readers.
func assertUnreadableBodyRefused(t errorReporter, testCase CapturedCase) {
	if err := testCase.Call(); !reportsUnreadableBody(err) {
		t.Errorf("%s error = %v, want a decode failure from either reader", testCase.Name, err)
	}
}

// reportsUnreadableBody reports whether err is the one a handler returns when
// GitLab's answer does not fit the type reading it, whichever of the two
// decoders noticed.
func reportsUnreadableBody(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, capturedDecodeFailure) || strings.Contains(text, sdkDecodeFailure)
}
