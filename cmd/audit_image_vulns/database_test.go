// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// database_test.go covers how the run learns which vulnerability database
// Trivy judged against, and the databases it refuses to be judged against.
package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixtureNow is the moment the database fixtures are read at: a day after
// the update they record and before the next one they announce.
var fixtureNow = time.Date(2026, 10, 25, 11, 0, 0, 0, time.UTC)

// fixtureDatabase is what `trivy version --format json` prints for a
// database fetched at fixtureNow.
func fixtureDatabase() map[string]any {
	return map[string]any{
		"Version": "0.76.1",
		"VulnerabilityDB": map[string]any{
			"Version":      2,
			"NextUpdate":   "2026-10-26T07:10:41.872962061Z",
			"UpdatedAt":    "2026-10-25T07:10:41.872962161Z",
			"DownloadedAt": "2026-10-25T10:59:10.298116628Z",
		},
	}
}

// TestReadDatabase_ReadsWhatTrivyVersionPrints covers the one shape the
// command reads: the Trivy version, the database schema and its two dates.
func TestReadDatabase_ReadsWhatTrivyVersionPrints(t *testing.T) {
	t.Parallel()

	got, err := readDatabase(writeJSON(t, fixtureDatabase()), fixtureNow)
	if err != nil {
		t.Fatalf("readDatabase: %v", err)
	}
	if got.trivyVersion != "0.76.1" || got.schema != 2 ||
		!got.updatedAt.Equal(time.Date(2026, 10, 25, 7, 10, 41, 872962161, time.UTC)) ||
		!got.nextUpdate.Equal(time.Date(2026, 10, 26, 7, 10, 41, 872962061, time.UTC)) {
		t.Fatalf("readDatabase = %+v, want Trivy 0.76.1, schema 2 and the two dates", got)
	}
}

// TestReadDatabase_RefusesADatabaseItCannotName covers every database the
// verdict must not be reached against: a file that is absent or is not what
// trivy version prints, one that names no database, which is what Trivy
// prints before it has fetched one, one that names no update, and one whose
// next update was due before the run, which Trivy itself calls stale. Its
// NextUpdate is the database's own, so the bound is Trivy's and not a number
// this command chose.
func TestReadDatabase_RefusesADatabaseItCannotName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		edit func(map[string]any)
		now  time.Time
		name string
		raw  string
		want string
	}{
		{name: "not JSON", raw: "Version: 0.76.1", want: "is not what trivy version --format json prints"},
		{name: "no database", edit: func(d map[string]any) { delete(d, "VulnerabilityDB") }, want: "names no vulnerability database: Trivy has not fetched one"},
		{
			name: "no update", want: "names no vulnerability database: Trivy has not fetched one",
			edit: func(d map[string]any) { delete(d["VulnerabilityDB"].(map[string]any), "UpdatedAt") },
		},
		{
			name: "no next update", want: "names no vulnerability database: Trivy has not fetched one",
			edit: func(d map[string]any) { delete(d["VulnerabilityDB"].(map[string]any), "NextUpdate") },
		},
		{
			name: "stale", now: time.Date(2026, 10, 26, 7, 10, 42, 0, time.UTC),
			want: "the vulnerability database was due for an update at 2026-10-26T07:10:41Z, before this run at 2026-10-26T07:10:42Z, so Trivy judged against a stale database",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var path string
			if tc.raw != "" {
				path = writeFile(t, []byte(tc.raw))
			} else {
				db := fixtureDatabase()
				if tc.edit != nil {
					tc.edit(db)
				}
				path = writeJSON(t, db)
			}
			now := tc.now
			if now.IsZero() {
				now = fixtureNow
			}
			_, err := readDatabase(path, now)
			if err == nil || !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readDatabase error = %v, want one naming %s and saying %q", err, path, tc.want)
			}
		})
	}
}

// TestReadDatabase_DueAtTheMomentOfTheRun_IsStillCurrent pins the boundary:
// a run at the very instant of the next update is judged against a database
// Trivy still calls current.
func TestReadDatabase_DueAtTheMomentOfTheRun_IsStillCurrent(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 26, 7, 10, 41, 872962061, time.UTC)
	if _, err := readDatabase(writeJSON(t, fixtureDatabase()), now); err != nil {
		t.Fatalf("readDatabase at the next update: %v", err)
	}
}

// TestReadDatabase_AFileThatIsNotThere_IsAnError covers the file itself being
// absent.
func TestReadDatabase_AFileThatIsNotThere_IsAnError(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "absent.json")
	if _, err := readDatabase(path, fixtureNow); err == nil || !strings.Contains(err.Error(), "absent.json") {
		t.Fatalf("readDatabase error = %v, want one naming the file", err)
	}
}
