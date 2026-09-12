// access_level_test.go verifies the human-readable label mapping for GitLab
// access level constants used in project/group member outputs.
package toolutil

import (
	"testing"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// TestAccessLevelDescription verifies human-readable labels for every known
// GitLab access level, plus the fallback for a value the table does not name,
// which keeps the number: "Unknown" told the reader nothing about what GitLab
// sent, and a level this server has no word for is still a level.
func TestAccessLevelDescription(t *testing.T) {
	tests := []struct {
		name  string
		level gl.AccessLevelValue
		want  string
	}{
		{"no access", gl.NoPermissions, "No access"},
		{"guest", gl.GuestPermissions, "Guest"},
		{"planner", gl.PlannerPermissions, "Planner"},
		{"reporter", gl.ReporterPermissions, "Reporter"},
		{"security manager", gl.SecurityManagerPermissions, "Security Manager"},
		{"developer", gl.DeveloperPermissions, "Developer"},
		{"maintainer", gl.MaintainerPermissions, "Maintainer"},
		{"owner", gl.OwnerPermissions, "Owner"},
		{"admin", gl.AdminPermissions, "Admin"},
		{"minimal", gl.MinimalAccessPermissions, "Minimal access"},
		{"unknown value keeps the number", gl.AccessLevelValue(99), "Level 99"},
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
