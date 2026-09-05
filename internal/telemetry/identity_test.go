package telemetry

import (
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
)

// attrMap flattens a redactor's output for assertion.
func attrMap(t *testing.T, attrs []attribute.KeyValue) map[string]string {
	t.Helper()
	out := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}

// newRedactor builds one over the test keyring.
func newRedactor(t *testing.T, policy IdentityPolicy) *Redactor {
	t.Helper()
	return NewRedactor(policy, testKeyring(t))
}

// TestRedactor_DefaultExportsNothing pins the default, which is the only value
// that is safe for an operator who has not thought about the question.
//
// The specification's obligation here is a SHOULD, not a prohibition: the
// MUST NOT on Opt-In attributes is scoped to instrumentation that does not
// support configuration, and this one does. It is honored anyway, because the
// choice of whether to record who made a call belongs to the operator and a
// default that decides for them is the wrong default whichever way it points.
func TestRedactor_DefaultExportsNothing(t *testing.T) {
	redactor := newRedactor(t, "")

	if got := redactor.Policy(); got != IdentityNone {
		t.Errorf("default policy = %q, want %q", got, IdentityNone)
	}
	if attrs := redactor.Attributes("42", "jane"); len(attrs) != 0 {
		t.Errorf("the default policy exported %v", attrs)
	}
}

// TestRedactor_NilIsUsable covers the zero value. Identity is consulted per
// request, so a redactor that had to be non-nil would put a check on the hot
// path and a crash behind forgetting it.
func TestRedactor_NilIsUsable(t *testing.T) {
	var redactor *Redactor

	if got := redactor.Policy(); got != IdentityNone {
		t.Errorf("nil policy = %q, want %q", got, IdentityNone)
	}
}

// TestRedactor_FullUsesTheRegistryNames asserts the keys, not just that
// something was emitted.
//
// The names matter more than they look. An earlier version of this file wrote
// enduser.name, which does not exist in any registry: inventing a key inside a
// namespace OpenTelemetry owns means a future release can give that key a
// different meaning, and nothing would warn. user.id and user.name are the
// defined pair, and using them here makes the span agree with the log record
// and with anything else that records identity.
func TestRedactor_FullUsesTheRegistryNames(t *testing.T) {
	redactor := newRedactor(t, IdentityFull)

	attrs := attrMap(t, redactor.Attributes("42", "jane"))
	if attrs[AttrUserID] != "42" {
		t.Errorf("%s = %q, want the user id", AttrUserID, attrs[AttrUserID])
	}
	if attrs[AttrUserName] != "jane" {
		t.Errorf("%s = %q, want the username", AttrUserName, attrs[AttrUserName])
	}
	for _, forbidden := range []string{"enduser.id", "enduser.name", "enduser.pseudo_id"} {
		t.Run(forbidden, func(t *testing.T) {
			if _, present := attrs[forbidden]; present {
				t.Errorf("%s is emitted; it is either misspelled or invented inside a namespace we do not own", forbidden)
			}
		})
	}
}

// TestRedactor_FullOmitsAnAbsentUsername pins that an empty value is left out
// rather than written.
//
// Emptiness is meaningful on the wire: "AnyValues expressing an empty value...
// are considered meaningful and MUST be stored and passed on to processors /
// exporters", so an always-set attribute with a zero default ships an empty
// string a backend will index and bill for.
func TestRedactor_FullOmitsAnAbsentUsername(t *testing.T) {
	redactor := newRedactor(t, IdentityFull)

	attrs := attrMap(t, redactor.Attributes("42", ""))
	if _, present := attrs[AttrUserName]; present {
		t.Errorf("%s was emitted empty rather than omitted", AttrUserName)
	}
	if attrs[AttrUserID] != "42" {
		t.Error("the user id was dropped along with the absent username")
	}
}

// TestRedactor_PseudonymousNamesNobody is the mode that exists for a shared
// endpoint: it keeps the one thing such a deployment genuinely needs, telling
// one caller's traffic from another's, without naming anyone.
func TestRedactor_PseudonymousNamesNobody(t *testing.T) {
	redactor := newRedactor(t, IdentityPseudonymous)

	attrs := attrMap(t, redactor.Attributes("42", "jane"))
	digest, present := attrs[AttrUserHash]
	if !present {
		t.Fatalf("no %s was emitted: %v", AttrUserHash, attrs)
	}
	for key, value := range attrs {
		if strings.Contains(value, "jane") || value == "42" {
			t.Errorf("%s carries the identity verbatim: %q", key, value)
		}
	}
	if _, leaked := attrs[AttrUserID]; leaked {
		t.Errorf("%s is emitted under the pseudonymous policy", AttrUserID)
	}
	if len(digest) != 16 {
		t.Errorf("digest %q is %d characters, want 16", digest, len(digest))
	}
}

// TestRedactor_PseudonymIsStableWithinAProcess covers what makes the mode
// useful: two calls by one user must correlate, or the attribute buys nothing
// over emitting no identity at all.
func TestRedactor_PseudonymIsStableWithinAProcess(t *testing.T) {
	redactor := newRedactor(t, IdentityPseudonymous)

	first := attrMap(t, redactor.Attributes("42", "jane"))[AttrUserHash]
	second := attrMap(t, redactor.Attributes("42", "jane"))[AttrUserHash]
	other := attrMap(t, redactor.Attributes("43", "john"))[AttrUserHash]

	if first != second {
		t.Errorf("one user produced two digests (%q, %q); their calls cannot be correlated", first, second)
	}
	if first == other {
		t.Errorf("two users share digest %q; the attribute distinguishes nobody", first)
	}
}

// TestRedactor_PseudonymDoesNotSurviveARestart is the property the salt exists
// for, and it is a deliberate cost rather than an oversight.
//
// A digest stable across processes would let anyone holding two deployments'
// exports link the same person across both, and would make the mapping
// recoverable by anyone who ever learns the salt. Per-process and never
// persisted means the link lives exactly as long as the process that made it,
// and the price is that a restart renumbers everyone.
func TestRedactor_PseudonymDoesNotSurviveARestart(t *testing.T) {
	first := newRedactor(t, IdentityPseudonymous)
	second := newRedactor(t, IdentityPseudonymous)

	a := attrMap(t, first.Attributes("42", "jane"))[AttrUserHash]
	b := attrMap(t, second.Attributes("42", "jane"))[AttrUserHash]

	if a == b {
		t.Errorf("the digest %q survives a new redactor; the salt is not per-process", a)
	}
}

// TestRedactor_UnauthenticatedCallsCarryNothing covers the case every policy
// answers the same way. There is no identity to redact or to publish, and
// emitting a placeholder would create a bucket that means "several different
// people" while looking like one.
func TestRedactor_UnauthenticatedCallsCarryNothing(t *testing.T) {
	for _, policy := range []IdentityPolicy{IdentityNone, IdentityPseudonymous, IdentityFull} {
		t.Run(string(policy), func(t *testing.T) {
			redactor := newRedactor(t, policy)
			if attrs := redactor.Attributes("", "jane"); len(attrs) != 0 {
				t.Errorf("an unauthenticated call carried %v", attrs)
			}
		})
	}
}

// TestParseIdentityPolicy_RefusesAnUnknownValueByName asserts that a typo fails
// at startup with the alternatives listed, rather than silently selecting the
// safe default.
//
// Silently defaulting would be defensible for a value that only widens
// exposure, and indefensible here: an operator who typed "pseudonimous" and got
// "none" would believe they had per-user correlation and find an empty
// dashboard, with nothing anywhere saying why.
func TestParseIdentityPolicy_RefusesAnUnknownValueByName(t *testing.T) {
	_, err := ParseIdentityPolicy("pseudonimous")
	if err == nil {
		t.Fatal("a misspelled policy was accepted")
	}
	for _, want := range []string{"pseudonimous", string(IdentityNone), string(IdentityPseudonymous), string(IdentityFull)} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the error does not mention %q: %v", want, err)
			}
		})
	}
}

// TestParseIdentityPolicy_AcceptsTheSpellingsOperatorsWrite covers the surface a
// person types by hand: case and surrounding whitespace, and an empty value
// meaning "unset" rather than "invalid".
func TestParseIdentityPolicy_AcceptsTheSpellingsOperatorsWrite(t *testing.T) {
	for input, want := range map[string]IdentityPolicy{
		"":               IdentityNone,
		"none":           IdentityNone,
		"NONE":           IdentityNone,
		"  full  ":       IdentityFull,
		"Pseudonymous":   IdentityPseudonymous,
		"PSEUDONYMOUS  ": IdentityPseudonymous,
	} {
		t.Run(input, func(t *testing.T) {
			got, err := ParseIdentityPolicy(input)
			if err != nil {
				t.Errorf("ParseIdentityPolicy(%q): %v", input, err)
				return
			}
			if got != want {
				t.Errorf("ParseIdentityPolicy(%q) = %q, want %q", input, got, want)
			}
		})
	}
}

// TestPolicyDescription_SaysWhatEachPolicyExports covers the sentence an
// operator reads at startup, which is the only place the policy explains itself
// outside the documentation.
//
// The assertions are about meaning rather than wording, because the wording is
// prose and pinning it would make this a change detector. What must hold is that
// the three are distinguishable and that none of them describes a different
// policy's behavior: a line telling somebody their deployment records nobody
// while it records names is worse than no line at all.
func TestPolicyDescription_SaysWhatEachPolicyExports(t *testing.T) {
	// The subtests below read this map, and subtests run in order, so every
	// policy that has a description is in it before they start.
	descriptions := map[IdentityPolicy]string{}
	for _, policy := range []IdentityPolicy{IdentityNone, IdentityPseudonymous, IdentityFull} {
		t.Run(string(policy)+" has a description", func(t *testing.T) {
			description := PolicyDescription(policy)
			if description == "" {
				t.Fatalf("policy %q has no description, so the startup line says nothing", policy)
			}
			descriptions[policy] = description
		})
	}

	t.Run("the three are distinguishable", func(t *testing.T) {
		seen := map[string]IdentityPolicy{}
		for policy, description := range descriptions {
			if other, duplicate := seen[description]; duplicate {
				t.Errorf("%q and %q share a description, so the startup line cannot tell them apart", policy, other)
			}
			seen[description] = policy
		}
	})

	t.Run("the default promises nothing about a caller", func(t *testing.T) {
		// The words that would make it wrong: a default whose description
		// mentioned a digest would be describing pseudonymous.
		for _, wrong := range []string{"digest", "username", "user id"} {
			t.Run(wrong, func(t *testing.T) {
				if strings.Contains(descriptions[IdentityNone], wrong) {
					t.Errorf("the default's description mentions %q: %q", wrong, descriptions[IdentityNone])
				}
			})
		}
	})

	t.Run("pseudonymous says it is not readable", func(t *testing.T) {
		if !strings.Contains(descriptions[IdentityPseudonymous], "digest") {
			t.Errorf("the pseudonymous description does not say what it records: %q",
				descriptions[IdentityPseudonymous])
		}
		if strings.Contains(descriptions[IdentityPseudonymous], "username") {
			t.Errorf("the pseudonymous description claims a username it does not export: %q",
				descriptions[IdentityPseudonymous])
		}
	})

	t.Run("full says the identity is readable", func(t *testing.T) {
		if !strings.Contains(descriptions[IdentityFull], "username") {
			t.Errorf("the full description does not name what it exports: %q", descriptions[IdentityFull])
		}
	})
}

// TestRedactor_ABrokenKeyringYieldsNoIdentityAtAll covers the failure shape a
// review named: pseudonymous with no usable keyring produced user.hash with an
// empty value, a bucket that reads as one anonymous person while meaning "the
// keyring is broken". Absence is the honest value.
func TestRedactor_ABrokenKeyringYieldsNoIdentityAtAll(t *testing.T) {
	t.Parallel()

	redactor := NewRedactor(IdentityPseudonymous, nil)

	if attrs := redactor.Attributes("42", "jane"); len(attrs) != 0 {
		t.Errorf("a redactor with no keyring produced %v; an empty digest is not a pseudonym", attrs)
	}
}

// TestPolicyDescription_AnUnknownPolicy_PromisesNothing covers the fallback of
// the sentence an operator reads at startup.
//
// The description exists so that one line says what will actually leave the
// process rather than a mode name somebody would have to look up. A value this
// build does not know must therefore describe the safe outcome — nothing
// recorded — because the parser that rejects unknown values runs elsewhere and
// a description promising more than the code does is worse than no line at all.
func TestPolicyDescription_AnUnknownPolicy_PromisesNothing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy IdentityPolicy
		want   string
	}{
		{name: "full names the user", policy: IdentityFull, want: "the GitLab user id and username"},
		{name: "pseudonymous names a digest", policy: IdentityPseudonymous, want: "a per-process digest, with no readable identity"},
		{name: "none names nobody", policy: IdentityNone, want: "nothing about who made a call"},
		{name: "the zero value names nobody", policy: IdentityPolicy(""), want: "nothing about who made a call"},
		{name: "a value from a future build names nobody", policy: IdentityPolicy("hashed-with-pepper"), want: "nothing about who made a call"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := PolicyDescription(tt.policy); got != tt.want {
				t.Errorf("PolicyDescription(%q) = %q, want %q", tt.policy, got, tt.want)
			}
		})
	}
}
