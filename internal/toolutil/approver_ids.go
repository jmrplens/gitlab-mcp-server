package toolutil

import (
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v2"
)

// ApproverIDsFilter mirrors GitLab's approver_ids and approved_by_ids merge
// request filters, which accept either a list of numeric user IDs or exactly
// one of the literals "Any" and "None".
//
// Elements are [StringOrInt], so both JSON numbers and JSON strings unmarshal
// cleanly. Actions exposing this filter must widen the published item type
// with [SchemaApproverIDsOverride], otherwise input validation rejects the
// literals before the handler ever sees them.
type ApproverIDsFilter []StringOrInt

// Approver filter literals accepted in place of a list of user IDs.
const (
	// ApproverAny matches merge requests with at least one approver.
	ApproverAny = "Any"
	// ApproverNone matches merge requests with no approvers.
	ApproverNone = "None"
)

// ApproverIDsValue converts the filter into the SDK value expected by the
// merge request list options. It returns nil for an empty filter, so callers
// can assign the result unconditionally.
//
// The literals are only meaningful on their own: GitLab has no notion of
// "None plus these IDs", so mixing them with user IDs is rejected rather than
// silently dropped, which would return a differently-filtered result set than
// the caller asked for.
//
//nolint:nilnil // a nil value with a nil error is the "no filter" result callers assign directly
func (f ApproverIDsFilter) ApproverIDsValue() (*gl.ApproverIDsValue, error) {
	if len(f) == 0 {
		return nil, nil
	}

	if literal, ok := approverLiteral(f[0]); ok {
		if len(f) > 1 {
			return nil, fmt.Errorf("%q must be the only value; it cannot be combined with user IDs", literal)
		}
		return gl.ApproverIDs(gl.UserIDValue(literal)), nil
	}

	ids := make([]int64, 0, len(f))
	for _, entry := range f {
		if _, ok := approverLiteral(entry); ok {
			return nil, fmt.Errorf("%q must be the only value; it cannot be combined with user IDs", entry)
		}
		id, err := entry.Int64()
		if err != nil {
			return nil, fmt.Errorf("%q is not a user ID, %q or %q", entry, ApproverAny, ApproverNone)
		}
		ids = append(ids, id)
	}
	return gl.ApproverIDs(ids), nil
}

// approverLiteral reports whether value is one of the special filter literals,
// returning it in the canonical capitalization GitLab expects. Matching is
// case-insensitive because models routinely emit "any" or "NONE".
func approverLiteral(value StringOrInt) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value.String())) {
	case "any":
		return ApproverAny, true
	case "none":
		return ApproverNone, true
	default:
		return "", false
	}
}
