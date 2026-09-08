package toolutil

import (
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v2/internal/gitlab"
)

// CapturedMember reads, off the captured answer to a request for one member,
// the fields client-go's member structs do not model. A body that decoded
// for the SDK and does not for [MemberExtra] is reported, since the fault is
// then in the type naming the fields.
func CapturedMember(capture *gitlabclient.ResponseCapture) (MemberExtra, error) {
	var extra MemberExtra
	if err := capture.Decode(&extra); err != nil {
		return MemberExtra{}, err
	}
	return extra, nil
}

// CapturedMembers reads the same off a list answer, one extra per member in
// order, the count held to what the SDK decoded as [CapturedNotes] holds it.
func CapturedMembers(capture *gitlabclient.ResponseCapture, decoded int) ([]MemberExtra, error) {
	return capturedList[MemberExtra](capture, decoded, "members")
}
