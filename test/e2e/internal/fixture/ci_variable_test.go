//go:build e2e

// ci_variable_test.go drives the CI variable builder's pure halves: the name
// spelling GitLab accepts, the instance create with the answer a retried
// attempt gets, and the removal's two endings.

package fixture

import (
	"net/http"
	"strings"
	"testing"
)

// TestCIVariableKeyFrom_RunScoping_BecomesANameGitLabAccepts checks that the
// dashes and dots a run-scoped name carries are gone, since GitLab refuses a
// variable key holding either.
func TestCIVariableKeyFrom_RunScoping_BecomesANameGitLabAccepts(t *testing.T) {
	cases := []struct {
		name  string
		scope string
		given string
		want  string
	}{
		{name: "dashes", scope: "PROJECT", given: "var-testname-ab12", want: "E2E_PROJECT_VAR_TESTNAME_AB12"},
		{name: "dots", scope: "GROUP", given: "var.test.1", want: "E2E_GROUP_VAR_TEST_1"},
		{name: "already plain", scope: "INSTANCE", given: "VAR_1", want: "E2E_INSTANCE_VAR_1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ciVariableKeyFrom(tc.scope, tc.given); got != tc.want {
				t.Errorf("ciVariableKeyFrom(%q, %q) = %q, want %q", tc.scope, tc.given, got, tc.want)
			}
		})
	}
}

// TestCreateInstanceCIVariable_Created_ReadsTheKeyAndValueBack checks the
// ordinary ending: one POST, and the variable GitLab answered with.
func TestCreateInstanceCIVariable_Created_ReadsTheKeyAndValueBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/admin/ci/variables", stubCreated(map[string]any{
		"key": "E2E_INSTANCE_ONE", "value": ciVariableValue,
	}))

	got, err := createInstanceCIVariable(t.Context(), client, "E2E_INSTANCE_ONE")
	if err != nil {
		t.Fatalf("createInstanceCIVariable() error = %v, want nil", err)
	}
	want := CIVariable{Key: "E2E_INSTANCE_ONE", Value: ciVariableValue}
	if got != want {
		t.Errorf("createInstanceCIVariable() = %+v, want %+v", got, want)
	}
}

// TestCreateInstanceCIVariable_AlreadyTaken_ReadsTheExistingOneBack checks the
// ending a retried attempt gets: the first attempt created the variable and
// lost the answer, so GitLab refuses the second and the variable that is
// there is what the caller asked for.
func TestCreateInstanceCIVariable_AlreadyTaken_ReadsTheExistingOneBack(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/admin/ci/variables",
		stubRefusal(http.StatusBadRequest, "key has already been taken"))
	stub.answers(http.MethodGet, "/api/v4/admin/ci/variables/E2E_INSTANCE_ONE", stubOK(map[string]any{
		"key": "E2E_INSTANCE_ONE", "value": ciVariableValue,
	}))

	got, err := createInstanceCIVariable(t.Context(), client, "E2E_INSTANCE_ONE")
	if err != nil {
		t.Fatalf("createInstanceCIVariable() error = %v, want nil", err)
	}
	if got.Key != "E2E_INSTANCE_ONE" {
		t.Errorf("createInstanceCIVariable() = %+v, want the variable that already exists", got)
	}
}

// TestCreateInstanceCIVariable_AlreadyTakenAndUnreadable_NamesTheKey checks
// that a read-back which itself fails reports the key, since a caller holding
// an empty variable would go on to address nothing.
func TestCreateInstanceCIVariable_AlreadyTakenAndUnreadable_NamesTheKey(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/admin/ci/variables",
		stubRefusal(http.StatusBadRequest, "key has already been taken"))
	stub.answers(http.MethodGet, "/api/v4/admin/ci/variables/E2E_INSTANCE_ONE",
		stubRefusal(http.StatusForbidden, "403 Forbidden"))

	_, err := createInstanceCIVariable(t.Context(), client, "E2E_INSTANCE_ONE")
	if err == nil {
		t.Fatal("createInstanceCIVariable() error = nil, want the unreadable read-back reported")
	}
	if !strings.Contains(err.Error(), "E2E_INSTANCE_ONE") {
		t.Errorf("createInstanceCIVariable() error = %q, want it to name the key", err)
	}
}

// TestRemoveInstanceCIVariable_Endings_ToleratesAReservationNothingUsed
// checks the three endings: a variable removed, a name nothing ever created,
// and a refusal that is a real failure.
func TestRemoveInstanceCIVariable_Endings_ToleratesAReservationNothingUsed(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "removed", answer: stubNoContent()},
		{name: "never created", answer: stubRefusal(http.StatusNotFound, "404 Variable Not Found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/admin/ci/variables/E2E_INSTANCE_ONE", tc.answer)

			err := removeInstanceCIVariable(t.Context(), client, "E2E_INSTANCE_ONE")
			if (err != nil) != tc.wantErr {
				t.Errorf("removeInstanceCIVariable() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
