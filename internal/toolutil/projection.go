package toolutil

import (
	"bytes"
	"encoding/json"
	"os"
)

// IsLeanOutputEnabled returns true if output projection is enabled via
// GITLAB_MCP_LEAN_OUTPUT (or alias GITLAB_LEAN_OUTPUT).
func IsLeanOutputEnabled() bool {
	if val := os.Getenv("GITLAB_MCP_LEAN_OUTPUT"); val != "" {
		return isTruthy(val)
	}
	return isTruthy(os.Getenv("GITLAB_LEAN_OUTPUT"))
}

// noiseKeySet contains metadata property keys commonly returned by GitLab APIs
// that consume large token counts in LLM context windows without providing functional value.
var noiseKeySet = map[string]struct{}{
	"_links":              {},
	"avatar_url":          {},
	"gravatar_id":         {},
	"node_id":             {},
	"site_admin":          {},
	"starred_url":         {},
	"subscriptions_url":   {},
	"organizations_url":   {},
	"repos_url":           {},
	"events_url":          {},
	"received_events_url": {},
	"gists_url":           {},
	"following_url":       {},
	"followers_url":       {},
}

// preserveEmptyStringKeys contains property keys whose semantic meaning relies on
// distinguishing an intentional empty string (e.g. empty file content or empty variable value)
// from an absent or nil field.
var preserveEmptyStringKeys = map[string]struct{}{
	"value":   {},
	"content": {},
	"diff":    {},
	"raw":     {},
}

// ProjectOutput recursively prunes metadata noise, unpopulated strings, and empty
// nested collections from GitLab API responses to optimize LLM context window token efficiency.
//
// Design Principle: "Lean means lean, not complete."
// The primary goal is minimizing token churn across agent conversations (often 500-3,000
// tokens per list query) rather than providing a 1:1 bitwise reflection of the GitLab REST API.
// Empty nested collections ([] and {}) and decorative empty strings are pruned because LLMs
// naturally assume absent fields denote empty/none, preventing token waste across multi-turn
// context histories. Semantic content keys (such as 'value', 'content', 'diff', 'raw') are
// preserved even when empty. For full, unadulterated API payloads, GITLAB_MCP_LEAN_OUTPUT can
// be left disabled.
func ProjectOutput(input any) any {
	if input == nil {
		return nil
	}

	var generic any
	switch v := input.(type) {
	case map[string]any, []any:
		generic = v
	default:
		b, err := json.Marshal(input)
		if err != nil {
			return input
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if err := dec.Decode(&generic); err != nil {
			return input
		}
	}

	projected := projectGenericValue(generic)
	if projected == nil {
		switch generic.(type) {
		case []any:
			return []any{}
		case map[string]any:
			return map[string]any{}
		}
	}
	return projected
}

func projectGenericValue(val any) any {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, item := range v {
			if _, isNoise := noiseKeySet[k]; isNoise {
				continue
			}
			cleaned := projectGenericValue(item)
			if cleaned == nil {
				continue
			}
			if str, isStr := cleaned.(string); isStr && str == "" {
				if _, keep := preserveEmptyStringKeys[k]; !keep {
					continue
				}
			}
			out[k] = cleaned
		}
		if len(out) == 0 {
			return nil
		}
		return out

	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			cleaned := projectGenericValue(item)
			if cleaned != nil && cleaned != "" {
				out = append(out, cleaned)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out

	default:
		return val
	}
}
