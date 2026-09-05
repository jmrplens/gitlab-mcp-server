// Package stdioe2e drives the real gitlab-mcp-server binary over pipes the way
// an MCP client does: process lifetime, stdout carrying nothing but JSON-RPC,
// logs on stderr, readiness before the catalog is built, and the environment
// variables stdio configuration actually reads. It needs no GitLab and no
// credentials, so it runs on every CI push.
//
// The tests carry the stdioe2e build tag; this file is what a plain build sees
// of the package. stdio is the primary transport and two defects shipped while
// nothing drove it against a binary, which is what this module automates.
package stdioe2e
