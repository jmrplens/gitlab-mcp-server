//go:build e2e

// spec.go reads what one configured model is: which adapter, which model, and
// the generation options that adapter may be given.
//
// The options are allow-listed per provider and an unknown key is refused
// rather than ignored. A key that is quietly dropped is a run reporting under
// the name of a configuration it did not have, which is the same defect as a
// misspelled surface: the row says reasoning_effort=none and the request
// carried no such field, and nobody reading the table can tell.

package provider

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// The option keys a spec may carry. They are spelled once here because the
// allow-list, the parser and the record's own rendering all name them.
const (
	// optionTemperature is the sampling temperature, or the word "default",
	// which omits the field.
	optionTemperature = "temperature"
	// optionMaxTokens is the ceiling on one answer.
	optionMaxTokens = "max_tokens"
	// optionReasoningEffort is OpenAI's reasoning control. It is the option
	// that decides whether the gpt-5.6 family will accept function tools at
	// all, which is why it exists here rather than being left to a default.
	optionReasoningEffort = "reasoning_effort"
	// optionEnableThinking is DashScope's switch for the thinking mode its
	// OpenAI-compatible endpoint refuses to stream tool calls under.
	optionEnableThinking = "enable_thinking"
)

// temperatureDefault is the value of optionTemperature that omits the field.
//
// It is a word rather than an absence because the two are different claims and
// the record publishes them differently: a spec that says nothing about
// temperature gets the deterministic 0 every measurement wants, and a spec that
// says "default" is asking for the provider's own, which is what a model that
// refuses the field needs.
const temperatureDefault = "default"

// allowedOptions is what each adapter accepts, and the refusal message's list.
var allowedOptions = map[string][]string{
	Anthropic: {optionTemperature, optionMaxTokens},
	OpenAI:    {optionTemperature, optionMaxTokens, optionReasoningEffort},
	Qwen:      {optionTemperature, optionMaxTokens, optionEnableThinking},
	Google:    {optionTemperature, optionMaxTokens},
	Fake:      {},
}

// Spec is one configured model: the adapter, the model and the request options
// as they will go on the wire.
type Spec struct {
	// Raw is the string this was parsed from, which is what a row names the
	// model by.
	Raw string
	// Provider is the adapter.
	Provider string
	// Model is the model identifier, or the fake's variant.
	Model string
	// Temperature is the sampling temperature, or nil to omit the field.
	Temperature *float64
	// MaxTokens is the ceiling on one answer.
	MaxTokens int
	// ReasoningEffort is OpenAI's reasoning control, empty to omit it.
	ReasoningEffort string
	// EnableThinking is DashScope's thinking switch, nil to omit it.
	EnableThinking *bool
}

// String returns the spec as it was configured, which is the identifier every
// line of the record joins on.
func (s Spec) String() string { return s.Raw }

// Wire renders the request options the way a published row must spell them.
//
// A temperature that is omitted reads "provider default" and never "0": they
// are different requests, and a table that printed the second for the first
// would be claiming a determinism the run did not ask for.
func (s Spec) Wire() map[string]string {
	options := map[string]string{
		optionMaxTokens: strconv.Itoa(s.MaxTokens),
	}
	if s.Temperature == nil {
		options[optionTemperature] = "provider default"
	} else {
		options[optionTemperature] = strconv.FormatFloat(*s.Temperature, 'g', -1, 64)
	}
	if s.ReasoningEffort != "" {
		options[optionReasoningEffort] = s.ReasoningEffort
	}
	if s.EnableThinking != nil {
		options[optionEnableThinking] = strconv.FormatBool(*s.EnableThinking)
	}
	return options
}

// ParseSpecs reads a comma-separated model list.
//
// An empty list is an error rather than a default model: a run that was not
// told which model to ask is a run nobody can read the results of, and guessing
// one spends money on a question that was not asked.
func ParseSpecs(list string) ([]Spec, error) {
	var specs []Spec
	for raw := range strings.SplitSeq(list, ",") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		spec, err := ParseSpec(raw)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(specs, func(seen Spec) bool { return seen.Raw == spec.Raw }) {
			continue
		}
		specs = append(specs, spec)
	}
	if len(specs) == 0 {
		return nil, fmt.Errorf("no model configured in %q", list)
	}
	return specs, nil
}

// ParseSpec reads one provider:model;key=value string.
func ParseSpec(raw string) (Spec, error) {
	trimmed := strings.TrimSpace(raw)
	head, options, err := splitSpec(trimmed)
	if err != nil {
		return Spec{}, err
	}
	spec, err := parseHead(trimmed, head)
	if err != nil {
		return Spec{}, err
	}
	if applyErr := applyOptions(&spec, options); applyErr != nil {
		return Spec{}, applyErr
	}
	applyDefaults(&spec, options)
	return spec, nil
}

// splitSpec separates the provider:model head from the options.
func splitSpec(raw string) (head string, options map[string]string, err error) {
	parts := strings.Split(raw, ";")
	options = map[string]string{}
	for _, part := range parts[1:] {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			return "", nil, fmt.Errorf("option %q in %q is not key=value", trimmed, raw)
		}
		key = strings.TrimSpace(key)
		if _, repeated := options[key]; repeated {
			return "", nil, fmt.Errorf("option %q is given twice in %q", key, raw)
		}
		options[key] = strings.TrimSpace(value)
	}
	return parts[0], options, nil
}

// parseHead reads the provider and the model.
func parseHead(raw, head string) (Spec, error) {
	provider, model, found := strings.Cut(strings.TrimSpace(head), ":")
	if !found {
		return Spec{}, fmt.Errorf("%q names no provider: write one of %s before the model, "+
			"since the same model name means different request shapes at different providers",
			raw, strings.Join(Names(), ", "))
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	if _, known := allowedOptions[provider]; !known {
		return Spec{}, fmt.Errorf("unsupported provider %q in %q, want one of %s",
			provider, raw, strings.Join(Names(), ", "))
	}
	model = strings.TrimSpace(model)
	if provider == Google {
		// The API takes the bare identifier in the path and the docs spell it
		// with this prefix, so both spellings are accepted and one is sent.
		model = strings.TrimPrefix(model, "models/")
	}
	if model == "" {
		return Spec{}, fmt.Errorf("%q names no model", raw)
	}
	return Spec{Raw: raw, Provider: provider, Model: model}, nil
}

// applyOptions reads the allow-listed options onto the spec.
//
// These four take the spec as a parameter rather than being methods on it,
// because everything a caller outside this file holds is a value and mixing the
// two receiver kinds on one type is how a copy comes to be modified and thrown
// away.
func applyOptions(s *Spec, options map[string]string) error {
	allowed := allowedOptions[s.Provider]
	for _, key := range slices.Sorted(maps.Keys(options)) {
		if !slices.Contains(allowed, key) {
			return fmt.Errorf("provider %s does not take option %q: it takes %s",
				s.Provider, key, acceptedList(allowed))
		}
		if err := applyOption(s, key, options[key]); err != nil {
			return err
		}
	}
	return nil
}

// applyOption reads one option.
func applyOption(s *Spec, key, value string) error {
	switch key {
	case optionTemperature:
		return applyTemperature(s, value)
	case optionMaxTokens:
		tokens, err := strconv.Atoi(value)
		if err != nil || tokens <= 0 {
			return fmt.Errorf("%s=%q is not a positive number of tokens", optionMaxTokens, value)
		}
		s.MaxTokens = tokens
		return nil
	case optionReasoningEffort:
		if value == "" {
			return fmt.Errorf("%s is empty: omit the option to send no reasoning control", optionReasoningEffort)
		}
		s.ReasoningEffort = value
		return nil
	default:
		thinking, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s=%q is not a boolean", optionEnableThinking, value)
		}
		s.EnableThinking = &thinking
		return nil
	}
}

// applyTemperature reads the temperature, which is the one option with a word
// among its values.
func applyTemperature(s *Spec, value string) error {
	if value == temperatureDefault {
		s.Temperature = nil
		return nil
	}
	degrees, err := strconv.ParseFloat(value, 64)
	if err != nil || degrees < 0 || degrees > 2 {
		return fmt.Errorf("%s=%q is neither %q nor a number between 0 and 2",
			optionTemperature, value, temperatureDefault)
	}
	s.Temperature = &degrees
	return nil
}

// applyDefaults fills what the spec did not say.
//
// Temperature 0 is the default because a measurement wants the same answer
// twice, with one exception that is not a taste: a model configured with a
// reasoning effort is one of the family that refuses the temperature field
// outright, so a spec that names the effort and not the temperature gets the
// provider's own rather than a request the provider will reject. Naming the
// temperature explicitly beside the effort overrides that, because a provider
// that starts accepting both should not need a code change to be measured.
func applyDefaults(s *Spec, options map[string]string) {
	if s.MaxTokens == 0 {
		s.MaxTokens = defaultMaxTokens
	}
	if _, stated := options[optionTemperature]; stated {
		return
	}
	if s.ReasoningEffort != "" {
		s.Temperature = nil
		return
	}
	zero := 0.0
	s.Temperature = &zero
}

// acceptedList spells an allow-list for a refusal.
func acceptedList(allowed []string) string {
	if len(allowed) == 0 {
		return "no options at all"
	}
	return strings.Join(allowed, ", ")
}
