package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestRunMain_Subcommands_DispatchOrRefuse verifies the dispatch: no
// subcommand and an unknown one are usage errors that print the usage, and
// each known subcommand is reached with the arguments after its name, which
// the plan's matrix and summarize's own usage error show.
func TestRunMain_Subcommands_DispatchOrRefuse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{name: "no subcommand", args: nil, wantCode: 2, wantStderr: usage},
		{
			name: "unknown subcommand", args: []string{"measure"}, wantCode: 2,
			wantStderr: "measure_conditions: unknown subcommand \"measure\"\n" + usage,
		},
		{
			name: "plan", args: []string{"plan", "-packages", "./cmd/server", "-systems", "macos-latest", "-gate", "all"},
			wantStdout: `matrix={"include":[{"system":"macos-latest","package":"./cmd/server","artifact":"gobco-macos-latest-cmd-server"}]}` + "\n",
			wantStderr: "measure_conditions: 1 run(s) planned, ./cmd/server on macos-latest, with GOBCO_GATE=all\n",
		},
		{
			name: "summarize", args: []string{"summarize"}, wantCode: 2,
			wantStderr: "measure_conditions: summarize takes -plan, -records, -gate and -commit, all four non-empty, and no arguments\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr bytes.Buffer
			if got := runMain(tc.args, &stdout, &stderr); got != tc.wantCode {
				t.Fatalf("runMain(%q) = %d, want %d; stderr:\n%s", tc.args, got, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if stderr.String() != tc.wantStderr {
				t.Errorf("stderr = %q, want %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}

// TestMain_NoSubcommand_ExitsWithTheUsageStatus verifies that main hands the
// process arguments after its own name to runMain and passes the status on to
// the exit seam.
func TestMain_NoSubcommand_ExitsWithTheUsageStatus(t *testing.T) {
	previousArgs, previousExit := os.Args, osExit
	t.Cleanup(func() { os.Args, osExit = previousArgs, previousExit })
	got := -1
	os.Args = []string{"measure_conditions"}
	osExit = func(code int) { got = code }
	main()
	if got != 2 {
		t.Errorf("main() exit = %d, want 2", got)
	}
}

// TestUsage_NamesEverySubcommand verifies the usage text names both
// subcommands the dispatch accepts.
func TestUsage_NamesEverySubcommand(t *testing.T) {
	t.Parallel()
	for _, subcommand := range []string{"plan", "summarize"} {
		t.Run(subcommand, func(t *testing.T) {
			t.Parallel()
			if !strings.Contains(usage, "  "+subcommand+" ") {
				t.Errorf("usage does not name %q:\n%s", subcommand, usage)
			}
		})
	}
}
