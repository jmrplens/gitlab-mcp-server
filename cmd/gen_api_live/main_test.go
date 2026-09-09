package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/apilive"
)

// wholeEnough builds an introspection big enough to clear every floor, so a
// test about one rule is not also a test about the floors.
//
// The floors exist for a boot that half ran, and a fixture that trips them by
// being a fixture would make every case here fail for the same uninteresting
// reason.
func wholeEnough() dumped {
	payload := dumped{
		SchemaVersion: apilive.SchemaVersion,
		Version:       "19.3.1-ee",
		Revision:      "9d55961fa80",
		Entities:      map[string]apilive.Entity{},
		Features:      map[string]string{},
	}
	for i := range minEntities + 1 {
		fields := make([]apilive.Field, 0, 13)
		for j := range 13 {
			fields = append(fields, apilive.Field{Name: fieldName(i, j)})
		}
		payload.Entities[entityName(i)] = apilive.Entity{Fields: fields}
	}
	for i := range minRoutes + 1 {
		payload.Routes = append(payload.Routes, apilive.Route{
			Method: "GET", Path: routePath(i), Entity: entityName(i % 7),
		})
	}
	for i := range minFeatures + 1 {
		payload.Features[featureName(i)] = apilive.TierPremium
	}
	return payload
}

func entityName(i int) string   { return "API::Entities::Fixture" + itoa(i) }
func fieldName(i, j int) string { return "field_" + itoa(i) + "_" + itoa(j) }
func routePath(i int) string    { return "/api/:version/fixture/" + itoa(i) }
func featureName(i int) string  { return "fixture_feature_" + itoa(i) }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

// writeDump stages an introspection on disk the way the container would leave
// it, and returns its path.
func writeDump(t *testing.T, payload dumped) string {
	t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "introspect.json")
	if writeErr := os.WriteFile(path, encoded, 0o600); writeErr != nil {
		t.Fatalf("write the fixture: %v", writeErr)
	}
	return path
}

// TestRunGenerate_ADumpOnDisk_BecomesARecordWithProvenance verifies the half of
// this command that has rules in it, without the half that needs Docker.
//
// The separation is the point of the -dump flag: a boot is slow and needs an
// image, and none of the judgement lives there.
func TestRunGenerate_ADumpOnDisk_BecomesARecordWithProvenance(t *testing.T) {
	dir := t.TempDir()
	payload := wholeEnough()

	if err := runGenerate(dir, writeDump(t, payload), "gitlab/gitlab-ee:latest", false); err != nil {
		t.Fatalf("runGenerate: %v", err)
	}

	doc, err := apilive.Read(dir)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, testCase := range []struct {
		name string
		got  any
		want any
	}{
		{name: "the schema version is this build's", got: doc.SchemaVersion, want: apilive.SchemaVersion},
		{name: "the version comes from the instance", got: doc.Source.Version, want: "19.3.1-ee"},
		{name: "the revision comes from the instance", got: doc.Source.Revision, want: "9d55961fa80"},
		{name: "the image is recorded", got: doc.Source.Image, want: "gitlab/gitlab-ee:latest"},
		{name: "the entity count is counted, not copied", got: doc.Source.Entities, want: len(payload.Entities)},
		{name: "the field count spans every entity", got: doc.Source.Fields, want: len(payload.Entities) * 13},
		{name: "the route count is counted", got: doc.Source.Routes, want: len(payload.Routes)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Errorf("got %v, want %v", testCase.got, testCase.want)
			}
		})
	}

	t.Run("the digest is of what the container produced", func(t *testing.T) {
		if len(doc.Source.SHA256) != 64 {
			t.Errorf("sha256 = %q, want 64 hex characters", doc.Source.SHA256)
		}
	})
	t.Run("the retrieval date is today", func(t *testing.T) {
		if doc.Source.RetrievedAt != time.Now().UTC().Format(time.DateOnly) {
			t.Errorf("retrieved_at = %q, want today", doc.Source.RetrievedAt)
		}
	})
}

// TestRunGenerate_ARecordThatIsNotAGitLab_IsRefusedRatherThanWritten verifies
// the floors at the moment they matter most.
//
// An introspection that half ran does not fail: it returns less. Written, that
// record says GitLab stopped sending things, and every audit downstream
// reports the difference as a gap in this server. Refusing to write it is the
// only place that can be caught, because afterwards it looks like data.
func TestRunGenerate_ARecordThatIsNotAGitLab_IsRefusedRatherThanWritten(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		mutate  func(*dumped)
		wantsIn string
	}{
		{
			name:    "too few entities",
			mutate:  func(d *dumped) { d.Entities = map[string]apilive.Entity{"API::Entities::One": {}} },
			wantsIn: "entities",
		},
		{
			name: "the entities are there and empty",
			mutate: func(d *dumped) {
				for name := range d.Entities {
					d.Entities[name] = apilive.Entity{}
				}
			},
			wantsIn: "exposed fields",
		},
		{
			name:    "too few routes",
			mutate:  func(d *dumped) { d.Routes = d.Routes[:3] },
			wantsIn: "routes",
		},
		{
			name:    "too few licensed features",
			mutate:  func(d *dumped) { d.Features = map[string]string{"one": apilive.TierPremium} },
			wantsIn: "licensed features",
		},
		{
			name: "an entity refused to describe itself",
			mutate: func(d *dumped) {
				d.Entities["API::Entities::Fixture1"] = apilive.Entity{Error: "NoMethodError"}
			},
			wantsIn: "refused to describe themselves",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			payload := wholeEnough()
			testCase.mutate(&payload)

			err := runGenerate(dir, writeDump(t, payload), "gitlab/gitlab-ee:latest", false)
			if err == nil {
				t.Fatal("the record was written, want a refusal")
			}
			if !strings.Contains(err.Error(), testCase.wantsIn) {
				t.Errorf("error = %q, want it to name %q", err, testCase.wantsIn)
			}
			if _, statErr := os.Stat(apilive.Path(dir)); statErr == nil {
				t.Error("a refused record was written anyway")
			}
		})
	}
}

// TestRunGenerate_AnIntrospectionFromAnotherSchema_IsRefused verifies that the
// script and the command are held to one version of the shape between them.
//
// They are separate files and a maintainer edits the Ruby without rebuilding
// the Go. Decoding a shape that moved would populate the fields that still
// match and silently drop the rest, which is the one failure a reader could
// not detect afterwards.
func TestRunGenerate_AnIntrospectionFromAnotherSchema_IsRefused(t *testing.T) {
	payload := wholeEnough()
	payload.SchemaVersion = apilive.SchemaVersion + 1

	err := runGenerate(t.TempDir(), writeDump(t, payload), "gitlab/gitlab-ee:latest", false)

	if err == nil {
		t.Fatal("an introspection from another schema was accepted")
	}
	if !strings.Contains(err.Error(), "schema version") {
		t.Errorf("error = %q, want it to name the schema version", err)
	}
}

// TestRunCheck_AStaleOrTruncatedRecord_IsRefused verifies the gate every audit
// rests on: it reads one file, asks nothing of Docker or the network, and
// fails on a record that cannot answer for a current GitLab.
func TestRunCheck_AStaleOrTruncatedRecord_IsRefused(t *testing.T) {
	fresh := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

	for _, testCase := range []struct {
		name        string
		retrievedAt string
		now         time.Time
		mutate      func(*dumped)
		wantsIn     string
	}{
		{name: "a whole record taken today", retrievedAt: "2026-09-09", now: fresh},
		{
			name: "a record older than the window", retrievedAt: "2025-01-01", now: fresh,
			wantsIn: "days old",
		},
		{
			name: "a record taken in the future", retrievedAt: "2027-01-01", now: fresh,
			wantsIn: "has not happened yet",
		},
		{
			name: "a retrieval date nothing can read", retrievedAt: "last Tuesday", now: fresh,
			wantsIn: "not a date",
		},
		{
			name: "a record that lost its routes", retrievedAt: "2026-09-09", now: fresh,
			mutate:  func(d *dumped) { d.Routes = nil },
			wantsIn: "routes",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			payload := wholeEnough()
			if testCase.mutate != nil {
				testCase.mutate(&payload)
			}
			// Written directly rather than through runGenerate, which stamps
			// today and refuses a short record: this case needs both.
			doc := apilive.Document{
				SchemaVersion: apilive.SchemaVersion,
				Source: apilive.Source{
					Image: "gitlab/gitlab-ee:latest", Version: payload.Version,
					RetrievedAt: testCase.retrievedAt,
				},
				Entities: payload.Entities, Routes: payload.Routes, Features: payload.Features,
			}
			if err := apilive.Write(dir, doc); err != nil {
				t.Fatalf("write the fixture: %v", err)
			}

			err := runCheck(dir, testCase.now)

			if testCase.wantsIn == "" {
				if err != nil {
					t.Fatalf("runCheck: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("the record passed, want a refusal")
			}
			if !strings.Contains(err.Error(), testCase.wantsIn) {
				t.Errorf("error = %q, want it to name %q", err, testCase.wantsIn)
			}
		})
	}
}

// TestIntrospection_WithoutADump_GoesToTheRunner verifies the seam that keeps
// Docker out of every test above.
func TestIntrospection_WithoutADump_GoesToTheRunner(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })

	var askedFor string
	var askedKeep bool
	runner = func(image string, keep bool) ([]byte, origin, error) {
		askedFor, askedKeep = image, keep
		return []byte("{}"), origin{image: image, digest: "sha256:abc"}, nil
	}

	raw, from, err := introspection("", "gitlab/gitlab-ee:17.0.0", true)
	if err != nil {
		t.Fatalf("introspection: %v", err)
	}
	t.Run("the image is the one asked for", func(t *testing.T) {
		if askedFor != "gitlab/gitlab-ee:17.0.0" {
			t.Errorf("image = %q, want the one passed", askedFor)
		}
	})
	t.Run("keep is passed through", func(t *testing.T) {
		if !askedKeep {
			t.Error("keep was not passed to the runner")
		}
	})
	t.Run("the digest travels with the output", func(t *testing.T) {
		if from.digest != "sha256:abc" || string(raw) != "{}" {
			t.Errorf("got %q and %+v, want the runner's own answer", raw, from)
		}
	})
}

// TestIntrospection_WithADump_NeverBootsAnything verifies that -dump is a
// genuine offline path and not a boot with a shortcut.
func TestIntrospection_WithADump_NeverBootsAnything(t *testing.T) {
	previous := runner
	t.Cleanup(func() { runner = previous })
	runner = func(string, bool) ([]byte, origin, error) {
		t.Error("the runner was called for a dump that was already on disk")
		return nil, origin{}, nil
	}

	path := writeDump(t, wholeEnough())
	raw, from, err := introspection(path, "gitlab/gitlab-ee:latest", false)
	if err != nil {
		t.Fatalf("introspection: %v", err)
	}
	if len(raw) == 0 || from.image != "gitlab/gitlab-ee:latest" {
		t.Errorf("got %d bytes from %+v, want the dump read back", len(raw), from)
	}
}
