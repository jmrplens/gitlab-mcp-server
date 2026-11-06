// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actioncatalog

import (
	"fmt"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
)

// Withheld records the actions a catalog carries but one session may not run,
// split by cause, and words the answer a call for one of them gets.
//
// It is the one place that answer is written, because two surfaces give it.
// The dynamic registry answers a call naming a withheld action through
// gitlab_execute_action, and the meta surface answers a call naming one
// through its group's tool, before the SDK refuses the action as a value
// outside that tool's enum (issue 1282). Each used to be told something
// different: the dynamic surface that the action exists and why it is
// withheld, the meta surface that the value was not one of the enum's, which
// reads as a typo and leaves a model concluding the capability is missing.
//
// The causes are kept apart because only one is something the caller can
// resolve: a narrow credential can be reauthorized, while an operator running
// the deployment read-only, or a licensing tier, cannot be argued with by the
// client. A key is a canonical action ID or any alias that used to resolve to
// one, lowercased and trimmed, and the zero value withholds nothing.
type Withheld struct {
	// byScope maps each key to the scopes the credential lacks for it, kept
	// apart by who asks for them, which the answer names.
	byScope    map[string]ScopeWithheld
	byOperator map[string]struct{}
	// byTier names the actions the instance's licensing tier removed, each key
	// with the lowest tier that serves its action and what would still
	// withhold it once the tier did, and tier and tierPinned are the tier the
	// session was built for and whether an operator named it. Like the
	// operator's narrowing it is not the caller's to resolve, and unlike it
	// the answer allows that a detected tier may be wrong (issue 956).
	byTier     map[string]TierWithheld
	tier       edition.Tier
	tierPinned bool
	tierKnown  bool
	// byCredentialKind names the actions GitLab refuses to the session's
	// kind of credential whatever its scopes, an OAuth access token's
	// (issue 1238), each with the scopes the credential lacks for it besides,
	// which a personal access token would have to carry. The caller can
	// resolve it, by authenticating with such a token, which the answer says,
	// unless personalTokensRefused records that the deployment refuses every
	// personal access token at its door (--oauth-client-uid, ADM-004), where
	// no credential it admits can run the action and the answer asks the
	// operator instead.
	byCredentialKind      map[string]ScopeWithheld
	personalTokensRefused bool
}

// RecordCredentialKind records the actions GitLab refuses to an OAuth access
// token on every way, which the session's credential is (register row
// AUT-009), each with the scopes the credential lacks for it besides, and
// whether the deployment refuses every personal access token, the one
// credential GitLab serves them to. It replaces what an earlier call
// recorded. An entry naming no scope lacks none: a personal access token
// carrying what the OAuth token carries runs it.
func (w *Withheld) RecordCredentialKind(byCredentialKind []ScopeWithheld, personalTokensRefused bool) {
	w.byCredentialKind = withheldKinds(byCredentialKind)
	w.personalTokensRefused = personalTokensRefused
}

// RecordActions records the actions filtered out of the catalog before it was
// handed over, split by cause: byTokenScope for actions the credential itself
// cannot reach, each with the scopes it lacks for it, byOperator for actions a
// deployment-wide setting removed. It replaces what an earlier call recorded
// for the two causes.
func (w *Withheld) RecordActions(byTokenScope []ScopeWithheld, byOperator []string) {
	w.byScope = withheldScopes(byTokenScope)
	w.byOperator = withheldKeySet(byOperator)
}

// RecordTier records the actions the instance's licensing tier removed, each
// with the lowest tier that serves its action, together with the tier the
// session was built for and whether an operator pinned it rather than the
// server detecting it. An action the operator excluded by name must not be
// among them (AUT-004).
func (w *Withheld) RecordTier(byTier []TierWithheld, tier edition.Tier, pinned bool) {
	w.byTier = withheldTiers(byTier)
	w.tier = tier
	w.tierPinned = pinned
	w.tierKnown = true
}

// SessionTier is the tier the session was built for and whether an operator
// pinned it, as [Withheld.RecordTier] recorded them, and known false when
// nothing recorded one. A refusal of a param the tier left out of an
// action's schema is worded with it (issue 1239).
func (w *Withheld) SessionTier() (tier edition.Tier, pinned, known bool) {
	return w.tier, w.tierPinned, w.tierKnown
}

// Holds reports whether the catalog carried the action key and one of the
// deployment's filters took it away, under any cause.
//
// [Withheld.Message] keeps the causes apart because only one of them is
// something the caller can act on. Here the question is narrower: whether
// asking for the action produces an answer that explains itself.
func (w *Withheld) Holds(key string) bool {
	key = withheldKey(key)
	if _, byScope := w.byScope[key]; byScope {
		return true
	}
	if _, byOperator := w.byOperator[key]; byOperator {
		return true
	}
	if _, byKind := w.byCredentialKind[key]; byKind {
		return true
	}
	_, byTier := w.byTier[key]
	return byTier
}

// Message explains an action the catalog knows about but this session may not
// run, rather than calling it unknown: toolName is the tool the call was made
// to, action is the action as the caller named it, which the answer quotes,
// and key is what the withheld record is looked up by. The dynamic surface
// passes the action it was asked for as both; the meta surface passes the
// action argument and the canonical ID its group's routes resolved it to,
// since the records are keyed by ID and a group's action names are not.
//
// Reporting a scope narrowing as a typo is actively misleading: the
// suggestions are all real read-only actions, so the answer looks
// authoritative, and the reader concludes the server lacks the capability
// instead of learning that the credential is what is narrow.
//
// The scope answer names the scopes the credential lacks for this action, and
// who asks for each, since only one of them is GitLab: api when GitLab refuses
// read_api for what the action sends, and admin_mode when this server serves
// the action's catalog group only to a credential carrying it, which GitLab
// does not ask of every action in such a group, nor of any on an instance
// with Admin Mode off. A token lacking only admin_mode was told to reauthorize
// with api until ADR-0026, which sent a caller that already held api round in
// a circle, and was then told GitLab required admin_mode for reads GitLab
// serves any authenticated token.
func (w *Withheld) Message(toolName, action, key string) (string, bool) {
	key = withheldKey(key)
	if withheld, ok := w.byScope[key]; ok {
		scopes, _ := scopeWords(withheld.Missing())
		return fmt.Sprintf("%s: action %q exists but is not available to this session: %s, so a narrowed action surface was built for it. Reauthorize with %s to use it; do not report the capability as missing.", toolName, action, scopeCauses(withheld), scopes), true
	}
	if _, ok := w.byOperator[key]; ok {
		return fmt.Sprintf("%s: action %q exists but is not available: this deployment is configured to withhold it, so a narrowed action surface was built. Ask the operator to enable it; do not report the capability as missing.", toolName, action), true
	}
	if withheld, ok := w.byCredentialKind[key]; ok {
		return w.credentialKindMessage(toolName, action, withheld), true
	}
	if withheld, ok := w.byTier[key]; ok {
		return w.tierWithheldMessage(toolName, action, withheld), true
	}
	return "", false
}

// credentialKindMessage explains an action GitLab refuses to an OAuth access
// token whatever its scopes (issue 1238, register row AUT-009). The way out
// is the one credential GitLab serves it to, a personal access token sent as
// the Bearer token, and the answer names the scopes that token must carry
// when the credential in use lacks them, since sending the caller to
// reauthorize the OAuth token for a scope would only reach this answer again.
//
// A deployment that pins the OAuth applications it admits refuses every
// personal access token at its door (--oauth-client-uid, register row
// ADM-004), so nothing the caller can authenticate with runs the action
// there, and the answer asks the operator: it used to send the caller to a
// personal access token, which the door then refused with 401.
func (w *Withheld) credentialKindMessage(toolName, action string, withheld ScopeWithheld) string {
	cause := fmt.Sprintf("%s: action %q exists but is not available to this session: GitLab answers what it sends only for a personal access token, and the credential in use is an OAuth access token, so a narrowed action surface was built for it.", toolName, action)
	if w.personalTokensRefused {
		return cause + " This deployment admits only the OAuth applications its operator named, which refuses every personal access token, so no credential it admits can run the action. Ask the operator for a deployment that admits personal access tokens; do not report the capability as missing."
	}
	missing := withheld.Missing()
	if len(missing) == 0 {
		return cause + " Authenticate with a personal access token, sent as the Bearer token, to use it; do not report the capability as missing."
	}
	scopes, _ := scopeWords(missing)
	return cause + " Authenticate with a personal access token carrying " + scopes + ", sent as the Bearer token, to use it, since " + scopeCauses(withheld) + "; do not report the capability as missing."
}

// tierWithheldMessage explains an action the instance's licensing tier
// removed (issue 956, register row AUT-003). The caller cannot change the
// tier, so the answer asks the operator, and it says which tier the action
// needs and which one the session was built for. Where the server detected the
// tier rather than an operator naming it, the answer allows that the detection
// may be wrong, since a tier nothing answered for resolves to Free and pinning
// it is how an operator corrects that. What would still withhold the action
// once the tier served it is said in the same answer ([tierAlsoWithheld]).
//
// The tier it needs, the tier the session was built for and the way out are
// the words every tier refusal shares, a param a tier left out of an action's
// schema among them (issue 1239), so they are toolutil's.
func (w *Withheld) tierWithheldMessage(toolName, action string, withheld TierWithheld) string {
	message := fmt.Sprintf("%s: action %q exists but is not available: it needs %s, and %s, so a narrowed action surface was built. %s",
		toolName, action, toolutil.TierRequired(withheld.Edition), toolutil.SessionTierClause(w.tier, w.tierPinned), toolutil.TierWayOut(w.tierPinned))
	return message + tierAlsoWithheld(withheld) + " Do not report the capability as missing."
}

// tierAlsoWithheld is the sentence the tier's answer adds when pinning the
// tier would only uncover the next refusal: the deployment's read-only mode
// for a write, or the scopes the credential lacks for the action and who asks
// for each, in the words the scope narrowing's own answer uses. Without it a
// Premium write on a read-only Free deployment sent the model to ask the
// operator to pin the tier, and the pinned deployment then answered that it
// withholds the action, so the operator was asked twice. It is empty when
// nothing else stands in the way.
func tierAlsoWithheld(withheld TierWithheld) string {
	if withheld.ReadOnlyMode {
		return " This deployment is also read-only, which withholds the action at any tier, so the operator would have to allow writes as well."
	}
	scope := ScopeWithheld{ByGitLab: withheld.ByGitLab, ByServer: withheld.ByServer}
	missing := scope.Missing()
	if len(missing) == 0 {
		return ""
	}
	scopes, _ := scopeWords(missing)
	return " Once the tier serves it, " + scopeCauses(scope) + ": reauthorize with " + scopes + " as well."
}

// scopeCauses says who asks for each scope a credential lacks for an action,
// GitLab for what the action sends and this server for the group the action
// belongs to, and that the credential carries none of them: "GitLab requires
// the api scope for it and the credential in use does not carry it", or both
// causes before a shared ending. Every entry names at least one scope, since
// [withheldScopes] records GitLab's api for one that names none.
func scopeCauses(withheld ScopeWithheld) string {
	var causes []string
	if len(withheld.ByGitLab) > 0 {
		gitlab, _ := scopeWords(withheld.ByGitLab)
		causes = append(causes, "GitLab requires "+gitlab+" for it")
	}
	if len(withheld.ByServer) > 0 {
		server, _ := scopeWords(withheld.ByServer)
		causes = append(causes, "this server serves this action's group only to a credential carrying "+server)
	}
	_, pronoun := scopeWords(withheld.Missing())
	if len(causes) == 1 {
		return causes[0] + " and the credential in use does not carry " + pronoun
	}
	return strings.Join(causes, ", ") + ", and the credential in use does not carry " + pronoun
}

// scopeWords spells scopes as the withheld answer names them, "the api scope"
// or "the admin_mode and api scopes", with the pronoun that refers back to
// them.
func scopeWords(scopes []string) (named, pronoun string) {
	if len(scopes) == 1 {
		return "the " + scopes[0] + " scope", "it"
	}
	return "the " + strings.Join(scopes, " and ") + " scopes", "them"
}

// withheldKey is how a key is indexed and looked up: lowercased and trimmed,
// which is how a caller's action is compared.
func withheldKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// withheldTiers indexes the actions the tier withheld by key. An entry with no
// key withholds nothing.
func withheldTiers(withheld []TierWithheld) map[string]TierWithheld {
	if len(withheld) == 0 {
		return nil
	}
	tiers := make(map[string]TierWithheld, len(withheld))
	for _, entry := range withheld {
		if key := withheldKey(entry.ID); key != "" {
			tiers[key] = entry
		}
	}
	return tiers
}

// withheldScopes indexes the actions a credential's scopes withheld by key,
// each with the scopes it lacks. An entry naming none is recorded as lacking
// api for GitLab, the scope every classic narrowing of a write lacks, so the
// answer always names something to reauthorize with.
func withheldScopes(withheld []ScopeWithheld) map[string]ScopeWithheld {
	if len(withheld) == 0 {
		return nil
	}
	scopes := make(map[string]ScopeWithheld, len(withheld))
	for _, entry := range withheld {
		key := withheldKey(entry.ID)
		if key == "" {
			continue
		}
		if len(entry.Missing()) == 0 {
			entry.ByGitLab = []string{"api"}
		}
		scopes[key] = entry
	}
	return scopes
}

// withheldKinds indexes the actions the credential's kind withheld by key,
// each with the scopes the credential lacks for it besides. Unlike
// [withheldScopes] it names no scope for an entry naming none: the kind is
// the cause there, and an entry lacks a scope only when the scope steps say
// so.
func withheldKinds(withheld []ScopeWithheld) map[string]ScopeWithheld {
	if len(withheld) == 0 {
		return nil
	}
	kinds := make(map[string]ScopeWithheld, len(withheld))
	for _, entry := range withheld {
		if key := withheldKey(entry.ID); key != "" {
			kinds[key] = entry
		}
	}
	return kinds
}

// withheldKeySet indexes a list of keys. A key that is blank or only spaces is
// not indexed, so a caller that sent an empty action is never answered as if
// it had named a withheld one.
func withheldKeySet(keys []string) map[string]struct{} {
	if len(keys) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if normalized := withheldKey(key); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	return set
}
