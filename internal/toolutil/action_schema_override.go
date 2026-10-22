package toolutil

import (
	"fmt"
	"slices"
	"strings"
)

// SchemaPropertyOverride returns an input-schema override for a property path.
func SchemaPropertyOverride(propertyPath string, values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{PropertyPath: strings.TrimSpace(propertyPath), Values: cloneSchemaMap(values)}
}

// SchemaEnumOverride returns an input-schema override that constrains the string
// parameter at propertyPath to a fixed set of enum values. It collapses the
// repeated `SchemaPropertyOverride(path, map[string]any{"enum": []any{...}})`
// block used across domains into a single call so the fixed-vocabulary
// constraint is expressed once per action without structural duplication.
func SchemaEnumOverride(propertyPath string, values ...string) InputSchemaOverride {
	enum := make([]any, len(values))
	for i, v := range values {
		enum[i] = v
	}
	return SchemaPropertyOverride(propertyPath, map[string]any{"enum": enum})
}

// SchemaFormatOverride returns an input-schema override that sets the JSON
// Schema `format` of the string parameter at propertyPath (e.g. "date" for a
// YYYY-MM-DD field or "uri" for a URL field).
func SchemaFormatOverride(propertyPath, format string) InputSchemaOverride {
	return SchemaPropertyOverride(propertyPath, map[string]any{"format": format})
}

// SchemaApproverIDsOverride widens the array item type of an
// [ApproverIDsFilter] parameter so both numeric user IDs and the "Any"/"None"
// literals pass input validation. Without it the reflected schema advertises
// strings only, which would reject the numeric IDs every existing caller
// sends.
func SchemaApproverIDsOverride(propertyPath string) InputSchemaOverride {
	return SchemaPropertyOverride(propertyPath, map[string]any{
		"items": map[string]any{"type": []any{"integer", "string"}},
	})
}

// SchemaRootOverride returns an input-schema override applied at the schema root.
func SchemaRootOverride(values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{Values: cloneSchemaMap(values)}
}

// SchemaAnyOfRequired returns a root override that requires at least one of the
// supplied property names to be present: GitLab's at_least_one_of.
func SchemaAnyOfRequired(propertyNames ...string) InputSchemaOverride {
	return requiredAlternatives("anyOf", propertyNames, presentBranch)
}

// SchemaOneOfRequired returns a root override that requires exactly one of the
// supplied property names to carry a value: GitLab's exactly_one_of, which
// refuses a call that sends two of them as it refuses one that sends none.
//
// A name counts only when its value is not an empty string. A handler decodes
// such a field into a string and leaves an empty one out of the request, so
// GitLab is asked with the names that carry a value, and those are the ones
// its exactly_one_of counts. A branch counting the name alone would refuse a
// call with a common name beside an empty filter as naming both, and the
// handler serves that call by sending the common name.
//
// Only a schema validator holds a call to one branch: the SDK's, on the
// individual tool and on the meta tool when its parameter schema is full. The
// meta dispatcher checks the root required list alone, and the dynamic one
// reads the branches as the alternative groups find publishes and refuses a
// call that completes none of them, not one that completes two. So the
// handler of an action carrying this override refuses a call naming more than
// one of the names before it sends anything, as groupldap's do.
func SchemaOneOfRequired(propertyNames ...string) InputSchemaOverride {
	return requiredAlternatives("oneOf", propertyNames, valuedBranch)
}

// requiredAlternatives returns a root override holding, under keyword, one
// branch per property name, built by branch, and no override when no name is
// left, which [ActionSpec.Validate] refuses as empty.
func requiredAlternatives(keyword string, propertyNames []string, branch func(string) map[string]any) InputSchemaOverride {
	branches := alternativeBranches(propertyNames, branch)
	if len(branches) == 0 {
		return InputSchemaOverride{}
	}
	return SchemaRootOverride(map[string]any{keyword: branches})
}

// alternativeBranches returns one branch per property name, built by branch,
// in the order the names come.
func alternativeBranches(propertyNames []string, branch func(string) map[string]any) []any {
	names := alternativeNames(propertyNames)
	branches := make([]any, 0, len(names))
	for _, name := range names {
		branches = append(branches, branch(name))
	}
	return branches
}

// alternativeNames returns the property names an alternative is built of:
// trimmed, a blank one left out and a repeated one kept once.
//
// A repeated name matters to oneOf: a call sending it would satisfy both of
// its branches, and oneOf refuses a call that satisfies more than one, so
// every call naming it would be refused.
func alternativeNames(propertyNames []string) []string {
	names := make([]string, 0, len(propertyNames))
	for _, propertyName := range propertyNames {
		propertyName = strings.TrimSpace(propertyName)
		if propertyName != "" && !slices.Contains(names, propertyName) {
			names = append(names, propertyName)
		}
	}
	return names
}

// SchemaItemsOverride returns an input-schema override applied to the item
// schema of the list parameter at propertyPath rather than to the list: what
// each element of it must be. A path that reaches no list is no target, and
// [ActionSpec.Validate] refuses it.
func SchemaItemsOverride(propertyPath string, values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{PropertyPath: strings.TrimSpace(propertyPath), Items: true, Values: cloneSchemaMap(values)}
}

// SchemaItemsOneOfRequired returns an items override that requires each
// element of the list at propertyPath to carry exactly one of propertyNames
// with a value that is not null: GitLab's ExactlyOnePresentValidator on the
// records a list of nested attributes creates, which refuses an element
// naming two as it refuses one naming none.
//
// Only a schema validator holds an element to one branch, as with
// [SchemaOneOfRequired]: the meta and the dynamic dispatchers read no item
// schema at all, so the handler of an action carrying this override refuses
// such an element itself before it sends anything.
func SchemaItemsOneOfRequired(propertyPath string, propertyNames ...string) InputSchemaOverride {
	return itemsAlternatives(propertyPath, "oneOf", alternativeBranches(propertyNames, nonNullBranch))
}

// SchemaItemsAnyOfRequired returns an items override that requires each
// element of the list at propertyPath to carry at least one of propertyNames
// with a value that is not null: GitLab's at_least_one_of inside a list's
// param block.
func SchemaItemsAnyOfRequired(propertyPath string, propertyNames ...string) InputSchemaOverride {
	return itemsAlternatives(propertyPath, "anyOf", alternativeBranches(propertyNames, nonNullBranch))
}

// SchemaItemsAnyOfRequiredUnlessTrue is [SchemaItemsAnyOfRequired] with an
// exemption: an element whose flag is true needs none of the names. It is
// Grape's at_least_one_of inside a given block on that flag, the shape GitLab
// gives a list of nested attributes whose _destroy removes an element instead
// of describing one. A blank flag exempts nothing.
func SchemaItemsAnyOfRequiredUnlessTrue(propertyPath, flag string, propertyNames ...string) InputSchemaOverride {
	branches := alternativeBranches(propertyNames, nonNullBranch)
	if flag = strings.TrimSpace(flag); flag != "" && len(branches) > 0 {
		branches = append(branches, trueBranch(flag))
	}
	return itemsAlternatives(propertyPath, "anyOf", branches)
}

// SchemaItemsAtMostOneOf returns an items override that refuses an element of
// the list at propertyPath carrying two or more of propertyNames with a value
// that is not null, and asks nothing of one carrying none: the half of
// exactly one that an element naming an existing record by its id is held
// to, where the id may stand in for the names. Fewer than two names
// constrain nothing and are no override.
//
// It is a not over one branch per pair of names. The lockdown does not
// descend into not, and each pair leaves the element's other properties open
// anyway, since a closed pair would match only an element carrying the pair
// and nothing else, and the not would let every other element through.
func SchemaItemsAtMostOneOf(propertyPath string, propertyNames ...string) InputSchemaOverride {
	names := alternativeNames(propertyNames)
	var pairs []any
	for i, first := range names {
		for _, second := range names[i+1:] {
			pairs = append(pairs, nonNullPropertiesBranch(first, second))
		}
	}
	if len(pairs) == 0 {
		return InputSchemaOverride{}
	}
	return SchemaItemsOverride(propertyPath, map[string]any{"not": map[string]any{"anyOf": pairs}})
}

// itemsAlternatives returns an items override holding branches under
// keyword, and no override when there are none.
func itemsAlternatives(propertyPath, keyword string, branches []any) InputSchemaOverride {
	if len(branches) == 0 {
		return InputSchemaOverride{}
	}
	return SchemaItemsOverride(propertyPath, map[string]any{keyword: branches})
}

// nonNullBranch is an items branch: the name present with a value that is not
// null.
func nonNullBranch(propertyName string) map[string]any {
	return nonNullPropertiesBranch(propertyName)
}

// nonNullPropertiesBranch is a branch requiring every one of propertyNames
// with a value that is not null. An element's fields decode into pointers,
// which a null leaves nil, and a handler sends the fields that are not nil,
// so a name counts as GitLab will count it only once its value is not null.
// The branch declares no type and leaves the element's other properties open,
// for the reason [valuedBranch] gives.
func nonNullPropertiesBranch(propertyNames ...string) map[string]any {
	properties := make(map[string]any, len(propertyNames))
	for _, name := range propertyNames {
		properties[name] = map[string]any{"not": map[string]any{"type": "null"}}
	}
	return map[string]any{"required": propertyNames, "properties": properties, "additionalProperties": true}
}

// trueBranch is a branch requiring flag to be true, the exemption of
// [SchemaItemsAnyOfRequiredUnlessTrue].
func trueBranch(flag string) map[string]any {
	return map[string]any{
		"required":             []string{flag},
		"properties":           map[string]any{flag: map[string]any{"const": true}},
		"additionalProperties": true,
	}
}

// presentBranch is an anyOf branch: the name present, whatever its value. It
// declares nothing but its required list, a type least of all.
// [LockdownInputSchemas] closes every object schema it reaches through anyOf
// or oneOf, and a branch typed as an object would be closed with no
// properties of its own, refusing every property a call sends.
func presentBranch(propertyName string) map[string]any {
	return map[string]any{"required": []string{propertyName}}
}

// valuedBranch is a oneOf branch: the name present with a value that is not
// an empty string. minLength holds a string alone, so a name of another type
// counts as soon as it is present. The branch declares no type and leaves the
// call's other properties open, because the lockdown closes a schema that has
// properties and no type as well, and closed it would refuse every property
// but the one it names. The root stays closed, so a call still sends nothing
// the schema does not name.
func valuedBranch(propertyName string) map[string]any {
	return map[string]any{
		"required":             []string{propertyName},
		"properties":           map[string]any{propertyName: map[string]any{"minLength": 1}},
		"additionalProperties": true,
	}
}

// applyInputSchemaOverrides patches schema in place with each override that
// names a property the schema carries.
//
// An action with no input schema at all reaches this with a nil map, and the
// nil is answered where every other unreachable path is: schemaOverrideTarget
// resolves nothing in it and the override is skipped. Refusing it here as well
// only meant that no test could tell the two refusals apart.
func applyInputSchemaOverrides(schema map[string]any, overrides []InputSchemaOverride) {
	if len(overrides) == 0 {
		return
	}
	for _, override := range overrides {
		target := overrideTarget(schema, override)
		if target == nil {
			continue
		}
		for key, value := range override.Values {
			target[key] = cloneSchemaValue(value)
		}
	}
}

// canonicalParamEnums maps GitLab-universal input parameter names to their fixed
// value set. These parameters carry the SAME enum across every endpoint that
// accepts them, so the enum is injected centrally (single source of truth) into
// every action's input schema by [NewActionSpec], rather than repeated per
// action. Only add a parameter here when its value set is identical across ALL
// GitLab endpoints; resource-specific value sets (order_by, state, scope, type,
// ...) differ per endpoint and must be set per action via InputSchemaOverrides.
var canonicalParamEnums = map[string][]string{
	"sort":          {"asc", "desc"},
	"visibility":    {"private", "internal", "public"},
	"variable_type": {"env_var", "file"},
}

// JSON Schema `format` values for date/time string parameters. GitLab timestamp
// filters take an ISO 8601 date-time; date-only fields take YYYY-MM-DD.
const (
	formatDateTime = "date-time"
	formatDate     = "date"
	formatURI      = "uri"
)

// canonicalParamFormats maps GitLab-universal date/time input parameters to
// their JSON Schema `format`. GitLab dates are always ISO 8601 — never locale
// ordered (DD/MM vs MM/DD): timestamp filters take an ISO 8601 date-time and
// date-only fields take YYYY-MM-DD. These names carry the same format across
// every endpoint, so the format is injected centrally (single source of truth)
// by [NewActionSpec]. expires_at is date-only (YYYY-MM-DD) for the access
// tokens, members, service accounts, invites, and share endpoints (client-go
// *ISOTime); the two deploy endpoints that take a full ISO 8601 timestamp
// (client-go *time.Time — deploy keys and deploy tokens) override it back to
// date-time per action via [SchemaFormatOverride].
var canonicalParamFormats = map[string]string{
	"created_after":        formatDateTime,
	"created_before":       formatDateTime,
	"updated_after":        formatDateTime,
	"updated_before":       formatDateTime,
	"last_used_after":      formatDateTime,
	"last_used_before":     formatDateTime,
	"last_activity_after":  formatDateTime,
	"last_activity_before": formatDateTime,
	"started_after":        formatDateTime,
	"started_before":       formatDateTime,
	"finished_after":       formatDateTime,
	"finished_before":      formatDateTime,
	"deployed_after":       formatDateTime,
	"deployed_before":      formatDateTime,
	"expires_after":        formatDateTime,
	"expires_before":       formatDateTime,
	"expires_at":           formatDate,
	"due_date":             formatDate,
	"start_date":           formatDate,
	// Input parameters that take an absolute URL. JSON Schema `uri` is advisory
	// (it does not reject input) and only documents the expected shape for the
	// model. Restricted to parameter names that are always a full URL across
	// GitLab endpoints (webhooks, mirrors, badges, release links, Jira/Bitbucket
	// integration); url_text and similar label fields are intentionally absent.
	"url":                  formatURI,
	"link_url":             formatURI,
	"image_url":            formatURI,
	"external_url":         formatURI,
	"remote_url":           formatURI,
	"bitbucket_server_url": formatURI,
}

// canonicalParamRange is an inclusive integer bound for a numeric parameter.
// Every parameter the table bounds has a floor, since each is a page or a page
// size, so Min is a value; a nil Max leaves the ceiling unconstrained.
type canonicalParamRange struct {
	Min int
	Max *int
}

// canonicalParamRanges maps GitLab-universal integer pagination parameters to
// their inclusive bounds, injected centrally by [NewActionSpec]. The GitLab
// REST API documents per_page as default 20 / max 100 for offset-based
// pagination, applied uniformly to every offset-paginated endpoint (the users
// endpoint additionally switches to keyset above 50,000 records, but its
// per_page cap is still 100). The maximum is therefore doc-grounded and safe.
// The minimum of 1 for per_page and page is the effective floor — GitLab treats
// a page size below 1 as "use the default" and pages are 1-based — rather than
// an explicitly documented value; it is included to steer the model away from
// requesting 0 or negative page sizes.
//
// See https://docs.gitlab.com/api/rest/#offset-based-pagination.
var canonicalParamRanges = map[string]canonicalParamRange{
	"per_page": {Min: 1, Max: new(100)},
	"page":     {Min: 1},
}

// applyCanonicalParamRanges injects [canonicalParamRanges] bounds into top-level
// integer properties of schema that do not already declare the corresponding
// bound. Per-action InputSchemaOverrides are applied first (see [NewActionSpec])
// and therefore win: a bound a property already carries is left untouched.
func applyCanonicalParamRanges(schema map[string]any) {
	if schema == nil {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, bound := range canonicalParamRanges {
		prop, isMap := props[name].(map[string]any)
		if !isMap || prop["type"] != "integer" {
			continue
		}
		if _, has := prop["minimum"]; !has {
			prop["minimum"] = bound.Min
		}
		if bound.Max != nil {
			if _, has := prop["maximum"]; !has {
				prop["maximum"] = *bound.Max
			}
		}
	}
}

// applyCanonicalParamFormats injects [canonicalParamFormats] into top-level
// string properties of schema that do not already declare a `format`. Per-action
// InputSchemaOverrides win (applied first by [NewActionSpec]); a property that
// already carries a format is left untouched, and so is one carrying an enum:
// a closed set of keywords is not a date, whatever the parameter is called.
// The issue lists' due_date is such a set, and a date format on it told a
// model to send a date that GitLab matches against no keyword.
func applyCanonicalParamFormats(schema map[string]any) {
	if schema == nil {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, format := range canonicalParamFormats {
		prop, isMap := props[name].(map[string]any)
		if !isMap || prop["type"] != "string" {
			continue
		}
		if _, has := prop["format"]; has {
			continue
		}
		if _, keyword := prop["enum"]; keyword {
			continue
		}
		prop["format"] = format
	}
}

// applyCanonicalParamEnums injects the [canonicalParamEnums] value sets into the
// top-level string properties of schema that do not already declare an enum.
// Per-action InputSchemaOverrides are applied before this (see [NewActionSpec])
// and therefore win: a property that already carries an enum is left untouched.
func applyCanonicalParamEnums(schema map[string]any) {
	if schema == nil {
		return
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, values := range canonicalParamEnums {
		prop, isMap := props[name].(map[string]any)
		if !isMap || prop["type"] != "string" {
			continue
		}
		if _, has := prop["enum"]; has {
			continue
		}
		enum := make([]any, len(values))
		for i, v := range values {
			enum[i] = v
		}
		prop["enum"] = enum
	}
}

// FilterOverridesForSchema returns the subset of overrides whose property path
// still resolves against schema (an empty path targets the root and is always
// kept). It is used after tier pruning removes higher-tier properties: an
// override that targeted a now-removed field (e.g. an ultimate-only parameter
// pruned for a Free instance) is dropped so it does not fail
// [validateInputSchemaOverrides] during catalog assembly. The enum/patch values
// were already applied to the property before pruning, so dropping the override
// here only keeps the override list consistent with the pruned schema.
func FilterOverridesForSchema(schema map[string]any, overrides []InputSchemaOverride) []InputSchemaOverride {
	if len(overrides) == 0 {
		return overrides
	}
	out := make([]InputSchemaOverride, 0, len(overrides))
	for _, override := range overrides {
		if isRootOverride(override) || overrideTarget(schema, override) != nil {
			out = append(out, override)
		}
	}
	return out
}

// isRootOverride reports whether override patches the schema root, which an
// override with no path does unless it targets items: the root is no list.
func isRootOverride(override InputSchemaOverride) bool {
	return strings.TrimSpace(override.PropertyPath) == "" && !override.Items
}

// overrideTarget returns the schema node override patches: the property its
// path names, or the item schema of the list there when it targets items, and
// nil when the path reaches nothing or, for an items override, no list.
func overrideTarget(root map[string]any, override InputSchemaOverride) map[string]any {
	target := schemaOverrideTarget(root, strings.TrimSpace(override.PropertyPath))
	if !override.Items {
		return target
	}
	items, _ := target["items"].(map[string]any)
	return resolveSchemaRef(root, items)
}

func validateInputSchemaOverrides(spec ActionSpec) error {
	if len(spec.InputSchemaOverrides) == 0 {
		return nil
	}
	if spec.Route.InputSchema == nil {
		return fmt.Errorf("action spec %q has input schema overrides without an input schema", spec.Name)
	}
	for _, override := range spec.InputSchemaOverrides {
		if len(override.Values) == 0 {
			return fmt.Errorf("action spec %q has empty input schema override", spec.Name)
		}
		if isRootOverride(override) || overrideTarget(spec.Route.InputSchema, override) != nil {
			continue
		}
		propertyPath := strings.TrimSpace(override.PropertyPath)
		if override.Items {
			return fmt.Errorf("action spec %q has input schema override for the items of %q, which is no list", spec.Name, propertyPath)
		}
		return fmt.Errorf("action spec %q has input schema override for unknown property path %q", spec.Name, propertyPath)
	}
	return nil
}

func schemaOverrideTarget(root map[string]any, propertyPath string) map[string]any {
	if propertyPath == "" {
		return root
	}
	parts := strings.Split(propertyPath, ".")
	return schemaOverrideTargetFrom(root, root, parts)
}

func schemaOverrideTargetFrom(root, schema map[string]any, parts []string) map[string]any {
	if len(parts) == 0 {
		return nil
	}
	schema = resolveSchemaRef(root, schema)
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	child, ok := properties[parts[0]].(map[string]any)
	if !ok {
		return nil
	}
	child = resolveSchemaRef(root, child)
	if len(parts) == 1 {
		return child
	}
	if child == nil {
		return nil
	}
	if items, hasItems := child["items"].(map[string]any); hasItems {
		child = resolveSchemaRef(root, items)
	}
	return schemaOverrideTargetFrom(root, child, parts[1:])
}

func cloneInputSchemaOverrides(overrides []InputSchemaOverride) []InputSchemaOverride {
	if len(overrides) == 0 {
		return nil
	}
	out := make([]InputSchemaOverride, 0, len(overrides))
	for _, override := range overrides {
		out = append(out, InputSchemaOverride{
			PropertyPath: strings.TrimSpace(override.PropertyPath),
			Items:        override.Items,
			Values:       cloneSchemaMap(override.Values),
		})
	}
	return out
}
