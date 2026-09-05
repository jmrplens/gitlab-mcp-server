// Package termio centralizes terminal and log-file output helpers used by the
// eval_mcp_surfaces CLI so commands can write progress to a tee'd destination
// (stdout plus an optional log file) with a consistent format.
//
// The package owns the process-wide [Output] sink, the [Configure] hook that
// installs the sink at startup, and the [Printf]/[Print]/[LogPrintf] helpers
// that route through it. Tests can swap the sink with [SetOutputForTest] to
// observe output without touching the real filesystem.
package termio
