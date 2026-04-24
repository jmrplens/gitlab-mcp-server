// tui_test.go contains unit tests for the TUI wizard mode, verifying
// state transitions and user input handling.

package wizard

import (
	"errors"
	"io"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// keyMsg builds a tea.KeyPressMsg for a special key press (enter, tab, etc.).
func keyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

// runeMsg builds a tea.KeyPressMsg for a single printable rune.
func runeMsg(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// pasteMsg builds a tea.PasteMsg that simulates bracketed-paste input.
func pasteMsg(text string) tea.PasteMsg {
	return tea.PasteMsg{Content: text}
}

// ctrlMsg builds a tea.KeyPressMsg for ctrl+letter.
func ctrlMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModCtrl}
}

// shiftKeyMsg builds a tea.KeyPressMsg for shift+key.
func shiftKeyMsg(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Mod: tea.ModShift}
}

func newTestModel(t *testing.T) tuiModel {
	t.Helper()
	stubLoadExistingConfig(t)
	return newTUIModel("1.0.0", io.Discard)
}

// advanceToGitLab moves the model from tuiStepInstall to tuiStepGitLab.
func advanceToGitLab(m tuiModel) tuiModel {
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	return result.(tuiModel)
}

// Global key handling.

// TestUpdate_CtrlC_Aborts verifies Ctrl+C sets aborted=true and returns a
// tea.Quit command from any step.
func TestUpdate_CtrlC_Aborts(t *testing.T) {
	m := newTestModel(t)
	result, cmd := m.Update(ctrlMsg('c'))
	final := result.(tuiModel)
	if !final.aborted {
		t.Error("expected aborted=true after ctrl+c")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

// TestUpdate_Esc_Aborts verifies Esc sets aborted=true and returns a
// tea.Quit command from any step.
func TestUpdate_Esc_Aborts(t *testing.T) {
	m := newTestModel(t)
	result, cmd := m.Update(keyMsg(tea.KeyEsc))
	final := result.(tuiModel)
	if !final.aborted {
		t.Error("expected aborted=true after esc")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

// TestUpdate_RoutesToCorrectStep uses table-driven subtests to verify the
// top-level Update router does not change the current step when a harmless
// rune is received in each of the install/gitlab/options/clients steps.
func TestUpdate_RoutesToCorrectStep(t *testing.T) {
	tests := []struct {
		name string
		step tuiStep
	}{
		{"install", tuiStepInstall},
		{"gitlab", tuiStepGitLab},
		{"options", tuiStepOptions},
		{"clients", tuiStepClients},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.step = tc.step
			// Send a harmless rune — should not panic or change step
			result, _ := m.Update(runeMsg('z'))
			final := result.(tuiModel)
			if final.step != tc.step {
				t.Errorf("step changed from %d to %d", tc.step, final.step)
			}
		})
	}
}

// TestUpdate_StepDone_Quits verifies Update returns a tea.Quit command on
// any input once the model has reached tuiStepDone.
func TestUpdate_StepDone_Quits(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepDone
	_, cmd := m.Update(runeMsg('x'))
	if cmd == nil {
		t.Error("expected tea.Quit command at tuiStepDone")
	}
}

// Step Install.

// TestUpdateInstall_Enter_AdvancesToGitLab verifies pressing Enter in the
// install step advances to tuiStepGitLab with focus on the URL field.
func TestUpdateInstall_Enter_AdvancesToGitLab(t *testing.T) {
	m := newTestModel(t)
	if m.step != tuiStepInstall {
		t.Fatal("expected initial step to be tuiStepInstall")
	}
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Errorf("expected tuiStepGitLab, got %d", final.step)
	}
	if final.gitlabFocus != 0 {
		t.Error("expected gitlabFocus=0 (URL field)")
	}
}

// TestUpdateInstall_Rune_UpdatesInput verifies typed runes are written into
// the install path text input without changing the current step.
func TestUpdateInstall_Rune_UpdatesInput(t *testing.T) {
	m := newTestModel(t)
	m.installInput.SetValue("")
	result, _ := m.Update(runeMsg('/'))
	final := result.(tuiModel)
	if final.step != tuiStepInstall {
		t.Error("step should remain tuiStepInstall")
	}
	if !strings.Contains(final.installInput.Value(), "/") {
		t.Error("expected '/' to be typed into install input")
	}
}

// TestUpdateInstall_PastedEnter_DoesNotAdvance verifies a tea.PasteMsg
// containing a newline does not advance past the install step (protects
// against bracketed-paste triggering step transitions).
func TestUpdateInstall_PastedEnter_DoesNotAdvance(t *testing.T) {
	m := newTestModel(t)
	// In v2, paste events are tea.PasteMsg — they won't match tea.KeyPressMsg
	result, _ := m.Update(tea.PasteMsg{Content: "\n"})
	final := result.(tuiModel)
	if final.step != tuiStepInstall {
		t.Error("pasted enter should not advance from install step")
	}
}

// Step GitLab.

// TestUpdateGitLab_TabSwitchesFields verifies Tab moves focus from the URL
// field to the Token field and Shift+Tab moves it back.
func TestUpdateGitLab_TabSwitchesFields(t *testing.T) {
	m := newTestModel(t)
	m = advanceToGitLab(m)
	if m.gitlabFocus != 0 {
		t.Fatal("expected start on URL field")
	}

	// Tab: URL → Token
	result, _ := m.Update(keyMsg(tea.KeyTab))
	m = result.(tuiModel)
	if m.gitlabFocus != 1 {
		t.Error("expected gitlabFocus=1 after Tab")
	}

	// Shift+Tab: Token → URL
	result, _ = m.Update(shiftKeyMsg(tea.KeyTab))
	m = result.(tuiModel)
	if m.gitlabFocus != 0 {
		t.Error("expected gitlabFocus=0 after Shift+Tab")
	}
}

// TestUpdateGitLab_EnterOnURL_MovesToToken verifies Enter on the URL field
// moves focus to the Token field without advancing the step.
func TestUpdateGitLab_EnterOnURL_MovesToToken(t *testing.T) {
	m := newTestModel(t)
	m = advanceToGitLab(m)
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	final := result.(tuiModel)
	if final.gitlabFocus != 1 {
		t.Error("Enter on URL field should move focus to token")
	}
	if final.step != tuiStepGitLab {
		t.Error("step should remain tuiStepGitLab")
	}
}

// TestUpdateGitLab_EnterOnToken_ValidatesAndAdvances uses table-driven
// subtests to verify Enter on the token field validates URL and token and
// advances to tuiStepClients only for valid input, setting error messages
// for empty URL, invalid URL format, or empty token.
func TestUpdateGitLab_EnterOnToken_ValidatesAndAdvances(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		token     string
		wantStep  tuiStep
		wantError string
	}{
		{
			name:      "empty URL",
			url:       "",
			token:     "glpat-test",
			wantStep:  tuiStepGitLab,
			wantError: "GitLab URL is required",
		},
		{
			name:      "invalid URL format",
			url:       "not-a-valid-url",
			token:     "glpat-test",
			wantStep:  tuiStepGitLab,
			wantError: "Invalid URL",
		},
		{
			name:      "empty token",
			url:       "https://gitlab.example.com",
			token:     "",
			wantStep:  tuiStepGitLab,
			wantError: "Token is required",
		},
		{
			name:     "valid input",
			url:      "https://gitlab.example.com",
			token:    "glpat-abc123",
			wantStep: tuiStepClients,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m = advanceToGitLab(m)
			m.urlInput.SetValue(tc.url)
			m.tokenInput.SetValue(tc.token)
			m.gitlabFocus = 1
			result, _ := m.Update(keyMsg(tea.KeyEnter))
			final := result.(tuiModel)
			if final.step != tc.wantStep {
				t.Errorf("expected step %d, got %d", tc.wantStep, final.step)
			}
			if tc.wantError != "" && !strings.Contains(final.err, tc.wantError) {
				t.Errorf("expected error containing %q, got %q", tc.wantError, final.err)
			}
			if tc.wantError == "" && final.err != "" {
				t.Errorf("unexpected error: %q", final.err)
			}
		})
	}
}

// TestUpdateGitLab_CtrlO_OpensAdvancedOptions uses table-driven subtests to
// verify Ctrl+O from the token field validates the URL/token and advances
// to tuiStepOptions, or produces the appropriate error. From the URL field
// Ctrl+O is ignored.
func TestUpdateGitLab_CtrlO_OpensAdvancedOptions(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		token     string
		focus     int
		wantStep  tuiStep
		wantError string
	}{
		{
			name:     "valid from token field",
			url:      "https://gitlab.example.com",
			token:    "glpat-abc123",
			focus:    1,
			wantStep: tuiStepOptions,
		},
		{
			name:      "empty URL from token field",
			url:       "",
			token:     "glpat-abc123",
			focus:     1,
			wantStep:  tuiStepGitLab,
			wantError: "GitLab URL is required",
		},
		{
			name:      "empty token from token field",
			url:       "https://gitlab.example.com",
			token:     "",
			focus:     1,
			wantStep:  tuiStepGitLab,
			wantError: "Token is required",
		},
		{
			name:     "from URL field — ignored",
			url:      "https://gitlab.example.com",
			token:    "glpat-abc123",
			focus:    0,
			wantStep: tuiStepGitLab,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.step = tuiStepGitLab
			m.urlInput.SetValue(tc.url)
			m.tokenInput.SetValue(tc.token)
			m.gitlabFocus = tc.focus
			result, _ := m.Update(ctrlMsg('o'))
			final := result.(tuiModel)
			if final.step != tc.wantStep {
				t.Errorf("expected step %d, got %d", tc.wantStep, final.step)
			}
			if tc.wantError != "" && final.err != tc.wantError {
				t.Errorf("expected error %q, got %q", tc.wantError, final.err)
			}
		})
	}
}

// TestUpdateGitLab_PrintableO_DoesNotOpenAdvanced verifies typing a printable
// lowercase 'o' into the token field does not trigger the advanced options
// shortcut (which requires Ctrl+O).
func TestUpdateGitLab_PrintableO_DoesNotOpenAdvanced(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("glpat-partial")
	m.gitlabFocus = 1
	m.tokenInput.Focus()

	// Typing lowercase 'o' should NOT trigger advanced options
	result, _ := m.Update(runeMsg('o'))
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Errorf("typing 'o' should not change step, got %d", final.step)
	}
	if final.showAdvanced {
		t.Error("typing 'o' should not set showAdvanced")
	}
}

// TestUpdateGitLab_PastedText_DoesNotTriggerShortcuts verifies bracketed
// paste of a token containing 'o' does not trigger advanced options nor
// change the current step.
func TestUpdateGitLab_PastedText_DoesNotTriggerShortcuts(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("")
	m.gitlabFocus = 1
	m.tokenInput.Focus()

	// Simulate bracketed paste of a token containing 'o'
	result, _ := m.Update(pasteMsg("glpat-xyzABCo123"))
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Error("pasted text should not change step")
	}
	if final.showAdvanced {
		t.Error("pasted text should not trigger advanced options")
	}
}

// TestUpdateGitLab_PastedEnter_DoesNotAdvance verifies a pasted newline does
// not advance past the GitLab step.
func TestUpdateGitLab_PastedEnter_DoesNotAdvance(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("glpat-test")
	m.gitlabFocus = 1

	// In v2, paste events are tea.PasteMsg — they won't match tea.KeyPressMsg
	result, _ := m.Update(tea.PasteMsg{Content: "\n"})
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Error("pasted Enter should not advance from GitLab step")
	}
}

// TestUpdateGitLab_Rune_UpdatesInput verifies typed runes are written into
// the focused URL input.
func TestUpdateGitLab_Rune_UpdatesInput(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("")
	m.urlInput.Focus()
	m.gitlabFocus = 0

	result, _ := m.Update(runeMsg('h'))
	final := result.(tuiModel)
	if !strings.Contains(final.urlInput.Value(), "h") {
		t.Error("expected 'h' to be typed into URL input")
	}
}

// Step Options.

// TestUpdateOptions_Navigation verifies Up/Down arrows move optCursor
// within bounds and Up at 0 stays at 0.
func TestUpdateOptions_Navigation(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	m.optCursor = 0

	// Move down
	result, _ := m.Update(keyMsg(tea.KeyDown))
	m = result.(tuiModel)
	if m.optCursor != 1 {
		t.Errorf("expected optCursor=1 after Down, got %d", m.optCursor)
	}

	// Move up
	result, _ = m.Update(keyMsg(tea.KeyUp))
	m = result.(tuiModel)
	if m.optCursor != 0 {
		t.Errorf("expected optCursor=0 after Up, got %d", m.optCursor)
	}

	// Up at 0 stays at 0
	result, _ = m.Update(keyMsg(tea.KeyUp))
	m = result.(tuiModel)
	if m.optCursor != 0 {
		t.Error("optCursor should not go below 0")
	}
}

// TestUpdateOptions_ToggleAll uses table-driven subtests to verify Space
// toggles each advanced option (skipTLS, meta-tools, auto-update, YOLO).
func TestUpdateOptions_ToggleAll(t *testing.T) {
	tests := []struct {
		cursor int
		field  func(tuiModel) bool
		name   string
	}{
		{0, func(m tuiModel) bool { return m.optSkipTLS }, "skipTLS"},
		{1, func(m tuiModel) bool { return m.optMeta }, "meta"},
		{2, func(m tuiModel) bool { return m.optAutoUpd }, "autoUpdate"},
		{3, func(m tuiModel) bool { return m.optYolo }, "yolo"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.step = tuiStepOptions
			m.optCursor = tc.cursor
			before := tc.field(m)
			result, _ := m.Update(keyMsg(tea.KeySpace))
			after := tc.field(result.(tuiModel))
			if before == after {
				t.Errorf("expected toggle for %s", tc.name)
			}
		})
	}
}

// TestUpdateOptions_LogLevelCycles verifies Space on the log level row
// cycles optLogLevel to the next value.
func TestUpdateOptions_LogLevelCycles(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	m.optCursor = 4
	m.optLogLevel = 0

	result, _ := m.Update(keyMsg(tea.KeySpace))
	final := result.(tuiModel)
	if final.optLogLevel != 1 {
		t.Errorf("expected optLogLevel=1, got %d", final.optLogLevel)
	}
}

// TestUpdateOptions_Enter_AdvancesToClients verifies Enter in the options
// step advances the model to tuiStepClients.
func TestUpdateOptions_Enter_AdvancesToClients(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	final := result.(tuiModel)
	if final.step != tuiStepClients {
		t.Errorf("expected tuiStepClients, got %d", final.step)
	}
}

// TestUpdateOptions_KJ_NavigatesUpDown verifies vim-style 'j' and 'k' keys
// move optCursor down and up respectively.
func TestUpdateOptions_KJ_NavigatesUpDown(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	m.optCursor = 2

	result, _ := m.Update(runeMsg('j'))
	m = result.(tuiModel)
	if m.optCursor != 3 {
		t.Errorf("expected optCursor=3 after 'j', got %d", m.optCursor)
	}

	result, _ = m.Update(runeMsg('k'))
	m = result.(tuiModel)
	if m.optCursor != 2 {
		t.Errorf("expected optCursor=2 after 'k', got %d", m.optCursor)
	}
}

// TestUpdateOptions_X_Toggles verifies 'x' toggles the option under the
// cursor (tested against the YOLO row).
func TestUpdateOptions_X_Toggles(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	m.optCursor = 3 // YOLO
	before := m.optYolo

	result, _ := m.Update(runeMsg('x'))
	final := result.(tuiModel)
	if final.optYolo == before {
		t.Error("expected 'x' to toggle optYolo")
	}
}

// TestUpdateOptions_DownAtMax_StaysAtMax verifies Down at the last option
// row leaves optCursor clamped at its maximum value.
func TestUpdateOptions_DownAtMax_StaysAtMax(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepOptions
	m.optCursor = 4

	result, _ := m.Update(keyMsg(tea.KeyDown))
	final := result.(tuiModel)
	if final.optCursor != 4 {
		t.Errorf("expected optCursor=4 at max, got %d", final.optCursor)
	}
}

// Step Clients.

// TestUpdateClients_Navigation verifies Up/Down arrows move clientCursor
// within bounds and Up at 0 stays at 0.
func TestUpdateClients_Navigation(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	m.clientCursor = 0

	result, _ := m.Update(keyMsg(tea.KeyDown))
	m = result.(tuiModel)
	if m.clientCursor != 1 {
		t.Errorf("expected clientCursor=1, got %d", m.clientCursor)
	}

	result, _ = m.Update(keyMsg(tea.KeyUp))
	m = result.(tuiModel)
	if m.clientCursor != 0 {
		t.Errorf("expected clientCursor=0, got %d", m.clientCursor)
	}

	// Up at 0 stays
	result, _ = m.Update(keyMsg(tea.KeyUp))
	m = result.(tuiModel)
	if m.clientCursor != 0 {
		t.Error("clientCursor should not go below 0")
	}
}

// TestUpdateClients_DownAtMax_StaysAtMax verifies Down at the last client
// row clamps clientCursor to the final index.
func TestUpdateClients_DownAtMax_StaysAtMax(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	m.clientCursor = len(m.clients) - 1

	result, _ := m.Update(keyMsg(tea.KeyDown))
	final := result.(tuiModel)
	if final.clientCursor != len(m.clients)-1 {
		t.Error("clientCursor should not exceed max")
	}
}

// TestUpdateClients_SpaceToggles verifies Space toggles the selection of
// the client under the cursor.
func TestUpdateClients_SpaceToggles(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	m.clientCursor = 0
	before := m.clientSel[0]

	result, _ := m.Update(keyMsg(tea.KeySpace))
	final := result.(tuiModel)
	if final.clientSel[0] == before {
		t.Error("Space should toggle client selection")
	}
}

// TestUpdateClients_X_Toggles verifies 'x' toggles the selection of the
// client under the cursor.
func TestUpdateClients_X_Toggles(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	m.clientCursor = 1
	before := m.clientSel[1]

	result, _ := m.Update(runeMsg('x'))
	final := result.(tuiModel)
	if final.clientSel[1] == before {
		t.Error("'x' should toggle client selection")
	}
}

// TestUpdateClients_A_SelectsAll verifies 'a' selects every client when
// starting from a fully-deselected state.
func TestUpdateClients_A_SelectsAll(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	// Deselect all first
	for i := range m.clientSel {
		m.clientSel[i] = false
	}

	result, _ := m.Update(runeMsg('a'))
	final := result.(tuiModel)
	for i, sel := range final.clientSel {
		if !sel {
			t.Errorf("client %d should be selected after 'a'", i)
		}
	}
}

// TestUpdateClients_A_DeselectsAll_WhenAllSelected verifies 'a' deselects
// every client when all are currently selected (toggle-all behavior).
func TestUpdateClients_A_DeselectsAll_WhenAllSelected(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	for i := range m.clientSel {
		m.clientSel[i] = true
	}

	result, _ := m.Update(runeMsg('a'))
	final := result.(tuiModel)
	for i, sel := range final.clientSel {
		if sel {
			t.Errorf("client %d should be deselected after 'a' when all selected", i)
		}
	}
}

// TestUpdateClients_Enter_AdvancesToDone verifies Enter from the clients
// step sets done=true, advances to tuiStepDone, and returns a tea.Quit
// command. Uses stubInstallBinary to avoid touching the filesystem.
func TestUpdateClients_Enter_AdvancesToDone(t *testing.T) {
	stubInstallBinary(t)

	m := newTestModel(t)
	m.step = tuiStepClients
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("glpat-test")

	result, cmd := m.Update(keyMsg(tea.KeyEnter))
	final := result.(tuiModel)
	if final.step != tuiStepDone {
		t.Errorf("expected tuiStepDone, got %d", final.step)
	}
	if !final.done {
		t.Error("expected done=true")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

// TestUpdateClients_PastedText_DoesNotTriggerShortcuts verifies pasted text
// containing 'a', 'x', 'j', or 'k' does not toggle selections or move the
// cursor (paste events must not fire keyboard shortcuts).
func TestUpdateClients_PastedText_DoesNotTriggerShortcuts(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepClients
	m.clientCursor = 0
	before := m.clientSel[0]

	// Paste containing 'a', 'x', 'j', 'k' should not trigger shortcuts
	result, _ := m.Update(pasteMsg("ajxk"))
	final := result.(tuiModel)
	if final.clientSel[0] != before {
		t.Error("pasted text should not toggle client selection")
	}
}

// View rendering.

// TestView_ContainsStepContent uses table-driven subtests to verify the
// rendered view for each step (install, gitlab, options, clients) contains
// its expected header text.
func TestView_ContainsStepContent(t *testing.T) {
	tests := []struct {
		name     string
		step     tuiStep
		contains string
	}{
		{"install", tuiStepInstall, "Binary Installation"},
		{"gitlab", tuiStepGitLab, "GitLab Configuration"},
		{"options", tuiStepOptions, "Advanced Options"},
		{"clients", tuiStepClients, "MCP Client Configuration"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t)
			m.step = tc.step
			view := m.View().Content
			if !strings.Contains(view, tc.contains) {
				t.Errorf("expected view to contain %q", tc.contains)
			}
		})
	}
}

// TestViewGitLab_ShowsCtrlOHelp verifies the GitLab view advertises the
// Ctrl+O shortcut for advanced options and does not suggest a plain 'o' key.
func TestViewGitLab_ShowsCtrlOHelp(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	view := m.View().Content
	if !strings.Contains(view, "Ctrl+O") {
		t.Error("expected help text to mention Ctrl+O for advanced options")
	}
	if strings.Contains(view, "'o'") {
		t.Error("help text should NOT mention 'o' as shortcut")
	}
}

// TestViewGitLab_ShowsScopeAndCtrlT verifies the GitLab view displays the
// required token scope (api) and the Ctrl+T shortcut hint.
func TestViewGitLab_ShowsScopeAndCtrlT(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	view := m.View().Content
	if !strings.Contains(view, "Scope: api") {
		t.Error("expected scope hint in GitLab view")
	}
	if !strings.Contains(view, "Ctrl+T") {
		t.Error("expected Ctrl+T hint in GitLab view")
	}
}

// TestUpdateGitLab_CtrlT_OpensBrowser verifies Ctrl+T from the GitLab step
// calls openBrowserFn with the GitLab personal access token creation URL
// (including scopes=api) without changing the current step.
func TestUpdateGitLab_CtrlT_OpensBrowser(t *testing.T) {
	orig := openBrowserFn
	var openedURL string
	openBrowserFn = func(u string) error { openedURL = u; return nil }
	t.Cleanup(func() { openBrowserFn = orig })

	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.gitlabFocus = 1

	result, _ := m.Update(ctrlMsg('t'))
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Error("Ctrl+T should not change step")
	}
	if openedURL == "" {
		t.Fatal("expected openBrowserFn to be called")
	}
	if !strings.Contains(openedURL, "personal_access_tokens") {
		t.Errorf("expected token creation URL, got %q", openedURL)
	}
	if !strings.Contains(openedURL, "scopes=api") {
		t.Errorf("expected scopes=api in URL, got %q", openedURL)
	}
}

// TestViewGitLab_ShowsError verifies a populated err field is rendered in
// the GitLab view.
func TestViewGitLab_ShowsError(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.err = "Token is required"
	view := m.View().Content
	if !strings.Contains(view, "Token is required") {
		t.Error("expected error message in view")
	}
}

// TestView_ContainsHeader verifies every view includes the wizard header
// "gitlab-mcp-server Setup Wizard".
func TestView_ContainsHeader(t *testing.T) {
	m := newTestModel(t)
	view := m.View().Content
	if !strings.Contains(view, "gitlab-mcp-server Setup Wizard") {
		t.Error("expected header in view")
	}
}

// TestView_ContainsVersion verifies the view renders the configured
// version string (e.g. "v1.0.0").
func TestView_ContainsVersion(t *testing.T) {
	m := newTestModel(t)
	view := m.View().Content
	if !strings.Contains(view, "v1.0.0") {
		t.Error("expected version in view")
	}
}

// TestView_ContainsCancelHelp verifies the footer includes the
// "Esc/Ctrl+C to cancel" help text.
func TestView_ContainsCancelHelp(t *testing.T) {
	m := newTestModel(t)
	view := m.View().Content
	if !strings.Contains(view, "Esc/Ctrl+C to cancel") {
		t.Error("expected cancel help text in view")
	}
}

// Init.

// TestInit_ReturnsBatchCmd verifies the model's Init method returns a
// non-nil tea.Cmd (used to blink the initial text input).
func TestInit_ReturnsBatchCmd(t *testing.T) {
	m := newTestModel(t)
	cmd := m.Init()
	if cmd == nil {
		t.Error("expected Init to return a non-nil command")
	}
}

// Full flow: Install → GitLab → Clients.

// TestFullFlow_InstallToGitLabToClients drives the TUI end-to-end through
// install, GitLab, and clients steps with Tab/Enter key events, and verifies
// the model reaches tuiStepDone with done=true, a non-nil result, and a
// tea.Quit command.
func TestFullFlow_InstallToGitLabToClients(t *testing.T) {
	stubInstallBinary(t)

	m := newTestModel(t)

	// Step 1: Install → press Enter
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)
	if m.step != tuiStepGitLab {
		t.Fatalf("expected tuiStepGitLab, got %d", m.step)
	}

	// Step 2: GitLab — fill URL and token
	m.urlInput.SetValue("https://gitlab.example.com")
	result, _ = m.Update(keyMsg(tea.KeyTab))
	m = result.(tuiModel)
	if m.gitlabFocus != 1 {
		t.Fatal("expected focus on token field")
	}

	// Type token
	m.tokenInput.SetValue("glpat-full-flow-test")

	// Press Enter to continue
	result, _ = m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)
	if m.step != tuiStepClients {
		t.Fatalf("expected tuiStepClients, got %d", m.step)
	}

	// Step 3: Clients — press Enter to finish
	result, cmd := m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)
	if m.step != tuiStepDone {
		t.Fatalf("expected tuiStepDone, got %d", m.step)
	}
	if !m.done {
		t.Error("expected done=true")
	}
	if m.result == nil {
		t.Error("expected result to be set")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

// Full flow with advanced options.

// TestFullFlow_WithAdvancedOptions drives the TUI end-to-end via Ctrl+O into
// the options step, toggles YOLO mode, and continues through clients to
// tuiStepDone, verifying the options branch is exercised.
func TestFullFlow_WithAdvancedOptions(t *testing.T) {
	stubInstallBinary(t)

	m := newTestModel(t)

	// Install → Enter
	result, _ := m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)

	// GitLab: fill URL and token, go to token
	m.urlInput.SetValue("https://gitlab.example.com")
	result, _ = m.Update(keyMsg(tea.KeyTab))
	m = result.(tuiModel)
	m.tokenInput.SetValue("glpat-adv-test")

	// Ctrl+O for advanced options
	result, _ = m.Update(ctrlMsg('o'))
	m = result.(tuiModel)
	if m.step != tuiStepOptions {
		t.Fatalf("expected tuiStepOptions, got %d", m.step)
	}
	if !m.showAdvanced {
		t.Error("expected showAdvanced=true")
	}

	// Toggle YOLO (cursor → 3, then space)
	m.optCursor = 3
	result, _ = m.Update(keyMsg(tea.KeySpace))
	m = result.(tuiModel)
	if !m.optYolo {
		t.Error("expected optYolo=true after toggle")
	}

	// Enter → Clients
	result, _ = m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)
	if m.step != tuiStepClients {
		t.Fatalf("expected tuiStepClients from options, got %d", m.step)
	}

	// Enter → Done
	result, cmd := m.Update(keyMsg(tea.KeyEnter))
	m = result.(tuiModel)
	if m.step != tuiStepDone {
		t.Fatalf("expected tuiStepDone, got %d", m.step)
	}
	if cmd == nil {
		t.Error("expected tea.Quit")
	}
}

// Paste safety (regression tests for the reported bugs).

// TestPasteSafety_TokenWithO_DoesNotOpenAdvanced is a regression test that
// feeds a token containing 'o' rune-by-rune and verifies neither the step
// nor the showAdvanced flag change mid-input.
func TestPasteSafety_TokenWithO_DoesNotOpenAdvanced(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("")
	m.gitlabFocus = 1
	m.tokenInput.Focus()

	// Simulate pasting a token character by character — 'o' must NOT trigger advanced
	for _, r := range "glpat-xoY9zO" {
		result, _ := m.Update(runeMsg(r))
		m = result.(tuiModel)
		if m.step != tuiStepGitLab {
			t.Fatalf("rune %q caused step change to %d", r, m.step)
		}
		if m.showAdvanced {
			t.Fatalf("rune %q triggered showAdvanced", r)
		}
	}
}

// TestPasteSafety_BracketedPaste_NoShortcuts verifies a bracketed paste
// (tea.PasteMsg) of a token containing shortcut letters does not change
// the current step.
func TestPasteSafety_BracketedPaste_NoShortcuts(t *testing.T) {
	m := newTestModel(t)
	m.step = tuiStepGitLab
	m.urlInput.SetValue("https://gitlab.example.com")
	m.tokenInput.SetValue("")
	m.gitlabFocus = 1
	m.tokenInput.Focus()

	// Bracketed paste — entire string at once, Paste=true
	result, _ := m.Update(pasteMsg("glpat-test-OXYGEN-token"))
	final := result.(tuiModel)
	if final.step != tuiStepGitLab {
		t.Errorf("bracketed paste should not change step, got %d", final.step)
	}
}

var errTestSentinel = errors.New("test install failure")

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
