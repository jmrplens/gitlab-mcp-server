// Package sourcewalk answers one question for every command that walks this
// repository's tree: which directories below a walk root are this
// repository's own source, and which are something else that merely lives
// inside the checkout.
//
// It exists because the answer was wrong in a way no reader could see. The
// parallel-agent tooling puts a git worktree per agent under
// .claude/worktrees/, so at any moment the checkout may contain a hundred and
// more complete copies of this repository, each on a different branch. Nothing
// excluded them. cmd/audit_catalog_first walks the repository root to prove
// that no production file calls a forbidden selector, and its skip list named
// .git, dist and site, so it descended into every one of those copies and
// judged their files as if they were ours.
//
// The way that announced itself was the benign half: a worktree the other
// agent removed between the walk's readdir and its open made the audit fail
// with "no such file or directory", which took make gen-testing-docs down with
// it. The half worth fixing is the one that does not crash. A worktree that
// stays put is read, its Go files are parsed, and its verdict is folded into
// ours without a word, so whether a gate passes depends on whether an agent
// happened to be running and on what branch it happened to have checked out,
// which is invisible to whoever reads the result. Several audits here count
// things, and an extra copy of internal/tools would inflate a count or make a
// duplicate look like a finding.
//
// # Two rules, and why both
//
// [SkipDir] is a name rule: a dot-directory is not source this repository
// holds to its conventions. It is Go's own rule for ./... and it is what
// cmd/internal/testsource already applied, which is why the five commands
// that read _test.go files through it were never affected. It is cheap, it
// needs no syscall, and it covers today's directory, .claude, exactly.
//
// [IsNestedCheckout] is a marker rule: a directory holding a .git entry is the
// root of another checkout, a linked worktree's .git file as much as a clone's
// .git directory. The name rule excludes .claude/worktrees because of where
// the tooling puts it; this one excludes a worktree because of what it is. A
// developer who runs `git worktree add ./scratch` gets no protection from the
// name rule, and the failure mode there is the silent one: scratch/ is read as
// ours. Naming .claude in a skip list would be fixing the crash and leaving
// the defect.
//
// The argument against the marker rule is that it is a filesystem probe rather
// than a name test, so it costs one Lstat per directory entered and it can be
// wrong in the other direction: a fixture tree that deliberately plants a .git
// would vanish from a walk meant to read it, and a submodule that is genuinely
// part of the source would be dropped. Neither has an instance here. This
// module vendors nothing and has no .gitmodules, and every fixture tree an
// audit is driven over is handed to it as the walk root, which is exempt. The
// cost is 697 directories, once per run.
//
// Both rules together are [SkipDirBelowRoot], which is what a walk calls.
//
// # What stays with each walker
//
// Only the minimum is here: "not this repository's source". Each command keeps
// its own extra names, because those say what that command is not interested
// in rather than what is not ours, and folding them together would silently
// change what each walk counts. testsource adds node_modules, dist and
// testdata; cmd/audit_catalog_first adds dist and site; cmd/audit_install_buttons
// adds node_modules and dist. Those lists are each command's business.
//
// # The walk root is always entered
//
// Every rule here judges a directory a walk descends into, never the root it
// was pointed at. The root is named by the caller, so a scan asked for a
// fixture tree scans it whatever it is called, and an audit driven over a
// temporary directory that is itself a checkout still runs. [SkipDirBelowRoot]
// carries that in its name because forgetting it is not a subtle failure: the
// repository root holds .git, so a walker that applied the marker rule to its
// own root would walk nothing at all and report a clean tree.
package sourcewalk
