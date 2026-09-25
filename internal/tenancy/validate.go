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

// Validate checks the rows against the invariants of spec section 3.3 and
// returns every violation joined, or nil.
//
// It is what answers, before review, the questions issues 540 and 561 each
// answered by hand: a share on a key a caller can mint is refused with the
// key's evidence in the message, a per-caller ceiling on a process resource
// with no process partner is refused, and so is a refusal on a channel the
// method cannot carry. A row that breaks an invariant today passes only
// through a finding recorded for that invariant (see [Finding.Invariants]).
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

// rowRules are the checks made one row at a time, in the order the design
// lists them.
func rowRules() []rowRule {
	return []rowRule{
		{"share-on-mintable", "INV-003", checkShareOnMintable},
		{"process-partner", "INV-004", checkProcessPartner},
		{"process-absent", "INV-018", checkProcessAbsent},
		{"across-keys", "INV-005", checkAcrossKeys},
		{"channel-carried", "INV-011", checkChannelCarried},
		{"zero-stated", "INV-015", checkZeroStated},
		{"units-agree", "INV-016", checkUnitsAgree},
		{"stated-unit", "INV-016", checkStatedUnit},
		{"class-consistent", "spec 2.3", checkClassConsistent},
		{"through-config", "INV-017", checkThroughConfig},
		{"capacity-stated", "VAL-006", checkCapacityStated},
		{"bounded-table", "INV-010", checkBoundedTable},
		{"charged-exists", "INV-007", checkChargedExists},
		{"stdio", "VAL-011", checkStdio},
		{"well-formed", "spec 3.2", checkWellFormed},
	}
}

// setRules are the checks made over the register as a whole.
func setRules() []setRule {
	return []setRule{
		{"one-code-per-answer", "INV-012", checkOneCodePerAnswer},
		{"unique", "spec 3.2", checkUnique},
		{"complete", "AC-002", checkComplete},
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
func checkProcessPartner(d Decision, rows map[string]Decision) []string {
	var out []string
	if d.ProtectsProcess && d.Key.Axis() == AxisRequester && d.Partner == "" && !d.RecordsDeparture("INV-004") {
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
	if d.Kind != Ceiling || d.Source != SourceNone || d.RecordsDeparture("INV-018") {
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
		out = append(out, refusalCarriage(&d.Refusals[i], d.RecordsDeparture("INV-011"))...)
	}
	return out
}

// refusalCarriage is [checkChannelCarried] for one refusal.
func refusalCarriage(r *Refusal, legacyCodeRecorded bool) []string {
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
	return append(out, codeCarriage(r.Code, legacyCodeRecorded)...)
}

// codeCarriage is what MCP and the SDK allow of an error's code.
func codeCarriage(code int, legacyCodeRecorded bool) []string {
	switch {
	case code == 0:
		return []string{"an error with no code reaches the client as code 0"}
	case code == codeMethodNotFound:
		return []string{"-32601's message is replaced by the SDK, so it cannot explain a refusal"}
	case code <= -32020 && code >= -32099 && !mcpDefinesCode(code):
		return []string{fmt.Sprintf("code %d is in the range MCP reserves and does not define", code)}
	case code <= -32000 && code >= -32019 && !legacyCodeRecorded:
		return []string{fmt.Sprintf("code %d is in the legacy range new implementations should not use", code)}
	default:
		return nil
	}
}

// checkZeroStated holds a valued decision to saying what zero means, and a
// zero that is not "off" to a finding.
func checkZeroStated(d Decision, _ map[string]Decision) []string {
	var out []string
	if d.Disposition == Valued && d.Zero == ZeroUnset {
		out = append(out, "a valued decision that does not say what zero means")
	}
	departs := d.Zero == ZeroRefused || d.Zero == ZeroSelectsDefault || d.OffWith != ""
	if departs && !d.RecordsDeparture("INV-015") {
		out = append(out, "a zero that does not mean off, with no finding")
	}
	return out
}

// checkUnitsAgree holds a class D disagreement to a finding.
func checkUnitsAgree(d Decision, _ map[string]Decision) []string {
	if !d.Disagrees() || d.RecordsDeparture("INV-016") {
		return nil
	}
	return []string{fmt.Sprintf("its reason is about the %s and its key is the %s, with no finding", d.ReasonUnit, d.Key)}
}

// checkStatedUnit holds a reason that misstates the unit of what it cites to
// a finding.
func checkStatedUnit(d Decision, _ map[string]Decision) []string {
	if d.StatedUnit == KeyNone || d.StatedUnit.Unit() == d.ReasonUnit.Unit() || d.RecordsDeparture("INV-016") {
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
// variable or a Go option reaches to a finding.
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
	case EnvOnly, OptionOnly:
		if !d.RecordsDeparture("INV-017") {
			out = append(out, "configurable outside the configuration package, with no finding")
		}
	}
	return out
}

// checkCapacityStated holds a ceiling to saying what happens when it is full.
func checkCapacityStated(d Decision, _ map[string]Decision) []string {
	if d.Kind != Ceiling || d.Source == SourceNone || d.AtCapacity != CapacityNone {
		return nil
	}
	return []string{"a ceiling that does not say what happens when it is full"}
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
// per-caller key collapses onto the process (TEN-005).
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

// danglingReferences is the half of [checkWellFormed] that follows a row's
// references: a partner and a zero partner must be rows, and a finding must
// be one of the specification's.
func danglingReferences(d Decision, rows map[string]Decision) []string {
	var out []string
	if _, ok := rows[d.Partner]; d.Partner != "" && !ok {
		out = append(out, fmt.Sprintf("its partner %s names no row", d.Partner))
	}
	if _, ok := rows[d.OffWith]; d.OffWith != "" && !ok {
		out = append(out, fmt.Sprintf("its zero follows %s, which names no row", d.OffWith))
	}
	for _, id := range d.Findings {
		if FindingIssue(id) == 0 {
			out = append(out, fmt.Sprintf("it carries %s, which is not a finding", id))
		}
	}
	return out
}

// checkOneCodePerAnswer holds each class of answer to one in-band code across
// the rows, so a client learns one number for "retry later". It compares the
// codes this server allocates; JSON-RPC's own codes carry the protocol's
// meaning whichever answer they come with. A row that records the divergence
// is left out of the comparison.
func checkOneCodePerAnswer(ds []Decision) []*ViolationError {
	type first struct {
		code int
		id   string
	}
	seen := map[Answer]first{}
	var out []*ViolationError
	for _, d := range ds {
		if d.RecordsDeparture("INV-012") {
			continue
		}
		for _, r := range d.Refusals {
			if r.Channel != RPC || standardJSONRPCCode(r.Code) {
				continue
			}
			f, ok := seen[r.Answer]
			if !ok {
				seen[r.Answer] = first{code: r.Code, id: d.ID}
				continue
			}
			if f.code != r.Code {
				out = append(out, &ViolationError{ID: d.ID, Detail: fmt.Sprintf(
					"answers %q in-band with code %d where %s uses %d", r.Answer, r.Code, f.id, f.code,
				)})
			}
		}
	}
	return out
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
			errs = append(errs, &ViolationError{ID: id, Rule: rule, Invariant: "INV-007", Detail: detail})
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
				ID: fn, Rule: "failure-count", Invariant: "INV-007",
				Detail: fmt.Sprintf("declares %d charged failures and lists %d", declared[fn], charged[fn]),
			})
		}
	}
	return errors.Join(errs...)
}
