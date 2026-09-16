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
// import cycle in its tests.
//
// It was moved out of cmd/server when a second caller needed it: the surface
// evaluator that used to live under cmd registered its own server, so the pass
// ran nowhere but the binary and a read-only or safe-mode meta evaluation
// scored a surface on which gitlab_interactive_issue_create still created the
// issue, while the product would have withdrawn or previewed it. That caller
// is gone and the defect with it, because the evaluation under test/e2e drives
// the real binary: the pass runs in cmd/server, where it always ran, and an
// evaluation now sees exactly the surface a client sees because it is the same
// process.
//
// What keeps it here rather than folded back is the other half, which the
// second caller only made visible: cmd/server is the one package exempt from
// the coverage rule, so while the pass lived there it had no unit test of its
// own on its policy. Here it has one.
package toolvisibility
