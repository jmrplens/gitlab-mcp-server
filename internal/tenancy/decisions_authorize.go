package tenancy

// authorizeDecisions are the rows that answer "what may it do?" (spec:
// The five questions): the surface a credential's scopes, the instance's tier
// and the operator's configuration leave, the local files a stdio process may
// reach, the destinations a client may dial, the response profile and cache
// hints a session is given, and which subscriptions may be made at all.
//
// Authority is the credential's own, which is why most of these are class C:
// two credentials of one tenant may carry different scopes and so be served
// different surfaces. A narrowed surface says why wherever it can (Withheld),
// and a tier narrowing does not yet (F-10, issue 956).
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
			ID: "AUT-001", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the read-only surface a credential without the api scope is served",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Decided:  []string{"ADR-0018"},
			Findings: []string{"F-17"},
			Refusals: narrowed(WidenScope),
			Sites: []Site{
				enforce(pkgGitLab, "WriteCapable"),
				enforce(pkgGitLab, "NarrowToTokenScope"),
				enforce(pkgTools, "FilterActionCatalog"),
				withheld,
			},
		},
		{
			ID: "AUT-002", Question: Authorize, Kind: Rule, Class: ClassC, Disposition: Ruled,
			Resource: "the catalog groups a credential without admin_mode loses whole",
			Key:      KeyEntry, StdioKey: KeyProcess,
			Refusals: narrowed(WidenScope),
			Sites: []Site{
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
			Findings:  []string{"F-33"},
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
