package tenancy

// admitDecisions are the rows that answer "may this credential enter, and for
// how long?" (spec: The five questions): verification in both modes, the
// caches that remember a verdict, the lifetimes that force a credential to be
// checked again, the Origin and Host guards, the authentication failure
// budgets that run before a tenant exists, and the destination rules at the
// door.
//
// Every gate refusal here is written by the gate or the bearer guard in front
// of the SDK, which is the only place a status other than 400 or 404 can be
// chosen (PAT-004). Which of them charge the failure budgets is [Failures]'s
// to say, one row per return. The admission rows also end the listens of an
// entry they evict or revoke (ADM-008 to ADM-010), refuse to start a
// deployment whose escape hatch is reachable (DST-002), and stop counting a
// new address silently once a tracking table is full (AUB-004).
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: its length is the number of decisions it declares.
func admitDecisions() []Decision {
	resolve := refuse(pkgServer, "mcpServerGate.resolve")
	// The gate's reading of the pool's refusal to build an entry: GitLab's
	// verdict on a credential the gate had not met, or none.
	gateClassify := refuse(pkgServer, "mcpServerGate.classify")
	check := refuse(pkgServer, "bearerGuard.check")
	classify := refuse(pkgServer, "bearerGuard.classify")
	invalidToken := refuse(pkgServer, "bearerGuard.invalidTokenFailure")
	unaccepted := refuse(pkgServer, "bearerGuard.unacceptedRecipientFailure")
	// The two doors' answer to a credential GitLab accepted and refused the
	// permission to read its own user, the legacy gate's with no challenge
	// and the bearer guard's with an insufficient_scope one.
	gatePermission := refuse(pkgServer, "doorPermissionFailure")
	guardPermission := refuse(pkgServer, "bearerGuard.permissionMissingFailure")
	// The two doors' answer to a credential GitLab accepted that carries
	// neither read_api nor api (issue 952), the same pair of shapes.
	gateBelowMinimum := refuse(pkgServer, "belowMinimumFailure")
	guardBelowMinimum := refuse(pkgServer, "bearerGuard.insufficientScopeFailure")
	// Stdio's answer to the same verdict about its one token: every catalog
	// method, in band.
	stdioBelowMinimum := refuse(pkgServer, "stdioBelowMinimumRefusal")
	stdioRefusedMethods := []string{
		"tools/list", "tools/call",
		"resources/list", "resources/templates/list", "resources/read", "resources/subscribe",
		"prompts/list", "prompts/get",
		"completion/complete", "subscriptions/listen",
	}
	belowMinimum := func(at Site, challenge bool, prefix string) Refusal {
		return Refusal{
			Methods: []string{MethodGate}, Channel: Gate, Code: CodeForbidden, Status: 403,
			Challenge: challenge, Prefix: prefix, Answer: WidenScope, At: at,
		}
	}
	rejectedCharges := []string{"AUB-001", "AUB-002", "AUB-003"}
	blocked := func(at Site) Refusal {
		return Refusal{
			Methods: []string{MethodGate}, Channel: Gate, Code: CodeTooManyRequests, Status: 429,
			RetryAfter: RetryAfterLongestBlock, Prefix: blockedPrefix, Answer: RetryLater, At: at,
		}
	}
	blockedRefusals := []Refusal{blocked(resolve), blocked(check)}
	permissionMissing := func(at Site, challenge bool) Refusal {
		return Refusal{
			Methods: []string{MethodGate}, Channel: Gate, Code: CodeForbidden, Status: 403,
			Challenge: challenge, Prefix: permissionPrefix, Answer: WidenScope, At: at,
		}
	}

	return []Decision{
		{
			// A credential GitLab accepted and refused the probe its
			// fine-grained permission, User: Read, is answered 403 and not
			// charged (INV-007): GitLab authenticated the token before it
			// judged the grant, so it is genuine, and why F-17 left this row
			// (issue 952). The challenge-free body quotes GitLab's sentence,
			// filtered to printable ASCII and cut at 512 bytes. The verdict is
			// remembered for the token's instance in ADM-006's structure,
			// since nothing else bounds it: such a request builds no entry and
			// is charged nothing, so the same token is answered from memory
			// for the cache's lifetime rather than holding a probe slot per
			// request. What stays bounded in concurrency alone, by POL-006's
			// slots and never in rate, is a flood of distinct minted tokens,
			// each a genuine credential of a real account.
			//
			// The admission minimum is the one ADR-0018 set at the OAuth door,
			// read_api or api, which issue 952 extended to this door and why
			// F-08 left this row and ADM-002: a token GitLab accepted that
			// carries neither is refused 403 with no challenge and not charged
			// (INV-007), whether the probe said so, answering 403
			// insufficient_scope, or the token's own description did, naming
			// read_user or another scope below it. The description is asked
			// before the tier, so such a token costs no tier request, and the
			// minimum holds under --ignore-scopes. A token whose scopes nothing
			// answered for is admitted, unknown counting as capable (ADR-0018),
			// and an entry built for one that is later found below the minimum
			// ends (ADM-006 remembers it).
			//
			// On stdio the same minimum holds for the process's one token:
			// asked before the tier, under --ignore-scopes too, it makes the
			// process answer every catalog method -40300 in band, naming the
			// minimum and the restart, while the handshake is still answered,
			// so a client is told why rather than seeing a process that died
			// (issue 952). A version refused for want of a scope is enough on
			// its own, since that endpoint takes read_user, ai_features,
			// ai_workflows, api and read_api. A token whose scopes a degraded
			// start could not read is admitted otherwise, and the round that
			// first reads them refuses from then on if they are below it.
			ID: "ADM-001", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "admission of a credential GitLab did not refuse, at the read_api minimum, in legacy HTTP and on stdio",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Decided: []string{"ADR-0018", "issue 952"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges, At: gateClassify,
				},
				gateRefusal(503, CodeUnavailable, "Could not initialize a GitLab session for this token.", RetryLater, gateClassify),
				permissionMissing(gatePermission, false),
				belowMinimum(gateBelowMinimum, false, belowMinimumPrefix),
				{
					Methods: stdioRefusedMethods, Era: EraStdio, Channel: RPC, Code: CodeForbidden,
					Prefix: "GitLab accepted the token this server was started with", Answer: WidenScope, At: stdioBelowMinimum,
				},
			},
			Sites: []Site{
				enforce(pkgPool, "verifyCredential"),
				enforce(pkgPool, "ServerPool.entryConfig"),
				enforce(pkgGitLab, "credentialVerdictFor"),
				enforce(pkgGitLab, "PermissionRefusal"),
				enforce(pkgGitLab, "MeetsMinimum"),
				enforce(pkgGitLab, "BelowMinimum"),
				enforce(pkgServer, "prepareStdioCatalog"),
				enforce(pkgServer, "refreshStdioAuthority"),
				enforce(pkgServer, "readinessGate.markRefused"),
				resolve, gateClassify, gatePermission, gateBelowMinimum, stdioBelowMinimum,
			},
		},
		{
			// The same verdict at the OAuth door, from the verifier's own
			// GET /user, answered 403 with an insufficient_scope challenge
			// whose error_description is this server's constant; GitLab's
			// sentence travels in the body only, filtered and cut as the
			// legacy gate's is, and is never charged. It is cached as ADM-006's
			// RejectionPermissionMissing for the reason ADM-001 gives, which
			// matters more here: the verifier collapses nothing, so one
			// genuine token sent by enough concurrent requests would otherwise
			// hold every ADM-014 slot. A flood of distinct minted tokens is
			// bounded in concurrency alone, by those slots. A fine-grained
			// token meets the read_api minimum (SatisfiesMinimum): its one
			// scope names no authority, which is why F-17 left this row too.
			ID: "ADM-002", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Valued,
			Resource: "admission of an OAuth token at the read_api minimum",
			Key:      KeyVerified, StdioKey: KeyNone,
			Values: []string{"UpstreamRetryAfter"}, Source: Constant, Zero: ZeroNotApplicable,
			// The verification's round trips to GitLab run under ADM-014's
			// slots, which answered F-30 (issue 950). The minimum is the
			// legacy door's too since issue 952 (ADM-001), the predicate
			// shared (MeetsMinimum), which is why F-08 left this row.
			Decided: []string{"ADR-0018", "issue 952"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges,
					At: classify, Via: invalidToken,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeForbidden, Status: 403,
					Challenge: true, Answer: WidenScope, At: check,
				},
				belowMinimum(guardBelowMinimum, true, insufficientScopePrefix),
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterUpstreamOrFixed, Prefix: "GitLab could not verify this token right now;",
					Answer: RetryLater, At: classify,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed, Prefix: "GitLab could not verify this token right now.",
					Answer: RetryLater, At: classify,
				},
				permissionMissing(guardPermission, true),
			},
			Sites: []Site{
				alias(pkgServer, "upstreamRetryAfter", "UpstreamRetryAfter"),
				enforce(pkgOAuth, "NewGitLabVerifierFor"),
				enforce(pkgOAuth, "newGitLabVerifier"),
				enforce(pkgOAuth, "askIdentity"),
				enforce(pkgOAuth, "SatisfiesMinimum"),
				enforce(pkgGitLab, "MeetsMinimum"),
				enforce(pkgGitLab, "PermissionRefusal"),
				classify, invalidToken, check, guardPermission, guardBelowMinimum,
				// The gate behind the guard gives the guard's answer to a
				// credential only the pool could judge: below the minimum where
				// the verifier assumed api, or refused User: Read where the two
				// probes disagreed.
				gateClassify,
			},
		},
		{
			// A 403 carrying insufficient_granular_scope on the token's own
			// description is itself an answer: that route's boundary is the
			// user and names no root namespace, so only a fine-grained token
			// is refused a grant there, and introspection reads it as such a
			// token's one scope without asking /oauth/token/info. Scopes
			// nobody answered for resolve wide, to api, while the tier nobody
			// answered for resolves narrow, to Free (AUT-003); issue 952 kept
			// both and recorded the tier as INV-008's exception, which is why
			// F-09 left this row.
			ID: "ADM-003", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the scopes assumed when introspection cannot answer",
			Key:      KeyVerified, StdioKey: KeyNone,
			Decided: []string{"ADR-0018", "issue 952"},
			Sites:   []Site{enforce(pkgOAuth, "introspectToken"), enforce(pkgOAuth, "fetchIntrospection")},
		},
		{
			// A token GitLab refused the permission to read its own user is
			// a personal access token, the only kind GitLab judges a
			// fine-grained grant of, so a pinned deployment answers it with
			// this row's refusal (pinnedRefusalOf) rather than ADM-002's,
			// whose advice names two credentials the pin refuses.
			ID: "ADM-004", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the OAuth applications whose tokens are admitted",
			Key:      KeyApplication, StdioKey: KeyNone,
			Source: Configurable, Flags: []string{"--oauth-client-uid"}, Envs: []string{"GITLAB_MCP_OAUTH_CLIENT_UID"},
			Config: []string{"OAuthClientUIDs"}, Malformed: AcceptsAny,
			Decided: []string{"ADR-0019"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: recipientText, Answer: Reauthorize, At: unaccepted,
				},
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed,
					Prefix:     "This deployment admits only tokens issued to specific OAuth applications",
					Answer:     RetryLater, At: classify,
				},
			},
			Sites: []Site{enforce(pkgOAuth, "acceptedRecipient"), enforce(pkgOAuth, "pinnedRefusalOf"), unaccepted, classify},
		},
		{
			ID: "ADM-005", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how long, and how many, verified OAuth tokens are reused",
			Key:      KeyVerified, StdioKey: KeyNone, Table: true,
			Values: []string{
				"OAuthCacheTTL", "OAuthCacheTTLFloor", "OAuthCacheTTLMax", "OAuthCacheSweepDivisor", "OAuthCacheSweepFloor",
				"OAuthCacheCapacity",
			},
			Source: Configurable, Flags: []string{"--oauth-cache-ttl"}, Envs: []string{"GITLAB_MCP_OAUTH_CACHE_TTL"},
			Config: []string{"OAuthCacheTTL"}, Malformed: RefuseStartup, Zero: ZeroSelectsDefault,
			// Full, the cache drops an expired identity or, when none has
			// expired, the one used least recently, to hold the one GitLab has
			// just verified, which issue 950 decided (answering F-29): refusing
			// the newcomer would refuse a credential GitLab accepted. The
			// identity taken costs its credential one more verification, which
			// ADM-014 bounds, and while its address is blocked the exemption a
			// cached identity gives it; the capacity is a constant, the largest
			// pool an operator may configure (POL-001's PoolSizeMax, held equal
			// by the values test).
			AtCapacity: EvictAcrossKeys,
			Decided:    []string{"issue 950"},
			Findings:   []string{"F-34"},
			Sites: []Site{
				alias(pkgConfig, "DefaultOAuthCacheTTL", "OAuthCacheTTL"),
				alias(pkgConfig, "MinOAuthCacheTTL", "OAuthCacheTTLFloor"),
				alias(pkgConfig, "MaxOAuthCacheTTL", "OAuthCacheTTLMax"),
				alias(pkgServer, "tokenCacheSweepDivisor", "OAuthCacheSweepDivisor"),
				alias(pkgServer, "tokenCacheSweepMinInterval", "OAuthCacheSweepFloor"),
				alias(pkgOAuth, "identityCacheCapacity", "OAuthCacheCapacity"),
				enforce(pkgOAuth, "effectiveCacheTTL"),
				enforce(pkgServer, "oauthCacheTTL"),
				enforce(pkgOAuth, "NewTokenCache"),
				enforce(pkgOAuth, "TokenCache.Put"),
				enforce(pkgOAuth, "TokenCache.Get"),
			},
		},
		{
			// It also remembers a token GitLab accepted and refused the
			// permission to read its own user (RejectionPermissionMissing),
			// recorded only when GET /user answered 403 with
			// insufficient_granular_scope: by the bearer guard in OAuth mode,
			// and by the legacy gate in a structure of this same shape, sized
			// by the same two values. A refusal served from here is the one the
			// round trip gave, uncharged as the fresh one is (issue 952),
			// because nothing at GitLab 19.4 edits a grant after its token is
			// created; the TTL bounds how long a feature flag an administrator
			// turns on for the user stays unseen.
			//
			// And it remembers a token GitLab accepted that carries neither
			// read_api nor api (RejectionBelowMinimum, issue 952): recorded by
			// the gate when the pool refuses it (on ADM-001's refusal in legacy
			// mode, on ADM-002's behind the bearer guard), by the bearer guard
			// on GET /user's 403 insufficient_scope, and by the pool, through
			// the record the gate hands it, when it learns that of a credential
			// it already serves and ends its entry. Served uncharged, in legacy
			// mode in the gate's words and in OAuth mode in the guard's, which
			// is the gate's answer there too, since a token's scopes cannot
			// change after it is created: GitLab has no route that edits a
			// personal access token's scopes, and an OAuth token granted more
			// is a new token.
			ID: "ADM-006", Question: Admit, Kind: Lifetime, Class: ClassA, Disposition: Valued,
			Resource: "how long, and how many, rejected tokens are remembered",
			Key:      KeyRefused, StdioKey: KeyNone, Table: true,
			Values: []string{"RejectedTokenTTL", "RejectedTokenCapacity"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: EvictOldest,
			Findings:   []string{"F-11", "F-16"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnauthorized, Status: 401,
					Challenge: true, Prefix: rejectedPrefix, Answer: Reauthorize, Charged: rejectedCharges,
					At: check, Via: invalidToken,
				},
				permissionMissing(gatePermission, false),
				permissionMissing(guardPermission, true),
				belowMinimum(gateBelowMinimum, false, belowMinimumPrefix),
				belowMinimum(guardBelowMinimum, true, insufficientScopePrefix),
			},
			Sites: []Site{
				alias(pkgServer, "rejectedTokenTTL", "RejectedTokenTTL"),
				alias(pkgServer, "rejectedTokenMaxSize", "RejectedTokenCapacity"),
				enforce(pkgServer, "registerOAuthMCPHandlers"),
				enforce(pkgServer, "registerLegacyMCPHandlers"),
				enforce(pkgOAuth, "RejectedTokens.RecordPermissionMissing"),
				enforce(pkgOAuth, "RejectedTokens.RecordBelowMinimum"),
				enforce(pkgOAuth, "RejectedTokens.LookupRefusal"),
				enforce(pkgPool, "ServerPool.RememberBelowMinimum"),
				enforce(pkgPool, "ServerPool.evictBelowMinimum"),
				check, invalidToken, resolve, gateClassify, classify, gatePermission, guardPermission, gateBelowMinimum, guardBelowMinimum,
			},
		},
		{
			ID: "ADM-007", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "a session presented by a credential other than its owner's",
			Key:      KeySession, StdioKey: KeyNone,
			Findings: []string{"F-22"},
			Refusals: []Refusal{
				gateRefusal(404, codeInvalidRequest, "This session does not belong to the presented credential.", StartOver,
					refuse(pkgServer, "sessionOwnershipFailure")),
			},
			Sites: []Site{
				enforce(pkgServer, "mcpServerGate.checkSessionOwnership"),
				refuse(pkgServer, "sessionOwnershipFailure"),
			},
		},
		{
			ID: "ADM-008", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "the longest an entry serves on one credential check",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"CredentialMaxAge", "CredentialMaxAgeCeiling"}, Source: OptionOnly, Zero: ZeroSelectsDefault,
			Findings: []string{"F-16", "F-34"},
			// An entry past the age is evicted on its next request, and its
			// open listens are told the credential was reset.
			Refusals: []Refusal{
				listenEnd("credential_reset", StartOver, refuse(pkgServer, "endOfCredentialReset")),
			},
			Sites: []Site{
				alias(pkgPool, "DefaultMaxCredentialAge", "CredentialMaxAge"),
				alias(pkgPool, "maxCredentialAgeCeiling", "CredentialMaxAgeCeiling"),
				enforce(pkgPool, "WithMaxCredentialAge"),
				enforce(pkgPool, "ServerPool.evictStaleCredential"),
			},
		},
		{
			ID: "ADM-009", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often every entry's credential is re-probed",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"RevalidateInterval", "RevalidateIntervalMax"},
			Source: Configurable, Flags: []string{"--revalidate-interval"},
			Envs: []string{"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL"}, Config: []string{"RevalidateInterval"},
			Malformed: RefuseStartup, Zero: ZeroOff,
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultRevalidateInterval", "RevalidateInterval"),
				alias(pkgConfig, "MaxRevalidateInterval", "RevalidateIntervalMax"),
				pin(pkgPool, "DefaultRevalidateInterval", "RevalidateInterval"),
				enforce(pkgPool, "ServerPool.revalidateAll"),
			},
		},
		{
			ID: "ADM-010", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often a 401 that named no cause is confirmed with a probe",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"UnexplainedRefusalCooldown"}, Source: Constant, Zero: ZeroNotApplicable,
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
			},
			Sites: []Site{
				alias(pkgPool, "unauthorizedConfirmCooldown", "UnexplainedRefusalCooldown"),
				enforce(pkgPool, "ServerPool.handleUnauthorized"),
			},
		},
		{
			ID: "ADM-012", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the origins a browser may reach the server from",
			Key:      KeyRequest, StdioKey: KeyNone,
			Source: Configurable, Flags: []string{"--trusted-origins"}, Envs: []string{"GITLAB_MCP_TRUSTED_ORIGINS"},
			Config: []string{"TrustedOrigins"}, Malformed: RefuseStartup,
			Refusals: []Refusal{
				gateRefusal(403, CodeForbidden, "Cross-origin request refused:", AskOperator,
					refuse(pkgServer, "writeUntrustedOrigin")),
			},
			Sites: []Site{
				enforce(pkgServer, "mcpOriginMiddleware"),
				enforce(pkgServer, "crossOriginProtectionMiddleware"),
				enforce(pkgServer, "buildTrustedOrigins"),
				refuse(pkgServer, "writeUntrustedOrigin"),
			},
		},
		{
			ID: "ADM-013", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the Host names the deployment answers",
			Key:      KeyRequest, StdioKey: KeyNone,
			Refusals: []Refusal{
				gateRefusal(403, CodeForbidden, "Request refused: the Host header names a host", AskOperator,
					refuse(pkgServer, "hostValidationMiddleware")),
			},
			Sites: []Site{
				enforce(pkgServer, "newHostGuard"),
				enforce(pkgServer, "hostGuard.permits"),
				enforce(pkgServer, "allowedHosts"),
				refuse(pkgServer, "hostValidationMiddleware"),
			},
		},
		{
			// The verifier's own slots, not POL-006's, so neither kind of work
			// takes the other's slots (issue 950, answering F-30).
			//
			// Its refusal is ADM-002's for a verification that produced no
			// verdict, word for word and from the same return: the next action
			// is the same, and a sentence of its own would tell a caller that
			// others are verifying. A caller refused after the slot wait while
			// it knows the instance to be healthy can infer that much from being
			// refused at all, which is the one bit INV-019 accepts for a bound
			// keyed on the process; the wording adds nothing to it. The log line
			// it writes is where an operator tells the two apart.
			ID: "ADM-014", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "OAuth token verifications the process runs at once",
			Key:      KeyProcess, StdioKey: KeyNone,
			Reason:     "counting work rather than callers",
			ReasonAt:   reasonAt(pkgOAuth, "verificationSlots"),
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"OAuthVerifications", "OAuthVerificationWait"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: WaitThenRefuse,
			Decided:    []string{"issue 950"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodGate}, Channel: Gate, Code: CodeUnavailable, Status: 503,
					RetryAfter: RetryAfterFixed, Prefix: "GitLab could not verify this token right now.",
					Answer: RetryLater, At: classify,
				},
			},
			Sites: []Site{
				alias(pkgOAuth, "verificationSlots", "OAuthVerifications"),
				alias(pkgOAuth, "verificationWait", "OAuthVerificationWait"),
				enforce(pkgOAuth, "NewGitLabVerifierFor"),
				enforce(pkgOAuth, "newGitLabVerifier"),
				enforce(pkgOAuth, "verificationGate.acquire"),
				classify,
			},
		},
		{
			ID: "AUB-001", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "failed authentications one address may produce in a window",
			Key:      KeyAddress, StdioKey: KeyNone,
			Values: []string{"AuthFailureLimit", "AuthFailureWindow", "AuthFailureLimitMax", "AuthFailureWindowMax"},
			// Which settings mean off is BudgetOn's answer.
			Functions: []string{"BudgetOn"},
			Source:    Configurable, Flags: []string{"--auth-failure-limit", "--auth-failure-window"},
			Envs:   []string{"GITLAB_MCP_AUTH_FAILURE_LIMIT", "GITLAB_MCP_AUTH_FAILURE_WINDOW"},
			Config: []string{"AuthFailureLimit", "AuthFailureWindow"}, Malformed: RefuseStartup, Zero: ZeroOff,
			Decided:  []string{"issue 790"},
			Findings: []string{"F-14", "F-25", "F-34"},
			Refusals: blockedRefusals,
			// The gate's own codes are declared here, with the budget row whose
			// layer moves them: they mirror the statuses every gate refusal
			// above and in the admission rows carries.
			Sites: []Site{
				alias(pkgConfig, "DefaultAuthFailureLimit", "AuthFailureLimit"),
				alias(pkgConfig, "DefaultAuthFailureWindow", "AuthFailureWindow"),
				alias(pkgConfig, "MaxAuthFailureLimit", "AuthFailureLimitMax"),
				alias(pkgConfig, "MaxAuthFailureWindow", "AuthFailureWindowMax"),
				alias(pkgServer, "authFailureWindow", "AuthFailureWindow"),
				alias(pkgServer, "errCodeUnauthorized", "CodeUnauthorized"),
				alias(pkgServer, "errCodeForbidden", "CodeForbidden"),
				alias(pkgServer, "errCodeTooManyRequests", "CodeTooManyRequests"),
				alias(pkgServer, "errCodeUpstreamUnavailable", "CodeUnavailable"),
				enforce(pkgServer, "authFailureLimiter"),
				resolve, check,
			},
		},
		{
			ID: "AUB-002", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct failing primary keys one transport source may produce in a window",
			Key:      KeySource, StdioKey: KeyNone,
			// The pairs already charged are remembered in a map keyed on the
			// source and the primary key, both of which a caller mints. A
			// source stops adding pairs once it is blocked, but a source that
			// arrives after the limiter's table is full is never blocked, so
			// nothing but the sweep bounds the map, which F-35 records
			// (issue 982). It is kept in both authentication modes, so it is
			// not F-29's, whose issue is about OAuth verification.
			Table:  true,
			Values: []string{"TransportSourceDistinctKeys"}, Source: Constant, Zero: ZeroNotApplicable,
			// Whether the budget exists is TransportSourceBudgetOn's answer.
			Functions: []string{"TransportSourceBudgetOn"},
			Findings:  []string{"F-16", "F-35"},
			Refusals:  blockedRefusals,
			// The window falls back to the default when AUB-001's is zero, in
			// three places kept in step by hand (issue 958). It is declared by
			// all three rather than moved to one.
			Sites: []Site{
				alias(pkgServer, "transportFailureLimit", "TransportSourceDistinctKeys"),
				enforce(pkgServer, "transportFailureBudget"),
				enforce(pkgServer, "newTransportBudget"),
				enforce(pkgServer, "transportBudget.window"),
				enforce(pkgServer, "transportBudget.charge"),
				resolve, check,
			},
		},
		{
			ID: "AUB-003", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct refused credentials one address may have in a window",
			Key:      KeyAddress, StdioKey: KeyNone,
			Values: []string{
				"AuthDistinctTokenLimit", "AuthDistinctTokenWindow", "AuthDistinctTokenLimitMax",
				"AuthDistinctTokenWindowMax", "AuthEscalationFirst", "AuthEscalationSecond", "AuthEscalationThird",
			},
			// Which settings mean off, the escalation step among them, is
			// EscalationOn's answer.
			Functions: []string{"EscalationOn"},
			Source:    Configurable, Flags: []string{"--auth-distinct-token-limit", "--auth-distinct-token-window"},
			Envs:   []string{"GITLAB_MCP_AUTH_DISTINCT_TOKEN_LIMIT", "GITLAB_MCP_AUTH_DISTINCT_TOKEN_WINDOW"},
			Config: []string{"AuthDistinctTokenLimit", "AuthDistinctWindow"}, Malformed: RefuseStartup,
			Zero: ZeroOff, OffWith: "AUB-001",
			Decided:  []string{"issue 790"},
			Findings: []string{"F-14", "F-25", "F-34"},
			Refusals: blockedRefusals,
			Sites: []Site{
				alias(pkgConfig, "DefaultAuthDistinctTokenLimit", "AuthDistinctTokenLimit"),
				alias(pkgConfig, "DefaultAuthDistinctWindow", "AuthDistinctTokenWindow"),
				alias(pkgConfig, "MaxAuthDistinctTokenLimit", "AuthDistinctTokenLimitMax"),
				alias(pkgConfig, "MaxAuthDistinctWindow", "AuthDistinctTokenWindowMax"),
				ladderStep(0, "AuthEscalationFirst"),
				ladderStep(1, "AuthEscalationSecond"),
				ladderStep(2, "AuthEscalationThird"),
				enforce(pkgPool, "NewDistinctTokenBudget"),
				enforce(pkgServer, "authSprayBudget"),
				resolve, check,
			},
		},
		{
			ID: "AUB-004", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "addresses the authentication failure table and the distinct-credential address table each track",
			Key:      KeyProcess, StdioKey: KeyNone,
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"AuthTrackedSources"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: StopCounting,
			Findings:   []string{"F-11"},
			// The one value caps both tables, and each stops counting a new
			// address once it is full.
			Refusals: []Refusal{
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "AuthRateLimiter.RecordFailure")},
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked")},
			},
			Sites: []Site{
				alias(pkgPool, "maxTrackedAuthSources", "AuthTrackedSources"),
				enforce(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
				refuse(pkgPool, "AuthRateLimiter.RecordFailure"),
				refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
			},
		},
		{
			ID: "AUB-005", Question: Admit, Kind: Lifetime, Class: ClassP, Disposition: Valued,
			Resource: "how often the authentication tables are swept",
			Key:      KeyProcess, StdioKey: KeyNone,
			Values: []string{"AuthSweepInterval"}, Source: Constant, Zero: ZeroNotApplicable,
			// A var rather than a const, so a test can make the tick arrive;
			// nothing at runtime writes it.
			Sites: []Site{alias(pkgServer, "periodicCleanupInterval", "AuthSweepInterval")},
		},
		{
			ID: "POL-006", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "credential probes the pool runs at once",
			Key:      KeyProcess, StdioKey: KeyNone,
			Reason:     "counting work rather than callers",
			ReasonAt:   reasonAt(pkgPool, "maxConcurrentCredentialProbes"),
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"CredentialProbes", "CredentialProbeWait"}, Source: Constant, Zero: ZeroNotApplicable,
			AtCapacity: WaitThenRefuse,
			// In oauth mode its probe is the second GET /user; the verifier's
			// round trips before it run under ADM-014's slots (F-30, answered
			// by issue 950).
			Findings: []string{"F-16"},
			Refusals: []Refusal{
				gateRefusal(503, CodeUnavailable, "Could not initialize a GitLab session for this token.", RetryLater, gateClassify),
			},
			Sites: []Site{
				alias(pkgPool, "maxConcurrentCredentialProbes", "CredentialProbes"),
				alias(pkgPool, "credentialProbeQueueTimeout", "CredentialProbeWait"),
				enforce(pkgPool, "New"),
				gateClassify,
			},
		},
		{
			ID: "POL-009", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Mechanism,
			Resource: "an entry marked rejected, rebuilt rather than served",
			Key:      KeyEntry, StdioKey: KeyNone,
			Sites: []Site{enforce(pkgPool, "ServerPool.dropRejectedEntry")},
		},
		{
			ID: "DST-001", Question: Admit, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "a caller-named instance the dialer would refuse, refused at the door",
			Key:      KeyRequest, StdioKey: KeyNone,
			Decided: []string{"ADR-0022"},
			Refusals: []Refusal{
				gateRefusal(400, codeInvalidRequest, "", AskOperator, resolve),
			},
			Sites: []Site{enforce(pkgGitLab, "CheckCallerNamedInstance"), resolve},
		},
		{
			ID: "DST-002", Question: Admit, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "the escape hatch that lets a caller name any instance",
			Key:      KeyDeployment, StdioKey: KeyNone,
			Flags:   []string{"--allow-any-gitlab-url"},
			Decided: []string{"ADR-0022"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodStartup}, Channel: Startup,
					Prefix: "--allow-any-gitlab-url names no instance and --http-addr ", Answer: AskOperator,
					At: refuse(pkgServer, "requireInstanceAllowList"),
				},
			},
			Sites: []Site{
				enforce(pkgServer, "requireInstanceAllowList"),
				enforce(pkgServer, "listenerIsHostLocal"),
			},
		},
	}
}

// ladderStep is one element of the distinct-credential budget's escalation
// ladder, a composite literal whose every element aliases a register constant.
func ladderStep(index int, reads string) Site {
	return Site{Pkg: pkgPool, Name: "escalationLadder", Role: Alias, Reads: reads, Arg: index}
}
