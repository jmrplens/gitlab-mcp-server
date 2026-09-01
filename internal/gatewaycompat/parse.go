// Package gatewaycompat rewrites the human-readable text this server lists —
// tool, prompt, resource and resource-template descriptions and titles, and
// the description and title annotations embedded in tool schemas — according
// to operator-defined substitutions.
//
// It exists because MCP gateways validate a server's catalog before admitting
// it, and their rules are the gateway operator's to choose: one production
// gateway (IBM mcp-context-forge before 0.7.0) refused any tool whose
// description contained a semicolon. This server keeps its own text clean of
// the characters known to be rejected (cmd/audit_gateway_chars gates that),
// but the next gateway rule is not this project's to predict. The
// substitution knob lets the operator comply with a rule the day they meet
// it, without waiting for a release.
//
// The knob rewrites catalog metadata and nothing else: names, URIs, schema
// constraints (pattern, const, enum values, defaults) and tool-call payloads
// are never touched, because those are contract, not prose.
package gatewaycompat

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// EnvVar configures the substitutions in both stdio and HTTP modes. The value
// is a comma-separated list of old=new pairs applied in order to every listed
// description and title. A backslash escapes a literal comma, equals sign or
// backslash inside either half, so a semicolon can become a comma:
//
//	GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS=';=\,'
//
// Whitespace is significant: "; = " substitutes "; " (semicolon, space) with
// " " (a single space). An empty or unset value disables the middleware.
const EnvVar = "GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS"

// Substitution is one ordered old→new text replacement.
type Substitution struct {
	// Old is the literal text to replace. Never empty.
	Old string
	// New is the literal replacement. May be empty, which deletes Old.
	New string
}

// FromEnv parses EnvVar. An unset or empty variable is not an error: it
// returns nil, nil, and the caller installs nothing.
func FromEnv() ([]Substitution, error) {
	subs, err := ParseSubstitutions(os.Getenv(EnvVar))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", EnvVar, err)
	}
	return subs, nil
}

// ParseSubstitutions parses a comma-separated list of old=new pairs.
//
// Within a pair, the first unescaped equals sign separates old from new;
// later unescaped equals signs in new are literal. Backslash escapes a comma,
// an equals sign or a backslash; any other escape is an error, because a
// silently absorbed typo here would ship a substitution the operator did not
// write. An empty old is an error for the same reason: ReplaceAll with an
// empty pattern inserts new between every pair of characters.
func ParseSubstitutions(value string) ([]Substitution, error) {
	if value == "" {
		return nil, nil
	}

	var subs []Substitution
	for _, pair := range splitUnescaped(value, ',') {
		halves := splitPairUnescaped(pair, '=')
		if len(halves) != 2 {
			return nil, fmt.Errorf("substitution %q has no unescaped %q separator", pair, "=")
		}
		oldText, err := unescape(halves[0])
		if err != nil {
			return nil, fmt.Errorf("substitution %q: %w", pair, err)
		}
		newText, err := unescape(halves[1])
		if err != nil {
			return nil, fmt.Errorf("substitution %q: %w", pair, err)
		}
		if oldText == "" {
			return nil, fmt.Errorf("substitution %q replaces the empty string", pair)
		}
		subs = append(subs, Substitution{Old: oldText, New: newText})
	}
	return subs, nil
}

// Apply runs every substitution over text, in declared order.
func Apply(subs []Substitution, text string) string {
	for _, sub := range subs {
		text = strings.ReplaceAll(text, sub.Old, sub.New)
	}
	return text
}

// splitUnescaped splits on every separator not preceded by a backslash,
// keeping the escapes in place for unescape to resolve.
func splitUnescaped(value string, sep byte) []string {
	var parts []string
	start := 0
	escaped := false
	for i := range len(value) {
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == sep:
			parts = append(parts, value[start:i])
			start = i + 1
		}
	}
	return append(parts, value[start:])
}

// splitPairUnescaped splits on the first unescaped separator only, so an
// unescaped equals sign inside the replacement half stays literal.
func splitPairUnescaped(value string, sep byte) []string {
	escaped := false
	for i := range len(value) {
		switch {
		case escaped:
			escaped = false
		case value[i] == '\\':
			escaped = true
		case value[i] == sep:
			return []string{value[:i], value[i+1:]}
		}
	}
	return []string{value}
}

// unescape resolves the three escapes the format defines and rejects the
// rest.
func unescape(value string) (string, error) {
	var out strings.Builder
	escaped := false
	for i := range len(value) {
		ch := value[i]
		if escaped {
			switch ch {
			case ',', '=', '\\':
				out.WriteByte(ch)
			default:
				return "", fmt.Errorf("unknown escape %q", "\\"+string(ch))
			}
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		out.WriteByte(ch)
	}
	if escaped {
		return "", errors.New("dangling backslash")
	}
	return out.String(), nil
}
