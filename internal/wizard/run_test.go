package wizard

import (
	"runtime"
	"testing"
)

// TestRun_UnknownMode verifies that Run returns an error for an
// unrecognized UI mode string.
func TestRun_UnknownMode(t *testing.T) {
	err := Run("1.0.0", "invalid-mode", nil, nil)
	if err == nil {
		t.Fatal("expected error for unknown UI mode, got nil")
	}
}

// TestHasDisplay_NoDisplayVars_ReturnsFalse verifies that hasDisplay returns
// false on Linux when neither DISPLAY nor WAYLAND_DISPLAY is set.
func TestHasDisplay_NoDisplayVars_ReturnsFalse(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test only applicable on Linux")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	if hasDisplay() {
		t.Error("hasDisplay() = true on headless Linux, want false")
	}
}

// TestHasDisplay_WithDISPLAY_ReturnsTrue verifies that hasDisplay returns
// true when the DISPLAY environment variable is set.
func TestHasDisplay_WithDISPLAY_ReturnsTrue(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test only applicable on Linux")
	}

	t.Setenv("DISPLAY", ":0")

	if !hasDisplay() {
		t.Error("hasDisplay() = false with DISPLAY=:0, want true")
	}
}

// TestHasDisplay_WithWAYLAND_ReturnsTrue verifies that hasDisplay returns
// true when the WAYLAND_DISPLAY environment variable is set.
func TestHasDisplay_WithWAYLAND_ReturnsTrue(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("test only applicable on Linux")
	}

	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")

	if !hasDisplay() {
		t.Error("hasDisplay() = false with WAYLAND_DISPLAY=wayland-0, want true")
	}
}
