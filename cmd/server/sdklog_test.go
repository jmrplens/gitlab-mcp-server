package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

// sdkLogSink builds a logger wired the way createServer wires the SDK's, and
// returns the buffer its records land in.
//
// The base is a JSON handler because that is what the server installs, and the
// collision this handler exists to prevent is a property of writing a record's
// fields and its attributes into one flat object.
func sdkLogSink(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: level})
	return slog.New(&sdkLogHandler{base: base}), &buf
}

// decodeRecords parses each line the sink collected.
func decodeRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()

	var records []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(line), &decoded); err != nil {
			t.Fatalf("log line is not JSON: %q (%v)", line, err)
		}
		records = append(records, decoded)
	}
	return records
}

// TestSDKLogHandler_SessionChatterIsNotTheDefaultStream pins that the SDK's
// per-session messages stay out of the default log.
//
// Measured on the shipped default before this existed: twenty-four tools/call
// produced ninety-six records, every one of them one of these four. In
// stateless HTTP each POST is a session, so they are emitted per request, and
// the calls themselves are logged at DEBUG by the dynamic surface and at INFO
// by nothing. The stream was full and carried no information.
//
// Demotion rather than suppression is the point of the second half: an operator
// debugging a session still gets all of it by asking for DEBUG.
func TestSDKLogHandler_SessionChatterIsNotTheDefaultStream(t *testing.T) {
	chatter := []string{
		"server run start",
		"server connecting",
		"server session connected",
		"session initialized",
		"client log level set",
		"server session disconnected",
		"server session ended",
	}

	t.Run("absent at the default level", func(t *testing.T) {
		logger, buf := sdkLogSink(slog.LevelInfo)
		for _, msg := range chatter {
			logger.Info(msg)
		}
		logger.Info("registered dynamic toolset", "tools", 2)

		records := decodeRecords(t, buf)
		if len(records) != 1 {
			t.Fatalf("%d records at INFO, want 1; only the message that says something", len(records))
		}
		if got := records[0]["msg"]; got != "registered dynamic toolset" {
			t.Errorf("the surviving record is %q, want the one carrying information", got)
		}
	})

	t.Run("available at debug", func(t *testing.T) {
		logger, buf := sdkLogSink(slog.LevelDebug)
		for _, msg := range chatter {
			logger.Info(msg)
		}

		records := decodeRecords(t, buf)
		if len(records) != len(chatter) {
			t.Fatalf("%d records at DEBUG, want all %d: demoted, not discarded", len(records), len(chatter))
		}
		for i, record := range records {
			if record["level"] != "DEBUG" {
				t.Errorf("%q logged at %v, want DEBUG", chatter[i], record["level"])
			}
		}
	})
}

// TestSDKLogHandler_LevelAttributeDoesNotOverwriteTheSeverity pins the fix for
// a corrupted record, not merely a noisy one.
//
// The SDK logs "client log level set" with an attribute it calls level, which
// is also slog's name for the record's severity. Neither side deduplicates, so
// the line went out with two level members, and a parser taking the last one
// (encoding/json does, and so does every aggregator I know of) read the
// severity as the empty string. One steady-state line in four arrived with no
// severity at all.
//
// The assertion is made through a real parser rather than on the raw text,
// because the defect is invisible in the text: both members are there.
func TestSDKLogHandler_LevelAttributeDoesNotOverwriteTheSeverity(t *testing.T) {
	logger, buf := sdkLogSink(slog.LevelDebug)
	logger.Info("client log level set", "level", "")

	records := decodeRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("%d records, want 1", len(records))
	}

	if got := records[0]["level"]; got != "DEBUG" {
		t.Errorf("level = %v, want DEBUG; the attribute overwrote the severity", got)
	}
	if _, ok := records[0]["sdk_level"]; !ok {
		t.Error("the SDK's own level attribute was dropped rather than renamed")
	}
}

// TestSDKLogHandler_DemotesTheShutdownPair pins that an ordinary stop is not
// announced as a failure.
//
// These are the two lines the SDK emits when a server stops: one when the
// context is cancelled, which in this process only ever happens because a
// signal arrived, and one when a session ends carrying an error, which on stdio
// is what a client closing its pipe mid-call produces. Both were ERROR. Between
// them they meant every normal shutdown wrote two error lines, alongside the
// nonzero exit status serveStdio no longer returns.
//
// Pinning the exact wording is the point of the first two cases: the match is
// on message text, because the sentinel behind the cancellation and the EOF
// behind the closing error are both unreachable through errors.Is. A wording
// change upstream fails here rather than silently restoring the noise.
func TestSDKLogHandler_DemotesTheShutdownPair(t *testing.T) {
	cases := []struct {
		name  string
		msg   string
		cause string
		want  string
	}{
		{
			name: "a signal stopped the server",
			msg:  "server run cancelled", cause: "context canceled", want: "DEBUG",
		},
		{
			name: "the client closed its pipe with a call in flight",
			msg:  "server session ended with error", cause: "server is closing: EOF", want: "DEBUG",
		},
		{
			// The demotion must not swallow a session that ended badly for a
			// reason nobody asked for.
			name: "a genuine transport failure is still an error",
			msg:  "server session ended with error", cause: "read: connection reset by peer", want: "ERROR",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logger, buf := sdkLogSink(slog.LevelDebug)
			logger.Error(tc.msg, "error", tc.cause)

			records := decodeRecords(t, buf)
			if len(records) != 1 {
				t.Fatalf("%d records, want 1", len(records))
			}
			if got := records[0]["level"]; got != tc.want {
				t.Errorf("%q with error %q logged at %v, want %s", tc.msg, tc.cause, got, tc.want)
			}
		})
	}
}

// TestSDKLogHandler_PassesEverythingElseThrough guards the other direction: a
// filter that quietly ate an unrelated message would be worse than the noise it
// was added to remove.
func TestSDKLogHandler_PassesEverythingElseThrough(t *testing.T) {
	logger, buf := sdkLogSink(slog.LevelInfo)
	logger.Error("jsonrpc2 internal error", "error", "boom")
	logger.Warn("handler returned both content and inputRequests")
	logger.Info("resource subscribed", "uri", "gitlab://project/42")

	records := decodeRecords(t, buf)
	if len(records) != 3 {
		t.Fatalf("%d records, want 3; nothing here is session chatter", len(records))
	}
	for i, want := range []string{"ERROR", "WARN", "INFO"} {
		if got := records[i]["level"]; got != want {
			t.Errorf("record %d logged at %v, want %s", i, got, want)
		}
	}
	if got := records[2]["uri"]; got != "gitlab://project/42" {
		t.Errorf("attributes were not preserved: uri = %v", got)
	}
}

// TestSDKLogHandler_WithAttrsKeepsTheHandlerWrapping checks that attaching
// attributes returns another wrapping handler rather than the bare base.
//
// slog calls WithAttrs whenever a logger is derived with slog.With, and a
// handler that returned its base there would quietly stop demoting and
// renaming for that logger. Nothing in the SDK derives one today, which is
// exactly why this needs a test: an implementation nobody exercises is one
// nobody notices breaking.
func TestSDKLogHandler_WithAttrsKeepsTheHandlerWrapping(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New((&sdkLogHandler{base: base}).WithAttrs([]slog.Attr{slog.String("component", "sdk")}))

	logger.Info("server session connected", "session_id", "")
	logger.Info("resource subscribed", "uri", "gitlab://project/42")

	records := decodeRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("%d records at INFO, want 1; the chatter must still be demoted after WithAttrs", len(records))
	}
	if got := records[0]["msg"]; got != "resource subscribed" {
		t.Errorf("the surviving record is %q, want the one that is not session chatter", got)
	}
	if got := records[0]["component"]; got != "sdk" {
		t.Errorf("the attached attribute was lost: %v", records[0])
	}
}

// TestSDKLogHandler_ShutdownWithoutACause covers a session error carrying no
// error attribute at all, which must stay at ERROR: the demotion is for the two
// shutdowns this server asked for, and an unexplained one is not among them.
func TestSDKLogHandler_ShutdownWithoutACause(t *testing.T) {
	logger, buf := sdkLogSink(slog.LevelDebug)
	logger.Error("server session ended with error", "session_id", "abc")

	records := decodeRecords(t, buf)
	if len(records) != 1 {
		t.Fatalf("%d records, want 1", len(records))
	}
	if got := records[0]["level"]; got != "ERROR" {
		t.Errorf("level = %v, want ERROR for a session error with no stated cause", got)
	}
}

// TestSDKLogHandler_GroupedAttributesAreLeftAlone checks the one case where the
// rename must not happen: inside a group the attributes are nested, so a key
// named level cannot reach the record's own field.
func TestSDKLogHandler_GroupedAttributesAreLeftAlone(t *testing.T) {
	var buf bytes.Buffer
	base := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New((&sdkLogHandler{base: base}).WithGroup("sdk"))
	logger.Info("resource updated notification sent", "level", "info")

	records := decodeRecords(t, &buf)
	if len(records) != 1 {
		t.Fatalf("%d records, want 1", len(records))
	}
	group, ok := records[0]["sdk"].(map[string]any)
	if !ok {
		t.Fatalf("the group is missing: %v", records[0])
	}
	if got := group["level"]; got != "info" {
		t.Errorf("grouped attribute renamed to something else: %v", group)
	}
	if got := records[0]["level"]; got != "INFO" {
		t.Errorf("record severity = %v, want INFO", got)
	}
}
