package securityscanprofiles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// gidPrefix is the leading token of a GitLab GraphQL global ID. Callers may
// pass a security scan profile identifier (numeric database ID or a built-in
// scan type such as "dependency_scanning", "sast", "secret_detection", or
// "container_scanning") or a fully formed global ID; the former is wrapped with
// [gl.SecurityScanProfileGID].
const gidPrefix = "gid://"

// AttachInput holds parameters for attaching a security scan profile to
// projects and/or groups.
type AttachInput struct {
	SecurityScanProfileID string  `json:"security_scan_profile_id" jsonschema:"Security scan profile identifier: a built-in scan type (dependency_scanning, sast, secret_detection, or container_scanning — attach creates the default profile on the fly) or the persisted profile's numeric database ID (required by detach). A full gid:// global ID is also accepted,required"`
	ProjectIDs            []int64 `json:"project_ids,omitempty" jsonschema:"Numeric IDs of the projects to attach the profile to"`
	GroupIDs              []int64 `json:"group_ids,omitempty" jsonschema:"Numeric IDs of the groups to attach the profile to"`
}

// DetachInput holds parameters for detaching a security scan profile from
// projects and/or groups.
type DetachInput struct {
	SecurityScanProfileID string  `json:"security_scan_profile_id" jsonschema:"Persisted scan profile identifier: the profile's numeric database ID (obtained from gitlab_list_project_scan_profile_statuses) or a full gid:// global ID. A scan-type name (dependency_scanning, sast, …) is not accepted by detach,required"`
	ProjectIDs            []int64 `json:"project_ids,omitempty" jsonschema:"Numeric IDs of the projects to detach the profile from"`
	GroupIDs              []int64 `json:"group_ids,omitempty" jsonschema:"Numeric IDs of the groups to detach the profile from"`
}

// ListProjectStatusesInput holds parameters for listing the scan profile
// statuses of a project.
type ListProjectStatusesInput struct {
	ProjectFullPath string `json:"project_full_path" jsonschema:"Full project path (namespace/project). Numeric project IDs are not accepted by the GraphQL project(fullPath:) field,required"`
}

// MutationOutput confirms a security scan profile attach or detach operation
// and echoes the resolved targets.
type MutationOutput struct {
	toolutil.HintableOutput
	Status                string  `json:"status"`
	Message               string  `json:"message"`
	SecurityScanProfileID string  `json:"security_scan_profile_id"`
	ProjectIDs            []int64 `json:"project_ids,omitempty"`
	GroupIDs              []int64 `json:"group_ids,omitempty"`
}

// ScanProfile represents a security scan profile.
type ScanProfile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ScanType string `json:"scan_type"`
}

// ScanProfileStatus represents the status of a scan profile for a project.
type ScanProfileStatus struct {
	Status      string      `json:"status"`
	ScanProfile ScanProfile `json:"scan_profile"`
}

// ListProjectStatusesOutput contains the scan profile statuses for a project.
type ListProjectStatusesOutput struct {
	toolutil.HintableOutput
	ProjectFullPath string              `json:"project_full_path"`
	Statuses        []ScanProfileStatus `json:"statuses"`
}

// resolveScanProfileGID normalizes a caller-supplied identifier to a GitLab
// GraphQL global ID. A value that is already a global ID is returned verbatim;
// any other value is wrapped with [gl.SecurityScanProfileGID].
func resolveScanProfileGID(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if strings.HasPrefix(identifier, gidPrefix) {
		return identifier
	}
	return gl.SecurityScanProfileGID(identifier)
}

// validateTargets ensures a scan profile identifier and at least one target
// project or group were supplied.
func validateTargets(profileID string, projectIDs, groupIDs []int64) error {
	if strings.TrimSpace(profileID) == "" {
		return toolutil.ErrFieldRequired("security_scan_profile_id")
	}
	if len(projectIDs) == 0 && len(groupIDs) == 0 {
		return errors.New("provide at least one of project_ids or group_ids")
	}
	return nil
}

// validateDetachIdentifier enforces the detach contract: unlike attach, which
// find-or-creates a profile from a scan-type name, detach operates on an
// already-persisted profile identified by its numeric database ID or a numeric
// gid:// global ID (as returned by list_project_statuses). Rejecting a
// scan-type name here turns an opaque GraphQL mutation failure into an
// actionable validation error before the request is dispatched.
func validateDetachIdentifier(identifier string) error {
	tail := strings.TrimSpace(identifier)
	if rest, ok := strings.CutPrefix(tail, gidPrefix); ok {
		if i := strings.LastIndex(rest, "/"); i >= 0 {
			tail = rest[i+1:]
		} else {
			tail = ""
		}
	}
	if !isAllDigits(tail) {
		return errors.New("detach requires the persisted profile's numeric ID " +
			"(from gitlab_list_project_scan_profile_statuses), not a scan-type name")
	}
	return nil
}

// isAllDigits reports whether s is non-empty and consists solely of ASCII
// digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Attach attaches a security scan profile to the given projects and/or groups.
func Attach(ctx context.Context, client *gitlabclient.Client, input AttachInput) (MutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutationOutput{}, err
	}
	if err := validateTargets(input.SecurityScanProfileID, input.ProjectIDs, input.GroupIDs); err != nil {
		return MutationOutput{}, err
	}

	opts := &gl.AttachSecurityScanProfileOptions{
		SecurityScanProfileID: resolveScanProfileGID(input.SecurityScanProfileID),
		ProjectIDs:            input.ProjectIDs,
		GroupIDs:              input.GroupIDs,
	}
	if _, err := client.GL().SecurityScanProfiles.AttachSecurityScanProfile(opts, gl.WithContext(ctx)); err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("attach security scan profile", err,
			"use a valid scan-type identifier (dependency_scanning, sast, secret_detection, or container_scanning); targets must belong to a group namespace (not a personal namespace) and share one root namespace; requires Maintainer or Owner on the targets and Ultimate")
	}
	return MutationOutput{
		Status:                "success",
		Message:               "Successfully attached security scan profile.",
		SecurityScanProfileID: input.SecurityScanProfileID,
		ProjectIDs:            input.ProjectIDs,
		GroupIDs:              input.GroupIDs,
	}, nil
}

// Detach detaches a security scan profile from the given projects and/or groups.
func Detach(ctx context.Context, client *gitlabclient.Client, input DetachInput) (MutationOutput, error) {
	if err := ctx.Err(); err != nil {
		return MutationOutput{}, err
	}
	if err := validateTargets(input.SecurityScanProfileID, input.ProjectIDs, input.GroupIDs); err != nil {
		return MutationOutput{}, err
	}
	if err := validateDetachIdentifier(input.SecurityScanProfileID); err != nil {
		return MutationOutput{}, err
	}

	opts := &gl.DetachSecurityScanProfileOptions{
		SecurityScanProfileID: resolveScanProfileGID(input.SecurityScanProfileID),
		ProjectIDs:            input.ProjectIDs,
		GroupIDs:              input.GroupIDs,
	}
	if _, err := client.GL().SecurityScanProfiles.DetachSecurityScanProfile(opts, gl.WithContext(ctx)); err != nil {
		return MutationOutput{}, toolutil.WrapErrWithHint("detach security scan profile", err,
			"detach requires the persisted profile's numeric ID (from gitlab_list_project_scan_profile_statuses), not a scan-type name; targets must belong to a group namespace and share one root namespace; requires Maintainer or Owner and Ultimate")
	}
	return MutationOutput{
		Status:                "success",
		Message:               "Successfully detached security scan profile.",
		SecurityScanProfileID: input.SecurityScanProfileID,
		ProjectIDs:            input.ProjectIDs,
		GroupIDs:              input.GroupIDs,
	}, nil
}

// ListProjectStatuses returns the scan profile statuses for a project.
func ListProjectStatuses(ctx context.Context, client *gitlabclient.Client, input ListProjectStatusesInput) (ListProjectStatusesOutput, error) {
	if err := ctx.Err(); err != nil {
		return ListProjectStatusesOutput{}, err
	}
	fullPath := strings.TrimSpace(input.ProjectFullPath)
	if fullPath == "" {
		return ListProjectStatusesOutput{}, toolutil.ErrFieldRequired("project_full_path")
	}

	statuses, _, err := client.GL().SecurityScanProfiles.ListProjectScanProfileStatuses(fullPath, gl.WithContext(ctx))
	if err != nil {
		return ListProjectStatusesOutput{}, toolutil.WrapErrWithHint("list project scan profile statuses", err,
			fmt.Sprintf("verify project_full_path %q is the full namespace/project path (not a numeric ID); requires the feature enabled (Ultimate)", fullPath))
	}

	out := ListProjectStatusesOutput{
		ProjectFullPath: fullPath,
		Statuses:        make([]ScanProfileStatus, 0, len(statuses)),
	}
	for _, status := range statuses {
		out.Statuses = append(out.Statuses, ScanProfileStatus{
			Status: status.Status,
			ScanProfile: ScanProfile{
				ID:       status.ScanProfile.ID,
				Name:     status.ScanProfile.Name,
				ScanType: status.ScanProfile.ScanType,
			},
		})
	}
	return out, nil
}
