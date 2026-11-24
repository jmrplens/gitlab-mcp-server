// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package conditions

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/recipe"
)

// gateAll is the gate that holds every condition, and beyondPrefix opens the
// one that holds only the files a platform does not build.
const (
	gateAll      = "all"
	beyondPrefix = "beyond:"
)

// referencePattern is a platform a beyond: gate names.
var referencePattern = regexp.MustCompile(`^[a-z0-9]+/[a-z0-9]+$`)

// figures are the words gobco opens its figure with, in each mode it counts
// in.
var figures = map[string]string{
	"condition": "Condition coverage",
	"branch":    "Branch coverage",
}

// settings are what the environment asks of one run: the words gobco opens
// its figure with in the mode it counts in, the arguments it is run with, the
// gate as it was written, empty for none, and the platform a beyond: gate
// compares this build with, empty for any other.
type settings struct {
	figure    string
	args      []string
	gate      string
	reference string
}

// readSettings reads GOBCO_MODE, GOBCO_GATE and GOBCO_TEST_FLAGS.
//
// gobco hands every -test option to its `go test` after the package, where
// the go command still reads a build flag, which is how the tags reach it.
func readSettings(environ []string, tags string) (settings, error) {
	mode, _ := recipe.Getenv(environ, "GOBCO_MODE")
	if mode == "" {
		mode = "condition"
	}
	figure, known := figures[mode]
	if !known {
		return settings{}, fmt.Errorf("GOBCO_MODE=%s is neither condition nor branch", mode)
	}
	read := settings{figure: figure}
	if mode == "branch" {
		read.args = append(read.args, "-branch")
	}
	if tags != "" {
		read.args = append(read.args, "-test=-tags="+tags)
	}
	flags, _ := recipe.Getenv(environ, "GOBCO_TEST_FLAGS")
	for _, flag := range recipe.Words(flags) {
		read.args = append(read.args, "-test="+flag)
	}
	read.gate, _ = recipe.Getenv(environ, "GOBCO_GATE")
	if read.gate == "" || read.gate == gateAll {
		return read, nil
	}
	reference, beyond := strings.CutPrefix(read.gate, beyondPrefix)
	if !beyond || !referencePattern.MatchString(reference) {
		return settings{}, fmt.Errorf("GOBCO_GATE=%s is neither all nor beyond:GOOS/GOARCH", read.gate)
	}
	read.reference = reference
	return read, nil
}
