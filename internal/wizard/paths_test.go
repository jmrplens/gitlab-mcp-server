// paths_test.go contains unit tests for platform-specific path resolution
// functions.
package wizard

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDefaultInstallDir_NotEmpty verifies DefaultInstallDir returns a
// non-empty platform-specific install directory.
func TestDefaultInstallDir_NotEmpty(t *testing.T) {
	dir := DefaultInstallDir()
	if dir == "" {
		t.Fatal("DefaultInstallDir returned empty string")
	}
}

// TestDefaultInstallDir_LinuxUsesHomeLocalBin verifies the Linux install
// default follows the documented ~/.local/bin convention.
func TestDefaultInstallDir_LinuxUsesHomeLocalBin(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := DefaultInstallDir()
	if !strings.HasSuffix(dir, ".local/bin") || !strings.HasPrefix(dir, home) {
		t.Fatalf("DefaultInstallDir() = %q, want under temp HOME .local/bin", dir)
	}
}

// TestDefaultBinaryName_Platform verifies DefaultBinaryName returns
// "gitlab-mcp-server.exe" on Windows and "gitlab-mcp-server" elsewhere.
func TestDefaultBinaryName_Platform(t *testing.T) {
	name := DefaultBinaryName()
	if runtime.GOOS == "windows" {
		if name != "gitlab-mcp-server.exe" {
			t.Errorf("got %q, want %q", name, "gitlab-mcp-server.exe")
		}
	} else {
		if name != "gitlab-mcp-server" {
			t.Errorf("got %q, want %q", name, "gitlab-mcp-server")
		}
	}
}

// TestExpandPath_Tilde verifies ExpandPath resolves a leading "~/" to
// the user's home directory and returns a non-empty expanded path.
func TestExpandPath_Tilde(t *testing.T) {
	expanded, err := ExpandPath("~/test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded == "~/test" {
		t.Error("tilde was not expanded")
	}
	if expanded == "" {
		t.Error("expanded path is empty")
	}
}

// TestExpandPath_AbsolutePassthrough verifies ExpandPath returns absolute
// paths unchanged on both Windows and Unix-like systems.
func TestExpandPath_AbsolutePassthrough(t *testing.T) {
	var path string
	if runtime.GOOS == "windows" {
		path = `C:\Users\test`
	} else {
		path = "/usr/local/bin"
	}

	expanded, err := ExpandPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if expanded != path {
		t.Errorf("got %q, want %q", expanded, path)
	}
}

// TestConfigDir_LinuxXDG verifies configDir uses XDG_CONFIG_HOME on Linux
// when the variable is set.
func TestConfigDir_LinuxXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg-test")
	dir := configDir("myapp")
	if dir != "/tmp/xdg-test/myapp" {
		t.Errorf("configDir = %q, want /tmp/xdg-test/myapp", dir)
	}
}

// TestConfigDir_LinuxDefault verifies configDir falls back to ~/.config on
// Linux when XDG_CONFIG_HOME is not set.
func TestConfigDir_LinuxDefault(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	dir := configDir("myapp")
	if dir == "" {
		t.Fatal("configDir returned empty string")
	}
	if !strings.Contains(dir, ".config/myapp") {
		t.Errorf("configDir = %q, want to contain .config/myapp", dir)
	}
}

// TestEnvFilePath_NotEmpty verifies EnvFilePath returns a non-empty path.
func TestEnvFilePath_NotEmpty(t *testing.T) {
	p := EnvFilePath()
	if p == "" {
		t.Fatal("EnvFilePath returned empty string")
	}
	if !strings.HasSuffix(p, EnvFileName) {
		t.Errorf("EnvFilePath = %q, want suffix %q", p, EnvFileName)
	}
}

// TestZedConfigPath_Linux verifies zedConfigPath returns a .config/zed path
// on Linux with XDG_CONFIG_HOME unset.
func TestZedConfigPath_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	p := zedConfigPath()
	if !strings.Contains(p, ".config/zed/settings.json") {
		t.Errorf("zedConfigPath = %q, want to contain .config/zed/settings.json", p)
	}
}

// TestZedConfigPath_LinuxXDG verifies zedConfigPath hooks into XDG_CONFIG_HOME.
func TestZedConfigPath_LinuxXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom-config")
	p := zedConfigPath()
	if p != "/tmp/custom-config/zed/settings.json" {
		t.Errorf("zedConfigPath = %q, want /tmp/custom-config/zed/settings.json", p)
	}
}

// TestCrushConfigPath_Linux verifies crushConfigPath uses configDir on Linux.
func TestCrushConfigPath_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	p := crushConfigPath()
	if !strings.Contains(p, ".config/crush/crush.json") {
		t.Errorf("crushConfigPath = %q, want to contain .config/crush/crush.json", p)
	}
}

// TestCrushConfigPath_LinuxXDG verifies crushConfigPath honors XDG_CONFIG_HOME
// through configDir on Linux.
func TestCrushConfigPath_LinuxXDG(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only test")
	}
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom-config")
	p := crushConfigPath()
	if p != "/tmp/custom-config/crush/crush.json" {
		t.Errorf("crushConfigPath = %q, want /tmp/custom-config/crush/crush.json", p)
	}
}

// TestAllConfigPaths_NonEmpty ensures all config path functions return
// non-empty strings on the current platform.
func TestAllConfigPaths_NonEmpty(t *testing.T) {
	fns := map[string]func() string{
		"vsCodeConfigPath":        vsCodeConfigPath,
		"claudeDesktopConfigPath": claudeDesktopConfigPath,
		"claudeCodeConfigPath":    claudeCodeConfigPath,
		"cursorConfigPath":        cursorConfigPath,
		"windsurfConfigPath":      windsurfConfigPath,
		"copilotCLIConfigPath":    copilotCLIConfigPath,
		"openCodeConfigPath":      openCodeConfigPath,
		"crushConfigPath":         crushConfigPath,
		"zedConfigPath":           zedConfigPath,
	}
	for name, fn := range fns {
		t.Run(name, func(t *testing.T) {
			p := fn()
			if p == "" {
				t.Errorf("%s returned empty string", name)
			}
		})
	}
}

// TestDefaultInstallDir_CurrentPlatform exercises the platform-specific
// branch on the current OS so the function is covered even when running
// on a non-Linux/Windows machine.
func TestDefaultInstallDir_CurrentPlatform(t *testing.T) {
	dir := DefaultInstallDir()
	switch runtime.GOOS {
	case "windows":
		// Either LOCALAPPDATA-derived or fallback to AppData/Local.
		home, _ := os.UserHomeDir()
		if dir == "" {
			t.Fatal("DefaultInstallDir returned empty on Windows")
		}
		if !strings.Contains(dir, "gitlab-mcp-server") {
			t.Errorf("DefaultInstallDir = %q, want to contain app name", dir)
		}
		_ = home
	default:
		if !strings.HasSuffix(dir, ".local/bin") {
			t.Errorf("DefaultInstallDir = %q, want suffix .local/bin on %s", dir, runtime.GOOS)
		}
	}
}

// TestConfigDir_CurrentPlatform exercises configDir on the current OS
// to cover the platform-specific branch.
func TestConfigDir_CurrentPlatform(t *testing.T) {
	dir := configDir("testapp")
	if dir == "" {
		t.Fatal("configDir returned empty")
	}
	if !strings.HasSuffix(dir, "testapp") {
		t.Errorf("configDir = %q, want suffix testapp", dir)
	}
	switch runtime.GOOS {
	case "darwin":
		// macOS uses Library/Application Support/<app>
		if !strings.Contains(dir, "Library/Application Support/testapp") {
			t.Errorf("configDir = %q, want to contain 'Library/Application Support/testapp' on darwin", dir)
		}
	case "windows":
		// Either APPDATA-derived or AppData/Roaming.
		if !strings.Contains(dir, "testapp") {
			t.Errorf("configDir = %q, want to contain app name on windows", dir)
		}
	default:
		// Linux/BSD: XDG_CONFIG_HOME if set, else ~/.config.
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg != "" {
			if dir != filepath.Join(xdg, "testapp") {
				t.Errorf("configDir = %q, want %q when XDG_CONFIG_HOME is set", dir, filepath.Join(xdg, "testapp"))
			}
		} else if !strings.Contains(dir, ".config/testapp") {
			t.Errorf("configDir = %q, want to contain .config/testapp on %s", dir, runtime.GOOS)
		}
	}
}

// TestZedConfigPath_CurrentPlatform exercises the zedConfigPath branch
// for the current OS.
func TestZedConfigPath_CurrentPlatform(t *testing.T) {
	p := zedConfigPath()
	if p == "" {
		t.Fatal("zedConfigPath returned empty")
	}
	if !strings.HasSuffix(p, "settings.json") {
		t.Errorf("zedConfigPath = %q, want suffix settings.json", p)
	}
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(p, ".config/zed/settings.json") {
			t.Errorf("zedConfigPath = %q, want to contain .config/zed/settings.json on darwin", p)
		}
	case "windows":
		if !strings.Contains(p, "Zed") {
			t.Errorf("zedConfigPath = %q, want to contain Zed on windows", p)
		}
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		if xdg != "" {
			if p != filepath.Join(xdg, "zed", "settings.json") {
				t.Errorf("zedConfigPath = %q, want %q", p, filepath.Join(xdg, "zed", "settings.json"))
			}
		} else if !strings.Contains(p, ".config/zed/settings.json") {
			t.Errorf("zedConfigPath = %q, want to contain .config/zed/settings.json on %s", p, runtime.GOOS)
		}
	}
}

// TestCrushConfigPath_CurrentPlatform exercises crushConfigPath on the
// current OS to cover platform branches.
func TestCrushConfigPath_CurrentPlatform(t *testing.T) {
	p := crushConfigPath()
	if p == "" {
		t.Fatal("crushConfigPath returned empty")
	}
	if !strings.HasSuffix(p, "crush.json") {
		t.Errorf("crushConfigPath = %q, want suffix crush.json", p)
	}
	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(p, "crush") {
			t.Errorf("crushConfigPath = %q, want to contain crush on windows", p)
		}
	case "darwin":
		// On macOS, configDir returns ~/Library/Application Support/...
		if !strings.Contains(p, "Library/Application Support/crush/crush.json") {
			t.Errorf("crushConfigPath = %q, want to contain Library/Application Support/crush/crush.json on darwin", p)
		}
	default:
		xdg := os.Getenv("XDG_CONFIG_HOME")
		var want string
		if xdg != "" {
			want = filepath.Join(xdg, "crush", "crush.json")
		} else {
			home, _ := os.UserHomeDir()
			want = filepath.Join(home, ".config", "crush", "crush.json")
		}
		if p != want {
			t.Errorf("crushConfigPath = %q, want %q", p, want)
		}
	}
}

// TestZedConfigPath_WindowsAPPDATA verifies zedConfigPath uses APPDATA on
// Windows when the env variable is set.
func TestZedConfigPath_WindowsAPPDATA(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	t.Setenv("APPDATA", `C:\Users\test\AppData\Roaming`)
	p := zedConfigPath()
	want := `C:\Users\test\AppData\Roaming\Zed\settings.json`
	if p != want {
		t.Errorf("zedConfigPath = %q, want %q", p, want)
	}
}

// TestZedConfigPath_WindowsFallback verifies zedConfigPath falls back to
// AppData/Roaming when APPDATA is unset.
func TestZedConfigPath_WindowsFallback(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	t.Setenv("APPDATA", "")
	home, _ := os.UserHomeDir()
	p := zedConfigPath()
	want := filepath.Join(home, "AppData", "Roaming", "Zed", "settings.json")
	if p != want {
		t.Errorf("zedConfigPath = %q, want %q", p, want)
	}
}

// fakeGetenv returns a getenv function backed by the given map, so
// platform-parameterized path helpers can be tested with deterministic
// environment values on any host.
func fakeGetenv(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// unsetHomeEnv clears the environment variables os.UserHomeDir consults on
// every platform (HOME on Unix, USERPROFILE on Windows, home on Plan 9) so
// home-directory resolution fails deterministically in error-branch tests.
func unsetHomeEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("home", "")
}

// TestDefaultInstallDirFor_PlatformBranches verifies the platform-specific
// install directory selection for every GOOS branch: Windows prefers
// %LOCALAPPDATA%, falls back to the home AppData path, and all other
// platforms use ~/.local/bin. The extracted helper makes each branch
// testable regardless of the host OS.
func TestDefaultInstallDirFor_PlatformBranches(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		home string
		want string
	}{
		{
			name: "windows with LOCALAPPDATA",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": filepath.Join("C:", "Users", "u", "AppData", "Local")},
			home: filepath.Join("C:", "Users", "u"),
			want: filepath.Join("C:", "Users", "u", "AppData", "Local", appName),
		},
		{
			name: "windows without LOCALAPPDATA falls back to home",
			goos: "windows",
			env:  map[string]string{},
			home: filepath.Join("C:", "Users", "u"),
			want: filepath.Join("C:", "Users", "u", "AppData", "Local", appName),
		},
		{
			name: "linux uses ~/.local/bin",
			goos: "linux",
			env:  map[string]string{},
			home: filepath.Join("/home", "u"),
			want: filepath.Join("/home", "u", ".local", "bin"),
		},
		{
			name: "darwin uses ~/.local/bin",
			goos: "darwin",
			env:  map[string]string{},
			home: filepath.Join("/Users", "u"),
			want: filepath.Join("/Users", "u", ".local", "bin"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultInstallDirFor(tt.goos, fakeGetenv(tt.env), tt.home)
			if got != tt.want {
				t.Errorf("defaultInstallDirFor(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

// TestDefaultBinaryNameFor_PlatformBranches verifies the binary name gets a
// .exe suffix only on Windows. The extracted helper makes the Windows branch
// testable on non-Windows hosts.
func TestDefaultBinaryNameFor_PlatformBranches(t *testing.T) {
	if got := defaultBinaryNameFor("windows"); got != appName+".exe" {
		t.Errorf("defaultBinaryNameFor(windows) = %q, want %q", got, appName+".exe")
	}
	if got := defaultBinaryNameFor("linux"); got != appName {
		t.Errorf("defaultBinaryNameFor(linux) = %q, want %q", got, appName)
	}
	if got := defaultBinaryNameFor("darwin"); got != appName {
		t.Errorf("defaultBinaryNameFor(darwin) = %q, want %q", got, appName)
	}
}

// TestExpandPath_HomeLookupError verifies ExpandPath surfaces the
// os.UserHomeDir error when a ~-prefixed path is expanded while no home
// directory can be resolved from the environment.
func TestExpandPath_HomeLookupError(t *testing.T) {
	unsetHomeEnv(t)
	if _, err := ExpandPath("~" + string(os.PathSeparator) + "sub"); err == nil {
		t.Fatal("ExpandPath(~/sub) error = nil, want home lookup error")
	}
}

// TestExpandPathWithHome_ErrorPropagates verifies the extracted helper
// returns the home-resolution error verbatim and an empty path, matching the
// original ExpandPath contract.
func TestExpandPathWithHome_ErrorPropagates(t *testing.T) {
	wantErr := os.ErrPermission
	got, err := expandPathWithHome("~/x", "", wantErr)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expandPathWithHome error = %v, want %v", err, wantErr)
	}
	if got != "" {
		t.Fatalf("expandPathWithHome path = %q, want empty on error", got)
	}
}

// TestConfigDirFor_PlatformBranches verifies the per-platform user config
// directory resolution: Windows prefers %APPDATA% with a home fallback,
// macOS uses Library/Application Support, and Linux honors XDG_CONFIG_HOME
// with a ~/.config fallback. The extracted helper makes every branch
// testable on any host.
func TestConfigDirFor_PlatformBranches(t *testing.T) {
	home := filepath.Join("/home", "u")
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "windows with APPDATA",
			goos: "windows",
			env:  map[string]string{"APPDATA": filepath.Join("C:", "roaming")},
			want: filepath.Join("C:", "roaming", "App"),
		},
		{
			name: "windows without APPDATA falls back to home",
			goos: "windows",
			env:  map[string]string{},
			want: filepath.Join(home, "AppData", "Roaming", "App"),
		},
		{
			name: "darwin uses Application Support",
			goos: "darwin",
			env:  map[string]string{},
			want: filepath.Join(home, "Library", "Application Support", "App"),
		},
		{
			name: "linux with XDG_CONFIG_HOME",
			goos: "linux",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg"},
			want: filepath.Join("/xdg", "App"),
		},
		{
			name: "linux without XDG_CONFIG_HOME falls back to ~/.config",
			goos: "linux",
			env:  map[string]string{},
			want: filepath.Join(home, configDirXDG, "App"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := configDirFor(tt.goos, fakeGetenv(tt.env), home, "App")
			if got != tt.want {
				t.Errorf("configDirFor(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

// TestCrushConfigPathFor_PlatformBranches verifies the Crush config path
// resolution: Windows prefers %LOCALAPPDATA% with a home fallback, all
// other platforms use the shared config directory. The extracted helper
// makes the Windows branches testable on any host.
func TestCrushConfigPathFor_PlatformBranches(t *testing.T) {
	home := filepath.Join("/home", "u")
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "windows with LOCALAPPDATA",
			goos: "windows",
			env:  map[string]string{"LOCALAPPDATA": filepath.Join("C:", "local")},
			want: filepath.Join("C:", "local", "crush", crushFile),
		},
		{
			name: "windows without LOCALAPPDATA falls back to home",
			goos: "windows",
			env:  map[string]string{},
			want: filepath.Join(home, "AppData", "Local", "crush", crushFile),
		},
		{
			name: "linux delegates to config dir",
			goos: "linux",
			env:  map[string]string{},
			want: filepath.Join(home, configDirXDG, "crush", crushFile),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := crushConfigPathFor(tt.goos, fakeGetenv(tt.env), home)
			if got != tt.want {
				t.Errorf("crushConfigPathFor(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}

// TestZedConfigPathFor_PlatformBranches verifies the Zed settings path
// resolution: macOS uses ~/.config/zed, Windows prefers %APPDATA% with a
// home fallback, and Linux honors XDG_CONFIG_HOME with a ~/.config
// fallback. The extracted helper makes every branch testable on any host.
func TestZedConfigPathFor_PlatformBranches(t *testing.T) {
	home := filepath.Join("/home", "u")
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "darwin uses ~/.config/zed",
			goos: "darwin",
			env:  map[string]string{},
			want: filepath.Join(home, configDirXDG, "zed", settingsFile),
		},
		{
			name: "windows with APPDATA",
			goos: "windows",
			env:  map[string]string{"APPDATA": filepath.Join("C:", "roaming")},
			want: filepath.Join("C:", "roaming", "Zed", settingsFile),
		},
		{
			name: "windows without APPDATA falls back to home",
			goos: "windows",
			env:  map[string]string{},
			want: filepath.Join(home, "AppData", "Roaming", "Zed", settingsFile),
		},
		{
			name: "linux with XDG_CONFIG_HOME",
			goos: "linux",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg"},
			want: filepath.Join("/xdg", "zed", settingsFile),
		},
		{
			name: "linux without XDG_CONFIG_HOME falls back to ~/.config",
			goos: "linux",
			env:  map[string]string{},
			want: filepath.Join(home, configDirXDG, "zed", settingsFile),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zedConfigPathFor(tt.goos, fakeGetenv(tt.env), home)
			if got != tt.want {
				t.Errorf("zedConfigPathFor(%q) = %q, want %q", tt.goos, got, tt.want)
			}
		})
	}
}
