// Package toolvisibility decides which registered tool a narrowed deployment
// serves, for every caller that registers a tool surface the way the server
// does.
//
// Read-only mode, safe mode and --exclude-tools act in two places. The
// catalog-backed tools receive them per action from the catalog filter, inside
// the catalog every assembler shares. The tools registered outside the catalog,
// the gitlab_interactive_* flows the meta and individual surfaces register,
// receive them from [Apply], a pass over the tools a server holds once
// registration is done: it removes the names --exclude-tools lists, in
// read-only mode removes every tool without a read-only hint, and in safe mode
// wraps what is left with previews, exempting the catalog-backed dispatchers
// because they already preview per action.
//
// It is a package of its own for the reason internal/tools/dynamiccatalog is:
// the pass needs the two filters internal/tools holds and the two tool names
// internal/tools/dynamic declares, and neither package can hold it without an
// import cycle in its tests. Here it is reachable from cmd/server and from the
// surface evaluator alike, which is the point. While the pass lived in
// cmd/server it ran nowhere else, so a read-only or safe-mode meta evaluation
// scored a surface on which gitlab_interactive_issue_create still created the
// issue, while the product would have withdrawn or previewed it; and the
// package cmd/server is the one exempt from the coverage rule, so the pass had
// no unit test of its own on its policy either.
package toolvisibility
