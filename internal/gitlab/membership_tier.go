package gitlab

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// MembershipTierQuery asks GitLab.com for the top-level groups the caller
// reaches and, for each, whether its plan carries a Premium and an Ultimate
// feature (issue 1224). It is exported for one reader, the test in
// internal/testutil that holds its cost to what GitLab.com measured: the
// estimate lives there, testutil imports this package so this package's tests
// cannot import it, and an external test package here would cost the package
// its condition gate (gobco cannot instrument one).
//
// **Features rather than the plan.** Group.plan is authorized for
// :admin_namespace, so a member who does not administer the group reads null,
// the same wall GET /namespaces puts up. licensedFeatureAvailability is open to
// any member, and on GitLab.com it answers from the plan of the group's root
// ancestor, trials and the open source program included, which is the check
// GitLab itself makes before serving a licensed feature. EPICS is a Premium
// feature and SECURITY_DASHBOARD an Ultimate one in GitlabSubscriptions::
// Features, and neither is global, which GitLab refuses to answer for.
//
// **Which groups.** allAvailable: false is GroupsFinder's authorized groups:
// direct memberships, the groups shared with them, and the groups of the
// projects the caller is a member of, each with its ancestors, so
// topLevelOnly: true returns the root of every group the caller works in, a
// developer of a subgroup included. GitLab.com answered this for the
// maintainer's account on 2026-10-08, a Developer of a subgroup of a licensed
// group who administers only free namespaces: gitlab-community available for
// both features, where the namespace step had answered Free. A role is not
// filtered on, because the page offers no role at a root a caller reaches only
// through a subgroup; a member of any role is served a licensed feature
// GitLab's own policy lets that role use, and the role decides the rest.
//
// **A fine-grained token gets no answer from it.** GitLab declares no
// fine-grained permission for LicensedFeatureAvailability (it is on the
// authorization to-do list of the 19.4.1 record), so it refuses that answer to
// every fine-grained token. The field and its available are both non-null, so
// the refusal nulls the whole node: such a token is answered a page of null
// groups beside an errors array, no group answers, and it keeps the tier the
// namespace step gave it.
//
// It costs 22 at a page of a hundred, measured on GitLab.com, against the 250
// GitLab refuses a query above.
const MembershipTierQuery = `query tierFromMembership($first: Int!, $after: String) {
  groups(allAvailable: false, topLevelOnly: true, first: $first, after: $after) {
    nodes {
      fullPath
      premium: licensedFeatureAvailability(feature: EPICS) { available }
      ultimate: licensedFeatureAvailability(feature: SECURITY_DASHBOARD) { available }
    }
    pageInfo { hasNextPage endCursor }
  }
}`

// membershipPageSize and membershipMaxPages bound the top-level groups the
// tier probe reads, for the reason namespacePlanPageSize and
// namespacePlanMaxPages give: the walk stops at the first Ultimate, which
// settles the common case on the first page, and after membershipMaxPages
// either way, so a caller reaching more top-level groups than that resolves on
// those it was shown.
const (
	membershipPageSize = tenancy.TierMembershipPageSize // register row AUT-003
	membershipMaxPages = tenancy.TierMembershipMaxPages // register row AUT-003
)

// membershipTierApplies reports whether the membership step is asked of c's
// instance: GitLab.com alone, the one instance that sells its plans per
// top-level group. Every other instance is licensed as a whole, which the
// license step reads. It is a variable so a test can stand an httptest server
// in for GitLab.com, whose host a test server cannot have.
var membershipTierApplies = (*Client).IsGitLabDotCom

// errNoGroupsConnection is a membership page GitLab answered with no groups
// connection and no error, which is no answer.
var errNoGroupsConnection = errors.New("the answer carried no groups connection")

// membershipTierAnswer is GitLab's answer to [MembershipTierQuery].
type membershipTierAnswer struct {
	Data struct {
		Groups *membershipGroups `json:"groups"`
	} `json:"data"`
	Errors []membershipTierError `json:"errors"`
}

// membershipTierError is one top-level error GitLab answered the document
// with.
type membershipTierError struct {
	Message string `json:"message"`
}

// membershipGroups is one page of the top-level groups the caller reaches.
type membershipGroups struct {
	Nodes    []*membershipGroup `json:"nodes"`
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
}

// membershipGroup is one top-level group and whether its plan carries the two
// features the query asks about.
type membershipGroup struct {
	FullPath string           `json:"fullPath"`
	Premium  featureAvailable `json:"premium"`
	Ultimate featureAvailable `json:"ultimate"`
}

// featureAvailable is GitLab's answer about one licensed feature.
type featureAvailable struct {
	Available bool `json:"available"`
}

// tier is the tier the group's plan carries, read from the features it makes
// available.
func (g *membershipGroup) tier() edition.Tier {
	switch {
	case g.Ultimate.Available:
		return edition.Ultimate
	case g.Premium.Available:
		return edition.Premium
	default:
		return edition.Free
	}
}

// tierFromMembership reads the plans of the top-level groups the caller
// reaches on GitLab.com and answers with the highest, for the reason
// [Client.tierFromNamespaces] gives for taking the highest. It answers false
// when no group answered, which a caller in no group is, and a read that fails
// keeps what earlier pages answered.
//
// A page GitLab answered in part keeps the groups it resolved and ends the
// walk. Every node that came back was resolved whole (see
// [MembershipTierQuery]), so a group answering Premium is evidence of Premium
// whatever GitLab refused beside it, and discarding it would resolve lower
// than the caller was shown to hold; the page after it would be refused the
// same way, which is what a fine-grained token's walk would otherwise spend
// up to membershipMaxPages requests learning.
//
// A read that did not come back whole is said at info, naming the setting
// that pins the tier, because the namespace step may already have answered
// Free: no warning fires then, and the tier a caller is served would say
// nothing about the step that could have raised it.
func (c *Client) tierFromMembership(ctx context.Context) (edition.Tier, bool) {
	best, found := edition.Free, false
	after := ""
	// The page cap ends the walk, as it does the namespace walk, and for the
	// same reason no second copy of it sits in the loop header.
	for page := 1; ; page++ {
		groups, err := c.membershipPage(ctx, after)
		if groups != nil {
			for _, group := range groups.Nodes {
				if group == nil {
					continue
				}
				tier := group.tier()
				best, found = max(best, tier), true
				slog.DebugContext(ctx, "a top-level group the token reaches answers for its plan", "group", group.FullPath, "tier", tier.String())
			}
		}
		if err != nil {
			slog.InfoContext(ctx, "could not read every top-level group the token is a member of; the tier is resolved without the rest, "+
				"and GITLAB_MCP_TIER (or --tier in HTTP mode) pins it", "page", page, "error", err)
			break
		}
		if best == edition.Ultimate || !groups.PageInfo.HasNextPage {
			break
		}
		if page == membershipMaxPages {
			slog.DebugContext(ctx, "stopped reading top-level groups at the page bound; a paid plan past it is not seen",
				"pages", membershipMaxPages, "per_page", membershipPageSize)
			break
		}
		after = groups.PageInfo.EndCursor
	}

	if found {
		slog.InfoContext(ctx, "detected GitLab tier from the top-level groups the token is a member of", "tier", best.String())
	}
	return best, found
}

// membershipPage asks for the page of top-level groups that follows after, or
// the first page when after is empty.
//
// GitLab answers a document it refuses with HTTP 200 and a top-level errors
// array, which client-go does not turn into an error, so the answer is read
// for one. Without a groups connection the page is no answer, and the error
// is GitLab's reason where it gave one. With one, the page is returned beside
// the error naming what GitLab refused in it, so the caller keeps the groups
// it resolved.
func (c *Client) membershipPage(ctx context.Context, after string) (*membershipGroups, error) {
	variables := map[string]any{"first": membershipPageSize}
	if after != "" {
		variables["after"] = after
	}
	var answer membershipTierAnswer
	if _, err := c.inner.GraphQL.Do(gl.GraphQLQuery{Query: MembershipTierQuery, Variables: variables}, &answer, gl.WithContext(ctx)); err != nil {
		return nil, err
	}
	refusal := graphQLRefusal(answer.Errors)
	if answer.Data.Groups == nil {
		if refusal != nil {
			return nil, refusal
		}
		return nil, errNoGroupsConnection
	}
	return answer.Data.Groups, refusal
}

// graphQLRefusal is the error a top-level errors array stands for, or nil
// when GitLab answered none.
func graphQLRefusal(refusals []membershipTierError) error {
	if len(refusals) == 0 {
		return nil
	}
	messages := make([]string, 0, len(refusals))
	for _, refusal := range refusals {
		messages = append(messages, refusal.Message)
	}
	return errors.New("GitLab answered the document with errors: " + strings.Join(messages, "; "))
}
