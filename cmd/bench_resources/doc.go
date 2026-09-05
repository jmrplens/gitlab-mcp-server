// Command bench_resources measures what this server costs to run, and draws
// the charts the documentation publishes.
//
// Everything else measured in this repository is about tokens and tool counts.
// None of it tells an operator how much memory to give a container, how long a
// client waits before the first tool call answers, or what a second credential
// adds to a shared deployment. This command answers those, from the real
// binary, on both transports, and writes one record every downstream artifact
// is rendered from.
//
// # What it needs
//
// Nothing but a Go toolchain. GitLab is stood in for by an in-process HTTP
// server on loopback, and the OTLP collector by another, so a run is offline
// and a second machine measures the same thing rather than its own network.
// The tool surface is passed to the server explicitly and never read from the
// environment, for the reason CLAUDE.md gives about generators: a developer
// machine exporting GITLAB_MCP_TOOL_SURFACE would otherwise publish different
// numbers than CI.
//
// # Usage
//
//	go run ./cmd/bench_resources/                 # measure, then render
//	go run ./cmd/bench_resources/ -render         # redraw from the record
//	go run ./cmd/bench_resources/ -check          # is the drawing current?
//	go run ./cmd/bench_resources/ -quick -json /tmp/x.json
package main
