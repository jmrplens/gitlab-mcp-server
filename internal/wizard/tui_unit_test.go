package wizard

import (
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
)

// TestBuildResult_EmptyInstallPath verifies buildResult uses the default
// install path when the install input is empty.
func TestBuildResult_EmptyInstallPath(t *testing.T) {
	stubInstallBinary(t)
	m := tuiModel{
		installInput: textinput.New(),
		urlInput:     textinput.New(),
		tokenInput:   textinput.New(),
		clientSel:    []bool{true, false},
		optLogLevel:  1,
	}
	m.buildResult()
	if m.result == nil {
		t.Fatal("expected result, got nil")
	}
	if m.result.BinaryPath == "" {
		t.Error("BinaryPath should not be empty")
	}
}

// TestBuildResult_InstallBinaryFails verifies buildResult falls back to the
// current executable when InstallBinary fails.
func TestBuildResult_InstallBinaryFails(t *testing.T) {
	orig := installBinaryFn
	installBinaryFn = func(string) (string, error) {
		return "", errTestSentinel
	}
	t.Cleanup(func() { installBinaryFn = orig })

	input := textinput.New()
	input.SetValue("/tmp/test-dir/gitlab-mcp-server")
	m := tuiModel{
		installInput: input,
		urlInput:     textinput.New(),
		tokenInput:   textinput.New(),
		clientSel:    []bool{},
		optLogLevel:  0,
	}
	m.buildResult()
	if m.result == nil {
		t.Fatal("expected result, got nil")
	}
	if m.result.BinaryPath == "" {
		t.Error("BinaryPath should fall back to current executable")
	}
}

// TestViewGitLab_Focus0_WithExistingToken_AndError verifies viewGitLab renders
// focused URL field, existing token hint, and error message.
func TestViewGitLab_Focus0_WithExistingToken_AndError(t *testing.T) {
	m := tuiModel{
		step:             tuiStepGitLab,
		gitlabFocus:      0,
		hasExistingToken: true,
		err:              "validation error",
		urlInput:         textinput.New(),
		tokenInput:       textinput.New(),
	}
	output := m.viewGitLab(60)
	if !strings.Contains(output, "▸") {
		t.Error("expected focus indicator ▸ for gitlabFocus=0")
	}
	if !strings.Contains(output, "Existing token loaded") {
		t.Error("expected existing token hint")
	}
	if !strings.Contains(output, "validation error") {
		t.Error("expected error message in output")
	}
}

var errTestSentinel = errors.New("test install failure")
