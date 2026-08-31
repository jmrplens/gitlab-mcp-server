package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/mcpotel"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v2/internal/toolutil"
)

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
	policy, err := telemetryIdentityPolicy()
	if err != nil {
		slog.Error("telemetry identity policy is unusable; recording nothing about callers",
			"component", "telemetry", "error", err)
		return nil
	}
	if policy == telemetry.IdentityNone {
		// Nil rather than a redactor that returns nothing: it takes the
		// middleware's own no-op path and never allocates a salt for a policy
		// that will not use one.
		return nil
	}

	redactor, err := telemetry.NewRedactor(policy)
	if err != nil {
		slog.Error("telemetry identity redactor could not be built; recording nothing about callers",
			"component", "telemetry", "error", err)
		return nil
	}
	logIdentityPolicy(policy)
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
	return telemetry.NewResourceRedactor(policy)
}
