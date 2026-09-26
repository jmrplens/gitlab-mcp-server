# ADR-0023: Tenant policy is declared once, in a register the layers read

## Status

Accepted, 2026-09-25.

## Context

[Issue 565](https://github.com/jmrplens/gitlab-mcp-server/issues/565) observed that the
credential is centralized and its policy is not. Six layers decided what a caller may
hold, each with its own criterion, and the fact that settled the two hardest of those
decisions had to be rediscovered each time: a share granted to a key the caller can mint
gives the honest tenant one unit and the attacker one per credential.
[Issue 540](https://github.com/jmrplens/gitlab-mcp-server/issues/540) found it about the
token and rejected four designs on it;
[issue 561](https://github.com/jmrplens/gitlab-mcp-server/issues/561) found it again about
the pool. It had never been written down anywhere a seventh layer would find it.

The specification (`docs/development/tenant-policy-spec.md`) answered the question the
issue put first. A tenant is the GitLab principal a request runs as, identified by the
pair (canonical instance URL, GitLab user id), because GitLab authorizes, attributes and
throttles per user. That pair is mintable too: on a self-managed instance any user with a
personal project can create project bots, each a user of its own, with no administrator
and no count cap. So the tenant is the unit of identity, attribution and authority, and
never of a share.

Measured at `cb6379f53`, seventy caller-keyed decisions and ten per-request bounds answer
who a caller is and what it may hold, spread over `cmd/server`, `internal/serverpool`,
`internal/subscriptions`, `internal/toolutil`, `internal/oauth`, `internal/config`,
`internal/gitlab`, `internal/elicitation`, `internal/cachehints`,
`internal/clientcompat` and `internal/tools`. The six families the issue names alone are
named on 147 non-test lines in 15 files. Four of the decisions are keyed finer than the
tenant while their own reason is per user or per process, and thirty-five findings record
where today's answers disagree with the definition, with each other or with the protocol.

The issue asked for a module that decides and does not enforce, where the next limit is
added in one place with the mintable-key answer already given, and where a change of
policy is a change to one file. It put any change of value or behavior out of scope,
because "a change of policy that rides in on a refactor is how a bound quietly moves".

## Decision

**A register, `internal/tenancy`, holds every decision; a type-checked gate binds it to
the code; the layers keep enforcing.**

**The register** is a leaf package that imports only `errors`, `fmt`, `strings` and
`time`, and declares no package-level variable. It holds:

- **The key vocabulary** (`tenancy.Key`). Each key states its axis (the request, before
  admission, the requester or the process), what it costs a caller to mint another value
  of it, the unit a reason about it is compared in, and the evidence for its mint cost,
  with the symbols that compute it today declared rather than moved. `KeyTenant` is
  mintable, which is the specification's central fact written as data.
- **One row per requirement** (`tenancy.Decisions()`), each with its question, kind,
  class, disposition, key and stdio key, the unit its stated reason names and the unit of
  what that reason cites, whether it protects a process resource and its process partner,
  its value source, flags and variables, its zero meaning and malformed-value policy, what
  happens at capacity, its refusals per method and era (channel, code, status,
  `Retry-After`, stable text prefix, class of next action and the budgets it is charged
  to), the records that decided it, the findings it carries, and the symbols that decide,
  enforce, refuse and state it.
- **The values** the layers alias (`values.go`) and the policy refusal codes
  (`codes.go`). Each constant has exactly the typedness of the literal it replaces, so a
  site that aliases one compiles to the same instructions and data.
- **The authentication failure table** (`tenancy.Failures()`): every refusal the gate and
  the bearer guard return, charged or not, which states `INV-007` as data where it used
  to be an absence.
- **The carried-channel matrix** (`tenancy.Carriages()`): which refusal channels go-sdk
  carries for which method in which era.
- **`Validate` and `ValidateFailures`**, which turn the invariants into errors: a share on
  any mintable key, a per-caller number protecting a process resource with no constant
  process partner, a holding taken across keys without a recorded decision, a refusal on
  a channel its method cannot carry, a second in-band code for one class of next action,
  a zero that does not mean off, a reason whose unit differs from its key, a structure
  keyed on a mintable value with no capacity, a configurable value that bypasses the
  configuration package, and a charged failure the caller did not cause. They refuse in
  three ways, which the specification lists rule by rule. With no exception: a share on a
  mintable key (`INV-003`); a charge to what is not a budget before admission, and a
  charged failure the caller did not cause (`INV-007`, the second through
  `ValidateFailures`); a refusal on a channel its method cannot carry, or with a status, a
  code, a `Retry-After` or a challenge the channel forbids (`INV-011`); a process partner
  that is configurable or not keyed on the process, and a reason about the process on a
  row that does not say it protects one (`INV-004`); a valued row that does not say what
  zero means (`INV-015`); and a variable without the `GITLAB_MCP_` prefix, or a
  configurable value with no flag, variable or malformed-value policy (`INV-017`). With a
  recorded decision: a holding taken across keys (`INV-005`). Only through a finding
  recorded for the invariant: a per-caller number protecting a process resource with no
  partner, a structure keyed on a mintable value with no capacity, a code in the legacy
  `-32000` range, a second in-band code for one class of next action, a zero that does not
  mean off, a reason whose unit differs from its key or misstates its own, a value only an
  environment variable or a Go option reaches, and a ceiling nothing bounds.

**The gate**, `cmd/audit_tenancy`, loads `./cmd/server` and `./internal/...` through the
type checker and holds the register to the code: each site aliases, pins or reads what its
row says; each refusal's text, status, code and headers are still where the row says; each
charge sits on the refusal the failure table says it does; and nothing limit-shaped in a
shape it reads exists outside a declared site or a reasoned exemption. It runs in
`make analyze` and in CI.

**Enforcement stays in the layers.** A stream ceiling is applied where streams are
counted, a token bucket where the method is dispatched. A value moves into the register as
a constant the layer aliases; a rule moves only when it is a pure function of values the
enforcing site already holds, calling it changes neither what is evaluated nor in what
order, its input domain can be enumerated or fuzzed against a verbatim copy of the code it
replaces, and the site's allocation and lock profile is unchanged. Every other rule stays
declared by the symbol that implements it. A value is pinned rather than aliased only
where aliasing would retire half of a recorded finding (the defaults
[issue 958](https://github.com/jmrplens/gitlab-mcp-server/issues/958) records as stated
twice), and the gate holds the pinned literal equal to the register's.

## Consequences

### Positive

- **POS-001**: The mintable-key question is answered before review. A share on any key a
  caller can mint, the tenant included, fails `Validate` with that key's evidence in the
  message.
- **POS-002**: A deliberate change of value is two lines in one package: the constant, and
  its pin in `TestValues_HoldTheirPins`. A change of value that rides in on a refactor
  fails that test.
- **POS-003**: A new limit the gate can read is refused until the row that declares it
  exists: one built with a listed constructor or options type outside every declared
  Enforce site, or from a literal or a package value no row declares; one whose refusal
  is a literal with a policy code or a 429 or 503, or such a status written with
  `http.Error` or `WriteHeader`, in a function that declares no refusal of that code or
  status; or one named with a limit word at package level in a package that holds an
  Alias, Arg, Pin or Enforce site of a row that is not a request bound. A limit built with
  none of the listed constructors, such as a counter guarded by a mutex, is not among them
  when it refuses by calling a function that already builds a declared refusal, or through
  a literal in a function that already declares one of that code or status, unless it is
  such a package-level name in such a package: the gate's documentation states that
  escape, and review is what sees it. The row then answers the specification's
  validation checklist, all of it but VAL-010 and VAL-012, which stay in the pull request
  because a declaration cannot hold them.
- **POS-004**: Moving a value into the register changes no code. The binary a value layer
  builds is byte-identical to the one its parent builds once the parent imports the
  register where the layer does, which the gate's code-identity mode checks for any change
  that claims to move policy without changing it. Against the parent itself the
  comparison fails, because an inserted import line resizes the line table and can move
  what the linker lays out after it, and a new importer changes where the linker places
  read-only data; the value layers of issue 565 measured both.

### Negative

- **NEG-001**: Most rule-shaped decisions are declared by symbol. A change to the logic of
  one of them fails nothing in the register; the gate fails only if the symbol disappears,
  and the layer's own tests remain what pins its behavior.
- **NEG-002**: The unit of what a reason cites is a declaration. The quoted reason, the
  unit it names in its own words and the gate's check that the quotation is still where
  the row says narrow it, and a reviewer still weighs it.
- **NEG-003**: The gate's exemption table is a set of judgements, each with a category and
  a reason, and a wrong category is caught only by review.
- **NEG-004**: The fairness benchmark can drive two bounds, the tool-call and the catalog
  buckets, so every other decision's proof that a migration changed nothing is by
  construction: code identity for a value, a verbatim oracle and fuzzing for a rule.
- **NEG-005**: The carried-channel matrix follows go-sdk; an SDK upgrade that changes what
  is carried edits it in the same change.
- **NEG-006**: A promoted rule is a call where the code used to be written in place, and it
  is not free. `MeterFor` inlines and leaves a second switch on the bucket it returns,
  about 0.9 ns more for a method no bucket meters; `Busy` does not inline, since it reads
  two interface methods, about 1 to 1.3 ns more for each pool entry an eviction scan
  passes; the budget switches inline, and the two startup functions that call them are
  laid out differently. None of them allocates, and each leaves its site's lock profile as
  it was, which is what the promotion rule protects: `Busy` reads the watcher count
  through an interface that takes the subscription manager's lock, exactly when the code
  it replaced did, only when no stream is open. Each is proved by its oracle and fuzz
  target rather than by the binary.

### Neutral

- **NEU-001**: No finding is fixed. Each keeps its own issue, and the rows that carry one
  pass `Validate` only because they do.

## Alternatives considered

**An interface consulted by the layers.** A `Decider` with one stateless implementation,
injected into each layer. It would move request-dependent rules into the policy package,
which a register of constants cannot. It was declined because it has a single
implementation and no second one in prospect, and injecting it without editing the tests
that build the layers needed parallel constructors and a rule that a nil decider means the
standard one, so a production path that forgot the decider would still run the shipped
policy silently; and because it rewrote refusal construction on the request path. It
becomes right when a decision has to be computed per request from the tenant.

**Typed limits resolved per entry.** A `Limits` value carried on each credential's state.
It was declined because an entry's configuration is the process configuration with a few
fields changed, so per-entry limits hold one copy of each process constant per entry and
make the entry, the key the class D findings are about, the natural unit of the type; it
needs a second mechanism for everything per process or before admission; and it evaluates
the watcher count eagerly, taking the manager's lock under the pool's where the code does
not today. It becomes right when values differ between tenants.

Both are policy changes (`INV-021`). The register is their input rather than their
obstacle: a resolver would be built from `Decisions()` and `Key.MintCost()`.

## Compliance

- `make check-tenancy`, in `make analyze` and in CI, holds the register to the code.
- `TestDecisions_EveryRequirementHasOneRow` holds the register to the specification's
  requirements, one row each.
- `TestDecisions_ValidateIsNil` and `TestFailures_ValidateFailuresIsNil` hold the rows to
  the invariants.
- `TestValues_HoldTheirPins` pins every policy value and its type.
- `TestPackage_ImportsTheStandardLibraryOnly`, `TestPackage_DependsOnTheStandardLibraryOnly`,
  `TestPackage_LinksNothingTheServerDoesNot` and `TestPackage_DeclaresNoPackageLevelVariable`
  (which refuses an init function too) hold the leaf to the conditions an unchanged binary
  rests on.
- `make tenancy-code-identity` proves that a change which claims to move policy without
  changing it changed no code: against its parent (`BASE=<ref>`) when it inserts no line
  and gives the register no new importer, and otherwise against its parent with the same
  import lines added as blank imports at the same positions (`BASE_TREE=<dir>`).
- Each promoted rule (`MeterFor`, `Busy`, `BudgetOn`, `EscalationOn`,
  `TransportSourceBudgetOn`) is held by an oracle test and a fuzz target to a verbatim copy
  of the code it replaced.

## Related

- ADR-0015 (polled resource subscriptions), ADR-0018 (admission at the minimum scope),
  ADR-0019 (audience binding), ADR-0020 (one server per configuration shape) and ADR-0022
  (operator-named destinations).
- The specification, `docs/development/tenant-policy-spec.md`.
- Issues [540](https://github.com/jmrplens/gitlab-mcp-server/issues/540),
  [561](https://github.com/jmrplens/gitlab-mcp-server/issues/561) and
  [565](https://github.com/jmrplens/gitlab-mcp-server/issues/565).
