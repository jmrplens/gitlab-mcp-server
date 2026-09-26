// Command audit_tenancy holds the tenant policy register, internal/tenancy,
// to the code that enforces it.
//
// # Why this needs a gate of its own
//
// The register declares every decision this server makes about who a caller
// is and what it may hold, spend or be told (issue 565, ADR-0023): one row per
// requirement of docs/development/tenant-policy-spec.md, with the symbols
// that decide, enforce, refuse and state it. A declaration nothing checks
// drifts from the code the day the code moves, and the register's own
// validator cannot see the code at all, since the register imports nothing.
// This command is what binds the two: each row's sites still exist and do
// what the row says, and nothing limit-shaped exists outside a row.
//
// # The rules
//
// Each rule answers one way the register and the code can disagree.
//
//   - G1 resolve. Every site of every row, key derivation and failure, and
//     every exemption, names exactly one declaration, and no exemption is also
//     a site.
//   - G2 alias. An Alias site's initializer is a reference to the register
//     constant it names, or to another declared alias of it, and a const
//     keeps the constant's typedness; for a composite literal every element
//     is declared and is one.
//   - G3 arg. Inside an Arg site's function, the calls to the callee it names
//     pass the register constant at its index, exactly as many times as it
//     says.
//   - G4 pin. A Pin site keeps its own literal, and that literal folds to the
//     register constant's value and type (the second statements issue 958
//     records).
//   - G5 references. An Enforce site that names what it reads refers to it,
//     and every register function a row names is called from one of its
//     Enforce sites.
//   - G6 orphans. Every exported constant of the register's value and code
//     files is read by a declared Alias or Arg site and by something outside
//     the register; every declared Alias is read by code other than an alias
//     initializer, directly or through a chain of aliases, so an enforcing
//     site that goes back to a literal leaves its alias unread and fails;
//     every exported function of its rule files is named by a row.
//   - G7 charges. Every refusal the authentication failure table's functions
//     return is matched to one row by its status and text, and is charged
//     exactly when a call of the charge helper precedes it in its own block or
//     in a block enclosing it; the calls that spend a budget are made only
//     from the charge helpers; and each failure agrees with its row's gate
//     refusal of the same status and prefix, which names a budget exactly when
//     the failure is charged.
//   - G8 refusal. Every refusal's stable text begins a string its code folds
//     (a format read up to its first verb), and the literal that carries it
//     has the row's status, code, Retry-After source and challenge; a JSON-RPC
//     refusal carries the row's code, read from the case that names its
//     sentinel where its holder switches on one, and a tool-error refusal's
//     result is flagged as an error. Every gate literal of a function that
//     holds a declared gate refusal is carried exactly by one of them, so a
//     sibling of the same status cannot stand in for a literal that drifted.
//   - G9 reason. Every quoted reason still appears in the doc comment of the
//     declaration it names, or of the block around it.
//   - G10 tripwire. A limit constructor or a literal of a limit's options
//     type sits in a declared Enforce site, with no literal or undeclared
//     package value among its inputs; a refusal literal that reads as a
//     limit's, and a 429 or 503 written with http.Error or WriteHeader, is
//     the refusal a row declares in its function, by code or by status; and a
//     package-level name that reads as a limit (in every package the
//     register's value and enforcing sites are in) is a declared site. Each
//     is otherwise answered by the exemption table.
//   - G11 validate. The register's own validators accept it.
//   - G12 leaf. The register imports only the packages the server already
//     imports and declares no package-level variable and no init function,
//     the conditions the code-identity proof rests on.
//   - G13 share words. A number on a key a caller can mint is not described
//     as fair, a quota or an entitlement, unless its row carries the finding
//     that records it.
//   - G14 config. Every variable a row names is on the configuration
//     package's list of prefixed names and read by nothing else (os.Getenv,
//     os.LookupEnv, their syscall twins, and a template os.ExpandEnv
//     expands), unless its row carries the finding that records it.
//
// Where a finding excuses a rule (G13, G14), a row carrying that finding
// while the rule would pass has a finding that no longer describes the tree,
// and that is reported too.
//
// # Declarations
//
// declarations.go holds what the rules match the code against and the table a
// reviewer judges. notADecision is the exemption table: each entry names a
// declaration shaped like a limit that decides nothing about a caller, keyed
// `package:Name` the way the register names a site, with a category and a
// reason. A declaration that answers nothing is a finding on the terms every
// declaration table here is held to: an exemption nothing needed, and one
// naming a category nobody defined. Every rule applies to every row; the
// deferral the migration of issue 565 needed while the values moved into the
// register is gone, so a row whose value its layer reads as a literal is a
// finding the day it is written.
//
// # What it reads, and what it cannot see
//
// ./cmd/server and ./internal/..., loaded through cmd/internal/goprogram
// without test files, with the test-support packages the server never links
// left out. A refusal-shaped literal in those answers no caller of the
// server. The load names linux/amd64 whatever the host is, so the verdict is
// the same on every machine that runs it; a file constrained to another
// platform is not read.
//
// It is a gate, not a proof. A new limit escapes G10, and only review sees it,
// when it uses none of the listed constructors or options types, refuses
// through none of the three literal types and no 429 or 503 status write (or
// not at all), and is named in a way part (c) does not read: with no lexicon
// word, in a package no row names, or as a value local to a function, which
// part (c) never reads (revalidateAll's ten seconds a probe is one, stated in
// ADM-009 and bound by no row value). G14 cannot trace a variable read through
// a name that does not fold. G7 binds a charge to a return by position, so a
// charge moved into a helper the table does not name fails rather than
// passing, and the finding names the position rather than the policy. A rule
// row's logic is declared by symbol, so G1 fails when the symbol disappears
// and nothing fails when its logic changes; that is what promoting a rule
// into the register exists for.
//
// # Code identity
//
// -compare-binaries A B is the other half of a change that moves a value into
// the register: it reads two ELF binaries and exits non-zero unless every
// allocated section but the line table, .gopclntab, is byte-identical.
// `make tenancy-code-identity BASE=<ref>` builds the server at the ref and in
// the working tree and runs the comparison, and BASE_TREE=<dir> names an
// exported tree instead.
//
// Which base the change is compared against is the whole of the proof. A
// change that inserts no line into the server's source and gives the register
// no new importer is compared against its parent as it is. A change that
// imports the register into a file is not, because the import alone moves
// sections: the inserted lines resize the line table, which can move what the
// linker lays out after it, and a new importer changes where the linker
// places content-addressed data. Such a change is compared against its parent
// with the same import lines added as blank imports at the same positions,
// passed as BASE_TREE; each value layer of issue 565 was byte-identical to
// that base as a whole file, while every one of them failed against its
// parent. A change to the register itself is the third case: rows, text and
// code the server never reaches still change the package's object, and the
// linker lays out content-addressed data and duplicated closures differently,
// so the comparison against the parent fails with every symbol the same size.
// Such a change is compared against its parent with the register replaced by
// its own, which proves nothing outside the register moved; that the linked
// code of the register did not change is its oracle tests' and value pins'.
//
// A rule promoted into the register is not proved by the binary at all: a
// layer calls it, and the compiler may lay its callers out differently. Its
// proof is an oracle test and a fuzz target holding it to a verbatim copy of
// the code it replaced, with its allocations pinned.
//
// Usage:
//
//	go run ./cmd/audit_tenancy/                    # report
//	go run ./cmd/audit_tenancy/ -check             # fail on a finding (CI gate)
//	go run ./cmd/audit_tenancy/ -v                 # also list what the exemption table answered
//	go run ./cmd/audit_tenancy/ -compare-binaries A B
package main
