//go:build e2e

package provider

import (
	"encoding/json"
	"testing"
)

// TestToolDigest_TheFourAdaptersHashOneToolListTheSame is the regression for
// finding F8, and it is the reason the digest exists at all.
//
// The evaluator this replaces injected thirty-six parameter names into the
// dynamic execute schema for OpenAI and Qwen only. Two columns of a published
// cross-vendor table were therefore measuring a surface the other two never
// received, and nothing on the row said so. One tool list in, four equal
// digests out: an adapter that starts rewriting a schema fails here, and if it
// somehow did not, the digest it publishes on its row would differ from its
// siblings' and a reader could see it.
func TestToolDigest_TheFourAdaptersHashOneToolListTheSame(t *testing.T) {
	tools := sampleTools(t)
	digests := map[string]string{}
	for _, raw := range []string{
		"anthropic:claude-haiku-4-5-20251001",
		"openai:gpt-5.4-nano",
		"qwen:qwen3.8-flash",
		"google:gemini-flash-latest",
		"fake:perfect",
	} {
		t.Run(raw, func(t *testing.T) {
			spec, err := ParseSpec(raw)
			if err != nil {
				t.Fatalf("ParseSpec(%q): %v", raw, err)
			}
			built, err := New(Config{Spec: spec, APIKey: testKey})
			if err != nil {
				t.Fatalf("New(%q): %v", raw, err)
			}
			digests[raw] = built.ToolDigest(tools)
		})
	}

	var first, firstOf string
	for raw, digest := range digests {
		if digest == "" {
			t.Errorf("%s hashed the tool list to nothing", raw)
		}
		if first == "" {
			first, firstOf = digest, raw
			continue
		}
		if digest != first {
			t.Errorf("%s hashes the tool list to %s and %s to %s: one of them is not sending the "+
				"schemas it was given, which is exactly what this digest exists to catch",
				raw, digest, firstOf, first)
		}
	}
}

func TestToolDigest_ChangesWithWhatIsSentAndWithTheOrderItIsSentIn(t *testing.T) {
	tools := sampleTools(t)
	adapter := adapterFor(t, "openai:gpt-5.4-nano", "")
	base := adapter.ToolDigest(tools)

	reordered := []Tool{tools[1], tools[0]}
	if adapter.ToolDigest(reordered) == base {
		t.Error("two orders of one list hash the same: a surface that lists its tools in another " +
			"sequence is a different prompt")
	}

	rewritten := append([]Tool(nil), tools...)
	injected, err := NewTool(rewritten[1].Name, rewritten[1].Description, json.RawMessage(
		`{"type":"object","properties":{"action":{"type":"string"},
		  "params":{"type":"object","properties":{"project_id":{"type":"string"}}}},
		  "required":["action","params"]}`,
	))
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	rewritten[1] = injected
	if adapter.ToolDigest(rewritten) == base {
		t.Error("a schema carrying an injected parameter hashes the same as the one the server " +
			"published, which is the rewrite this digest is for")
	}

	if adapter.ToolDigest(nil) == base {
		t.Error("an empty tool list hashes the same as a populated one")
	}
}

func TestDigestTools_IsTotalEvenOnSomethingItCannotEncode(t *testing.T) {
	// A raw message holding text that is not JSON cannot be marshaled. It
	// cannot arrive through NewTool, which refuses one, so this asserts the
	// fallback rather than a reachable path: a digest that panicked would
	// take a run down at the session line, and one that returned the empty
	// string would collide with the empty list.
	broken := digestTools([]toolEntry{{Name: "t", Schema: json.RawMessage(`{"`)}})
	if broken == "" {
		t.Fatal("an undigestable entry hashed to nothing")
	}
	if broken == digestTools(nil) {
		t.Error("an undigestable entry hashes the same as an empty list")
	}
	if len(broken) != digestLength {
		t.Errorf("digest length = %d, want %d", len(broken), digestLength)
	}
}
