package telemetry

import (
	"strings"
	"testing"
)

const probeURI = "gitlab://project/82077663/mr/42"

// attrsOf flattens a redactor's answer into key/value pairs a test can read.
func attrsOf(r *ResourceRedactor, uri string) map[string]string {
	out := map[string]string{}
	for _, kv := range r.ResourceAttributes(uri) {
		out[string(kv.Key)] = kv.Value.AsString()
	}
	return out
}

// TestResourceRedactor_FullExportsTheURI pins the level an operator reaches for
// when they want to know which project a call touched.
//
// It is gated behind the policy that already exports user.id and user.name
// because it is the same decision: a deployment recording who the caller is has
// accepted recording what they were working on, and one that records nobody has
// not. The key is the convention's own mcp.resource.uri, which is Conditionally
// Required on a span naming a resource, so this is also the level at which this
// server satisfies that requirement rather than declining it.
func TestResourceRedactor_FullExportsTheURI(t *testing.T) {
	got := attrsOf(NewResourceRedactor(IdentityFull), probeURI)

	if got[AttrResourceURI] != probeURI {
		t.Errorf("%s = %q, want the URI", AttrResourceURI, got[AttrResourceURI])
	}
	if _, present := got[AttrResourceRef]; present {
		t.Error("the digest is recorded alongside the URI, which says the same thing twice and doubles the label space")
	}
}

// TestResourceRedactor_LesserPoliciesExportOnlyADigest is the half that made
// this type necessary.
//
// A subscription poll span carried gitlab://project/82077663 verbatim on the
// hosted deployment, which contradicted this server's documented position that
// resource URIs are not exported. The contradiction mattered most on that span
// in particular: a poll repeats for the life of the watch, so a single
// subscription wrote a project id into a backend hundreds of times.
func TestResourceRedactor_LesserPoliciesExportOnlyADigest(t *testing.T) {
	for _, policy := range []IdentityPolicy{IdentityNone, IdentityPseudonymous} {
		t.Run(string(policy), func(t *testing.T) {
			got := attrsOf(NewResourceRedactor(policy), probeURI)

			if value, present := got[AttrResourceURI]; present {
				t.Errorf("%s = %q under policy %q; the URI names a project", AttrResourceURI, value, policy)
			}
			digest, present := got[AttrResourceRef]
			if !present {
				t.Fatalf("no %s under policy %q; two watchers of one kind would be indistinguishable", AttrResourceRef, policy)
			}
			for _, fragment := range []string{"gitlab", "82077663", "project"} {
				if strings.Contains(digest, fragment) {
					t.Errorf("digest %q contains %q from the URI", digest, fragment)
				}
			}
		})
	}
}

// TestResourceRedactor_NilAndEmptyRecordNothing keeps two shapes from becoming
// a fabricated value: a caller that never wired a redactor, and a request that
// named no resource. The second is the sharper one, since a digest of the empty
// string is a constant that looks like a real resource.
func TestResourceRedactor_NilAndEmptyRecordNothing(t *testing.T) {
	var nilRedactor *ResourceRedactor
	if got := nilRedactor.ResourceAttributes(probeURI); got != nil {
		t.Errorf("a nil redactor produced %v", got)
	}
	if got := NewResourceRedactor(IdentityFull).ResourceAttributes(""); got != nil {
		t.Errorf("an empty URI produced %v", got)
	}
}

// TestResourceRedactor_PolicyIsTheOnlyDifference states the invariant that
// makes one flag enough: the same URI under two policies differs in how it is
// named and in nothing else.
func TestResourceRedactor_PolicyIsTheOnlyDifference(t *testing.T) {
	full := NewResourceRedactor(IdentityFull).ResourceAttributes(probeURI)
	none := NewResourceRedactor(IdentityNone).ResourceAttributes(probeURI)

	if len(full) != 1 || len(none) != 1 {
		t.Fatalf("full recorded %d attributes and none recorded %d; each policy records exactly one", len(full), len(none))
	}
	if full[0].Key == none[0].Key {
		t.Errorf("both policies used the key %s; a consumer reading mcp.resource.uri is entitled to find a URI there", full[0].Key)
	}
}
