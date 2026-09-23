//go:build e2e

// verbs_test.go covers the parts of the non-tool verbs that can be tested
// without a GitLab: the fan-out that decides which test a resource-updated
// notification reaches, and the resource reads a stub instance can answer.
//
// The fan-out is the piece worth testing on its own. One session is shared by
// many tests and the SDK delivers a notification to the client rather than to
// whoever subscribed, so a test waiting for its own resource to change is
// waiting on a channel this code chose.

package harness

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil/e2ecalls"
)

// The two resources the in-process subscription tests ask for: one the stub
// server agrees to watch and one it declines.
const (
	watchedURI  = "gitlab://project/7"
	declinedURI = "gitlab://project/8"
)

// errStubDeclined is what the in-process stub answers a subscription it
// declines with, standing in for the real server's failed first read.
var errStubDeclined = errors.New("stub: the first read of the resource failed")

// subscriptionStub is an SDK server in this process that declines a
// subscription to the URIs it was told to and accepts every other, counting
// what it was asked. Its handlers run on the SDK's goroutines, so they count
// with atomics and never touch a test.
type subscriptionStub struct {
	server       *mcp.Server
	declined     map[string]bool
	subscribes   atomic.Int64
	unsubscribes atomic.Int64
}

// newSubscriptionStub builds one that speaks only the given protocol revisions
// when any are named, and every revision the SDK knows otherwise.
func newSubscriptionStub(versions []string, declined ...string) *subscriptionStub {
	stub := &subscriptionStub{declined: map[string]bool{}}
	for _, uri := range declined {
		stub.declined[uri] = true
	}
	stub.server = mcp.NewServer(&mcp.Implementation{Name: "subscription-stub", Version: "1"}, &mcp.ServerOptions{
		SupportedProtocolVersions: versions,
		SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
			stub.subscribes.Add(1)
			if stub.declined[req.Params.URI] {
				return errStubDeclined
			}
			return nil
		},
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error {
			stub.unsubscribes.Add(1)
			return nil
		},
	})
	// A resource, because a server declares the subscribe capability only
	// inside the resources capability, and a listen asking for a resource
	// subscription is acknowledged only by a server that declares it.
	stub.server.AddResource(&mcp.Resource{URI: watchedURI, Name: "watched"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: watchedURI, Text: "{}"}}}, nil
		})
	return stub
}

// inProcessSession connects a harness session to an SDK server in this
// process, with the recording middleware and the notifiers a started session
// has, so a subscribe can be answered however a test needs without a binary
// or a GitLab.
func inProcessSession(t *testing.T, env *Env, server *mcp.Server) *Session {
	t.Helper()

	const label = "dynamic-default-full-in-process"
	conn := &sessionConn{
		label: label,
		cfg:   ServerConfig{}.normalized(),
		inst:  env.inst,
		// A process that was never started, so a verb that fails and
		// describes the child it would have died of says there is none
		// rather than reading a process that is not there.
		proc:        newServerProcess(label, "", childEnv{}),
		notifier:    newUpdateNotifier(),
		acks:        newUpdateNotifier(),
		progress:    newProgressCollector(),
		subscribers: newSubscriberIndex(),
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "harness-in-process", Version: "1"}, conn.clientOptions())
	client.AddSendingMiddleware(conn.recordSending())
	client.AddReceivingMiddleware(conn.recordReceiving())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("connecting the in-process server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connecting to the in-process server: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	conn.session = session
	return &Session{env: env, conn: conn}
}

// shortAckWait sets how long a subscribe waits for its acknowledgement for
// one test.
func shortAckWait(t *testing.T, wait time.Duration) {
	t.Helper()
	saved := subscribeAckTimeout
	subscribeAckTimeout = wait
	t.Cleanup(func() { subscribeAckTimeout = saved })
}

// subscribeLines returns the subscribe lines a test recorded, finishing its
// record.
func subscribeLines(env *Env) []*e2ecalls.Call {
	var lines []*e2ecalls.Call
	for _, line := range env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed) {
		if call, isCall := line.(*e2ecalls.Call); isCall && call.Method == methodSubscribe {
			lines = append(lines, call)
		}
	}
	return lines
}

// describeSubscribeLines spells subscribe lines as their target, expectation
// and outcome, for a failure message: the lines are pointers, which %v prints
// as addresses.
func describeSubscribeLines(lines []*e2ecalls.Call) string {
	described := make([]string, 0, len(lines))
	for _, line := range lines {
		described = append(described, line.Target+" expecting "+line.Expectation+": "+line.Outcome)
	}
	return "[" + strings.Join(described, ", ") + "]"
}

// awaitCount waits for an atomic counter a server goroutine moves to reach a
// value, failing the test when it does not in time.
func awaitCount(t *testing.T, counter *atomic.Int64, want int64, what string) {
	t.Helper()
	err := Poll(t.Context(), 10*time.Millisecond, 5*time.Second, func() (bool, string, error) {
		got := counter.Load()
		return got == want, what + " = " + strconv.FormatInt(got, 10), nil
	})
	if err != nil {
		t.Fatalf("waiting for %s to reach %d: %v", what, want, err)
	}
}

// fatalCaseEnv names the case [TestFatalCase_NamedByTheParent_RunsInAChildProcess]
// runs, and is set only by a parent test that expects that child to fail.
const fatalCaseEnv = "E2E_HARNESS_FATAL_CASE"

// fatalCaseReturned is what a child prints when the verb it drove returned
// instead of ending its test, which is the one outcome a parent must tell
// apart from the failure it expects.
const fatalCaseReturned = "the verb returned instead of failing its test"

// fatalCases drive a verb into the branch that fails the test calling it.
//
// A verb that fails its test calls t.Fatalf, which ends the test that called
// it, so the branch can be watched only from outside the process it ends: a
// parent runs the case in a child and reads what the child printed.
var fatalCases = map[string]func(t *testing.T){
	"complete refused": func(t *testing.T) {
		t.Helper()
		server := mcp.NewServer(&mcp.Implementation{Name: "completion-stub", Version: "1"}, &mcp.ServerOptions{
			CompletionHandler: func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
				return nil, errStubCompletion
			},
		})
		session := inProcessSession(t, newEnv(t, offlineInstance()), server)
		session.CompletePrompt("summarize", "project_id", "")
	},
	"subscribe not acknowledged": func(t *testing.T) {
		t.Helper()
		shortAckWait(t, 200*time.Millisecond)
		session := inProcessSession(t, newEnv(t, offlineInstance()), newSubscriptionStub(nil, declinedURI).server)
		session.Subscribe(declinedURI)
	},
}

// errStubCompletion is what the in-process completion stub answers every
// completion with.
var errStubCompletion = errors.New("stub: no completion for this argument")

// TestFatalCase_NamedByTheParent_RunsInAChildProcess runs the fatal case a
// parent test named, and skips when none did, which is every ordinary run.
func TestFatalCase_NamedByTheParent_RunsInAChildProcess(t *testing.T) {
	name := os.Getenv(fatalCaseEnv)
	if name == "" {
		t.Skip("runs only as the child of a test that expects it to fail")
	}
	run, known := fatalCases[name]
	if !known {
		t.Fatalf("no fatal case is named %q", name)
	}
	run(t)
	t.Error(fatalCaseReturned)
}

// runFatalCase runs one fatal case in a child test process and returns what
// the child printed, failing unless the child failed its test by ending it.
//
// The child is this test binary asked for the one test above. The coverage
// directory this process was given is handed on, so what the child executes
// is counted in the profile of the run that started it.
func runFatalCase(t *testing.T, name string) string {
	t.Helper()
	args := []string{"-test.run=^TestFatalCase_NamedByTheParent_RunsInAChildProcess$", "-test.count=1"}
	for _, arg := range os.Args[1:] {
		if strings.HasPrefix(arg, "-test.gocoverdir=") {
			args = append(args, arg)
		}
	}
	// #nosec G204 G702 -- the binary is this test binary, and the arguments are
	// constants plus the coverage flag it was itself started with.
	cmd := exec.CommandContext(t.Context(), os.Args[0], args...)
	cmd.Env = append(os.Environ(), fatalCaseEnv+"="+name)
	out, err := cmd.CombinedOutput()
	if _, exited := errors.AsType[*exec.ExitError](err); !exited {
		t.Fatalf("the %q case did not fail its child test (%v):\n%s", name, err, out)
	}
	if strings.Contains(string(out), fatalCaseReturned) {
		t.Fatalf("in the %q case %s:\n%s", name, fatalCaseReturned, out)
	}
	return string(out)
}

// TestSession_Complete_Refused_FailsTheTestNamingTheArgument checks the
// completion verb's refusal: a completion the server answers with an error
// fails the test, naming the reference and argument it asked about and the
// server's own reason.
func TestSession_Complete_Refused_FailsTheTestNamingTheArgument(t *testing.T) {
	out := runFatalCase(t, "complete refused")

	if !strings.Contains(out, "completion/complete summarize project_id: ") || !strings.Contains(out, errStubCompletion.Error()) {
		t.Errorf("the child printed:\n%s\nwant the refused completion named with the server's reason", out)
	}
}

// TestSession_Subscribe_NotAcknowledged_FailsTheTestNamingTheURI checks the
// subscribe verb's refusal on protocol 2026-07-28: a subscription the server
// never acknowledges fails the test, naming the resource and why it counts as
// refused, rather than handing back a subscription nothing will ever notify.
func TestSession_Subscribe_NotAcknowledged_FailsTheTestNamingTheURI(t *testing.T) {
	out := runFatalCase(t, "subscribe not acknowledged")

	if !strings.Contains(out, "resources/subscribe "+declinedURI+": ") || !strings.Contains(out, errNotAcknowledged.Error()) {
		t.Errorf("the child printed:\n%s\nwant the unacknowledged subscription named with the reason", out)
	}
}

// TestUpdateNotifier_OneResource_ReachesEveryWatcher checks that two tests
// watching one resource are both told when it changes.
func TestUpdateNotifier_OneResource_ReachesEveryWatcher(t *testing.T) {
	notifier := newUpdateNotifier()
	first := notifier.watch("gitlab://project/7")
	second := notifier.watch("gitlab://project/7")

	notifier.deliver("gitlab://project/7")

	watchers := map[string]chan struct{}{"first": first, "second": second}
	for name, updates := range watchers {
		t.Run(name, func(t *testing.T) {
			select {
			case <-updates:
			default:
				t.Error("a watcher of the changed resource was not told")
			}
		})
	}
}

// TestUpdateNotifier_AnotherResource_ReachesNobody checks that a watcher is
// only told about the resource it asked for, which is what lets one session
// carry several tests' subscriptions.
func TestUpdateNotifier_AnotherResource_ReachesNobody(t *testing.T) {
	notifier := newUpdateNotifier()
	updates := notifier.watch("gitlab://project/7")

	notifier.deliver("gitlab://project/8")

	select {
	case <-updates:
		t.Error("a watcher of project 7 was told about project 8")
	default:
	}
}

// TestUpdateNotifier_Forget_StopsDelivery checks that a subscription's cleanup
// really unhooks it, so a test that has ended cannot be handed a notification
// for a resource it no longer cares about.
func TestUpdateNotifier_Forget_StopsDelivery(t *testing.T) {
	notifier := newUpdateNotifier()
	kept := notifier.watch("gitlab://project/7")
	dropped := notifier.watch("gitlab://project/7")

	notifier.forget("gitlab://project/7", dropped)
	if watching := notifier.watching("gitlab://project/7"); watching != 1 {
		t.Fatalf("the resource has %d watchers after one was dropped, want 1", watching)
	}
	notifier.deliver("gitlab://project/7")

	select {
	case <-dropped:
		t.Error("a dropped watcher was still delivered to")
	default:
	}
	select {
	case <-kept:
	default:
		t.Error("the remaining watcher was not delivered to")
	}
}

// TestUpdateNotifier_LastWatcherForgotten_LeavesNoEntry checks that a resource
// nobody watches any more is dropped rather than accumulating for the life of
// the run.
func TestUpdateNotifier_LastWatcherForgotten_LeavesNoEntry(t *testing.T) {
	notifier := newUpdateNotifier()
	updates := notifier.watch("gitlab://project/7")

	notifier.forget("gitlab://project/7", updates)

	if watching := notifier.watching("gitlab://project/7"); watching != 0 {
		t.Errorf("the resource still has %d watchers", watching)
	}
	// Delivering to nobody must not panic: a notification can arrive after the
	// test that subscribed has ended and its cleanup has run.
	notifier.deliver("gitlab://project/7")
}

// TestUpdateNotifier_SlowWatcher_IsNotBlockedOn checks that a test that never
// read its updates cannot stop the session's notification handler.
//
// The handler runs on the client's own goroutine, so a blocking send there
// would stall every other notification the session delivers, including those
// other tests are waiting for.
func TestUpdateNotifier_SlowWatcher_IsNotBlockedOn(t *testing.T) {
	notifier := newUpdateNotifier()
	notifier.watch("gitlab://project/7")

	for range updateBuffer * 3 {
		notifier.deliver("gitlab://project/7")
	}
}

// TestSession_ReadResource_ReadsThroughTheRealBinary checks the resource verb
// end to end against a session of the real server.
//
// gitlab://tools is the one resource every surface serves and that needs
// nothing of GitLab, so it is what this can assert on with a stub behind it.
func TestSession_ReadResource_ReadsThroughTheRealBinary(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Private: true})

	result := session.ReadResource("gitlab://tools")

	if len(result.Contents) == 0 {
		t.Fatal("gitlab://tools answered with no content")
	}
	if !strings.Contains(result.Contents[0].Text, "gitlab_execute_action") {
		t.Errorf("the tool manifest does not mention the dynamic execute tool: %.200s", result.Contents[0].Text)
	}
}

// TestSession_TryReadResource_UnknownURI_ReportsInsteadOfFailing checks the
// escape hatch a test of a refusal needs.
func TestSession_TryReadResource_UnknownURI_ReportsInsteadOfFailing(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Private: true})

	_, err := session.TryReadResource("gitlab://not-a-resource")

	if err == nil {
		t.Fatal("a resource the server does not serve was read without an error")
	}
}

// TestSession_Raw_SendsExactlyWhatItIsGiven checks that the protocol escape
// hatch names no action and applies no projection.
//
// It is what the protocol tests need: a tool name no surface registers, sent
// as written, so the refusal the server makes is the subject.
func TestSession_Raw_SendsExactlyWhatItIsGiven(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Private: true})

	result, err := session.Raw(&mcp.CallToolParams{Name: "gitlab_not_a_tool", Arguments: map[string]any{}})

	if err == nil && (result == nil || !result.IsError) {
		t.Fatal("a tool nothing registers was accepted")
	}
}

// TestSession_Prompts_AreListedOnTheFullCapabilitySurface checks the listings
// a session records when it starts, which the coverage record's session line
// is built from.
func TestSession_Prompts_AreListedOnTheFullCapabilitySurface(t *testing.T) {
	inst := stubInstance(t)
	env := newEnv(t, inst)
	session := env.Session(ServerConfig{Private: true})

	cases := []struct {
		name string
		got  int
	}{
		{name: "tools", got: len(session.Tools())},
		{name: "resources", got: len(session.Resources())},
		{name: "resource templates", got: len(session.ResourceTemplates())},
		{name: "prompts", got: len(session.Prompts())},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got == 0 {
				t.Errorf("the session listed no %s, and the full capability surface serves several", testCase.name)
			}
		})
	}
}

// TestCompletionCallTarget_JoinsReferenceAndArgument pins the spelling a
// completion call's record and a session line's completion list share.
func TestCompletionCallTarget_JoinsReferenceAndArgument(t *testing.T) {
	if got, want := completionCallTarget("gitlab://project/{project_id}", "project_id"), "gitlab://project/{project_id} project_id"; got != want {
		t.Errorf("completionCallTarget() = %q, want %q", got, want)
	}
}

// TestSession_TrySubscribe_Acknowledged_IsRecordedAsAccepted checks the
// ordinary case on protocol 2026-07-28: the server acknowledges the URI, the
// verb hands back a subscription, and the record says the subscribe was
// answered, which is what the coverage command credits.
func TestSession_TrySubscribe_Acknowledged_IsRecordedAsAccepted(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub(nil)
	session := inProcessSession(t, env, stub.server)
	if !subscribesByListening(session.conn.client()) {
		t.Fatal("the in-process session did not negotiate a listening protocol, so this test would not reach the acknowledgement")
	}

	subscription, err := session.TrySubscribe(watchedURI)
	if err != nil {
		t.Fatalf("TrySubscribe() error = %v, want the acknowledged subscription", err)
	}
	if subscription.URI() != watchedURI || stub.subscribes.Load() != 1 {
		t.Errorf("subscription for %q after %d subscribes, want %q after one", subscription.URI(), stub.subscribes.Load(), watchedURI)
	}
	lines := subscribeLines(env)
	if len(lines) != 1 || lines[0].Outcome != e2ecalls.OutcomeOK || lines[0].Expectation != ExpectationAny || lines[0].Target != watchedURI {
		t.Errorf("subscribe lines = %s, want one ok line for %s expecting any answer", describeSubscribeLines(lines), watchedURI)
	}
}

// TestSession_Subscribe_Acknowledged_HandsBackAWatchedSubscription checks the
// verb that fails the test on a refusal, on the path where there is none: the
// subscription it hands back is watched on the session until it is closed.
func TestSession_Subscribe_Acknowledged_HandsBackAWatchedSubscription(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	env := newEnv(t, offlineInstance())
	session := inProcessSession(t, env, newSubscriptionStub(nil).server)

	subscription := session.Subscribe(watchedURI)

	if rec, watched := session.conn.subscribers.recorderFor(watchedURI); !watched || rec != env.recorder {
		t.Error("the subscription is not recorded as this test's")
	}
	if watching := session.conn.notifier.watching(watchedURI); watching != 1 {
		t.Errorf("the resource has %d update watchers, want the subscription's one", watching)
	}
	if lines := subscribeLines(env); len(lines) != 1 || lines[0].Expectation != ExpectationOK {
		t.Errorf("subscribe lines = %s, want one expecting success", describeSubscribeLines(lines))
	}
	subscription.Close()
	if _, watched := session.conn.subscribers.recorderFor(watchedURI); watched {
		t.Error("a closed subscription is still recorded as watched")
	}
	if watching := session.conn.notifier.watching(watchedURI); watching != 0 {
		t.Errorf("a closed subscription still has %d update watchers", watching)
	}
}

// TestSession_TrySubscribe_NotAcknowledged_IsReportedAndCanBeAskedAgain checks
// the case this verb exists for: the server declines, which on protocol
// 2026-07-28 the client learns only because no acknowledgement comes. The
// refusal is returned and recorded as one rather than credited, and the URI
// can be asked for again, which it could not if the SDK still held the listen
// it opened for the refused one.
func TestSession_TrySubscribe_NotAcknowledged_IsReportedAndCanBeAskedAgain(t *testing.T) {
	shortAckWait(t, 300*time.Millisecond)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub(nil, declinedURI)
	session := inProcessSession(t, env, stub.server)

	for attempt := range 2 {
		subscription, err := session.TrySubscribe(declinedURI)
		if !errors.Is(err, errNotAcknowledged) || subscription != nil {
			t.Fatalf("attempt %d: TrySubscribe() = %v, %v; want no subscription and errNotAcknowledged", attempt+1, subscription, err)
		}
	}
	if got := stub.subscribes.Load(); got != 2 {
		t.Errorf("the server was asked %d times, want the second attempt to reach it too", got)
	}
	if _, watched := session.conn.subscribers.recorderFor(declinedURI); watched {
		t.Error("a declined subscription is still recorded as watched")
	}
	if watching := session.conn.notifier.watching(declinedURI); watching != 0 {
		t.Errorf("a declined subscription left %d update watchers", watching)
	}
	lines := subscribeLines(env)
	if len(lines) != 2 {
		t.Fatalf("recorded %d subscribe lines, want one per attempt", len(lines))
	}
	for _, line := range lines {
		if line.Outcome != e2ecalls.OutcomeProtocolError {
			t.Errorf("a declined subscribe was recorded as %q, want %q", line.Outcome, e2ecalls.OutcomeProtocolError)
		}
	}
}

// TestSession_TrySubscribe_OlderProtocol_TakesTheRequestsAnswer checks the
// protocol where a subscribe is an ordinary request: its error is the refusal
// and nothing waits for an acknowledgement, which would never come. The wait
// is set to nothing, so a verb that waited anyway would decline every
// subscribe here.
func TestSession_TrySubscribe_OlderProtocol_TakesTheRequestsAnswer(t *testing.T) {
	shortAckWait(t, time.Nanosecond)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub([]string{"2025-11-25"}, declinedURI)
	session := inProcessSession(t, env, stub.server)
	if subscribesByListening(session.conn.client()) {
		t.Fatal("the in-process session negotiated a listening protocol, so this test would not reach the older one")
	}

	subscription, err := session.TrySubscribe(watchedURI)
	if err != nil {
		t.Fatalf("TrySubscribe() error = %v, want the request's own acceptance", err)
	}
	_, err = session.TrySubscribe(declinedURI)
	if err == nil || errors.Is(err, errNotAcknowledged) || !strings.Contains(err.Error(), errStubDeclined.Error()) {
		t.Errorf("TrySubscribe(declined) error = %v, want the server's own refusal", err)
	}
	if got := stub.unsubscribes.Load(); got != 0 {
		t.Errorf("%d unsubscribes were sent before anything was closed, want none: a refused request subscribed nothing", got)
	}
	subscription.Close()
	if got := stub.unsubscribes.Load(); got != 1 {
		t.Errorf("closing the subscription sent %d unsubscribes, want one", got)
	}
	// One line per subscribe sent, written by the sending middleware: the verb
	// records only where the protocol hides the request from it.
	lines := subscribeLines(env)
	if len(lines) != 2 || lines[0].Outcome != e2ecalls.OutcomeOK || lines[1].Outcome != e2ecalls.OutcomeProtocolError {
		t.Fatalf("subscribe lines = %s, want one ok line and one protocol_error line, one per subscribe sent", describeSubscribeLines(lines))
	}
	if lines[0].Target != watchedURI || lines[1].Target != declinedURI {
		t.Errorf("recorded subscribe targets %q and %q, want %q and %q", lines[0].Target, lines[1].Target, watchedURI, declinedURI)
	}
}

// TestSession_TrySubscribe_SecondOnOneSession_IsTurnedAwayBeforeAnythingIsSent
// checks the one subscription per URI per session: the SDK would answer the
// second from the first one's listen without asking the server, so the verb
// refuses it itself, sends nothing and records nothing, and lets the URI be
// claimed again once the first is closed.
//
// The stub's handlers only count, so what this proves is the harness's own
// claim and release, and that a released URI is asked of the server again. It
// cannot prove that the second subscription delivers: against the real server
// on protocol 2026-07-28 a re-subscribe that lands before the first listen's
// teardown is acknowledged and never delivers, which is why
// [Session.TrySubscribe] tells a test re-subscribing a URI to take a private
// session per subscription.
func TestSession_TrySubscribe_SecondOnOneSession_IsTurnedAwayBeforeAnythingIsSent(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub(nil)
	session := inProcessSession(t, env, stub.server)

	first, err := session.TrySubscribe(watchedURI)
	if err != nil {
		t.Fatalf("TrySubscribe() error = %v", err)
	}
	if _, err = session.TrySubscribe(watchedURI); !errors.Is(err, errSubscribedOnSession) {
		t.Errorf("a second TrySubscribe() on the session = %v, want errSubscribedOnSession", err)
	}
	if got := stub.subscribes.Load(); got != 1 {
		t.Errorf("the server was asked %d times, want the second subscribe turned away before it", got)
	}
	first.Close()
	again, err := session.TrySubscribe(watchedURI)
	if err != nil || again == nil {
		t.Errorf("TrySubscribe() after the first was closed = %v, %v; want a subscription", again, err)
	}
	if lines := subscribeLines(env); len(lines) != 2 {
		t.Errorf("recorded %d subscribe lines, want the two that reached the server", len(lines))
	}
}

// TestSubscription_Close_ReleasesBeforeTheTestEnds checks that closing a
// subscription early tells the server, which is what keeps a sweep that
// subscribes in turn inside the watcher cap. On protocol 2026-07-28 the server
// learns it asynchronously, so the release is waited for rather than read at
// once.
func TestSubscription_Close_ReleasesBeforeTheTestEnds(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub(nil)
	session := inProcessSession(t, env, stub.server)

	subscription, err := session.TrySubscribe(watchedURI)
	if err != nil {
		t.Fatalf("TrySubscribe() error = %v", err)
	}
	subscription.Close()

	awaitCount(t, &stub.unsubscribes, 1, "server unsubscribes")
}

// TestSubscription_CloseAndCleanup_UnsubscribeOnce checks that the close a
// test calls and the one its cleanup runs are the same close: on the older
// protocol every unsubscribe is a request, so a second one would reach the
// server.
func TestSubscription_CloseAndCleanup_UnsubscribeOnce(t *testing.T) {
	shortAckWait(t, time.Nanosecond)
	stub := newSubscriptionStub([]string{"2025-11-25"})

	t.Run("closed early", func(t *testing.T) {
		env := newEnv(t, offlineInstance())
		session := inProcessSession(t, env, stub.server)
		subscription, err := session.TrySubscribe(watchedURI)
		if err != nil {
			t.Fatalf("TrySubscribe() error = %v", err)
		}
		subscription.Close()
		subscription.Close()
	})

	if got := stub.unsubscribes.Load(); got != 1 {
		t.Errorf("two closes and the cleanup sent %d unsubscribes, want one", got)
	}
}

// TestSession_TrySubscribe_TestContextEnded_StopsWaiting checks that a
// subscribe whose test is already over does not sit out the whole
// acknowledgement wait.
func TestSession_TrySubscribe_TestContextEnded_StopsWaiting(t *testing.T) {
	shortAckWait(t, time.Minute)
	env := newEnv(t, offlineInstance())
	session := inProcessSession(t, env, newSubscriptionStub(nil, declinedURI).server)
	ended, cancel := context.WithCancel(context.Background())
	cancel()
	env.Ctx = ended

	_, err := session.TrySubscribe(declinedURI)

	if !errors.Is(err, context.Canceled) {
		t.Errorf("TrySubscribe() error = %v, want the test's context ending the wait", err)
	}
}

// TestSubscription_Next_ServerReportsAChange_IsDeliveredAndRecorded checks
// the delivery half end to end in process: nothing is delivered before the
// resource changes, the change wakes the subscription, and the notification is
// recorded against the test that subscribed.
func TestSubscription_Next_ServerReportsAChange_IsDeliveredAndRecorded(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	env := newEnv(t, offlineInstance())
	stub := newSubscriptionStub(nil)
	session := inProcessSession(t, env, stub.server)
	subscription := session.Subscribe(watchedURI)

	if subscription.TryNext(50 * time.Millisecond) {
		t.Fatal("an update arrived before the resource changed")
	}
	if err := stub.server.ResourceUpdated(t.Context(), &mcp.ResourceUpdatedNotificationParams{URI: watchedURI}); err != nil {
		t.Fatalf("ResourceUpdated() error = %v", err)
	}
	subscription.Next(5 * time.Second)

	var updates []*e2ecalls.Call
	for _, line := range env.recorder.finish(&capturedReporter{}, e2ecalls.StatusPassed) {
		if call, isCall := line.(*e2ecalls.Call); isCall && call.Method == methodResourceUpdated {
			updates = append(updates, call)
		}
	}
	if len(updates) != 1 || updates[0].Target != watchedURI || updates[0].Test != env.T.Name() {
		t.Errorf("update lines = %+v, want one for %s credited to this test", updates, watchedURI)
	}
}

// TestSession_TrySubscribe_RealBinary_AcknowledgesWhatItCanReadAndDeclinesTheRest
// holds the verb to the real server: a project the stub GitLab serves is
// acknowledged, and one it answers 404 for is declined, because the server's
// first read of a watched resource is its authorization check.
func TestSession_TrySubscribe_RealBinary_AcknowledgesWhatItCanReadAndDeclinesTheRest(t *testing.T) {
	shortAckWait(t, 5*time.Second)
	gitlab := startStubGitLab(t, stubRoute{pattern: "/api/v4/projects/42", handler: func(w http.ResponseWriter, _ *http.Request) {
		writeStubJSON(w, map[string]any{"id": 42, "name": "watched", "path_with_namespace": "harness/watched", "default_branch": "main"})
	}})
	env := newEnv(t, instanceForStub(t, gitlab))
	session := env.Session(ServerConfig{Private: true})

	watched, err := session.TrySubscribe("gitlab://project/42")
	if err != nil {
		t.Fatalf("TrySubscribe() of a project the server can read = %v, want it acknowledged", err)
	}
	watched.Close()
	if _, err = session.TrySubscribe("gitlab://project/43"); !errors.Is(err, errNotAcknowledged) {
		t.Errorf("TrySubscribe() of a project the server cannot read = %v, want it declined", err)
	}

	lines := subscribeLines(env)
	if len(lines) != 2 || lines[0].Outcome != e2ecalls.OutcomeOK || lines[1].Outcome != e2ecalls.OutcomeProtocolError {
		t.Errorf("subscribe lines = %s, want the readable one ok and the other a protocol error", describeSubscribeLines(lines))
	}
}
