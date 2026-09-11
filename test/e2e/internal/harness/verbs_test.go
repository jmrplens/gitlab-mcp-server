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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
