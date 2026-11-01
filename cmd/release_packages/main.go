// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// toolName is how the usage and the findings name the command.
const toolName = "release_packages"

// usage is the command line of every subcommand.
const usage = `usage: release_packages mcpb VERSION [DIST]
       release_packages mcpb-manifest PLATFORM [MANIFEST]
       release_packages server-json-packages [SERVER_JSON]
`

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// runMain runs the subcommand its first argument names, from the working
// directory, and returns the process exit code: 2 for arguments it cannot
// use, and otherwise what the subcommand decides.
func runMain(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	return runIn(".", args, stdout, stderr, getenv)
}

// runIn is runMain with the directory every relative path is read from.
func runIn(root string, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	name, rest := args[0], args[1:]
	switch name {
	case "mcpb":
		if len(rest) < 1 || len(rest) > 2 {
			break
		}
		return newMcpbBuild(root, optional(rest, 1, "dist"), rest[0], stdout, stderr, getenv).run()
	case "mcpb-manifest":
		if len(rest) < 1 || len(rest) > 2 {
			break
		}
		return printPlatformManifest(root, rest[0], optional(rest, 1, manifestPath), stdout, stderr)
	case "server-json-packages":
		if len(rest) > 1 {
			break
		}
		return newServerJSONCheck(root, optional(rest, 0, "server.json"), stdout, stderr, getenv).run()
	case "-h", "-help", "--help", "help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "%s: unknown subcommand %q\n%s", toolName, name, usage)
		return 2
	}
	fmt.Fprintf(stderr, "%s: %s takes other arguments\n%s", toolName, name, usage)
	return 2
}

// localPath is a path named relative to root, on disk, or the path itself
// when it is absolute.
func localPath(root, rel string) string {
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

// optional returns args[i], or fallback when there are not that many.
func optional(args []string, i int, fallback string) string {
	if i < len(args) {
		return args[i]
	}
	return fallback
}

// printPlatformManifest prints the manifest the per-OS bundle for platform
// carries, derived from the manifest at path, as `make check-mcpb` validates
// it.
func printPlatformManifest(root, platform, path string, stdout, stderr io.Writer) int {
	data, err := os.ReadFile(localPath(root, path)) //#nosec G304,G703 -- the manifest the operator named, read and never written
	var manifest *jsonValue
	if err == nil {
		manifest, err = parseJSON(data)
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %s could not be read as JSON: %v\n", toolName, path, err)
		return 1
	}
	derived, err := derivePlatformManifest(manifest, platform)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	_, err = stdout.Write(derived.pretty())
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	return 0
}
