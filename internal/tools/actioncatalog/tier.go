// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package actioncatalog

// TierWithheld is one action key the instance's licensing tier withheld, with
// the lowest tier that serves the action, as its spec spells it ("premium" or
// "ultimate"). The key is a canonical action ID or an alias that resolved to
// one, as a caller may ask for either.
//
// The tier filter runs while the catalog is built, so an action it removed is
// in no catalog a session holds and cannot be recorded the way the scope and
// operator narrowings record theirs, by comparing a catalog before and after.
// It is recorded from the specs instead (issue 956), so a call for one can be
// answered with the tier it needs, which only an operator can change, rather
// than as an action that does not exist.
//
// The rest says what else would withhold the action once the tier served it,
// so that answer can say so too rather than send the operator to pin a tier
// that would only uncover the next refusal: a deployment's read-only mode,
// for a write, or the scopes the session's credential lacks for it.
type TierWithheld struct {
	ID      string
	Edition string
	// ReadOnlyMode says the deployment's read-only mode withholds the action
	// too, since it writes. Where it is set the scopes below are empty, as the
	// filters file such a write under the operator whatever scope the
	// credential lacks besides.
	ReadOnlyMode bool
	// ByGitLab and ByServer are the scopes the credential lacks for the
	// action, as [ScopeWithheld] names them.
	ByGitLab []string
	ByServer []string
}
