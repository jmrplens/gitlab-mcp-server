// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

// Command audit_surface_quality is the consolidated MCP tool surface quality
// audit. It combines the metadata-quality checks (naming, annotations, schema
// shape — formerly audit_tools) and the output-quality checks (OutputSchema,
// "Returns:"/"See also:", Title — formerly audit_output) behind a single
// -view flag.
//
// Usage:
//
//	go run ./cmd/audit_surface_quality/                  # both views
//	go run ./cmd/audit_surface_quality/ -view=metadata   # metadata only
//	go run ./cmd/audit_surface_quality/ -view=output
package main
