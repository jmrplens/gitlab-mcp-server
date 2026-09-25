package tenancy

// Failure is one refusal an authentication path returns, and whether it is
// charged to the authentication budgets.
//
// INV-007 used to be an absence: a failure was uncharged where nobody had
// written a charge call. The table states it instead, one row for every refusal
// the three admitting functions return, charged or not, and the gate binds each
// row to its return. So a charge added to a refusal the caller did not cause,
// or moved from one branch to its sibling, fails until the row says so, and
// [ValidateFailures] refuses the row unless the failure is the caller's doing.
type Failure struct {
	// Kind names the failure, "missing-credential".
	Kind string
	// Attributable says the failure is the caller's doing: no credential, or a
	// credential GitLab refused.
	Attributable bool
	// Charged says it is charged to AUB-001, AUB-003 and, behind a trusted
	// proxy header, AUB-002.
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
}

// The three functions that admit or refuse a credential, as [Failure.At]
// names them.
func resolveSite() Site {
	return Site{Pkg: "cmd/server", Name: "mcpServerGate.resolve", Role: Charge, Call: "mcpServerGate.chargeFailure", Count: 2}
}

func checkSite() Site {
	return Site{Pkg: "cmd/server", Name: "bearerGuard.check", Role: Charge, Call: "bearerGuard.recordFailure", Count: 2}
}

func classifySite() Site {
	return Site{Pkg: "cmd/server", Name: "bearerGuard.classify", Role: Charge, Call: "bearerGuard.recordFailure", Count: 1}
}

// The refusal texts more than one failure begins with.
const (
	blockedPrefix  = "Too many failed authentication attempts from this address."
	rejectedPrefix = "GitLab rejected this token."
	severalPrefix  = "This deployment serves several GitLab instances"
	recipientText  = "This token is valid for the GitLab instance"
)

// Failures returns every refusal the legacy gate and the OAuth bearer guard
// return: eight in the gate's resolve, seven in the guard's check and six in
// its classify. Five are charged, and each of them is a failure the caller
// caused; the rest are refused without a charge, because the credential was
// never judged or the refusal is about the request rather than the token.
//
// A return that hands back another listed function's answer, as check does
// with classify's, is not a refusal of its own and has no row.
func Failures() []Failure {
	return []Failure{
		// The legacy gate.
		{Kind: "blocked", Decision: "AUB-001", At: resolveSite(), Status: 429, Prefix: blockedPrefix},
		{
			Kind: "missing-credential", Attributable: true, Charged: true, Decision: "IDN-001",
			At: resolveSite(), Status: 401, Prefix: "Authentication required: send a GitLab personal access token",
		},
		{Kind: "no-instance-selected", Decision: "ADM-011", At: resolveSite(), Status: 400, Prefix: severalPrefix},
		{Kind: "no-instance-published", Decision: "ADM-011", At: resolveSite(), Status: 400},
		{Kind: "invalid-instance", Decision: "ADM-011", At: resolveSite(), Status: 400},
		{Kind: "destination-refused", Decision: "DST-001", At: resolveSite(), Status: 400},
		{
			Kind: "gitlab-rejected", Attributable: true, Charged: true, Decision: "ADM-001",
			At: resolveSite(), Status: 401, Prefix: rejectedPrefix,
		},
		{
			Kind: "upstream", Decision: "ADM-001", At: resolveSite(), Status: 503,
			Prefix: "Could not initialize a GitLab session for this token.",
		},

		// The OAuth bearer guard, before verification.
		{Kind: "blocked", Decision: "AUB-001", At: checkSite(), Status: 429, Prefix: blockedPrefix},
		{
			Kind: "missing-credential", Attributable: true, Charged: true, Decision: "IDN-001",
			At: checkSite(), Status: 401, Prefix: "Authentication required: send an OAuth access token",
		},
		{Kind: "no-instance-selected", Decision: "ADM-011", At: checkSite(), Status: 400, Prefix: severalPrefix},
		{
			Kind: "instance-not-published", Decision: "ADM-011", At: checkSite(), Status: 403,
			Prefix: "This deployment does not serve the GitLab instance",
		},
		{Kind: "cached-unaccepted-recipient", Decision: "ADM-004", At: checkSite(), Status: 401, Prefix: recipientText},
		{
			Kind: "cached-rejection", Attributable: true, Charged: true, Decision: "ADM-006",
			At: checkSite(), Status: 401, Prefix: rejectedPrefix,
		},
		{Kind: "scope-below-minimum", Decision: "ADM-002", At: checkSite(), Status: 403},

		// The OAuth bearer guard, classifying a verification error.
		{
			Kind: "upstream", Decision: "ADM-002", At: classifySite(), Status: 503,
			Prefix: "GitLab could not verify this token right now;",
		},
		{
			Kind: "gitlab-insufficient-scope", Decision: "ADM-002", At: classifySite(), Status: 403,
			Prefix: "GitLab rejected this token for lacking the scope",
		},
		{
			Kind: "introspection-unanswered", Decision: "ADM-004", At: classifySite(), Status: 503,
			Prefix: "This deployment admits only tokens issued to specific OAuth applications",
		},
		{Kind: "unaccepted-recipient", Decision: "ADM-004", At: classifySite(), Status: 401, Prefix: recipientText},
		{
			Kind: "gitlab-rejected", Attributable: true, Charged: true, Decision: "ADM-002",
			At: classifySite(), Status: 401, Prefix: rejectedPrefix,
		},
		{
			Kind: "verification-failed", Decision: "ADM-002", At: classifySite(), Status: 503,
			Prefix: "GitLab could not verify this token right now.",
		},
	}
}
