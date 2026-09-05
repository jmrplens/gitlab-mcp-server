package main

import (
	"flag"
	"io"
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v2/internal/config"
)

// withFreshFlagSet swaps in a clean flag set, so a test can register and parse
// without inheriting whatever main already declared or another test already
// passed.
func withFreshFlagSet(t *testing.T) {
	t.Helper()

	previous := flag.CommandLine
	flag.CommandLine = flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	// io.Discard, not a file over descriptor 0. os.NewFile(0, ...) wraps
	// STDIN, so sending flag errors there writes to the process's own input
	// and disturbs it for every later test in the package: the first version
	// of this helper did exactly that, and an unrelated stdio test started
	// failing with "read /dev/stdin: transport endpoint is not connected".
	flag.CommandLine.SetOutput(io.Discard)
	t.Cleanup(func() { flag.CommandLine = previous })
}

// TestEnvBackedFlags_APassedFlagBeatsTheEnvironment pins the precedence this
// project documents everywhere else: an explicitly passed flag, then the
// environment, then the built-in default.
//
// It is the assertion that matters most for these four, because they are
// implemented by writing the environment rather than by being read directly. A
// version that wrote unconditionally would work in this direction and break the
// next one.
//
// Asserted through [config.Getenv] rather than os.Getenv, because that is what
// the readers use: the flag writes the prefixed spelling, and a flag losing to
// the operator's own deprecated variable is exactly the regression reading the
// bare name here would miss.
func TestEnvBackedFlags_APassedFlagBeatsTheEnvironment(t *testing.T) {
	withFreshFlagSet(t)
	// applyEnvBackedFlags writes the environment with os.Setenv, which nothing
	// would undo on its own. Claiming the name through t.Setenv first is what
	// makes that write revert with this test instead of leaking a log level
	// into every later test in the package.
	t.Setenv(config.EnvPrefix+"LOG_LEVEL", "")
	t.Setenv("LOG_LEVEL", "debug")

	registerEnvBackedFlags()
	if err := flag.CommandLine.Parse([]string{"-log-level=error"}); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	applyEnvBackedFlags()

	if got := config.Getenv("LOG_LEVEL"); got != "error" {
		t.Errorf("LOG_LEVEL = %q, want the explicitly passed %q", got, "error")
	}
}

// TestEnvBackedFlags_AnUnpassedFlagLeavesTheEnvironmentAlone is the direction
// that a naive implementation gets wrong, and it is the one that would break
// every deployment configured through the environment.
//
// Writing the flag's value unconditionally means writing the empty string when
// nobody passed it, which clears a variable the operator did set. Nothing would
// error; the server would just start at the wrong log level, with no
// compatibility mode, and with the upload limit back to its default.
func TestEnvBackedFlags_AnUnpassedFlagLeavesTheEnvironmentAlone(t *testing.T) {
	withFreshFlagSet(t)
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("CLIENT_COMPAT", "off")
	t.Setenv("UPLOAD_MAX_FILE_SIZE", "1048576")
	t.Setenv("YOLO_MODE", "true")

	registerEnvBackedFlags()
	if err := flag.CommandLine.Parse(nil); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	applyEnvBackedFlags()

	for name, want := range map[string]string{
		"LOG_LEVEL":            "debug",
		"CLIENT_COMPAT":        "off",
		"UPLOAD_MAX_FILE_SIZE": "1048576",
		"YOLO_MODE":            "true",
	} {
		t.Run(name, func(t *testing.T) {
			if got := os.Getenv(name); got != want {
				t.Errorf("%s = %q, want %q; an unpassed flag cleared a variable the operator set", name, got, want)
			}
		})
	}
}

// TestEnvBackedFlags_EverySettingIsReachableFromTheCommandLine is the rule this
// file exists to satisfy: one command must be able to configure the server.
//
// Enumerated rather than inferred, because a new setting added to the list has
// to be a deliberate act. The pairing is asserted too: a flag pointing at the
// wrong variable would pass every other test here and silently configure
// something else.
//
// Every name carries the GITLAB_MCP_ prefix, YOLO_MODE included: the bare
// spelling other agent tooling sets is still read as the old name of that
// setting, but a flag writes the canonical one, so the variable it wrote is
// the one every reader consults first.
func TestEnvBackedFlags_EverySettingIsReachableFromTheCommandLine(t *testing.T) {
	want := map[string]string{
		"log-level":                 "GITLAB_MCP_LOG_LEVEL",
		"client-compat":             "GITLAB_MCP_CLIENT_COMPAT",
		"upload-max-file-size":      "GITLAB_MCP_UPLOAD_MAX_FILE_SIZE",
		"yolo-mode":                 "GITLAB_MCP_YOLO_MODE",
		"description-substitutions": "GITLAB_MCP_DESCRIPTION_SUBSTITUTIONS",
	}

	got := make(map[string]string, len(envBackedFlags))
	for _, entry := range envBackedFlags {
		got[entry.flagName] = entry.envName
	}

	for flagName, envName := range want {
		t.Run(flagName, func(t *testing.T) {
			if got[flagName] != envName {
				t.Errorf("-%s maps to %q, want %q", flagName, got[flagName], envName)
			}
		})
	}
	if len(got) != len(want) {
		t.Errorf("%d env-backed flags declared, expected %d; a new one needs a case here", len(got), len(want))
	}
}

// TestEnvBackedFlags_NoTokenFlag pins a deliberate absence.
//
// A --gitlab-token flag would put a credential on the command line, where ps
// shows it to every user on the machine, process accounting captures it, and
// shell history keeps it. The environment is not perfect either, but it is not
// world-readable on any platform this server supports. Adding the flag would
// make the insecure path the convenient one, so its absence is a decision and
// needs an assertion, or somebody will add it as an obvious omission.
func TestEnvBackedFlags_NoTokenFlag(t *testing.T) {
	for _, entry := range envBackedFlags {
		if entry.envName == "GITLAB_TOKEN" {
			t.Fatal("a flag was added for GITLAB_TOKEN; a token on a command line is visible through ps and in shell history")
		}
	}

	withFreshFlagSet(t)
	registerEnvBackedFlags()
	if flag.CommandLine.Lookup("gitlab-token") != nil {
		t.Error("-gitlab-token is registered")
	}
}
