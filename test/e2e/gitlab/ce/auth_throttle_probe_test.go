// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

//go:build e2e

// auth_throttle_probe_test.go asks a GitLab what an invalid token costs the
// address that sends it (issue 954): whether a REST request carrying one is
// counted against the per-IP limit on unauthenticated API requests, whether a
// ban follows, and whether a token GitLab accepts is still served from that
// address once the limit is reached.
//
// The distinct-credential budget states its reason in those terms
// (internal/serverpool/distinct_token_budget.go): every distinct token GitLab
// refuses that reaches the server's verification is a GET /api/v4/user from
// the deployment's own address. The probe sends that request itself, from the
// host the server runs on, and has the real binary send it in HTTP mode, so
// the address GitLab counts is the one a deployment's verification comes from.
//
// It changes an instance-wide setting, the unauthenticated API throttle, which
// issue 954 decided no complete run may do: a run cut short before the probe's
// cleanup would leave the throttle on for every package that runs after it.
// So it runs only when E2E_AUTH_THROTTLE_PROBE=true, only against the
// disposable Docker stack, and on its own. A complete run refuses to start
// with the switch set (test/e2e/scripts/skip-gate.sh), and the skip it sees
// without it is declared in cmd/audit_e2e_coverage/skip_declarations.go.

package ce

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	gl "gitlab.com/gitlab-org/api/client-go/v3"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tools/users"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/fixture"
	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// actionUserCurrent reads the authenticated user: the call the probe has a
// freshly started server make for a token GitLab accepts, while the address
// both run from is throttled.
const actionUserCurrent harness.ActionID = "user.current"

const (
	// settingAuthThrottleProbe is the switch that lets the probe change the
	// instance's throttle. Nothing sets it but a person running the probe.
	settingAuthThrottleProbe = "E2E_AUTH_THROTTLE_PROBE"

	// unauthenticatedAPIThrottle is the throttle under test, as GitLab names
	// it in the RateLimit-Name header of every response the throttle counted.
	unauthenticatedAPIThrottle = "throttle_unauthenticated_api"

	// probeLimit is how many unauthenticated API requests the probe lets one
	// address make in a window. It is small so the limit is reached in a few
	// requests, and large enough for the four the probe makes before it
	// starts counting toward the limit on purpose: a calibration request, the
	// server's verification of an invented token, the request that reads
	// what that verification cost, and an invented token sent as a Bearer
	// token.
	probeLimit = 4

	// probePeriod is the throttle's window, in seconds, which is long enough
	// for the probe to reach the limit and then hold three fresh server
	// processes to it before the window ends.
	//
	// GitLab 19.5 counts the window with Labkit rather than Rack::Attack:
	// every Labkit cohort enforces by default
	// (config/feature_flags/beta/rate_limiter_use_labkit_rack_cohort_*_enforce.yml
	// at v19.5.0-ee), and Rack::Attack then safelists every request as
	// labkit_fully_enforced (lib/gitlab/rack_attack.rb), which is what
	// api_json.log records against each of the probe's requests. Labkit's
	// counter is a Redis key that the first request it counts creates with the
	// period as its lifetime (lib/labkit/rate_limit/evaluator.rb at
	// labkit-ruby v5.5.0), so a window opens with the first counted request
	// and ends a period later, whatever the clock says. The RateLimit-Reset
	// and Retry-After GitLab writes are still computed for a window aligned to
	// the epoch (RequestThrottleData.from_labkit_result), so neither says when
	// the counter's window ends, and the probe measures that from the answer
	// that opened it instead.
	probePeriod = 120

	// windowMargin is what the probe adds to a period after an answer before
	// it counts on the window the answer was counted in having ended. The
	// counter exists before GitLab answers, so the window has ended a period
	// after the answer reached the probe; the margin is conservatism only.
	windowMargin = 2 * time.Second

	// throttleSettle is how long after a change of the throttle every GitLab
	// process applies it: each keeps the application settings in process
	// memory for application_settings_cache_seconds, 60 by default
	// (app/models/concerns/cacheable_attributes.rb at v19.5.0-ee), and the
	// throttle reads its settings from there on every request. The second on
	// top is margin.
	throttleSettle = time.Minute + time.Second

	// probeRequestTimeout bounds each request the probe sends itself.
	probeRequestTimeout = 15 * time.Second

	// probeBodyBytes is how much of an answer's body the probe reads before
	// closing it: none of what it asserts is in a body.
	probeBodyBytes = 64 << 10
)

// TestAuthThrottle_InvalidTokens_SpendTheAddressUnauthenticatedAllowance
// turns GitLab's unauthenticated API throttle on with a limit of four
// requests in two minutes and records, request by request, what GitLab
// answers and what it says it counted:
//
//   - with the throttle off, as the Docker stack is provisioned and as GitLab
//     ships on a self-managed instance, invalid tokens are answered 401 and
//     counted against no limit, and the server refuses an invented one 401;
//   - with it on, every GET /api/v4/user carrying an invalid token is counted
//     against the address's unauthenticated allowance, in PRIVATE-TOKEN as the
//     server's legacy verification sends it and as a Bearer token as its
//     oauth verification does, the server's own verification of an invented
//     token exactly once, and the request past the limit is answered 429, as
//     is an anonymous request from the same address;
//   - a token GitLab accepts is answered 200 from the throttled address, and a
//     server started there verifies it and serves it on every surface;
//   - once the window ends an invalid token is answered 401 and counted afresh,
//     so no ban followed, and once the throttle is off again nothing counts it.
//
// The instance's own settings are read first and written back at the end,
// whatever the test ended with.
func TestAuthThrottle_InvalidTokens_SpendTheAddressUnauthenticatedAllowance(t *testing.T) {
	e := harness.New(t, harness.Needs(harness.NeedAdmin), harness.Locks(harness.LockInstanceGlobal), harness.Serial())
	if !strings.EqualFold(e.Setting(settingAuthThrottleProbe), "true") {
		e.Skipf("the throttle probe turns GitLab's unauthenticated API throttle on for the whole instance; set %s=true to run it alone", settingAuthThrottleProbe)
	}
	if !e.DockerMode() {
		e.Skipf("the throttle probe changes instance-wide settings and runs only against the disposable Docker stack")
	}

	p := newThrottleProbe(e)
	original := p.read()
	e.T.Logf("GitLab %s, unauthenticated API throttle as found: %+v", e.Runtime().Version, original)
	if original.Enabled {
		e.T.Fatalf("the instance already throttles unauthenticated API requests (%+v); the probe starts from the throttle off", original)
	}

	p.countsNothing("off")
	p.throttle(throttleSettings{Enabled: true, Requests: probeLimit, Period: probePeriod}, original)

	opened := p.freshWindow()
	verified := p.verificationIsCountedOnce(opened)
	p.bearerIsCountedOnce(verified)
	p.untilThrottled()
	p.sameAddressPastTheLimit()

	harness.EachSurface(e, func(e *harness.Env, surface harness.Surface) {
		s := e.Session(harness.ServerConfig{Surface: surface, Transport: harness.TransportHTTP, Private: true})
		if me := harness.Do[users.Output](s, actionUserCurrent, nil); me.ID != e.Runtime().UserID {
			e.T.Errorf("a server started at the throttled address served %s as user %d, want the run's user %d", actionUserCurrent, me.ID, e.Runtime().UserID)
		}
	})

	still := p.invalid("on: invalid token after the servers were served")
	if still.Status != http.StatusTooManyRequests {
		e.T.Fatalf("the window GitLab throttled the address in ended before the servers were held to it (%s); nothing above says whether they were served while it was throttled", still)
	}

	p.awaitWindowEnd(opened)
	again := p.invalid("on: invalid token in the next window")
	if problem := nextWindowProblem(again); problem != "" {
		e.T.Errorf("once the window GitLab throttled the address in had ended, an invalid token was answered %s: %s", again, problem)
	}

	p.restore()
	p.countsNothing("restored")
}

// countsNothing holds the instance with the throttle off to answering invalid
// tokens 401 and counting them against no unauthenticated limit, the
// limit's worth and two more of them, and has the server refuse one 401.
func (p *throttleProbe) countsNothing(phase string) {
	p.e.T.Helper()
	for i := range probeLimit + 2 {
		if got := p.invalid(fmt.Sprintf("%s: invalid token %d", phase, i+1)); got.Status != http.StatusUnauthorized || got.Name == unauthenticatedAPIThrottle {
			p.e.T.Errorf("with the throttle off an invalid token was answered %s, want 401 counted by no unauthenticated limit", got)
		}
	}
	if refused := p.e.RefusedAdmission(harness.ServerConfig{Token: inventedToken(), Transport: harness.TransportHTTP}); refused.Status != http.StatusUnauthorized {
		p.e.T.Errorf("with the throttle off the server refused an invented token %d (%d %q), want 401", refused.Status, refused.Code, refused.Message)
	}
}

// verificationIsCountedOnce has a server verify an invented token, and reads
// off the address's count either side of it that the verification was
// counted against the unauthenticated allowance once. It returns the answer
// that read the count after it.
func (p *throttleProbe) verificationIsCountedOnce(first throttleAnswer) throttleAnswer {
	p.e.T.Helper()
	if refused := p.e.RefusedAdmission(harness.ServerConfig{Token: inventedToken(), Transport: harness.TransportHTTP}); refused.Status != http.StatusUnauthorized {
		p.e.T.Fatalf("under the limit the server refused an invented token %d (%d %q), want 401: GitLab refused its verification", refused.Status, refused.Code, refused.Message)
	}
	after := p.invalid("on: invalid token after the server's verification")
	if after.Status != http.StatusUnauthorized || after.Name != unauthenticatedAPIThrottle {
		p.e.T.Fatalf("after the server's verification an invalid token was answered %s, want 401 counted by %s", after, unauthenticatedAPIThrottle)
	}
	if spent := after.Observed - first.Observed - 1; spent != 1 {
		p.e.T.Errorf("the server's verification of one invented token was counted %d times against the address (observed %d, then %d), want once", spent, first.Observed, after.Observed)
	}
	return after
}

// bearerIsCountedOnce sends an invented token as a Bearer token, the way the
// server's oauth verification sends a token, and holds GitLab to counting it
// against the address once, as it counted the one sent in PRIVATE-TOKEN just
// before.
func (p *throttleProbe) bearerIsCountedOnce(before throttleAnswer) {
	p.e.T.Helper()
	got := p.invalidBearer("on: invalid Bearer token")
	if got.Status != http.StatusUnauthorized || got.Name != unauthenticatedAPIThrottle {
		p.e.T.Fatalf("an invalid Bearer token was answered %s, want 401 counted by %s", got, unauthenticatedAPIThrottle)
	}
	if spent := got.Observed - before.Observed; spent != 1 {
		p.e.T.Errorf("an invalid Bearer token was counted %d times against the address (observed %d, then %d), want once", spent, before.Observed, got.Observed)
	}
}

// sameAddressPastTheLimit holds GitLab, once it throttled the address, to
// answering an anonymous request from it 429 too, and the run's own token
// 200, counted by no unauthenticated limit.
func (p *throttleProbe) sameAddressPastTheLimit() {
	p.e.T.Helper()
	if anonymous := p.anonymous("on: anonymous request past the limit"); anonymous.Status != http.StatusTooManyRequests || anonymous.Name != unauthenticatedAPIThrottle {
		p.e.T.Errorf("past the limit an anonymous request from the same address was answered %s, want 429 from %s", anonymous, unauthenticatedAPIThrottle)
	}
	if valid := p.valid("on: the run's token past the limit"); valid.Status != http.StatusOK || valid.Name == unauthenticatedAPIThrottle {
		p.e.T.Errorf("past the limit the run's token was answered %s, want 200 counted by no unauthenticated limit", valid)
	}
}

// throttleSettings is the part of the application settings the probe changes:
// the unauthenticated API throttle's switch, limit and period.
type throttleSettings struct {
	Enabled  bool
	Requests int64
	Period   int64
}

// throttleProbe sends the probe's own requests, straight to GitLab from the
// host the server runs on, and keeps the instance's throttle as it was found.
type throttleProbe struct {
	e *harness.Env
	// base is the instance's address, the one every server the harness
	// starts is pointed at.
	base string
	// token is the run's own credential, a token GitLab accepts.
	token  string
	client *http.Client
	// original is the throttle as the probe found it, and restored says it
	// has been written back.
	original throttleSettings
	restored bool
}

// newThrottleProbe returns a probe for the instance e runs against.
//
// Its client is a plain one with no retries, since a 429 is what it asks
// about and client-go would wait out the window GitLab names before
// returning one.
func newThrottleProbe(e *harness.Env) *throttleProbe {
	e.T.Helper()
	token := e.Setting(fixture.SettingGitLabToken)
	if token == "" {
		e.T.Fatalf("%s is required: the probe asks GitLab what it answers the run's token", fixture.SettingGitLabToken)
	}
	return &throttleProbe{
		e:      e,
		base:   strings.TrimSuffix(e.Runtime().URL, "/"),
		token:  token,
		client: &http.Client{Timeout: probeRequestTimeout},
	}
}

// read returns the throttle as the instance holds it now.
func (p *throttleProbe) read() throttleSettings {
	p.e.T.Helper()
	got, _, err := p.e.Client().GL().Settings.GetSettings(gl.WithContext(p.e.Ctx))
	if err != nil {
		p.e.T.Fatalf("reading the application settings: %v", err)
	}
	return throttleSettings{
		Enabled:  got.ThrottleUnauthenticatedAPIEnabled,
		Requests: got.ThrottleUnauthenticatedAPIRequestsPerPeriod,
		Period:   got.ThrottleUnauthenticatedAPIPeriodInSeconds,
	}
}

// write sets the throttle to want.
func (p *throttleProbe) write(ctx context.Context, want throttleSettings) error {
	_, _, err := p.e.Client().GL().Settings.UpdateSettings(&gl.UpdateSettingsOptions{
		ThrottleUnauthenticatedAPIEnabled:           new(want.Enabled),
		ThrottleUnauthenticatedAPIRequestsPerPeriod: new(want.Requests),
		ThrottleUnauthenticatedAPIPeriodInSeconds:   new(want.Period),
	}, gl.WithContext(ctx))
	return err
}

// throttle sets the throttle to want and returns once every GitLab process
// applies it. The test's cleanup writes original back and waits the same
// time before the instance is given back, unless the body has already done
// so.
//
// The restore is a cleanup of the test rather than of its ledger, as the
// Admin Mode scenario's is: it runs before the serial gate opens, with a
// deadline of its own, since the wait alone outlasts the budget the ledger's
// work shares.
func (p *throttleProbe) throttle(want, original throttleSettings) {
	p.e.T.Helper()
	p.original = original
	p.e.T.Cleanup(func() {
		if p.restored {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*throttleSettle)
		defer cancel()
		if err := p.write(ctx, original); err != nil {
			p.e.T.Errorf("writing the unauthenticated API throttle back to %+v: %v", original, err)
			return
		}
		if err := awaitSettled(ctx, throttleSettle); err != nil {
			p.e.T.Errorf("waiting for the unauthenticated API throttle to be back everywhere: %v", err)
		}
	})
	if err := p.write(p.e.Ctx, want); err != nil {
		p.e.T.Fatalf("setting the unauthenticated API throttle to %+v: %v", want, err)
	}
	if got := p.read(); got != want {
		p.e.T.Fatalf("the unauthenticated API throttle reads %+v after the change, want %+v", got, want)
	}
	if err := awaitSettled(p.e.Ctx, throttleSettle); err != nil {
		p.e.T.Fatalf("waiting for the unauthenticated API throttle to apply everywhere: %v", err)
	}
}

// restore writes the throttle back as it was found, waits until every GitLab
// process applies it, and checks the instance holds it.
func (p *throttleProbe) restore() {
	p.e.T.Helper()
	if err := p.write(p.e.Ctx, p.original); err != nil {
		p.e.T.Fatalf("writing the unauthenticated API throttle back to %+v: %v", p.original, err)
	}
	p.restored = true
	if got := p.read(); got != p.original {
		p.e.T.Errorf("the unauthenticated API throttle reads %+v after the restore, want %+v", got, p.original)
	}
	if err := awaitSettled(p.e.Ctx, throttleSettle); err != nil {
		p.e.T.Fatalf("waiting for the unauthenticated API throttle to be back everywhere: %v", err)
	}
}

// freshWindow returns the request that opened the window the probe then
// spends: the first calibration GitLab counted as the first request of its
// window, which has the whole period ahead of it (see probePeriod). A
// calibration counted into a window something else opened waits that window
// out, since nothing GitLab answers says how much of it is left.
func (p *throttleProbe) freshWindow() throttleAnswer {
	p.e.T.Helper()
	for attempt := 1; ; attempt++ {
		got := p.invalid(fmt.Sprintf("on: calibration %d", attempt))
		if got.Status != http.StatusUnauthorized || got.Name != unauthenticatedAPIThrottle || got.Observed < 1 {
			p.e.T.Fatalf("with the throttle on an invalid token was answered %s, want 401 counted by %s", got, unauthenticatedAPIThrottle)
		}
		if opensWindow(got) {
			return got
		}
		if attempt == 3 {
			p.e.T.Fatalf("no window opened with a calibration as its first request in three tries (last %s)", got)
		}
		p.awaitWindowEnd(got)
	}
}

// opensWindow reports whether GitLab counted the request as the first of its
// window, which is the request a Labkit window opens with.
func opensWindow(a throttleAnswer) bool {
	return a.Observed == 1
}

// untilThrottled sends invalid tokens until GitLab answers one 429, and
// fails the test when the limit's worth and two more were all let through.
// Every one let through must have been counted.
func (p *throttleProbe) untilThrottled() {
	p.e.T.Helper()
	for i := range probeLimit + 2 {
		got := p.invalid(fmt.Sprintf("on: invalid token %d toward the limit", i+1))
		if got.Status == http.StatusTooManyRequests {
			if got.Name != unauthenticatedAPIThrottle || got.RetryAfter <= 0 {
				p.e.T.Errorf("the 429 names the throttle %q with Retry-After %s, want %s with a positive Retry-After", got.Name, got.RetryAfter, unauthenticatedAPIThrottle)
			}
			if got.Observed != probeLimit+1 {
				p.e.T.Errorf("GitLab throttled the address at its request %d of the window, want %d: the limit is %d", got.Observed, probeLimit+1, probeLimit)
			}
			return
		}
		if got.Status != http.StatusUnauthorized || got.Name != unauthenticatedAPIThrottle {
			p.e.T.Fatalf("an invalid token toward the limit was answered %s, want 401 counted by %s", got, unauthenticatedAPIThrottle)
		}
	}
	p.e.T.Fatalf("GitLab let %d invalid tokens through a limit of %d without a 429", probeLimit+2, probeLimit)
}

// awaitWindowEnd waits until the window an answer was counted in has ended,
// by the probe's own clock and measured from now, so no wait the probe
// worked out earlier is spent twice.
func (p *throttleProbe) awaitWindowEnd(counted throttleAnswer) {
	p.e.T.Helper()
	wait := time.Until(counted.windowEndsBy())
	p.e.T.Logf("waiting %s for the window %q was counted in to end", wait.Round(time.Second), counted.Label)
	if err := awaitSettled(p.e.Ctx, wait); err != nil {
		p.e.T.Fatalf("waiting for the window %q was counted in to end: %v", counted.Label, err)
	}
}

// nextWindowProblem judges the answer to an invalid token sent once the
// window GitLab throttled the address in has ended, and says what is wrong
// with it, or nothing.
//
// A refusal is a ban, since the counter that refused the address has expired
// by then. A count other than one is not: it is a window that counted
// something besides the probe, which says nothing about a ban either way.
func nextWindowProblem(a throttleAnswer) string {
	switch {
	case a.Status == http.StatusTooManyRequests || a.Status == http.StatusForbidden:
		return "GitLab still refused the address a period after the window it throttled it in opened, which is a ban"
	case a.Status != http.StatusUnauthorized:
		return "want 401, the refusal of the token, and nothing about the address"
	case a.Name != unauthenticatedAPIThrottle:
		return "want it counted by " + unauthenticatedAPIThrottle + ", as every invalid token before it was"
	case a.Observed != 1:
		return fmt.Sprintf("counted as request %d of its window, want 1: the window counted something besides the probe, which says nothing about a ban", a.Observed)
	}
	return ""
}

// invalid sends GET /api/v4/user with an invented token in PRIVATE-TOKEN, as
// the server's legacy verification of a token it does not hold sends it.
func (p *throttleProbe) invalid(label string) throttleAnswer {
	p.e.T.Helper()
	return p.ask(label, "/api/v4/user", privateTokenHeader, inventedToken())
}

// invalidBearer sends GET /api/v4/user with an invented token as a Bearer
// token, as the server's oauth verification sends it. The token carries no
// glpat- prefix, as an OAuth access token does not, so GitLab looks it up as
// an OAuth token before it tries a personal access token.
func (p *throttleProbe) invalidBearer(label string) throttleAnswer {
	p.e.T.Helper()
	return p.ask(label, "/api/v4/user", "Authorization", "Bearer "+rand.Text())
}

// anonymous sends a request with no credential at all.
func (p *throttleProbe) anonymous(label string) throttleAnswer {
	p.e.T.Helper()
	return p.ask(label, "/api/v4/projects?per_page=1", "", "")
}

// valid sends GET /api/v4/user with the run's own token.
func (p *throttleProbe) valid(label string) throttleAnswer {
	p.e.T.Helper()
	return p.ask(label, "/api/v4/user", privateTokenHeader, p.token)
}

// privateTokenHeader is the header the server's legacy verification sends a
// token in.
const privateTokenHeader = "PRIVATE-TOKEN"

// ask sends one GET, with the credential in the header named when one is,
// and logs what GitLab answered and counted.
func (p *throttleProbe) ask(label, path, header, credential string) throttleAnswer {
	p.e.T.Helper()
	ctx, cancel := context.WithTimeout(p.e.Ctx, probeRequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.base+path, http.NoBody)
	if err != nil {
		p.e.T.Fatalf("building %s: %v", label, err)
	}
	if header != "" {
		req.Header.Set(header, credential)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		p.e.T.Fatalf("sending %s: %v", label, err)
	}
	received := time.Now()
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, probeBodyBytes))
	answer := readThrottleAnswer(label, resp)
	answer.At = received
	p.e.T.Logf("%s", answer)
	return answer
}

// throttleAnswer is what GitLab answered one probe request and what it said
// it counted, read off the headers GitLab 19.5 writes on every response a
// throttle counted (LabkitRackRateLimit#annotate_rate_limit_headers in
// lib/gitlab/middleware/labkit_rack_rate_limit.rb) and, on a 429, the two
// lib/gitlab/rack_attack/request_throttle_data.rb adds.
type throttleAnswer struct {
	Label  string
	Status int
	// Name is RateLimit-Name, the throttle that counted the request, empty
	// when none did.
	Name string
	// Observed is RateLimit-Observed, the requests that throttle counted in
	// the window this one included, and -1 when the header is absent.
	Observed int
	// Remaining is RateLimit-Remaining as GitLab wrote it.
	Remaining string
	// RetryAfter is Retry-After, which GitLab writes on a 429 only. It runs
	// to the end of a window aligned to the epoch rather than to the end of
	// the counter's window (see probePeriod), so the probe holds it to being
	// there and waits by At instead.
	RetryAfter time.Duration
	// At is when the probe received the answer, by its own clock and with
	// the monotonic reading time.Now gives it: what a wait for the end of
	// the window the request was counted in is measured from.
	At time.Time
}

// readThrottleAnswer reads the answer's status and rate limit headers.
func readThrottleAnswer(label string, resp *http.Response) throttleAnswer {
	answer := throttleAnswer{Label: label, Status: resp.StatusCode, Observed: -1}
	answer.Name = resp.Header.Get("RateLimit-Name")
	answer.Remaining = resp.Header.Get("RateLimit-Remaining")
	if observed, err := strconv.Atoi(resp.Header.Get("RateLimit-Observed")); err == nil {
		answer.Observed = observed
	}
	if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil {
		answer.RetryAfter = time.Duration(seconds) * time.Second
	}
	return answer
}

// windowEndsBy returns a time by which the window the request was counted in
// has ended: Labkit's window opened no later than the request, so it ends no
// later than a period after the probe received the answer.
func (a throttleAnswer) windowEndsBy() time.Time {
	return a.At.Add(probePeriod*time.Second + windowMargin)
}

// String renders the answer as the probe's record writes it.
func (a throttleAnswer) String() string {
	name := a.Name
	if name == "" {
		name = "(no throttle)"
	}
	return fmt.Sprintf("%s: %d %s observed=%d remaining=%q retry_after=%s",
		a.Label, a.Status, name, a.Observed, a.Remaining, a.RetryAfter)
}

// inventedToken returns a token no GitLab issued, distinct on every call, in
// the shape of a personal access token, as a sprayer would send them.
func inventedToken() string {
	return "glpat-" + rand.Text()
}

// awaitSettled holds for lifetime, or until ctx ends.
func awaitSettled(ctx context.Context, lifetime time.Duration) error {
	timer := time.NewTimer(lifetime)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// probeAnswerDate is the Date of the answers the tests below build: GitLab's
// clock at calibration 3 of the run the probe's record comes from.
var probeAnswerDate = time.Date(2026, 10, 20, 17, 6, 2, 0, time.UTC)

// countedHeader builds the headers GitLab 19.5 writes on an answer its
// unauthenticated API throttle counted, with the RateLimit-Reset it writes
// for a window aligned to the epoch resetIn after the answer's Date.
func countedHeader(observed int, resetIn time.Duration) http.Header {
	header := http.Header{}
	header.Set("RateLimit-Name", unauthenticatedAPIThrottle)
	header.Set("RateLimit-Observed", strconv.Itoa(observed))
	header.Set("RateLimit-Remaining", strconv.Itoa(max(probeLimit-observed, 0)))
	header.Set("RateLimit-Reset", strconv.FormatInt(probeAnswerDate.Add(resetIn).Unix(), 10))
	header.Set("Date", probeAnswerDate.Format(http.TimeFormat))
	return header
}

// TestThrottleAnswer_ReadsTheRateLimitHeaders holds the reading of GitLab's
// rate limit headers the probe judges by, with no GitLab: a counted 401, a
// 429 with Retry-After, and an answer no throttle counted, whose missing
// headers read as no throttle and no count rather than as zeros a probe would
// take for a count. RateLimit-Reset and Date are read by nothing, since they
// describe a window aligned to the epoch rather than the counter's.
func TestThrottleAnswer_ReadsTheRateLimitHeaders(t *testing.T) {
	throttled := countedHeader(probeLimit+1, 80*time.Second)
	throttled.Set("Retry-After", "80")
	tests := []struct {
		name   string
		status int
		header http.Header
		want   throttleAnswer
	}{
		{
			name:   "a counted refusal",
			status: http.StatusUnauthorized,
			header: countedHeader(2, 90*time.Second),
			want:   throttleAnswer{Label: "a counted refusal", Status: http.StatusUnauthorized, Name: unauthenticatedAPIThrottle, Observed: 2, Remaining: "2"},
		},
		{
			name:   "a throttled request",
			status: http.StatusTooManyRequests,
			header: throttled,
			want: throttleAnswer{
				Label: "a throttled request", Status: http.StatusTooManyRequests, Name: unauthenticatedAPIThrottle, Observed: probeLimit + 1, Remaining: "0",
				RetryAfter: 80 * time.Second,
			},
		},
		{
			name:   "an answer no throttle counted",
			status: http.StatusUnauthorized,
			header: http.Header{"Date": {probeAnswerDate.Format(http.TimeFormat)}},
			want:   throttleAnswer{Label: "an answer no throttle counted", Status: http.StatusUnauthorized, Observed: -1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: tt.header}
			if got := readThrottleAnswer(tt.name, resp); got != tt.want {
				t.Errorf("readThrottleAnswer() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestThrottleWindow_OpensWithItsFirstCountedRequest holds the probe to the
// window GitLab 19.5 counts in, with no GitLab, using the three calibrations
// of the probe's recorded run: a request counted first opens a window
// however little the epoch-aligned RateLimit-Reset says is left of one, a
// request counted second opens none however much it says is left, and the
// window a request was counted in has ended a period after the probe received
// its answer, whatever the reset says.
//
// The first two cases are why the record's calibration 2 was counted second:
// a probe that took the reset for the window's end waited 52 s after
// calibration 1 for a fresh window, and sent calibration 2 into the window
// calibration 1 had opened.
func TestThrottleWindow_OpensWithItsFirstCountedRequest(t *testing.T) {
	received := time.Date(2026, 10, 20, 17, 6, 2, 122_000_000, time.UTC)
	tests := []struct {
		name     string
		observed int
		resetIn  time.Duration
		opens    bool
	}{
		{name: "calibration 1, counted first with the epoch's window 51 s from its end", observed: 1, resetIn: 51 * time.Second, opens: true},
		{name: "calibration 2, counted second with the epoch's window 119 s from its end", observed: 2, resetIn: 119 * time.Second, opens: false},
		{name: "calibration 3, counted first with the epoch's window 118 s from its end", observed: 1, resetIn: 118 * time.Second, opens: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: http.StatusUnauthorized, Header: countedHeader(tt.observed, tt.resetIn)}
			answer := readThrottleAnswer(tt.name, resp)
			answer.At = received
			if got := opensWindow(answer); got != tt.opens {
				t.Errorf("opensWindow(%s) = %t, want %t", answer, got, tt.opens)
			}
			if got, want := answer.windowEndsBy(), received.Add(probePeriod*time.Second+windowMargin); !got.Equal(want) {
				t.Errorf("windowEndsBy() = %s, want %s: a period and the margin after the answer reached the probe", got, want)
			}
		})
	}
}

// TestNextWindowProblem_TellsABanFromAnUnexpectedCount holds the verdict on
// the invalid token the probe sends once the throttled window has ended, with
// no GitLab: a 401 counted afresh is no problem, a refusal of the address is
// a ban, and a 401 counted as anything but the window's first request is a
// count the probe did not expect, which says nothing about a ban.
func TestNextWindowProblem_TellsABanFromAnUnexpectedCount(t *testing.T) {
	tests := []struct {
		name   string
		status int
		header http.Header
		// want is a fragment of the problem, empty when there is none.
		want string
		ban  bool
	}{
		{name: "counted afresh", status: http.StatusUnauthorized, header: countedHeader(1, 110*time.Second)},
		{name: "still throttled", status: http.StatusTooManyRequests, header: countedHeader(probeLimit+4, 100*time.Second), want: "which is a ban", ban: true},
		{name: "forbidden", status: http.StatusForbidden, header: http.Header{}, want: "which is a ban", ban: true},
		{name: "counted second", status: http.StatusUnauthorized, header: countedHeader(2, 110*time.Second), want: "counted as request 2 of its window, want 1"},
		{name: "counted by no throttle", status: http.StatusUnauthorized, header: http.Header{}, want: "want it counted by " + unauthenticatedAPIThrottle},
		{name: "a server error", status: http.StatusInternalServerError, header: http.Header{}, want: "want 401"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{StatusCode: tt.status, Header: tt.header}
			problem := nextWindowProblem(readThrottleAnswer(tt.name, resp))
			switch {
			case tt.want == "" && problem != "":
				t.Errorf("nextWindowProblem() = %q, want none", problem)
			case !strings.Contains(problem, tt.want):
				t.Errorf("nextWindowProblem() = %q, want one containing %q", problem, tt.want)
			}
			if names := strings.Contains(problem, "is a ban"); names != tt.ban {
				t.Errorf("nextWindowProblem() = %q, naming a ban %t, want %t", problem, names, tt.ban)
			}
		})
	}
}
