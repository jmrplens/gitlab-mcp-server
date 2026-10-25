package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// database is the vulnerability database Trivy judged the reports against,
// as `trivy version --format json` names it.
type database struct {
	updatedAt, nextUpdate time.Time
	trivyVersion          string
	schema                int
}

// trivyVersion is the part of `trivy version --format json` this command
// reads.
type trivyVersion struct {
	VulnerabilityDB *struct {
		UpdatedAt  time.Time
		NextUpdate time.Time
		Version    int
	}
	Version string
}

// readDatabase reads what trivy version printed about the database in the
// cache the scans used, and refuses a database it cannot name and one Trivy
// itself would call stale at now: past the NextUpdate it was published with.
func readDatabase(path string, now time.Time) (database, error) {
	body, err := os.ReadFile(path) // #nosec G304 -- the file the caller names with -db.
	if err != nil {
		return database{}, err
	}
	var version trivyVersion
	if parseErr := json.Unmarshal(body, &version); parseErr != nil {
		return database{}, fmt.Errorf("%s: is not what trivy version --format json prints: %w", path, parseErr)
	}
	db := version.VulnerabilityDB
	if db == nil || db.UpdatedAt.IsZero() || db.NextUpdate.IsZero() {
		return database{}, fmt.Errorf("%s: names no vulnerability database: Trivy has not fetched one into the cache it was pointed at", path)
	}
	if now.After(db.NextUpdate) {
		return database{}, fmt.Errorf("%s: the vulnerability database was due for an update at %s, before this run at %s, so Trivy judged against a stale database",
			path, db.NextUpdate.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	}
	return database{trivyVersion: version.Version, schema: db.Version, updatedAt: db.UpdatedAt, nextUpdate: db.NextUpdate}, nil
}
