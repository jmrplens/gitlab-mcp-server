// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

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
	// A count the flag package cannot parse refuses startup, as every
	// flag's does.
	rqb001.Malformed = RefuseStartup

	rqb004 := bound("RQB-004", "the length of one stdio line",
		enforce(pkgServer, "defaultMaxStdioLineBytes"),
		enforce(pkgServer, "stdioMaxLineBytesEnv"),
		enforce(pkgServer, "stdioMaxLineBytesSetting"))
	rqb004.Envs = []string{"GITLAB_MCP_STDIO_MAX_LINE_BYTES"}
	// A value that does not parse warns and keeps the default rather than
	// refusing startup (issue 957): stdio configuration arrives through the
	// environment of whatever launched the process, and failing to start over
	// one mistyped number would take the client down with it.
	rqb004.Malformed, rqb004.MalformedBy = WarnKeepDefault, "issue 957"
	rqb004.Decided = []string{"issue 957"}

	rqb005 := bound("RQB-005", "how long one action's handler may run",
		enforce(pkgConfig, "DefaultActionTimeout"),
		enforce(pkgConfig, "MaxActionTimeout"),
		enforce(pkgToolutil, "actionTimeoutNanos"))
	rqb005.Flags = []string{"--action-timeout"}
	rqb005.Envs = []string{"GITLAB_MCP_ACTION_TIMEOUT"}
	rqb005.Config = []string{"ActionTimeout"}
	// A duration that does not parse, or one above the ceiling, refuses
	// startup, from the variable on both transports and from the flag in
	// HTTP mode. A blank variable is unset on both, as the overlay and
	// stdio's reader each read it.
	rqb005.Malformed = RefuseStartup

	rqb007 := bound("RQB-007", "the size of one uploaded or read file",
		enforce(pkgConfig, "DefaultMaxFileSize"),
		enforce(pkgConfig, "MaxFileSize"),
		enforce(pkgConfig, "UploadMaxFileSizeFromEnv"),
		enforce(pkgConfig, "loadOverlayUploadLimit"),
		enforce(pkgToolutil, "DefaultMaxFileSize"))
	rqb007.Flags = []string{"--upload-max-file-size"}
	rqb007.Envs = []string{"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE"}
	rqb007.Config = []string{"UploadMaxFileSize"}
	// A size that does not parse, one that is not positive and one above
	// the ceiling refuse startup on both transports (issue 1170): the flag
	// writes the variable, and stdio's Load and HTTP mode's overlay read it
	// through the one reader that holds it to its bounds, which reads a
	// blank value as unset as the overlay does. HTTP mode used to warn and
	// use the default, or the maximum, where stdio refused.
	rqb007.Malformed = RefuseStartup
	rqb007.Decided = []string{"issue 1170"}

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
		bound("RQB-010", "what one action's output, or one resource read, may carry",
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
			// The context lines a code search asks GitLab for on each side of
			// a match, GitLab's own ceiling, refused before the request as
			// the coordinate length below is.
			enforce("internal/tools/search", "maxContextLines"),
			// The patch text the Markdown of one merge request changes read
			// carries; a patch that does not fit is skipped and counted in a
			// note rather than cut.
			enforce("internal/tools/mrchanges", "diffBudgetBytes"),
			enforce("internal/tools/projects", "maxAvatarBytes"),
			enforce("internal/tools/jobs", "maxTraceBytes"),
			// The end of a log one trace read takes in and strips of the
			// runner's markup, ten times what it returns.
			enforce("internal/tools/jobs", "traceWindowBytes"),
			// The lines of a log one trace read counts back from its end.
			enforce("internal/tools/jobs", "maxTraceTailLines"),
			enforce("internal/tools/jobs", "maxArtifactBytes"),
			// The file content one read of the file blob resource carries;
			// a larger file is answered with its metadata and no content.
			enforce("internal/resources", "fileBlobMaxBytes"),
			enforce("internal/tools/modelregistry", "maxModelFileBytes"),
			enforce("internal/tools/settings", "maxNamedUnmodeledKeys"),
			// The one ceiling every download action reads its body under:
			// the project and group export archives and the dependency list
			// export, each refused whole above it rather than cut short.
			enforce(pkgToolutil, "MaxDownloadBytes"),
			enforce("internal/tools/dependencyfirewall", "maxCoordinateLength"),
			enforce(pkgToolutil, "maxGitLabMessageLen"),
			// GitLab's reason for a failed transfer as a project or group
			// transfer quotes it, the length GitLab stores it at.
			enforce("internal/tools/waitpoll", "transferReasonMaxRunes"),
			// What the Orbit reads refuse before they send anything, each a
			// decision of this server wherever its number comes from (issue
			// 1209). GitLab.com's own 400s of 2026-10-29 give two: the 256
			// bytes of the search of orbit.grep, in the refusal of a short
			// one, and the one token orbit.context resolves, in the refusal
			// of two.
			enforce("internal/tools/orbit", "maxGrepSearchBytes"),
			enforce("internal/tools/orbit", "maxContextTokens"),
			// GitLab's source declares two that no captured answer refuses a
			// call past (ee/lib/api/orbit/data.rb): the results of orbit.grep
			// and those of one scope.
			enforce("internal/tools/orbit", "maxGrepLimit"),
			enforce("internal/tools/orbit", "maxGrepPerScope"),
			// And two are ceilings of this server's own, twenty where GitLab
			// lists ten scopes and served three commands: the scopes
			// orbit.grep names and the command names orbit.command_list
			// describes.
			enforce("internal/tools/orbit", "maxGrepScopes"),
			enforce("internal/tools/orbit", "maxCommandNames")),
		// A fine-grained token's grant is the minter's to size: every project
		// or group id becomes a scope of its own, each scope may name every
		// assignable permission, so the body GitLab answers the grant read
		// with is bounded where it is read, below the client's ceiling of
		// RQB-009, and its scopes are counted as they are decoded. The grant
		// is read from one of two answers, held to the same bounds: the self
		// route's, which carries it from GitLab 19.5 on and which every
		// token's description is therefore read under, and GET
		// /personal_access_tokens/:id's, asked only when the self answer did
		// not carry it (issue 1400). A grant past either bound is not
		// evaluated, and the session stays in phase A with the reason
		// (AUT-008); a self answer past the byte bound is a fine-grained
		// token's whose grant is too large, never a token of unknown kind.
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
		// The Orbit graph schema and the Orbit tool list are read a page at a
		// time, each page asked with the cursor the last one ended at, and
		// only GitLab's answer says when the cursor runs out. An answer whose
		// cursor never ends would hold one call for as long as GitLab kept
		// answering, so the pages one call follows are bounded where it
		// follows them, as RQB-012 and RQB-013 bound the pages their walks
		// read (issue 1717).
		bound("RQB-014", "the pages one Orbit schema or tool list read follows",
			enforce("internal/tools/orbit", "maxSchemaPages"),
			enforce("internal/tools/orbit", "maxToolPages")),
		// What one request writes into the operator's telemetry, which the
		// operator pays to export and keep: the tracestate a caller hands
		// over, carried onto every span of the trace and chosen before the
		// caller has authenticated on the HTTP edge; an HTTP method the
		// semantic conventions do not name, kept on the span beside _OTHER;
		// and one exported log attribute, which a refusal path fills with
		// what it refused. The logs SDK applies no length of its own, so
		// each is cut where it is written (issue 1717).
		bound("RQB-015", "what one request writes into the operator's telemetry",
			enforce("internal/mcpotel", "maxTraceStateBytes"),
			enforce("internal/mcpotel", "maxOriginalMethod"),
			enforce("internal/telemetry", "maxExportedAttrValue")),
	}
}
