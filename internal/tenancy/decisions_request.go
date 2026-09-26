package tenancy

// requestDecisions are the per-request bounds of spec section 3.2.10. They
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
			enforce("internal/tools/projects", "maxAvatarBytes"),
			enforce("internal/tools/jobs", "maxTraceBytes"),
			enforce("internal/tools/jobs", "maxArtifactBytes"),
			enforce("internal/tools/modelregistry", "maxModelFileBytes"),
			enforce("internal/tools/settings", "maxNamedUnmodeledKeys"),
			enforce("internal/tools/groupimportexport", "maxExportBytes"),
			enforce("internal/tools/dependencyfirewall", "maxCoordinateLength"),
			// Local to the function, so the function is the site.
			enforce("internal/tools/dependencies", "DownloadExport"),
			enforce(pkgToolutil, "maxGitLabMessageLen")),
	}
}
