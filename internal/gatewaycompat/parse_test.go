// parse_test.go verifies the GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS format:
// comma-separated old=new pairs with backslash escapes, strict about
// everything it does not define, because a silently absorbed typo would ship
// a substitution the operator did not write.
package gatewaycompat_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/gatewaycompat"
)

// TestParseSubstitutions_ValidInputs_ReturnsPairs verifies the accepted
// grammar: single and multiple pairs, the three escapes, a literal equals
// sign in the replacement half, significant whitespace, and an empty
// replacement (deletion).
func TestParseSubstitutions_ValidInputs_ReturnsPairs(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []gatewaycompat.Substitution
	}{
		{
			name:  "empty value disables",
			value: "",
			want:  nil,
		},
		{
			name:  "single pair",
			value: ";=.",
			want:  []gatewaycompat.Substitution{{Old: ";", New: "."}},
		},
		{
			name:  "escaped comma as replacement",
			value: `;=\,`,
			want:  []gatewaycompat.Substitution{{Old: ";", New: ","}},
		},
		{
			name:  "escaped equals in old half",
			value: `\==-`,
			want:  []gatewaycompat.Substitution{{Old: "=", New: "-"}},
		},
		{
			name:  "escaped backslash",
			value: `\\=/`,
			want:  []gatewaycompat.Substitution{{Old: `\`, New: "/"}},
		},
		{
			name:  "unescaped equals in replacement is literal",
			value: "a=b=c",
			want:  []gatewaycompat.Substitution{{Old: "a", New: "b=c"}},
		},
		{
			name:  "multiple ordered pairs",
			value: "; =. ,:=-",
			want: []gatewaycompat.Substitution{
				{Old: "; ", New: ". "},
				{Old: ":", New: "-"},
			},
		},
		{
			name:  "empty replacement deletes",
			value: ";=",
			want:  []gatewaycompat.Substitution{{Old: ";", New: ""}},
		},
		{
			name:  "whitespace is significant",
			value: " ; = . ",
			want:  []gatewaycompat.Substitution{{Old: " ; ", New: " . "}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gatewaycompat.ParseSubstitutions(tt.value)
			if err != nil {
				t.Fatalf("ParseSubstitutions(%q) returned error: %v", tt.value, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseSubstitutions(%q) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}

// TestParseSubstitutions_InvalidInputs_ReturnsError verifies each rejection:
// a pair without a separator, an empty old half (ReplaceAll with an empty
// pattern would insert the replacement between every pair of characters), an
// escape the format does not define, and a dangling backslash.
func TestParseSubstitutions_InvalidInputs_ReturnsError(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantSub string
	}{
		{name: "pair without separator", value: "abc", wantSub: "separator"},
		{name: "second pair without separator", value: ";=.,abc", wantSub: "separator"},
		{name: "empty old half", value: "=x", wantSub: "empty string"},
		{name: "unknown escape", value: `\n=x`, wantSub: "unknown escape"},
		{name: "dangling backslash in new half", value: `;=\`, wantSub: "dangling backslash"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gatewaycompat.ParseSubstitutions(tt.value)
			if err == nil {
				t.Fatalf("ParseSubstitutions(%q) accepted an invalid value", tt.value)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("ParseSubstitutions(%q) error %q does not mention %q", tt.value, err, tt.wantSub)
			}
		})
	}
}

// TestApply_OrderedSubstitutions_ChainInDeclaredOrder verifies that
// substitutions run in declared order over the same text, so a later pair
// sees the earlier pair's output — the property an operator relies on when
// one rule's replacement would trip another gateway rule.
func TestApply_OrderedSubstitutions_ChainInDeclaredOrder(t *testing.T) {
	subs := []gatewaycompat.Substitution{
		{Old: ";", New: ":"},
		{Old: ":", New: "-"},
	}
	if got := gatewaycompat.Apply(subs, "a;b:c"); got != "a-b-c" {
		t.Errorf("Apply chained = %q, want %q", got, "a-b-c")
	}
}

// TestFromEnv_EnvironmentValues_ParsesOrFails verifies the one reader of the
// environment variable: unset means disabled, a valid value parses, and an
// invalid value returns an error naming the variable so the startup failure
// says what to fix.
func TestFromEnv_EnvironmentValues_ParsesOrFails(t *testing.T) {
	t.Run("unset disables", func(t *testing.T) {
		t.Setenv(gatewaycompat.EnvVar, "")
		subs, err := gatewaycompat.FromEnv()
		if err != nil || subs != nil {
			t.Errorf("FromEnv() with unset variable = %#v, %v; want nil, nil", subs, err)
		}
	})
	t.Run("valid value parses", func(t *testing.T) {
		t.Setenv(gatewaycompat.EnvVar, ";=.")
		subs, err := gatewaycompat.FromEnv()
		if err != nil {
			t.Fatalf("FromEnv() returned error: %v", err)
		}
		want := []gatewaycompat.Substitution{{Old: ";", New: "."}}
		if !reflect.DeepEqual(subs, want) {
			t.Errorf("FromEnv() = %#v, want %#v", subs, want)
		}
	})
	t.Run("invalid value names the variable", func(t *testing.T) {
		t.Setenv(gatewaycompat.EnvVar, "=x")
		_, err := gatewaycompat.FromEnv()
		if err == nil {
			t.Fatal("FromEnv() accepted an invalid value")
		}
		if !strings.Contains(err.Error(), gatewaycompat.EnvVar) {
			t.Errorf("FromEnv() error %q does not name %s", err, gatewaycompat.EnvVar)
		}
	})
}
