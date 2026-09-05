// Package mcpsurface introspects the registered MCP surface for the generator
// commands.
//
// Several commands under cmd/ need the same thing: the tools, prompts and
// resources the server actually registers, read over a real MCP round-trip
// rather than described by hand, and against a surface that does not depend on
// the ambient environment. The server chooses its catalog from TOOL_SURFACE and
// its client from GITLAB_URL/GITLAB_TOKEN, so a generator that read either would
// emit different files on a developer machine than in CI. Every constructor here
// pins the surface explicitly and talks to an in-process stub instead, which is
// what makes the committed artifacts reproducible.
package mcpsurface
