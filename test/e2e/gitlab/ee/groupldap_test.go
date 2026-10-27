// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// groupldap_test.go covers a group's LDAP links and the LDAP sync. A link
// is a record GitLab keeps about a provider it will consult later, so the
// link actions answer on any licensed instance whether or not a directory
// is reachable; the sync is what needs group sync configured on the
// instance, and the Docker stack does not configure it, so it is refused
// and the refusal is asserted.

package ee

import (
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/groupldap"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// The provider the Docker stack configures and the access a link grants.
const (
	ldapProvider    = "ldapmain"
	ldapGroupAccess = int64(30)
)

// ldapLinkCNs lists the common names of a link listing.
func ldapLinkCNs(links []groupldap.Output) []string {
	cns := make([]string, 0, len(links))
	for _, link := range links {
		cns = append(cns, link.CN)
	}
	return cns
}

// containsCN reports whether a listing holds a link by common name.
func containsCN(links []groupldap.Output, cn string) bool {
	for _, link := range links {
		if link.CN == cn {
			return true
		}
	}
	return false
}

// TestGroupLDAPLinks_Lifecycle_AddListDeleteAndDeleteForProvider adds a
// link on every surface in a group of its own, finds it in the listing,
// deletes it by common name, then adds another and deletes it by provider.
//
// Replaces: TestMeta_GroupLDAPLinks, TestEE_MetaGroupEnterpriseOperations
func TestGroupLDAPLinks_Lifecycle_AddListDeleteAndDeleteForProvider(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldap"))
		params := map[string]any{"group_id": group.IDParam()}
		cn := e.Name("cn")

		empty := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if len(empty.Links) != 0 {
			e.T.Errorf("a fresh group lists %d LDAP link(s): %v", len(empty.Links), ldapLinkCNs(empty.Links))
		}

		added := harness.Do[groupldap.Output](s, actionGroupLDAPLinkAdd, withParams(params, map[string]any{
			"cn": cn, "group_access": ldapGroupAccess, "provider": ldapProvider,
		}))
		if added.CN != cn || added.Provider != ldapProvider {
			e.T.Errorf("ldap_link_add answered %+v, want the link %q on %q", added, cn, ldapProvider)
		}
		listed := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if !containsCN(listed.Links, cn) {
			e.T.Errorf("the group's LDAP links do not hold %q: %v", cn, ldapLinkCNs(listed.Links))
		}

		harness.DoVoid(s, actionGroupLDAPLinkDelete, withParams(params, map[string]any{"cn": cn, "provider": ldapProvider}))
		after := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if containsCN(after.Links, cn) {
			e.T.Errorf("the group's LDAP links still hold %q after its delete", cn)
		}

		byProvider := e.Name("provider-cn")
		harness.DoVoid(s, actionGroupLDAPLinkAdd, withParams(params, map[string]any{
			"cn": byProvider, "group_access": ldapGroupAccess, "provider": ldapProvider,
		}))
		harness.DoVoid(s, actionGroupLDAPLinkDeleteForProvider, withParams(params, map[string]any{"cn": byProvider, "provider": ldapProvider}))
		final := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, params)
		if containsCN(final.Links, byProvider) {
			e.T.Errorf("the group's LDAP links still hold %q after the delete by provider", byProvider)
		}
	})
}

// TestGroupLDAPLinks_BothOrNeitherSelector_AreRefusedBeforeGitLab asks every
// surface to add and to delete an LDAP link naming both a common name and a
// filter, and naming neither. GitLab declares exactly_one_of :cn, :filter on
// both routes (issue 1270), so each call is one it refuses, and the test
// checks that each is refused here, before it is sent, and that the group's
// one link is the one the test added by common name and is still there.
//
// The link is added, and deleted at the end, with an empty filter beside its
// common name. A selector with no value counts as none on every surface, as
// the handler counts it when it builds the request: the individual tool's
// oneOf used to count the key alone and refuse that call as naming both,
// which the other two surfaces served.
//
// Each surface refuses at the first place that checks, and the test holds
// each to its own. The individual tool is served without the oneOf the
// catalog states the constraint with, since the Anthropic Messages API
// refuses a tool list holding a root oneOf, so its arguments reach the
// handler as the meta dispatcher's do: it checks the required list alone and
// hands both calls to the handler, which refuses a link naming both as well
// as one naming neither. The dynamic dispatcher refuses a call naming neither
// itself, since it completes none of the alternatives find publishes, and
// hands one naming both to the handler, and so does the individual tool for
// the delete, which is destructive and refuses such a call in the same words
// before it asks for the delete to be confirmed. The words asserted are the
// server's own and none of them is in the hint a GitLab refusal is wrapped
// with, so a call that reached GitLab would not pass for one refused here.
func TestGroupLDAPLinks_BothOrNeitherSelector_AreRefusedBeforeGitLab(t *testing.T) {
	e := harness.New(t)

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.On(surface)
		group := fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldapone"))
		target := map[string]any{"group_id": group.IDParam(), "provider": ldapProvider}
		cn := e.Name("cn")
		both := withParams(target, map[string]any{"cn": cn, "filter": "(uid=" + e.Name("uid") + ")"})
		access := map[string]any{"group_access": ldapGroupAccess}

		byCommonName := withParams(target, map[string]any{"cn": cn, "filter": ""})
		harness.DoVoid(s, actionGroupLDAPLinkAdd, withParams(byCommonName, access))

		calls := []struct {
			name    string
			id      harness.ActionID
			params  map[string]any
			handler string
		}{
			{name: "an add naming both", id: actionGroupLDAPLinkAdd, params: withParams(both, access), handler: "not both"},
			{name: "an add naming neither", id: actionGroupLDAPLinkAdd, params: withParams(target, access), handler: "exactly one of cn or filter is required"},
			{name: "a delete naming both", id: actionGroupLDAPLinkDelete, params: both, handler: "not both"},
			{name: "a delete naming neither", id: actionGroupLDAPLinkDelete, params: target, handler: "exactly one of cn or filter is required"},
		}
		for _, call := range calls {
			dispatcherRefuses := surface == harness.SurfaceDynamic ||
				surface == harness.SurfaceIndividual && call.id == actionGroupLDAPLinkDelete
			switch {
			case dispatcherRefuses && call.handler != "not both":
				harness.ExpectToolError(s, call.id, call.params, "missing exactly one of cn or filter")
			default:
				refused := harness.ExpectToolError(s, call.id, call.params, call.handler)
				assertMentions(e, call.name+" refused", refused, "exactly one of cn or filter")
			}
		}

		listed := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, map[string]any{"group_id": group.IDParam()})
		if len(listed.Links) != 1 || listed.Links[0].CN != cn || listed.Links[0].Filter != "" {
			e.T.Errorf("the group's LDAP links after the refused calls = %v, want the one link %q added by common name", ldapLinkCNs(listed.Links), cn)
		}

		harness.DoVoid(s, actionGroupLDAPLinkDelete, byCommonName)
		if after := harness.Do[groupldap.ListOutput](s, actionGroupLDAPLinkList, map[string]any{"group_id": group.IDParam()}); containsCN(after.Links, cn) {
			e.T.Errorf("the group's LDAP links still hold %q after its delete with an empty filter", cn)
		}
	})
}

// TestGroupLDAPSync_NoGroupSync_IsRefusedWithTheProviderHint asks a fresh
// group to sync with LDAP on every surface and checks the refusal names
// what a caller has to have for it: the provider, the tier and the access.
//
// Replaces: TestMeta_GroupLDAPSync
func TestGroupLDAPSync_NoGroupSync_IsRefusedWithTheProviderHint(t *testing.T) {
	e := harness.New(t)

	harness.SurfacesWith(e, func(e *harness.Env) fixture.Group {
		return fixture.NewGroup(e, fixture.WithGroupNamePrefix("ldapsync"))
	}, func(e *harness.Env, surface harness.Surface, group fixture.Group) {
		s := e.On(surface)
		refused := harness.Refused(s, actionGroupLDAPSync, map[string]any{"group_id": group.IDParam()}, harness.FailureNotFound)
		assertMentions(e, "the LDAP sync refusal", refused, "LDAP provider", "Premium/Ultimate", "Owner access")
	})
}
