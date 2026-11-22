// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/edition"
)

// instancePlanSinceMajor and instancePlanSinceMinor are the first release whose
// version answer carries the instance plan: GitLab 19.6 added `plan` to
// GET /metadata and its alias GET /version (lib/api/entities/metadata.rb,
// doc/api/metadata.md, issue 1491). Before it the key is absent, and an
// instance that reports an older release is never asked to have it.
const (
	instancePlanSinceMajor = 19
	instancePlanSinceMinor = 6
)

// instancePlanMaxBytes bounds the raw plan value that is read at all. The
// longest plan GitLab sends is "ultimate" quoted, ten bytes; anything longer
// is no plan of GitLab's, and under --allow-any-gitlab-url the instance is one
// a caller named.
const instancePlanMaxBytes = 32

// instancePlanKind is what the version answer said about the instance plan.
type instancePlanKind uint8

const (
	// instancePlanAbsent is no key at all: a release before 19.6, or
	// GitLab.com's prerelease before the field was deployed there. It is the
	// zero value, so a read that kept nothing answers nothing.
	instancePlanAbsent instancePlanKind = iota
	// instancePlanPerNamespace is a null plan: GitLab saying subscriptions
	// are held per namespace (gitlab_com_subscriptions, GitLab.com), where
	// /license answers nobody and describes no caller's entitlement.
	instancePlanPerNamespace
	// instancePlanRecognized is one of the four plans
	// ee/app/models/ee/app_config/instance_metadata.rb sends: the instance's
	// own answer for every caller that may read the metadata.
	instancePlanRecognized
	// instancePlanUnrecognized is any other value, which is no answer and
	// never Free, on ADR-0018's asymmetry.
	instancePlanUnrecognized
)

// instancePlan is the instance plan one version read reported.
type instancePlan struct {
	kind instancePlanKind
	// tier is the tier a recognized plan carries, Free otherwise.
	tier edition.Tier
	// name is the recognized plan as GitLab spelled it, for the line that
	// reports the answer; empty otherwise.
	name string
}

// parseInstancePlan reads the raw plan value of a version answer: an absent
// key leaves raw empty and null decodes as the bytes null, which a *string
// would both read as nil, and the two mean different things (see
// [instancePlanKind]). Only the four plans GitLab sends are recognized, as
// GitLab spells them: an expired paid license reports its plan, an expired
// trial, no license or a CE build "free", and a license naming no plan
// "starter", so nothing else is a plan of GitLab's.
func parseInstancePlan(raw json.RawMessage) instancePlan {
	raw = bytes.TrimSpace(raw)
	switch {
	case len(raw) == 0:
		return instancePlan{}
	case len(raw) > instancePlanMaxBytes:
		return instancePlan{kind: instancePlanUnrecognized}
	case bytes.Equal(raw, []byte("null")):
		return instancePlan{kind: instancePlanPerNamespace}
	}
	var name string
	if json.Unmarshal(raw, &name) != nil {
		return instancePlan{kind: instancePlanUnrecognized}
	}
	switch name {
	case "free", "starter", "premium", "ultimate":
		return instancePlan{kind: instancePlanRecognized, tier: edition.TierFromPlan(name), name: name}
	default:
		return instancePlan{kind: instancePlanUnrecognized}
	}
}

// tierFromInstancePlan is the tier probe's first step from GitLab 19.6 on: the
// instance plan the version answer carries, which any token that may read the
// metadata is told (Metadata: Read for a fine-grained one, every classic
// scope this server admits). It reads the version this client already holds,
// and asks /version only when no read has answered yet ([Client.AtLeast]),
// so a stdio start, whose Initialize read it, costs no request.
//
// An instance not known to run 19.6 or later answers nothing: one that
// reported an older release, one that refused this token the version
// (Metadata: Read, [Client.VersionRefusal]) and one that did not answer. A
// value outside the four plans is logged at debug and answers nothing either.
func (c *Client) tierFromInstancePlan(ctx context.Context) instancePlan {
	if !c.AtLeast(ctx, instancePlanSinceMajor, instancePlanSinceMinor) {
		return instancePlan{}
	}
	plan := c.plan.Load()
	if plan == nil {
		return instancePlan{}
	}
	switch plan.kind {
	case instancePlanRecognized:
		slog.InfoContext(ctx, "detected GitLab tier from the instance plan", "plan", plan.name, "tier", plan.tier.String())
	case instancePlanPerNamespace:
		slog.DebugContext(ctx, "the instance reports that subscriptions are held per namespace; not asking for the instance license")
	case instancePlanUnrecognized:
		slog.DebugContext(ctx, "the instance reports a plan this server does not recognize; detecting the tier as before 19.6")
	}
	return *plan
}
