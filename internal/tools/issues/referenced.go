package issues

import (
	gl "gitlab.com/gitlab-org/api/client-go/v3"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
)

// ReferencedOutput is one row of the issues a merge request closes or relates
// to. lib/api/merge_requests.rb answers both listings with two entities in one
// array: an issue of this instance as API::Entities::IssueBasic, and an issue
// of the external tracker a project can use instead (Jira and its peers) as
// API::Entities::ExternalIssue, which is a title and an `id` holding the
// tracker's own identifier as a string. client-go's Issue moves a string id
// into ExternalID, the one place that identifier survives the decode, so the
// row carries it beside the basic issue; an issue of this instance leaves it
// empty and omitted.
//
// No other route renders an ExternalIssue, which is why the key lives on this
// row rather than on the basic issue every issue listing shares. It is defined
// here rather than beside the merge request handlers so that the basic issue
// it embeds is a type of its own package.
type ReferencedOutput struct {
	BasicOutput
	ExternalID string `json:"external_id,omitempty"`
}

// toReferencedOutput converts one row of the two listings: the basic issue,
// and the external tracker's identifier when the row is one of its issues.
func toReferencedOutput(issue *gl.Issue, basic BasicOutput) ReferencedOutput {
	return ReferencedOutput{BasicOutput: basic, ExternalID: issue.ExternalID}
}

// ToReferencedOutputs converts a page of a merge request's closes-issues or
// related-issues listing, reading the keys client-go does not model off the
// captured answer the way every IssueBasic listing does.
func ToReferencedOutputs(list []*gl.Issue, captured *gitlabclient.ResponseCapture) ([]ReferencedOutput, error) {
	basics, err := ToBasicOutputs(list, captured)
	if err != nil {
		return nil, err
	}
	out := make([]ReferencedOutput, len(list))
	for i, issue := range list {
		out[i] = toReferencedOutput(issue, basics[i])
	}
	return out, nil
}
