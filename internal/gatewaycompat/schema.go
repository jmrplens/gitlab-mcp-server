package gatewaycompat

// schemaDataKeys are the schema keywords whose value is data rather than
// schema: a default, constant, enum or example is served back verbatim in
// calls, so a rewrite there would change behavior, not presentation. Their
// subtrees are never entered. The pattern keyword needs no entry — its value
// is a string, and a bare string is never rewritten without a prose keyword
// above it.
var schemaDataKeys = map[string]bool{
	"default": true, "const": true, "enum": true, "examples": true,
}

// schemaMapKeys are the schema keywords whose value maps user-chosen names to
// subschemas. Below them, a key is a property name or definition name, not a
// keyword: a property named "default" is an ordinary schema to descend, and a
// property named "description" holds a schema, not prose.
var schemaMapKeys = map[string]bool{
	"properties":        true,
	"patternProperties": true,
	"$defs":             true,
	"definitions":       true,
	"dependentSchemas":  true,
}

// RewriteSchemaProse walks a decoded JSON schema value, passes every prose
// string — the value of a description or title keyword, however deeply
// nested — to rewrite, stores what it returns, and reports whether anything
// changed. Everything that is not prose survives verbatim: names, patterns,
// and the data keywords (default, const, enum, examples), whose subtrees are
// never entered.
//
// It is exported for cmd/audit_gateway_chars, which scans the same strings
// this package rewrites: sharing the walk is what keeps "what the audit
// checks" and "what the knob can fix" the same set by construction.
func RewriteSchemaProse(v any, rewrite func(text string) string) bool {
	switch value := v.(type) {
	case map[string]any:
		return rewriteSchemaMap(value, rewrite)
	case []any:
		changed := false
		for _, inner := range value {
			if RewriteSchemaProse(inner, rewrite) {
				changed = true
			}
		}
		return changed
	}
	return false
}

// rewriteSchemaMap handles the keyword position: each key is a schema
// keyword, deciding whether its value is data (skipped), a name→schema map
// (children descended as schemas), prose (rewritten), or schema (descended).
func rewriteSchemaMap(value map[string]any, rewrite func(text string) string) bool {
	changed := false
	for key, inner := range value {
		switch {
		case schemaDataKeys[key]:
			// Data, not schema: never entered.
		case schemaMapKeys[key]:
			children, ok := inner.(map[string]any)
			if !ok {
				continue
			}
			for _, child := range children {
				if RewriteSchemaProse(child, rewrite) {
					changed = true
				}
			}
		case key == "description" || key == "title":
			text, ok := inner.(string)
			if !ok {
				continue
			}
			if rewritten := rewrite(text); rewritten != text {
				value[key] = rewritten
				changed = true
			}
		default:
			if RewriteSchemaProse(inner, rewrite) {
				changed = true
			}
		}
	}
	return changed
}
