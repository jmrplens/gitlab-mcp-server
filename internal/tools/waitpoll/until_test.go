package waitpoll

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// untilState is what a test's Read hands [Until]: which read produced it,
// and whether a test's Landed accepts it.
type untilState struct {
	Read   int
	Landed bool
}

// readSequence answers the reads of one test in order, repeating the last
// answer once the list is spent, and counts them.
type readSequence struct {
	answers []readAnswer
	reads   int
}

// readAnswer is one answer of a [readSequence]: whether the state lands, and
// the error the read fails with.
type readAnswer struct {
	landed bool
	err    error
}

// read is the Read a test hands [Until].
func (s *readSequence) read(context.Context) (untilState, error) {
	answer := s.answers[min(s.reads, len(s.answers)-1)]
	s.reads++
	return untilState{Read: s.reads, Landed: answer.landed}, answer.err
}

// landed is the Landed a test hands [Until].
func landed(state untilState) bool { return state.Landed }

// untilOptions builds options that read fast and wait at most bound.
func untilOptions(seq *readSequence, bound time.Duration) UntilOptions[untilState] {
	return UntilOptions[untilState]{
		Interval: time.Millisecond,
		Bound:    bound,
		Read:     seq.read,
		Landed:   landed,
	}
}

// TestUntil_LandsOnALaterRead_AnswersThatState verifies that the wait keeps
// reading while the state has not landed and answers the first state that
// has, with the read that produced it.
func TestUntil_LandsOnALaterRead_AnswersThatState(t *testing.T) {
	seq := &readSequence{answers: []readAnswer{{}, {}, {landed: true}}}

	state, ok, err := Until(context.Background(), untilOptions(seq, 10*time.Second))
	if err != nil {
		t.Fatalf("Until() error = %v, want nil", err)
	}
	if !ok || state != (untilState{Read: 3, Landed: true}) {
		t.Errorf("Until() = %+v, %v, want the third read and true", state, ok)
	}
	if seq.reads != 3 {
		t.Errorf("reads = %d, want 3: the wait stops at the state that landed", seq.reads)
	}
}

// TestUntil_FailedRead_IsNotAnAnswer verifies that a read which fails is
// waited through, even when the state it came back with would land: a state
// that arrived with an error is not one GitLab served.
func TestUntil_FailedRead_IsNotAnAnswer(t *testing.T) {
	seq := &readSequence{answers: []readAnswer{
		{landed: true, err: errors.New("502 Bad Gateway")},
		{landed: true},
	}}

	state, ok, err := Until(context.Background(), untilOptions(seq, 10*time.Second))
	if err != nil {
		t.Fatalf("Until() error = %v, want nil: a failed read does not end the wait", err)
	}
	if !ok || state.Read != 2 {
		t.Errorf("Until() = %+v, %v, want the second read and true", state, ok)
	}
}

// TestUntil_NeverLands_AnswersNotLandedAtTheBound verifies that a state that
// never lands ends the wait at the bound with the zero state, false and no
// error: running out of time is an answer, not a failure.
func TestUntil_NeverLands_AnswersNotLandedAtTheBound(t *testing.T) {
	seq := &readSequence{answers: []readAnswer{{}}}
	bound := 50 * time.Millisecond

	start := time.Now()
	state, ok, err := Until(context.Background(), untilOptions(seq, bound))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Until() error = %v, want nil at the bound", err)
	}
	if ok || state != (untilState{}) {
		t.Errorf("Until() = %+v, %v, want the zero state and false", state, ok)
	}
	if seq.reads == 0 {
		t.Error("reads = 0, want the wait to have read before the bound ran out")
	}
	if elapsed < bound {
		t.Errorf("Until() returned after %v, before the %v bound", elapsed, bound)
	}
}

// TestUntil_CallerContextEnds_AnswersItsError verifies that the caller's
// context ending stops the wait with that context's error rather than the
// not-landed answer the bound gives.
func TestUntil_CallerContextEnds_AnswersItsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seq := &readSequence{answers: []readAnswer{{}}}
	opts := untilOptions(seq, 10*time.Second)
	opts.Read = func(readCtx context.Context) (untilState, error) {
		cancel()
		return seq.read(readCtx)
	}

	_, ok, err := Until(ctx, opts)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Until() error = %v, want context.Canceled", err)
	}
	if ok {
		t.Error("Until() reported the state landed after the caller went away")
	}
}

// TestUntil_Read_RunsUnderTheBound verifies that each read is handed a
// context carrying the bound's deadline, so a read still in flight when the
// bound runs out is abandoned rather than awaited.
func TestUntil_Read_RunsUnderTheBound(t *testing.T) {
	bound := 10 * time.Second
	var deadline time.Time
	var hasDeadline bool
	opts := UntilOptions[untilState]{
		Interval: time.Millisecond,
		Bound:    bound,
		Read: func(readCtx context.Context) (untilState, error) {
			deadline, hasDeadline = readCtx.Deadline()
			return untilState{Landed: true}, nil
		},
		Landed: landed,
	}

	start := time.Now()
	if _, _, err := Until(context.Background(), opts); err != nil {
		t.Fatalf("Until() error = %v", err)
	}
	if !hasDeadline {
		t.Fatal("the read's context carries no deadline, want the bound's")
	}
	if deadline.After(start.Add(bound + time.Second)) {
		t.Errorf("the read's deadline is %v after the start, want at most the %v bound", deadline.Sub(start), bound)
	}
}

// TestUntil_FirstRead_WaitsAnInterval verifies that the first read happens
// one interval after the call rather than at once: the caller has just been
// told the change was accepted, so reading immediately would only find it
// pending.
func TestUntil_FirstRead_WaitsAnInterval(t *testing.T) {
	interval := 40 * time.Millisecond
	var firstRead time.Duration
	start := time.Now()
	opts := UntilOptions[untilState]{
		Interval: interval,
		Bound:    10 * time.Second,
		Read: func(context.Context) (untilState, error) {
			firstRead = time.Since(start)
			return untilState{Landed: true}, nil
		},
		Landed: landed,
	}

	if _, _, err := Until(context.Background(), opts); err != nil {
		t.Fatalf("Until() error = %v", err)
	}
	if firstRead < interval {
		t.Errorf("first read after %v, want at least the %v interval", firstRead, interval)
	}
}

// TestUntil_ProgressNotifications_NumberEachRead verifies that each read is
// announced by one progress notification carrying its number and the
// options' message. Every other test leaves Request nil, so the tracker is
// inactive and the progress path is a no-op nothing else observes.
func TestUntil_ProgressNotifications_NumberEachRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const message = "Waiting for GitLab to apply the change"

	server := mcp.NewServer(&mcp.Implementation{Name: "until-test-server", Version: "0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "until_with_progress"},
		func(callCtx context.Context, req *mcp.CallToolRequest, _ progressToolInput) (*mcp.CallToolResult, progressToolOutput, error) {
			seq := &readSequence{answers: []readAnswer{{}, {}, {landed: true}}}
			opts := untilOptions(seq, 10*time.Second)
			opts.Request = req
			opts.Message = message
			if _, _, err := Until(callCtx, opts); err != nil {
				return nil, progressToolOutput{}, err
			}
			return nil, progressToolOutput{FinalStatus: "landed"}, nil
		})

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	notifications := make(chan *mcp.ProgressNotificationParams, 8)
	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "until-test-client"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			select {
			case notifications <- req.Params:
			default:
			}
		},
	})
	clientSession, err := mcpClient.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() {
		clientSession.Close()
		_ = serverSession.Wait()
	})

	res, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "until_with_progress",
		Arguments: map[string]any{},
		Meta:      mcp.Meta{"progressToken": "until-progress-token"},
	})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool result reports an error: %+v", res.Content)
	}

	for read := 1; read <= 3; read++ {
		select {
		case params := <-notifications:
			if params.Progress != float64(read) {
				t.Errorf("notification %d: progress = %v, want %d", read, params.Progress, read)
			}
			if params.Message != message {
				t.Errorf("notification %d: message = %q, want %q", read, params.Message, message)
			}
		case <-ctx.Done():
			t.Fatalf("timed out waiting for progress notification %d", read)
		}
	}
}

// TestTransferTiming_StaysUnderTheClientTimeout pins the transfer timing and
// what it is chosen against: a bound under the 60 seconds a common MCP client
// waits for a tool call, and an interval that reads more than once inside it.
func TestTransferTiming_StaysUnderTheClientTimeout(t *testing.T) {
	if TransferBound != 45*time.Second {
		t.Errorf("TransferBound = %v, want 45s", TransferBound)
	}
	if TransferInterval != 2*time.Second {
		t.Errorf("TransferInterval = %v, want 2s", TransferInterval)
	}
	if clientTimeout := 60 * time.Second; TransferBound >= clientTimeout {
		t.Errorf("TransferBound = %v, want it under the %v client timeout", TransferBound, clientTimeout)
	}
	if reads := TransferBound / TransferInterval; reads < 2 {
		t.Errorf("TransferBound / TransferInterval = %d reads, want several", reads)
	}
}
