package apiexposes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/cmdutil"
)

const (
	// FileName is the committed record, beside the OpenAPI record it
	// qualifies.
	FileName = "gitlab-api-exposes.json"
	// DefaultDir is where the record lives, repository relative, the same
	// directory the OpenAPI record is written to so the two are read together.
	DefaultDir = "docs/development"
	// SchemaVersion is the artifact's shape. A reader that does not recognize
	// it must refuse rather than guess.
	SchemaVersion = 1
	// MinimumEntities is the floor an extraction has to clear to be GitLab's
	// whole entity tree. The three directories held 569 entity files on the
	// day this was written, declaring more classes than that, so a count well
	// under it is a truncated download or a tree that is not this one.
	MinimumEntities = 400
	// MinimumFeatures is the same floor for the feature table, which listed
	// several hundred symbols on that day.
	MinimumFeatures = 200
)

// Tiers, as the feature table names them. A symbol under a "starter" list is
// premium: GitLab folded that tier into Premium and kept the list.
const (
	TierPremium  = "premium"
	TierUltimate = "ultimate"
	// TierGlobal is the table's GLOBAL_FEATURES list: features every paid
	// plan carries, so a field behind one is licensed without being a tier.
	TierGlobal = "global"
)

// Document is the committed record.
type Document struct {
	SchemaVersion int    `json:"schema_version"`
	Note          string `json:"note"`
	Source        Source `json:"source"`
	// Entities is keyed by the name the OpenAPI document gives the entity's
	// schema, APIEntitiesProject for API::Entities::Project, so a reader
	// holding the OpenAPI record's entity name joins here without translating.
	Entities map[string]Entity `json:"entities"`
	// Features maps every licensed feature symbol to the tier that unlocks
	// it, from GitLab's feature table.
	Features map[string]string `json:"features"`
}

// Source is where the extraction came from.
type Source struct {
	// Ref is the ref asked for and Commit the one it resolved to on the day,
	// which is what makes two extractions comparable.
	Ref         string `json:"ref"`
	Commit      string `json:"commit"`
	RetrievedAt string `json:"retrieved_at"`
	// SHA256 is of the fetched sources, in the order they were fetched, not
	// of this file.
	SHA256   string `json:"sha256"`
	Files    int    `json:"files"`
	Entities int    `json:"entities"`
	Exposes  int    `json:"exposes"`
	Features int    `json:"features"`
}

// Entity is one Grape entity: a class under API::Entities.
type Entity struct {
	// File and Line are where the class is declared, repository relative.
	File string `json:"file"`
	Line int    `json:"line"`
	// Parent is the superclass in the same naming, or "" when the entity
	// descends from Grape::Entity directly.
	Parent string `json:"parent,omitempty"`
	// Edition is "ee" for an entity only Enterprise code declares.
	Edition string `json:"edition,omitempty"`
	// Fields are the exposes in declaration order, the Enterprise prepends
	// after the Community ones, without the parent's: see [Document.Effective].
	Fields []Field `json:"fields"`
}

// Field is one expose.
type Field struct {
	// Name is the key GitLab sends, after `as:`.
	Name string `json:"name"`
	// File is set when the field is declared in a file other than the
	// entity's own, which is what an Enterprise prepend is.
	File string `json:"file,omitempty"`
	Line int    `json:"line"`
	// Edition is "ee" for a field an Enterprise module prepends into a
	// Community entity.
	Edition string `json:"edition,omitempty"`
	// If and Unless are the conditions as written, an enclosing
	// with_options scope joined to the field's own with " && ".
	If     string `json:"if,omitempty"`
	Unless string `json:"unless,omitempty"`
	// Features are the licensed feature symbols the conditions name, and
	// Tier the highest tier the feature table puts one of them under. A
	// condition naming a symbol the table does not list has no tier: a
	// project feature such as :issues is a setting, not a license.
	Features []string `json:"features,omitempty"`
	Tier     string   `json:"tier,omitempty"`
	// Using names the entity the value is rendered with, in the same naming
	// as [Document.Entities], or the literal when it could not be resolved.
	Using string `json:"using,omitempty"`
	// Merge is `merge: true`: the used entity's fields land at this level
	// rather than under Name.
	Merge bool `json:"merge,omitempty"`
	// Nested are the fields an `expose :x do ... end` block declares under
	// this one.
	Nested []Field `json:"nested,omitempty"`
}

// String renders the provenance as one reportable line.
func (s Source) String() string {
	return fmt.Sprintf("%d entities and %d licensed features from gitlab-org/gitlab at %s (%s), retrieved %s",
		s.Entities, s.Features, s.Ref, shortCommit(s.Commit), s.RetrievedAt)
}

// shortCommit abbreviates a commit the way git does.
func shortCommit(commit string) string {
	if len(commit) > 8 {
		return commit[:8]
	}
	return commit
}

// OpenAPIName renders a Ruby constant path the way GitLab's generated OpenAPI
// document names the component for it: the segments run together with nothing
// between them, so API::Entities::Ci::Variable is APIEntitiesCiVariable.
func OpenAPIName(rubyPath string) string {
	return strings.ReplaceAll(strings.TrimPrefix(rubyPath, "::"), "::", "")
}

// Write commits the document to dir.
func Write(dir string, doc Document) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	doc.SchemaVersion = SchemaVersion
	// Marshaling a struct of strings, ints and slices cannot fail, and the
	// repository's rule for that is to say so at the leaf rather than carry a
	// branch no test can reach.
	encoded := cmdutil.Must(json.MarshalIndent(doc, "", " "))
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, append(encoded, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Read loads the committed document from dir.
func Read(dir string) (Document, error) {
	path := filepath.Join(dir, FileName)
	raw, err := os.ReadFile(path) //#nosec G304 -- the directory is the caller's own flag
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	var doc Document
	if parseErr := json.Unmarshal(raw, &doc); parseErr != nil {
		return Document{}, fmt.Errorf("parse %s: %w", path, parseErr)
	}
	if doc.SchemaVersion != SchemaVersion {
		return Document{}, fmt.Errorf("%s is schema version %d and this build reads %d: regenerate it with `make gen-api-exposes`",
			path, doc.SchemaVersion, SchemaVersion)
	}
	return doc, nil
}

// Names returns every entity name, sorted.
func (d Document) Names() []string {
	names := make([]string, 0, len(d.Entities))
	for name := range d.Entities {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Effective returns every field an entity sends, the way Grape assembles
// them: the parent chain's fields first, oldest ancestor first, then the
// entity's own, with a `merge: true` field replaced by the fields of the
// entity it merges. The second result is false for a name the record does
// not hold. A chain that loops, which Ruby would refuse to load, is cut where
// it repeats.
func (d Document) Effective(name string) ([]Field, bool) {
	if _, ok := d.Entities[name]; !ok {
		return nil, false
	}
	return d.effective(name, map[string]bool{}), true
}

func (d Document) effective(name string, walking map[string]bool) []Field {
	entity, ok := d.Entities[name]
	if !ok || walking[name] {
		return nil
	}
	walking[name] = true
	defer delete(walking, name)

	var fields []Field
	if entity.Parent != "" {
		fields = append(fields, d.effective(entity.Parent, walking)...)
	}
	for _, field := range entity.Fields {
		if !field.Merge {
			fields = append(fields, field)
			continue
		}
		for _, merged := range d.effective(field.Using, walking) {
			merged.If = joinConditions(field.If, merged.If)
			merged.Unless = joinConditions(field.Unless, merged.Unless)
			fields = append(fields, merged)
		}
	}
	return fields
}

// joinConditions combines an enclosing condition with an inner one.
func joinConditions(outer, inner string) string {
	switch {
	case outer == "":
		return inner
	case inner == "":
		return outer
	default:
		return outer + " && " + inner
	}
}
