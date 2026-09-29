package securityscanprofiles

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// gidPrefix is the leading token of a GitLab GraphQL global ID. Callers may
// pass a security scan profile identifier (a numeric database ID or one of
// [DefaultProfileNames]) or a fully formed global ID; the former is wrapped
// with [gl.SecurityScanProfileGID].
const gidPrefix = "gid://"

// DefaultProfileNames lists the names attach takes in place of a persisted
// profile's ID, written as the sentence fragment every text that offers them
// repeats: the input's description, the attach usage line, the attach error
// hint and the meta-tool group description in internal/tools.
//
// They are the preset keys of the default profiles GitLab defines
// (Security::DefaultScanProfilesHelper at 19.5.0-pre, 5041f73d695): a key is
// the profile's scan type, except for the three Triage and Remediation
// presets, which GitLab 19.4 added behind the triage_and_remediation_profile
// flag, on by default. For such a name attach finds or creates the
// namespace's default profile. They are not the SecurityScanProfileType enum:
// container_scanning and business_logic are scan types GitLab defines no
// default profile for, and FindOrCreateService answers either by name with
// "Could not find a default scan profile for this type", which the mutation
// turns into a resource-not-available error (see [RefusedByName]). The
// package's tests hold this list to that enum, so a scan type GitLab adds
// fails them until someone decides which side of the line it falls on.
//
// It is one literal rather than two joined with a plus, for the reason
// [errDetachIdentifier] gives: a plus at package level is a mutant no test
// can reach.
const DefaultProfileNames = "secret_detection, sast, dependency_scanning, dependency_scanning_post_processing, triage_and_remediation_conservative, triage_and_remediation_standard, or triage_and_remediation_proactive"

// DefaultProfileFloors says from which GitLab release each name of
// [DefaultProfileNames] exists, in the words every text offering the names
// adds after them, so a caller of an older instance is not handed a name its
// GitLab refuses. secret_detection is as old as scan profiles themselves
// (18.7), which is the floor of the whole domain and needs no word of its
// own. Read from Security::DefaultScanProfilesHelper at every stable branch
// from 18-7 to 19-4: sast arrives at 18.10 and dependency_scanning at 18.11,
// both behind a feature flag until 19.0, dependency_scanning_post_processing
// at 19.2, and the Triage and Remediation presets at 19.4.
const DefaultProfileFloors = "sast needs GitLab 18.10 and dependency_scanning needs 18.11, both behind a feature flag until 19.0, dependency_scanning_post_processing needs 19.2, and the triage_and_remediation presets need 19.4"

// RefusedByName names the SecurityScanProfileType values attach refuses by
// name, in the sentence every text offering [DefaultProfileNames] adds. Two
// are scan types GitLab builds no default profile for; the third is the scan
// type whose default profiles are keyed by preset, so the bare type is the
// key of none of them. FindOrCreateService answers each with "Could not find
// a default scan profile for this type", which the attach mutation replaces
// with a generic resource-not-available error naming none of this, and that
// is why the handler's hint says it (upstream register, "the scan profile
// attach mutation drops the reason it refused a name").
const RefusedByName = "container_scanning and business_logic have no default profile, and the bare triage_and_remediation names none of its presets, so attach refuses all three by name"

// gidAuthority is the authority every global ID GitLab issues carries, as in
// gid://gitlab/Security::ScanProfile/90. It is what distinguishes a global ID
// from a string that merely starts like one.
const gidAuthority = "gitlab"

// AttachInput holds parameters for attaching a security scan profile to
// projects and/or groups.
type AttachInput struct {
	SecurityScanProfileID string  `json:"security_scan_profile_id" jsonschema:"Security scan profile identifier: the name of a GitLab default profile (secret_detection, sast, dependency_scanning, dependency_scanning_post_processing, triage_and_remediation_conservative, triage_and_remediation_standard, or triage_and_remediation_proactive), for which attach creates the namespace's default profile on the fly, or the persisted profile's numeric database ID (required by detach). A full gid:// global ID is also accepted. sast needs GitLab 18.10 and dependency_scanning needs 18.11, both behind a feature flag until 19.0, dependency_scanning_post_processing needs 19.2, and the triage_and_remediation presets need 19.4. container_scanning and business_logic have no default profile, and the bare triage_and_remediation names none of its presets, so attach refuses all three by name,required"`
	ProjectIDs            []int64 `json:"project_ids,omitempty" jsonschema:"Numeric IDs of the projects to attach the profile to"`
	GroupIDs              []int64 `json:"group_ids,omitempty" jsonschema:"Numeric IDs of the groups to attach the profile to"`
}

// DetachInput holds parameters for detaching a security scan profile from
// projects and/or groups.
type DetachInput struct {
	SecurityScanProfileID string  `json:"security_scan_profile_id" jsonschema:"Persisted scan profile identifier: the profile's numeric database ID (obtained from security_scan_profile.list_project_statuses) or a full gid:// global ID. A scan-type name (dependency_scanning, sast, ...) is not accepted by detach,required"`
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
		// A global ID is gid://gitlab/<Type>/<id>, and all three parts carry
		// meaning. Reading only the text after the last slash accepted
		// "gid:///5", which has no authority and no type: it passed here and
		// went on the wire unchanged, and GitLab answered a malformed-id error
		// naming nothing the caller could act on. That opaque failure is the
		// one this guard exists to turn into a local one, so the authority and
		// the type are required rather than assumed. The type is not compared
		// against Security::ScanProfile: a global ID for the wrong resource is
		// a different question, and nothing in the detach contract establishes
		// how it should answer.
		authority, typeAndID, found := strings.Cut(rest, "/")
		if !found || authority != gidAuthority {
			return errDetachIdentifier
		}
		// A global ID with no type leaves an empty tail on purpose, so the
		// digit check below is the one place an identifier is refused for
		// being empty. Refusing it here instead would make that check
		// unreachable from this function, and deleting it in turn would make
		// isAllDigits("") answer true, since a loop over no runes finds
		// nothing to object to.
		//
		// The index must be past the start, not merely present: at zero the
		// type is the empty string, which is "gid://gitlab//5". That carries
		// an authority and a separator and still names no resource, so GitLab
		// answers the same malformed-id error as a global ID with no type at
		// all, and it is refused here for the same reason.
		if i := strings.LastIndex(typeAndID, "/"); i > 0 {
			tail = typeAndID[i+1:]
		} else {
			tail = ""
		}
	}
	if !isAllDigits(tail) {
		return errDetachIdentifier
	}
	return nil
}

// errDetachIdentifier is what every rejection in [validateDetachIdentifier]
// answers, so the ways an identifier can be wrong cannot drift into several
// wordings of the same advice.
//
// The message is one literal rather than two joined with a plus. A package
// level declaration carries no coverage counter, so a mutation testing run
// files the arithmetic mutant on that plus as not covered rather than not
// viable, and no test can ever kill it: subtracting one string from another
// does not compile.
var errDetachIdentifier = errors.New("detach requires the persisted profile's numeric ID (from security_scan_profile.list_project_statuses), not a scan-type name")

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
			"use the name of a GitLab default profile ("+DefaultProfileNames+") or a persisted profile's numeric ID; "+
				DefaultProfileFloors+"; "+RefusedByName+"; targets must belong to a group namespace "+
				"(not a personal namespace) and share one root namespace; requires Maintainer or Owner on the targets and Ultimate")
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
			"detach requires the persisted profile's numeric ID (from security_scan_profile.list_project_statuses), not a scan-type name; targets must belong to a group namespace and share one root namespace; requires Maintainer or Owner and Ultimate")
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
