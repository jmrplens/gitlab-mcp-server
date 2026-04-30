// accesslevel.go maps GitLab access level constants to human-readable labels
// for use in Markdown formatters and tool output across all domain sub-packages.
package toolutil

import gl "gitlab.com/gitlab-org/api/client-go/v2"

// accessLevelNames maps GitLab access level values to human-readable labels.
var accessLevelNames = map[gl.AccessLevelValue]string{
	gl.NoPermissions:            "No access",
	gl.MinimalAccessPermissions: "Minimal access",
	gl.GuestPermissions:         "Guest",
	gl.ReporterPermissions:      "Reporter",
	gl.DeveloperPermissions:     "Developer",
	gl.MaintainerPermissions:    "Maintainer",
	gl.OwnerPermissions:         "Owner",
}

// AccessLevelDescription maps GitLab access level integers to human-readable labels.
func AccessLevelDescription(level gl.AccessLevelValue) string {
	if name, ok := accessLevelNames[level]; ok {
		return name
	}
	return "Unknown"
}
