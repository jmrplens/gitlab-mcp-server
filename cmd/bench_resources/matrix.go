// matrix.go declares what gets measured.
//
// The axes are the ones that actually move the numbers: the transport, because
// stdio gives every client its own process and HTTP gives every credential its
// own pooled catalog; the tool surface, because one registers two tools and
// another about a thousand; telemetry, because exporting is work the server
// would not otherwise do; and concurrency, in both of its meanings, since
// clients and in-flight requests cost different things.

package main

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Transports and surfaces, named once so a typo in the matrix is a compile
// error rather than a scenario that silently measures the default.
const (
	transportStdio = "stdio"
	transportHTTP  = "http"

	surfaceDynamic    = "dynamic"
	surfaceMeta       = "meta"
	surfaceIndividual = "individual"
)

// scenarioPlan is one point of the matrix before it is measured, or one
// concurrency series when Steps is set.
type scenarioPlan struct {
	ID        string
	Transport string
	Surface   string
	Telemetry bool
	// Clients is credentials on HTTP and processes on stdio.
	Clients int
	// Parallel is requests in flight per client.
	Parallel int
	Rounds   int
	// Steps, when set, makes the plan a concurrency series: the credential
	// counts to step through, ascending, on one HTTP process. A series has no
	// Clients and no Rounds; it has a steady phase of StepDuration per step.
	Steps        []int
	StepDuration time.Duration
}

// isSeries reports whether the plan steps through credential counts rather
// than measuring one point.
func (p scenarioPlan) isSeries() bool { return len(p.Steps) > 0 }

// describe renders a plan for the progress line.
func (p scenarioPlan) describe() string {
	telemetry := "telemetry off"
	if p.Telemetry {
		telemetry = "telemetry on"
	}
	if p.isSeries() {
		return fmt.Sprintf("%s, %s surface, %d parallel, %d credential counts from %d to %d, %s per step, %s",
			p.Transport, p.Surface, p.Parallel, len(p.Steps), p.Steps[0], p.Steps[len(p.Steps)-1],
			p.StepDuration, telemetry)
	}
	return fmt.Sprintf("%s, %s surface, %d clients, %d parallel, %s",
		p.Transport, p.Surface, p.Clients, p.Parallel, telemetry)
}

// matrixSettings are the knobs a matrix is built with: the rounds of the
// point scenarios, and the credential counts and steady-phase length of the
// series.
type matrixSettings struct {
	rounds       int
	steps        []int
	stepDuration time.Duration
}

// defaultSeriesSteps are the credential counts the published series steps
// through: roughly logarithmic, so the slope is measured over three decades
// with ten steps rather than a thousand. The memory budget decides where the
// list actually ends on a given host.
var defaultSeriesSteps = []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000}

// defaultStepDuration is the steady phase of a published step. Long enough
// for a CPU profile to sample something and for the percentiles to have
// thousands of calls behind them; short enough that ten steps on three
// surfaces stay inside the hour after which the pool rebuilds a credential.
const defaultStepDuration = 10 * time.Second

// quickSeriesSteps and quickStepDuration are the smoke series: three counts
// and two seconds each, so a change to the series can be verified in the
// time the smoke matrix already takes.
var quickSeriesSteps = []int{1, 2, 5}

const quickStepDuration = 2 * time.Second

// seriesPlan builds the concurrency series for one surface. Parallelism
// follows the point scenarios' rule: the individual surface runs fewer
// requests in flight because its tools/list is three megabytes.
func seriesPlan(surface string, parallel int, settings matrixSettings) scenarioPlan {
	return scenarioPlan{
		ID: "http-" + surface + "-series", Transport: transportHTTP, Surface: surface,
		Parallel: parallel, Steps: settings.steps, StepDuration: settings.stepDuration,
	}
}

// publishedMatrix is what the committed record and every chart are drawn from.
//
// The individual surface runs with fewer parallel requests than the others, and
// that is not a hedge: one of its tools/list responses is three megabytes, so
// the same parallelism would measure how fast this machine can serialize
// rather than what the server costs. The three series come last so the point
// figures are on disk before the hour the series can take begins.
func publishedMatrix(settings matrixSettings) []scenarioPlan {
	rounds := settings.rounds
	return []scenarioPlan{
		{ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "stdio-meta", Transport: transportStdio, Surface: surfaceMeta, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "stdio-individual", Transport: transportStdio, Surface: surfaceIndividual, Clients: 4, Parallel: 2, Rounds: rounds},
		{ID: "stdio-dynamic-telemetry", Transport: transportStdio, Surface: surfaceDynamic, Telemetry: true, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic, Clients: 8, Parallel: 4, Rounds: rounds},
		{ID: "http-meta", Transport: transportHTTP, Surface: surfaceMeta, Clients: 8, Parallel: 4, Rounds: rounds},
		{ID: "http-individual", Transport: transportHTTP, Surface: surfaceIndividual, Clients: 8, Parallel: 2, Rounds: rounds},
		{ID: "http-dynamic-telemetry", Transport: transportHTTP, Surface: surfaceDynamic, Telemetry: true, Clients: 8, Parallel: 4, Rounds: rounds},
		seriesPlan(surfaceDynamic, 4, settings),
		seriesPlan(surfaceMeta, 4, settings),
		seriesPlan(surfaceIndividual, 2, settings),
	}
}

// quickMatrix is the smoke run: the two transports and the two surfaces that
// bracket the range, one round, two clients, and the dynamic series over the
// given counts. It exists so a change to this command can be verified in a
// minute without publishing anything.
func quickMatrix(settings matrixSettings) []scenarioPlan {
	return []scenarioPlan{
		{ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic, Clients: 2, Parallel: 2, Rounds: 1},
		{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic, Clients: 2, Parallel: 2, Rounds: 1},
		seriesPlan(surfaceDynamic, 2, settings),
	}
}

// parseSteps reads a -clients list: positive integers, comma-separated,
// strictly ascending. Ascending is required rather than sorted for the
// caller, because a series admits credentials and never releases them, so a
// list that went down would be measuring the previous step's pool again
// under a smaller name.
func parseSteps(raw string) ([]int, error) {
	var steps []int
	for field := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			continue
		}
		value, err := strconv.Atoi(trimmed)
		if err != nil || value <= 0 {
			return nil, fmt.Errorf("-clients: %q is not a positive credential count", trimmed)
		}
		if len(steps) > 0 && value <= steps[len(steps)-1] {
			return nil, fmt.Errorf("-clients: %d does not come after %d; the counts must ascend", value, steps[len(steps)-1])
		}
		steps = append(steps, value)
	}
	if len(steps) == 0 {
		return nil, errors.New("-clients: no credential counts given")
	}
	return slices.Clip(steps), nil
}

// selectPlans keeps the scenarios named in a comma-separated filter, in matrix
// order. An empty filter keeps everything; a name that matches nothing is an
// error, because a typo that silently measures a smaller matrix would publish
// a page with holes in it.
func selectPlans(plans []scenarioPlan, filter string) ([]scenarioPlan, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return plans, nil
	}
	wanted := map[string]bool{}
	for name := range strings.SplitSeq(filter, ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			wanted[trimmed] = false
		}
	}
	var kept []scenarioPlan
	for _, plan := range plans {
		if _, ok := wanted[plan.ID]; ok {
			wanted[plan.ID] = true
			kept = append(kept, plan)
		}
	}
	for name, matched := range wanted {
		if !matched {
			return nil, fmt.Errorf("no scenario named %q", name)
		}
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("filter %q selected no scenarios", filter)
	}
	return kept, nil
}

// toolCall is the tools/call this benchmark makes on a given surface.
//
// Every one of them is answered without leaving the machine: the dynamic
// surface searches its own catalog, and the server status tools reach the
// stand-in instance on loopback. A call that queried real GitLab data would
// publish that instance's latency as if it were the server's.
type toolCall struct {
	Name string
	Args map[string]any
	// Detail is what the published table prints beside the method, since the
	// tool differs per surface and a percentile with no call behind it is not
	// comparable to anything.
	Detail string
}

// callFor returns the tools/call for a surface.
func callFor(surface string) (toolCall, error) {
	switch surface {
	case surfaceDynamic:
		return toolCall{
			Name:   "gitlab_find_action",
			Args:   map[string]any{"query": "list issues"},
			Detail: "gitlab_find_action",
		}, nil
	case surfaceMeta:
		return toolCall{
			Name:   "gitlab_server",
			Args:   map[string]any{"action": "status"},
			Detail: "gitlab_server (status)",
		}, nil
	case surfaceIndividual:
		return toolCall{
			Name:   "gitlab_server_status",
			Args:   map[string]any{},
			Detail: "gitlab_server_status",
		}, nil
	default:
		return toolCall{}, fmt.Errorf("unknown surface %q", surface)
	}
}
