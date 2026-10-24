package tenancy

import (
	"errors"
	"fmt"
	"strings"
)

// ViolationError is one way a declaration breaks an invariant of the
// specification.
type ViolationError struct {
	// ID is the row, or, for a failure, its function and kind.
	ID string
	// Rule names the check, "share-on-mintable".
	Rule string
	// Invariant is what the check holds, "INV-003".
	Invariant string
	// Detail says what is wrong, in terms a reviewer can act on.
	Detail string
}

// Error renders the violation as one line.
func (v *ViolationError) Error() string {
	return fmt.Sprintf("%s: %s (%s): %s", v.ID, v.Rule, v.Invariant, v.Detail)
}

// rowRule is a check of one row, which may consult the others by id.
type rowRule struct {
	name      string
	invariant string
	check     func(d Decision, rows map[string]Decision) []string
}

// setRule is a check of the register as a whole.
type setRule struct {
	name      string
	invariant string
	check     func(ds []Decision) []*ViolationError
}

// Validate checks the rows against the invariants (spec: Invariants) and
// returns every violation joined, or nil.
//
// It is what answers, before review, the questions issues 540 and 561 each
// answered by hand: a share on a key a caller can mint is refused with the
// key's evidence in the message, a per-caller ceiling on a process resource
// with no process partner is refused, and so is a refusal on a channel the
// method cannot carry. A rule that can be excused passes a row breaking its
// invariant only through a finding recorded for it (see
// [Finding.Invariants]); some rules cannot be excused at all, and two are
// excused by a recorded decision instead: checkAcrossKeys, and the half of
// checkZeroStated that lets a process partner follow the row it stands
// beside. The specification's section on invariants says which is which.
func Validate(ds []Decision) error {
	rows := make(map[string]Decision, len(ds))
	for _, d := range ds {
		if _, seen := rows[d.ID]; !seen {
			rows[d.ID] = d
		}
	}
	var errs []error
	for _, r := range rowRules() {
		for _, d := range ds {
			for _, detail := range r.check(d, rows) {
				errs = append(errs, &ViolationError{ID: d.ID, Rule: r.name, Invariant: r.invariant, Detail: detail})
			}
		}
	}
	for _, r := range setRules() {
		for _, v := range r.check(ds) {
			v.Rule, v.Invariant = r.name, r.invariant
			errs = append(errs, v)
		}
	}
	return errors.Join(errs...)
}

// The invariants the checks below name in more than one place: the rule that
// reports a breach and the departure a row records to be let through have to
// spell the same ID, so they read it from one constant.
const (
	// invNotJudgedNotCharged is INV-007: what was not judged is not charged.
	invNotJudgedNotCharged = "INV-007"
	// invUnitsAgree is INV-016: every limit states its key, mint cost and
	// reason, and the units agree.
	invUnitsAgree = "INV-016"
	// invProcessOnProcess is INV-018: bound the process on the process.
	invProcessOnProcess = "INV-018"
)

// rowRules are the checks made one row at a time, in the order the design
// lists them.
func rowRules() []rowRule {
	return []rowRule{
		{"share-on-mintable", "INV-003", checkShareOnMintable},
		{"process-partner", "INV-004", checkProcessPartner},
		{"process-absent", invProcessOnProcess, checkProcessAbsent},
		{"across-keys", "INV-005", checkAcrossKeys},
		{"channel-carried", "INV-011", checkChannelCarried},
		{"zero-stated", "INV-015", checkZeroStated},
		{"units-agree", invUnitsAgree, checkUnitsAgree},
		{"stated-unit", invUnitsAgree, checkStatedUnit},
		{"class-consistent", "VAL-003", checkClassConsistent},
		{"through-config", "INV-017", checkThroughConfig},
		{"fixed-reasoned", "VAL-009", checkFixedReasoned},
		{"capacity-stated", "VAL-006", checkCapacityStated},
		{"capacity-fits", "VAL-006", checkCapacityFits},
		{"bounded-table", "INV-010", checkBoundedTable},
		{"charged-exists", invNotJudgedNotCharged, checkChargedExists},
		{"stdio", "VAL-011", checkStdio},
		{"well-formed", "VAL-001 to VAL-009", checkWellFormed},
	}
}

// setRules are the checks made over the register as a whole.
func setRules() []setRule {
	return []setRule{
		{"code-mirrors-status", "INV-012", checkCodeMirrorsStatus},
		{"unique", "one row per requirement", checkUnique},
		{"complete", "one row per requirement", checkComplete},
	}
}

// checkShareOnMintable refuses a share on any key a caller can mint, the
// tenant included, with no escape: a share promises every key its portion, and
// a caller who can mint keys takes one portion per key. The detail quotes the
// key's evidence, so the refusal explains issue 540's fact where it happens.
func checkShareOnMintable(d Decision, _ map[string]Decision) []string {
	if d.Kind != Share || !d.Key.Mintable() {
		return nil
	}
	return []string{fmt.Sprintf("a share on the %s key, which a caller can mint: %s", d.Key, d.Key.Evidence())}
}

// checkProcessPartner holds a per-caller number that protects a process
// resource to a non-configurable process ceiling beside it, and a reason about
// the process to saying so.
//
// A caller's key before admission is held as one after it is: an address is a
// key a caller holds one of and can mint another of, so a number on it
// multiplies with every address the caller holds, as one on a credential does
// with every token. Only a bound on one request is left out, since a request
// is nobody's holding. RTC-008 is the row this reaches, and it passes by the
// finding that records issue 1053's decision to leave it without a partner.
func checkProcessPartner(d Decision, rows map[string]Decision) []string {
	var out []string
	perCaller := d.Key.Axis() == AxisRequester || d.Key.Axis() == AxisPreAdmission
	if d.ProtectsProcess && perCaller && d.Partner == "" && !d.RecordsDeparture("INV-004") {
		out = append(out, fmt.Sprintf("protects a process resource on the %s key with no process partner", d.Key))
	}
	if partner, ok := rows[d.Partner]; ok && (partner.Key != KeyProcess || partner.Source != Constant) {
		out = append(out, fmt.Sprintf("its partner %s is not a constant keyed on the process", d.Partner))
	}
	if d.ReasonUnit.Unit() == UnitProcess && !d.ProtectsProcess {
		out = append(out, "its reason is about the process and it does not say it protects one")
	}
	return out
}

// checkProcessAbsent refuses a ceiling nothing bounds, the way a row records a
// resource with no bound, unless a finding records it.
func checkProcessAbsent(d Decision, _ map[string]Decision) []string {
	if d.Kind != Ceiling || d.Source != SourceNone || d.RecordsDeparture(invProcessOnProcess) {
		return nil
	}
	return []string{"a ceiling nothing bounds, with no finding"}
}

// checkAcrossKeys refuses taking one key's holding to admit another without a
// recorded decision.
func checkAcrossKeys(d Decision, _ map[string]Decision) []string {
	if d.AtCapacity != EvictAcrossKeys || len(d.Decided) > 0 {
		return nil
	}
	return []string{"takes a holding across keys with no recorded decision"}
}

// checkChannelCarried holds every refusal to a channel go-sdk v1.8.0 carries
// for its methods and era, and to a code MCP lets this server send there.
func checkChannelCarried(d Decision, _ map[string]Decision) []string {
	var out []string
	for i := range d.Refusals {
		out = append(out, refusalCarriage(&d.Refusals[i])...)
	}
	return out
}

// refusalCarriage is [checkChannelCarried] for one refusal.
func refusalCarriage(r *Refusal) []string {
	var out []string
	if len(r.Methods) == 0 {
		out = append(out, "a refusal names no method")
	}
	for _, m := range r.Methods {
		if !Carries(m, r.Era, r.Channel) {
			out = append(out, fmt.Sprintf("the %s channel is not carried for %s", r.Channel, m))
		}
	}
	if r.Channel != Gate && (r.Status != 0 || r.RetryAfter != RetryAfterNone || r.Challenge) {
		out = append(out, "a status, a Retry-After or a challenge on a channel that carries none")
	}
	if r.Channel == Gate {
		mirrored := r.Code == r.Status*-100
		invalidRequest := (r.Status == 400 || r.Status == 404) && r.Code == codeInvalidRequest
		if !mirrored && !invalidRequest {
			out = append(out, fmt.Sprintf("the gate's code %d does not mirror status %d", r.Code, r.Status))
		}
		if r.Status == 401 && !r.Challenge {
			out = append(out, "a 401 without a WWW-Authenticate challenge")
		}
	}
	if r.Channel != Gate && r.Channel != RPC {
		if r.Code != 0 {
			out = append(out, fmt.Sprintf("code %d on a channel that carries none", r.Code))
		}
		return out
	}
	return append(out, codeCarriage(r.Code)...)
}

// codeCarriage is what MCP and the SDK allow of an error's code.
//
// The legacy sub-range is refused with no exception. A finding used to excuse
// it, F-07, while the listen and watcher ceilings still answered -32000; issue
// 956 retired the code from every refusal, and MCP 2026-07-28 says new codes
// must not be allocated there and receivers must not assume a meaning, so no
// row has a reason left to use one.
func codeCarriage(code int) []string {
	if code == 0 {
		return []string{"an error with no code reaches the client as code 0"}
	}
	if code == codeMethodNotFound {
		return []string{"-32601's message is replaced by the SDK, so it cannot explain a refusal"}
	}
	if reservedForMCP(code) && !mcpDefinesCode(code) {
		return []string{fmt.Sprintf("code %d is in the range MCP reserves and does not define", code)}
	}
	if inLegacyRange(code) {
		return []string{fmt.Sprintf("code %d is in the legacy range new implementations should not use", code)}
	}
	return nil
}

// reservedForMCP reports whether code lies in the sub-range of JSON-RPC's
// implementation-defined errors that MCP reserves for itself, -32020 to
// -32099.
func reservedForMCP(code int) bool {
	return code <= -32020 && code >= -32099
}

// inLegacyRange reports whether code lies in the legacy sub-range, -32000 to
// -32019, where new codes must not be allocated and receivers must not assume
// a meaning.
func inLegacyRange(code int) bool {
	return code <= -32000 && code >= -32019
}

// checkZeroStated holds a valued decision to saying what zero means, and a
// zero that is not "off" to a finding or a recorded decision.
//
// One zero decided by another row passes without a finding: a process partner
// switched off with the row it stands beside, when a recorded decision says
// so. It is still switching one limit off with another, but the other is the
// per-key limit the partner exists for, and following it is a policy choice
// the register records rather than a defect it carries (RTC-007, issue 951).
// The decision is the one OffWithBy names, and it must be among the row's
// Decided: a row carrying a decision about something else does not pass.
//
// A zero refused at startup, or one that selects the default, passes the same
// way when the record ZeroBy names decided it and the row records that
// decision (issue 958), whether it is the row's own zero or the zero of one of
// its settings: it is then a stated exception with its reason, rather than a
// silent normalization the register carries as a finding. A ZeroBy on a row
// whose every zero means off decides nothing, and is refused, so a decision
// cannot stay on a row after the zero it decided has gone.
func checkZeroStated(d Decision, rows map[string]Decision) []string {
	var out []string
	if d.Disposition == Valued && d.Zero == ZeroUnset {
		out = append(out, "a valued decision that does not say what zero means")
	}
	departing := zeroDeparts(d.Zero)
	for _, s := range d.SettingZeros {
		departing = departing || zeroDeparts(s.Zero)
	}
	decided := d.ZeroBy != "" && has(d.Decided, d.ZeroBy)
	followsPartnered := rows[d.OffWith].Partner == d.ID && has(d.Decided, d.OffWithBy)
	departs := (departing && !decided) || (d.OffWith != "" && !followsPartnered)
	if departs && !d.RecordsDeparture("INV-015") {
		out = append(out, "a zero that does not mean off, with no decision or finding")
	}
	if d.ZeroBy != "" && !departing {
		out = append(out, fmt.Sprintf("its zero is decided by %s, and every zero it states means off", d.ZeroBy))
	}
	return out
}

// zeroDeparts reports whether a zero meaning departs from INV-015: a zero
// refused at startup, or one replaced by the default.
func zeroDeparts(z Zero) bool {
	return z == ZeroRefused || z == ZeroSelectsDefault
}

// checkUnitsAgree holds a class D disagreement to a finding.
func checkUnitsAgree(d Decision, _ map[string]Decision) []string {
	if !d.Disagrees() || d.RecordsDeparture(invUnitsAgree) {
		return nil
	}
	return []string{fmt.Sprintf("its reason is about the %s and its key is the %s, with no finding", d.ReasonUnit, d.Key)}
}

// checkStatedUnit holds a reason that misstates the unit of what it cites to
// a finding.
func checkStatedUnit(d Decision, _ map[string]Decision) []string {
	if d.StatedUnit == KeyNone || d.StatedUnit.Unit() == d.ReasonUnit.Unit() || d.RecordsDeparture(invUnitsAgree) {
		return nil
	}
	return []string{fmt.Sprintf("its reason names the %s while what it cites is kept per %s, with no finding",
		d.StatedUnit, d.ReasonUnit)}
}

// checkClassConsistent holds the declared class to the two facts it follows
// from: whether the row disagrees, and which side of admission its key is on.
func checkClassConsistent(d Decision, _ map[string]Decision) []string {
	var out []string
	if (d.Class == ClassD) != d.Disagrees() {
		out = append(out, "class D and Disagrees differ")
	}
	if d.Class == ClassA && d.Key.Axis() != AxisPreAdmission {
		out = append(out, fmt.Sprintf("class A on the %s key, which exists after admission", d.Key))
	}
	if d.Class == ClassP && d.Key.Axis() == AxisRequester {
		out = append(out, fmt.Sprintf("class P on the %s key, which is a requester's", d.Key))
	}
	return out
}

// checkThroughConfig holds a configurable value to a flag, a GITLAB_MCP_
// variable and a malformed-value policy, and a value only an environment
// variable reaches to a finding. A value only a Go option reaches is no
// operator's configuration: it is a constant to every operator, and
// checkFixedReasoned holds it to its reason as it holds one (issue 958).
func checkThroughConfig(d Decision, _ map[string]Decision) []string {
	var out []string
	for _, env := range d.Envs {
		if !strings.HasPrefix(env, "GITLAB_MCP_") {
			out = append(out, fmt.Sprintf("the variable %s does not carry the GITLAB_MCP_ prefix", env))
		}
	}
	switch d.Source {
	case Configurable:
		if len(d.Flags) == 0 {
			out = append(out, "configurable with no flag")
		}
		if len(d.Envs) == 0 {
			out = append(out, "configurable with no GITLAB_MCP_ variable")
		}
		if d.Malformed == MalformedNone {
			out = append(out, "configurable with no malformed-value policy")
		}
	case EnvOnly:
		if !d.RecordsDeparture("INV-017") {
			out = append(out, "configurable outside the configuration package, with no finding")
		}
	}
	return out
}

// checkFixedReasoned holds a value no operator setting reaches, a constant or
// a Go option's, to saying why (issue 958). The process partners had a
// reason, INV-004's, and the other constants had none a reader could tell
// from an accident, so the next one copied whichever neighbor it was written
// beside. A reason on a value a setting reaches, or on one derived from
// another value or from a limit the process starts under, says something
// false about it, and is refused too.
func checkFixedReasoned(d Decision, _ map[string]Decision) []string {
	fixed := d.Source == Constant || d.Source == OptionOnly
	switch {
	case fixed && d.Fixed == "":
		return []string{"a value no setting reaches, with no reason it is fixed"}
	case !fixed && d.Fixed != "":
		return []string{"says why no setting reaches its value, which is not a constant or a Go option's"}
	}
	return nil
}

// checkCapacityStated holds a ceiling to saying what happens when it is full.
func checkCapacityStated(d Decision, _ map[string]Decision) []string {
	if d.Kind != Ceiling || d.Source == SourceNone || d.AtCapacity != CapacityNone {
		return nil
	}
	return []string{"a ceiling that does not say what happens when it is full"}
}

// checkCapacityFits is the capacity rule issue 958 stated: which of the
// behaviors at capacity a row may name, read off its kind, its key and
// whether it holds a record per key.
//
// Only what fills can state one: a ceiling, a rate, a budget, a share or a
// table. A wait is a ceiling's, for a slot another request frees. Reclaiming
// takes back what the same key holds, so it needs a key a caller holds. The
// oldest record is evicted only from a table kept before admission, where no
// tenant exists yet to hold anything and a record is a count or a verdict
// against its key, so evicting one takes nothing from another key (INV-005);
// and only where a record has a lifetime or a refill to be the oldest by,
// which is how EvictOldest reads. Counting stops only where the newcomer it no
// longer counts is let through, which a row says by declaring that silent
// refusal. And a holding is taken across keys only from a table, by the
// recorded decision checkAcrossKeys asks for (INV-005: the pool entry, POL-001
// and POL-002, and the verified OAuth identity, ADM-005). Refusing the
// newcomer fits anything that fills. Where more than one behavior fits, the
// rule does not choose between them: the row names one, and its comment says
// why.
func checkCapacityFits(d Decision, _ map[string]Decision) []string {
	if d.AtCapacity == CapacityNone {
		return nil
	}
	if !d.Kind.IsAllowance() && !d.Table {
		return []string{"states what it does when full, and holds nothing that fills: a ceiling, a rate, a budget, a share or a table"}
	}
	switch d.AtCapacity {
	case WaitThenRefuse:
		if d.Kind != Ceiling {
			return []string{"waits for room, and is not a ceiling a slot frees in"}
		}
	case ReclaimOwnThenRefuse:
		if axis := d.Key.Axis(); axis != AxisRequester && axis != AxisPreAdmission {
			return []string{fmt.Sprintf("reclaims its own holding on the %s key, which no caller holds", d.Key)}
		}
	case EvictOldest:
		if !d.Table || (d.Kind != Lifetime && d.Kind != Rate) {
			return []string{"evicts its oldest record, and is not a table of records with a lifetime or a refill"}
		}
		if d.Key.Axis() != AxisPreAdmission {
			return []string{fmt.Sprintf("evicts the oldest record on the %s key, where a record is a holding a caller has (INV-005)", d.Key)}
		}
	case StopCounting:
		if !declaresSilentRefusal(d) {
			return []string{"stops counting, and declares no silent refusal where the newcomer goes uncounted"}
		}
	case EvictAcrossKeys:
		if !d.Table {
			return []string{"takes a holding across keys, and is not a table one is taken from"}
		}
	}
	return nil
}

// declaresSilentRefusal reports whether one of the row's refusals reaches the
// caller as nothing at all.
func declaresSilentRefusal(d Decision) bool {
	for _, r := range d.Refusals {
		if r.Channel == Silent {
			return true
		}
	}
	return false
}

// checkBoundedTable refuses a structure keyed on a value a caller can mint
// that neither evicts nor refuses at a bound, unless a finding records it.
func checkBoundedTable(d Decision, _ map[string]Decision) []string {
	if !d.Table || !d.Key.Mintable() || d.AtCapacity != CapacityNone || d.RecordsDeparture("INV-010") {
		return nil
	}
	return []string{fmt.Sprintf("a structure keyed on the mintable %s key with no capacity, with no finding", d.Key)}
}

// checkChargedExists holds what a refusal is charged to to budgets that run
// before admission.
func checkChargedExists(d Decision, rows map[string]Decision) []string {
	var out []string
	for _, r := range d.Refusals {
		for _, id := range r.Charged {
			if row, ok := rows[id]; !ok || row.Kind != Budget || row.Key.Axis() != AxisPreAdmission {
				out = append(out, fmt.Sprintf("charged to %s, which is not a budget before admission", id))
			}
		}
	}
	return out
}

// checkStdio holds the stdio key to one no caller can multiply: on stdio every
// per-caller key collapses onto the process (spec: Per mode).
func checkStdio(d Decision, _ map[string]Decision) []string {
	if axis := d.StdioKey.Axis(); axis != AxisRequester && axis != AxisPreAdmission {
		return nil
	}
	return []string{fmt.Sprintf("the %s key on stdio, where every per-caller key is the process", d.StdioKey)}
}

// checkWellFormed refuses a row whose own fields contradict each other or name
// what does not exist.
func checkWellFormed(d Decision, rows map[string]Decision) []string {
	var out []string
	if d.Question == 0 || d.Kind == 0 || d.Class == 0 || d.Disposition == 0 {
		out = append(out, "a question, kind, class or disposition left unset")
	}
	for _, r := range d.Refusals {
		if r.Channel == 0 || r.Answer == 0 {
			out = append(out, "a refusal with no channel or no answer")
		}
	}
	out = append(out, danglingReferences(d, rows)...)
	out = append(out, settingZerosContradict(d)...)
	if d.Disposition == Promoted && len(d.Functions) == 0 {
		out = append(out, "promoted with no function")
	}
	if d.Disposition == Valued && len(d.Values) == 0 {
		out = append(out, "valued with no value")
	}
	if (d.Disposition == Mechanism || d.Disposition == RequestBound) && d.Kind.IsAllowance() {
		out = append(out, "a mechanism or a request bound shaped as an allowance")
	}
	return out
}

// settingZerosContradict is what a row's SettingZeros say against each other
// and against its Zero: a setting whose zero is stated twice has two meanings,
// and SettingZeros that name every one of the row's settings leave its Zero,
// the meaning of the rest, the meaning of none.
func settingZerosContradict(d Decision) []string {
	var out, stated []string
	for _, s := range d.SettingZeros {
		if has(stated, s.Config) {
			out = append(out, fmt.Sprintf("it states the zero of %s twice", s.Config))
		}
		stated = append(stated, s.Config)
	}
	if len(stated) > 0 && !leavesASetting(d.Config, stated) {
		out = append(out, "it states the zero of each setting, and leaves none of its settings to the row's own zero")
	}
	return out
}

// leavesASetting reports whether one of settings is not among stated.
func leavesASetting(settings, stated []string) bool {
	for _, c := range settings {
		if !has(stated, c) {
			return true
		}
	}
	return false
}

// danglingReferences is the half of [checkWellFormed] that follows a row's
// references: a partner and a zero partner must be rows, a record said to
// decide its zero must be one it records, a setting whose zero it states must
// be one of its settings, and a finding must be one of the specification's.
func danglingReferences(d Decision, rows map[string]Decision) []string {
	var out []string
	if _, ok := rows[d.Partner]; d.Partner != "" && !ok {
		out = append(out, fmt.Sprintf("its partner %s names no row", d.Partner))
	}
	if _, ok := rows[d.OffWith]; d.OffWith != "" && !ok {
		out = append(out, fmt.Sprintf("its zero follows %s, which names no row", d.OffWith))
	}
	for _, by := range []string{d.OffWithBy, d.ZeroBy} {
		if by != "" && !has(d.Decided, by) {
			out = append(out, fmt.Sprintf("its zero is decided by %s, which it does not record", by))
		}
	}
	for _, s := range d.SettingZeros {
		switch {
		case !has(d.Config, s.Config):
			out = append(out, fmt.Sprintf("it states the zero of %s, which is not one of its settings", s.Config))
		case s.Zero == ZeroUnset:
			out = append(out, fmt.Sprintf("it states the zero of %s and does not say what it means", s.Config))
		case s.Zero == d.Zero:
			out = append(out, fmt.Sprintf("the zero of %s means what the row's own does", s.Config))
		}
	}
	for _, id := range d.Findings {
		if FindingIssue(id) == 0 {
			out = append(out, fmt.Sprintf("it carries %s, which is not a finding", id))
		}
	}
	return out
}

// checkCodeMirrorsStatus holds every in-band code this server allocates to the
// HTTP status the standard assigns to what its refusal says, mirrored as the
// status times -100, the convention the gate's codes already follow (issue
// 956). A refusal that tells its caller it made too many requests is 429's
// -42900, and one that says the server cannot serve it right now is 503's
// -50300, so a client reads the same number for the same thing in-band and at
// the gate. What a refusal can say is read off its class: its code must mirror
// a status some gate refusal of the same class answers with, so a class
// carries one code for each thing it says and none the gate would not answer
// it with. JSON-RPC's own codes carry the protocol's meaning whichever answer
// they come with and are left out, and so is a row that records the departure.
func checkCodeMirrorsStatus(ds []Decision) []*ViolationError {
	statuses := map[Answer][]int{}
	for _, d := range ds {
		for _, r := range d.Refusals {
			if r.Channel == Gate && !has(statuses[r.Answer], r.Status) {
				statuses[r.Answer] = append(statuses[r.Answer], r.Status)
			}
		}
	}
	var out []*ViolationError
	for _, d := range ds {
		if d.RecordsDeparture("INV-012") {
			continue
		}
		for _, r := range d.Refusals {
			if r.Channel != RPC || standardJSONRPCCode(r.Code) {
				continue
			}
			if r.Code%100 == 0 && has(statuses[r.Answer], r.Code/-100) {
				continue
			}
			out = append(out, &ViolationError{ID: d.ID, Detail: fmt.Sprintf(
				"answers %q in-band with code %d, which mirrors no status the gate answers it with (%s)",
				r.Answer, r.Code, statusList(statuses[r.Answer]),
			)})
		}
	}
	return out
}

// statusList spells the statuses a class is answered with at the gate, in the
// order the register first declares them, or "none".
func statusList(statuses []int) string {
	if len(statuses) == 0 {
		return "none"
	}
	// fmt spells an []int as "[429 503]"; the register imports neither
	// strconv nor anything else that would spell one number at a time.
	return strings.ReplaceAll(strings.Trim(fmt.Sprint(statuses), "[]"), " ", ", ")
}

// checkUnique refuses two rows for one requirement.
func checkUnique(ds []Decision) []*ViolationError {
	seen := map[string]bool{}
	var out []*ViolationError
	for _, d := range ds {
		if seen[d.ID] {
			out = append(out, &ViolationError{ID: d.ID, Detail: "declared twice"})
		}
		seen[d.ID] = true
	}
	return out
}

// checkComplete holds the rows to the specification's requirements, one for
// one: a requirement with no row is a decision nobody declared, and a row with
// no requirement is a decision the specification does not know.
func checkComplete(ds []Decision) []*ViolationError {
	declared := map[string]bool{}
	for _, d := range ds {
		declared[d.ID] = true
	}
	required := map[string]bool{}
	var out []*ViolationError
	for _, id := range requirementIDs() {
		required[id] = true
		if !declared[id] {
			out = append(out, &ViolationError{ID: id, Detail: "a requirement with no row"})
		}
	}
	for _, d := range ds {
		if !required[d.ID] {
			out = append(out, &ViolationError{ID: d.ID, Detail: "a row the specification has no requirement for"})
		}
	}
	return out
}

// ValidateFailures checks the authentication failure table and returns every
// violation joined, or nil.
//
// A failure is charged only when the caller caused it (INV-007): a missing
// credential, or one GitLab refused. An upstream outage, an unanswered
// introspection, probe saturation, an under-scoped but genuine token, an
// unadmitted application or a misaddressed instance says nothing about the
// credential, and charging it would let a GitLab outage lock out callers
// holding valid tokens.
func ValidateFailures(fs []Failure) error {
	rows := map[string]bool{}
	for _, d := range Decisions() {
		rows[d.ID] = true
	}
	var errs []error
	charged := map[string]int{}
	declared := map[string]int{}
	var order []string
	for _, f := range fs {
		fn := f.At.Pkg + "." + f.At.Name
		id := fn + ":" + f.Kind
		violation := func(rule, detail string) {
			errs = append(errs, &ViolationError{ID: id, Rule: rule, Invariant: invNotJudgedNotCharged, Detail: detail})
		}
		if f.Charged && !f.Attributable {
			violation("charged-attributable", "charged although the caller did not cause it")
		}
		if !rows[f.Decision] {
			violation("failure-decision", fmt.Sprintf("its decision %s names no row", f.Decision))
		}
		if f.At.Role != Charge || f.At.Name == "" || f.At.Call == "" {
			violation("failure-site", "not returned by a charging function with a named charge helper")
		}
		if f.Status < 400 || f.Status > 599 {
			violation("failure-status", fmt.Sprintf("status %d is not a refusal", f.Status))
		}
		if _, ok := declared[fn]; !ok {
			declared[fn] = f.At.Count
			order = append(order, fn)
		}
		if f.Charged {
			charged[fn]++
		}
	}
	for _, fn := range order {
		if charged[fn] != declared[fn] {
			errs = append(errs, &ViolationError{
				ID: fn, Rule: "failure-count", Invariant: invNotJudgedNotCharged,
				Detail: fmt.Sprintf("declares %d charged failures and lists %d", declared[fn], charged[fn]),
			})
		}
	}
	return errors.Join(errs...)
}
