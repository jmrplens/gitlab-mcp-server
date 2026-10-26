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
	rejectedCharges := []string{"AUB-001", "AUB-002", "AUB-003", "AUB-006"}
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
			Fixed: "it is a hint of when to come back and bounds nothing, so no deployment has a figure of its " +
				"own for it; where GitLab says when, GitLab's Retry-After is advertised instead",
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
			// A zero, or less, is refused at startup by the flag and the
			// variable alike, in either authentication mode, which issue 958
			// decided rather than reading it as off: the verifier stamps the
			// TTL into every token's expiration, so a token cached for no time
			// expires as it is verified and every request is refused, and a
			// cache with no lifetime would keep a revoked token's identity for
			// the life of the process. Neither is a deployment an operator
			// asking for zero could want. The flag used to pass a zero through,
			// in oauth mode to the point of use, which replaced it with the
			// default, and in legacy mode unread, while the variable refused it
			// in both (F-34); the replacement stays for a Config no validation
			// saw. The range is checked in oauth mode only, where the TTL is
			// used, through the flag and the variable alike.
			ID: "ADM-005", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how long, and how many, verified OAuth tokens are reused",
			Key:      KeyVerified, StdioKey: KeyNone, Table: true,
			Values: []string{
				"OAuthCacheTTL", "OAuthCacheTTLFloor", "OAuthCacheTTLMax", "OAuthCacheSweepDivisor", "OAuthCacheSweepFloor",
				"OAuthCacheCapacity",
			},
			Source: Configurable, Flags: []string{"--oauth-cache-ttl"}, Envs: []string{"GITLAB_MCP_OAUTH_CACHE_TTL"},
			Config: []string{"OAuthCacheTTL"}, Malformed: RefuseStartup, Zero: ZeroRefused, ZeroBy: "issue 958",
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
			Decided:    []string{"issue 950", "issue 958"},
			Sites: []Site{
				alias(pkgConfig, "DefaultOAuthCacheTTL", "OAuthCacheTTL"),
				alias(pkgConfig, "MinOAuthCacheTTL", "OAuthCacheTTLFloor"),
				alias(pkgConfig, "MaxOAuthCacheTTL", "OAuthCacheTTLMax"),
				alias(pkgServer, "tokenCacheSweepDivisor", "OAuthCacheSweepDivisor"),
				alias(pkgServer, "tokenCacheSweepMinInterval", "OAuthCacheSweepFloor"),
				alias(pkgOAuth, "identityCacheCapacity", "OAuthCacheCapacity"),
				enforce(pkgOAuth, "effectiveCacheTTL"),
				enforce(pkgServer, "refuseNonPositiveOAuthCacheTTL"),
				enforce(pkgServer, "validateOAuthCacheTTL"),
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
			Fixed: "it remembers GitLab's own verdicts to spare GitLab a request: the lifetime is how long a token " +
				"fixed at GitLab stays refused here and the capacity is the memory a flood of invented tokens may " +
				"take, both set against GitLab and the process rather than against a deployment's traffic",
			// Full, it evicts the record nearest its expiry, which the
			// capacity rule allows a table kept before admission (issue 958):
			// a record is GitLab's verdict against a token, so evicting one
			// takes nothing a tenant holds and sends that token to GitLab once
			// more, where the authentication budgets count it. The rule allows
			// refusing the newcomer too, and this row chooses eviction because
			// refusing would let a flood of invented tokens fill the cache and
			// leave every later refusal unremembered, while the newest verdicts
			// are the ones a retrying client presents next.
			AtCapacity: EvictOldest,
			Decided:    []string{"issue 958"},
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
			// One answer whether another credential opened the session or the
			// process no longer holds it (it idled out, was deleted, went with
			// its credential's pool entry, or was opened before a restart or
			// on another replica), by issue 1102's decision: the words used to
			// name the first case alone, which was false for the second. Two
			// answers would tell a caller whether another credential holds the
			// ID (INV-019) and would need a record of each credential's ended
			// session IDs, keyed on a value the caller mints, so the row keeps
			// nothing. The gate's log tells the two cases apart.
			ID: "ADM-007", Question: Admit, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "a session ID that names no session open for the presented credential",
			Key:      KeySession, StdioKey: KeyNone,
			Decided:  []string{"issue 1102"},
			Findings: []string{"F-22"},
			Refusals: []Refusal{
				gateRefusal(404, codeInvalidRequest, "This session is not open for the presented credential.", StartOver,
					refuse(pkgServer, "sessionOwnershipFailure")),
			},
			Sites: []Site{
				enforce(pkgServer, "mcpServerGate.checkSessionOwnership"),
				refuse(pkgServer, "sessionOwnershipFailure"),
			},
		},
		{
			// A zero, or less, passed to the pool's option keeps the default,
			// which issue 958 keeps and states as an exception rather than
			// reading it as off: the ceiling exists so that every credential
			// is re-checked, whatever else an operator turns off
			// (--revalidate-interval 0 among them), so a zero must not switch
			// it off.
			ID: "ADM-008", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "the longest an entry serves on one credential check",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"CredentialMaxAge", "CredentialMaxAgeCeiling"}, Source: OptionOnly, Zero: ZeroSelectsDefault,
			Fixed: "it is the floor under every setting that turns re-checking off, --revalidate-interval 0 among " +
				"them, and bounds how long a revoked token's surface stays served, so a setting that raised it would " +
				"widen the one window nothing else bounds; the option exists for the tests, and no operator reaches it",
			ZeroBy:  "issue 958",
			Decided: []string{"issue 958"},
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
			// On stdio the process's fine-grained token is re-read at the same
			// setting, its grant and the instance version, or its kind while
			// that is unknown, and a zero stops those re-reads as it stops the
			// pool's. Stdio used to parse the variable and then re-read at the
			// default whatever it said, so the row had no stdio key; issue 1171
			// (Q16 of its triage) gave stdio the setting.
			ID: "ADM-009", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often every entry's credential is re-probed, and on stdio the process's fine-grained token re-read",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Values: []string{"RevalidateInterval", "RevalidateIntervalMax"},
			Source: Configurable, Flags: []string{"--revalidate-interval"},
			Envs: []string{"GITLAB_MCP_SESSION_REVALIDATE_INTERVAL"}, Config: []string{"RevalidateInterval"},
			Malformed: RefuseStartup, Zero: ZeroOff,
			// The pool's own fallback aliases the value as the configuration's
			// default does, where it used to state it a second time (issue 958,
			// answering F-16 here).
			Decided: []string{"issue 958", "issue 1171"},
			Refusals: []Refusal{
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultRevalidateInterval", "RevalidateInterval"),
				alias(pkgConfig, "MaxRevalidateInterval", "RevalidateIntervalMax"),
				alias(pkgPool, "DefaultRevalidateInterval", "RevalidateInterval"),
				enforce(pkgPool, "ServerPool.revalidateAll"),
				enforce(pkgServer, "prepareStdioCatalog"),
				enforce(pkgServer, "refreshStdioAuthority"),
				// The flag's range, a negative value refused as the variable
				// refuses it (issue 1171).
				enforce(pkgServer, "validateHTTPDurationConfig"),
			},
		},
		{
			ID: "ADM-010", Question: Admit, Kind: Lifetime, Class: ClassC, Disposition: Valued,
			Resource: "how often a 401 that named no cause is confirmed with a probe",
			Key:      KeyEntry, StdioKey: KeyNone,
			Values: []string{"UnexplainedRefusalCooldown"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "it paces the probe a 401 that named no cause triggers to one per credential per window and must " +
				"outlast that probe, so it is set against the probe's own timeout rather than against a deployment: " +
				"shorter would let two refusals confirm twice, and longer would leave a deleted token served longer",
			Decided: []string{"issue 958"},
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
			Fixed: "a ceiling on the process's own work with no per-caller number beside it, which an operator who " +
				"could raise it could undo (INV-004), as the code beside it says",
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
			// Keyed on IDN-004's address, an IPv6 one by its /64 (issue 954,
			// which answered F-14 and F-25 here and on AUB-003).
			//
			// A zero window switches this budget off and no other, which issue
			// 958 decided (answering F-34): the distinct-credential budget's
			// escalation steps by this window and used to be switched off with
			// it, and steps by the default window instead, as the
			// transport-source budget and the /48 aggregate already counted in
			// it (windowOrDefault).
			ID: "AUB-001", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "failed authentications one address may produce in a window",
			Key:      KeyAddress, StdioKey: KeyNone,
			Values: []string{"AuthFailureLimit", "AuthFailureWindow", "AuthFailureLimitMax", "AuthFailureWindowMax"},
			// Which settings mean off is BudgetOn's answer.
			Functions: []string{"BudgetOn"},
			Source:    Configurable, Flags: []string{"--auth-failure-limit", "--auth-failure-window"},
			Envs:   []string{"GITLAB_MCP_AUTH_FAILURE_LIMIT", "GITLAB_MCP_AUTH_FAILURE_WINDOW"},
			Config: []string{"AuthFailureLimit", "AuthFailureWindow"}, Malformed: RefuseStartup, Zero: ZeroOff,
			Decided:  []string{"issue 790", "issue 954", "issue 958"},
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
			// The source is keyed as the address is, an IPv6 peer by its /64
			// and a unix-socket peer as unix (issue 954), so a proxy that
			// rotates its own low bits is still one source.
			ID: "AUB-002", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct failing primary keys one transport source, an IPv6 one by its /64, may produce in a window",
			Key:      KeySource, StdioKey: KeyNone,
			Decided: []string{"issue 954", "issue 982", "issue 958"},
			// The pairs already charged are remembered in a map keyed on the
			// source and the primary key, both of which a caller mints, in
			// both authentication modes, and issue 982 bounded it twice. A
			// source holds at most the value's worth of pairs, and at that
			// bound a fresh key reclaims the source's own oldest pair and is
			// charged, so nothing is taken across sources (INV-005) and a
			// source released from its block is still charged for the keys
			// it fails with. And a pair is kept only for a failure the
			// limiter recorded, and once the budget indexes as many sources
			// as AUB-004 lets that limiter track, a new source first makes it
			// forget every source with no live pair, at most once an eighth
			// of a window as the limiter sweeps its own table. Past that
			// many, the sources it holds were all charged within one window
			// and an eighth, each while the limiter held a record of it, so
			// they are at most three times what that table holds, whatever
			// AUB-001's window;
			// the sweep (AUB-005) alone let one table's worth stay for every
			// window between two sweeps. A source that arrives once that
			// table is full is given no place, and is not blocked by this
			// budget either: that saturation is left to the /48 aggregate
			// (AUB-006), and it stays under the capacity rule issue 958
			// stated, which allows a full table to stop counting, the behavior
			// AUB-004 chooses rather than evicting a count that blocks
			// somebody. The map used to be bounded by its sweep alone, which
			// F-35 recorded.
			Table:  true,
			Values: []string{"TransportSourceDistinctKeys"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "no legitimate traffic reaches it: five hundred distinct failing identities inside a window from " +
				"one transport source is a rotation or an incident, so a setting could only loosen the bound on a " +
				"proxy that forwards what it is told",
			AtCapacity: ReclaimOwnThenRefuse,
			// Whether the budget exists is TransportSourceBudgetOn's answer.
			Functions: []string{"TransportSourceBudgetOn"},
			Refusals:  blockedRefusals,
			// The window falls back to the default when AUB-001's is zero, in
			// the one helper every budget that reads another's window goes
			// through, called where the budget is built (issue 958, answering
			// F-16 here): it was kept in step by hand in that function and in
			// the budget's constructor. The budget's window reads what was
			// resolved there, and the default only for a budget that does not
			// exist.
			Sites: []Site{
				alias(pkgServer, "transportFailureLimit", "TransportSourceDistinctKeys"),
				enforce(pkgServer, "windowOrDefault"),
				enforce(pkgServer, "transportFailureBudget"),
				enforce(pkgServer, "newTransportBudget"),
				enforce(pkgServer, "transportBudget.window"),
				enforce(pkgServer, "transportBudget.charge"),
				enforce(pkgServer, "transportBudget.keepLocked"),
				enforce(pkgServer, "transportBudget.forgetLapsedSourcesLocked"),
				enforce(pkgServer, "transportBudget.reclaimOldestLocked"),
				resolve, check,
			},
		},
		{
			// The escalation steps by AUB-001's window, or by the default when
			// that window is zero, which switches AUB-001 off and leaves this
			// budget on (issue 958, answering F-34): only its own limit or
			// window turns it off. It used to be switched off with AUB-001.
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
			Zero:     ZeroOff,
			Decided:  []string{"issue 790", "issue 954", "issue 958"},
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
				enforce(pkgServer, "windowOrDefault"),
				resolve, check,
			},
		},
		{
			ID: "AUB-004", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "keys the authentication failure table, the distinct-credential address table and the /48 table each track",
			Key:      KeyProcess, StdioKey: KeyNone,
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"AuthTrackedSources"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "it bounds tables keyed on addresses a caller mints, at a few hundred kilobytes, and the " +
				"transport-source budget sizes its own record against it (AUB-002): raising it lets a flood hold " +
				"more, and lowering it leaves more newcomers uncounted, neither of which a deployment's traffic asks for",
			AtCapacity: StopCounting,
			Decided:    []string{"issue 958"},
			// The one value caps the three tables, and each stops counting a
			// new key once it is full. The capacity rule allows that to a row
			// that declares the silent refusal its uncounted newcomer gets
			// (issue 958), as it allows refusing the newcomer, and this row
			// chooses it: evicting a record would let a flood of fresh
			// addresses push out the counts that block the one it rotates
			// from, and refusing the newcomer would refuse every new address
			// while the flood lasts. What stopping costs is a newcomer past
			// the bound going uncounted until the table has room, which the
			// /48 aggregate (AUB-006) and the transport-source budget (AUB-002)
			// narrow and do not close. The /48 table of AUB-006 is a
			// DistinctTokenBudget of its own, so its refusal is the second
			// below; keying the address on the /64 lowers the keys a caller
			// can mint against all three. The transport-source budget reads
			// it as well (issue 982): once its index of charged sources holds
			// that many, a new source first makes it forget every source with
			// no live pair, which is what bounds AUB-002's pairs across
			// sources.
			Refusals: []Refusal{
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "AuthRateLimiter.RecordFailure")},
				{Methods: []string{MethodGate}, Channel: Silent, Answer: NoAnswer, At: refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked")},
			},
			Sites: []Site{
				alias(pkgPool, "maxTrackedAuthSources", "AuthTrackedSources"),
				alias(pkgServer, "transportTrackedSources", "AuthTrackedSources"),
				enforce(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
				refuse(pkgPool, "AuthRateLimiter.RecordFailure"),
				refuse(pkgPool, "DistinctTokenBudget.roomForNewKeyLocked"),
			},
		},
		{
			// The discovery documents' table (RTC-008) is swept on the same
			// interval, of the buckets that have refilled: the sweep changes no
			// answer there and only gives memory back, so a change of this
			// value made for the authentication tables changes how long a
			// flood's buckets outlive it and nothing else.
			ID: "AUB-005", Question: Admit, Kind: Lifetime, Class: ClassP, Disposition: Valued,
			Resource: "how often the authentication tables and the discovery documents' table are swept",
			Key:      KeyProcess, StdioKey: KeyNone,
			Values: []string{"AuthSweepInterval"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "the sweep gives back the memory of records that have lapsed, which a full table also clears for " +
				"itself before it stops counting, so its interval is housekeeping a deployment has no figure for",
			// A var rather than a const, so a test can make the tick arrive;
			// nothing at runtime writes it.
			Sites: []Site{alias(pkgServer, "periodicCleanupInterval", "AuthSweepInterval")},
		},
		{
			// The aggregate one level above the address (issue 954): an IPv6
			// caller that rotates /64s inside its /48 gets a fresh AUB-001 and
			// AUB-003 per /64, so the /64s of one /48 that had a credential
			// refused are counted, in AUB-003's window and up the ladder
			// AUB-003 climbs, and at the limit the /48 is refused before its
			// credential is read. It counts what AUB-003 counts, a credential
			// GitLab refused, and never a request that carried none, since a
			// network of honest subnets produces that. The /64s are held as
			// digests, in the table AUB-004 caps. Its value is the register's
			// and no operator's (issue 954's decision), so it is on wherever
			// the gate runs and its zero is not one anybody sets; the windows
			// it reads fall back to their defaults when AUB-001 or AUB-003 is
			// switched off by a zero, so turning those off does not turn this
			// off (INV-015). An IPv4 address and a unix-socket peer have no
			// /48 and are not counted.
			ID: "AUB-006", Question: Admit, Kind: Budget, Class: ClassA, Disposition: Valued,
			Resource: "distinct IPv6 /64s with a credential refused one /48 may produce in AUB-003's window",
			Key:      KeyNetwork, StdioKey: KeyNone,
			Values: []string{"AuthNetworkFailingPrefixes"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "issue 954 decided the figure is the register's and no operator's: it bounds the rotation across " +
				"a /48's /64s that keying the address budgets on the /64 opened, and a setting would let a " +
				"deployment reopen it",
			Decided:  []string{"issue 954"},
			Refusals: blockedRefusals,
			Sites: []Site{
				alias(pkgServer, "networkFailingPrefixes", "AuthNetworkFailingPrefixes"),
				enforce(pkgServer, "authNetworkBudget"),
				enforce(pkgServer, "windowOrDefault"),
				resolve, check,
			},
		},
		{
			ID: "POL-006", Question: Admit, Kind: Ceiling, Class: ClassP, Disposition: Valued,
			Resource: "credential probes the pool runs at once",
			Key:      KeyProcess, StdioKey: KeyNone,
			Reason:     "counting work rather than callers",
			ReasonAt:   reasonAt(pkgPool, "maxConcurrentCredentialProbes"),
			ReasonUnit: KeyProcess, ProtectsProcess: true,
			Values: []string{"CredentialProbes", "CredentialProbeWait"}, Source: Constant, Zero: ZeroNotApplicable,
			Fixed: "a ceiling on the probes the process sends GitLab for credentials it has never met, keyed on the " +
				"process with no per-caller number beside it, which an operator who could raise it could undo " +
				"(INV-004), as ADM-014's is for the OAuth verifications",
			AtCapacity: WaitThenRefuse,
			// In oauth mode its probe is the second GET /user; the verifier's
			// round trips before it run under ADM-014's slots (F-30, answered
			// by issue 950).
			Decided: []string{"issue 958"},
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
			// A value the flag package cannot read as a boolean refuses
			// startup, as every flag's does.
			Flags: []string{"--allow-any-gitlab-url"}, Malformed: RefuseStartup,
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
