package main

import (
	"flag"
	"os"
)

// envBackedFlags are the settings whose only home used to be an environment
// variable, given a flag each so that one command can configure the whole
// server.
//
// # Why the flag writes the variable instead of being read directly
//
// Each of these is consumed somewhere that reads the process environment and
// nothing else: parseLogLevel takes os.Getenv("LOG_LEVEL"),
// clientcompat.Enabled reads CLIENT_COMPAT, toolutil.IsYOLOMode reads YOLO_MODE
// and AUTOPILOT, and the upload limit is read per request inside the tools that
// enforce it. Threading a flag value to each of those would mean four new
// parameters through four unrelated call paths, and it would create a second
// source of truth in every one of them: a reader that consults the flag and a
// reader that consults the variable, disagreeing the first time somebody adds a
// caller.
//
// So the flag sets the variable, once, before anything reads either. There
// stays exactly one reader per setting, the precedence rule is the same one
// this server already documents (an explicitly passed flag beats the
// environment beats the default), and nothing downstream needs to know a flag
// exists.
//
// # Why GITLAB_TOKEN is not in this list
//
// It is the one setting deliberately left without a flag, and that is a
// security position rather than an oversight. A token on a command line is
// visible to every user on the machine through ps, is captured by process
// accounting, and lands in shell history. The environment is not perfect
// either, but it is not world-readable on any platform this server supports.
// A --gitlab-token flag would make the insecure path the convenient one.
var envBackedFlags = []struct {
	// flagName is what the operator types.
	flagName string
	// envName is the variable the rest of the process actually reads.
	envName string
	// value holds the parsed flag, filled by registerEnvBackedFlags.
	value *string
	// usage is the help text.
	usage string
}{
	{
		flagName: "log-level",
		envName:  "LOG_LEVEL",
		usage:    "Logging verbosity: debug, info, warn or error",
	},
	{
		flagName: "client-compat",
		envName:  "CLIENT_COMPAT",
		usage:    "Per-client response compatibility: auto (default) or off",
	},
	{
		flagName: "upload-max-file-size",
		envName:  "UPLOAD_MAX_FILE_SIZE",
		usage:    "Maximum size in bytes for upload and file-read tools",
	},
	{
		flagName: "yolo-mode",
		envName:  "YOLO_MODE",
		usage:    "Skip the confirmation prompt on destructive actions: true or false",
	},
}

// registerEnvBackedFlags declares the flags. Call before flag.Parse.
func registerEnvBackedFlags() {
	for i := range envBackedFlags {
		entry := &envBackedFlags[i]
		entry.value = flag.String(entry.flagName, "", entry.usage)
	}
}

// applyEnvBackedFlags copies each explicitly passed flag into its environment
// variable. Call after flag.Parse and before anything reads configuration.
//
// Only flags that were actually passed are copied, which is what preserves the
// precedence: a variable already exported keeps its value unless the operator
// typed the flag as well. An unset flag writes nothing, so it cannot clear a
// variable by accident, which a naive implementation writing the empty string
// would do to every deployment that configures through the environment.
func applyEnvBackedFlags() {
	for _, entry := range envBackedFlags {
		if entry.value == nil || !isFlagPassed(entry.flagName) {
			continue
		}
		// The error is ignored for the same reason os.Setenv's error exists at
		// all: it can only fail on a name containing NUL or "=", and these
		// names are compile-time constants.
		_ = os.Setenv(entry.envName, *entry.value)
	}
}
