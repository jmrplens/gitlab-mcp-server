// parse_test.go verifies the GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS format:
// comma-separated old=new pairs with backslash escapes, strict about
// everything it does not define, because a silently absorbed typo would ship
// a substitution the operator did not write.
package gatewaycompat_test

import (
	"bytes"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
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

// TestParseSubstitutions_ExceedsBounds_ReturnsError verifies that the knob is
// bounded rather than unlimited: a rule list long enough to be a program, or a
// half long enough to be a paragraph, is refused at startup with an error
// naming the limit.
//
// The bound matters because this is the one setting that writes chosen prose
// into the channel the model reads as instructions, and the value is an
// ordinary environment variable. The rows at the limit are the ones that keep
// the feature usable: the limit is a ceiling on abuse, not on the substitution
// an operator actually needs.
func TestParseSubstitutions_ExceedsBounds_ReturnsError(t *testing.T) {
	pairs := func(n int) string {
		parts := make([]string, n)
		for i := range parts {
			parts[i] = "o" + strconv.Itoa(i) + "=n"
		}
		return strings.Join(parts, ",")
	}

	tests := []struct {
		name     string
		value    string
		wantErr  bool
		wantSub  string
		wantSubs int
	}{
		{
			name:     "rule count at the limit is accepted",
			value:    pairs(gatewaycompat.MaxSubstitutions),
			wantSubs: gatewaycompat.MaxSubstitutions,
		},
		{
			name:    "rule count over the limit is refused",
			value:   pairs(gatewaycompat.MaxSubstitutions + 1),
			wantErr: true,
			wantSub: strconv.Itoa(gatewaycompat.MaxSubstitutions),
		},
		{
			name:     "replacement at the limit is accepted",
			value:    ";=" + strings.Repeat("x", gatewaycompat.MaxSubstitutionBytes),
			wantSubs: 1,
		},
		{
			name:    "replacement over the limit is refused",
			value:   ";=" + strings.Repeat("x", gatewaycompat.MaxSubstitutionBytes+1),
			wantErr: true,
			wantSub: strconv.Itoa(gatewaycompat.MaxSubstitutionBytes),
		},
		{
			name:    "pattern over the limit is refused",
			value:   strings.Repeat("y", gatewaycompat.MaxSubstitutionBytes+1) + "=z",
			wantErr: true,
			wantSub: strconv.Itoa(gatewaycompat.MaxSubstitutionBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subs, err := gatewaycompat.ParseSubstitutions(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSubstitutions accepted %d bytes of value, want an error", len(tt.value))
				}
				if !strings.Contains(err.Error(), tt.wantSub) {
					t.Errorf("error %q does not name the limit %q", err, tt.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSubstitutions returned error: %v", err)
			}
			if len(subs) != tt.wantSubs {
				t.Errorf("ParseSubstitutions returned %d substitutions, want %d", len(subs), tt.wantSubs)
			}
		})
	}
}

// TestApply_ExplosiveGrowth_KeepsOriginalText verifies the bound that a
// per-rule length limit cannot provide: an attacker who cannot write a long
// replacement writes a short one for a frequent word instead, and multiplies
// the served text anyway. Apply refuses to grow a string beyond a ceiling
// relative to its own length, and serves it as written when it would.
//
// The refusal is all-or-nothing on purpose. Serving half a substitution set
// would be a third behavior nobody configured, and the original text is the
// one this project's own gateway-character audit vouches for.
func TestApply_ExplosiveGrowth_KeepsOriginalText(t *testing.T) {
	const text = "List the merge requests in a project."

	// A description long enough that the proportional half of the ceiling is
	// the binding one, rather than the flat allowance that protects titles.
	longText := strings.Repeat("describe a merge request. ", 24)

	tests := []struct {
		name string
		text string
		subs []gatewaycompat.Substitution
		want string
	}{
		{
			name: "ordinary substitution applies",
			subs: []gatewaycompat.Substitution{{Old: ".", New: " and nothing else."}},
			want: "List the merge requests in a project and nothing else.",
		},
		{
			name: "a long description may still grow within the factor",
			text: longText,
			subs: []gatewaycompat.Substitution{{Old: "merge request", New: "merge request (MR)"}},
			want: strings.ReplaceAll(longText, "merge request", "merge request (MR)"),
		},
		{
			name: "a long description is refused past the factor",
			text: longText,
			subs: []gatewaycompat.Substitution{{Old: "a", New: strings.Repeat("a", 64)}},
			want: longText,
		},
		{
			name: "deletion applies",
			subs: []gatewaycompat.Substitution{{Old: " in a project", New: ""}},
			want: "List the merge requests.",
		},
		{
			name: "one long replacement of a frequent letter is refused",
			subs: []gatewaycompat.Substitution{{Old: "e", New: strings.Repeat("SYSTEM NOTE ", 20)}},
			want: text,
		},
		{
			name: "growth accumulated across rules is refused",
			subs: []gatewaycompat.Substitution{
				{Old: "e", New: strings.Repeat("ee", 4)},
				{Old: "e", New: strings.Repeat("ee", 4)},
				{Old: "e", New: strings.Repeat("ee", 4)},
			},
			want: text,
		},
		{
			name: "a rule that only shrinks is never refused",
			subs: []gatewaycompat.Substitution{{Old: "merge requests", New: "MRs"}},
			want: "List the MRs in a project.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.text
			if input == "" {
				input = text
			}
			if got := gatewaycompat.Apply(tt.subs, input); got != tt.want {
				t.Errorf("Apply() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFromEnv_ActiveSubstitutions_AnnounceOnce verifies that a rewritten
// model-facing catalog is never a silent one: reading a non-empty value logs a
// WARN naming the variable and how many rules are active, once per process
// rather than once per pooled server entry, and an unset variable logs nothing
// at all.
func TestFromEnv_ActiveSubstitutions_AnnounceOnce(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		calls       int
		wantRecords int
		wantPhrases []string
	}{
		{
			name:        "active substitutions announce once",
			value:       ";=.,GitLab=GitLab instance",
			calls:       3,
			wantRecords: 1,
			wantPhrases: []string{`"level":"WARN"`, gatewaycompat.EnvVar, `"substitutions":2`},
		},
		{
			name:  "an unset variable says nothing",
			value: "",
			calls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatewaycompat.ResetAnnouncementsForTest()
			t.Cleanup(gatewaycompat.ResetAnnouncementsForTest)
			logged := captureGatewayLogs(t)
			t.Setenv(gatewaycompat.EnvVar, tt.value)

			for range tt.calls {
				if _, err := gatewaycompat.FromEnv(); err != nil {
					t.Fatalf("FromEnv() returned error: %v", err)
				}
			}

			output := logged()
			if got := strings.Count(output, `"msg":`); got != tt.wantRecords {
				t.Errorf("log records = %d, want %d (log: %q)", got, tt.wantRecords, output)
			}
			for _, phrase := range tt.wantPhrases {
				if !strings.Contains(output, phrase) {
					t.Errorf("log = %q, want it to mention %q", output, phrase)
				}
			}
		})
	}
}

// TestApply_ClampedGrowth_AnnouncesOnce verifies that the clamp itself is
// visible: an operator whose substitution set is being refused sees one WARN
// saying so rather than silently unrewritten text.
func TestApply_ClampedGrowth_AnnouncesOnce(t *testing.T) {
	gatewaycompat.ResetAnnouncementsForTest()
	t.Cleanup(gatewaycompat.ResetAnnouncementsForTest)
	logged := captureGatewayLogs(t)

	subs := []gatewaycompat.Substitution{{Old: "e", New: strings.Repeat("SYSTEM NOTE ", 20)}}
	for range 3 {
		gatewaycompat.Apply(subs, "the merge request")
	}

	output := logged()
	if got := strings.Count(output, `"msg":`); got != 1 {
		t.Errorf("log records = %d, want 1 (log: %q)", got, output)
	}
	if !strings.Contains(output, gatewaycompat.EnvVar) {
		t.Errorf("log = %q, want it to name %s", output, gatewaycompat.EnvVar)
	}
}

// captureGatewayLogs installs a capturing default logger and returns an
// accessor for everything written to it.
func captureGatewayLogs(t *testing.T) func() string {
	t.Helper()
	writer := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return writer.String
}

// syncBuffer is a bytes.Buffer safe to read while a logger writes to it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the guarded buffer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns everything written so far.
func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
