package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/config"
	gitlabclient "github.com/jmrplens/gitlab-mcp-server/v3/internal/gitlab"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/telemetry"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/testutil"
	"github.com/jmrplens/gitlab-mcp-server/v3/internal/toolutil"
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
	t.Setenv("GITLAB_MCP_LOG_LEVEL", "debug")

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
	t.Setenv("GITLAB_MCP_LOG_LEVEL", "debug")
	t.Setenv("GITLAB_MCP_CLIENT_COMPAT", "off")
	t.Setenv("GITLAB_MCP_UPLOAD_MAX_FILE_SIZE", "1048576")
	t.Setenv("GITLAB_MCP_YOLO_MODE", "true")

	registerEnvBackedFlags()
	if err := flag.CommandLine.Parse(nil); err != nil {
		t.Fatalf("parsing: %v", err)
	}
	applyEnvBackedFlags()

	for name, want := range map[string]string{
		"GITLAB_MCP_LOG_LEVEL":            "debug",
		"GITLAB_MCP_CLIENT_COMPAT":        "off",
		"GITLAB_MCP_UPLOAD_MAX_FILE_SIZE": "1048576",
		"GITLAB_MCP_YOLO_MODE":            "true",
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
		"pprof-addr":                "GITLAB_MCP_PPROF_ADDR",
		"allow-private-instances":   "GITLAB_MCP_ALLOW_PRIVATE_INSTANCES",
		"max-listen-streams":        "GITLAB_MCP_MAX_LISTEN_STREAMS",
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

// TestEnvBackedFlags_UploadLimitUsage_NamesTheSuffixes verifies that the
// usage of -upload-max-file-size says what the value is written as. It used to
// call it a size in bytes, which a reader takes to mean a plain number, while
// the parser takes a KB, MB or GB suffix and the documented default is 2GB.
func TestEnvBackedFlags_UploadLimitUsage_NamesTheSuffixes(t *testing.T) {
	var usage string
	for _, entry := range envBackedFlags {
		if entry.flagName == "upload-max-file-size" {
			usage = entry.usage
		}
	}
	if usage == "" {
		t.Fatal("-upload-max-file-size is not an env-backed flag with a usage")
	}
	if strings.Contains(usage, "in bytes") {
		t.Errorf("usage %q calls the limit a size in bytes, but it takes a KB, MB or GB suffix", usage)
	}
	for _, want := range []string{
		"KB, MB or GB suffix",
		fmt.Sprintf("default %dGB, at most %dGB", config.DefaultMaxFileSize>>30, config.MaxFileSize>>30),
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(usage, want) {
				t.Errorf("usage %q does not carry %q", usage, want)
			}
		})
	}
}

// TestEnvBackedFlags_ListenStreamsUsage_NamesBothCeilings verifies that the
// usage of -max-listen-streams states the default the variable has and the
// process-wide ceiling that stays when it is 0, held to the constants the
// middleware enforces (register rows HLD-001 and HLD-002), since the usage
// spells both as literals.
func TestEnvBackedFlags_ListenStreamsUsage_NamesBothCeilings(t *testing.T) {
	var usage string
	for _, entry := range envBackedFlags {
		if entry.flagName == "max-listen-streams" {
			usage = entry.usage
		}
	}
	for _, want := range []string{
		fmt.Sprintf("(default %d; 0 removes this ceiling", maxListenStreamsPerServer),
		fmt.Sprintf("the process-wide one of %d stays", maxListenStreamsPerProcess),
		"warns and keeps the default",
	} {
		t.Run(want, func(t *testing.T) {
			if !strings.Contains(usage, want) {
				t.Errorf("usage %q does not carry %q", usage, want)
			}
		})
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

// settingOutcome is what a reader made of the value it was given.
type settingOutcome int

const (
	// outcomeApplied is the value taking effect.
	outcomeApplied settingOutcome = iota + 1
	// outcomeRefused is the value refused: an error from its reader, or from
	// the flag package, which ends the start before anything reads it.
	outcomeRefused
	// outcomeKeptDefault is the default kept, with a warning.
	outcomeKeptDefault
	// outcomeSkipped is an entry that does not resolve skipped with a warning,
	// and the rest of the list kept.
	outcomeSkipped
	// outcomeReadAsOff is the value read as off.
	outcomeReadAsOff
	// outcomeUnread is the value taking no effect at all, which is what a
	// reader that does not read the variable makes of every value.
	outcomeUnread
)

// outcomeOf is what a value that does not parse does under each policy the
// register states (tenancy.Malformed).
var outcomeOf = map[tenancy.Malformed]settingOutcome{
	tenancy.RefuseStartup:   outcomeRefused,
	tenancy.WarnKeepDefault: outcomeKeptDefault,
	tenancy.SkipEntry:       outcomeSkipped,
	tenancy.ReadsAsOff:      outcomeReadAsOff,
}

// settingCase is one setting issue 957 moved onto the configuration
// package's list, with a value its reader cannot use and one it can.
type settingCase struct {
	// setting is the name config.Getenv reads, without the prefix.
	setting string
	// row is the register row that names the setting, "" for one no row
	// names because it is not a limit.
	row string
	// flag is the flag that sets it, "" for one with no flag.
	flag string
	// register declares flag on a fresh flag set, for a flag that is not one
	// of envBackedFlags; nil for one that is, or for no flag.
	register func(t *testing.T)
	// malformed is a value the reader cannot use, "" for a setting every
	// value of which means something, and wellFormed one it can.
	malformed, wellFormed string
	// values makes the two values in place of the fields, for a setting whose
	// values name something the test has to create first; nil for the rest.
	values func(t *testing.T) (malformed, wellFormed string)
	// policy is what malformed does, which is its row's when a row names it.
	policy tenancy.Malformed
	// flagPolicy is what malformed does as the flag's value, when that is not
	// policy: a flag the flag package parses refuses what it cannot parse.
	flagPolicy tenancy.Malformed
	// observe reads the setting the way the server does and says what it
	// made of value.
	observe func(t *testing.T, value string) settingOutcome
}

// numberOutcome classifies a numeric reader's answer: the value configured,
// or the default with the warning that it could not be parsed.
func numberOutcome(got int, value string, def int, warned bool) settingOutcome {
	if got != def && strconv.Itoa(got) == strings.TrimSpace(value) {
		return outcomeApplied
	}
	if got == def && warned {
		return outcomeKeptDefault
	}
	return outcomeUnread
}

// settingsMovedByIssue957 are the settings issue 957 moved onto the
// configuration package's list, each with how the server reads it.
func settingsMovedByIssue957() []settingCase {
	return slices.Concat(ceilingSettings(), telemetrySettings(), guardSettings())
}

// ceilingSettings are the two ceilings issue 957 moved, which warn and keep
// their default (HLD-001, RQB-004).
func ceilingSettings() []settingCase {
	return []settingCase{
		{
			setting: "MAX_LISTEN_STREAMS", row: "HLD-001", flag: "max-listen-streams",
			malformed: "many", wellFormed: "8", policy: tenancy.WarnKeepDefault,
			observe: func(t *testing.T, value string) settingOutcome {
				t.Helper()
				warned := captureLogMessages(t)
				got := listenLimitsFromEnv().perCredential
				return numberOutcome(got, value, maxListenStreamsPerServer, warned("listen stream limit could not be parsed"))
			},
		},
		{
			setting: "STDIO_MAX_LINE_BYTES", row: "RQB-004",
			malformed: "4MiB", wellFormed: "65536", policy: tenancy.WarnKeepDefault,
			observe: func(t *testing.T, value string) settingOutcome {
				t.Helper()
				warned := captureLogMessages(t)
				got := stdioLimitsFromEnv().maxLineBytes
				return numberOutcome(got, value, defaultMaxStdioLineBytes, warned("stdio line limit could not be parsed"))
			},
		},
	}
}

// telemetrySettings are telemetry's switch, its identity settings
// (IDN-009) and its tool-name dimension (IDN-014).
func telemetrySettings() []settingCase {
	return []settingCase{
		{
			// No row names the switch, which is not a limit. Its variable is
			// parsed by the OpenTelemetry boolean grammar, under which a value
			// that is not true or false is a warning and false, and its flag
			// by Go's, which refuses one.
			setting: "TELEMETRY", flag: "telemetry",
			register: func(t *testing.T) {
				t.Helper()
				previous := telemetryFlag
				t.Cleanup(func() { telemetryFlag = previous })
				telemetryFlag = flag.Bool("telemetry", false, "")
			},
			malformed: "ture", wellFormed: "true", policy: tenancy.ReadsAsOff, flagPolicy: tenancy.RefuseStartup,
			observe: func(t *testing.T, _ string) settingOutcome {
				t.Helper()
				warned := captureLogMessages(t)
				if telemetryEnabled() {
					return outcomeApplied
				}
				if warned("ignoring unrecognized boolean environment variable") {
					return outcomeReadAsOff
				}
				return outcomeUnread
			},
		},
		{
			// The reader's error is what this row observes, and it is not
			// yet the refused start IDN-009 states: the server logs it and
			// records nothing about callers, which issue 1170 and the last
			// layer of issue 957 turn into a refusal. The rotation's row
			// below is the same.
			setting: "TELEMETRY_IDENTITY", row: "IDN-009", flag: "telemetry-identity",
			register: func(t *testing.T) {
				t.Helper()
				previous := telemetryIdentityFlag
				t.Cleanup(func() { telemetryIdentityFlag = previous })
				telemetryIdentityFlag = flag.String("telemetry-identity", string(telemetry.DefaultIdentityPolicy), "")
			},
			malformed: "pseudonimous", wellFormed: string(telemetry.IdentityFull), policy: tenancy.RefuseStartup,
			observe: func(t *testing.T, value string) settingOutcome {
				t.Helper()
				policy, err := telemetryIdentityPolicy()
				if err != nil {
					return outcomeRefused
				}
				if string(policy) == value {
					return outcomeApplied
				}
				return outcomeUnread
			},
		},
		{
			// A key is any text, so no value of it fails to parse; the row's
			// policy is its two siblings'. It has no flag on purpose, since
			// process arguments are readable through /proc.
			setting: "TELEMETRY_IDENTITY_KEY", row: "IDN-009",
			wellFormed: "a deployment-wide secret", policy: tenancy.RefuseStartup,
			observe: func(t *testing.T, value string) settingOutcome {
				t.Helper()
				if telemetryIdentityKey() == value {
					return outcomeApplied
				}
				return outcomeUnread
			},
		},
		{
			// As for the policy above, the reader's error is observed, and
			// the server does not refuse to start over it yet (issue 1170).
			setting: "TELEMETRY_IDENTITY_ROTATION", row: "IDN-009", flag: "telemetry-identity-rotation",
			register: func(t *testing.T) {
				t.Helper()
				previous := telemetryIdentityRotationFlag
				t.Cleanup(func() { telemetryIdentityRotationFlag = previous })
				telemetryIdentityRotationFlag = flag.String("telemetry-identity-rotation", "", "")
			},
			malformed: "24", wellFormed: "6h", policy: tenancy.RefuseStartup,
			observe: func(t *testing.T, value string) settingOutcome {
				t.Helper()
				rotation, err := telemetryIdentityRotation()
				if err != nil {
					return outcomeRefused
				}
				if want, parseErr := time.ParseDuration(value); parseErr == nil && rotation == want && rotation != telemetry.DefaultKeyRotation {
					return outcomeApplied
				}
				return outcomeUnread
			},
		},
		{
			setting: "TELEMETRY_TOOL_NAME", row: "IDN-014", flag: "telemetry-tool-name",
			register: func(t *testing.T) {
				t.Helper()
				previous := telemetryToolNameFlag
				t.Cleanup(func() { telemetryToolNameFlag = previous })
				telemetryToolNameFlag = flag.String("telemetry-tool-name", "", "")
			},
			// Off drops the dimensions on the dynamic surface, where auto, the
			// default, keeps them.
			malformed: "sometimes", wellFormed: string(telemetry.ToolNameOff), policy: tenancy.WarnKeepDefault,
			observe: func(t *testing.T, _ string) settingOutcome {
				t.Helper()
				warned := captureLogMessages(t)
				dropped := dropToolNameFromMetrics(config.ToolSurfaceDynamic)
				if dropped {
					return outcomeApplied
				}
				if warned("telemetry tool-name policy is unusable; using auto") {
					return outcomeKeptDefault
				}
				return outcomeUnread
			},
		},
	}
}

// guardSettings are the settings that widen what a guard lets through: the
// private-instance opt-out (DST-003) and the three local allow-lists
// (AUT-006).
func guardSettings() []settingCase {
	return []settingCase{
		{
			// The opt-out turns a guard off, so a value it does not parse
			// reads as off and the guard stays on. A caller-named instance on
			// a private address is what the guard refuses.
			setting: "ALLOW_PRIVATE_INSTANCES", row: "DST-003", flag: "allow-private-instances",
			malformed: "ture", wellFormed: "true", policy: tenancy.ReadsAsOff,
			observe: func(t *testing.T, _ string) settingOutcome {
				t.Helper()
				err := gitlabclient.CheckCallerNamedInstance("https://10.1.2.3")
				if err == nil {
					return outcomeApplied
				}
				if errors.Is(err, gitlabclient.ErrDestinationRefused) {
					return outcomeReadAsOff
				}
				t.Errorf("CheckCallerNamedInstance() = %v, want nil or a refusal of the destination", err)
				return outcomeUnread
			},
		},
		allowListCase("ALLOWED_UPLOAD_DIRS", func(t *testing.T, dir string) error {
			t.Helper()
			path := filepath.Join(dir, "upload.txt")
			if err := os.WriteFile(path, []byte("payload"), 0o600); err != nil {
				t.Fatalf("WriteFile(%s): %v", path, err)
			}
			_, err := toolutil.CanonicalLocalFilePath(path)
			return err
		}),
		allowListCase("ALLOWED_DOWNLOAD_DIRS", func(t *testing.T, dir string) error {
			t.Helper()
			_, err := toolutil.WriteDownloadOutputFile(filepath.Join(dir, "download.bin"), func(w io.Writer) error {
				_, writeErr := io.WriteString(w, "payload")
				return writeErr
			})
			return err
		}),
		allowListCase("ALLOWED_IMPORT_DIRS", func(t *testing.T, dir string) error {
			t.Helper()
			path := filepath.Join(dir, "project-export.tar.gz")
			if err := os.WriteFile(path, []byte("archive"), 0o600); err != nil {
				t.Fatalf("WriteFile(%s): %v", path, err)
			}
			_, err := toolutil.CanonicalImportArchivePath(path)
			return err
		}),
	}
}

// allowListCase is the row of one of the three local allow-lists (register
// row AUT-006), which only stdio reads and which have no flag. admit asks the
// tool path the list guards to accept something in dir, the last directory
// the value names.
//
// The working directory and the temporary directory are always allowed, so
// [allowListValues] moves both away from the directory the value names, and
// the local filesystem is opened for the test, as a stdio process opens it.
// The value that does not parse is a list whose first directory does not
// exist: that entry is skipped with a warning and the second one is honored.
func allowListCase(setting string, admit func(t *testing.T, dir string) error) settingCase {
	return settingCase{
		setting: setting, row: "AUT-006", policy: tenancy.SkipEntry, values: allowListValues,
		observe: func(t *testing.T, value string) settingOutcome {
			t.Helper()
			previous := toolutil.LocalFilesystemAccessAllowed()
			toolutil.SetLocalFilesystemAccess(true)
			t.Cleanup(func() { toolutil.SetLocalFilesystemAccess(previous) })
			warned := captureLogMessages(t)
			entries := filepath.SplitList(value)
			if err := admit(t, entries[len(entries)-1]); err != nil {
				return outcomeUnread
			}
			if len(entries) > 1 && warned("skipping invalid allowlist directory") {
				return outcomeSkipped
			}
			return outcomeApplied
		},
	}
}

// allowListValues confines the roots every allow-list keeps to a directory
// of the test's own and returns a list naming a missing directory and then a
// real one, and the real one alone.
func allowListValues(t *testing.T) (malformed, wellFormed string) {
	t.Helper()
	root := t.TempDir()
	work, temporary, configured := filepath.Join(root, "work"), filepath.Join(root, "tmp"), filepath.Join(root, "configured")
	for _, dir := range []string{work, temporary, configured} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	testutil.IsolateTempDir(t, temporary)
	t.Chdir(work)
	missing := filepath.Join(root, "missing")
	return missing + string(os.PathListSeparator) + configured, configured
}

// setFlag passes value to tc's flag on a fresh flag set, the way an operator
// types it, and reports the flag package's refusal. A flag of envBackedFlags
// then writes its variable, as main does before anything reads one.
func setFlag(t *testing.T, tc settingCase, value string) error {
	t.Helper()
	withFreshFlagSet(t)
	if tc.register != nil {
		tc.register(t)
	} else {
		registerEnvBackedFlags()
	}
	if err := flag.CommandLine.Parse([]string{"-" + tc.flag + "=" + value}); err != nil {
		return err
	}
	applyEnvBackedFlags()
	return nil
}

// TestSettingsMovedByIssue957_AValueOfEach_GetsItsPolicyAtTheEnvironmentAndTheFlag
// is the table issue 957 measures its fix with: one row per setting it moved
// onto the configuration package's list, each fed a value its reader cannot
// use and one it can, through the variable and, where it has one, through its
// flag. The value that does not parse must do what the row's malformed-value
// policy says, and the one that does must take effect, which is also what
// shows the reader reads that variable: a reader left on another name makes
// nothing of either, and a reader that never reads a value cannot be told
// apart by the malformed one alone where the policy is to read it as off.
//
// At the flag the variable is set beside it to the other value, so the flag
// is seen to win over the environment in both directions, the precedence HTTP
// mode documents and both transports keep for these flags. A setting a row
// names is held to that row too: the row names its variable and its flag, and
// its malformed-value policy is the one the reader keeps.
func TestSettingsMovedByIssue957_AValueOfEach_GetsItsPolicyAtTheEnvironmentAndTheFlag(t *testing.T) {
	for _, tc := range settingsMovedByIssue957() {
		t.Run(tc.setting, func(t *testing.T) {
			if tc.values != nil {
				tc.malformed, tc.wellFormed = tc.values(t)
			}
			if !slices.Contains(config.PrefixedEnvNames(), tc.setting) {
				t.Errorf("%s is not on the configuration package's list", tc.setting)
			}
			tc.checkRow(t)
			tc.checkEnvironment(t)
			tc.checkFlag(t)
		})
	}
}

// checkRow holds the register row that names the setting to it: the row
// names its variable, and its flag exactly when it has one, and its
// malformed-value policy is the one the reader keeps.
func (tc settingCase) checkRow(t *testing.T) {
	t.Helper()
	if tc.row == "" {
		return
	}
	d, ok := tenancy.Lookup(tc.row)
	if !ok {
		t.Fatalf("%s names no register row", tc.row)
	}
	env := config.EnvPrefix + tc.setting
	if !slices.Contains(d.Envs, env) || d.Malformed != tc.policy {
		t.Errorf("%s: envs %v, malformed policy %d; want %s among them and policy %d",
			tc.row, d.Envs, d.Malformed, env, tc.policy)
	}
	if hasFlag := slices.Contains(d.Flags, "--"+tc.flag); hasFlag != (tc.flag != "") {
		t.Errorf("%s: flags %v, want --%s named exactly when the setting has a flag", tc.row, d.Flags, tc.flag)
	}
}

// checkEnvironment sets the variable to each value and asks the reader.
func (tc settingCase) checkEnvironment(t *testing.T) {
	t.Helper()
	env := config.EnvPrefix + tc.setting
	for _, layer := range []struct {
		name, value string
		want        settingOutcome
	}{
		{"the environment, well formed", tc.wellFormed, outcomeApplied},
		{"the environment, malformed", tc.malformed, outcomeOf[tc.policy]},
	} {
		if layer.value == "" {
			continue
		}
		t.Run(layer.name, func(t *testing.T) {
			t.Setenv(env, layer.value)
			if got := tc.observe(t, layer.value); got != layer.want {
				t.Errorf("%s=%q: outcome %d, want %d", env, layer.value, got, layer.want)
			}
		})
	}
}

// checkFlag passes each value to the flag with the variable set to the other
// one, and asks the reader, for a setting that has a flag.
func (tc settingCase) checkFlag(t *testing.T) {
	t.Helper()
	if tc.flag == "" {
		return
	}
	env := config.EnvPrefix + tc.setting
	malformedWant := outcomeOf[tc.policy]
	if tc.flagPolicy != tenancy.MalformedNone {
		malformedWant = outcomeOf[tc.flagPolicy]
	}
	for _, layer := range []struct {
		name, value, beside string
		want                settingOutcome
	}{
		{"the flag, well formed over a malformed variable", tc.wellFormed, tc.malformed, outcomeApplied},
		{"the flag, malformed over a well-formed variable", tc.malformed, tc.wellFormed, malformedWant},
	} {
		if layer.value == "" {
			continue
		}
		t.Run(layer.name, func(t *testing.T) {
			t.Setenv(env, layer.beside)
			if err := setFlag(t, tc, layer.value); err != nil {
				if layer.want != outcomeRefused {
					t.Errorf("-%s=%q refused by the flag package: %v, want outcome %d", tc.flag, layer.value, err, layer.want)
				}
				return
			}
			if got := tc.observe(t, layer.value); got != layer.want {
				t.Errorf("-%s=%q beside %s=%q: outcome %d, want %d", tc.flag, layer.value, env, layer.beside, got, layer.want)
			}
		})
	}
}
