// export_test.go exposes unexported clientcompat internals to the external
// test package so every defensive branch (nil clientInfo, sessions without
// initialize params, unknown content types) is reachable without fabricating
// SDK state that the public API cannot produce.
package clientcompat

var (
	// ProfileFromClientInfoForTest exposes profileFromClientInfo.
	ProfileFromClientInfoForTest = profileFromClientInfo
	// ProfileForRequestForTest exposes profileForRequest.
	ProfileForRequestForTest = profileForRequest
	// RoundPriorityForTest exposes roundPriority.
	RoundPriorityForTest = roundPriority
	// RoundContentPrioritiesForTest exposes roundContentPriorities.
	RoundContentPrioritiesForTest = roundContentPriorities
	// SanitizeForCodexForTest exposes sanitizeForCodex.
	SanitizeForCodexForTest = sanitizeForCodex
)
