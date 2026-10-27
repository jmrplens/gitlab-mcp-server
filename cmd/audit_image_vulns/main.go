// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"
)

// toolName is how the report names itself.
const toolName = "audit_image_vulns"

// defaultPlatforms are the platforms the release pushes, which a run expects
// one report for each of unless -platforms names others. A test holds them to
// the multi-platform build in release.yml.
const defaultPlatforms = "linux/amd64,linux/arm64"

// auditConfig is one configured run: the reports to judge, the platforms they
// must cover, the database they were judged against, the declarations they
// may rely on, and the moment of the run.
type auditConfig struct {
	declared           map[string]declaration
	now                time.Time
	db                 string
	reports, platforms []string
}

// exitProcess is [os.Exit] behind a seam, so the code [runMain] decided is a
// value a test can read rather than the end of the test binary.
var exitProcess = os.Exit

func main() {
	exitProcess(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// runMain parses the command line and runs the audit it describes, returning
// the process exit code: 2 for arguments it cannot parse, and otherwise what
// [run] decides.
func runMain(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet(toolName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	platforms := flags.String("platforms", defaultPlatforms, "comma-separated os/arch platforms the reports must cover, one report each")
	db := flags.String("db", "", "the output of trivy version --format json for the cache the scans used")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		fmt.Fprintf(stderr, "%s: name the Trivy report of each platform as arguments\n", toolName)
		return 2
	}
	if *db == "" {
		fmt.Fprintf(stderr, "%s: -db names the output of trivy version --format json, so the verdict can say what it was judged against\n", toolName)
		return 2
	}
	want, ok := parsePlatforms(*platforms)
	if !ok {
		fmt.Fprintf(stderr, "%s: -platforms %q does not name each platform once as os/arch, separated by commas\n", toolName, *platforms)
		return 2
	}
	return run(auditConfig{reports: flags.Args(), platforms: want, db: *db, declared: acceptedFindings, now: time.Now()}, stdout, stderr)
}

// parsePlatforms reads a comma-separated list of os/arch platforms, each
// named once, reporting false for any other list.
func parsePlatforms(list string) ([]string, bool) {
	platforms := strings.Split(list, ",")
	for i, platform := range platforms {
		system, arch, _ := strings.Cut(platform, "/")
		if system == "" || arch == "" || strings.ContainsAny(platform, " \t") || slices.Contains(platforms[:i], platform) {
			return nil, false
		}
	}
	return platforms, true
}

// run judges and reports, returning the process exit code.
//
// Exit 1 covers two different things, told apart by which stream spoke: a run
// that could not be judged says so on stderr, and a run that found something
// says it on stdout. Both fail the gate, because an image that could not be
// checked is not one that passed.
func run(cfg auditConfig, stdout, stderr io.Writer) int {
	v, err := audit(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", toolName, err)
		return 1
	}
	v.write(stdout)
	if !v.ok() {
		return 1
	}
	return 0
}

// audit reads the database and every report, holds the reports to the
// platforms, and judges the findings.
func audit(cfg auditConfig) (verdict, error) {
	db, err := readDatabase(cfg.db, cfg.now)
	if err != nil {
		return verdict{}, err
	}
	scans := make([]scan, 0, len(cfg.reports))
	for _, path := range cfg.reports {
		s, readErr := readScan(path)
		if readErr != nil {
			return verdict{}, readErr
		}
		scans = append(scans, s)
	}
	if platformErr := checkPlatforms(scans, cfg.platforms); platformErr != nil {
		return verdict{}, platformErr
	}
	return judge(db, scans, cfg.declared, cfg.now), nil
}
