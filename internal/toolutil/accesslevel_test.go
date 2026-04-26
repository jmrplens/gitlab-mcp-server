// accesslevel_test.go verifies the human-readable label mapping for GitLab
// access level constants used in project/group member outputs.

package toolutil

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// TestAccessLevelDescription verifies human-readable labels for every known
// GitLab access level, plus the fallback for unknown values.
func TestAccessLevelDescription(t *testing.T) {
	tests := []struct {
		name  string
		level gl.AccessLevelValue
		want  string
	}{
		{"no access", gl.NoPermissions, "No access"},
		{"guest", gl.GuestPermissions, "Guest"},
		{"reporter", gl.ReporterPermissions, "Reporter"},
		{"developer", gl.DeveloperPermissions, "Developer"},
		{"maintainer", gl.MaintainerPermissions, "Maintainer"},
		{"owner", gl.OwnerPermissions, "Owner"},
		{"minimal", gl.MinimalAccessPermissions, "Minimal access"},
		{"unknown value", gl.AccessLevelValue(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AccessLevelDescription(tt.level)
			if got != tt.want {
				t.Errorf("AccessLevelDescription(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}
