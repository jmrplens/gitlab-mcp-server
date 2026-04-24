// prompt_test.go contains unit tests for interactive prompt input handling.

package wizard

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrompter_AskString_ValidInput(t *testing.T) {
	r := strings.NewReader("hello world\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskString("Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestPrompter_AskString_EmptyThenValid(t *testing.T) {
	r := strings.NewReader("\n  \nhello\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskString("Name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
	if !strings.Contains(w.String(), "cannot be empty") {
		t.Error("expected empty-input warning in output")
	}
}

func TestPrompter_AskStringDefault_UsesDefault(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskStringDefault("URL", "https://gitlab.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://gitlab.com" {
		t.Errorf("got %q, want %q", got, "https://gitlab.com")
	}
}

func TestPrompter_AskStringDefault_OverridesDefault(t *testing.T) {
	r := strings.NewReader("https://custom.dev\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskStringDefault("URL", "https://gitlab.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "https://custom.dev" {
		t.Errorf("got %q, want %q", got, "https://custom.dev")
	}
}

func TestPrompter_AskYesNo_Defaults(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
	}{
		{"empty default yes", "\n", true, true},
		{"empty default no", "\n", false, false},
		{"y", "y\n", false, true},
		{"Y", "Y\n", false, true},
		{"yes", "yes\n", false, true},
		{"n", "n\n", true, false},
		{"N", "N\n", true, false},
		{"no", "no\n", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			w := &bytes.Buffer{}
			p := NewPrompter(r, w)

			got, err := p.AskYesNo("Continue?", tt.defaultYes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrompter_AskChoice_ValidSelection(t *testing.T) {
	r := strings.NewReader("2\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskChoice("Pick", []string{"A", "B", "C"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("got index %d, want 1", got)
	}
}

func TestPrompter_AskChoice_InvalidThenValid(t *testing.T) {
	r := strings.NewReader("0\n5\n1\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskChoice("Pick", []string{"A", "B"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Errorf("got index %d, want 0", got)
	}
}

func TestPrompter_AskMultiChoice_SpaceSeparated(t *testing.T) {
	r := strings.NewReader("1 3\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskMultiChoice("Select", []string{"A", "B", "C"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []bool{true, false, true}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("index %d: got %v, want %v", i, v, expected[i])
		}
	}
}

func TestPrompter_AskMultiChoice_All(t *testing.T) {
	r := strings.NewReader("a\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskMultiChoice("Select", []string{"A", "B"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range got {
		if !v {
			t.Errorf("index %d: got false, want true", i)
		}
	}
}

func TestPrompter_AskPassword_ValidInput(t *testing.T) {
	r := strings.NewReader("glpat-secret123\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskPassword("Token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "glpat-secret123" {
		t.Errorf("got %q, want %q", got, "glpat-secret123")
	}
}

// TestPrompter_AskPassword_EmptyThenValid verifies AskPassword rejects empty
// input and retries until it gets a non-empty value.
func TestPrompter_AskPassword_EmptyThenValid(t *testing.T) {
	r := strings.NewReader("\n  \nsecret\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskPassword("Token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}
	if !strings.Contains(w.String(), "cannot be empty") {
		t.Error("expected empty-input warning in output")
	}
}

// TestPrompter_AskString_EOF verifies AskString returns an error when
// the input stream ends before a valid value is provided.
func TestPrompter_AskString_EOF(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	_, err := p.AskString("Name")
	if err == nil {
		t.Fatal("expected error for EOF input, got nil")
	}
}

// TestPrompter_AskStringDefault_EOF verifies AskStringDefault returns
// an error when EOF is encountered.
func TestPrompter_AskStringDefault_EOF(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	_, err := p.AskStringDefault("URL", "https://default.com")
	if err == nil {
		t.Fatal("expected error for EOF input, got nil")
	}
}

// TestPrompter_AskYesNo_InvalidInput verifies that unrecognized input
// falls back to the default value.
func TestPrompter_AskYesNo_InvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
	}{
		{"garbage defaults to yes", "maybe\n", true, true},
		{"garbage defaults to no", "maybe\n", false, false},
		{"number defaults to yes", "42\n", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.input)
			w := &bytes.Buffer{}
			p := NewPrompter(r, w)

			got, err := p.AskYesNo("Continue?", tt.defaultYes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPrompter_AskYesNo_EOF verifies AskYesNo returns an error on EOF.
func TestPrompter_AskYesNo_EOF(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	_, err := p.AskYesNo("Continue?", false)
	if err == nil {
		t.Fatal("expected error for EOF input, got nil")
	}
}

// TestPrompter_AskChoice_EOF verifies AskChoice returns an error on EOF.
func TestPrompter_AskChoice_EOF(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	_, err := p.AskChoice("Pick", []string{"A", "B"})
	if err == nil {
		t.Fatal("expected error for EOF input, got nil")
	}
}

// TestPrompter_AskMultiChoice_DefaultsOnEnter verifies that pressing Enter
// with no input returns the default selections.
func TestPrompter_AskMultiChoice_DefaultsOnEnter(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	defaults := []bool{true, false, true}
	got, err := p.AskMultiChoice("Select", []string{"A", "B", "C"}, defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, want := range defaults {
		if got[i] != want {
			t.Errorf("index %d: got %v, want %v", i, got[i], want)
		}
	}
}

// TestPrompter_AskMultiChoice_InvalidThenValid verifies that invalid
// selections are rejected and the user is re-prompted.
func TestPrompter_AskMultiChoice_InvalidThenValid(t *testing.T) {
	r := strings.NewReader("0 99\n2\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskMultiChoice("Select", []string{"A", "B", "C"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := []bool{false, true, false}
	for i, v := range got {
		if v != expected[i] {
			t.Errorf("index %d: got %v, want %v", i, v, expected[i])
		}
	}
}

// TestPrompter_AskMultiChoice_AllKeyword verifies "all" keyword works.
func TestPrompter_AskMultiChoice_AllKeyword(t *testing.T) {
	r := strings.NewReader("all\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskMultiChoice("Select", []string{"A", "B"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, v := range got {
		if !v {
			t.Errorf("index %d: got false, want true", i)
		}
	}
}

// TestPrompter_AskMultiChoice_EOF verifies AskMultiChoice returns error on EOF.
func TestPrompter_AskMultiChoice_EOF(t *testing.T) {
	r := strings.NewReader("")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	_, err := p.AskMultiChoice("Select", []string{"A", "B"}, nil)
	if err == nil {
		t.Fatal("expected error for EOF input, got nil")
	}
}

func TestPrompter_AskPasswordDefault_ReturnsDefaultOnEmpty(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskPasswordDefault("Token", "glpat-abc123def456ghi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "glpat-abc123def456ghi" {
		t.Errorf("got %q, want default value", got)
	}
	if !strings.Contains(w.String(), MaskToken("glpat-abc123def456ghi")) {
		t.Error("expected masked token hint in output")
	}
}

func TestPrompter_AskPasswordDefault_OverridesDefault(t *testing.T) {
	r := strings.NewReader("glpat-newtoken789\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskPasswordDefault("Token", "glpat-oldtoken123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "glpat-newtoken789" {
		t.Errorf("got %q, want %q", got, "glpat-newtoken789")
	}
}

func TestPrompter_AskPasswordDefault_ShortToken(t *testing.T) {
	r := strings.NewReader("\n")
	w := &bytes.Buffer{}
	p := NewPrompter(r, w)

	got, err := p.AskPasswordDefault("Token", "short")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "short" {
		t.Errorf("got %q, want %q", got, "short")
	}
	if !strings.Contains(w.String(), MaskToken("short")) {
		t.Error("expected masked short token in output")
	}
}
