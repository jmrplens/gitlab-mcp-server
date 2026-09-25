package tenancy

// identifyDecisions are the rows that answer "who is this request?" (spec 4.4):
// how the credential is read, what the entry, the owner, the address and the
// source are, how a request is bound to its credential, and how the tenant
// itself is resolved.
//
// The tenant is resolved and read for attribution only: no allowance is keyed
// on it (F-01, issue 955), and no Tenant type exists, because nothing would
// consume one. IDN-008 names the code home of TEN-006's unknown rule, a zero or
// missing user id is unknown, so that it cannot move unnoticed.
func identifyDecisions() []Decision {
	return []Decision{
		{
			ID: "IDN-001", Question: Identify, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the credential a request presents",
			Key:      KeyRequest, StdioKey: KeyProcess,
			// Charged to the fast budget and the transport-source one. The
			// distinct-credential budget is handed the empty credential too
			// and counts nothing for it: a client that forgot its header must
			// not move the escalation ladder.
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: "Authentication required: send a GitLab personal access token",
					Answer: Reauthorize, Charged: []string{"AUB-001", "AUB-002"},
					At: refuse(pkgServer, "mcpServerGate.resolve"),
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: "Authentication required: send an OAuth access token",
					Answer: Reauthorize, Charged: []string{"AUB-001", "AUB-002"},
					At: refuse(pkgServer, "bearerGuard.check"),
				},
			},
			Sites: []Site{
				enforce(pkgPool, "ExtractToken"),
				enforce(pkgServer, "mcpServerGate.extractCredential"),
				refuse(pkgServer, "mcpServerGate.resolve"),
				refuse(pkgServer, "bearerGuard.check"),
			},
		},
		{
			ID: "IDN-002", Question: Identify, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "the pool entry, one per credential per instance",
			Key:      KeyEntry, StdioKey: KeyNone,
			Findings: []string{"F-01"},
			Sites: []Site{
				derive(pkgPool, "sessionKey"),
				derive(pkgPool, "canonicalHost"),
			},
		},
		{
			ID: "IDN-003", Question: Identify, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "the random owner token naming one entry build",
			Key:      KeyOwner, StdioKey: KeyNone,
			Decided: []string{"ADR-0020"},
			Sites: []Site{
				derive(pkgPool, "ServerPool.buildEntry"),
				derive(pkgPool, "Entry.Owner"),
			},
		},
		{
			ID: "IDN-004", Question: Identify, Kind: Rule, Class: ClassA, Disposition: Ruled,
			Resource: "the client address authentication budgets are charged to",
			Key:      KeyAddress, StdioKey: KeyNone,
			Findings: []string{"F-14"},
			Sites:    []Site{derive(pkgServer, "clientIP")},
		},
		{
			ID: "IDN-005", Question: Identify, Kind: Rule, Class: ClassA, Disposition: Ruled,
			Resource: "the socket peer, every header ignored",
			Key:      KeySource, StdioKey: KeyNone,
			Sites: []Site{derive(pkgServer, "transportSource")},
		},
		{
			ID: "IDN-006", Question: Identify, Kind: Rule, Class: ClassR, Disposition: Mechanism,
			Resource: "the binding of a request to its entry's client and state",
			Key:      KeyEntry, StdioKey: KeyNone,
			Decided: []string{"ADR-0020"},
			Sites: []Site{
				enforce(pkgServer, "credentialStates.bindCredential"),
				enforce(pkgServer, "carriedMCPHandler"),
			},
		},
		{
			ID: "IDN-007", Question: Identify, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "the state a request no credential was bound to runs under",
			Key:      KeyUnbound, StdioKey: KeyNone,
			Findings: []string{"F-15"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: ToolError, Answer: RetryLater,
					At: refuse(pkgToolutil, "UnattributedRequestMessage"),
				},
				{
					Methods: []string{"resources/read", "prompts/get"}, Channel: RPC, Code: codeInternalError,
					Prefix: "this request could not be attributed to a credential", Answer: RetryLater,
					At: refuse(pkgToolutil, "UnattributedRequestError"),
				},
				{
					Methods: subscribeMethods(), Channel: RPC, Code: codeInternalError,
					Prefix: "this subscription could not be attributed to a credential", Answer: RetryLater,
					At: refuse(pkgServer, "errUnboundSubscribe"),
				},
			},
			Sites: []Site{
				enforce(pkgServer, "serverShell.defaultCredentialState"),
				enforce(pkgServer, "serverShell.stateFor"),
				refuse(pkgToolutil, "UnattributedRequestMessage"),
				refuse(pkgToolutil, "UnattributedRequestError"),
				refuse(pkgServer, "errUnboundSubscribe"),
			},
		},
		{
			ID: "IDN-008", Question: Identify, Kind: Rule, Class: ClassT, Disposition: Ruled,
			Resource: "the tenant: (canonical instance URL, GitLab user id), or unknown",
			Key:      KeyTenant, StdioKey: KeyProcess,
			Findings: []string{"F-01", "F-28"},
			Sites: []Site{
				enforce(pkgPool, "resolveIdentity"),
				enforce(pkgPool, "ServerPool.IdentityFor"),
				enforce(pkgOAuth, "admitToken"),
				enforce(pkgServer, "prepareStdioCatalog"),
				enforce(pkgServer, "mcpServerGate.withIdentity"),
				enforce(pkgToolutil, "ResolveIdentity"),
			},
		},
		{
			ID: "IDN-010", Question: Identify, Kind: Rule, Class: ClassC, Disposition: Mechanism,
			Resource: "which owner each session belongs to",
			Key:      KeySession, StdioKey: KeyNone, Table: true,
			Decided:  []string{"ADR-0020"},
			Findings: []string{"F-31"},
			Sites:    []Site{enforce(pkgServer, "sessionOwners.record")},
		},
		{
			ID: "ADM-011", Question: Identify, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the GitLab instance a request selects among the published ones",
			Key:      KeyRequest, StdioKey: KeyNone,
			Refusals: []Refusal{
				gateRefusal(400, codeInvalidRequest, severalPrefix, FixRequest, refuse(pkgServer, "mcpServerGate.resolve")),
				gateRefusal(400, codeInvalidRequest, "", FixRequest, refuse(pkgServer, "mcpServerGate.resolve")),
				gateRefusal(400, codeInvalidRequest, severalPrefix, FixRequest, refuse(pkgServer, "bearerGuard.check")),
				gateRefusal(403, CodeForbidden, "This deployment does not serve the GitLab instance", AskOperator,
					refuse(pkgServer, "bearerGuard.check")),
			},
			Sites: []Site{
				enforce(pkgPool, "ResolveRequestOptionsFor"),
				enforce(pkgServer, "requireExplicitInstance"),
				refuse(pkgServer, "mcpServerGate.resolve"),
				refuse(pkgServer, "bearerGuard.check"),
			},
		},
	}
}
