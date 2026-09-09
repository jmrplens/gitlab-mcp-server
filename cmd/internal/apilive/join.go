package apilive

import (
	"regexp"
	"sort"
	"strings"
)

// EndpointPrefix is what Grape mounts every route under. A caller writes
// /projects/:id and Grape holds /api/:version/projects/:id, so one of the two
// spellings has to give, and it is this one: the inventory records what the
// SDK sent, which is what a reader of a finding will go looking for.
const EndpointPrefix = "/api/:version"

// NormalizePath trims the mount prefix off a route, leaving the path a caller
// writes.
//
// Grape's own placeholders are already the spelling the request inventory
// records (:id, :user_id), so nothing else has to be rewritten. That is a
// property of asking the router rather than reading a generated document: the
// OpenAPI record spells the same placeholder {id} and had to be translated.
func NormalizePath(path string) string {
	trimmed := strings.TrimPrefix(path, EndpointPrefix)
	if trimmed == "" {
		return "/"
	}
	return trimmed
}

// Fields is the entity's exposures by the key GitLab sends, the last
// declaration of a name winning as Grape's own render does, and false for an
// entity this record does not hold.
//
// Nothing here has to be resolved first: the introspection reads
// root_exposures, which is the class as it will render, with everything it
// inherits and everything an Enterprise module prepended already flattened in.
// The scanned record this replaced had to walk a parent chain to reach the same
// list, and could not see a prepend it had no file for.
func (d Document) Fields(entity string) (map[string]Field, bool) {
	found, ok := d.Entities[entity]
	if !ok {
		return nil, false
	}
	byName := make(map[string]Field, len(found.Fields))
	for _, field := range found.Fields {
		byName[field.Name] = field
	}
	return byName, true
}

// FieldNames lists the keys an entity sends, sorted, and nil for one this
// record does not hold. It is what an audit holds a response against.
func (d Document) FieldNames(entity string) []string {
	fields, ok := d.Entities[entity]
	if !ok {
		return nil
	}
	names := make([]string, 0, len(fields.Fields))
	seen := make(map[string]bool, len(fields.Fields))
	for _, field := range fields.Fields {
		if !seen[field.Name] {
			seen[field.Name] = true
			names = append(names, field.Name)
		}
	}
	sort.Strings(names)
	return names
}

// NestedNames is, for each field of the entity that renders with an entity of
// its own, the keys that child sends.
//
// This is the tree the record carries and a generated OpenAPI document
// flattens: `expose :author, using: Entities::UserBasic` says exactly which
// object sits under `author`, so a nested comparison joins on the edge rather
// than on a property name that happened to describe an object.
func (d Document) NestedNames(entity string) map[string][]string {
	found, ok := d.Entities[entity]
	if !ok {
		return nil
	}
	var nested map[string][]string
	for _, field := range found.Fields {
		if field.Using == "" {
			continue
		}
		names := d.FieldNames(field.Using)
		if len(names) == 0 {
			continue
		}
		if nested == nil {
			nested = map[string][]string{}
		}
		nested[field.Name] = names
	}
	return nested
}

// licensedFeature is a symbol a condition asks a license about, in the
// spellings the entities use: a model's own feature_available?, the licensed_
// prefixed form, and ::License.feature_available?.
var licensedFeature = regexp.MustCompile(`(?:licensed_feature_available\?|feature_available\?)\(\s*:([a-z0-9_]+)`)

// LicensedFeatures names the features a condition asks a license about, in
// order of appearance and without repeats.
//
// A symbol found here is not necessarily licensed: feature_available?(:issues)
// asks a project setting, not a plan. [Document.Tier] is what tells them apart,
// because only the licensed feature table can.
func LicensedFeatures(condition string) []string {
	var symbols []string
	seen := map[string]bool{}
	for _, match := range licensedFeature.FindAllStringSubmatch(condition, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			symbols = append(symbols, match[1])
		}
	}
	return symbols
}

// Gate is what a field's conditions amount to, in the terms a finding reports.
type Gate struct {
	// If and Unless are the condition texts, joined with " && " when a field
	// carries several, since Grape requires all of them to pass.
	If     string
	Unless string
	// Tier is the highest plan the conditions demand, empty when they demand
	// none.
	Tier string
	// Edition is "ee" when a condition was written under ee/, which is where
	// Enterprise prepends live.
	//
	// It is read from the condition's own file, so a field with no condition
	// has no location to answer from and reports nothing: an exposure records
	// where its lambda was written and not where the exposure itself was.
	Edition string
}

// Gated reports whether the field is sent under any condition at all.
func (g Gate) Gated() bool { return g.If != "" || g.Unless != "" }

// GateOf reads what a field's conditions demand, resolving the tier against
// this record's own licensed feature table.
func (d Document) GateOf(field Field) Gate {
	var gate Gate
	var ifs, unlesses, features []string
	for _, condition := range field.Conditions {
		text := condition.Text
		if text == "" {
			text = condition.Hash
		}
		if condition.Inverse {
			unlesses = append(unlesses, text)
		} else {
			ifs = append(ifs, text)
		}
		features = append(features, LicensedFeatures(text)...)
		if strings.HasPrefix(condition.File, "ee/") {
			gate.Edition = "ee"
		}
	}
	gate.If = strings.Join(nonEmpty(ifs), " && ")
	gate.Unless = strings.Join(nonEmpty(unlesses), " && ")
	gate.Tier = d.Tier(features...)
	return gate
}

// nonEmpty drops the conditions that carry no text, which a hash condition
// with no data does. Joining them would produce a leading or doubled
// separator that reads as a condition somebody forgot to write down.
func nonEmpty(texts []string) []string {
	out := texts[:0:0]
	for _, text := range texts {
		if strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}
