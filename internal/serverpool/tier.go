// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package serverpool

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// tierProvisionalRetry is how long an entry built on a provisional tier is
// served before the sweep first asks it again, and how often the sweep looks
// for one whose wait is over.
const tierProvisionalRetry = tenancy.TierProvisionalRetry // register row AUT-010

// tierProvisionalRetryMax is the longest the wait between two asks of a tier
// that stays provisional grows to.
const tierProvisionalRetryMax = tenancy.TierProvisionalRetryMax // register row AUT-010

// tierRedetectionSweep is the sweep's name on the line [recoverSweep] writes
// when a tick of it, an ask it started or the sweep's own goroutine panics.
const tierRedetectionSweep = "tier re-detection"

// provisionalTier is what an entry serving a provisional tier carries: a step
// of the probe GitLab did not answer could raise it, so the sweep asks it
// again (register row AUT-010).
type provisionalTier struct {
	// unresolved names the steps of the probe that were not read, in the
	// order the probe asks them, for the lines that report the entry.
	unresolved []string
	// asked counts the re-detections that have been made of the tier while
	// it was provisional, the entry's and those of the entries it replaced.
	asked int
	// wait is how long the sweep waits before asking, from the moment the
	// entry was built or last asked.
	wait time.Duration
	// due is when the sweep asks again.
	due time.Time
}

// newProvisionalTier returns the mark an entry built on resolution carries,
// nil when the resolution is not provisional. The sweep asks it after
// [ServerPool.tierRetry] once the entry is inserted ([provisionalTier.askAfter]).
func newProvisionalTier(resolution gitlabclient.TierResolution) *provisionalTier {
	if !resolution.Provisional {
		return nil
	}
	return &provisionalTier{unresolved: resolution.Unresolved}
}

// askAfter schedules the next ask wait from now.
func (t *provisionalTier) askAfter(now time.Time, wait time.Duration) {
	t.wait, t.due = wait, now.Add(wait)
}

// tierRedetectDelay is how long the sweep waits before asking a provisional
// tier again, given how long it waited before the ask that left it
// provisional: twice that, never past [tierProvisionalRetryMax]. The first
// wait is [ServerPool.tierRetry], and each ask falls on the sweep's tick
// nearest the end of its wait ([ServerPool.dueTiers]), so the asks come about
// a minute after the build and then two, four, eight and every fifteen minutes
// after the ask before, each within half a minute of that.
func tierRedetectDelay(previous time.Duration) time.Duration {
	return min(2*previous, tierProvisionalRetryMax)
}

// WithTierRetry sets how long an entry built on a provisional tier is served
// before it is first asked again, and how often the sweep looks for one whose
// wait is over; the wait doubles from it, never past
// [tenancy.TierProvisionalRetryMax].
//
// The register fixes the value (row AUT-010): the option exists for the tests,
// and no operator reaches it, since the operator's lever is the tier itself,
// --tier. A value of zero or less keeps [tenancy.TierProvisionalRetry], and one
// above the ceiling is clamped down to it.
func WithTierRetry(d time.Duration) Option {
	return func(p *ServerPool) {
		if d <= 0 {
			p.tierRetry = tierProvisionalRetry
			return
		}
		p.tierRetry = min(d, tierProvisionalRetryMax)
	}
}

// StartTierRedetection launches the sweep that asks provisional tiers again
// (register row AUT-010). Cancel the context, or close the pool, to stop it:
// [ServerPool.Close] ends it and waits for it.
//
// It runs whatever --revalidate-interval says and whether or not the entry has
// a subscription open, since neither has anything to do with whether its tier
// was read whole: an entry serving nothing but a subscriptions/listen makes no
// request through the pool and is never revalidated with the interval at
// zero, and those were the two ways a tier read wrong at a build used to be
// kept for as long as the entry lived. Under --tier no entry is ever
// provisional, so the sweep is not started.
func (p *ServerPool) StartTierRedetection(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background() //nolint:contextcheck // defensive: nil-ctx guard for callers that pass uninitialized context
	}
	if p.cfg.TierExplicit {
		slog.InfoContext(ctx, "server pool: tier re-detection not started, the tier is configured")
		return
	}

	slog.InfoContext(ctx, "server pool: starting tier re-detection", "interval", p.tierRetry)

	ctx, stop := context.WithCancel(ctx)
	p.sweepMu.Lock()
	p.sweepStops = append(p.sweepStops, stop)
	p.sweepMu.Unlock()
	// Counted until the stop line is written, so Close returns after it.
	p.sweeps.Go(func() {
		defer recoverSweep(ctx, tierRedetectionSweep)

		ticker := time.NewTicker(p.tierRetry)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.InfoContext(ctx, "server pool: tier re-detection stopped")
				return
			case <-ticker.C:
				sweepTick(ctx, tierRedetectionSweep, func() { p.redetectDueTiers(ctx) })
			}
		}
	})
}

// dueTier is an entry whose provisional tier the sweep asks again, and the key
// it was pooled under when the sweep found it.
type dueTier struct {
	key   string
	entry *Entry
}

// redetectDueTiers is one tick of the sweep: every entry whose provisional
// tier's wait is over is asked again, each on a goroutine of its own holding
// one of the pool's credential probe slots (register row POL-006).
//
// The slot is taken without waiting, as a confirmation of a 401 takes it: a
// re-detection refines an entry already served, while a build is a credential
// waiting to be served, so when GitLab is slow enough to keep the slots busy it
// is the re-detection that gives way. An entry that finds no slot stays due
// and is asked on the next tick. The tick waits for the asks it started, so two
// ticks never ask one entry at once, and a re-detection costs at most one tier
// probe per entry per wait.
func (p *ServerPool) redetectDueTiers(ctx context.Context) {
	var asks sync.WaitGroup
	for _, due := range p.dueTiers(time.Now()) {
		if ctx.Err() != nil {
			break
		}
		release, ok := p.tryProbeSlot()
		if !ok {
			slog.DebugContext(ctx, "server pool: no credential probe slot free to ask a provisional tier again; asking on the next tick")
			break
		}
		asks.Go(func() {
			defer release()
			// Its own recovery, since the tick's does not reach a goroutine:
			// settling the answer calls back into code the pool does not own.
			defer recoverSweep(ctx, tierRedetectionSweep)
			p.redetectTier(ctx, due.key, due.entry)
		})
	}
	asks.Wait()
}

// dueTiers returns the entries whose provisional tier is due at now, the
// moment of a tick: those whose wait ends no later than half a tick after it,
// so each ask falls on the tick nearest the end of its wait.
//
// A wait is measured from when the entry was built or last asked, which is
// after the tick that asked it by however long GitLab took to answer, and the
// waits are whole multiples of the tick. Compared with the tick alone, the
// tick on which a wait ends found it a few milliseconds short of due and left
// it to the next one, so every ask came one tick late: the first a minute or
// two after the build, the next ones three, five and nine minutes apart where
// the waits were two, four and eight. Half a tick of slack absorbs any answer
// shorter than half a minute, which every ask is, since its reads are bounded
// by entryDetectTimeout and the membership step's eight seconds.
func (p *ServerPool) dueTiers(now time.Time) []dueTier {
	p.mu.RLock()
	defer p.mu.RUnlock()
	horizon := now.Add(p.tierRetry / 2)
	var due []dueTier
	for key, entry := range p.entries {
		if entry.provisional != nil && !horizon.Before(entry.provisional.due) {
			due = append(due, dueTier{key: key, entry: entry})
		}
	}
	return due
}

// redetectTier asks the tier of entry, pooled under key, again, and acts on the
// answer ([ServerPool.settleTier]).
//
// The question is [gitlabclient.Client.ResolveTier], which stores nothing on
// the client: the entry's catalog is registered for the tier its client
// carries, and only a replacement on another shape may move that. The
// license and namespace steps share [entryDetectTimeout] (the pool's
// detectTimeout), as at a build, and
// the membership step runs on its own clock measured from the sweep's
// context, which ends at shutdown.
func (p *ServerPool) redetectTier(ctx context.Context, key string, entry *Entry) {
	// An entry dropped or replaced since the tick found it is not asked: the
	// question was about it, and a replacement carries a mark of its own.
	if !p.isCurrent(key, entry) {
		return
	}
	askCtx, cancel := context.WithTimeout(ctx, p.detectTimeout)
	defer cancel()
	slog.DebugContext(ctx, "server pool: asking a provisional tier again",
		"tier", entry.serverConfig.Tier.String(),
		telemetry.LogFieldCredentialHash, entry.credentialHash)
	got := entry.client.ResolveTier(askCtx, ctx)
	p.metrics.TierRedetections.Add(1)
	p.settleTier(ctx, key, entry, got)
}

// settleTier acts on what a re-detection of entry's provisional tier
// concluded, on ADR-0018's asymmetry: a tier too low withholds actions without
// a word, while one too high is corrected by GitLab's own refusal.
//
//   - A tier higher than the entry's, provisional or not, replaces the entry on
//     the shape of that tier ([ServerPool.replaceTier]).
//   - A different tier read whole replaces it too, a lower one included: the
//     plan was read, and it is lower than the steps that answered at the build.
//   - The same tier read whole clears the mark, and the tier is not asked again.
//   - Anything else is a provisional answer no higher than the entry's, which
//     proves nothing lower: a step that fails again is no evidence that the
//     caller holds less. The entry keeps its tier and is asked again later.
//
// A provisional answer never lowers the tier and there are three tiers, so an
// entry is replaced at most twice.
func (p *ServerPool) settleTier(ctx context.Context, key string, entry *Entry, got gitlabclient.TierResolution) {
	// An if chain rather than a tagless switch: gremlins attributes no
	// coverage to a case's expression and would report its mutants as not
	// covered.
	current := entry.serverConfig.Tier
	if got.Tier > current || (!got.Provisional && got.Tier != current) {
		p.replaceTier(ctx, key, entry, got)
		return
	}
	if !got.Provisional {
		p.keepResolvedTier(ctx, key, entry)
		return
	}
	p.askTierLater(ctx, key, entry)
}

// keepResolvedTier clears entry's provisional mark, if entry is still the one
// under key: GitLab answered the tier whole and it is the one served.
func (p *ServerPool) keepResolvedTier(ctx context.Context, key string, entry *Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries[key] != entry {
		return
	}
	asked := entry.provisional.asked + 1
	entry.provisional = nil
	slog.InfoContext(ctx, "server pool: a provisional tier was answered whole and is kept",
		"tier", entry.serverConfig.Tier.String(),
		"asked", asked,
		telemetry.LogFieldCredentialHash, entry.credentialHash)
}

// askTierLater schedules the next ask of entry's provisional tier, if entry is
// still the one under key, twice as far away as the last, never further than
// [tierProvisionalRetryMax].
func (p *ServerPool) askTierLater(ctx context.Context, key string, entry *Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries[key] != entry {
		return
	}
	p.askTierLaterLocked(ctx, entry)
}

// askTierLaterLocked is [ServerPool.askTierLater] for a caller that holds
// p.mu and has checked that entry is pooled.
func (p *ServerPool) askTierLaterLocked(ctx context.Context, entry *Entry) {
	mark := entry.provisional
	mark.asked++
	mark.askAfter(time.Now(), tierRedetectDelay(mark.wait))
	slog.DebugContext(ctx, "server pool: a provisional tier is still provisional; asking again later",
		"tier", entry.serverConfig.Tier.String(),
		"asked", mark.asked,
		"next_ask_in", mark.wait,
		telemetry.LogFieldCredentialHash, entry.credentialHash)
}

// errNoReplacementServer is a factory that reported success while handing
// back no server for the shape a provisional tier resolved to.
var errNoReplacementServer = errors.New("creating MCP server for pool: factory returned no server")

// replaceTier serves the credential pooled under key on the tier got resolved,
// replacing old, its provisional entry, with one on the shape of that tier
// (register rows AUT-010 and POL-007).
//
// The tier is part of the shape a server is built for (ADR-0020), and a
// session is bound to the server that opened it, so the tier cannot change in
// place: the replacement is a new entry. It keeps everything that belongs to
// the credential rather than to the shape: the client, which already carries
// the credential, the identity, the token's facts, the credential handle and
// the credential's age, since no new credential check was made, and the LRU
// position. It gets a server of the new shape and an owner of its own, so the
// eviction callback ends what the old owner held exactly as a rebuild ends it
// ([CauseTierResolved]): its listens are told the credential was reset, and on
// a stateful deployment its sessions are forgotten, so their next request is
// answered with the 404 that asks the client to start over. Nothing is asked
// of GitLab a second time.
//
// The old entry is kept, and asked again later, when the factory cannot build
// the new shape, when it hands back no server, and when the insert callback
// refuses the replacement: an entry on the tier it has is better than none.
// It is not replaced either when it is no longer the one under key, or when
// GitLab has refused its credential, since the eviction that follows ends it.
//
// The entry's client is told the new tier once the replacement is pooled, and
// its 401 hook is pointed at the replacement. A call the old server is still
// running reads the new tier from the shared client, lower or higher than the
// one that server was built for, where a handler asks whether the instance is
// licensed, and GitLab decides what it answers. A 401 handed to the old
// entry in the window around the swap, before the new hook is set or by a
// call that loaded the old one, ends the replacement: the drop follows the
// client to the entry that carries it ([ServerPool.holderLocked]), since a
// 401 naming the credential reaches the client's hook only once and the
// replacement would otherwise serve a refused credential. A verdict the
// revalidation sweep reaches for the old entry, whose snapshot predates the
// swap, follows the client the same way.
func (p *ServerPool) replaceTier(ctx context.Context, key string, old *Entry, got gitlabclient.TierResolution) {
	cfg := *old.serverConfig
	cfg.Tier = got.Tier
	server, err := p.factory(old.client, &cfg)
	if err == nil && server == nil {
		err = errNoReplacementServer
	}
	if err != nil {
		slog.WarnContext(ctx, "server pool: could not build the shape a provisional tier resolved to; keeping the entry and asking again",
			"tier", old.serverConfig.Tier.String(),
			"resolved", got.Tier.String(),
			"error", err,
			telemetry.LogFieldCredentialHash, old.credentialHash)
		p.askTierLater(ctx, key, old)
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries[key] != old || old.rejected.Load() {
		return
	}
	now := time.Now()
	next := assembleEntry(old.seed(), server, &cfg)
	next.element = old.element
	next.createdAt = now
	next.lastValidated = old.lastValidated
	next.lastUsed = old.lastUsed
	if next.provisional = newProvisionalTier(got); next.provisional != nil {
		next.provisional.asked = old.provisional.asked + 1
		next.provisional.askAfter(now, tierRedetectDelay(old.provisional.wait))
	}

	// The replacement is pooled before the insert callback runs, as a built
	// entry is, so a callback that starts work which may later have to evict
	// it can find it. A refusal puts the old entry back before the lock is
	// released, so no lookup ever sees the refused one.
	p.entries[key] = next
	if p.onInsert != nil && !p.onInsert(next) {
		p.entries[key] = old
		slog.WarnContext(ctx, "server pool: the shape a provisional tier resolved to could not take the entry; keeping it and asking again",
			"tier", old.serverConfig.Tier.String(),
			"resolved", got.Tier.String(),
			telemetry.LogFieldCredentialHash, old.credentialHash)
		p.askTierLaterLocked(ctx, old)
		return
	}
	old.client.SetTier(got.Tier)
	p.watchUnauthorized(key, next) //nolint:contextcheck // the hook runs on a later 401, on the refused call's goroutine, under no context of this sweep's
	p.metrics.TierResolvedEvictions.Add(1)
	slog.InfoContext(ctx, "server pool: moved an entry to the tier its re-detection resolved",
		"tier_from", old.serverConfig.Tier.String(),
		"tier_to", got.Tier.String(),
		"tier_source", p.tierSource(next),
		"gitlab_url", cfg.GitLabURL,
		telemetry.LogFieldCredentialHash, next.credentialHash)
	// The second of the two places an entry leaves p.entries, beside
	// dropEntry: here something takes its place, and the callback is told the
	// old one is gone all the same. Last, so a callback that panics leaves the
	// replacement whole: its client on the new tier and its 401 hook on it.
	if p.onEvict != nil {
		p.onEvict(old, CauseTierResolved)
	}
}
