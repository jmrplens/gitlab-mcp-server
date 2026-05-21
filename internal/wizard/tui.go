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

const (
	tuiOptSkipTLS = iota
	tuiOptToolSurface
	tuiOptCapabilitySurface
	tuiOptMetaParamSchema
	tuiOptEnterprise
	tuiOptReadOnly
	tuiOptSafeMode
	tuiOptEmbeddedResources
	tuiOptIgnoreScopes
	tuiOptExcludeTools
	tuiOptUploadMaxFileSize
	tuiOptAutoUpdateMode
	tuiOptAutoUpdateRepo
	tuiOptAutoUpdateTimeout
	tuiOptRateLimitRPS
	tuiOptRateLimitBurst
	tuiOptYolo
	tuiOptLogLevel
	tuiOptionCount
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
	optCursor            int
	optEditing           bool
	optEditInput         textinput.Model
	optSkipTLS           bool
	optMeta              bool
	optToolSurface       int
	optCapabilitySurface int
	optMetaParamSchema   int
	optEnterprise        bool
	optReadOnly          bool
	optSafeMode          bool
	optEmbeddedResources bool
	optExcludeTools      string
	optIgnoreScopes      bool
	optUploadMaxFileSize string
	optAutoUpd           bool
	optAutoUpdateMode    int
	optAutoUpdateRepo    string
	optAutoUpdateTimeout string
	optRateLimitRPS      string
	optRateLimitBurst    string
	optYolo              bool
	optLogLevel          int // index into LogLevelOptions

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

	cfg := DefaultServerConfig()
	if hasExisting {
		cfg = existing.withDefaults()
	}

	optEditInput := textinput.New()
	optEditInput.CharLimit = 256
	optEditInput.SetWidth(42)

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
		optEditInput:     optEditInput,
		optSkipTLS:       cfg.SkipTLSVerify,
		optMeta:          cfg.MetaTools,
		optToolSurface:   choiceIndex(ToolSurfaceOptions, cfg.ToolSurface, 0),
		optCapabilitySurface: choiceIndex(
			CapabilitySurfaceOptions,
			cfg.CapabilitySurface,
			0,
		),
		optMetaParamSchema:   choiceIndex(MetaParamSchemaOptions, cfg.MetaParamSchema, 0),
		optEnterprise:        cfg.Enterprise,
		optReadOnly:          cfg.ReadOnly,
		optSafeMode:          cfg.SafeMode,
		optEmbeddedResources: cfg.EmbeddedResources,
		optExcludeTools:      cfg.ExcludeTools,
		optIgnoreScopes:      cfg.IgnoreScopes,
		optUploadMaxFileSize: cfg.UploadMaxFileSize,
		optAutoUpd:           cfg.AutoUpdate,
		optAutoUpdateMode:    choiceIndex(AutoUpdateModeOptions, cfg.AutoUpdateMode, 0),
		optAutoUpdateRepo:    cfg.AutoUpdateRepo,
		optAutoUpdateTimeout: cfg.AutoUpdateTimeout,
		optRateLimitRPS:      cfg.RateLimitRPS,
		optRateLimitBurst:    cfg.RateLimitBurst,
		optYolo:              cfg.YoloMode,
		optLogLevel:          choiceIndex(LogLevelOptions, cfg.LogLevel, 1),
		clients:              clients,
		clientSel:            sel,
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
		case "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "esc":
			if m.step == tuiStepOptions && m.optEditing {
				m.optEditing = false
				m.optEditInput.Blur()
				return m, nil
			}
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
			return m.handleGitLabEnter()
		case "ctrl+t":
			_ = openBrowserFn(TokenCreationURL(m.urlInput.Value()))
			return m, nil
		case "ctrl+o":
			return m.handleGitLabAdvanced()
		case "shift+tab":
			return m.handleGitLabShiftTab()
		case "tab":
			return m.handleGitLabTab()
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

func (m tuiModel) handleGitLabEnter() (tea.Model, tea.Cmd) {
	if m.gitlabFocus == 0 {
		return m.focusGitLabToken()
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
	m.step = tuiStepClients
	m.tokenInput.Blur()
	return m, nil
}

func (m tuiModel) handleGitLabAdvanced() (tea.Model, tea.Cmd) {
	if m.gitlabFocus != 1 {
		return m, nil
	}
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

func (m tuiModel) handleGitLabShiftTab() (tea.Model, tea.Cmd) {
	if m.gitlabFocus != 1 {
		return m, nil
	}
	m.gitlabFocus = 0
	m.tokenInput.Blur()
	m.urlInput.Focus()
	return m, textinput.Blink
}

func (m tuiModel) handleGitLabTab() (tea.Model, tea.Cmd) {
	if m.gitlabFocus != 0 {
		return m, nil
	}
	return m.focusGitLabToken()
}

func (m tuiModel) focusGitLabToken() (tea.Model, tea.Cmd) {
	m.urlInput.SetValue(effectiveGitLabURL(m.urlInput.Value()))
	m.gitlabFocus = 1
	m.urlInput.Blur()
	m.tokenInput.Focus()
	return m, textinput.Blink
}

// updateOptions handles keyboard navigation and toggles for advanced wizard
// settings such as TLS verification, meta-tools, auto-update, and log level.
func (m tuiModel) updateOptions(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.optEditing {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "enter":
				m.setCurrentTextOption(m.optEditInput.Value())
				m.optEditing = false
				m.optEditInput.Blur()
				return m, nil
			case "esc":
				m.optEditing = false
				m.optEditInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.optEditInput, cmd = m.optEditInput.Update(msg)
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "up", "k":
			if m.optCursor > 0 {
				m.optCursor--
			}
		case "down", "j":
			if m.optCursor < tuiOptionCount-1 {
				m.optCursor++
			}
		case "space", "x":
			if m.isCurrentTextOption() {
				m.optEditInput.SetValue(m.currentTextOption())
				m.optEditInput.Focus()
				m.optEditing = true
				return m, textinput.Blink
			}
			m.toggleOrCycleCurrentOption()
		case "enter":
			m.step = tuiStepClients
			return m, nil
		}
	}
	return m, nil
}

func (m *tuiModel) toggleOrCycleCurrentOption() {
	switch m.optCursor {
	case tuiOptSkipTLS:
		m.optSkipTLS = !m.optSkipTLS
	case tuiOptToolSurface:
		m.optToolSurface = (m.optToolSurface + 1) % len(ToolSurfaceOptions)
		m.optMeta = ToolSurfaceOptions[m.optToolSurface] != "individual"
	case tuiOptCapabilitySurface:
		m.optCapabilitySurface = (m.optCapabilitySurface + 1) % len(CapabilitySurfaceOptions)
	case tuiOptMetaParamSchema:
		m.optMetaParamSchema = (m.optMetaParamSchema + 1) % len(MetaParamSchemaOptions)
	case tuiOptEnterprise:
		m.optEnterprise = !m.optEnterprise
	case tuiOptReadOnly:
		m.optReadOnly = !m.optReadOnly
	case tuiOptSafeMode:
		m.optSafeMode = !m.optSafeMode
	case tuiOptEmbeddedResources:
		m.optEmbeddedResources = !m.optEmbeddedResources
	case tuiOptIgnoreScopes:
		m.optIgnoreScopes = !m.optIgnoreScopes
	case tuiOptAutoUpdateMode:
		m.optAutoUpdateMode = (m.optAutoUpdateMode + 1) % len(AutoUpdateModeOptions)
		m.optAutoUpd = AutoUpdateModeOptions[m.optAutoUpdateMode] != "false"
	case tuiOptYolo:
		m.optYolo = !m.optYolo
	case tuiOptLogLevel:
		m.optLogLevel = (m.optLogLevel + 1) % len(LogLevelOptions)
	}
}

func (m tuiModel) isCurrentTextOption() bool {
	switch m.optCursor {
	case tuiOptExcludeTools, tuiOptUploadMaxFileSize, tuiOptAutoUpdateRepo,
		tuiOptAutoUpdateTimeout, tuiOptRateLimitRPS, tuiOptRateLimitBurst:
		return true
	default:
		return false
	}
}

func (m tuiModel) currentTextOption() string {
	switch m.optCursor {
	case tuiOptExcludeTools:
		return m.optExcludeTools
	case tuiOptUploadMaxFileSize:
		return m.optUploadMaxFileSize
	case tuiOptAutoUpdateRepo:
		return m.optAutoUpdateRepo
	case tuiOptAutoUpdateTimeout:
		return m.optAutoUpdateTimeout
	case tuiOptRateLimitRPS:
		return m.optRateLimitRPS
	case tuiOptRateLimitBurst:
		return m.optRateLimitBurst
	default:
		return ""
	}
}

func (m *tuiModel) setCurrentTextOption(value string) {
	value = strings.TrimSpace(value)
	switch m.optCursor {
	case tuiOptExcludeTools:
		m.optExcludeTools = value
	case tuiOptUploadMaxFileSize:
		m.optUploadMaxFileSize = firstNonEmpty(value, defaultUploadMaxFileSize)
	case tuiOptAutoUpdateRepo:
		m.optAutoUpdateRepo = firstNonEmpty(value, DefaultServerConfig().AutoUpdateRepo)
	case tuiOptAutoUpdateTimeout:
		m.optAutoUpdateTimeout = firstNonEmpty(value, defaultAutoUpdateTimeout)
	case tuiOptRateLimitRPS:
		m.optRateLimitRPS = firstNonEmpty(value, defaultRateLimitRPS)
	case tuiOptRateLimitBurst:
		m.optRateLimitBurst = firstNonEmpty(value, defaultRateLimitBurst)
	}
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
			BinaryPath:        binaryPath,
			GitLabURL:         m.urlInput.Value(),
			GitLabToken:       m.tokenInput.Value(),
			SkipTLSVerify:     m.optSkipTLS,
			MetaTools:         m.optMeta,
			ToolSurface:       ToolSurfaceOptions[m.optToolSurface],
			CapabilitySurface: CapabilitySurfaceOptions[m.optCapabilitySurface],
			MetaParamSchema:   MetaParamSchemaOptions[m.optMetaParamSchema],
			Enterprise:        m.optEnterprise,
			ReadOnly:          m.optReadOnly,
			SafeMode:          m.optSafeMode,
			EmbeddedResources: m.optEmbeddedResources,
			ExcludeTools:      m.optExcludeTools,
			IgnoreScopes:      m.optIgnoreScopes,
			UploadMaxFileSize: m.optUploadMaxFileSize,
			AutoUpdate:        m.optAutoUpd,
			AutoUpdateMode:    AutoUpdateModeOptions[m.optAutoUpdateMode],
			AutoUpdateRepo:    m.optAutoUpdateRepo,
			AutoUpdateTimeout: m.optAutoUpdateTimeout,
			RateLimitRPS:      m.optRateLimitRPS,
			RateLimitBurst:    m.optRateLimitBurst,
			YoloMode:          m.optYolo,
			LogLevel:          LogLevelOptions[m.optLogLevel],
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
		switch {
		case s.completed:
			icon = tuiProgressDone.Render("✓")
			label = tuiProgressDone.Render(s.name)
		case s.active:
			icon = tuiProgressActive.Render("●")
			label = tuiProgressActive.Render(s.name)
		default:
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
	content.WriteString(tuiSectionTitle.Render("Advanced Options") + "\n\n")

	rows := m.optionRows()
	for i, row := range rows {
		cursor := "  "
		if m.optCursor == i {
			cursor = tuiCursorStyle.Render("▸ ")
		}
		value := row.value
		if m.optEditing && m.optCursor == i {
			value = m.optEditInput.View()
		}
		fmt.Fprintf(&content, "%s%s: %s\n", cursor, tuiListItemStyle.Render(row.name), tuiAccentStyle.Render(value))
	}

	help := "↑↓ navigate · Space edit/cycle · Enter continue"
	if m.optEditing {
		help = "Enter save · Esc cancel"
	}
	if m.optCursor < len(rows) {
		content.WriteString("\n" + tuiHelpStyle.Render(rows[m.optCursor].description))
	}
	content.WriteString("\n" + tuiHelpStyle.Render(help))
	return tuiActivePanelStyle.Width(width).Render(content.String())
}

type tuiOptionRow struct {
	name        string
	value       string
	description string
}

func (m tuiModel) optionRows() []tuiOptionRow {
	return []tuiOptionRow{
		{"Skip TLS verification", boolLabel(m.optSkipTLS), "Allow GitLab instances with self-signed or private CA certificates."},
		{"Tool surface", ToolSurfaceOptions[m.optToolSurface], "Choose the stdio tool catalog exposed to MCP clients."},
		{"Capability surface", CapabilitySurfaceOptions[m.optCapabilitySurface], "Use minimal with dynamic mode to keep startup context small."},
		{"Meta parameter schema", MetaParamSchemaOptions[m.optMetaParamSchema], "Controls how much schema detail meta-tools advertise."},
		{"Enterprise/Premium catalog", boolLabel(m.optEnterprise), "Expose Enterprise/Premium tools when your GitLab instance supports them."},
		{"Read-only mode", boolLabel(m.optReadOnly), "Register only tools that do not mutate GitLab state."},
		{"Safe mode previews", boolLabel(m.optSafeMode), "Return previews for mutating calls instead of executing them."},
		{"Embedded resources", boolLabel(m.optEmbeddedResources), "Include canonical MCP resource links in get_* tool results."},
		{"Ignore PAT scopes", boolLabel(m.optIgnoreScopes), "Skip token scope detection and register tools without scope filtering."},
		{"Excluded tools", emptyLabel(m.optExcludeTools), "Comma-separated tool names to omit from registration."},
		{"Upload max file size", m.optUploadMaxFileSize, "Maximum file size accepted by upload and file tools."},
		{"Auto-update mode", AutoUpdateModeOptions[m.optAutoUpdateMode], "true applies background updates, check logs only, false disables."},
		{"Auto-update repository", m.optAutoUpdateRepo, "GitHub owner/repo used for release update checks."},
		{"Auto-update timeout", m.optAutoUpdateTimeout, "Maximum time spent on startup/background update checks."},
		{"Rate limit RPS", m.optRateLimitRPS, "Global stdio tools/call limit; 0 disables the limiter."},
		{"Rate limit burst", m.optRateLimitBurst, "Token-bucket burst size when rate limiting is enabled."},
		{"YOLO mode", boolLabel(m.optYolo), "Enable less restrictive local execution safeguards."},
		{"Log level", LogLevelOptions[m.optLogLevel], "Controls stderr logging verbosity."},
	}
}

func boolLabel(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func emptyLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(none)"
	}
	return value
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
