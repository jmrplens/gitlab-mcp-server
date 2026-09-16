//go:build e2e

// geo_test.go drives the Geo site builder's pure halves against the stub:
// that a site is registered disabled, the two endings a deletion has, and what
// the read-back answers.

package fixture

import (
	"net/http"
	"testing"
)

// TestCreateGeoSite_Created_RegistersADisabledSite checks that the builder
// asks for a site that is switched off, which is what lets an instance with no
// Geo deployment hold one.
func TestCreateGeoSite_Created_RegistersADisabledSite(t *testing.T) {
	stub, client := newStubGitLab(t)
	stub.answers(http.MethodPost, "/api/v4/geo_sites", stubCreated(map[string]any{
		"id": 5, "name": "eval-geo", "url": "https://eval-geo.geo.example.invalid/", "enabled": false,
	}))

	got, err := createGeoSite(t.Context(), client, "eval-geo", "https://eval-geo.geo.example.invalid/")
	if err != nil {
		t.Fatalf("createGeoSite() error = %v, want nil", err)
	}
	want := GeoSite{ID: 5, Name: "eval-geo", URL: "https://eval-geo.geo.example.invalid/"}
	if got != want {
		t.Errorf("createGeoSite() = %+v, want %+v", got, want)
	}

	requests := stub.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("createGeoSite() sent %d requests, want 1", len(requests))
	}
	if requests[0].Body["enabled"] != false {
		t.Errorf("createGeoSite() sent %v, want a site that is switched off", requests[0].Body)
	}
}

// TestDeleteGeoSite_Endings_ToleratesOneACaseDeleted checks that a site a case
// already deleted is not a cleanup failure and any other refusal is.
func TestDeleteGeoSite_Endings_ToleratesOneACaseDeleted(t *testing.T) {
	cases := []struct {
		name    string
		answer  scriptedAnswer
		wantErr bool
	}{
		{name: "deleted", answer: stubNoContent()},
		{name: "already gone", answer: stubRefusal(http.StatusNotFound, "404 Not found")},
		{name: "refused", answer: stubRefusal(http.StatusForbidden, "403 Forbidden"), wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub, client := newStubGitLab(t)
			stub.answers(http.MethodDelete, "/api/v4/geo_sites/5", tc.answer)

			err := deleteGeoSite(t.Context(), client, 5)
			if (err != nil) != tc.wantErr {
				t.Errorf("deleteGeoSite() error = %v, wantErr = %t", err, tc.wantErr)
			}
		})
	}
}
