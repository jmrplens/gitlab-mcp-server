package wizard

import (
	"runtime"
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
