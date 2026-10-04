package apilive

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/cmdutil"
)

// SchemaVersion is the shape of this record. A reader refuses a version it was
// not written for rather than guessing at a field that moved.
//
// Version 2 records [Field.Merge]. The bump is not cosmetic: version 1 spelled
// a merged exposure exactly like a nested one, so a reader of an old record
// would resolve it into the wrong keys and never know, which is the one
// failure a version guard exists to stop.
//
// Version 3 records what a hash or a symbol condition tests, [Condition.Hash]
// and [Condition.Symbol]. Version 2 had the hash key and never filled it, and
// had no key for a symbol at all, so every such condition reached the record
// as its kind alone: 41 of the 914 on 19.3.1-ee, each the only gate on its
// field. A reader of that record would take those 41 fields for unconditional,
// which is the same silent inversion version 2 was cut for.
//
// Version 4 records fine-grained authorization: [Route.Authorization],
// [Document.Granular] and [Document.GraphQLAuthz]. In version 4 a route with
// no authorization declares nothing, so a fine-grained token is refused there;
// in version 3 the same absence meant "not recorded". A reader taking a
// version 3 record for version 4 would deny every route, which is the silent
// inversion the version exists to stop.
const SchemaVersion = 4

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
	// Granular is the permission vocabulary a fine-grained token is granted
	// in, and GraphQLAuthz what the GraphQL schema demands of one. A version 4
	// record without either cannot answer what a fine-grained token reaches,
	// and the gate refuses it.
	Granular     *Granular     `json:"granular,omitempty"`
	GraphQLAuthz *GraphQLAuthz `json:"graphql_authz,omitempty"`
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
	// AuthorizationCounts are the fine-grained figures, counted from the
	// record when it is written.
	AuthorizationCounts
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
	// Merge is set for an exposure whose value is merged into the object
	// around it instead of being placed under Name: `expose :user, merge:
	// true, using: UserBasic` on a member sends the user's own keys on the
	// member, and no `user` key at all.
	//
	// Recording it is what keeps the record from reading as the opposite of
	// what GitLab sends. Without it a member said it carries a `user` object
	// and said nothing about the id, username and name it really carries, so
	// an audit reported the one key GitLab never sends as missing and the
	// nine it does send as invented.
	Merge bool `json:"merge,omitempty"`
	// Conditions gate the field. Empty means GitLab sends it with every
	// response of every endpoint that renders this entity.
	Conditions []Condition `json:"conditions,omitempty"`
}

// Condition is one gate on an exposure.
type Condition struct {
	// Kind is Grape's own class name for it: BlockCondition for a lambda,
	// HashCondition for `if: {…}`, SymbolCondition for `if: :option`.
	Kind string `json:"kind"`
	// Inverse is true for `unless:`.
	Inverse bool `json:"inverse,omitempty"`
	// File and Line locate a block condition's lambda, repository relative.
	// A hash or a symbol condition carries neither: grape-entity keeps the
	// options it tests and nothing about where the exposure declaring it was
	// written, and the exposure records no location of its own either.
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
	// Text is those lines read back from inside the image and squeezed onto
	// one. A Proc knows where it was written and not what it says, so this is
	// the only way the condition arrives readable, and it is what a rule
	// classifies on.
	Text string `json:"text,omitempty"`
	// Hash is a hash condition's own data, as Ruby inspects it: the options
	// the presenter must be given, and the values they must hold.
	Hash string `json:"hash,omitempty"`
	// Symbol is a symbol condition's option, without its colon: the field is
	// sent when the presenter is given that option with a truthy value, which
	// is a choice of the endpoint rendering it and never of the entity.
	Symbol string `json:"symbol,omitempty"`
}

// Readable reports whether the condition says what it tests: a block
// condition's text, a hash condition's data or a symbol condition's option.
//
// One that carries none of them still gates its field, since Grape skips the
// exposure whenever it fails, but by something the record cannot name. That
// is what an introspection produces when it meets a condition kind it does not
// read, and it is what version 2 of the record did to every hash and symbol
// condition it held.
func (c Condition) Readable() bool {
	return strings.TrimSpace(c.Text) != "" || strings.TrimSpace(c.Hash) != "" || strings.TrimSpace(c.Symbol) != ""
}

// Describe renders the condition the way a finding quotes it.
//
// A block condition is its own text, which already reads as written. A symbol
// and a hash condition are spelled the way Grape declares them, `if: :option`
// and `if: {…}`, or `unless:` for an inverse one, because the bare option name
// reads as a field rather than as a gate. A condition carrying none of the
// three is never rendered as nothing: it is named by its kind, so a field it
// gates reads as gated by something unreadable rather than as gated by
// nothing, which is the opposite of what the record holds.
func (c Condition) Describe() string {
	if strings.TrimSpace(c.Text) != "" {
		return c.Text
	}
	keyword := "if:"
	if c.Inverse {
		keyword = "unless:"
	}
	if strings.TrimSpace(c.Symbol) != "" {
		return keyword + " :" + c.Symbol
	}
	if strings.TrimSpace(c.Hash) != "" {
		return keyword + " " + c.Hash
	}
	kind := c.Kind
	if kind == "" {
		kind = "condition"
	}
	return keyword + " (unreadable " + kind + ")"
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
	// Authorization is what the route demands of a fine-grained token. nil
	// means it declares nothing, which GitLab answers a fine-grained token by
	// refusing it.
	Authorization *RouteAuthorization `json:"authorization,omitempty"`
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
	// Marshaling a struct of strings, numbers, slices and maps cannot fail, and
	// this repository's rule for that is to say so at the leaf rather than carry
	// a branch no test can reach.
	encoded := append(cmdutil.Must(json.MarshalIndent(doc, "", " ")), '\n')
	// The directory is created rather than required: the generator writes into
	// a checkout that has it, and a test writes into a temporary root that does
	// not, and neither should have to know which case it is in.
	if dirErr := os.MkdirAll(dir, 0o750); dirErr != nil {
		return fmt.Errorf("preparing the directory for the live API record: %w", dirErr)
	}
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
