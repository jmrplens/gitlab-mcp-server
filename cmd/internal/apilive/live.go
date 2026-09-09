// Package apilive is the committed record of what a booted GitLab says its
// own REST API is, and the one reader of it.
//
// Every other oracle this repository holds about GitLab's REST API is a
// reading of text: the OpenAPI document GitLab commits to its own repository,
// or a scan of the Grape source that document is generated from. Both are
// downstream of the object that decides what a request returns, which is the
// Rails application with its classes loaded, and both lose the same thing when
// a name is not written down. GeoSiteStatus exposes its fields by iterating a
// constant assembled from two method calls: the source says "expose the loop
// variable", a scanner reads 26 fields, and GitLab sends 606. That is not a
// hole a better parser closes.
//
// So this record is produced by asking the application. cmd/gen_api_live boots
// a released GitLab image, runs one script inside it, and writes what comes
// back here; every audit then reads this file with no Docker and no network,
// the way cmd/gen_graphql_schema's pin is read. The boot is a generator, never
// an audit: an audit that needed a container could not be a gate.
//
// One thing evaluation does not give and the record therefore carries from
// source: a Grape condition is a Proc, and a Proc knows where it was written
// but not what it says. The generator reads those lines back from inside the
// same image, so a condition arrives here both located and quoted.
package apilive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SchemaVersion is the shape of this record. A reader refuses a version it was
// not written for rather than guessing at a field that moved.
const SchemaVersion = 1

// DefaultDir is where the record lives, beside the other pinned records.
const DefaultDir = "docs/development"

// FileName is the record's name on disk.
const FileName = "gitlab-api-live.json"

// Tier values, matching the licensed feature table's own three lists.
const (
	// TierPremium unlocks with any paid plan from Premium up.
	TierPremium = "premium"
	// TierUltimate unlocks with Ultimate only.
	TierUltimate = "ultimate"
	// TierGlobal is the table's GLOBAL_FEATURES: licensed, but not a tier,
	// since every paid plan carries it.
	TierGlobal = "global"
)

// Document is the committed record.
type Document struct {
	SchemaVersion int    `json:"schema_version"`
	Note          string `json:"note"`
	Source        Source `json:"source"`
	// Entities is every API::Entities class the instance had loaded, keyed by
	// its Ruby name (API::Entities::Project). OpenAPIName translates that to
	// the spelling the OpenAPI record uses, for a reader joining the two.
	Entities map[string]Entity `json:"entities"`
	// Routes is every endpoint Grape had mounted, in the order the router
	// holds them.
	Routes []Route `json:"routes"`
	// Features maps a licensed feature symbol to the tier that unlocks it.
	Features map[string]string `json:"features"`
}

// Source is the instance the record was taken from.
//
// The image reference and the version are both recorded because they answer
// different questions: the reference is what to run to reproduce this, and the
// version is what to compare against a GitLab release. A record whose version
// is behind the current release is stale in the sense that matters, and no
// amount of re-running the same image fixes it.
type Source struct {
	Image       string `json:"image"`
	Digest      string `json:"digest,omitempty"`
	Version     string `json:"version"`
	Revision    string `json:"revision,omitempty"`
	RetrievedAt string `json:"retrieved_at"`
	// SHA256 is of the introspection output as it left the container, before
	// this record wrapped it, so two runs of one image can be compared without
	// the wrapper's own fields entering the digest.
	SHA256   string `json:"sha256"`
	Entities int    `json:"entities"`
	Fields   int    `json:"fields"`
	Routes   int    `json:"routes"`
	Features int    `json:"features"`
}

// Entity is one Grape entity as the loaded class describes itself.
type Entity struct {
	// Error is set when the class refused to describe itself, which is a fact
	// about that class rather than a reason to drop it: a silently missing
	// entity reads as one GitLab does not have.
	Error string `json:"error,omitempty"`
	// Fields are the exposures in declaration order, with everything the
	// class inherits already flattened in, because root_exposures answers for
	// the class as it will render.
	Fields []Field `json:"fields,omitempty"`
}

// Field is one exposure.
type Field struct {
	// Name is the key GitLab sends.
	Name string `json:"name"`
	// Attribute is the method the value comes from, recorded only when `as:`
	// made it differ from Name: the key is what a client sees and the
	// attribute is what a reader greps the source for.
	Attribute string `json:"attribute,omitempty"`
	// Using names the entity the value renders with, in the same keying as
	// Document.Entities. It is the edge that makes the record a tree rather
	// than a list.
	Using string `json:"using,omitempty"`
	// Conditions gate the field. Empty means GitLab sends it with every
	// response of every endpoint that renders this entity.
	Conditions []Condition `json:"conditions,omitempty"`
}

// Condition is one gate on an exposure.
type Condition struct {
	// Kind is Grape's own class name for it: BlockCondition for a lambda,
	// HashCondition for `if: {…}`.
	Kind string `json:"kind"`
	// Inverse is true for `unless:`.
	Inverse bool `json:"inverse,omitempty"`
	// File and Line locate a block condition's lambda, repository relative.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	// Text is those lines read back from inside the image and squeezed onto
	// one. A Proc knows where it was written and not what it says, so this is
	// the only way the condition arrives readable, and it is what a rule
	// classifies on.
	Text string `json:"text,omitempty"`
	// Hash is a hash condition's own data.
	Hash string `json:"hash,omitempty"`
}

// Route is one mounted endpoint.
type Route struct {
	Method string `json:"method"`
	// Path is Grape's origin, without the (.:format) suffix the router adds,
	// since no caller sends that.
	Path string `json:"path"`
	// Entity is what the endpoint's `desc … success/entity` annotation names,
	// which is the same annotation GitLab's OpenAPI generator reads. It can be
	// wrong about what the endpoint really presents, and being wrong in the
	// record is better than being absent: GET /keys is annotated
	// APIEntitiesUserWithAdmin and serves an SSH key with a user under it.
	Entity  string `json:"entity,omitempty"`
	Summary string `json:"summary,omitempty"`
	// Params is what the endpoint declares it accepts, keyed by name.
	Params map[string]Param `json:"params,omitempty"`
}

// Param is one declared parameter.
type Param struct {
	Required bool   `json:"required,omitempty"`
	Type     string `json:"type,omitempty"`
	Default  string `json:"default,omitempty"`
	Desc     string `json:"desc,omitempty"`
}

// Path is where the record lives under a repository root.
func Path(dir string) string { return filepath.Join(dir, FileName) }

// Read loads the committed record.
//
// A schema version this build was not written for is refused rather than
// decoded: the fields would parse and mean something else, which is the one
// failure a reader cannot detect later.
func Read(dir string) (Document, error) {
	raw, err := os.ReadFile(Path(dir))
	if err != nil {
		return Document{}, fmt.Errorf("reading the live API record: %w", err)
	}
	var doc Document
	if decodeErr := json.Unmarshal(raw, &doc); decodeErr != nil {
		return Document{}, fmt.Errorf("decoding the live API record: %w", decodeErr)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf(
			"the live API record is schema version %d and this build reads version %d: regenerate it with make gen-api-live",
			doc.SchemaVersion, SchemaVersion,
		)
	}
	return doc, nil
}

// Write commits the record, formatted so a re-pin is a readable diff rather
// than one very long line.
func Write(dir string, doc Document) error {
	encoded, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return fmt.Errorf("encoding the live API record: %w", err)
	}
	encoded = append(encoded, '\n')
	if writeErr := os.WriteFile(Path(dir), encoded, 0o600); writeErr != nil {
		return fmt.Errorf("writing the live API record: %w", writeErr)
	}
	return nil
}

// OpenAPIName is the entity's Ruby name in the spelling GitLab's OpenAPI
// document gives its schema, so a reader holding one can look up the other.
//
// API::Entities::Ci::Variable becomes APIEntitiesCiVariable: the separators go
// and the segments keep their own casing.
func OpenAPIName(rubyName string) string {
	var out strings.Builder
	for segment := range strings.SplitSeq(rubyName, "::") {
		out.WriteString(segment)
	}
	return out.String()
}

// Names lists the entities in a stable order.
func (d Document) Names() []string {
	names := make([]string, 0, len(d.Entities))
	for name := range d.Entities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FieldCount totals the exposures across every entity, which is the figure a
// floor is set against: a record that lost half its fields is one an audit
// would read as GitLab having stopped sending them.
func (d Document) FieldCount() int {
	total := 0
	for _, entity := range d.Entities {
		total += len(entity.Fields)
	}
	return total
}

// Tier resolves the highest tier the named features unlock at.
//
// Ultimate wins over premium, and premium over global, because a field gated
// by two features needs the dearer plan. A feature the table does not list
// resolves to nothing: a project setting such as :issues is not a license.
func (d Document) Tier(features ...string) string {
	best := ""
	for _, feature := range features {
		switch d.Features[feature] {
		case TierUltimate:
			return TierUltimate
		case TierPremium:
			best = TierPremium
		case TierGlobal:
			if best == "" {
				best = TierGlobal
			}
		}
	}
	return best
}

// RoutesByEntity indexes the endpoints each entity is annotated on, which is
// the join an audit walks: from a type, to the entity it models, to the
// endpoints that render it.
func (d Document) RoutesByEntity() map[string][]Route {
	index := map[string][]Route{}
	for _, route := range d.Routes {
		if route.Entity == "" {
			continue
		}
		index[route.Entity] = append(index[route.Entity], route)
	}
	return index
}

// String renders the provenance as one reportable line.
func (s Source) String() string {
	return fmt.Sprintf(
		"%d entities (%d fields), %d routes and %d licensed features from GitLab %s (%s), retrieved %s",
		s.Entities, s.Fields, s.Routes, s.Features, s.Version, s.Image, s.RetrievedAt,
	)
}
