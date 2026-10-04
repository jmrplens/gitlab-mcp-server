package tenancy

// authorizeDecisions are the rows that answer "what may it do?" (spec:
// The five questions): the surface a credential's scopes, the instance's tier
// and the operator's configuration leave, the local files a stdio process may
// reach, the destinations a client may dial, the response profile and cache
// hints a session is given, which subscriptions may be made at all, and what a
// fine-grained token is withheld (AUT-007).
//
// Authority is the credential's own, which is why most of these are class C:
// two credentials of one tenant may carry different scopes and so be served
// different surfaces. A narrowed surface says why wherever it can (Withheld),
// and a tier narrowing does not yet (F-10, issue 956).
//
//nolint:maintidx // one table of data, cyclomatic complexity 1: its length is the number of decisions it declares.
func authorizeDecisions() []Decision {
	withheld := refuse(pkgDynamic, "Registry.withheldActionMessage")
	filter := refuse(pkgTools, "FilterActionCatalog")
	narrowed := func(answer Answer) []Refusal {
		return []Refusal{
			{Methods: []string{"tools/call"}, Channel: Withheld, Answer: answer, At: withheld},
			{Methods: []string{"tools/list"}, Channel: Absent, Answer: answer, At: filter},
		}
	}
	notSubscribable := refuse(pkgSubscriptions, "ErrNotSubscribable")
	inaccessible := refuse(pkgSubscriptions, "ErrInaccessible")
	wire := refuse(pkgServer, "wireSubscribeError")

	return []Decision{
		{
			// A fine-grained token is the exception, and why F-17 left this row:
			// its scope list is the single value granular, which says nothing
			// about what it may do, so it is unknown authority (INV-008) and
			// never narrowed to the read-only surface by that list; the
			// operator's read-only (AUT-004) still applies to it. GitLab judges
			// its writes per call against the permissions it was granted
			// (issue 952, ADR-0024).
			ID: "AUT-001", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the read-only surface a credential without the api scope is served, a fine-grained token excepted",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Decided:  []string{"ADR-0018", "ADR-0024", "issue 952"},
			Refusals: narrowed(WidenScope),
			Sites: []Site{
				enforce(pkgGitLab, "FineGrained"),
				enforce(pkgGitLab, "WriteCapable"),
				enforce(pkgGitLab, "NarrowToTokenScope"),
				enforce(pkgTools, "FilterActionCatalog"),
				withheld,
			},
		},
		{
			// A fine-grained token's list is read as unknown here too
			// (CatalogScopes), by the filter and by the key that names its
			// catalog alike, so no group is removed on the strength of a legacy
			// scope such a token cannot carry.
			ID: "AUT-002", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the catalog groups a credential without admin_mode loses whole",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Refusals: narrowed(WidenScope),
			Sites: []Site{
				enforce(pkgGitLab, "CatalogScopes"),
				enforce(pkgTools, "FilterScopeFilteredCatalog"),
				withheld,
			},
		},
		{
			ID: "AUT-003", Question: Authorize, Kind: Rule, Class: ClassE, Disposition: Valued,
			Resource: "the licensing tier, and the namespace pages read to detect it",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Values: []string{"TierNamespacePageSize", "TierNamespaceMaxPages"}, Source: Constant, Zero: ZeroNotApplicable,
			Flags: []string{"--tier"}, Envs: []string{"GITLAB_MCP_TIER"}, Malformed: RefuseStartup,
			Findings: []string{"F-09", "F-10"},
			// The tier filter runs before the withheld lists exist, so an action
			// it removed is answered as unknown rather than withheld.
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: Unknown, Answer: NoAnswer,
					At: refuse(pkgDynamic, "Registry.unknownActionMessage"),
				},
				{
					Methods: []string{"tools/list"}, Channel: Absent, Answer: NoAnswer,
					At: refuse(pkgTools, "filterActionSpecGroupsByTier"),
				},
			},
			Sites: []Site{
				alias(pkgGitLab, "namespacePlanPageSize", "TierNamespacePageSize"),
				alias(pkgGitLab, "namespacePlanMaxPages", "TierNamespaceMaxPages"),
				enforce(pkgGitLab, "Client.DetectTier"),
				refuse(pkgDynamic, "Registry.unknownActionMessage"),
				refuse(pkgTools, "filterActionSpecGroupsByTier"),
			},
		},
		{
			ID: "AUT-004", Question: Authorize, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "the operator's exclusions, read-only mode and safe-mode previews",
			Key:      KeyDeployment, StdioKey: KeyDeployment,
			Source: Configurable, Flags: []string{"--read-only", "--safe-mode", "--exclude-tools"},
			Envs:   []string{"GITLAB_MCP_READ_ONLY", "GITLAB_MCP_SAFE_MODE", "GITLAB_MCP_EXCLUDE_TOOLS"},
			Config: []string{"ReadOnly", "SafeMode", "ExcludeTools"}, Malformed: RefuseStartup,
			Refusals: narrowed(AskOperator),
			Sites: []Site{
				enforce(pkgTools, "FilterActionCatalog"),
				enforce(pkgVisibility, "Apply"),
				withheld,
			},
		},
		{
			// AUTOPILOT sets it too, as the alias IsYOLOMode consults only when
			// GITLAB_MCP_YOLO_MODE is unset. It is left out of Envs on purpose:
			// it is a convention other agent tooling sets, one of the three
			// groups deliberately kept bare, and Envs holds the variables this
			// project defines, which carry the prefix and are read through
			// internal/config (INV-017, G14).
			ID: "AUT-005", Question: Authorize, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "whether a destructive action asks for confirmation",
			Key:      KeyProcess, StdioKey: KeyProcess,
			Source: Configurable, Flags: []string{"--yolo-mode"}, Envs: []string{"GITLAB_MCP_YOLO_MODE"},
			Malformed: AcceptsAny,
			Sites:     []Site{enforce(pkgToolutil, "IsYOLOMode")},
		},
		{
			ID: "AUT-006", Question: Authorize, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "the local directories a stdio tool may read from or write to",
			Key:      KeyProcess, StdioKey: KeyProcess,
			Source: EnvOnly,
			Envs: []string{
				"GITLAB_MCP_ALLOWED_UPLOAD_DIRS", "GITLAB_MCP_ALLOWED_DOWNLOAD_DIRS", "GITLAB_MCP_ALLOWED_IMPORT_DIRS",
			},
			Malformed: WarnKeepDefault,
			Findings:  []string{"F-13"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: ToolError, Answer: AskOperator,
					At: refuse(pkgToolutil, "outsideAllowedDirsError"),
				},
			},
			Sites: []Site{
				enforce(pkgToolutil, "allowedLocalDirs"),
				enforce(pkgToolutil, "UploadDirAllowlistEnv"),
				enforce(pkgToolutil, "DownloadDirAllowlistEnv"),
				enforce(pkgToolutil, "ImportArchiveAllowlistEnv"),
				refuse(pkgToolutil, "outsideAllowedDirsError"),
			},
		},
		{
			// Phase A of issue 952: the actions no fine-grained token can reach
			// at the GitLab version the table was recorded from, because the
			// GraphQL types or mutations they read declare no fine-grained
			// permission there, are withheld from a fine-grained session with
			// the reason, the version and the way out, never answered as
			// unknown. The authority is computed per pool entry (per process on
			// stdio) and carried by the entry's client, which the pool already
			// bounds and evicts, so the row has no capacity of its own and
			// keys nothing on the credential: the shape, catalog and manifest
			// keys stay what they were (INV-010), the listing varies with the
			// authorization on the request alone (INV-009), and a withheld call
			// charges no failure budget (INV-007) while it spends its token of
			// the credential's rate bucket like every other refused call. An
			// action the table has no row for is unknown authority and served
			// (INV-008). The refusal the call middleware makes of a registered
			// tool's call is a tool result built before the SDK's dispatcher,
			// which labels only what it answers, so the middleware gives it
			// the resultType its revision requires (toolutil.LabelForRevision,
			// upstream-bugs row 66), as the rate limiter does its own, and the
			// row carries no F-20; the one dynamic execute makes in its
			// handler is labeled by the dispatcher like any served call.
			ID: "AUT-007", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the actions no fine-grained token can reach at the GitLab release the table was recorded from",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Decided: []string{"ADR-0024", "issue 952"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: Withheld, Answer: WidenScope,
					Prefix: "exists but is not available to a fine-grained personal access token",
					At:     refuse(pkgFinegrained, "Authority.WithheldText"),
				},
				{
					Methods: []string{"tools/list"}, Channel: Absent, Answer: WidenScope,
					At: refuse(pkgVisibility, "ToolActions.Filter"),
				},
			},
			Sites: []Site{
				enforce(pkgFinegrained, "Authority.Decide"),
				enforce(pkgActiongrants, "Build"),
				enforce(pkgVisibility, "CallMiddleware"),
				enforce(pkgToolutil, "FineGrainedRefusal"),
				enforce(pkgVisibility, "ListingMiddleware"),
				refuse(pkgFinegrained, "Authority.WithheldText"),
				refuse(pkgVisibility, "ToolActions.Filter"),
			},
		},
		{
			ID: "POL-007", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			// The scope half is class C, the tier half class E (spec:
			// Alignment classes); the row carries the stricter of the two.
			Resource: "tier and scopes, detected per entry with its own credential",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Sites: []Site{enforce(pkgPool, "ServerPool.entryConfig")},
		},
		{
			ID: "DST-003", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the destinations a client may dial",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Source: Configurable, Flags: []string{"--allow-private-instances"},
			Envs:      []string{"GITLAB_MCP_ALLOW_PRIVATE_INSTANCES"},
			Malformed: AcceptsAny,
			Decided:   []string{"ADR-0022"},
			Findings:  []string{"F-13"},
			Refusals: []Refusal{
				{
					Methods: []string{"tools/call"}, Channel: ToolError,
					Prefix: "this server refused to connect to that address", Answer: AskOperator,
					At: refuse(pkgToolutil, "DestinationRefusedMessage"),
				},
			},
			Sites: []Site{
				enforce(pkgGitLab, "destinationPolicy.coversInstance"),
				refuse(pkgToolutil, "DestinationRefusedMessage"),
			},
		},
		{
			ID: "IDN-012", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the cache scope and lifetime of a result",
			Key:      KeyCredential, StdioKey: KeyProcess,
			// The three lifetimes are declared for existence and not moved:
			// they say how long a client may keep a surface narrowed or widened
			// for an authorization that has since changed.
			Sites: []Site{
				enforce(pkgCacheHints, "applyHints"),
				enforce(pkgCacheHints, "Middleware"),
				enforce(pkgCacheHints, "staticListTTLMs"),
				enforce(pkgCacheHints, "detectedListTTLMs"),
				enforce(pkgCacheHints, "staticReadTTLMs"),
			},
		},
		{
			ID: "IDN-013", Question: Authorize, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the response profile chosen from a session's self-reported clientInfo",
			Key:      KeySession, StdioKey: KeyProcess,
			Source: Configurable, Flags: []string{"--client-compat"}, Envs: []string{"GITLAB_MCP_CLIENT_COMPAT"},
			Malformed: AcceptsAny,
			// Choosing a response from the self-reported clientInfo goes
			// against MCP 2026-07-28, which says implementations SHOULD NOT
			// use it "to change the behavior of the client or server" (F-33).
			// Issue 959 keeps it as a deliberate deviation: it changes only
			// how a priority is written, never who a caller is or what it may
			// do (INV-001), GITLAB_MCP_CLIENT_COMPAT=off removes it, and it
			// retires with the Codex defect it works around (row 17 of
			// docs/development/upstream-bugs.md). Keyed on the session, it
			// reaches only a session that knows its client: a request at
			// 2025-11-25 or earlier over stateless HTTP belongs to a session
			// that never saw initialize, so the profile does not apply to it.
			Decided: []string{"issue 959"},
			Sites: []Site{
				enforce(pkgClientCompat, "profileFromClientInfo"),
				enforce(pkgClientCompat, "profileForRequest"),
				enforce(pkgClientCompat, "Middleware"),
				enforce(pkgClientCompat, "Enabled"),
			},
		},
		{
			ID: "HLD-005", Question: Authorize, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "the resource kinds that may be subscribed",
			Key:      KeyRequest, StdioKey: KeyRequest,
			Refusals: []Refusal{
				{
					Methods: subscribeMethods(), Channel: RPC, Code: codeInvalidParams,
					Prefix: "subscriptions: resource is not subscribable", Answer: FixRequest,
					At: notSubscribable, Via: wire,
				},
			},
			Sites: []Site{enforce(pkgSubscriptions, "Classify"), notSubscribable, wire},
		},
		{
			ID: "HLD-006", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the first read of a watch, made with the subscriber's own client",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Refusals: []Refusal{
				{
					Methods: subscribeMethods(), Channel: RPC, Code: codeInvalidParams,
					Prefix: "subscriptions: resource inaccessible", Answer: FixRequest,
					At: inaccessible, Via: wire,
				},
			},
			Sites: []Site{enforce(pkgSubscriptions, "Manager.Subscribe"), inaccessible, wire},
		},
		{
			ID: "HLD-008", Question: Authorize, Kind: Rule, Class: ClassQ, Disposition: Ruled,
			Resource: "a session-era subscribe on the stateless transport",
			Key:      KeyRequest, StdioKey: KeyNone,
			Refusals: []Refusal{
				{
					Methods: []string{"resources/subscribe"}, Era: EraLegacy, Channel: RPC, Code: codeInvalidRequest,
					Prefix: "resources/subscribe cannot be honored in stateless HTTP mode", Answer: FixRequest,
					At: refuse(pkgServer, "errStatelessSubscribe"),
				},
			},
			Sites: []Site{
				enforce(pkgServer, "sessionBridge.subscribeUnlessStateless"),
				refuse(pkgServer, "errStatelessSubscribe"),
			},
		},
		{
			ID: "HLD-009", Question: Authorize, Kind: Rule, Class: ClassP, Disposition: Ruled,
			Resource: "whether subscriptions are offered at all",
			Key:      KeyDeployment, StdioKey: KeyDeployment,
			Source: Configurable, Flags: []string{"--capability-surface"},
			Envs: []string{"GITLAB_MCP_CAPABILITY_SURFACE"}, Config: []string{"CapabilitySurface"},
			Malformed: RefuseStartup,
			Sites:     []Site{enforce(pkgServer, "newSubscriptionShape")},
		},
	}
}
