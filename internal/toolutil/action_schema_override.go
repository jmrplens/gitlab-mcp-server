package toolutil

import (
	"fmt"
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

// SchemaRootOverride returns an input-schema override applied at the schema root.
func SchemaRootOverride(values map[string]any) InputSchemaOverride {
	return InputSchemaOverride{Values: cloneSchemaMap(values)}
}

// SchemaAnyOfRequired returns a root override that requires at least one of the
// supplied property names to be present.
func SchemaAnyOfRequired(propertyNames ...string) InputSchemaOverride {
	branches := make([]any, 0, len(propertyNames))
	for _, propertyName := range propertyNames {
		propertyName = strings.TrimSpace(propertyName)
		if propertyName == "" {
			continue
		}
		branches = append(branches, map[string]any{"required": []string{propertyName}})
	}
	if len(branches) == 0 {
		return InputSchemaOverride{}
	}
	return SchemaRootOverride(map[string]any{"anyOf": branches})
}

func applyInputSchemaOverrides(schema map[string]any, overrides []InputSchemaOverride) {
	if schema == nil || len(overrides) == 0 {
		return
	}
	for _, override := range overrides {
		target := schemaOverrideTarget(schema, strings.TrimSpace(override.PropertyPath))
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
// A nil Min or Max leaves that bound unconstrained.
type canonicalParamRange struct {
	Min *int
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
	"per_page": {Min: new(1), Max: new(100)},
	"page":     {Min: new(1)},
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
		if bound.Min != nil {
			if _, has := prop["minimum"]; !has {
				prop["minimum"] = *bound.Min
			}
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
// already carries a format is left untouched.
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
		path := strings.TrimSpace(override.PropertyPath)
		if path == "" || schemaOverrideTarget(schema, path) != nil {
			out = append(out, override)
		}
	}
	return out
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
		propertyPath := strings.TrimSpace(override.PropertyPath)
		if propertyPath == "" {
			continue
		}
		if schemaOverrideTarget(spec.Route.InputSchema, propertyPath) == nil {
			return fmt.Errorf("action spec %q has input schema override for unknown property path %q", spec.Name, propertyPath)
		}
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
			Values:       cloneSchemaMap(override.Values),
		})
	}
	return out
}
