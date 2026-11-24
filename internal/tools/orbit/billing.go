// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package orbit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// billingNamespaceCode is the code GitLab refuses a graph query with when the
// token's user has chosen no default group for Orbit billing
// (Analytics::KnowledgeGraph::GoverningNamespaceError): the body of a 422 from
// POST /orbit/query and /orbit/query/:name.
const billingNamespaceCode = "billing_namespace_required"

// billingNamespaceHint follows a graph query GitLab refused with 422, which
// doc/api/orbit.md gives one meaning.
const billingNamespaceHint = "GitLab.com runs an Orbit query only for a user who has chosen a default group for Orbit billing; the token's user chooses it in their GitLab preferences and the query can then be sent again"

// BillingCandidate is one group the token's user can choose as their default
// group for Orbit billing.
type BillingCandidate struct {
	// ID is the group's ID.
	ID int64 `json:"id"`
	// Name is the group's name.
	Name string `json:"name"`
}

// billingNamespaceError is the body GitLab refuses a graph query with when no
// group is chosen to bill it to.
type billingNamespaceError struct {
	Code       string             `json:"code"`
	Message    string             `json:"message"`
	DeepLink   string             `json:"deep_link"`
	Candidates []BillingCandidate `json:"candidates"`
}

// wrapUnprocessable classifies a 422 from an Orbit route: the billing group
// the token's user has to choose, in GitLab's words when the answer is
// GitLab's ([billingNamespaceRefusal]).
func wrapUnprocessable(op string, err error) error {
	hint := billingNamespaceHint
	if refusal := billingNamespaceRefusal(err); refusal != "" {
		hint = refusal + ". " + hint
	}
	return toolutil.WrapErrWithHint(op, err, hint)
}

// billingNamespaceRefusal renders GitLab's account of the billing group a 422
// asks for: its message, where the user chooses the group and the groups to
// choose from. It returns "" for any other answer. internal/toolutil reflects
// a GitLab body in GitLab's own error shape alone, which this one's extra keys
// put it outside, so it is read here and only in exactly that shape: a key
// GitLab does not send, or another code, is somebody else's answer on the same
// route. A link that is not an address a browser opens is left out, and the
// whole is bounded as every reflected message is.
func billingNamespaceRefusal(err error) string {
	var glErr *gl.ErrorResponse
	if !errors.As(err, &glErr) {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(glErr.Body))
	decoder.DisallowUnknownFields()
	var body billingNamespaceError
	if decoder.Decode(&body) != nil || body.Code != billingNamespaceCode {
		return ""
	}
	text := "GitLab answered " + billingNamespaceCode + ": " + body.Message
	if strings.HasPrefix(body.DeepLink, "https://") {
		text += " Chosen at " + body.DeepLink
	}
	if len(body.Candidates) > 0 {
		groups := make([]string, 0, len(body.Candidates))
		for _, candidate := range body.Candidates {
			groups = append(groups, fmt.Sprintf("%s (%d)", candidate.Name, candidate.ID))
		}
		text += " Groups to choose from: " + strings.Join(groups, ", ")
	}
	return toolutil.BoundedGitLabMessage(text)
}
