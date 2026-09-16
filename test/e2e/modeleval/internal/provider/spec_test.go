//go:build e2e

package provider

import (
	"strings"
	"testing"
)

func TestParseSpec_ReadsTheProviderTheModelAndTheOptions(t *testing.T) {
	for _, one := range []struct {
		name        string
		raw         string
		provider    string
		model       string
		temperature string
		maxTokens   int
	}{
		{
			"a bare model", "anthropic:claude-haiku-4-5-20251001", Anthropic,
			"claude-haiku-4-5-20251001", "0", defaultMaxTokens,
		},
		{"a temperature", "openai:gpt-5.4-nano;temperature=0.7", OpenAI, "gpt-5.4-nano", "0.7", defaultMaxTokens},
		{
			"the provider's own temperature", "qwen:qwen3.8-flash;temperature=default", Qwen,
			"qwen3.8-flash", "provider default", defaultMaxTokens,
		},
		{"a ceiling", "google:gemini-flash-latest;max_tokens=128", Google, "gemini-flash-latest", "0", 128},
		{
			"the documented google prefix", "google:models/gemini-flash-latest", Google,
			"gemini-flash-latest", "0", defaultMaxTokens,
		},
		{"a variant", "fake:perfect", Fake, "perfect", "0", defaultMaxTokens},
	} {
		t.Run(one.name, func(t *testing.T) {
			spec, err := ParseSpec(one.raw)
			if err != nil {
				t.Fatalf("ParseSpec(%q): %v", one.raw, err)
			}
			if spec.Provider != one.provider || spec.Model != one.model {
				t.Errorf("provider/model = %s/%s, want %s/%s",
					spec.Provider, spec.Model, one.provider, one.model)
			}
			if spec.MaxTokens != one.maxTokens {
				t.Errorf("MaxTokens = %d, want %d", spec.MaxTokens, one.maxTokens)
			}
			if got := spec.Wire()[optionTemperature]; got != one.temperature {
				t.Errorf("wire temperature = %q, want %q", got, one.temperature)
			}
			if spec.String() != one.raw {
				t.Errorf("String() = %q, want the spec as configured %q", spec.String(), one.raw)
			}
		})
	}
}

func TestParseSpec_AReasoningModelIsNotSentATemperatureItRefuses(t *testing.T) {
	// The gpt-5.6 family refuses function tools without a reasoning effort,
	// and the gpt-5.5 one refuses the temperature the old adapter always
	// sent. A spec that names the effort therefore omits the field, and the
	// row says "provider default" rather than claiming a 0 nobody sent.
	effort, err := ParseSpec("openai:gpt-5.6-luna;reasoning_effort=none")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if effort.Temperature != nil {
		t.Errorf("Temperature = %v, want it omitted beside a reasoning effort", *effort.Temperature)
	}
	if got := effort.Wire()[optionTemperature]; got != "provider default" {
		t.Errorf("wire temperature = %q, want %q", got, "provider default")
	}
	if got := effort.Wire()[optionReasoningEffort]; got != "none" {
		t.Errorf("wire reasoning_effort = %q, want none", got)
	}

	// Naming both is honored, so a provider that starts accepting the pair
	// can be measured without a code change.
	both, err := ParseSpec("openai:gpt-5.6-luna;reasoning_effort=low;temperature=0")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if both.Temperature == nil || *both.Temperature != 0 {
		t.Errorf("Temperature = %v, want the 0 the spec asked for", both.Temperature)
	}
}

func TestParseSpec_TheThinkingSwitchIsSentOnlyWhenItWasAskedFor(t *testing.T) {
	quiet, err := ParseSpec("qwen:qwen3.8-flash;enable_thinking=false")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if quiet.EnableThinking == nil || *quiet.EnableThinking {
		t.Errorf("EnableThinking = %v, want false", quiet.EnableThinking)
	}
	if got := quiet.Wire()[optionEnableThinking]; got != "false" {
		t.Errorf("wire enable_thinking = %q, want false", got)
	}

	plain, err := ParseSpec("qwen:qwen3.8-flash")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if plain.EnableThinking != nil {
		t.Error("the switch is set without being asked for, which sends a field the row does not name")
	}
	if _, present := plain.Wire()[optionEnableThinking]; present {
		t.Error("the wire options name a switch that was not sent")
	}
}

func TestParseSpec_RefusesWhatItCannotSend(t *testing.T) {
	for _, one := range []struct{ name, raw, says string }{
		{"no provider", "claude-haiku-4-5", "names no provider"},
		{"an unknown provider", "mistral:large", "unsupported provider"},
		{"no model", "anthropic:", "names no model"},
		{"an option that is not a pair", "anthropic:m;temperature", "is not key=value"},
		{"an option given twice", "anthropic:m;temperature=0;temperature=1", "is given twice"},
		{"an unknown option", "anthropic:m;top_p=0.9", "does not take option"},
		{"an option another provider takes", "anthropic:m;reasoning_effort=low", "does not take option"},
		{"a temperature that is not a number", "anthropic:m;temperature=warm", "neither"},
		{"a temperature out of range", "anthropic:m;temperature=9", "neither"},
		{"a ceiling that is not a count", "anthropic:m;max_tokens=lots", "positive number of tokens"},
		{"a ceiling of zero", "anthropic:m;max_tokens=0", "positive number of tokens"},
		{"an empty reasoning effort", "openai:m;reasoning_effort=", "omit the option"},
		{"a thinking switch that is not a boolean", "qwen:m;enable_thinking=maybe", "not a boolean"},
		{"an option the fake does not take", "fake:perfect;temperature=0", "no options at all"},
	} {
		t.Run(one.name, func(t *testing.T) {
			_, err := ParseSpec(one.raw)
			if err == nil {
				t.Fatalf("ParseSpec(%q) was accepted", one.raw)
			}
			if !strings.Contains(err.Error(), one.says) {
				t.Errorf("the refusal of %q does not say %q: %v", one.raw, one.says, err)
			}
		})
	}
}

func TestParseSpecs_ReadsAListAndRefusesAnEmptyOne(t *testing.T) {
	specs, err := ParseSpecs("anthropic:claude-haiku-4-5-20251001, openai:gpt-5.4-nano;max_tokens=64 ,," +
		"anthropic:claude-haiku-4-5-20251001")
	if err != nil {
		t.Fatalf("ParseSpecs: %v", err)
	}
	// The repeated entry is dropped: a list assembled by a shell loop may
	// repeat itself, and asking one model twice doubles a run's cost for no
	// second measurement.
	if len(specs) != 2 {
		t.Fatalf("ParseSpecs returned %d specs, want 2: %v", len(specs), specs)
	}
	if specs[1].MaxTokens != 64 {
		t.Errorf("the second spec's MaxTokens = %d, want 64", specs[1].MaxTokens)
	}

	if _, emptyErr := ParseSpecs("  , "); emptyErr == nil {
		t.Error("an empty list was accepted, so a run would have to guess which model was meant")
	}
	if _, badErr := ParseSpecs("anthropic:m;top_p=1"); badErr == nil {
		t.Error("a list carrying a bad spec was accepted")
	}
}

func TestSpecWire_SaysWhatWentOnTheWireAndNotWhatWasConfigured(t *testing.T) {
	spec, err := ParseSpec("anthropic:claude-haiku-4-5-20251001;max_tokens=2048")
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	wire := spec.Wire()
	if wire[optionMaxTokens] != "2048" {
		t.Errorf("wire max_tokens = %q, want 2048", wire[optionMaxTokens])
	}
	if _, present := wire[optionReasoningEffort]; present {
		t.Error("the wire options name a reasoning effort no request carried")
	}
}
