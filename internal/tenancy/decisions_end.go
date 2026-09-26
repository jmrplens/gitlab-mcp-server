package tenancy

// endDecisions are the rows that answer "what happens when the server ends
// what a caller held?" (spec: The five questions): why a listen was ended and
// what its client is told, how the sessions no stream ended are closed, how a
// notification reaches only its owner, what eviction ends, and how long a
// stateful session may sit idle.
//
// An ending is not a refusal: a conformant client comes back. So each ending
// names its cause from a closed vocabulary where a channel exists, and a cause
// nobody decided produces no reason at all (INV-020).
func endDecisions() []Decision {
	endSessions := refuse(pkgServer, "sessionOwners.endSessionsWithoutStreams")
	return []Decision{
		{
			ID: "END-001", Question: End, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "why an entry ended, told to its open listens",
			Key:      KeyOwner, StdioKey: KeyNone,
			Decided: []string{"ADR-0020"},
			Refusals: []Refusal{
				listenEnd("credential_evicted", StartOver, refuse(pkgServer, "endOfCredentialEviction")),
				listenEnd("credential_reset", StartOver, refuse(pkgServer, "endOfCredentialReset")),
				listenEnd("credential_revoked", Reauthorize, refuse(pkgServer, "endOfCredentialRevocation")),
				listenEnd("shutdown", RetryLater, refuse(pkgServer, "endOfShutdown")),
			},
			Sites: []Site{enforce(pkgServer, "watchEndForCause")},
		},
		{
			ID: "END-002", Question: End, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "why a watch ended, told only to its owner's listens",
			Key:      KeyOwner, StdioKey: KeyProcess,
			Refusals: []Refusal{
				listenEnd("resource_gone", FixRequest, refuse(pkgServer, "resourceGoneEnd")),
				listenEnd("lifetime_reached", StartOver, refuse(pkgServer, "endOfLifetime")),
				listenEnd("watcher_evicted", StartOver, refuse(pkgServer, "endOfWatcherEviction")),
			},
			Sites: []Site{enforce(pkgServer, "watchEndForStop")},
		},
		{
			ID: "END-003", Question: End, Kind: Rule, Class: ClassR, Disposition: Ruled,
			Resource: "the session-era subscribers an eviction closes",
			Key:      KeyOwner, StdioKey: KeyNone,
			Refusals: []Refusal{
				{
					Methods: []string{MethodEviction}, Era: EraLegacy, Channel: SessionClose, Answer: StartOver,
					At: endSessions,
				},
			},
			Sites: []Site{enforce(pkgServer, "sessionOwners.endSessionsWithoutStreams")},
		},
		{
			ID: "END-004", Question: End, Kind: Rule, Class: ClassR, Disposition: Mechanism,
			Resource: "a resource-updated notification, delivered only to its owner's sessions",
			Key:      KeyOwner, StdioKey: KeyNone,
			Sites: []Site{enforce(pkgServer, "sessionOwners.sendingMiddleware")},
		},
		{
			// Surfaced by the gate as the register landed, not by the survey of
			// limits before it, and made a row then. Through the flag a zero
			// leaves the SDK with no idle timeout; the environment variable is
			// read by the parser every positive duration shares, which refuses
			// a zero at startup. That second half is an INV-015 departure of
			// the kind issue 958 records.
			ID: "END-005", Question: End, Kind: Lifetime, Class: ClassQ, Disposition: Valued,
			Resource: "how long a stateful session may sit idle",
			Key:      KeySession, StdioKey: KeyNone,
			Values: []string{"SessionIdleTimeout", "SessionIdleTimeoutMax"},
			Source: Configurable, Flags: []string{"--session-timeout"}, Envs: []string{"GITLAB_MCP_SESSION_TIMEOUT"},
			Config: []string{"SessionTimeout"}, Malformed: RefuseStartup, Zero: ZeroOff,
			Findings: []string{"F-34"},
			Refusals: []Refusal{
				{
					Methods: []string{MethodExpiry}, Era: EraLegacy, Channel: SessionClose, Answer: StartOver,
					At: refuse(pkgServer, "streamableHTTPOptions"),
				},
			},
			Sites: []Site{
				alias(pkgConfig, "DefaultSessionTimeout", "SessionIdleTimeout"),
				alias(pkgConfig, "MaxSessionTimeout", "SessionIdleTimeoutMax"),
				enforce(pkgServer, "streamableHTTPOptions"),
				enforce(pkgServer, "validateHTTPDurationConfig"),
			},
		},
		{
			ID: "POL-008", Question: End, Kind: Rule, Class: ClassR, Disposition: Mechanism,
			Resource: "what an eviction ends: sessions, watchers and streams",
			Key:      KeyOwner, StdioKey: KeyNone,
			Sites: []Site{
				enforce(pkgServer, "credentialState.close"),
				enforce(pkgServer, "credentialStates.remove"),
			},
		},
	}
}
