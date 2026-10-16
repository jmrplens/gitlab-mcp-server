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

// ProjectOutput recursively prunes metadata noise, empty strings, and nil fields
// from GitLab API responses to optimize LLM context window token efficiency.
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
			if cleaned != nil && cleaned != "" {
				out[k] = cleaned
			}
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
