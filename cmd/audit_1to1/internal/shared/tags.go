package shared

import (
	"reflect"
	"strings"
)

// NormalizeSDKTag maps a client-go url-tag name to the snake_case json name
// the MCP inputs use: trailing array notation ("iids[]" → "iids") and bracket
// negation notation ("not[author_id]" → "not_author_id").
//
// GraphQL-backed SDK structs tag fields in camelCase (createdAt, targetBranch)
// where the MCP output uses the project's snake_case convention, so the result
// is also lowered to snake_case. Callers run this only in the fallback path,
// after an exact-tag match fails, so it can only ADD a match (camelCase SDK
// tag <-> snake_case MCP tag), never break an exact one.
func NormalizeSDKTag(tag string) string {
	tag = strings.TrimSuffix(tag, "[]")
	tag = strings.ReplaceAll(tag, "[", "_")
	tag = strings.ReplaceAll(tag, "]", "")
	return camelToSnake(tag)
}

// camelToSnake lowercases a camelCase identifier with underscore separators
// (createdAt -> created_at). A snake_case input is returned unchanged (no
// uppercase).
func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TagName returns the name carried by the first of keys present in raw, with
// any ",omitempty"-style options stripped. It is "" when none of the keys
// names the field, and "-" when the field is explicitly excluded.
func TagName(raw reflect.StructTag, keys []string) string {
	for _, key := range keys {
		if value, ok := raw.Lookup(key); ok {
			name, _, _ := strings.Cut(value, ",")
			if name != "" {
				return name
			}
		}
	}
	return ""
}
