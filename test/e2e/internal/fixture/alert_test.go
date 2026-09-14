//go:build e2e

// alert_test.go pins the one piece of the alert seeder that is not a call to
// GitLab: reading the IID out of the notify response, since a body that names
// no alert would otherwise leave the metric image scenario standing on a zero.

package fixture

import (
	"strings"
	"testing"
)

// TestParseAlertIID_ReadsTheOneAlertOrRefuses checks the notify parser on the
// answer GitLab gives and on the shapes it must refuse: an empty list, more
// than one alert, and an alert with no IID.
func TestParseAlertIID_ReadsTheOneAlertOrRefuses(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    int64
		refused string
	}{
		{name: "one alert", payload: `[{"iid":12}]`, want: 12},
		{name: "no alert", payload: `[]`, refused: "exactly one"},
		{name: "two alerts", payload: `[{"iid":1},{"iid":2}]`, refused: "exactly one"},
		{name: "no iid", payload: `[{"title":"x"}]`, refused: "exactly one"},
		{name: "not an array", payload: `{"iid":1}`, refused: "decoding"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseAlertIID([]byte(testCase.payload))
			if testCase.refused != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.refused) {
					t.Fatalf("parseAlertIID(%q) = %d, %v; want an error mentioning %q", testCase.payload, got, err, testCase.refused)
				}
				return
			}
			if err != nil || got != testCase.want {
				t.Errorf("parseAlertIID(%q) = %d, %v; want %d", testCase.payload, got, err, testCase.want)
			}
		})
	}
}
