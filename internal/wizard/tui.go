// tui.go implements the terminal UI (TUI) wizard mode using the Bubble Tea
// framework for interactive, in-terminal configuration.
package wizard

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// TUI color palette matches the web UI dark theme so all wizard modes keep a
// consistent visual identity.
var (
	colorText      = lipgloss.Color("#e6edf3")
	colorMuted     = lipgloss.Color("#8b949e")
	colorAccent    = lipgloss.Color("#58a6ff")
	colorSuccess   = lipgloss.Color("#3fb950")
	colorError     = lipgloss.Color("#f85149")
	colorHighlight = lipgloss.Color("#1f6feb")
)

// TUI styles centralize lipgloss rendering choices used by all wizard views.
var (
	tuiAccentStyle  = lipgloss.NewStyle().Foreground(colorAccent)
	tuiSuccessStyle = lipgloss.NewStyle().Foreground(colorSuccess)
	tuiMutedStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	tuiErrorStyle   = lipgloss.NewStyle().Foreground(colorError).Bold(true)

	tuiHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			Background(colorHighlight).
			Padding(0, 2).
			Align(lipgloss.Center)

	tuiVersionStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Align(lipgloss.Center)

	tuiActivePanelStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorAccent).
				Padding(1, 2)

	tuiSectionTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			MarginBottom(1)

	tuiProgressDone    = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	tuiProgressActive  = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	tuiProgressPending = lipgloss.NewStyle().Foreground(colorMuted)

	tuiHelpStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	tuiListItemStyle = lipgloss.NewStyle().Foreground(colorText)
	tuiCursorStyle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
)

// tuiStep identifies the current screen in the terminal wizard state machine.
type tuiStep int

// TUI wizard steps are ordered in the same sequence the user completes them.
const (
	tuiStepInstall tuiStep = iota
	tuiStepGitLab
	tuiStepOptions
	tuiStepClients
	tuiStepDone
)

// tuiModel stores all Bubble Tea state for the terminal wizard, including
// focused inputs, option selections, client selections, and the final result.
type tuiModel struct { //nolint:recvcheck // buildResult needs pointer receiver, Bubble Tea interface requires value receivers
	version string
	step    tuiStep
	err     string
	w       io.Writer

	// Step 1: install
	installInput textinput.Model

	// Step 2: GitLab
	urlInput         textinput.Model
	tokenInput       textinput.Model
	gitlabFocus      int // 0=url, 1=token
	hasExistingToken bool

	// Step 3: options
	optCursor   int
	optSkipTLS  bool
	optMeta     bool
	optAutoUpd  bool
	optYolo     bool
	optLogLevel int // index into LogLevelOptions

	// Step 4: clients
	clients      []ClientInfo
	clientSel    []bool
	clientCursor int

	// Result
	result       *Result
	showAdvanced bool
	done         bool
	aborted      bool
}

// newTUIModel creates the initial terminal wizard model with defaults from any
// existing configuration and sensible client selections.
func newTUIModel(version string, w io.Writer) tuiModel {
	// Load existing configuration as defaults
	existing, hasExisting := loadExistingConfigFn()

	installInput := textinput.New()
	installInput.Placeholder = filepath.Join(DefaultInstallDir(), DefaultBinaryName())
	installInput.SetValue(installInput.Placeholder)
	installInput.Focus()
	installInput.CharLimit = 256
	installInput.SetWidth(60)

	defaultURL := DefaultGitLabURL
	if hasExisting && existing.GitLabURL != "" {
		defaultURL = existing.GitLabURL
	}

	urlInput := textinput.New()
	urlInput.Placeholder = defaultURL
	urlInput.SetValue(defaultURL)
	urlInput.CharLimit = 256
	urlInput.SetWidth(60)

	tokenInput := textinput.New()
	if hasExisting && existing.GitLabToken != "" {
		tokenInput.Placeholder = MaskToken(existing.GitLabToken)
		tokenInput.SetValue(existing.GitLabToken)
	} else {
		tokenInput.Placeholder = "glpat-..."
	}
	tokenInput.EchoMode = textinput.EchoPassword
	tokenInput.CharLimit = 256
	tokenInput.SetWidth(60)

	skipTLS := false
	if hasExisting {
		skipTLS = existing.SkipTLSVerify
	}

	clients := AllClients()
	sel := make([]bool, len(clients))
	for i, c := range clients {
		sel[i] = c.DefaultSelected
	}

	return tuiModel{
		version:          version,
		step:             tuiStepInstall,
		w:                w,
		installInput:     installInput,
		urlInput:         urlInput,
		tokenInput:       tokenInput,
		hasExistingToken: hasExisting && existing.GitLabToken != "",
		optSkipTLS:       skipTLS,
		optMeta:          true,
		optAutoUpd:       true,
		optLogLevel:      1, // "info"
		clients:          clients,
		clientSel:        sel,
	}
}

// Init implements [tea.Model] by starting the cursor blink and clearing the
// screen on wizard launch.
func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tea.ClearScreen)
}

// Update implements [tea.Model] by routing the incoming message to the
// per-step handler (install, GitLab credentials, options, clients) and
// handling global quit shortcuts (Ctrl+C, Esc).
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c", "esc":
			m.aborted = true
			return m, tea.Quit
		}
	}

	switch m.step {
	case tuiStepInstall:
		return m.updateInstall(msg)
	case tuiStepGitLab:
		return m.updateGitLab(msg)
	case tuiStepOptions:
		return m.updateOptions(msg)
	case tuiStepClients:
		return m.updateClients(msg)
	case tuiStepDone:
		return m, tea.Quit
	}
	return m, nil
}

// updateInstall handles the install-path step and advances to GitLab settings
// when the user presses Enter.
func (m tuiModel) updateInstall(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "enter" {
		m.step = tuiStepGitLab
		m.urlInput.Focus()
		m.gitlabFocus = 0
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.installInput, cmd = m.installInput.Update(msg)
	return m, cmd
}

// updateGitLab handles URL/token input, validation, token-link shortcut, and
// optional navigation into advanced settings.
func (m tuiModel) updateGitLab(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "enter":
			if m.gitlabFocus == 0 {
				m.urlInput.SetValue(effectiveGitLabURL(m.urlInput.Value()))
				m.gitlabFocus = 1
				m.urlInput.Blur()
				m.tokenInput.Focus()
				return m, textinput.Blink
			}
			m.urlInput.SetValue(effectiveGitLabURL(m.urlInput.Value()))
			if _, parseErr := url.ParseRequestURI(m.urlInput.Value()); parseErr != nil {
				m.err = fmt.Sprintf("Invalid URL: %v", parseErr)
				return m, nil
			}
			if m.tokenInput.Value() == "" {
				m.err = "Token is required"
				return m, nil
			}
			m.err = ""
			// Skip advanced options, go straight to clients
			m.step = tuiStepClients
			m.tokenInput.Blur()
			return m, nil
		case "ctrl+t":
			_ = openBrowserFn(TokenCreationURL(m.urlInput.Value()))
			return m, nil
		case "ctrl+o":
			// Only from token field: open advanced options
			if m.gitlabFocus == 1 {
				m.urlInput.SetValue(effectiveGitLabURL(m.urlInput.Value()))
				if m.tokenInput.Value() == "" {
					m.err = "Token is required"
					return m, nil
				}
				m.err = ""
				m.showAdvanced = true
				m.step = tuiStepOptions
				m.tokenInput.Blur()
				return m, nil
			}
		case "shift+tab":
			if m.gitlabFocus == 1 {
				m.gitlabFocus = 0
				m.tokenInput.Blur()
				m.urlInput.Focus()
				return m, textinput.Blink
			}
		case "tab":
			if m.gitlabFocus == 0 {
				m.gitlabFocus = 1
				m.urlInput.Blur()
				m.tokenInput.Focus()
				return m, textinput.Blink
			}
		}
	}

	var cmd tea.Cmd
	if m.gitlabFocus == 0 {
		m.urlInput, cmd = m.urlInput.Update(msg)
	} else {
		m.tokenInput, cmd = m.tokenInput.Update(msg)
	}
	return m, cmd
}

// updateOptions handles keyboard navigation and toggles for advanced wizard
// settings such as TLS verification, meta-tools, auto-update, and log level.
func (m tuiModel) updateOptions(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.optCursor > 0 {
				m.optCursor--
			}
		case "down", "j":
			if m.optCursor < 4 { // 5 options: 0-4
				m.optCursor++
			}
		case "space", "x":
			switch m.optCursor {
			case 0:
				m.optSkipTLS = !m.optSkipTLS
			case 1:
				m.optMeta = !m.optMeta
			case 2:
				m.optAutoUpd = !m.optAutoUpd
			case 3:
				m.optYolo = !m.optYolo
			case 4:
				m.optLogLevel = (m.optLogLevel + 1) % len(LogLevelOptions)
			}
		case "enter":
			m.step = tuiStepClients
			return m, nil
		}
	}
	return m, nil
}

// updateClients handles client selection, select-all behavior, and completion
// of the wizard once the user confirms the chosen MCP clients.
func (m tuiModel) updateClients(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.clientCursor > 0 {
				m.clientCursor--
			}
		case "down", "j":
			if m.clientCursor < len(m.clients)-1 {
				m.clientCursor++
			}
		case "space", "x":
			m.clientSel[m.clientCursor] = !m.clientSel[m.clientCursor]
		case "a":
			allSelected := true
			for _, s := range m.clientSel {
				if !s {
					allSelected = false
					break
				}
			}
			for i := range m.clientSel {
				m.clientSel[i] = !allSelected
			}
		case "enter":
			m.buildResult()
			m.step = tuiStepDone
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// buildResult converts the current TUI selections into the shared wizard
// [Result] structure consumed by [Apply].
func (m *tuiModel) buildResult() {
	installPath := m.installInput.Value()
	if installPath == "" {
		installPath = filepath.Join(DefaultInstallDir(), DefaultBinaryName())
	}

	var selected []int
	for i, sel := range m.clientSel {
		if sel {
			selected = append(selected, i)
		}
	}

	binaryPath := installPath
	installDir := installPath
	if strings.HasSuffix(installDir, DefaultBinaryName()) {
		installDir = filepath.Dir(installDir)
	}
	expandedDir, err := ExpandPath(installDir)
	if err == nil {
		installed, installErr := InstallBinary(expandedDir)
		if installErr == nil {
			binaryPath = installed
		} else {
			exe, _ := os.Executable()
			binaryPath = exe
		}
	}

	m.result = &Result{
		InstallDir: installDir,
		BinaryPath: binaryPath,
		Config: ServerConfig{
			BinaryPath:    binaryPath,
			GitLabURL:     m.urlInput.Value(),
			GitLabToken:   m.tokenInput.Value(),
			SkipTLSVerify: m.optSkipTLS,
			MetaTools:     m.optMeta,
			AutoUpdate:    m.optAutoUpd,
			YoloMode:      m.optYolo,
			LogLevel:      LogLevelOptions[m.optLogLevel],
		},
		SelectedClients: selected,
	}
}

// View implements [tea.Model] by rendering the current wizard step (header,
// progress bar, step-specific UI, and footer) into a styled bubbletea view.
func (m tuiModel) View() tea.View {
	var b strings.Builder
	const panelWidth = 64

	// Header
	header := tuiHeaderStyle.Width(panelWidth).Render("gitlab-mcp-server Setup Wizard")
	version := tuiVersionStyle.Width(panelWidth).Render(fmt.Sprintf("v%s — GitLab MCP Server for AI Assistants", m.version))
	b.WriteString(header + "\n" + version + "\n\n")

	// Progress bar
	b.WriteString(m.renderProgress(panelWidth))
	b.WriteString("\n\n")

	// Current step panel
	var panel string
	switch m.step {
	case tuiStepInstall:
		panel = m.viewInstall(panelWidth)
	case tuiStepGitLab:
		panel = m.viewGitLab(panelWidth)
	case tuiStepOptions:
		panel = m.viewOptions(panelWidth)
	case tuiStepClients:
		panel = m.viewClients(panelWidth)
	}
	b.WriteString(panel)

	// Footer
	b.WriteString("\n")
	b.WriteString(tuiHelpStyle.Render("  Esc/Ctrl+C to cancel"))
	b.WriteString("\n")

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

// renderProgress returns the centered progress indicator shown above the
// current wizard panel.
func (m tuiModel) renderProgress(width int) string {
	type stepInfo struct {
		name      string
		completed bool
		active    bool
	}
	steps := []stepInfo{
		{"Install", m.step > tuiStepInstall, m.step == tuiStepInstall},
		{"GitLab", m.step > tuiStepGitLab && m.step != tuiStepOptions, m.step == tuiStepGitLab || m.step == tuiStepOptions},
		{"Clients", m.step > tuiStepClients, m.step == tuiStepClients},
	}

	var parts []string
	for i, s := range steps {
		var icon, label string
		if s.completed {
			icon = tuiProgressDone.Render("✓")
			label = tuiProgressDone.Render(s.name)
		} else if s.active {
			icon = tuiProgressActive.Render("●")
			label = tuiProgressActive.Render(s.name)
		} else {
			icon = tuiProgressPending.Render("○")
			label = tuiProgressPending.Render(s.name)
		}
		parts = append(parts, fmt.Sprintf(" %s %s ", icon, label))
		if i < len(steps)-1 {
			if s.completed {
				parts = append(parts, tuiProgressDone.Render("━━━"))
			} else {
				parts = append(parts, tuiProgressPending.Render("───"))
			}
		}
	}

	bar := strings.Join(parts, "")
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Center).Render(bar)
}

// viewInstall renders the binary installation step.
func (m tuiModel) viewInstall(width int) string {
	var content strings.Builder
	content.WriteString(tuiSectionTitle.Render("Binary Installation") + "\n\n")
	content.WriteString(tuiListItemStyle.Render("Install path:") + "\n")
	content.WriteString(m.installInput.View() + "\n\n")
	content.WriteString(tuiHelpStyle.Render("Enter to continue"))
	return tuiActivePanelStyle.Width(width).Render(content.String())
}

// viewGitLab renders the GitLab URL and token step, including validation
// errors and existing-token hints.
func (m tuiModel) viewGitLab(width int) string {
	var content strings.Builder
	content.WriteString(tuiSectionTitle.Render("GitLab Configuration") + "\n\n")

	if m.gitlabFocus == 0 {
		content.WriteString(tuiAccentStyle.Render("▸ ") + tuiListItemStyle.Render("GitLab URL:") + "\n")
	} else {
		content.WriteString("  " + tuiListItemStyle.Render("GitLab URL:") + "\n")
	}
	content.WriteString("  " + m.urlInput.View() + "\n\n")

	if m.gitlabFocus == 1 {
		content.WriteString(tuiAccentStyle.Render("▸ ") + tuiListItemStyle.Render("Personal Access Token:") + "\n")
	} else {
		content.WriteString("  " + tuiListItemStyle.Render("Personal Access Token:") + "\n")
	}
	content.WriteString("  " + m.tokenInput.View() + "\n")
	if m.hasExistingToken {
		content.WriteString(tuiMutedStyle.Render("  Existing token loaded · Edit to overwrite") + "\n")
	}
	content.WriteString(tuiMutedStyle.Render("  Scope: api · Ctrl+T to create token in browser") + "\n")

	if m.err != "" {
		content.WriteString("\n" + tuiErrorStyle.Render("  ✗ "+m.err) + "\n")
	}

	content.WriteString("\n" + tuiHelpStyle.Render("Tab/Shift+Tab switch · Enter continue · Ctrl+O options"))
	return tuiActivePanelStyle.Width(width).Render(content.String())
}

// viewOptions renders the advanced options step.
func (m tuiModel) viewOptions(width int) string {
	var content strings.Builder
	content.WriteString(tuiSectionTitle.Render("⚙ Advanced Options") + "\n\n")

	opts := []struct {
		name string
		on   bool
	}{
		{"Skip TLS verification", m.optSkipTLS},
		{"Enable meta-tools", m.optMeta},
		{"Enable auto-update", m.optAutoUpd},
		{"Enable YOLO mode", m.optYolo},
	}
	for i, opt := range opts {
		cursor := "  "
		if m.optCursor == i {
			cursor = tuiCursorStyle.Render("▸ ")
		}
		check := tuiMutedStyle.Render("[ ]")
		if opt.on {
			check = tuiSuccessStyle.Render("[✓]")
		}
		fmt.Fprintf(&content, "%s%s %s\n", cursor, check, tuiListItemStyle.Render(opt.name))
	}

	cursor := "  "
	if m.optCursor == 4 {
		cursor = tuiCursorStyle.Render("▸ ")
	}
	fmt.Fprintf(&content, "%s    Log level: %s\n", cursor, tuiAccentStyle.Render(LogLevelOptions[m.optLogLevel]))

	content.WriteString("\n" + tuiHelpStyle.Render("↑↓ navigate · Space toggle · Enter continue"))
	return tuiActivePanelStyle.Width(width).Render(content.String())
}

// viewClients renders the MCP client selection step.
func (m tuiModel) viewClients(width int) string {
	var content strings.Builder
	content.WriteString(tuiSectionTitle.Render("MCP Client Configuration") + "\n\n")

	for i, c := range m.clients {
		cursor := "  "
		if i == m.clientCursor {
			cursor = tuiCursorStyle.Render("▸ ")
		}
		check := tuiMutedStyle.Render("[ ]")
		if m.clientSel[i] {
			check = tuiSuccessStyle.Render("[✓]")
		}
		name := tuiListItemStyle.Render(c.Name)
		if c.DisplayOnly {
			name += tuiMutedStyle.Render(" (prints JSON)")
		}
		fmt.Fprintf(&content, "%s%s %s\n", cursor, check, name)
	}

	content.WriteString("\n" + tuiHelpStyle.Render("↑↓ navigate · Space toggle · a select all · Enter configure"))
	return tuiActivePanelStyle.Width(width).Render(content.String())
}

// RunTUI runs the Bubble Tea interactive setup wizard.
// It uses the alternate screen buffer to provide a clean full-screen experience.
func RunTUI(version string, w io.Writer) error {
	model := newTUIModel(version, w)
	p := tea.NewProgram(model, tea.WithOutput(w))
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	final, ok := finalModel.(tuiModel)
	if !ok {
		return errors.New("unexpected model type")
	}
	if final.aborted {
		fmt.Fprintln(w, "\n  Setup cancelled.")
		return nil
	}

	if final.result == nil {
		return nil
	}

	printSection(w, "Writing Configurations (TUI)")
	return Apply(w, final.result)
}
