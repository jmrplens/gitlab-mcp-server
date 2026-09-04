package toolutil

import gl "gitlab.com/gitlab-org/api/client-go/v2"

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

// AccessLevelDescription maps GitLab access level integers to human-readable labels.
func AccessLevelDescription(level gl.AccessLevelValue) string {
	if name, ok := accessLevelNames[level]; ok {
		return name
	}
	return "Unknown"
}
