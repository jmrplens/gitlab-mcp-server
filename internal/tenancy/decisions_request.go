package tenancy

// requestDecisions are the per-request bounds, the RQB rows. They
// answer the allow question for one request rather than for a caller, so they
// are class P on the request key and are declared rather than owned: they carry
// no values here, and nothing about them moves.
//
// Declaring them gives the specification one list of requirements, and lets
// the gate hold that their sites still exist. Their key is the request, so the
// mintable-key question is vacuous for them; the per-action output caps live in
// domain packages that import toolutil and nothing policy-shaped; and owning
// them would turn the register from who a caller is and what it may hold into
// every number in the server. A bound that names a variable says so in Envs,
// so how the variable is read is still held.
func requestDecisions() []Decision {
	bound := func(id, resource string, sites ...Site) Decision {
		return Decision{
			ID: id, Question: Allow, Kind: Bound, Class: ClassP, Disposition: RequestBound,
			Resource: resource, Key: KeyRequest, StdioKey: KeyRequest, Sites: sites,
		}
	}
	rqb001 := bound("RQB-001", "the size of one HTTP request body",
		enforce(pkgServer, "inboundLimitsFor"),
		enforce(pkgServer, "streamableHTTPOptions"))
	rqb001.StdioKey = KeyNone
	rqb001.Flags = []string{"--max-request-body-bytes"}
	rqb001.Config = []string{"MaxRequestBodyBytes"}

	rqb004 := bound("RQB-004", "the length of one stdio line",
		enforce(pkgServer, "defaultMaxStdioLineBytes"),
		enforce(pkgServer, "stdioMaxLineBytesEnv"))
	rqb004.Envs = []string{"GITLAB_MCP_STDIO_MAX_LINE_BYTES"}
	rqb004.Findings = []string{"F-13"}

	rqb005 := bound("RQB-005", "how long one action's handler may run",
		enforce(pkgConfig, "DefaultActionTimeout"),
		enforce(pkgConfig, "MaxActionTimeout"),
		enforce(pkgToolutil, "actionTimeoutNanos"))
	rqb005.Flags = []string{"--action-timeout"}
	rqb005.Envs = []string{"GITLAB_MCP_ACTION_TIMEOUT"}
	rqb005.Config = []string{"ActionTimeout"}

	rqb007 := bound("RQB-007", "the size of one uploaded or read file",
		enforce(pkgConfig, "DefaultMaxFileSize"),
		enforce(pkgConfig, "MaxFileSize"),
		enforce(pkgToolutil, "DefaultMaxFileSize"))
	rqb007.Flags = []string{"--upload-max-file-size"}
	rqb007.Envs = []string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE"}
	rqb007.Config = []string{"UploadMaxFileSize"}

	return []Decision{
		rqb001,
		bound("RQB-002", "the nesting of an inbound JSON body",
			enforce(pkgServer, "maxInboundJSONDepth")),
		bound("RQB-003", "the nesting of a tool's arguments",
			enforce(pkgToolutil, "DefaultMaxArgumentDepth")),
		rqb004,
		rqb005,
		bound("RQB-006", "the length of a find query",
			enforce(pkgDynamic, "MaxSearchQueryLength")),
		rqb007,
		bound("RQB-008", "a local path named over HTTP",
			enforce(pkgToolutil, "requireLocalFilesystemAccess"),
			enforce(pkgToolutil, "httpTransportConfigured"),
			enforce(pkgServer, "applyLocalFilesystemPolicy")),
		bound("RQB-009", "the size of one GitLab response body",
			enforce(pkgGitLab, "DefaultMaxResponseBytes")),
		bound("RQB-010", "what one action's output may carry",
			enforce(pkgDynamic, "maxLimit"),
			enforce(pkgDynamic, "maxSegmentTerms"),
			enforce("internal/tools/usagedata", "maxMetricDefinitionsBytes"),
			enforce("internal/tools/usagedata", "maxRenderedMetrics"),
			enforce("internal/tools/usagedata", "maxRenderedYAMLBytes"),
			// The code search Markdown shows the head of each snippet, a
			// bounded run of bounded lines, and holds its heading, table
			// rows and snippet sections to one budget for the page, as the
			// client receives them.
			enforce("internal/tools/search", "maxSnippetLines"),
			enforce("internal/tools/search", "maxSnippetLineRunes"),
			enforce("internal/tools/search", "maxCodeMarkdownBytes"),
			enforce("internal/tools/projects", "maxAvatarBytes"),
			enforce("internal/tools/jobs", "maxTraceBytes"),
			// The lines of a log one trace read counts back from its end.
			enforce("internal/tools/jobs", "maxTraceTailLines"),
			enforce("internal/tools/jobs", "maxArtifactBytes"),
			enforce("internal/tools/modelregistry", "maxModelFileBytes"),
			enforce("internal/tools/settings", "maxNamedUnmodeledKeys"),
			// The one ceiling every download action reads its body under:
			// the project and group export archives and the dependency list
			// export, each refused whole above it rather than cut short.
			enforce(pkgToolutil, "MaxDownloadBytes"),
			enforce("internal/tools/dependencyfirewall", "maxCoordinateLength"),
			enforce(pkgToolutil, "maxGitLabMessageLen")),
		// A fine-grained token's grant is the minter's to size: every project
		// or group id becomes a scope of its own, each scope may name every
		// assignable permission, so the body GitLab answers the grant read
		// with is bounded where it is read, below the client's ceiling of
		// RQB-009, and its scopes are counted as they are decoded. A grant
		// past either bound is not evaluated, and the session stays in phase
		// A with the reason (AUT-008).
		bound("RQB-011", "a fine-grained token's grant, as read",
			enforce(pkgGitLab, "GrantMaxBytes"),
			enforce(pkgGitLab, "GrantMaxScopes"),
			enforce(pkgGitLab, "WithResponseLimit")),
		// The latest-pipeline fallback reads the pipeline list of one ref a
		// page at a time until a row of the ref's own turns up, and on a
		// branch whose rows are merge request pipelines or from a source that
		// does not set its status nothing else ends the walk. The
		// latest_pipeline resource runs it on every poll of a watch, so the
		// pages one read sends are bounded where the walk is (issue 1242).
		bound("RQB-012", "the pages of the pipeline list one latest-pipeline fallback reads",
			enforce("internal/tools/pipelines", "latestFallbackMaxPages")),
		// An inline discussion or draft note checks its position against the
		// merge request's diff, listed 100 files a page, and the walk stops at
		// the page that names the file. GitLab stores at most diff_max_files
		// files in one diff version (3,000 at the highest setting), so a real
		// instance ends the walk within 30 pages, and the bound ends a walk
		// whose next page never runs out. Reaching it skips the check and
		// GitLab judges the write.
		bound("RQB-013", "the diff pages one advisory merge request position check reads",
			enforce(pkgToolutil, "mergeRequestDiffPositionMaxPages")),
	}
}
