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
	"fmt"
	"strings"
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

// scenarioPlan is one point of the matrix before it is measured.
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
}

// describe renders a plan for the progress line.
func (p scenarioPlan) describe() string {
	telemetry := "telemetry off"
	if p.Telemetry {
		telemetry = "telemetry on"
	}
	return fmt.Sprintf("%s, %s surface, %d clients, %d parallel, %s",
		p.Transport, p.Surface, p.Clients, p.Parallel, telemetry)
}

// publishedMatrix is what the committed record and every chart are drawn from.
//
// The individual surface runs with fewer parallel requests than the others, and
// that is not a hedge: one of its tools/list responses is three megabytes, so
// the same parallelism would measure how fast this machine can serialize
// rather than what the server costs.
func publishedMatrix(rounds int) []scenarioPlan {
	return []scenarioPlan{
		{ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "stdio-meta", Transport: transportStdio, Surface: surfaceMeta, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "stdio-individual", Transport: transportStdio, Surface: surfaceIndividual, Clients: 4, Parallel: 2, Rounds: rounds},
		{ID: "stdio-dynamic-telemetry", Transport: transportStdio, Surface: surfaceDynamic, Telemetry: true, Clients: 4, Parallel: 4, Rounds: rounds},
		{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic, Clients: 8, Parallel: 4, Rounds: rounds},
		{ID: "http-meta", Transport: transportHTTP, Surface: surfaceMeta, Clients: 8, Parallel: 4, Rounds: rounds},
		{ID: "http-individual", Transport: transportHTTP, Surface: surfaceIndividual, Clients: 8, Parallel: 2, Rounds: rounds},
		{ID: "http-dynamic-telemetry", Transport: transportHTTP, Surface: surfaceDynamic, Telemetry: true, Clients: 8, Parallel: 4, Rounds: rounds},
	}
}

// quickMatrix is the smoke run: the two transports and the two surfaces that
// bracket the range, one round, two clients. It exists so a change to this
// command can be verified in a minute without publishing anything.
func quickMatrix() []scenarioPlan {
	return []scenarioPlan{
		{ID: "stdio-dynamic", Transport: transportStdio, Surface: surfaceDynamic, Clients: 2, Parallel: 2, Rounds: 1},
		{ID: "http-dynamic", Transport: transportHTTP, Surface: surfaceDynamic, Clients: 2, Parallel: 2, Rounds: 1},
	}
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
