package tenancy

import "time"

// The policy values. Each constant has exactly the typedness of the literal it
// replaces at its site: untyped where the site was untyped, and time.Duration
// where the site multiplied by a time unit. So a site that aliases one compiles
// to what it compiled to before, and a deliberate change of value edits the
// constant here and its frozen pin in values_test.go, two lines in one package
// (INV-021).
//
// Each group names the rows that read it. The reasons stay in the comments of
// the sites, where the gate finds them; what is here is the number.

// Holdings (HLD-001 to HLD-004, HLD-007, RTC-005).
const (
	// ListenStreamsPerCredential is how many subscriptions/listen streams one
	// credential may hold open (HLD-001).
	ListenStreamsPerCredential = 64
	// ListenStreamsPerProcess is how many subscriptions/listen streams the
	// process holds open across every credential (HLD-002).
	ListenStreamsPerProcess = 512
	// WatchersPerCredential is how many resource watchers one credential's
	// manager holds (HLD-003).
	WatchersPerCredential = 10
	// WatchersPerProcess is how many resource watchers the process holds
	// across every credential (HLD-004).
	WatchersPerProcess = 512
	// WatchLease is how long a watch runs at its own cadence without traffic
	// before it is demoted (HLD-007).
	WatchLease = 30 * time.Minute
	// WatchSlowInterval is the poll a demoted watch runs at (HLD-007).
	WatchSlowInterval = 10 * time.Minute
	// WatchMaxLifetime is when a watch stops however it is used (HLD-007).
	WatchMaxLifetime = 24 * time.Hour
	// WatchBaseInterval is the poll a watch starts at (HLD-007).
	WatchBaseInterval = 15 * time.Second
	// WatchMinInterval is the fastest poll a watch may run at (HLD-007; the
	// five-second floor HLD-003's own reason is sized against).
	WatchMinInterval = 5 * time.Second
	// WatchRateLimitPause is the first pause of every watcher of a manager
	// that saw a GitLab 429 (RTC-005).
	WatchRateLimitPause = 30 * time.Second
	// WatchRateLimitPauseMax is the longest that pause doubles to (RTC-005).
	WatchRateLimitPauseMax = 5 * time.Minute
	// WatchRateLimitJitter is the fraction the pause is spread by (RTC-005).
	WatchRateLimitJitter = 0.2
)

// Rates (RTC-001 to RTC-003, RTC-006).
const (
	// ToolCallRateHTTP is the tool-call bucket's refill, in requests a second,
	// an HTTP deployment gets unless it says otherwise (RTC-001).
	ToolCallRateHTTP = 10
	// ToolCallRateEnvDefault is the refill stdio gets when nothing is set:
	// zero, which switches the limiter off (RTC-001, CON-010). The HTTP
	// environment overlay passes it too, but only as its parser's default for
	// an empty value, which the overlay's presence check never lets through,
	// so HTTP with nothing set keeps ToolCallRateHTTP.
	ToolCallRateEnvDefault = 0
	// ToolCallBurst is the tool-call bucket's size (RTC-001).
	ToolCallBurst = 40
	// ToolCallRateMax is the highest refill an operator may configure
	// (RTC-001).
	ToolCallRateMax = 1000
	// ToolCallBurstMax is the largest bucket an operator may configure
	// (RTC-001).
	ToolCallBurstMax = 10000
	// CompletionFactor is how much larger and faster the completion bucket is
	// than the tool-call one (RTC-002).
	CompletionFactor = 10
	// CatalogDivisor is how much slower the tools/list bucket refills than the
	// tool-call one (RTC-003).
	CatalogDivisor = 10
	// UpstreamRetries is how many times a failed GitLab request is re-sent
	// (RTC-006).
	UpstreamRetries = 2
	// UpstreamRetryWaitMax caps one wait between attempts (RTC-006).
	UpstreamRetryWaitMax = 5 * time.Second
	// UpstreamRetryStep is the per-attempt wait for a failure that is not a
	// 429 (RTC-006).
	UpstreamRetryStep = 700 * time.Millisecond
)

// The pool (POL-001, POL-004, POL-006).
const (
	// PoolSize is how many entries the pool keeps unless the operator says
	// otherwise (POL-001).
	PoolSize = 100
	// PoolSizeMax is the largest pool an operator may configure (POL-001).
	PoolSizeMax = 10000
	// PoolIdleTimeout is how long an entry may go unused before it is
	// reclaimed (POL-004).
	PoolIdleTimeout = 1 * time.Hour
	// PoolIdleTimeoutMax is the longest idle timeout an operator may configure
	// (POL-004).
	PoolIdleTimeoutMax = 24 * time.Hour
	// PoolIdleSweepDivisor sets the idle sweep's cadence as a fraction of the
	// timeout (POL-004).
	PoolIdleSweepDivisor = 4
	// PoolIdleSweepFloor is the floor under that cadence (POL-004).
	PoolIdleSweepFloor = 1 * time.Minute
	// CredentialProbes is how many credential probes the pool runs at once
	// (POL-006).
	CredentialProbes = 16
	// CredentialProbeWait is how long a build waits for a probe slot
	// (POL-006).
	CredentialProbeWait = 5 * time.Second
)

// Admission (ADM-002, ADM-005, ADM-006, ADM-008 to ADM-010, IDN-011, END-005).
const (
	// UpstreamRetryAfter is the Retry-After a gate refusal advertises when
	// GitLab failed the verification without saying when to come back
	// (ADM-002).
	UpstreamRetryAfter = 30 * time.Second
	// OAuthCacheTTL is how long a verified OAuth token is reused (ADM-005).
	OAuthCacheTTL = 15 * time.Minute
	// OAuthCacheTTLFloor is the shortest cache lifetime an operator may
	// configure (ADM-005).
	OAuthCacheTTLFloor = 1 * time.Minute
	// OAuthCacheTTLMax is the longest cache lifetime an operator may configure
	// (ADM-005).
	OAuthCacheTTLMax = 2 * time.Hour
	// OAuthCacheSweepDivisor sets the cache sweep's cadence as a fraction of
	// the lifetime (ADM-005).
	OAuthCacheSweepDivisor = 4
	// OAuthCacheSweepFloor is the floor under that cadence (ADM-005).
	OAuthCacheSweepFloor = 30 * time.Second
	// RejectedTokenTTL is how long a rejected token is remembered (ADM-006).
	RejectedTokenTTL = 5 * time.Minute
	// RejectedTokenCapacity is how many rejected tokens are remembered
	// (ADM-006).
	RejectedTokenCapacity = 4096
	// CredentialMaxAge is the longest an entry serves on the strength of one
	// credential check (ADM-008).
	CredentialMaxAge = 1 * time.Hour
	// CredentialMaxAgeCeiling is the largest age the pool option honors
	// (ADM-008).
	CredentialMaxAgeCeiling = 24 * time.Hour
	// RevalidateInterval is how often every entry's credential is re-probed
	// (ADM-009).
	RevalidateInterval = 15 * time.Minute
	// RevalidateIntervalMax is the longest interval an operator may configure
	// (ADM-009).
	RevalidateIntervalMax = 24 * time.Hour
	// UnexplainedRefusalCooldown is how long a 401 that named no cause waits
	// before another one is confirmed with a probe (ADM-010).
	UnexplainedRefusalCooldown = 30 * time.Second
	// RequestStateTTL is how long signed multi-round-trip request state stays
	// usable (IDN-011).
	RequestStateTTL = 10 * time.Minute
	// SessionIdleTimeout is how long a stateful session may sit idle before
	// the SDK closes it (END-005).
	SessionIdleTimeout = 30 * time.Minute
	// SessionIdleTimeoutMax is the longest idle timeout an operator may
	// configure (END-005).
	SessionIdleTimeoutMax = 24 * time.Hour
)

// Authentication failure budgets (AUB-001 to AUB-005).
const (
	// AuthFailureLimit is how many failed authentications one address may
	// produce inside the window before it is blocked (AUB-001).
	AuthFailureLimit = 10
	// AuthFailureWindow is the window that budget counts in, and the step the
	// distinct-credential escalation is built from (AUB-001, AUB-003).
	AuthFailureWindow = 1 * time.Minute
	// AuthFailureLimitMax is the largest limit an operator may configure
	// (AUB-001).
	AuthFailureLimitMax = 100000
	// AuthFailureWindowMax is the longest window an operator may configure
	// (AUB-001).
	AuthFailureWindowMax = 24 * time.Hour
	// TransportSourceDistinctKeys is how many distinct failing primary keys
	// one transport source may produce inside the window (AUB-002).
	TransportSourceDistinctKeys = 500
	// AuthDistinctTokenLimit is how many distinct refused credentials one
	// address may have inside its window before it is blocked (AUB-003).
	AuthDistinctTokenLimit = 50
	// AuthDistinctTokenWindow is the window that budget counts in (AUB-003).
	AuthDistinctTokenWindow = 10 * time.Minute
	// AuthDistinctTokenLimitMax is the largest limit an operator may configure
	// (AUB-003).
	AuthDistinctTokenLimitMax = 100000
	// AuthDistinctTokenWindowMax is the longest window an operator may
	// configure (AUB-003).
	AuthDistinctTokenWindowMax = 24 * time.Hour
	// AuthEscalationFirst is the first block's length in steps (AUB-003).
	AuthEscalationFirst = 1
	// AuthEscalationSecond is the second block's length in steps (AUB-003).
	AuthEscalationSecond = 10
	// AuthEscalationThird is the third and every later block's length in
	// steps (AUB-003).
	AuthEscalationThird = 60
	// AuthTrackedSources is how many addresses the failure table tracks
	// (AUB-004).
	AuthTrackedSources = 4096
	// AuthSweepInterval is how often the authentication tables are swept
	// (AUB-005).
	AuthSweepInterval = 5 * time.Minute
)

// The tier (AUT-003).
const (
	// TierNamespacePageSize is the page size of the namespace listing the
	// tier probe reads (AUT-003).
	TierNamespacePageSize = 100
	// TierNamespaceMaxPages is how many pages of it the probe reads at most
	// (AUT-003).
	TierNamespaceMaxPages = 10
)
