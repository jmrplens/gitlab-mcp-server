package main

import (
	"fmt"
	"io"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/internal/requestinventory"
)

// catalogActions is a variable so the failure branch stays reachable from a
// test: the catalog is compiled into this binary and cannot be made to fail
// from outside.
var catalogActions = requestinventory.Actions

// summarize prints what the merged inventory holds, and how much of the
// catalog the recording could see.
//
// A failure to build the catalog costs the coverage line and nothing else: the
// inventory is the artifact this command exists to write, and it is complete
// with or without a count of what is missing from it.
func summarize(progress io.Writer, root string, rows []requestinventory.Row, verbose bool) {
	fmt.Fprintf(progress, "request inventory: %d rows, %d distinct paths, %d packages\n",
		len(rows),
		countDistinct(rows, func(r requestinventory.Row) string { return r.Path }),
		countDistinct(rows, func(r requestinventory.Row) string { return r.Package }))

	actions, err := catalogActions()
	if err != nil {
		fmt.Fprintf(progress, "  catalog coverage unavailable: %v\n", err)
		return
	}
	summary := requestinventory.Classify(root, rows, actions)
	fmt.Fprintf(progress, "  %d of %d catalog actions are owned by a package that recorded a request\n", summary.Covered, summary.Total)
	fmt.Fprintf(progress, "  %d in %d package(s) recorded none, and %d are owned by a name that is no package under %s\n",
		summary.Silent, len(summary.SilentOwners), summary.Unmapped, requestinventory.ToolsDir)
	if !verbose {
		return
	}
	for _, owner := range summary.SilentOwners {
		fmt.Fprintf(progress, "  silent: %s/%s\n", requestinventory.ToolsDir, owner.Package)
	}
	for _, owner := range summary.UnmappedOwners {
		fmt.Fprintf(progress, "  no package of that name: %s\n", owner.Package)
	}
}

// countDistinct counts the distinct values of one field across the rows.
func countDistinct(rows []requestinventory.Row, field func(requestinventory.Row) string) int {
	seen := map[string]struct{}{}
	for _, r := range rows {
		seen[field(r)] = struct{}{}
	}
	return len(seen)
}
