// paths_test.go contains unit tests for platform-specific path resolution
// functions.
package wizard

import (
	"runtime"
	"strings"
	"testing"
)

func TestDefaultInstallDir_NotEmpty(t *testing.T) {
	dir := DefaultInstallDir()
	if dir == "" {
		t.Fatal("DefaultInstallDir returned empty string")
	}
}

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
