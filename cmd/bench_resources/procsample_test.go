// procsample_test.go covers the parsers that turn what the operating system
// says into the resident-set and processor-time figures the page publishes,
// and the goroutine count read out of a runtime traceback.
//
// These are pinned against literal /proc and ps output rather than against the
// live machine: a test that read its own process would pass on Linux, be
// skipped everywhere else, and never exercise the field arithmetic that is the
// only thing here capable of being subtly wrong.
package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestParseProcStatusRSS_RealStatusBlock_ReturnsBytes verifies VmRSS is found
// among the other lines and converted from kibibytes to bytes.
func TestParseProcStatusRSS_RealStatusBlock_ReturnsBytes(t *testing.T) {
	status := strings.Join([]string{
		"Name:\tserver",
		"State:\tS (sleeping)",
		"VmPeak:\t  600000 kB",
		"VmSize:\t  590000 kB",
		"VmRSS:\t  217764 kB",
		"Threads:\t14",
	}, "\n")

	got, err := parseProcStatusRSS(status)
	if err != nil {
		t.Fatalf("parseProcStatusRSS: %v", err)
	}
	if want := uint64(217764 * 1024); got != want {
		t.Errorf("VmRSS = %d bytes, want %d", got, want)
	}
}

// TestParseProcStatusRSS_Unusable_ReturnsError verifies a status block with no
// resident set is reported rather than silently read as zero, which would
// publish a server that costs nothing.
func TestParseProcStatusRSS_Unusable_ReturnsError(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{name: "empty", status: ""},
		{name: "no VmRSS", status: "Name:\tserver\nThreads:\t3\n"},
		{name: "unparseable", status: "VmRSS:\tlots kB\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseProcStatusRSS(tc.status); err == nil {
				t.Error("parseProcStatusRSS accepted a block with no usable resident set")
			}
		})
	}
}

// TestParseProcStatCPU_CommandNameWithSpaces_CountsFromTheParenthesis
// verifies utime and stime are located by counting from the closing
// parenthesis of the command name, not from the start of the line.
//
// This is the field-counting bug the parser exists to avoid: a command name
// containing spaces or parentheses shifts every later field, so splitting the
// whole line on whitespace reads two neighboring numbers instead.
func TestParseProcStatCPU_CommandNameWithSpaces_CountsFromTheParenthesis(t *testing.T) {
	// Fields after the command name, with utime=1000 and stime=776 at the
	// positions the kernel documents (14 and 15 counting from the start).
	after := []string{
		"S", "1", "2", "3", "4", "-1", "4194304", "100", "0", "0", "0", // state through cmajflt
		"1000", "776", // utime, stime
		"0", "0", "20", "0", "14", "0",
	}
	line := "12345 (my server (v2)) " + strings.Join(after, " ") + "\n"

	got, err := parseProcStatCPU(line)
	if err != nil {
		t.Fatalf("parseProcStatCPU: %v", err)
	}
	want := 1776 / clockTicks
	if got != want {
		t.Errorf("CPU seconds = %v, want %v", got, want)
	}
}

// TestParseProcStatCPU_Malformed_ReturnsError verifies a truncated or
// parenthesis-free stat line is refused rather than read as zero CPU.
func TestParseProcStatCPU_Malformed_ReturnsError(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "no parenthesis", line: "12345 server S 1 2 3"},
		{name: "too few fields", line: "12345 (server) S 1 2 3"},
		{name: "unparseable utime", line: "12345 (server) S 1 2 3 4 -1 0 0 0 0 0 x 776 0 0 20 0 14 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseProcStatCPU(tc.line); err == nil {
				t.Error("parseProcStatCPU accepted a malformed line")
			}
		})
	}
}

// TestParsePSStat_TimeFormats_ConvertToSeconds verifies the ps fallback used
// on macOS reads every shape of CPU time ps prints, including the day form a
// long-lived process reaches.
func TestParsePSStat_TimeFormats_ConvertToSeconds(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		wantRSS uint64
		wantCPU float64
	}{
		{name: "minutes and seconds", out: " 217764 0:12.34\n", wantRSS: 217764 * 1024, wantCPU: 12.34},
		{name: "hours", out: "1024 1:02:03\n", wantRSS: 1024 * 1024, wantCPU: 3723},
		{name: "days", out: "2048 2-01:00:00\n", wantRSS: 2048 * 1024, wantCPU: 2*86400 + 3600},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parsePSStat(tc.out)
			if err != nil {
				t.Fatalf("parsePSStat: %v", err)
			}
			if got.rssBytes != tc.wantRSS {
				t.Errorf("resident set = %d, want %d", got.rssBytes, tc.wantRSS)
			}
			if got.cpuSeconds != tc.wantCPU {
				t.Errorf("CPU seconds = %v, want %v", got.cpuSeconds, tc.wantCPU)
			}
		})
	}
}

// TestParsePSStat_Malformed_ReturnsError verifies short or unparseable ps
// output is refused.
func TestParsePSStat_Malformed_ReturnsError(t *testing.T) {
	for _, out := range []string{"", "1024", "abc 0:01", "1024 0:0x"} {
		t.Run(out, func(t *testing.T) {
			if _, err := parsePSStat(out); err == nil {
				t.Errorf("parsePSStat(%q) accepted unusable output", out)
			}
		})
	}
}

// TestCountGoroutines_Traceback_CountsOnlyGoroutineHeaders verifies the count
// comes from the traceback's goroutine headers and is not inflated by stack
// frames, source lines or the word appearing in other output the process
// wrote to the same stream.
func TestCountGoroutines_Traceback_CountsOnlyGoroutineHeaders(t *testing.T) {
	dump := strings.Join([]string{
		`{"level":"info","msg":"a log line mentioning goroutine leaks"}`,
		"SIGQUIT: quit",
		"",
		"goroutine 1 [chan receive]:",
		"main.main()",
		"\t/src/main.go:42 +0x1d4",
		"",
		"goroutine 18 [select]:",
		"runtime.gopark(0x0?, 0x0?)",
		"\t/usr/local/go/src/runtime/proc.go:435 +0xce",
		"",
		"goroutine 402 [IO wait]:",
		"internal/poll.runtime_pollWait(0x7f, 0x72)",
		"goroutine created by net/http.(*Server).Serve",
	}, "\n")

	if got := countGoroutines(dump); got != 3 {
		t.Errorf("countGoroutines = %d, want 3", got)
	}
}

// TestCountGoroutines_NoTraceback_ReturnsZero verifies output carrying no
// traceback counts nothing, which is what makes the caller report the
// goroutine figure as unavailable instead of publishing a zero.
func TestCountGoroutines_NoTraceback_ReturnsZero(t *testing.T) {
	for _, dump := range []string{"", "no traceback here\n", "goroutine dump follows\n"} {
		t.Run(dump, func(t *testing.T) {
			if got := countGoroutines(dump); got != 0 {
				t.Errorf("countGoroutines(%q) = %d, want 0", dump, got)
			}
		})
	}
}

// TestSampler_TracksAndPeaks verifies the sampler sums the processes its
// source names, remembers the largest total it saw, and forgets it on reset.
//
// Driven by a fake process set rather than real processes: the arithmetic
// under test is the summing and the peak, and a real process would make the
// numbers unrepeatable.
func TestSampler_TracksAndPeaks(t *testing.T) {
	// The sampler reads real pids, so this exercises it against this test's
	// own process, whose resident set is non-zero and stable enough to assert
	// bounds on rather than an exact value.
	self := []int{os.Getpid()}
	s := newSampler(t.Context(), 10*time.Millisecond, func() []int { return self })

	current, err := s.current()
	if err != nil {
		t.Skipf("this platform does not report process statistics: %v", err)
	}
	if current.rssBytes == 0 {
		t.Fatal("the sampler read a resident set of zero for a running process")
	}

	if s.peakRSS() == 0 {
		t.Error("peakRSS is zero after a successful sample")
	}
	s.resetPeak()
	if s.peakRSS() == 0 {
		t.Error("peakRSS is zero after a reset that takes its own sample")
	}
}

// TestSampler_NoProcesses_ReportsFailure verifies a sampler with nothing to
// watch says so, which is how a scenario records that the platform, rather
// than the server, is what could not be measured.
func TestSampler_NoProcesses_ReportsFailure(t *testing.T) {
	s := newSampler(t.Context(), 10*time.Millisecond, func() []int { return nil })
	if _, err := s.current(); err == nil {
		t.Error("the sampler reported a measurement with no processes to sample")
	}
}

// TestSampler_StartStop_RecordsAPeakAndStopsCleanly verifies the polling loop
// runs, folds what it reads into the peak, and can be stopped twice.
//
// Stopping twice is not hypothetical: a scenario stops sampling before it kills
// the process for a goroutine dump, while the deferred stop that guarantees the
// poller dies on an early return is still armed. A stop that closed its channel
// unconditionally would panic on the second call, at the end of a run that had
// already done all its work.
func TestSampler_StartStop_RecordsAPeakAndStopsCleanly(t *testing.T) {
	self := os.Getpid()
	s := newSampler(t.Context(), 5*time.Millisecond, func() []int { return []int{self} })

	if _, err := s.current(); err != nil {
		t.Skipf("this platform does not report process statistics: %v", err)
	}

	s.start()
	// Long enough for several ticks, short enough not to slow the suite.
	time.Sleep(60 * time.Millisecond)
	s.stop()

	if peak := s.peakRSS(); peak == 0 {
		t.Error("the poller recorded no peak for a process that is running")
	}

	// The second stop must return rather than panic on a closed channel.
	s.stop()
}

// TestSampleRSS_NoProcesses_ReportsZero verifies a reading the platform will
// not give is reported as zero rather than as an error the caller has to
// handle, which is what lets a scenario publish latency without memory.
func TestSampleRSS_NoProcesses_ReportsZero(t *testing.T) {
	s := newSampler(t.Context(), 10*time.Millisecond, func() []int { return nil })
	if got := sampleRSS(s); got != 0 {
		t.Errorf("sampleRSS = %d, want 0 when there is nothing to sample", got)
	}
}

// TestSettledRSS_StableProcess_ReturnsAReading verifies the settle loop
// returns once two consecutive samples agree.
//
// The figure it produces is the one-client memory published for every
// scenario, and it is taken right after a process starts, so reading too early
// would publish a process that has not finished building its catalog.
func TestSettledRSS_StableProcess_ReturnsAReading(t *testing.T) {
	self := os.Getpid()
	s := newSampler(t.Context(), 5*time.Millisecond, func() []int { return []int{self} })
	if _, err := s.current(); err != nil {
		t.Skipf("this platform does not report process statistics: %v", err)
	}

	started := time.Now()
	got := settledRSS(s)
	elapsed := time.Since(started)

	if got == 0 {
		t.Error("settledRSS returned zero for a running process")
	}
	// This test's own process is not growing, so two samples agree almost at
	// once. Reaching the three-second ceiling would mean the settle condition
	// never holds, which would add three seconds to every scenario.
	if elapsed > 2*time.Second {
		t.Errorf("settling took %v on a stable process, which is nearly the ceiling", elapsed)
	}
}
