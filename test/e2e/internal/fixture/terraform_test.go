//go:build e2e

// terraform_test.go pins the two pure halves of the Terraform state push that
// a test can check without a GitLab: the URL a version is pushed to and the
// state document it carries, since a malformed one is refused by the backend
// with a message about the wrong thing.

package fixture

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestTerraformStateURL_BuildsTheBackendPath checks the push URL names the
// project and the state and carries exactly one slash between the base and the
// API path whatever trailing slash the base had.
func TestTerraformStateURL_BuildsTheBackendPath(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{name: "no trailing slash", base: "http://gitlab:8929", want: "http://gitlab:8929/api/v4/projects/7/terraform/state/tf"},
		{name: "trailing slash", base: "http://gitlab:8929/", want: "http://gitlab:8929/api/v4/projects/7/terraform/state/tf"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := terraformStateURL(testCase.base, 7, "tf")
			if got != testCase.want {
				t.Errorf("terraformStateURL(%q, 7, tf) = %q, want %q", testCase.base, got, testCase.want)
			}
		})
	}
}

// TestTerraformStateBody_IsValidJSONAtTheSerial checks the pushed document
// parses and carries the serial and lineage the caller asked for, so a bad
// interpolation fails here rather than as a 400 from the backend.
func TestTerraformStateBody_IsValidJSONAtTheSerial(t *testing.T) {
	body := terraformStateBody(42, 3)
	var decoded struct {
		Version int    `json:"version"`
		Serial  int    `json:"serial"`
		Lineage string `json:"lineage"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("terraformStateBody produced invalid JSON: %v (%s)", err, body)
	}
	if decoded.Version != 4 || decoded.Serial != 3 {
		t.Errorf("terraformStateBody(42, 3) has version=%d serial=%d, want 4 and 3", decoded.Version, decoded.Serial)
	}
	if !strings.Contains(decoded.Lineage, "42") {
		t.Errorf("terraformStateBody(42, 3) lineage %q does not name the project", decoded.Lineage)
	}
}

// TestTerraformBasicAuth_EncodesUserAndToken checks the header is the base64
// of user:token under the Basic scheme, which is what the backend decodes.
func TestTerraformBasicAuth_EncodesUserAndToken(t *testing.T) {
	got := terraformBasicAuth("root", "glpat-xyz")
	const want = "Basic cm9vdDpnbHBhdC14eXo="
	if got != want {
		t.Errorf("terraformBasicAuth(root, glpat-xyz) = %q, want %q", got, want)
	}
}

// TestTerraformStateIsLocked_Answers covers the three answers the lock
// read-back distinguishes, including the two shapes of "no lock": a state that
// carries none, and a project or state GitLab answered nothing for.
func TestTerraformStateIsLocked_Answers(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		want    bool
		wantErr bool
	}{
		{
			name:   "locked",
			answer: `{"data":{"project":{"terraformState":{"lockedAt":"2026-01-01T00:00:00Z"}}}}`,
			want:   true,
		},
		{name: "unlocked", answer: `{"data":{"project":{"terraformState":{"lockedAt":null}}}}`},
		{name: "no state", answer: `{"data":{"project":{"terraformState":null}}}`},
		{name: "no project", answer: `{"data":{"project":null}}`},
		{name: "refused", answer: `{"errors":[{"message":"forbidden"}]}`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.configure(func() { stub.graphqlAnswers = []string{tc.answer} })

			got, err := TerraformStateIsLocked(t.Context(), client, "group/project", "production")
			if (err != nil) != tc.wantErr {
				t.Fatalf("TerraformStateIsLocked() error = %v, wantErr = %t", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("TerraformStateIsLocked() = %t, want %t", got, tc.want)
			}
		})
	}
}
