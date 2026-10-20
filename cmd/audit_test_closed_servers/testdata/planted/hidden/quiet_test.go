//go:build neverset

package hidden

import "testing"

// TestPlanted_HiddenWithoutHTTPTest_Clean starts no server, so leaving it out
// hides nothing the gate would judge.
func TestPlanted_HiddenWithoutHTTPTest_Clean(t *testing.T) {}
