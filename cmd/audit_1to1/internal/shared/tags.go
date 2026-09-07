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

// FieldNameTag is the name encoding/json gives an exported field that carries
// no tag: its own, which this converts to the snake_case the MCP side writes.
//
// It is acronym-aware where [camelToSnake] is not, because it converts a Go
// identifier rather than a tag somebody already wrote in one style: AvatarURL
// is avatar_url and not avatar_u_r_l, NamespaceID is namespace_id.
//
// The plural of an acronym is the case the usual rule gets wrong. Breaking
// before the last capital of a run that is followed by a lowercase letter is
// right for CRMContact, and wrong for AssigneeIDs, which it renders
// assignee_i_ds. A run whose whole lowercase remainder is a final "s" is
// therefore kept together, which is the only shape Go spells that way.
func FieldNameTag(name string) string {
	runes := []rune(name)
	upper := func(i int) bool { return i >= 0 && i < len(runes) && runes[i] >= 'A' && runes[i] <= 'Z' }
	lower := func(i int) bool {
		return i >= 0 && i < len(runes) && ((runes[i] >= 'a' && runes[i] <= 'z') || (runes[i] >= '0' && runes[i] <= '9'))
	}

	var b strings.Builder
	for i, r := range runes {
		if upper(i) && i > 0 {
			switch {
			case lower(i - 1):
				// A capital after a lowercase letter always starts a word.
				b.WriteByte('_')
			case upper(i-1) && lower(i+1) && !pluralTail(runes, i+1):
				// The last capital of an acronym run starts the next word,
				// unless what follows is the acronym's own plural s.
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		} else if upper(i) {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// pluralTail reports whether the rest of the identifier from i is exactly "s",
// which is how Go writes the plural of an acronym: IDs, URLs, IIDs.
func pluralTail(runes []rune, i int) bool {
	return i == len(runes)-1 && runes[i] == 's'
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
