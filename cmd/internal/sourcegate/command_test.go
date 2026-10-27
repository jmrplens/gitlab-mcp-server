// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sourcegate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
)

// gateName is the name the command lines under test are given.
const gateName = "audit_example"

// exampleUsage is the help text the command lines under test are given, each
// line distinct so the help output shows which flag it belongs to.
var exampleUsage = Usage{
	Dir:     "the directory the example is run in",
	Check:   "exit non-zero when the example finds something",
	Verbose: "also list what the example excused",
}

// TestCommand_Parse_ReadsTheThreeFlagsAndWhatFollowsThem holds what each
// command line does: an unknown flag is a usage error and a help request is
// not a failure, each answered on the gate's standard error under the gate's
// name, and anything that parses reaches the fields and the patterns it
// describes.
func TestCommand_Parse_ReadsTheThreeFlagsAndWhatFollowsThem(t *testing.T) {
	defaults := []string{"./default/..."}
	for _, tt := range []struct {
		name         string
		args         []string
		wantCode     int
		wantGoOn     bool
		wantStderr   []string
		wantCommand  Command
		wantPatterns []string
	}{
		{
			name: "an unknown flag", args: []string{"-nope"}, wantCode: 2,
			wantStderr: []string{"flag provided but not defined: -nope\nUsage of audit_example:\n"},
		},
		{
			name: "a request for help", args: []string{"-h"}, wantCode: 0,
			wantStderr: []string{"Usage of audit_example:\n", exampleUsage.Dir, exampleUsage.Check, exampleUsage.Verbose},
		},
		{
			name: "nothing", wantGoOn: true,
			wantCommand: Command{Dir: "."}, wantPatterns: defaults,
		},
		{
			name: "every flag and two patterns", args: []string{"-dir", "/repo", "-check", "-v", "./a/...", "./b"}, wantGoOn: true,
			wantCommand: Command{Dir: "/repo", Check: true, Verbose: true}, wantPatterns: []string{"./a/...", "./b"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stderr strings.Builder
			cmd := NewCommand(gateName, exampleUsage, &stderr)
			code, goOn := cmd.Parse(tt.args)
			if code != tt.wantCode || goOn != tt.wantGoOn {
				t.Fatalf("Parse(%q) = %d, %v, want %d, %v", tt.args, code, goOn, tt.wantCode, tt.wantGoOn)
			}
			for _, want := range tt.wantStderr {
				if !strings.Contains(stderr.String(), want) {
					t.Errorf("stderr = %q, want it to hold %q", stderr.String(), want)
				}
			}
			if len(tt.wantStderr) == 0 && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want nothing", stderr.String())
			}
			if !goOn {
				return
			}
			got := Command{Dir: cmd.Dir, Check: cmd.Check, Verbose: cmd.Verbose}
			if got != tt.wantCommand {
				t.Errorf("parsed = %+v, want %+v", got, tt.wantCommand)
			}
			if patterns := cmd.Patterns(defaults); !slices.Equal(patterns, tt.wantPatterns) {
				t.Errorf("Patterns = %v, want %v", patterns, tt.wantPatterns)
			}
		})
	}
}

// TestCommand_Flags_TakesAFlagOfTheGatesOwn verifies a gate can define a flag
// beside the three before parsing, and that the arguments after the flags are
// read from the same set.
func TestCommand_Flags_TakesAFlagOfTheGatesOwn(t *testing.T) {
	cmd := NewCommand(gateName, exampleUsage, io.Discard)
	compare := cmd.Flags.Bool("compare", false, "compare instead")
	if code, goOn := cmd.Parse([]string{"-compare", "-check", "left", "right"}); code != 0 || !goOn {
		t.Fatalf("Parse = %d, %v, want 0, true", code, goOn)
	}
	if !*compare || !cmd.Check {
		t.Errorf("compare = %v, check = %v, want both set", *compare, cmd.Check)
	}
	if got := cmd.Flags.Args(); !slices.Equal(got, []string{"left", "right"}) {
		t.Errorf("Args = %v, want left right", got)
	}
}

// TestMain_HandsTheProcessToTheCommandLineAndExitsWithItsCode verifies Main
// gives a gate's command line the arguments after the program's name and the
// process's standard output and standard error, in that order, and ends
// through Exit with the code the command line returned.
func TestMain_HandsTheProcessToTheCommandLineAndExitsWithItsCode(t *testing.T) {
	previousExit, previousArgs := Exit, os.Args
	t.Cleanup(func() { Exit, os.Args = previousExit, previousArgs })
	var codes []int
	Exit = func(code int) { codes = append(codes, code) }
	os.Args = []string{gateName, "-check", "./a/..."}
	var gotArgs []string
	var gotStdout, gotStderr io.Writer
	Main(func(args []string, stdout, stderr io.Writer) int {
		gotArgs, gotStdout, gotStderr = args, stdout, stderr
		return 3
	})
	if !slices.Equal(codes, []int{3}) {
		t.Errorf("exit codes = %v, want the 3 the command line returned", codes)
	}
	if !slices.Equal(gotArgs, []string{"-check", "./a/..."}) {
		t.Errorf("args = %q, want the ones after the program's name", gotArgs)
	}
	if gotStdout != io.Writer(os.Stdout) || gotStderr != io.Writer(os.Stderr) {
		t.Errorf("streams = %v, %v, want standard output then standard error", gotStdout, gotStderr)
	}
}

// TestRun_StartsTheGateOnlyOnACommandLineThatParsed verifies Run hands a
// parsed command line to the gate and returns what the gate answered, and
// that a request for help or an unknown flag ends the run with Parse's code
// before the gate is started.
func TestRun_StartsTheGateOnlyOnACommandLineThatParsed(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		wantCode    int
		wantStarted bool
	}{
		{name: "a command line that parses", args: []string{"-dir", "/repo", "-v"}, wantCode: 7, wantStarted: true},
		{name: "a request for help", args: []string{"-h"}, wantCode: 0},
		{name: "an unknown flag", args: []string{"-nope"}, wantCode: 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var started *Command
			code := Run(gateName, exampleUsage, tt.args, io.Discard, func(cmd *Command) int {
				started = cmd
				return 7
			})
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d", code, tt.wantCode)
			}
			if (started != nil) != tt.wantStarted {
				t.Fatalf("started = %v, want %v", started != nil, tt.wantStarted)
			}
			if started != nil && (started.Dir != "/repo" || !started.Verbose || started.Check) {
				t.Errorf("started with %+v, want the command line it was given", started)
			}
		})
	}
}

// exampleReport is a report whose verdict is set by the test, and which writes
// one line saying whether it was asked to be verbose.
type exampleReport struct{ clean bool }

// Write writes the one line.
func (r exampleReport) Write(out io.Writer, verbose bool) {
	fmt.Fprintf(out, "report verbose=%v\n", verbose)
}

// OK is the verdict the test set.
func (r exampleReport) OK() bool { return r.clean }

// TestFinish_ExitCodesAndStreams holds the end of every run: a run that could
// not be made says so on standard error under the gate's name, writes no
// report and exits 1; a run that was made writes its report, verbose or not as
// asked, and exits 1 only under -check over a report that is not clean.
func TestFinish_ExitCodesAndStreams(t *testing.T) {
	for _, tt := range []struct {
		name       string
		report     exampleReport
		err        error
		check      bool
		verbose    bool
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "a run that could not be made", err: errors.New("no root"), check: true, wantCode: 1, wantStderr: "audit_example: no root\n"},
		{name: "a finding without check", wantStdout: "report verbose=false\n"},
		{name: "a finding under check", check: true, wantCode: 1, wantStdout: "report verbose=false\n"},
		{name: "a clean run under check", report: exampleReport{clean: true}, check: true, wantStdout: "report verbose=false\n"},
		{name: "a verbose run", report: exampleReport{clean: true}, verbose: true, wantStdout: "report verbose=true\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			code := Finish(gateName, tt.report, tt.err, tt.check, tt.verbose, &stdout, &stderr)
			if code != tt.wantCode {
				t.Errorf("exit = %d, want %d", code, tt.wantCode)
			}
			if stdout.String() != tt.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantStdout)
			}
			if stderr.String() != tt.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
