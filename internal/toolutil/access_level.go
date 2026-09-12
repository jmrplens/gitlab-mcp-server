package toolutil

import (
	"fmt"

	gl "gitlab.com/gitlab-org/api/client-go/v3"
)

// accessLevelNames maps GitLab access level values to human-readable labels.
var accessLevelNames = map[gl.AccessLevelValue]string{
	gl.NoPermissions:              "No access",
	gl.MinimalAccessPermissions:   "Minimal access",
	gl.GuestPermissions:           "Guest",
	gl.PlannerPermissions:         "Planner",
	gl.ReporterPermissions:        "Reporter",
	gl.SecurityManagerPermissions: "Security Manager",
	gl.DeveloperPermissions:       "Developer",
	gl.MaintainerPermissions:      "Maintainer",
	gl.OwnerPermissions:           "Owner",
	gl.AdminPermissions:           "Admin",
}

// AccessLevelDescription maps a GitLab access level to its human-readable
// label. A level the table does not name is rendered with its number, "Level
// 35", because the number is what GitLab sent and the one thing a reader can
// act on; "Unknown" told them nothing, which is why two packages kept a copy
// of this table with the numeric fallback of their own.
func AccessLevelDescription(level gl.AccessLevelValue) string {
	if name, ok := accessLevelNames[level]; ok {
		return name
	}
	return fmt.Sprintf("Level %d", level)
}
