// The payload half of validation: what each line type must carry to mean
// anything, checked before a reader hands it to anything that scores.
//
// record.go validates the envelope, which is schema, type, and exactly one
// payload. That is enough to say a line is well formed and not enough to say it
// is an observation. `{"schema":1,"type":"attempt","attempt":{}}` passes every
// envelope check and describes nothing: no case, no model, no session, no
// ending. A scorer given it counts an attempt that never happened, and the
// figure it lands in is a published one.
//
// So each payload says what it must carry. The bar is deliberately "what a
// reader has to have to place this line", not "what the writer happens to
// fill": a field the record's own doc comment marks optional is optional here,
// and only the fields a join or a published column reads are required. The
// enum values are checked against the constants rather than a copy of them, so
// a value added there without a thought about the reader fails here.
//
// Errors name the field in the JSON spelling a reader of a shard sees, since
// the thing in front of somebody debugging one is the line and not this file.

package modelrecord

import (
	"fmt"
	"slices"
	"strings"
)

// endings are the values [Attempt.EndedBy] may take.
var endings = []string{
	EndedCompleted, EndedOverBudget, EndedMalformed, EndedNoToolCall,
	EndedProviderError, EndedHarnessError, EndedSkipped,
}

// turnStatuses are the values [Turn.Status] may take.
var turnStatuses = []string{
	TurnOK, TurnRateLimited, TurnServerError, TurnRequestError, TurnTransportError,
}

// callOutcomes are the values [Call.Outcome] may take whole. A refusal is the
// sixth and is a prefix rather than a value, so it is matched separately.
var callOutcomes = []string{
	OutcomeOK, OutcomeToolError, OutcomeProtocolError, OutcomeTransportError, OutcomePreview,
}

// validatePayload checks the payload the line's type names.
//
// Only that one: the envelope check above has already established that exactly
// one payload is present and that it is the one the type names, so reaching
// into another here would be checking a field that cannot be set.
func (r Record) validatePayload() error {
	switch {
	case r.Run != nil:
		return r.Run.validate()
	case r.Session != nil:
		return r.Session.validate()
	case r.Attempt != nil:
		return r.Attempt.validate()
	case r.Turn != nil:
		return r.Turn.validate()
	case r.Call != nil:
		return r.Call.validate()
	case r.Verify != nil:
		return r.Verify.validate()
	}
	return nil
}

// required reports the first named field that is empty, so one check reads as
// one line rather than six.
func required(fields map[string]string, order ...string) error {
	for _, name := range order {
		if strings.TrimSpace(fields[name]) == "" {
			return fmt.Errorf("%s is empty", name)
		}
	}
	return nil
}

// positive reports a positional field that is not the 1-based index its own
// documentation promises. Zero is the value a field nobody wrote carries, which
// is exactly the case worth catching: it reads as a real position.
func positive(name string, value int) error {
	if value < 1 {
		return fmt.Errorf("%s is %d, want 1 or more", name, value)
	}
	return nil
}

// validate checks the run line: what a reader needs to place every other line
// of the shard beside it.
// The provenance fields are deliberately not required here, and the line
// between the two kinds is the one started_at taught: what a reader needs to
// place a line is checked at read time, and what a publication needs to be
// trustworthy is refused where it is published. edition, gitlab_version, tier,
// the two digests and the package are provenance;
// cmd/gen_model_results refuses a row whose provenance is short, by name, and
// says which field. Refusing them here would turn a shard that layer can
// report into one nothing can read.
func (r *Run) validate() error {
	if err := required(map[string]string{"run_id": r.RunID}, "run_id"); err != nil {
		return fmt.Errorf("run line: %w", err)
	}
	// started_at is deliberately not required. A run that never started leaves
	// it zero, and that hole is kept as a hole on purpose: the zero time would
	// render as the year one, which reads as a day somebody might have
	// measured on, and cmd/gen_model_results refuses the empty date by name in
	// its provenance rule. Refusing it here would take that refusal away from
	// the layer that owns it and make the shard unreadable instead of
	// reportable, which is the worse of the two.
	if err := positive("repeat", r.Repeat); err != nil {
		return fmt.Errorf("run line: %w", err)
	}
	return nil
}

// validate checks the session line: the shape the model was talking to.
// Only the label is required, for the same reason the run line requires only
// its identifier: the label is what every attempt line names, so a session
// without one is a line nothing can be joined to. Surface, mode, capabilities
// and the served count describe the session rather than identify it, and a
// publication that needs them says so where it needs them.
func (s *Session) validate() error {
	if err := required(map[string]string{"label": s.Label}, "label"); err != nil {
		return fmt.Errorf("session line: %w", err)
	}
	if s.ServedTools < 0 {
		return fmt.Errorf("session line: served_tools is %d", s.ServedTools)
	}
	if s.SliceSize < 0 {
		return fmt.Errorf("session line: slice_size is %d", s.SliceSize)
	}
	return nil
}

// validate checks the attempt line, which is the one this whole file exists
// for: it is what a published row counts.
func (a *Attempt) validate() error {
	// id, case and model are what a row is counted under and cannot be
	// inferred from anywhere else. surface is deliberately not among them: an
	// attempt line may leave it out and the key falls back to the session's
	// own, which is what lets a run whose attempts did not all share the
	// session's surface still be published (cmd/gen_model_results,
	// TestKeyOf_ReadsTheSurfaceTheScorerReads).
	if err := required(map[string]string{
		"id":    a.ID,
		"case":  a.Case,
		"model": a.Model,
	}, "id", "case", "model"); err != nil {
		return fmt.Errorf("attempt line: %w", err)
	}
	// session is required only of an attempt that reached one. Two endings
	// precede the session and cannot name it: a case whose needs the runtime
	// did not meet never opened one, and a harness error raised while opening
	// the session is written down as how far the attempt got. Requiring it of
	// those two would refuse exactly the lines that exist so a failure is not
	// lost, which is what the record was rebuilt for.
	if a.EndedBy != EndedSkipped && a.EndedBy != EndedHarnessError && strings.TrimSpace(a.Session) == "" {
		return fmt.Errorf("attempt line: session is empty on an attempt that ended %q", a.EndedBy)
	}
	if err := positive("repeat", a.Repeat); err != nil {
		return fmt.Errorf("attempt line: %w", err)
	}
	// An attempt shown a negative number of tools is the same class as a
	// negative token counter: a reader compares it with the session's budget
	// and with other attempts of the row, and a number below zero makes the
	// row's span reach somewhere no attempt was. Zero is left legal, because
	// it is what an attempt that never reached a session carries.
	if a.ShownTools < 0 {
		return fmt.Errorf("attempt line: shown_tools is %d", a.ShownTools)
	}
	if !slices.Contains(endings, a.EndedBy) {
		return fmt.Errorf("attempt line: ended_by is %q, and an attempt is scored by how it ended", a.EndedBy)
	}
	// The two endings whose meaning is carried by the reason rather than by the
	// value. A skip with no reason cannot be told from a case nobody wrote,
	// which is the confusion the record was rebuilt to remove; and a completed
	// attempt carrying a reason is a contradiction the record's own doc comment
	// rules out.
	if a.EndedBy == EndedSkipped && strings.TrimSpace(a.Reason) == "" {
		return fmt.Errorf("attempt line: ended_by is %q with no reason, so nothing says what the runtime did not meet", EndedSkipped)
	}
	if a.EndedBy == EndedCompleted && strings.TrimSpace(a.Reason) != "" {
		return fmt.Errorf("attempt line: ended_by is %q and carries a reason %q", EndedCompleted, a.Reason)
	}
	return nil
}

// validate checks a turn line: one provider request and its answer.
func (t *Turn) validate() error {
	if err := required(map[string]string{"attempt": t.Attempt}, "attempt"); err != nil {
		return fmt.Errorf("turn line: %w", err)
	}
	if err := positive("index", t.Index); err != nil {
		return fmt.Errorf("turn line: %w", err)
	}
	if err := positive("try", t.Try); err != nil {
		return fmt.Errorf("turn line: %w", err)
	}
	if !slices.Contains(turnStatuses, t.Status) {
		return fmt.Errorf("turn line: status is %q", t.Status)
	}
	// A negative counter is worse than a wrong one, because both places that
	// read these numbers add them up: costOf multiplies each by its price into
	// what the run has spent, and tokensOf sums them into the published totals.
	// One negative value therefore makes a run look cheaper than it was and a
	// model look more frugal than it was, and the spend is what stops a paid
	// run. The values come from a provider's own decoded response, so this is
	// the boundary where they stop being somebody else's number.
	for _, counter := range []struct {
		name  string
		value int
	}{
		{"usage.input", t.Usage.Input},
		{"usage.output", t.Usage.Output},
		{"usage.cache_created", t.Usage.CacheCreated},
		{"usage.cache_read", t.Usage.CacheRead},
	} {
		if counter.value < 0 {
			return fmt.Errorf("turn line: %s is %d", counter.name, counter.value)
		}
	}
	for index, block := range t.Blocks {
		switch block.Kind {
		case BlockText, BlockThinking, BlockToolCall:
		default:
			return fmt.Errorf("turn line: block %d has kind %q", index+1, block.Kind)
		}
	}
	return nil
}

// validate checks a call line: one tools/call and what the server did with it.
func (c *Call) validate() error {
	if err := required(map[string]string{
		"attempt": c.Attempt,
		"tool":    c.Tool,
	}, "attempt", "tool"); err != nil {
		return fmt.Errorf("call line: %w", err)
	}
	if err := positive("turn", c.Turn); err != nil {
		return fmt.Errorf("call line: %w", err)
	}
	if err := positive("index", c.Index); err != nil {
		return fmt.Errorf("call line: %w", err)
	}
	// A refusal spells its reason after the prefix, so it is matched as one
	// rather than listed: the reasons are the server's and this record does not
	// own them.
	if !slices.Contains(callOutcomes, c.Outcome) && !strings.HasPrefix(c.Outcome, OutcomeRefusedPrefix) {
		return fmt.Errorf("call line: outcome is %q", c.Outcome)
	}
	if strings.HasPrefix(c.Outcome, OutcomeRefusedPrefix) &&
		strings.TrimSpace(strings.TrimPrefix(c.Outcome, OutcomeRefusedPrefix)) == "" {
		return fmt.Errorf("call line: outcome is %q with no reason after the prefix", c.Outcome)
	}
	return nil
}

// validate checks a verify line: one recipe's post-run check.
func (v *Verify) validate() error {
	if err := required(map[string]string{
		"attempt": v.Attempt,
		"name":    v.Name,
	}, "attempt", "name"); err != nil {
		return fmt.Errorf("verify line: %w", err)
	}
	// A failed check with nothing said about it is a row a reader cannot act
	// on, and the detail is where the recipe says what it found.
	if !v.Passed && strings.TrimSpace(v.Detail) == "" {
		return fmt.Errorf("verify line: %q did not pass and carries no detail", v.Name)
	}
	return nil
}
