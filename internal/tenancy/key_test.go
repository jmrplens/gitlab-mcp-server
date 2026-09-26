package tenancy

import (
	"strings"
	"testing"
)

// TestKey_EveryKeyIsDescribed pins each key's name, axis, mint cost and unit
// to the key table (spec: Keys), and holds it to evidence a refusal can quote.
func TestKey_EveryKeyIsDescribed(t *testing.T) {
	for _, tc := range []struct {
		key  Key
		name string
		axis Axis
		mint MintCost
		unit Unit
	}{
		{KeyNone, "none", AxisNone, NotMintable, UnitNone},
		{KeyRequest, "request", AxisRequest, MintFree, UnitRequest},
		{KeyCredential, "credential", AxisRequester, MintCredential, UnitCredential},
		{KeyEntry, "entry", AxisRequester, MintCredential, UnitCredential},
		{KeyOwner, "owner", AxisRequester, MintCredential, UnitCredential},
		{KeyTenant, "tenant", AxisRequester, MintPrincipal, UnitPrincipal},
		{KeyVerified, "verified", AxisRequester, MintCredential, UnitCredential},
		{KeyApplication, "application", AxisRequester, MintFree, UnitApplication},
		{KeySession, "session", AxisRequester, MintFree, UnitSession},
		{KeyAddress, "address", AxisPreAdmission, MintAddress, UnitAddress},
		{KeySource, "source", AxisPreAdmission, MintAddress, UnitAddress},
		{KeyRefused, "refused", AxisPreAdmission, MintFree, UnitCredential},
		{KeyUnbound, "unbound", AxisProcess, NotMintable, UnitProcess},
		{KeyProcess, "process", AxisProcess, NotMintable, UnitProcess},
		{KeyDeployment, "deployment", AxisProcess, NotMintable, UnitProcess},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key.String(); got != tc.name {
				t.Errorf("String() = %q, want %q", got, tc.name)
			}
			if got := tc.key.Axis(); got != tc.axis {
				t.Errorf("Axis() = %d, want %d", got, tc.axis)
			}
			if got := tc.key.MintCost(); got != tc.mint {
				t.Errorf("MintCost() = %d, want %d", got, tc.mint)
			}
			if got := tc.key.Unit(); got != tc.unit {
				t.Errorf("Unit() = %d, want %d", got, tc.unit)
			}
			if evidence := tc.key.Evidence(); len(evidence) < 30 || !strings.HasSuffix(evidence, ".") {
				t.Errorf("Evidence() = %q, want a sentence a refusal can quote", evidence)
			}
		})
	}
}

// TestKey_MintableIsFalseExactlyForTheKeysNoCallerControls is the mintable-key
// rule as a test (spec: Two axes): every key a request yields is mintable, the
// tenant included, and only the absence of a key, the unbound state, the
// process and the deployment are not.
func TestKey_MintableIsFalseExactlyForTheKeysNoCallerControls(t *testing.T) {
	notMintable := map[Key]bool{KeyNone: true, KeyUnbound: true, KeyProcess: true, KeyDeployment: true}
	keys := Keys()
	if len(keys) != 15 {
		t.Fatalf("Keys() returned %d keys, want fifteen: KeyNone and the fourteen of the spec's key table", len(keys))
	}
	for _, k := range keys {
		t.Run(k.String(), func(t *testing.T) {
			if k.Mintable() == notMintable[k] {
				t.Errorf("Mintable() = %v", k.Mintable())
			}
		})
	}
	if !KeyTenant.Mintable() || KeyTenant.MintCost() != MintPrincipal {
		t.Error("the tenant must be mintable, at the cost of another GitLab user (spec: Two axes)")
	}
}

// TestKey_UnknownKeyIsNamedByItsNumber gives a value outside the vocabulary a
// readable name and nothing else.
func TestKey_UnknownKeyIsNamedByItsNumber(t *testing.T) {
	k := Key(200)
	if k.String() != "Key(200)" || k.Mintable() || k.Axis() != AxisNone || k.Unit() != UnitNone ||
		k.Evidence() != "" || k.Derivation() != nil {
		t.Errorf("Key(200) = %q, %v, %d, %d, %q, %v", k, k.Mintable(), k.Axis(), k.Unit(), k.Evidence(), k.Derivation())
	}
}

// TestKey_DerivationsAreDeclaredBySymbol holds each key's derivation to the
// symbols the design names, each with the derive role, and the keys with no
// derivation to none.
func TestKey_DerivationsAreDeclaredBySymbol(t *testing.T) {
	for _, tc := range []struct {
		key  Key
		want string
	}{
		{KeyNone, ""},
		{KeyRequest, ""},
		{KeyCredential, "internal/serverpool.ExtractToken,cmd/server.mcpServerGate.extractCredential"},
		{KeyEntry, "internal/serverpool.sessionKey,internal/serverpool.canonicalHost"},
		{KeyOwner, "internal/serverpool.ServerPool.buildEntry"},
		{KeyTenant, "internal/serverpool.resolveIdentity,internal/oauth.admitToken,cmd/server.prepareStdioCatalog"},
		{KeyVerified, "internal/oauth.tokenKey"},
		{KeyApplication, "internal/oauth.acceptedRecipient"},
		{KeySession, "cmd/server.sessionOwners.record"},
		{KeyAddress, "cmd/server.clientIP"},
		{KeySource, "cmd/server.transportSource"},
		{KeyRefused, "internal/oauth.rejectedKey,internal/serverpool.DistinctTokenBudget.Charge"},
		{KeyUnbound, ""},
		{KeyProcess, ""},
		{KeyDeployment, ""},
	} {
		t.Run(tc.key.String(), func(t *testing.T) {
			var names []string
			for _, s := range tc.key.Derivation() {
				if s.Role != Derive {
					t.Errorf("%s.%s has role %d, want Derive", s.Pkg, s.Name, s.Role)
				}
				names = append(names, s.Pkg+"."+s.Name)
			}
			if got := strings.Join(names, ","); got != tc.want {
				t.Errorf("Derivation() = %s, want %s", got, tc.want)
			}
		})
	}
}
