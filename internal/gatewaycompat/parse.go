package gatewaycompat

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
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

// MaxSubstitutions and MaxSubstitutionBytes bound the configuration, and
// growthFactor and growthAllowance bound its effect.
//
// The knob is the only setting that writes operator-chosen prose into the
// channel a model reads as instructions, and it is an ordinary environment
// variable rather than a flag. Bounding it keeps a compliance tool a
// compliance tool: a gateway rule is satisfied by a handful of short
// rewrites, so a configuration that needs hundreds of rules, or a paragraph
// per rule, is not the use this exists for.
//
// The two effect bounds exist because a length limit per rule does not bound
// the result. A rule replacing a single frequent letter with 200 bytes stays
// well inside every configuration limit and still multiplies the served
// catalog. So the ceiling is relative to the text being rewritten: output may
// not exceed growthFactor times the input, nor the input plus
// growthAllowance bytes, whichever is larger. The allowance is what keeps
// short titles rewritable.
const (
	MaxSubstitutions     = 32
	MaxSubstitutionBytes = 256

	growthFactor    = 2
	growthAllowance = 512
)

// announceOnce and clampOnce keep each announcement to one occurrence per
// process. FromEnv runs once per pooled server entry in HTTP mode, and Apply
// runs once per string per listing, but what they report is a property of the
// configuration, not of the call.
var (
	announceOnce sync.Once
	clampOnce    sync.Once
)

// Substitution is one ordered old→new text replacement.
type Substitution struct {
	// Old is the literal text to replace. Never empty.
	Old string
	// New is the literal replacement. May be empty, which deletes Old.
	New string
}

// FromEnv parses EnvVar. An unset or empty variable is not an error: it
// returns nil, nil, and the caller installs nothing.
//
// An active configuration is announced at WARN, once per process. A rewritten
// catalog is otherwise indistinguishable at runtime from an unrewritten one,
// which makes "the descriptions the model reads are not the ones this build
// ships" a fact with no local evidence.
func FromEnv() ([]Substitution, error) {
	subs, err := ParseSubstitutions(os.Getenv(EnvVar))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", EnvVar, err)
	}
	if len(subs) > 0 {
		announceOnce.Do(func() {
			slog.Warn("rewriting served catalog text",
				"env", EnvVar,
				"substitutions", len(subs),
				"scope", "tool, prompt, resource and resource-template descriptions and titles")
		})
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
		if len(oldText) > MaxSubstitutionBytes || len(newText) > MaxSubstitutionBytes {
			return nil, fmt.Errorf("substitution %.40q exceeds %d bytes per half", pair, MaxSubstitutionBytes)
		}
		subs = append(subs, Substitution{Old: oldText, New: newText})
		if len(subs) > MaxSubstitutions {
			return nil, fmt.Errorf("more than %d substitutions", MaxSubstitutions)
		}
	}
	return subs, nil
}

// Apply runs every substitution over text, in declared order, unless doing so
// would grow the text past the ceiling growthLimit sets — in which case text
// is returned as written.
//
// The refusal is all-or-nothing. Applying the rules that fit would serve a
// third text nobody configured, and the unrewritten one is the text this
// project's own gateway-character audit vouches for.
//
// Each rule's result length is computed before the rule runs, so an
// oversized string is never built.
func Apply(subs []Substitution, text string) string {
	limit := growthLimit(len(text))
	out := text
	for _, sub := range subs {
		if grown := len(out) + strings.Count(out, sub.Old)*(len(sub.New)-len(sub.Old)); grown > limit {
			announceClamp(len(subs), len(text), grown, limit)
			return text
		}
		out = strings.ReplaceAll(out, sub.Old, sub.New)
	}
	return out
}

// growthLimit is the longest rewritten form of an n-byte string this package
// will serve.
func growthLimit(n int) int {
	if scaled := n * growthFactor; scaled > n+growthAllowance {
		return scaled
	}
	return n + growthAllowance
}

// announceClamp reports the first refusal, so an operator whose substitutions
// stopped taking effect is told rather than left comparing catalogs.
func announceClamp(rules, original, grown, limit int) {
	clampOnce.Do(func() {
		slog.Warn("serving catalog text unrewritten: the substitutions would multiply it",
			"env", EnvVar,
			"substitutions", rules,
			"original_bytes", original,
			"rewritten_bytes", grown,
			"limit_bytes", limit)
	})
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
