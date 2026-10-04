package tenancy

// Failure is one refusal an authentication path returns, and whether it is
// charged to the authentication budgets.
//
// INV-007 used to be an absence: a failure was uncharged where nobody had
// written a charge call. The table states it instead, one row for every refusal
// the four admitting functions return, charged or not, and the gate binds each
// row to its return. So a charge added to a refusal the caller did not cause,
// or moved from one branch to its sibling, fails until the row says so, and
// [ValidateFailures] refuses the row unless the failure is the caller's doing.
type Failure struct {
	// Kind names the failure, "missing-credential".
	Kind string
	// Attributable says the failure is the caller's doing: no credential, or a
	// credential GitLab refused.
	Attributable bool
	// Charged says it is charged to the authentication budgets. Which of them
	// is its row's refusal's to say ([Refusal.Charged]): a missing credential
	// spends AUB-001 and AUB-002 and not AUB-003, which counts nothing for an
	// empty credential, and a rejected one spends all three. The gate holds the
	// two statements to each other (G7).
	Charged bool
	// Decision is the row whose refusal it is.
	Decision string
	// At is the function that returns the refusal, with Role Charge, the
	// charge helper in Call and the charged failures it returns in Count.
	At Site
	// Status is the refusal's HTTP status, which identifies the return
	// together with Prefix.
	Status int
	// Prefix is the refusal text's leading words where they fold to a
	// constant, and empty where they do not.
	Prefix string
	// Challenge says the refusal carries a WWW-Authenticate challenge. It
	// completes the identity Status and Prefix begin: the gate answers one
	// verdict in its own words in legacy mode and in the bearer guard's
	// behind it, with the same status and the same leading sentence, and only
	// the guard's challenge tells the two returns apart.
	Challenge bool
}

// The four functions that admit or refuse a credential, as [Failure.At]
// names them: each door's own, and each door's reading of the error its
// verification returned.
func resolveSite() Site {
	return Site{Pkg: "cmd/server", Name: "mcpServerGate.resolve", Role: Charge, Call: "mcpServerGate.chargeFailure", Count: 1}
}

func gateClassifySite() Site {
	return Site{Pkg: "cmd/server", Name: "mcpServerGate.classify", Role: Charge, Call: "mcpServerGate.chargeFailure", Count: 1}
}

func checkSite() Site {
	return Site{Pkg: "cmd/server", Name: "bearerGuard.check", Role: Charge, Call: "bearerGuard.recordFailure", Count: 2}
}

func classifySite() Site {
	return Site{Pkg: "cmd/server", Name: "bearerGuard.classify", Role: Charge, Call: "bearerGuard.recordFailure", Count: 1}
}

// The refusal texts more than one failure begins with.
const (
	blockedPrefix    = "Too many failed authentication attempts from this address."
	rejectedPrefix   = "GitLab rejected this token."
	severalPrefix    = "This deployment serves several GitLab instances"
	recipientText    = "This token is valid for the GitLab instance"
	permissionPrefix = "GitLab accepted this token and refused it the permission to read its own user."
	// The legacy gate's refusal of a token below the read_api minimum, and
	// the bearer guard's of one GitLab refused its own user for want of a
	// scope (issue 952).
	belowMinimumPrefix      = "GitLab accepted this token, which carries neither the read_api nor the api scope"
	insufficientScopePrefix = "GitLab rejected this token for lacking the scope"
)

// Failures returns every refusal the legacy gate and the OAuth bearer guard
// return: ten in the gate's resolve and six in its classify, nine in the
// guard's check and seven in its classify. Five are charged, and each of them
// is a failure the caller caused; the rest are refused without a charge,
// because the credential was never judged, the refusal is about the request
// rather than the token, or GitLab accepted the token and refused it only the
// permission this server's identity check needs, or found it below the
// read_api minimum.
//
// A return that hands back another listed function's answer, as the gate's
// resolve and the guard's check each do with their classify's, is not a
// refusal of its own and has no row.
func Failures() []Failure {
	return []Failure{
		// The legacy gate.
		{Kind: "blocked", Decision: "AUB-001", At: resolveSite(), Status: 429, Prefix: blockedPrefix},
		{
			Kind: "missing-credential", Attributable: true, Charged: true, Decision: "IDN-001",
			At: resolveSite(), Status: 401, Prefix: "Authentication required: send a GitLab personal access token",
			Challenge: true,
		},
		// The gate's text for this one is built by missingURLMessage, so it does
		// not fold at the return.
		{Kind: "no-instance-selected", Decision: "ADM-011", At: resolveSite(), Status: 400},
		{Kind: "no-instance-published", Decision: "ADM-011", At: resolveSite(), Status: 400},
		{Kind: "invalid-instance", Decision: "ADM-011", At: resolveSite(), Status: 400},
		{Kind: "destination-refused", Decision: "DST-001", At: resolveSite(), Status: 400},
		// GitLab accepted the token and refused the probe User: Read, answered
		// from the rejected-token structure once known and from the pool's
		// probe the first time. Neither is the caller's doing: the token is
		// genuine (INV-007).
		{Kind: "cached-permission-missing", Decision: "ADM-006", At: resolveSite(), Status: 403, Prefix: permissionPrefix},
		// GitLab accepted the token and it carries neither read_api nor api:
		// the probe answered 403 insufficient_scope, or the token's own
		// description named only scopes below the minimum (issue 952). From
		// memory once known, as the permission refusal is, and genuine all the
		// same (INV-007).
		{Kind: "cached-below-minimum", Decision: "ADM-006", At: resolveSite(), Status: 403, Prefix: belowMinimumPrefix},
		// The same two behind the bearer guard in oauth mode, answered in the
		// guard's words and with its insufficient_scope challenge, which the
		// guard repeats from memory. There the pool finds a token below the
		// minimum only when the verifier admitted it on its own api
		// assumption, a personal access token no introspection describes, and
		// one refused User: Read only when its probe and the verifier's
		// disagree. The permission refusal shares its status and its leading
		// sentence with the gate's own, and only the challenge tells them
		// apart.
		{
			Kind: "cached-permission-missing-oauth", Decision: "ADM-006", At: resolveSite(), Status: 403,
			Prefix: permissionPrefix, Challenge: true,
		},
		{
			Kind: "cached-below-minimum-oauth", Decision: "ADM-006", At: resolveSite(), Status: 403,
			Prefix: insufficientScopePrefix, Challenge: true,
		},

		// The legacy gate, classifying the pool's refusal to build an entry:
		// a credential GitLab rejected, one nothing judged, and the first
		// time each of the four refusals above is known, in either mode.
		{
			Kind: "gitlab-rejected", Attributable: true, Charged: true, Decision: "ADM-001",
			At: gateClassifySite(), Status: 401, Prefix: rejectedPrefix, Challenge: true,
		},
		{
			Kind: "upstream", Decision: "ADM-001", At: gateClassifySite(), Status: 503,
			Prefix: "Could not initialize a GitLab session for this token.",
		},
		{Kind: "gitlab-permission-missing", Decision: "ADM-001", At: gateClassifySite(), Status: 403, Prefix: permissionPrefix},
		{Kind: "gitlab-below-minimum", Decision: "ADM-001", At: gateClassifySite(), Status: 403, Prefix: belowMinimumPrefix},
		{
			Kind: "gitlab-permission-missing-oauth", Decision: "ADM-002", At: gateClassifySite(), Status: 403,
			Prefix: permissionPrefix, Challenge: true,
		},
		{
			Kind: "gitlab-below-minimum-oauth", Decision: "ADM-002", At: gateClassifySite(), Status: 403,
			Prefix: insufficientScopePrefix, Challenge: true,
		},

		// The OAuth bearer guard, before verification.
		{Kind: "blocked", Decision: "AUB-001", At: checkSite(), Status: 429, Prefix: blockedPrefix},
		{
			Kind: "missing-credential", Attributable: true, Charged: true, Decision: "IDN-001",
			At: checkSite(), Status: 401, Prefix: "Authentication required: send an OAuth access token",
			Challenge: true,
		},
		{Kind: "no-instance-selected", Decision: "ADM-011", At: checkSite(), Status: 400, Prefix: severalPrefix},
		{
			Kind: "instance-not-published", Decision: "ADM-011", At: checkSite(), Status: 403,
			Prefix: "This deployment does not serve the GitLab instance",
		},
		{
			Kind: "cached-unaccepted-recipient", Decision: "ADM-004", At: checkSite(), Status: 401,
			Prefix: recipientText, Challenge: true,
		},
		{
			Kind: "cached-permission-missing", Decision: "ADM-006", At: checkSite(), Status: 403,
			Prefix: permissionPrefix, Challenge: true,
		},
		{
			Kind: "cached-below-minimum", Decision: "ADM-006", At: checkSite(), Status: 403,
			Prefix: insufficientScopePrefix, Challenge: true,
		},
		{
			Kind: "cached-rejection", Attributable: true, Charged: true, Decision: "ADM-006",
			At: checkSite(), Status: 401, Prefix: rejectedPrefix, Challenge: true,
		},
		{Kind: "scope-below-minimum", Decision: "ADM-002", At: checkSite(), Status: 403, Challenge: true},

		// The OAuth bearer guard, classifying a verification error.
		{
			Kind: "upstream", Decision: "ADM-002", At: classifySite(), Status: 503,
			Prefix: "GitLab could not verify this token right now;",
		},
		{
			Kind: "gitlab-insufficient-scope", Decision: "ADM-002", At: classifySite(), Status: 403,
			Prefix: insufficientScopePrefix, Challenge: true,
		},
		{
			Kind: "gitlab-permission-missing", Decision: "ADM-002", At: classifySite(), Status: 403,
			Prefix: permissionPrefix, Challenge: true,
		},
		{
			Kind: "introspection-unanswered", Decision: "ADM-004", At: classifySite(), Status: 503,
			Prefix: "This deployment admits only tokens issued to specific OAuth applications",
		},
		{
			Kind: "unaccepted-recipient", Decision: "ADM-004", At: classifySite(), Status: 401,
			Prefix: recipientText, Challenge: true,
		},
		{
			Kind: "gitlab-rejected", Attributable: true, Charged: true, Decision: "ADM-002",
			At: classifySite(), Status: 401, Prefix: rejectedPrefix, Challenge: true,
		},
		// A verification that produced no verdict, whether the round trip
		// failed or no slot came free: ADM-014's refusal is this return too, in
		// the same words, as POL-006's is ADM-001's above.
		{
			Kind: "verification-failed", Decision: "ADM-002", At: classifySite(), Status: 503,
			Prefix: "GitLab could not verify this token right now.",
		},
	}
}
