// channels_integration_test.go holds the carried-channel matrix,
// tenancy.Carriages(), to the go-sdk this module pins. It drives the SDK in
// process, over its in-memory transport and over httptest, in the protocol
// era each row names, and fails when a row lists a channel the SDK does not
// carry or the SDK carries a refusal channel the row does not list. The pull
// request that bumps go-sdk is therefore the one that fails when a channel
// moves, and the failure names the row to edit.
//
// It also holds the facts the rows rest on without being rows: which codes a
// refusal keeps on the wire, the only statuses the SDK gives an in-band error,
// how it answers an empty input-request map, what a typed nil result does, and
// how a subscription stream can be answered.
//
// It is the external test package so that go-sdk stays out of the register:
// internal/tenancy is a leaf the server links for free, and an import made by
// a test file reaches no binary.
package tenancy_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// The protocol revisions the two eras are driven as, spelled as the SDK
// spells them.
const (
	legacyVersion = "2025-11-25"
	modernVersion = "2026-07-28"
)

// The MCP methods this file sends, or watches for, by name.
const (
	methodInitialize   = "initialize"
	methodToolsList    = "tools/list"
	methodToolsCall    = "tools/call"
	methodSubscribe    = "resources/subscribe"
	methodListen       = "subscriptions/listen"
	methodCompletion   = "completion/complete"
	methodAcknowledged = "notifications/subscriptions/acknowledged"
	methodUpdated      = "notifications/resources/updated"
)

// The names the matrix server registers.
const (
	okTool       = "ok"
	narrowedTool = "narrowed"
	withheldTool = "withheld"
	unknownTool  = "unknown"
	busyTool     = "busy"
	promptName   = "p"
	resourceURI  = "test://r"
	refusedURI   = "test://refused"
	sentinelURI  = "test://sentinel"
)

const (
	// wireWait bounds every wait for something the SDK is expected to send. A
	// passing run never reaches it: it is reached only when the SDK stopped
	// sending what is waited for, which is the change this file exists to see.
	wireWait = 10 * time.Second

	// sessionTimeout is the idle timeout the expiry row is driven with: short,
	// because the test waits it out, and long enough that the SDK's timer
	// cannot fire between creating a session and serving the POST that
	// created it, which is plain computation with no I/O in between.
	sessionTimeout = 100 * time.Millisecond

	// refusalText is the message every attempted in-band refusal carries, so
	// that a message the SDK rewrote is told apart from the one it was sent.
	refusalText = "refused by the carried-channel probe"

	// endReasonKey and endReason are what a middleware stamps on the result
	// that ends a stream, the way cmd/server stamps a watch-end reason.
	endReasonKey = "tenancy.test/end"
	endReason    = "probe_ended"

	// withheldText and unknownText are the two answers a narrowed tools/call
	// gives: one naming the cause, the other calling the action unknown.
	withheldText = "the action exists, and the credential in use does not reach it"
	unknownText  = `unknown action "probe.narrowed"`

	// busyText is the error go-sdk answers a legacy request with when a
	// handler sheds load with an empty input-request map.
	busyText = "the server is busy, retry later"

	// matrixFix ends every matrix failure: the matrix is data about the SDK,
	// so the pull request that changed the SDK is the one that changes it.
	matrixFix = "Carriages() no longer describes the go-sdk this module builds against: the fix is an edit to internal/tenancy/channels.go in this same pull request, with the Refusal channels section of docs/development/tenant-policy-spec.md beside it."
)

// sdkModule is the module whose behavior this file holds the matrix to.
const sdkModule = "github.com/modelcontextprotocol/go-sdk"

// sdkName names the go-sdk this test binary was built with, so that a failure
// says which version stopped matching the matrix.
func sdkName() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "the pinned go-sdk"
	}
	for _, dep := range info.Deps {
		if dep.Path != sdkModule {
			continue
		}
		if dep.Replace != nil {
			return fmt.Sprintf("go-sdk %s (replaced by %s %s)", dep.Version, dep.Replace.Path, dep.Replace.Version)
		}
		return "go-sdk " + dep.Version
	}
	return "the pinned go-sdk"
}

// erasOf returns the protocol eras a row holds in. EraAny holds in both, and
// so does EraStdio: the stdio transport speaks both revisions, and the
// in-memory transport this file drives carries what stdio carries.
func erasOf(row tenancy.Era) []tenancy.Era {
	if row == tenancy.EraLegacy || row == tenancy.EraModern {
		return []tenancy.Era{row}
	}
	return []tenancy.Era{tenancy.EraLegacy, tenancy.EraModern}
}

// bothEras is every era a request can be driven in.
func bothEras() []tenancy.Era {
	return erasOf(tenancy.EraAny)
}

// eraName is the protocol revision an era is driven as.
func eraName(e tenancy.Era) string {
	if e == tenancy.EraModern {
		return modernVersion
	}
	return legacyVersion
}

// rowLabel names a matrix row the way a failure quotes it.
func rowLabel(row tenancy.Carriage) string {
	era := "either era"
	switch row.Era {
	case tenancy.EraLegacy:
		era = legacyVersion
	case tenancy.EraModern:
		era = modernVersion
	case tenancy.EraStdio:
		era = "stdio"
	}
	return fmt.Sprintf("{%s; %s; %s}", strings.Join(row.Methods, ", "), era, channelList(row.Channels))
}

// channelList spells a set of channels as the matrix test in channels_test.go
// does.
func channelList(cs []tenancy.Channel) string {
	names := make([]string, 0, len(cs))
	for _, c := range cs {
		names = append(names, c.String())
	}
	return strings.Join(names, ",")
}

// channels lists every channel the register defines, in its order, by walking
// the enumeration until a value has no name of its own, so that a channel
// added to the register is judged here without an edit.
func channels() []tenancy.Channel {
	var all []tenancy.Channel
	for c := tenancy.Gate; !strings.HasPrefix(c.String(), "Channel("); c++ {
		all = append(all, c)
	}
	return all
}

// narrowing reports whether a channel records a narrowed surface rather than
// a refusal (see the comment on tenancy.Carriages). A narrowing is this
// server's choice of which surface to serve, and the SDK serves whatever
// surface it is given, so a narrowing is observed only where
// narrowingObservers has an observer for it, which is on the two tool methods
// the matrix names narrowings for; there it is held both ways like a refusal.
// Elsewhere it is held one way: a row listing one this file cannot observe
// fails, and none a row leaves out is attempted.
func narrowing(c tenancy.Channel) bool {
	return c == tenancy.Withheld || c == tenancy.Absent || c == tenancy.Unknown
}

// sdkOwns reports whether go-sdk is what carries a method's refusals. The
// gate, startup and eviction are this server's own layers, which the SDK
// never sees, so driving the SDK says nothing about their rows.
func sdkOwns(method string) bool {
	return method != tenancy.MethodGate && method != tenancy.MethodStartup && method != tenancy.MethodEviction
}

// report is one attempt at a channel: whether the channel reached the client
// intact, and what did reach it.
type report struct {
	carried bool
	arrived string
}

// observation is what the SDK carried for one method in one era, per channel
// attempted.
type observation map[tenancy.Channel]report

// TestCarriages_DrivenThroughTheSDK_CarryExactlyWhatEachRowLists drives every
// row of the carried-channel matrix whose method go-sdk owns, in each era the
// row holds in, and holds it both ways: every channel the row lists reaches
// the client, and no channel this file can attempt on that method, which is
// every refusal channel the SDK could answer a method with and the narrowings
// of the two tool methods, reaches it unless the matrix lists it for that
// method and era.
//
// A channel is attempted the way a server would use it, and it counts as
// carried only when what the server sent arrives intact: an in-band error with
// the code and the text sent, a result with the error flag the server set, a
// completion the server emptied, a stream ended with the reason the server
// stamped, or no answer at all. An answer the SDK made up itself, such as its
// own -32601 for a method it removed from an era, is not the server's refusal
// having been carried.
func TestCarriages_DrivenThroughTheSDK_CarryExactlyWhatEachRowLists(t *testing.T) {
	sdk := sdkName()
	driven := 0
	for _, row := range tenancy.Carriages() {
		for _, method := range row.Methods {
			if !sdkOwns(method) {
				continue
			}
			for _, e := range erasOf(row.Era) {
				driven++
				t.Run(method+"@"+eraName(e), func(t *testing.T) {
					checkCarriage(t, sdk, row, method, e)
				})
			}
		}
	}
	if driven == 0 {
		t.Fatal("no row of Carriages() names a method go-sdk owns, so nothing held the matrix to the SDK")
	}
}

// checkCarriage holds one method of one row, in one era, to what the SDK
// carried for it.
func checkCarriage(t *testing.T, sdk string, row tenancy.Carriage, method string, e tenancy.Era) {
	t.Helper()
	seen := observe(t, method, e)
	for _, want := range row.Channels {
		got, attempted := seen[want]
		if attempted && got.carried {
			continue
		}
		t.Errorf("row %s, %s at %s: the row lists the %s channel, and %s did not carry it (expected %s, observed %s).%s %s",
			rowLabel(row), method, eraName(e), want, sdk, want, arrivedOrNothing(got, attempted), whyNotAttempted(want, attempted), matrixFix)
	}
	for _, c := range channels() {
		if got := seen[c]; got.carried && !tenancy.Carries(method, e, c) {
			t.Errorf("row %s, %s at %s: %s carried the %s channel (observed %s), and the matrix lists only %s for this method in this era. %s",
				rowLabel(row), method, eraName(e), sdk, c, got.arrived, channelList(row.Channels), matrixFix)
		}
	}
}

// arrivedOrNothing is what reached the client for an attempt, or says that no
// attempt was made.
func arrivedOrNothing(got report, attempted bool) string {
	if !attempted {
		return "no attempt"
	}
	return got.arrived
}

// whyNotAttempted explains a listed channel this file never attempts on a
// method, which is itself the answer: the SDK cannot carry it there.
func whyNotAttempted(c tenancy.Channel, attempted bool) string {
	switch {
	case attempted:
		return ""
	case c == tenancy.Gate:
		return " A gate status is written by the handler in front of the SDK, which answers an in-band error with no status of its own but 400 and 404 (TestInBandErrors_OverStreamableHTTP_TakeOnlyTheStatusesTheSDKMaps)."
	case c == tenancy.Startup:
		return " A refusal to start is the process's own, and no method carries it."
	case c == tenancy.SessionClose:
		return " A closed session ends the session rather than answering a method: the expiry and eviction rows carry it."
	case narrowing(c):
		return " This test has no observer for that narrowing on this method: add one to narrowingObservers before a row lists it."
	default:
		return " This test attempts no refusal on that channel for this method."
	}
}

// observe attempts, for one method in one era, every refusal channel this file
// knows how to attempt there, and every narrowing it can observe there.
func observe(t *testing.T, method string, e tenancy.Era) observation {
	t.Helper()
	var seen observation
	switch method {
	case tenancy.MethodExpiry:
		seen = observeExpiry(t, e)
	case methodUpdated:
		seen = observeNotification(t, e)
	default:
		seen = observeRequest(t, method, e)
	}
	for key, observer := range narrowingObservers() {
		if key.method == method {
			seen[key.channel] = observer(t, e)
		}
	}
	return seen
}

// observeRequest makes every attempt on a request method, each on a server of
// its own.
func observeRequest(t *testing.T, method string, e tenancy.Era) observation {
	t.Helper()
	params, ok := requestParams(method)
	if !ok {
		t.Fatalf("a row names %s, which this test has no request for: add its params to requestParams so that the row is held to the SDK", method)
	}
	seen := observation{}
	for _, a := range attempts() {
		seen[a.channel] = a.run(t, method, e, params)
	}
	return seen
}

// requestParams are the params each request the matrix names is sent with:
// something the matrix server can answer, so a refusal is the only thing that
// stops an answer.
func requestParams(method string) (map[string]any, bool) {
	switch method {
	case methodToolsList, "prompts/list", "resources/list", "resources/templates/list", "server/discover":
		return map[string]any{}, true
	case methodToolsCall:
		return toolCall(okTool), true
	case "resources/read", methodSubscribe:
		return map[string]any{"uri": resourceURI}, true
	case "prompts/get":
		return map[string]any{"name": promptName}, true
	case methodCompletion:
		return map[string]any{
			"ref":      map[string]any{"type": "ref/prompt", "name": promptName},
			"argument": map[string]any{"name": "a", "value": "b"},
		}, true
	case methodListen:
		return listenParams(resourceURI), true
	case methodInitialize:
		return initializeParams(), true
	default:
		return nil, false
	}
}

// toolCall is the params of a tools/call for name.
func toolCall(name string) map[string]any {
	return map[string]any{"name": name, "arguments": map[string]any{}}
}

// listenParams is the params of a subscriptions/listen watching uris.
func listenParams(uris ...string) map[string]any {
	return map[string]any{"notifications": map[string]any{"resourceSubscriptions": uris}}
}

// initializeParams is the params of a 2025-11-25 initialize.
func initializeParams() map[string]any {
	return map[string]any{
		"protocolVersion": legacyVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "carried-channels", "version": "0"},
	}
}

// attemptKind is how a server tries to refuse a request on one channel.
type attemptKind int

const (
	// inBandError refuses with a JSON-RPC error from a receiving middleware.
	inBandError attemptKind = iota
	// errorFlag answers with the SDK's own result, its error flag set where
	// the result has one.
	errorFlag
	// emptiedCompletion answers with the SDK's own result, its completion
	// values emptied where the result has any.
	emptiedCompletion
	// endedStream ends the stream the SDK acknowledged for the request, and
	// stamps a reason on the result the SDK then answers with.
	endedStream
	// noAnswer answers with neither a result nor an error.
	noAnswer
)

// attempt is one way of refusing a request, and the channel it tries.
type attempt struct {
	channel tenancy.Channel
	kind    attemptKind
}

// attempts are the refusal channels a request can be tried on. The gate and
// startup are not among them, being this server's own layers, and neither is
// a session close, which ends a session rather than answering a method and is
// held on the expiry row.
func attempts() []attempt {
	return []attempt{
		{tenancy.RPC, inBandError},
		{tenancy.ToolError, errorFlag},
		{tenancy.EmptyCompletion, emptiedCompletion},
		{tenancy.ListenEnd, endedStream},
		{tenancy.Silent, noAnswer},
	}
}

// run drives one attempt on one method in one era, on a server of its own.
func (a attempt) run(t *testing.T, method string, e tenancy.Era, params map[string]any) report {
	t.Helper()
	s, _ := newMatrixServer(nil)
	s.AddReceivingMiddleware(a.middleware(method))
	s.AddSendingMiddleware(endOnAcknowledgment)
	w := connect(t, s, e)
	if method != methodInitialize {
		w.handshake(t)
	}
	ex := w.call(t, method, params)
	return report{carried: a.judge(ex), arrived: ex.describe()}
}

// middleware makes the attempt on method and lets every other method through,
// so that a legacy handshake is never what gets refused.
func (a attempt) middleware(method string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, m string, req mcp.Request) (mcp.Result, error) {
			if m != method {
				return next(ctx, m, req)
			}
			switch a.kind {
			case inBandError:
				return nil, &jsonrpc.Error{Code: tenancy.CodeTooManyRequests, Message: refusalText}
			case noAnswer:
				// The attempt is a server that answers a request with nothing
				// at all, neither a result nor an error.
				return nil, nil
			}
			res, acknowledged, err := callEndingStreams(ctx, next, m, req)
			if err != nil {
				return res, err
			}
			switch a.kind {
			case errorFlag:
				setBoolField(res, "isError")
			case emptiedCompletion:
				emptyCompletionValues(res)
			case endedStream:
				if acknowledged {
					stampEnd(res)
				}
			}
			return res, nil
		}
	}
}

// judge reports whether the attempt's channel reached the client intact.
func (a attempt) judge(ex exchange) bool {
	switch a.kind {
	case inBandError:
		we, ok := ex.wireError()
		return ok && we.Code == tenancy.CodeTooManyRequests && we.Message == refusalText
	case errorFlag:
		result, ok := ex.result()
		return ok && result["isError"] == true
	case emptiedCompletion:
		result, ok := ex.result()
		return ok && completionIsEmpty(result)
	case endedStream:
		result, ok := ex.result()
		return ok && ex.acknowledged() && metaOf(result)[endReasonKey] == endReason
	case noAnswer:
		return ex.response == nil
	default:
		return false
	}
}

// openStream is a request's context, as the sending middleware finds it when
// the SDK acknowledges a stream for that request.
type openStream struct {
	cancel       context.CancelFunc
	acknowledged atomic.Bool
}

// openStreamKey carries the openStream in the context.
type openStreamKey struct{}

// callEndingStreams calls next with a context that ends once the SDK has
// acknowledged a stream for the request, so a subscriptions/listen answers
// rather than parking, and reports whether an acknowledgment was sent.
func callEndingStreams(ctx context.Context, next mcp.MethodHandler, method string, req mcp.Request) (mcp.Result, bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := &openStream{cancel: cancel}
	res, err := next(context.WithValue(ctx, openStreamKey{}, stream), method, req)
	return res, stream.acknowledged.Load(), err
}

// endOnAcknowledgment is a sending middleware that ends a request's context
// once the SDK has sent the acknowledgment of its stream, the way cmd/server
// ends a subscriptions/listen whose watches have stopped: the SDK is sending
// the acknowledgment with the handler's own context, so the handler is the
// one that sees it end.
func endOnAcknowledgment(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		res, err := next(ctx, method, req)
		if stream, ok := ctx.Value(openStreamKey{}).(*openStream); ok && method == methodAcknowledged {
			stream.acknowledged.Store(true)
			stream.cancel()
		}
		return res, err
	}
}

// fieldByJSONName finds the settable field whose JSON name is name in the
// struct v holds or points to, looking through embedded structs as
// encoding/json does.
func fieldByJSONName(v reflect.Value, name string) (reflect.Value, bool) {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return reflect.Value{}, false
	}
	for i := range v.NumField() {
		field, value := v.Type().Field(i), v.Field(i)
		if tag, _, _ := strings.Cut(field.Tag.Get("json"), ","); tag == name && value.CanSet() {
			return value, true
		}
		if !field.Anonymous {
			continue
		}
		if found, ok := fieldByJSONName(value, name); ok {
			return found, true
		}
	}
	return reflect.Value{}, false
}

// setBoolField sets the boolean field whose JSON name is name on the SDK's own
// result, when the result has one. Only a result type that carries the field
// can carry the flag: whether it does is read off the value the SDK built, so
// a field added to a result type by an SDK upgrade is seen without an edit.
func setBoolField(res mcp.Result, name string) {
	if field, ok := fieldByJSONName(reflect.ValueOf(res), name); ok && field.Kind() == reflect.Bool {
		field.SetBool(true)
	}
}

// emptyCompletionValues empties the completion values of the SDK's own
// result, when the result has any.
func emptyCompletionValues(res mcp.Result) {
	completion, ok := fieldByJSONName(reflect.ValueOf(res), "completion")
	if !ok {
		return
	}
	if values, found := fieldByJSONName(completion, "values"); found && values.Kind() == reflect.Slice {
		values.Set(reflect.MakeSlice(values.Type(), 0, 0))
	}
}

// stampEnd stamps the ending reason on a result's metadata.
func stampEnd(res mcp.Result) {
	meta := res.GetMeta()
	if meta == nil {
		meta = map[string]any{}
	}
	meta[endReasonKey] = endReason
	res.SetMeta(meta)
}

// completionIsEmpty reports whether a result is a completion with no values.
func completionIsEmpty(result map[string]any) bool {
	completion, ok := result["completion"].(map[string]any)
	if !ok {
		return false
	}
	values, present := completion["values"]
	if !present {
		return false
	}
	if values == nil {
		return true
	}
	list, isList := values.([]any)
	return isList && len(list) == 0
}

// metaOf is a result's _meta object, nil when it has none.
func metaOf(result map[string]any) map[string]any {
	meta, _ := result["_meta"].(map[string]any)
	return meta
}

// textOf is the text of a tool result's first content item.
func textOf(result map[string]any) string {
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		return ""
	}
	first, _ := content[0].(map[string]any)
	text, _ := first["text"].(string)
	return text
}

// narrowingKey names a narrowing on one method.
type narrowingKey struct {
	channel tenancy.Channel
	method  string
}

// narrowingObservers are the narrowings this file can watch reach a client: a
// listing that leaves a tool out, a call for a tool that is not there, and a
// call answered with its cause or as unknown. Each runs whenever its method is
// driven, so a row that drops one fails as surely as a row that adds one, and
// a row listing a narrowing with no observer here fails, so that a new one is
// held before it is relied on.
func narrowingObservers() map[narrowingKey]func(*testing.T, tenancy.Era) report {
	return map[narrowingKey]func(*testing.T, tenancy.Era) report{
		{tenancy.Absent, methodToolsList}:   observeAbsentFromListing,
		{tenancy.Absent, methodToolsCall}:   observeAbsentFromCall,
		{tenancy.Withheld, methodToolsCall}: observeWithheld,
		{tenancy.Unknown, methodToolsCall}:  observeUnknown,
	}
}

// observeAbsentFromListing lists the tools, removes one, and lists them again:
// the narrowing is carried when the second listing is a result without the
// tool the first one had.
func observeAbsentFromListing(t *testing.T, e tenancy.Era) report {
	t.Helper()
	s, _ := newMatrixServer(nil)
	w := dial(t, s, e)
	before := w.call(t, methodToolsList, nil)
	s.RemoveTools(narrowedTool)
	after := w.call(t, methodToolsList, nil)
	carried := listsTool(before, narrowedTool) && listsTool(after, okTool) && !listsTool(after, narrowedTool)
	return report{carried: carried, arrived: "before the narrowing " + before.describe() + "; after it " + after.describe()}
}

// observeAbsentFromCall calls a tool the server does not serve: the narrowing
// is carried when the answer is an in-band error naming the tool and no
// handler ran.
func observeAbsentFromCall(t *testing.T, e tenancy.Era) report {
	t.Helper()
	s, counts := newMatrixServer(nil)
	s.RemoveTools(narrowedTool)
	w := dial(t, s, e)
	ex := w.call(t, methodToolsCall, toolCall(narrowedTool))
	we, ok := ex.wireError()
	carried := ok && strings.Contains(we.Message, strconv.Quote(narrowedTool)) && counts.narrowedCalls.Load() == 0
	return report{carried: carried, arrived: ex.describe()}
}

// observeWithheld calls the tool whose answer names the cause of a narrowing.
func observeWithheld(t *testing.T, e tenancy.Era) report {
	t.Helper()
	return observeToolAnswer(t, e, withheldTool, withheldText)
}

// observeUnknown calls the tool whose answer calls the action unknown.
func observeUnknown(t *testing.T, e tenancy.Era) report {
	t.Helper()
	return observeToolAnswer(t, e, unknownTool, unknownText)
}

// observeToolAnswer calls a tool that answers with a result flagged as an
// error: the narrowing is carried when that result reaches the client with its
// text as written.
func observeToolAnswer(t *testing.T, e tenancy.Era, tool, text string) report {
	t.Helper()
	s, _ := newMatrixServer(nil)
	w := dial(t, s, e)
	ex := w.call(t, methodToolsCall, toolCall(tool))
	result, ok := ex.result()
	return report{carried: ok && result["isError"] == true && textOf(result) == text, arrived: ex.describe()}
}

// listsTool reports whether a tools/list answer is a result naming the tool.
func listsTool(ex exchange, name string) bool {
	result, ok := ex.result()
	if !ok {
		return false
	}
	tools, _ := result["tools"].([]any)
	return slices.ContainsFunc(tools, func(tool any) bool {
		entry, _ := tool.(map[string]any)
		return entry["name"] == name
	})
}

// observeNotification attempts the two ways a server can refuse a
// resources/updated notification it is about to send, an error and nothing
// at all, and reads what the client received before a notification sent after
// it. A notification has no result, so the flag, completion and ending
// attempts have nothing to act on.
func observeNotification(t *testing.T, e tenancy.Era) observation {
	t.Helper()
	inBand := func() (mcp.Result, error) {
		return nil, &jsonrpc.Error{Code: tenancy.CodeTooManyRequests, Message: refusalText}
	}
	dropped := func() (mcp.Result, error) {
		return nil, nil //nolint:nilnil // the attempt is a server that sends nothing at all
	}
	return observation{
		tenancy.RPC:    refuseNotification(t, e, inBand, carriesProbeError),
		tenancy.Silent: refuseNotification(t, e, dropped, leavesNoTrace),
	}
}

// refuseNotification subscribes to two resources, has the server refuse the
// notification for the first with refuse and send the one for the second, and
// judges what the client read up to the second. Notifications on one
// connection arrive in the order they were sent, so the second arriving means
// the first would have arrived before it.
func refuseNotification(t *testing.T, e tenancy.Era, refuse func() (mcp.Result, error), judge func([]jsonrpc.Message) bool) report {
	t.Helper()
	s, _ := newMatrixServer(nil)
	s.AddSendingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if p, ok := req.GetParams().(*mcp.ResourceUpdatedNotificationParams); ok && p != nil && method == methodUpdated && p.URI == refusedURI {
				return refuse()
			}
			return next(ctx, method, req)
		}
	})
	w := dial(t, s, e)
	w.subscribeTo(t, refusedURI, sentinelURI)

	sent := make(chan struct{})
	go func() {
		defer close(sent)
		for _, uri := range []string{refusedURI, sentinelURI} {
			_ = s.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri})
		}
	}()
	read, arrived := w.readUntil(t, isUpdateFor(sentinelURI))
	waitFor(t, sent, "the server to finish sending its resources/updated notifications")
	if !arrived {
		t.Fatalf("at %s the client received no resources/updated notification for %s, so whether the refused one reached it cannot be judged; read: %s",
			eraName(e), sentinelURI, describeMessages(read))
	}
	return report{carried: judge(read), arrived: describeMessages(read)}
}

// carriesProbeError reports whether anything the client read carries the
// probe's in-band refusal.
func carriesProbeError(read []jsonrpc.Message) bool {
	return slices.ContainsFunc(read, func(msg jsonrpc.Message) bool {
		resp, ok := msg.(*jsonrpc.Response)
		if !ok {
			return false
		}
		we, isWire := errors.AsType[*jsonrpc.Error](resp.Error)
		return isWire && we.Code == tenancy.CodeTooManyRequests && we.Message == refusalText
	})
}

// leavesNoTrace reports whether the client read nothing about the refused
// resource: no notification for it and no error.
func leavesNoTrace(read []jsonrpc.Message) bool {
	return !slices.ContainsFunc(read, func(msg jsonrpc.Message) bool {
		if resp, ok := msg.(*jsonrpc.Response); ok {
			return resp.Error != nil
		}
		return isUpdateFor(refusedURI)(msg)
	})
}

// isUpdateFor matches the resources/updated notification for uri.
func isUpdateFor(uri string) func(jsonrpc.Message) bool {
	return func(msg jsonrpc.Message) bool {
		req, ok := msg.(*jsonrpc.Request)
		if !ok || req.Method != methodUpdated {
			return false
		}
		var params struct {
			URI string `json:"uri"`
		}
		return json.Unmarshal(req.Params, &params) == nil && params.URI == uri
	}
}

// observeExpiry drives the SDK's idle timeout and classifies the answer a
// request naming the expired session gets afterwards. A session the SDK never
// issues cannot expire, which is what a 2026-07-28 request, carrying no
// session, is expected to show.
func observeExpiry(t *testing.T, e tenancy.Era) observation {
	t.Helper()
	s, _ := newMatrixServer(nil)
	url := serveHTTP(t, s, &mcp.StreamableHTTPOptions{
		Stateless: e == tenancy.EraModern, JSONResponse: true, SessionTimeout: sessionTimeout,
	})
	var opened answer
	if e == tenancy.EraModern {
		opened = post(t, url, modernHeaders(methodToolsList), rpcMessage(1, methodToolsList, modernMeta(nil)))
	} else {
		opened = post(t, url, nil, rpcMessage(1, methodInitialize, initializeParams()))
	}
	sid := opened.header.Get("Mcp-Session-Id")
	if sid == "" {
		return observation{tenancy.SessionClose: {arrived: "no Mcp-Session-Id was issued, so there is no session to expire: " + opened.describe()}}
	}
	waitForSessionsToEnd(t, s)
	return classifyAfterExpiry(askAfterExpiry(t, url, sid, e))
}

// waitForSessionsToEnd waits for every session the server holds to end, which
// on an idle stateful session only the SDK's timeout does.
func waitForSessionsToEnd(t *testing.T, s *mcp.Server) {
	t.Helper()
	for session := range s.Sessions() {
		ended := make(chan struct{})
		go func() {
			defer close(ended)
			_ = session.Wait()
		}()
		waitFor(t, ended, fmt.Sprintf("an idle session to be ended by the SDK's %s timeout", sessionTimeout))
	}
}

// askAfterExpiry sends a request naming an expired session until the handler
// has dropped the session. The SDK closes a session's connection a moment
// before it removes the id from its table, and a request landing in between
// gets an empty answer rather than the refusal.
func askAfterExpiry(t *testing.T, url, sid string, e tenancy.Era) answer {
	t.Helper()
	headers := map[string]string{"Mcp-Session-Id": sid, "MCP-Protocol-Version": eraName(e)}
	deadline := time.Now().Add(wireWait)
	for {
		got := post(t, url, headers, rpcMessage(2, methodToolsList, nil))
		if got.status == http.StatusNotFound || time.Now().After(deadline) {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// classifyAfterExpiry names the channel the answer to a request on an expired
// session arrived on: a 404 that is not a JSON-RPC error is the session close,
// a JSON-RPC error is an in-band refusal, and no answer is silence.
func classifyAfterExpiry(got answer) observation {
	_, isRPC := decodeRPCError(got.body)
	silent := got.err != nil || strings.TrimSpace(got.body) == ""
	return observation{
		tenancy.SessionClose: {carried: got.status == http.StatusNotFound && !isRPC, arrived: got.describe()},
		tenancy.RPC:          {carried: isRPC, arrived: got.describe()},
		tenancy.Silent:       {carried: silent && got.status != http.StatusNotFound, arrived: got.describe()},
	}
}

// serverCounts records, from the server's goroutines, what its handlers were
// asked to do, for the test goroutine to read once an answer has arrived.
type serverCounts struct {
	subscribes    atomic.Int64
	unsubscribes  atomic.Int64
	narrowedCalls atomic.Int64
	busyCalls     atomic.Int64
}

// newMatrixServer builds the server every attempt drives: one of each thing a
// method can name, so that a refusal is the only thing that keeps a method
// from being answered, and a subscribe handler that answers subscribeErr.
func newMatrixServer(subscribeErr error) (*mcp.Server, *serverCounts) {
	counts := &serverCounts{}
	s := mcp.NewServer(&mcp.Implementation{Name: "carried-channels", Version: "0"}, &mcp.ServerOptions{
		CompletionHandler: func(context.Context, *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{"alpha", "beta"}}}, nil
		},
		SubscribeHandler: func(context.Context, *mcp.SubscribeRequest) error {
			counts.subscribes.Add(1)
			return subscribeErr
		},
		UnsubscribeHandler: func(context.Context, *mcp.UnsubscribeRequest) error {
			counts.unsubscribes.Add(1)
			return nil
		},
	})
	addTool(s, okTool, func() *mcp.CallToolResult { return textResult("fine", false) })
	addTool(s, narrowedTool, func() *mcp.CallToolResult {
		counts.narrowedCalls.Add(1)
		return textResult("a tool the narrowing removes", false)
	})
	addTool(s, withheldTool, func() *mcp.CallToolResult { return textResult(withheldText, true) })
	addTool(s, unknownTool, func() *mcp.CallToolResult { return textResult(unknownText, true) })
	addTool(s, busyTool, func() *mcp.CallToolResult {
		counts.busyCalls.Add(1)
		return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{}}
	})
	s.AddPrompt(&mcp.Prompt{Name: promptName}, func(context.Context, *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{Messages: []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: "hello"}}}}, nil
	})
	read := func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, Text: "contents"}}}, nil
	}
	for _, uri := range []string{resourceURI, refusedURI, sentinelURI} {
		s.AddResource(&mcp.Resource{URI: uri, Name: strings.TrimPrefix(uri, "test://")}, read)
	}
	s.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: "test://t/{name}", Name: "t"}, read)
	return s, counts
}

// addTool registers a tool that takes no arguments and answers with what
// respond returns.
func addTool(s *mcp.Server, name string, respond func() *mcp.CallToolResult) {
	s.AddTool(&mcp.Tool{Name: name, InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return respond(), nil
		})
}

// textResult is a tool result carrying text, flagged as an error or not.
func textResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}, IsError: isError}
}

// answerWith is a receiving middleware that answers method with what respond
// returns and lets every other method through.
func answerWith(method string, respond func() (mcp.Result, error)) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, m string, req mcp.Request) (mcp.Result, error) {
			if m == method {
				return respond()
			}
			return next(ctx, m, req)
		}
	}
}

// refuseWith answers method with err.
func refuseWith(method string, err error) mcp.Middleware {
	return answerWith(method, func() (mcp.Result, error) { return nil, err })
}

// wireClient is a JSON-RPC client over the SDK's in-memory transport that
// interprets nothing: what it reads is what the server wrote. A goroutine
// pumps every message into inbox, so the server is never blocked on a client
// that is not reading yet.
type wireClient struct {
	conn  mcp.Connection
	inbox chan jsonrpc.Message
	era   tenancy.Era
	next  float64
}

// connect connects a wire client to s in era e, without a handshake.
func connect(t *testing.T, s *mcp.Server, e tenancy.Era) *wireClient {
	t.Helper()
	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	session, err := s.Connect(context.Background(), serverEnd, nil)
	if err != nil {
		t.Fatalf("connect the server: %v", err)
	}
	conn, err := clientEnd.Connect(context.Background())
	if err != nil {
		_ = session.Close()
		t.Fatalf("connect the client: %v", err)
	}
	w := &wireClient{conn: conn, inbox: make(chan jsonrpc.Message, 256), era: e}
	pumpCtx, stop := context.WithCancel(context.Background())
	pumped := make(chan struct{})
	go func() {
		defer close(pumped)
		defer close(w.inbox)
		for {
			msg, readErr := conn.Read(pumpCtx)
			if readErr != nil {
				return
			}
			select {
			case w.inbox <- msg:
			case <-pumpCtx.Done():
				return
			}
		}
	}()
	t.Cleanup(func() {
		stop()
		_ = conn.Close()
		_ = session.Close()
		<-pumped
	})
	return w
}

// dial connects a wire client to s in era e and opens the session the way a
// client of that era does.
func dial(t *testing.T, s *mcp.Server, e tenancy.Era) *wireClient {
	t.Helper()
	w := connect(t, s, e)
	w.handshake(t)
	return w
}

// handshake opens a 2025-11-25 session with initialize, and does nothing at
// 2026-07-28, where every request carries its own protocol version.
func (w *wireClient) handshake(t *testing.T) {
	t.Helper()
	if w.era != tenancy.EraLegacy {
		return
	}
	if ex := w.call(t, methodInitialize, initializeParams()); ex.response == nil || ex.response.Error != nil {
		t.Fatalf("the %s handshake failed: %s", legacyVersion, ex.describe())
	}
	w.send(t, &jsonrpc.Request{Method: "notifications/initialized", Params: json.RawMessage(`{}`)})
}

// modernMeta returns params with the _meta a 2026-07-28 request carries.
func modernMeta(params map[string]any) map[string]any {
	withMeta := maps.Clone(params)
	if withMeta == nil {
		withMeta = map[string]any{}
	}
	withMeta["_meta"] = map[string]any{
		mcp.MetaKeyProtocolVersion:    modernVersion,
		mcp.MetaKeyClientCapabilities: map[string]any{},
		mcp.MetaKeyClientInfo:         map[string]any{"name": "carried-channels", "version": "0"},
	}
	return withMeta
}

// send writes one message to the server.
func (w *wireClient) send(t *testing.T, msg jsonrpc.Message) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wireWait)
	defer cancel()
	if err := w.conn.Write(ctx, msg); err != nil {
		t.Fatalf("write %T: %v", msg, err)
	}
}

// start sends a request and returns its id without waiting for the answer.
func (w *wireClient) start(t *testing.T, method string, params map[string]any) jsonrpc.ID {
	t.Helper()
	if params == nil {
		params = map[string]any{}
	}
	if w.era == tenancy.EraModern {
		params = modernMeta(params)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("encode the params of %s: %v", method, err)
	}
	w.next++
	id, err := jsonrpc.MakeID(w.next)
	if err != nil {
		t.Fatalf("make request id %v: %v", w.next, err)
	}
	w.send(t, &jsonrpc.Request{ID: id, Method: method, Params: raw})
	return id
}

// call sends a request and waits for its answer.
func (w *wireClient) call(t *testing.T, method string, params map[string]any) exchange {
	t.Helper()
	return w.await(t, w.start(t, method, params))
}

// await reads until the answer to id arrives, keeping what the server sent
// before it.
func (w *wireClient) await(t *testing.T, id jsonrpc.ID) exchange {
	t.Helper()
	read, answered := w.readUntil(t, func(msg jsonrpc.Message) bool {
		resp, ok := msg.(*jsonrpc.Response)
		return ok && resp.ID == id
	})
	var ex exchange
	for _, msg := range read {
		if req, ok := msg.(*jsonrpc.Request); ok {
			ex.before = append(ex.before, req)
		}
	}
	if answered {
		ex.response, _ = read[len(read)-1].(*jsonrpc.Response)
	}
	return ex
}

// readUntil reads what the server sends until a message satisfies done, and
// returns everything read, that message included. It reports false when the
// connection ended or the wait ran out first.
func (w *wireClient) readUntil(t *testing.T, done func(jsonrpc.Message) bool) ([]jsonrpc.Message, bool) {
	t.Helper()
	deadline := time.NewTimer(wireWait)
	defer deadline.Stop()
	var read []jsonrpc.Message
	for {
		select {
		case msg, open := <-w.inbox:
			if !open {
				return read, false
			}
			read = append(read, msg)
			if done(msg) {
				return read, true
			}
		case <-deadline.C:
			return read, false
		}
	}
}

// subscribeTo subscribes to uris the way a client of the era does: one
// resources/subscribe each at 2025-11-25, one subscriptions/listen at
// 2026-07-28, which the server acknowledges and then keeps open.
func (w *wireClient) subscribeTo(t *testing.T, uris ...string) {
	t.Helper()
	if w.era == tenancy.EraModern {
		w.start(t, methodListen, listenParams(uris...))
		if read, acknowledged := w.readUntil(t, isMethod(methodAcknowledged)); !acknowledged {
			t.Fatalf("the listen for %v was not acknowledged: %s", uris, describeMessages(read))
		}
		return
	}
	for _, uri := range uris {
		if ex := w.call(t, methodSubscribe, map[string]any{"uri": uri}); ex.response == nil || ex.response.Error != nil {
			t.Fatalf("the subscription to %s failed: %s", uri, ex.describe())
		}
	}
}

// isMethod matches a notification or server request by method.
func isMethod(method string) func(jsonrpc.Message) bool {
	return func(msg jsonrpc.Message) bool {
		req, ok := msg.(*jsonrpc.Request)
		return ok && req.Method == method
	}
}

// exchange is what the client read for one request: what the server sent
// before the answer, and the answer, nil when none arrived.
type exchange struct {
	before   []*jsonrpc.Request
	response *jsonrpc.Response
}

// wireError is the answer's JSON-RPC error.
func (ex exchange) wireError() (*jsonrpc.Error, bool) {
	if ex.response == nil || ex.response.Error == nil {
		return nil, false
	}
	if we, ok := errors.AsType[*jsonrpc.Error](ex.response.Error); ok {
		return we, true
	}
	return &jsonrpc.Error{Message: ex.response.Error.Error()}, true
}

// result is the answer's result decoded as an object, nil for a null result.
// It reports false when the answer is an error or never came.
func (ex exchange) result() (map[string]any, bool) {
	if ex.response == nil || ex.response.Error != nil {
		return nil, false
	}
	var result map[string]any
	if !nullResult(ex.response.Result) {
		if err := json.Unmarshal(ex.response.Result, &result); err != nil {
			return nil, false
		}
	}
	return result, true
}

// nullResult reports whether a result is JSON null, which a transport may
// also hand over as no bytes at all.
func nullResult(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "null"
}

// acknowledged reports whether the server acknowledged a stream before it
// answered.
func (ex exchange) acknowledged() bool {
	return slices.ContainsFunc(ex.before, func(req *jsonrpc.Request) bool { return req.Method == methodAcknowledged })
}

// describe says what the client read, in the words a failure quotes.
func (ex exchange) describe() string {
	var b strings.Builder
	for _, req := range ex.before {
		fmt.Fprintf(&b, "%s, then ", req.Method)
	}
	switch {
	case ex.response == nil:
		fmt.Fprintf(&b, "no answer within %s", wireWait)
	case ex.response.Error != nil:
		we, _ := ex.wireError()
		fmt.Fprintf(&b, "error %d %q", we.Code, we.Message)
	case nullResult(ex.response.Result):
		b.WriteString("result null")
	default:
		b.WriteString("result " + clip(string(ex.response.Result)))
	}
	return b.String()
}

// describeMessages says what the client read, in order.
func describeMessages(read []jsonrpc.Message) string {
	if len(read) == 0 {
		return "nothing"
	}
	parts := make([]string, 0, len(read))
	for _, msg := range read {
		switch m := msg.(type) {
		case *jsonrpc.Request:
			parts = append(parts, m.Method+" "+clip(string(m.Params)))
		case *jsonrpc.Response:
			parts = append(parts, exchange{response: m}.describe())
		}
	}
	return strings.Join(parts, "; ")
}

// clip shortens what a failure quotes.
func clip(s string) string {
	const most = 240
	if len(s) <= most {
		return s
	}
	return s[:most] + "..."
}

// waitFor waits for done to close, and fails the test naming what was waited
// for when wireWait runs out first.
func waitFor(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(wireWait):
		t.Fatalf("waited %s for %s", wireWait, what)
	}
}

// answer is what an HTTP request to the SDK's streamable handler got back.
type answer struct {
	status int
	header http.Header
	body   string
	err    error
}

// describe says what the answer was, in the words a failure quotes.
func (a answer) describe() string {
	if a.err != nil {
		return "no answer: " + a.err.Error()
	}
	return fmt.Sprintf("HTTP %d %s %q", a.status, a.header.Get("Content-Type"), clip(a.body))
}

// serveHTTP serves s through the SDK's streamable handler on a loopback
// httptest server and returns its URL.
func serveHTTP(t *testing.T, s *mcp.Server, opts *mcp.StreamableHTTPOptions) string {
	t.Helper()
	srv := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s }, opts))
	t.Cleanup(func() {
		for session := range s.Sessions() {
			_ = session.Close()
		}
		srv.Close()
	})
	return srv.URL
}

// post sends one JSON-RPC message as a POST, with the headers a client of the
// streamable transport sends and headers over them.
func post(t *testing.T, url string, headers map[string]string, message map[string]any) answer {
	t.Helper()
	body, err := json.Marshal(message)
	if err != nil {
		t.Fatalf("encode %v: %v", message, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wireWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build the POST: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return answer{err: err}
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return answer{status: resp.StatusCode, header: resp.Header, body: string(data), err: err}
}

// rpcMessage is a JSON-RPC request as it is written on the wire.
func rpcMessage(id int, method string, params map[string]any) map[string]any {
	if params == nil {
		params = map[string]any{}
	}
	return map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
}

// modernHeaders are the headers a 2026-07-28 request carries.
func modernHeaders(method string) map[string]string {
	return map[string]string{"MCP-Protocol-Version": modernVersion, "Mcp-Method": method}
}

// openLegacySession initializes a 2025-11-25 session over HTTP and returns
// its id.
func openLegacySession(t *testing.T, url string) string {
	t.Helper()
	opened := post(t, url, nil, rpcMessage(1, methodInitialize, initializeParams()))
	sid := opened.header.Get("Mcp-Session-Id")
	if opened.err != nil || opened.status != http.StatusOK || sid == "" {
		t.Fatalf("the %s initialize was answered %s with Mcp-Session-Id %q", legacyVersion, opened.describe(), sid)
	}
	return sid
}

// decodeRPCError reads the JSON-RPC error out of an HTTP answer's body. A body
// that is not JSON, as the SDK's own plain-text refusals are not, carries none.
func decodeRPCError(body string) (*jsonrpc.Error, bool) {
	var envelope struct {
		Error *jsonrpc.Error `json:"error"`
	}
	decoded := json.Unmarshal([]byte(body), &envelope) == nil
	return envelope.Error, decoded && envelope.Error != nil
}

// inBand reports whether a channel is an answer the SDK carries in the
// response to the request itself.
func inBand(c tenancy.Channel) bool {
	return c == tenancy.RPC || c == tenancy.ToolError || c == tenancy.EmptyCompletion
}

// TestRegisterRefusals_InBand_ReachTheClientIntact makes every in-band refusal
// the register declares, on each method and in each era the matrix carries it
// for, and holds it to reaching the client as written: an in-band error with
// its code and its text, a tool result flagged as an error with its text, a
// completion with no values. The text begins with the row's Prefix, which is
// what a client may recognize a refusal by, so a prefix the SDK rewrote fails
// here too.
func TestRegisterRefusals_InBand_ReachTheClientIntact(t *testing.T) {
	sdk := sdkName()
	held := 0
	for _, d := range tenancy.Decisions() {
		for i, r := range d.Refusals {
			if !inBand(r.Channel) {
				continue
			}
			for _, method := range r.Methods {
				for _, e := range erasOf(r.Era) {
					if !tenancy.Carries(method, e, r.Channel) {
						continue
					}
					held++
					t.Run(fmt.Sprintf("%s.%d/%s@%s", d.ID, i, method, eraName(e)), func(t *testing.T) {
						checkInBandRefusal(t, sdk, d.ID, r, method, e)
					})
				}
			}
		}
	}
	if held == 0 {
		t.Fatal("the register declares no in-band refusal the matrix carries, so nothing held them to the SDK")
	}
}

// checkInBandRefusal makes one declared refusal of one method in one era, the
// way the layers make it, and holds what reached the client to it.
func checkInBandRefusal(t *testing.T, sdk, id string, r tenancy.Refusal, method string, e tenancy.Era) {
	t.Helper()
	params, ok := requestParams(method)
	if !ok {
		t.Fatalf("%s refuses %s, which this test has no request for: add its params to requestParams", id, method)
	}
	text := r.Prefix + id + " refused this request"
	s, _ := newMatrixServer(nil)
	s.AddReceivingMiddleware(answerWith(method, inBandAnswer(r, text)))
	w := connect(t, s, e)
	if method != methodInitialize {
		w.handshake(t)
	}
	ex := w.call(t, method, params)
	if !arrivedIntact(ex, r, text) {
		t.Errorf("%s refuses %s at %s on the %s channel with code %d and text %q, and %s delivered %s. The register declares a refusal the SDK no longer carries as written: edit the row in internal/tenancy in this same pull request, and Carriages() in internal/tenancy/channels.go if the channel itself moved.",
			id, method, eraName(e), r.Channel, r.Code, text, sdk, ex.describe())
	}
}

// inBandAnswer is how a layer answers with a refusal on its channel.
func inBandAnswer(r tenancy.Refusal, text string) func() (mcp.Result, error) {
	return func() (mcp.Result, error) {
		switch r.Channel {
		case tenancy.ToolError:
			return textResult(text, true), nil
		case tenancy.EmptyCompletion:
			return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: []string{}}}, nil
		default:
			return nil, &jsonrpc.Error{Code: int64(r.Code), Message: text}
		}
	}
}

// arrivedIntact reports whether a refusal reached the client as it was made.
func arrivedIntact(ex exchange, r tenancy.Refusal, text string) bool {
	switch r.Channel {
	case tenancy.ToolError:
		result, ok := ex.result()
		return ok && result["isError"] == true && textOf(result) == text
	case tenancy.EmptyCompletion:
		result, ok := ex.result()
		return ok && completionIsEmpty(result)
	default:
		we, ok := ex.wireError()
		return ok && we.Code == int64(r.Code) && we.Message == text
	}
}

// TestMiddlewareErrors_OnTheWire_KeepTheirCodeUnlessTheSDKRewritesThem holds
// the three rewrites the Refusal channels section rests on when it says an
// in-band error carries a JSON-RPC code and never -32601: a plain Go error
// reaches the client as code 0, a wrapped JSON-RPC error keeps its code and
// takes the outer text, and a -32601 loses its text to the SDK's own. After
// each, the session answers the next request, so a refusal costs the caller
// one request and not the session.
func TestMiddlewareErrors_OnTheWire_KeepTheirCodeUnlessTheSDKRewritesThem(t *testing.T) {
	sdk := sdkName()
	cases := []struct {
		name    string
		err     error
		code    int64
		message string
	}{
		{"a plain Go error arrives with code 0", errors.New(refusalText), 0, refusalText},
		{
			"a wrapped JSON-RPC error keeps its code and takes the outer text",
			fmt.Errorf("outer text: %w", &jsonrpc.Error{Code: tenancy.CodeTooManyRequests, Message: refusalText}),
			tenancy.CodeTooManyRequests, "outer text: " + refusalText,
		},
		{
			"a -32601 loses its text to the SDK's own",
			&jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: refusalText},
			jsonrpc.CodeMethodNotFound, `method not found: "tools/list"`,
		},
	}
	for _, e := range bothEras() {
		for _, tc := range cases {
			t.Run(eraName(e)+"/"+tc.name, func(t *testing.T) {
				s, _ := newMatrixServer(nil)
				s.AddReceivingMiddleware(refuseWith(methodToolsList, tc.err))
				w := dial(t, s, e)
				refused := w.call(t, methodToolsList, nil)
				if we, ok := refused.wireError(); !ok || we.Code != tc.code || we.Message != tc.message {
					t.Errorf("at %s a middleware refusing tools/list with %v reached the client as %s, want error %d %q (%s). The Refusal channels section of docs/development/tenant-policy-spec.md, and the codes Validate allows, rest on this: edit them in this same pull request.",
						eraName(e), tc.err, refused.describe(), tc.code, tc.message, sdk)
				}
				if next := w.call(t, methodToolsCall, toolCall(okTool)); next.response == nil || next.response.Error != nil {
					t.Errorf("at %s the session did not answer the request after a refusal (%s): %s", eraName(e), sdk, next.describe())
				}
			})
		}
	}
}

// statusCodes are the codes whose HTTP status is held: every in-band code the
// register declares, code 0 for a plain Go error, and the four the SDK maps to
// a status of its own.
func statusCodes() []int64 {
	codes := []int64{
		0, jsonrpc.CodeMethodNotFound, jsonrpc.CodeInvalidParams,
		mcp.CodeMissingRequiredClientCapabilities, mcp.CodeUnsupportedProtocolVersion,
	}
	for _, d := range tenancy.Decisions() {
		for _, r := range d.Refusals {
			if code := int64(r.Code); r.Channel == tenancy.RPC && !slices.Contains(codes, code) {
				codes = append(codes, code)
			}
		}
	}
	return codes
}

// sdkStatus is the status go-sdk gives an in-band error at 2026-07-28: 404 for
// -32601, 400 for -32602, -32021 and -32022, and none of its own otherwise, so
// the answer is a 200 carrying the error.
func sdkStatus(code int64) int {
	switch code {
	case jsonrpc.CodeMethodNotFound:
		return http.StatusNotFound
	case jsonrpc.CodeInvalidParams, mcp.CodeMissingRequiredClientCapabilities, mcp.CodeUnsupportedProtocolVersion:
		return http.StatusBadRequest
	default:
		return http.StatusOK
	}
}

// TestInBandErrors_OverStreamableHTTP_TakeOnlyTheStatusesTheSDKMaps holds the
// statement that once the SDK handler runs, the only statuses an in-band error
// can produce are 404 and 400, and those only at 2026-07-28. It is why no row
// but the gate's carries a status: 401, 403, 413, 429 and 503 exist only in
// front of the SDK.
func TestInBandErrors_OverStreamableHTTP_TakeOnlyTheStatusesTheSDKMaps(t *testing.T) {
	sdk := sdkName()
	for _, e := range bothEras() {
		for _, code := range statusCodes() {
			t.Run(fmt.Sprintf("%s/%d", eraName(e), code), func(t *testing.T) {
				want := http.StatusOK
				if e == tenancy.EraModern {
					want = sdkStatus(code)
				}
				got := refuseOverHTTP(t, e, code)
				we, ok := decodeRPCError(got.body)
				if got.status != want || !ok || we.Code != code {
					t.Errorf("at %s a middleware refusing tools/list with code %d was answered %s, want HTTP %d carrying error %d (%s). The Refusal channels section of docs/development/tenant-policy-spec.md says an in-band error takes no status but 404 and 400, at 2026-07-28 alone, and Carriages() gives the gate row alone a status: edit both in this same pull request.",
						eraName(e), code, got.describe(), want, code, sdk)
				}
			})
		}
	}
}

// refuseOverHTTP refuses a tools/list with code through the SDK's streamable
// handler, stateless at 2026-07-28 and on a session at 2025-11-25, and returns
// the answer. Code 0 is a plain Go error.
func refuseOverHTTP(t *testing.T, e tenancy.Era, code int64) answer {
	t.Helper()
	var refusal error = &jsonrpc.Error{Code: code, Message: refusalText}
	if code == 0 {
		refusal = errors.New(refusalText)
	}
	s, _ := newMatrixServer(nil)
	s.AddReceivingMiddleware(refuseWith(methodToolsList, refusal))
	url := serveHTTP(t, s, &mcp.StreamableHTTPOptions{Stateless: e == tenancy.EraModern, JSONResponse: true})
	if e == tenancy.EraModern {
		return post(t, url, modernHeaders(methodToolsList), rpcMessage(2, methodToolsList, modernMeta(nil)))
	}
	sid := openLegacySession(t, url)
	return post(t, url, map[string]string{"Mcp-Session-Id": sid, "MCP-Protocol-Version": legacyVersion}, rpcMessage(2, methodToolsList, nil))
}

// TestLoadShedding_EmptyInputRequests_BusyAtLegacyAndRetriedAtModern holds how
// go-sdk answers a handler that sheds load with an empty input-request map.
// MCP has no "retry later" code, so the Refusal channels section sends a delay
// through the gate or through prose; this is the one mechanism that looks like
// one, and no row uses it, because a 2025-11-25 client gets a code-0 error and
// a 2026-07-28 SDK client retries on its own and then gives up without telling
// the model why.
func TestLoadShedding_EmptyInputRequests_BusyAtLegacyAndRetriedAtModern(t *testing.T) {
	sdk := sdkName()
	fix := "No row carries load shedding for that reason; if a row is to use it now, it is decided in the Refusal channels section of docs/development/tenant-policy-spec.md and in Carriages() in this same pull request."
	t.Run(legacyVersion+" on the wire", func(t *testing.T) {
		s, counts := newMatrixServer(nil)
		ex := dial(t, s, tenancy.EraLegacy).call(t, methodToolsCall, toolCall(busyTool))
		if we, ok := ex.wireError(); !ok || we.Code != 0 || we.Message != busyText || counts.busyCalls.Load() != 1 {
			t.Errorf("at %s a shed call was answered %s after %d handler call(s), want error 0 %q after one (%s). %s",
				legacyVersion, ex.describe(), counts.busyCalls.Load(), busyText, sdk, fix)
		}
	})
	t.Run(modernVersion+" on the wire", func(t *testing.T) {
		s, counts := newMatrixServer(nil)
		ex := dial(t, s, tenancy.EraModern).call(t, methodToolsCall, toolCall(busyTool))
		result, ok := ex.result()
		requests, isMap := result["inputRequests"].(map[string]any)
		if !ok || result["resultType"] != "input_required" || !isMap || len(requests) != 0 || counts.busyCalls.Load() != 1 {
			t.Errorf("at %s a shed call was answered %s after %d handler call(s), want an input_required result with no input requests after one (%s). %s",
				modernVersion, ex.describe(), counts.busyCalls.Load(), sdk, fix)
		}
	})
	t.Run(modernVersion+" through the SDK client", func(t *testing.T) {
		s, counts := newMatrixServer(nil)
		cs := sdkSession(t, s, tenancy.EraModern)
		_, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: busyTool, Arguments: map[string]any{}})
		_, fromTheWire := errors.AsType[*jsonrpc.Error](err)
		if err == nil || fromTheWire || !strings.Contains(err.Error(), "load-shedding") || counts.busyCalls.Load() != 3 {
			t.Errorf("at %s the SDK client's call to a shedding tool returned %v after %d handler call(s), want a local load-shedding error after three (%s). %s",
				modernVersion, err, counts.busyCalls.Load(), sdk, fix)
		}
	})
}

// sdkSession connects go-sdk's own client to s in era e.
func sdkSession(t *testing.T, s *mcp.Server, e tenancy.Era) *mcp.ClientSession {
	t.Helper()
	serverEnd, clientEnd := mcp.NewInMemoryTransports()
	session, err := s.Connect(context.Background(), serverEnd, nil)
	if err != nil {
		t.Fatalf("connect the server: %v", err)
	}
	var opts *mcp.ClientSessionOptions
	if e == tenancy.EraLegacy {
		opts = &mcp.ClientSessionOptions{ProtocolVersion: legacyVersion}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "carried-channels-client", Version: "0"}, nil).
		Connect(context.Background(), clientEnd, opts)
	if err != nil {
		_ = session.Close()
		t.Fatalf("connect the SDK client at %s: %v", eraName(e), err)
	}
	t.Cleanup(func() {
		_ = cs.Close()
		_ = session.Close()
	})
	if res := cs.InitializeResult(); res == nil || res.ProtocolVersion != eraName(e) {
		t.Fatalf("the SDK client negotiated %+v, want %s", res, eraName(e))
	}
	return cs
}

// The typed nil result is answered in a process of its own, because at
// 2026-07-28 the answer is a panic that ends the process.
const (
	// typedNilTest is the name of the test below, which starts this binary
	// again to run itself alone.
	typedNilTest = "TestTypedNilResult_FromAMiddleware_NullAtLegacyAndAPanicAtModern"
	// typedNilChild marks the process started to answer one typed nil result,
	// and names the protocol revision it answers in.
	typedNilChild = "TENANCY_TYPED_NIL_CHILD_REVISION"
	// typedNilMarker starts the line the child prints, so that the parent finds
	// it among the test framework's own output.
	typedNilMarker = "typed nil answered:"
)

// TestTypedNilResult_FromAMiddleware_NullAtLegacyAndAPanicAtModern holds what
// go-sdk does with a receiving middleware that answers with a typed nil result
// and no error: at 2025-11-25 it sends a null result, and at 2026-07-28 it
// dereferences the nil in setCompleteResultType, after the middleware chain
// has returned, where no middleware can recover it, and the process ends. The
// register's refusals are therefore errors, never a nil result, on every
// channel. Each era is answered in a child process of this test binary, so the
// panic ends the child and not the suite.
func TestTypedNilResult_FromAMiddleware_NullAtLegacyAndAPanicAtModern(t *testing.T) {
	if revision := os.Getenv(typedNilChild); revision != "" {
		answerTypedNil(t, revision)
		return
	}
	sdk := sdkName()
	t.Run(legacyVersion, func(t *testing.T) {
		status, out := runTypedNilChild(t, legacyVersion)
		if status != 0 || !strings.Contains(out, typedNilMarker+" result null") {
			t.Errorf("at %s a typed nil result exited %d and printed:\n%s\nwant a null result and exit 0 (%s). What a refusal may be answered with rests on this: re-read the refusals the layers return in this same pull request.",
				legacyVersion, status, out, sdk)
		}
	})
	t.Run(modernVersion, func(t *testing.T) {
		status, out := runTypedNilChild(t, modernVersion)
		if status == 0 || !strings.Contains(out, "nil pointer dereference") || !strings.Contains(out, "setCompleteResultType") {
			t.Errorf("at %s a typed nil result exited %d and printed:\n%s\nwant the process to panic in setCompleteResultType (%s). go-sdk no longer ends the process on it: the spec's Refusal channels section and the refusals the layers return may relax that rule in this same pull request.",
				modernVersion, status, out, sdk)
		}
	})
}

// answerTypedNil is the child: it answers one tools/list with a typed nil
// result in revision and prints what the client read.
func answerTypedNil(t *testing.T, revision string) {
	t.Helper()
	e := tenancy.EraLegacy
	if revision == modernVersion {
		e = tenancy.EraModern
	}
	s, _ := newMatrixServer(nil)
	s.AddReceivingMiddleware(answerWith(methodToolsList, func() (mcp.Result, error) {
		var none *mcp.ListToolsResult
		return none, nil
	}))
	ex := dial(t, s, e).call(t, methodToolsList, nil)
	fmt.Println(typedNilMarker, ex.describe())
}

// runTypedNilChild starts this test binary again to run the typed nil test
// alone in revision, and returns its exit status and everything it printed.
func runTypedNilChild(t *testing.T, revision string) (int, string) {
	t.Helper()
	env := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); name != "GOCOVERDIR" && name != typedNilChild {
			env = append(env, entry)
		}
	}
	// A binary built for coverage writes its counters where GOCOVERDIR says,
	// and warns on its output when nothing says, which is output this reads.
	env = append(env, typedNilChild+"="+revision, "GOCOVERDIR="+t.TempDir())

	// #nosec G204 G702 -- the program is this test binary, started again so that a panic ends a process of its own
	child := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+typedNilTest+"$", "-test.count=1")
	child.Env = env
	out, err := child.CombinedOutput()
	if exited, ok := errors.AsType[*exec.ExitError](err); ok {
		return exited.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("start the child: %v", err)
	}
	return 0, string(out)
}

// TestSubscriptionsListenResult_BuiltByApplicationCode_AnswersOnlyInPlaceOfTheSDKHandler
// holds what a server can do with the result that ends a subscription stream.
//
// Application code can build a SubscriptionsListenResult, and a middleware
// that returns one instead of calling the SDK's handler has it sent: the
// listen is answered before anything is subscribed or acknowledged. What
// application code has no way to do is answer a stream the SDK's handler has
// already acknowledged; ending that handler's context is what makes the SDK
// answer it, with its own result, which a middleware can still stamp. That is
// the mechanism cmd/server's listenStreams uses, and it is why it ends
// streams by context rather than by building the result.
func TestSubscriptionsListenResult_BuiltByApplicationCode_AnswersOnlyInPlaceOfTheSDKHandler(t *testing.T) {
	sdk := sdkName()
	t.Run("a middleware's own result answers a listen before anything is subscribed", func(t *testing.T) {
		s, counts := newMatrixServer(nil)
		s.AddReceivingMiddleware(answerWith(methodListen, func() (mcp.Result, error) {
			return &mcp.SubscriptionsListenResult{Meta: mcp.Meta{endReasonKey: endReason}}, nil
		}))
		ex := dial(t, s, tenancy.EraModern).call(t, methodListen, listenParams(resourceURI))
		result, ok := ex.result()
		if !ok || ex.acknowledged() || metaOf(result)[endReasonKey] != endReason || result["resultType"] != "complete" || counts.subscribes.Load() != 0 {
			t.Errorf("a listen answered by a middleware with a SubscriptionsListenResult it built reached the client as %s after %d subscribe(s), want that result, complete, with nothing acknowledged or subscribed (%s). cmd/server's listenStreams ends streams on the strength of what the SDK does with this result: re-read it in this same pull request.",
				ex.describe(), counts.subscribes.Load(), sdk)
		}
	})
	t.Run("an acknowledged stream ends with the SDK's own result once its handler's context ends", func(t *testing.T) {
		s, counts := newMatrixServer(nil)
		s.AddReceivingMiddleware(attempt{tenancy.ListenEnd, endedStream}.middleware(methodListen))
		s.AddSendingMiddleware(endOnAcknowledgment)
		w := dial(t, s, tenancy.EraModern)
		id := w.start(t, methodListen, listenParams(resourceURI))
		ex := w.await(t, id)
		result, ok := ex.result()
		meta := metaOf(result)
		// The id is compared as JSON writes it: the SDK holds a numeric id as
		// an int64, and a JSON number decodes as a float64.
		if !ok || !ex.acknowledged() || result["resultType"] != "complete" || fmt.Sprint(meta[mcp.MetaKeySubscriptionID]) != fmt.Sprint(id.Raw()) ||
			meta[endReasonKey] != endReason || counts.subscribes.Load() != 1 || counts.unsubscribes.Load() != 1 {
			t.Errorf("a listen whose handler's context ended after the acknowledgment reached the client as %s after %d subscribe(s) and %d unsubscribe(s), want the SDK's complete result carrying its subscription id %v and the stamped reason, after one of each (%s). This is how cmd/server's listenStreams ends a stream, and the ListenEnd channel of Carriages() rests on it: re-read both in this same pull request.",
				ex.describe(), counts.subscribes.Load(), counts.unsubscribes.Load(), id.Raw(), sdk)
		}
	})
}

// TestListenRefusal_BeforeTheAcknowledgment_UnseenByTheModernSDKClient holds
// the Refusal channels statement that a subscriptions/listen refusal is made
// before the acknowledgment and that a modern Go SDK client never observes it,
// whether a middleware or the subscribe handler refuses, while a legacy SDK
// client's subscribe does report the refusal.
func TestListenRefusal_BeforeTheAcknowledgment_UnseenByTheModernSDKClient(t *testing.T) {
	sdk := sdkName()
	refusal := &jsonrpc.Error{Code: tenancy.CodeServerBusyLegacy, Message: refusalText}
	fix := "The Refusal channels section of docs/development/tenant-policy-spec.md states this: edit it, and the listen row of Carriages() if its channels moved, in this same pull request."
	servers := []struct {
		name  string
		build func() *mcp.Server
	}{
		{"refused by a receiving middleware", func() *mcp.Server {
			s, _ := newMatrixServer(nil)
			s.AddReceivingMiddleware(refuseWith(methodListen, refusal))
			return s
		}},
		{"refused by the subscribe handler", func() *mcp.Server {
			s, _ := newMatrixServer(refusal)
			return s
		}},
	}
	for _, server := range servers {
		t.Run(server.name+" on the wire", func(t *testing.T) {
			ex := dial(t, server.build(), tenancy.EraModern).call(t, methodListen, listenParams(resourceURI))
			if we, ok := ex.wireError(); !ok || we.Code != tenancy.CodeServerBusyLegacy || ex.acknowledged() {
				t.Errorf("a refused listen reached the client as %s, want error %d with no acknowledgment before it (%s). %s",
					ex.describe(), tenancy.CodeServerBusyLegacy, sdk, fix)
			}
		})
		t.Run(server.name+" through the SDK client", func(t *testing.T) {
			cs := sdkSession(t, server.build(), tenancy.EraModern)
			if err := cs.Subscribe(context.Background(), &mcp.SubscribeParams{URI: resourceURI}); err != nil {
				t.Errorf("a modern SDK client's Subscribe to a refused resource returned %v, want nil: the client does not wait for the listen it opens (%s). %s", err, sdk, fix)
			}
		})
	}
	t.Run("a legacy SDK client sees the subscribe handler's refusal", func(t *testing.T) {
		s, _ := newMatrixServer(refusal)
		err := sdkSession(t, s, tenancy.EraLegacy).Subscribe(context.Background(), &mcp.SubscribeParams{URI: resourceURI})
		if we, ok := errors.AsType[*jsonrpc.Error](err); !ok || we.Code != tenancy.CodeServerBusyLegacy {
			t.Errorf("a legacy SDK client's Subscribe to a refused resource returned %v, want error %d (%s). %s", err, tenancy.CodeServerBusyLegacy, sdk, fix)
		}
	})
}
