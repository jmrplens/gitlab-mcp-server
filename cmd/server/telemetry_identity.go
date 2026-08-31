package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

// telemetryIdentityKeyFlag holds --telemetry-identity-key, and
// telemetryIdentityRotationFlag holds --telemetry-identity-rotation. Pointers,
// for the same reason the policy flag is one: an explicit empty value on the
// command line has to beat an environment variable, or the more specific
// instruction loses.
var (
	telemetryIdentityKeyFlag      *string
	telemetryIdentityRotationFlag *string
)

// identityKeyring is the process's one keyring, built on first use.
//
// One, for the same reason there is one redactor: every pseudonym this process
// emits has to come from the same secret, or the same caller reads as two
// people across signals.
var (
	identityKeyringOnce sync.Once
	identityKeyringVal  *Keyring
)

// Keyring is the local alias, so this file reads without the package prefix on
// every line.
type Keyring = telemetry.Keyring

// telemetryIdentityKey resolves the operator's pseudonymisation secret.
//
// Empty means none was supplied, which is the default and generates one
// instead. The flag wins over the environment when it was actually passed.
func telemetryIdentityKey() string {
	value := os.Getenv(telemetry.EnvIdentityKeyName)
	if telemetryIdentityKeyFlag != nil && isFlagPassed("telemetry-identity-key") {
		value = *telemetryIdentityKeyFlag
	}
	return value
}

// telemetryIdentityRotation resolves how long a generated key lives.
//
// Unparseable is fatal rather than silently defaulted, like the policy: an
// operator who typed "24" meaning hours should be told, not given a key that
// rotates every twenty-four nanoseconds.
func telemetryIdentityRotation() (time.Duration, error) {
	value := os.Getenv(telemetry.EnvIdentityRotationName)
	if telemetryIdentityRotationFlag != nil && isFlagPassed("telemetry-identity-rotation") {
		value = *telemetryIdentityRotationFlag
	}
	if strings.TrimSpace(value) == "" {
		return telemetry.DefaultKeyRotation, nil
	}
	rotation, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", telemetry.EnvIdentityRotationName, err)
	}
	return rotation, nil
}

// identityKeyring returns the process's keyring, building it the first time.
//
// A failure records nothing rather than stopping the process, matching every
// other decision in this file: telemetry that cannot pseudonymize must not fall
// back to naming people, and must not take the server down either.
func identityKeyring() *Keyring {
	identityKeyringOnce.Do(func() {
		rotation, err := telemetryIdentityRotation()
		if err != nil {
			slog.Error("telemetry identity key rotation is unusable; recording nothing about callers",
				"component", "telemetry", "error", err)
			return
		}

		secret := telemetryIdentityKey()
		ring, err := telemetry.NewKeyring(secret, rotation)
		if err != nil {
			slog.Error("telemetry pseudonymisation keyring could not be built; recording nothing about callers",
				"component", "telemetry", "error", err)
			return
		}

		logKeyringChoice(ring, rotation)
		identityKeyringVal = ring
	})
	return identityKeyringVal
}

// logKeyringChoice says what an operator configured, including the case where
// one setting silently cancels another.
//
// The ignored rotation is the line worth having. Somebody who supplied a key
// and also asked for rotation has expressed two intentions that cannot both
// hold, and the one this server honors is the key, because rotating a secret
// its owner supplied would destroy the correlation they configured it for.
func logKeyringChoice(ring *Keyring, requested time.Duration) {
	switch {
	case ring.Configured() && requested > 0:
		slog.Warn("telemetry pseudonyms use the configured key; the rotation interval is ignored",
			"component", "telemetry", "requested_rotation", requested,
			"why", "a key supplied by the operator is theirs to rotate")
	case ring.Configured():
		slog.Info("telemetry pseudonyms use the configured key, so they are stable across replicas and restarts",
			"component", "telemetry")
	case ring.Rotation() > 0:
		slog.Info("telemetry pseudonyms use a generated key that rotates",
			"component", "telemetry", "rotation", ring.Rotation())
	}
}

// telemetryIdentityFlag holds --telemetry-identity.
//
// A pointer, like the telemetry switch itself, so that "not passed" stays
// distinguishable from "passed as the default". Without that an explicit
// --telemetry-identity=none could not beat an environment variable saying
// otherwise, which is backwards: a person typing a value on the command line
// has been more specific than one who exported it.
var telemetryIdentityFlag *string

// telemetryIdentityPolicy resolves how much this deployment records about who
// made a call.
//
// The default is none, and the reason is not caution for its own sake. The
// attributes involved are Opt-In in the semantic conventions, whose rule reads
// "Instrumentations SHOULD populate the attribute if and only if the user
// configures the instrumentation to do so". That is a SHOULD rather than a
// prohibition, and the harder MUST NOT beside it is scoped to instrumentation
// that cannot be configured, which this can. So the obligation is one we choose
// to honor: whether to record a person's identity in exported telemetry is the
// operator's decision about their own users, and a default that decides for
// them is the wrong default whichever way it points.
//
// An unparseable value is fatal rather than silently defaulted. Falling back to
// none would leave an operator who typed "pseudonimous" believing they had
// per-user correlation, looking at an empty dashboard, with nothing anywhere
// saying why.
func telemetryIdentityPolicy() (telemetry.IdentityPolicy, error) {
	value := os.Getenv(telemetry.EnvIdentityName)
	if telemetryIdentityFlag != nil && isFlagPassed("telemetry-identity") {
		value = *telemetryIdentityFlag
	}
	return telemetry.ParseIdentityPolicy(value)
}

// newUserAttributer connects the identity policy to the identity this server
// already resolves for its own log records.
//
// It is the whole point of the seam: internal/mcpotel knows nothing about
// GitLab identities, internal/telemetry knows nothing about MCP requests, and
// this function is the only place that knows both.
//
// The identity is read exactly the way every tool handler reads it, through
// toolutil.ResolveIdentity, so the span cannot disagree with the log line about
// who made a call. That matters more than it looks: two subsystems each doing
// their own resolution is how a trace ends up attributing a request to the
// wrong person after somebody changes one of them.
func newUserAttributer(redactor *telemetry.Redactor) mcpotel.UserAttributer {
	return mcpotel.UserAttributerFunc(func(ctx context.Context, req mcp.Request) []attribute.KeyValue {
		// ResolveIdentity takes a CallToolRequest because that is the only
		// request type carrying Extra.TokenInfo. For every other method the
		// context is the source, which is what stdio uses anyway.
		callReq, _ := req.(*mcp.CallToolRequest)
		identity := toolutil.ResolveIdentity(ctx, callReq)
		return redactor.Attributes(identity.UserID, identity.Username)
	})
}

// logIdentityPolicy tells the operator what they configured, once.
//
// Only when it is not the default. A line saying "we are recording nothing"
// on every startup is noise; a line saying "we are recording usernames" is the
// one somebody needs to see in a log they are skimming for surprises.
func logIdentityPolicy(policy telemetry.IdentityPolicy) {
	if policy == telemetry.DefaultIdentityPolicy {
		return
	}
	slog.Warn("telemetry records caller identity",
		"component", "telemetry",
		"policy", string(policy),
		"exported", telemetry.PolicyDescription(policy))
}

// identityRedactor is the process's one identity redactor, built on first use.
//
// One, deliberately. Each redactor generates its own salt, so a second would
// give the same person a different user.hash on a log record than on a span:
// one person appearing as two, which is worse than not recording them at all.
// It is also what keeps [TestRedactor_PseudonymDoesNotSurviveARestart] honest,
// since that test uses a new redactor as a stand-in for a new process.
//
// Nil under [telemetry.IdentityNone] and after any failure, which every
// consumer reads as "record nothing": the zero value of the policy.
var (
	identityRedactorOnce sync.Once
	identityRedactorVal  *telemetry.Redactor
)

// identityRedactor returns it, building it the first time.
func identityRedactor() *telemetry.Redactor {
	identityRedactorOnce.Do(func() {
		policy, err := telemetryIdentityPolicy()
		if err != nil {
			slog.Error("telemetry identity policy is unusable; recording nothing about callers",
				"component", "telemetry", "error", err)
			return
		}
		if policy == telemetry.IdentityNone {
			return
		}
		redactor, err := telemetry.NewRedactor(policy, identityKeyring())
		if err != nil {
			slog.Error("telemetry identity redactor could not be built; recording nothing about callers",
				"component", "telemetry", "error", err)
			return
		}
		logIdentityPolicy(policy)
		identityRedactorVal = redactor
	})
	return identityRedactorVal
}

// telemetryUsers builds the attributer the middleware consults, or the one that
// answers nothing.
//
// A policy this server cannot parse is reported and treated as none. That is
// the opposite of what [telemetryIdentityPolicy] does for a startup check, and
// deliberately so: by the time a server is registering middleware it is already
// running, and the safe answer to "I do not understand your privacy setting" is
// to record less rather than to guess or to crash mid-startup. The startup
// validation is where a typo is meant to be caught loudly.
func telemetryUsers() mcpotel.UserAttributer {
	redactor := identityRedactor()
	if redactor == nil {
		// Nil rather than a redactor that returns nothing: it takes the
		// middleware's own no-op path and never allocates a salt for a policy
		// that will not use one.
		return nil
	}
	return newUserAttributer(redactor)
}

// telemetryResources builds the redactor that decides how much a span may say
// about which resource a request named.
//
// It reads the same policy as [telemetryUsers], deliberately: a resource URI in
// this server embeds project and group identifiers, so it is the same class of
// disclosure as a caller's name, and an operator has already made that decision
// once. Two flags would let one deployment export project paths while claiming
// to record nobody.
//
// Unlike the identity case, IdentityNone still gets a redactor rather than nil.
// Its answer under that policy is a keyed digest, which names no project and is
// what keeps two watchers of the same kind distinguishable in a trace.
func telemetryResources() mcpotel.ResourceAttributer {
	policy, err := telemetryIdentityPolicy()
	if err != nil {
		slog.Error("telemetry identity policy is unusable; recording nothing about resources",
			"component", "telemetry", "error", err)
		return nil
	}
	return telemetry.NewResourceRedactor(policy, identityKeyring())
}
