package finegrained

import (
	"fmt"
	"strings"
)

// classicWayOut is the way out every withheld answer offers besides a new
// grant. It is qualified for both enforcement forms GitLab has: on GitLab.com
// a top-level group can refuse classic tokens under it, and on a self-managed
// instance enforcement is instance-wide and blocks creating or rotating
// classic tokens after its date while existing ones keep working
// (doc/auth/tokens/fine_grained_access_tokens.md), so it never assumes either.
// It is one literal rather than two joined, since a constant expression has
// no statement a test can cover and a mutation tool reports its operator as
// not covered.
const classicWayOut = "(an existing one, on an instance that no longer lets you create them), where the group does not refuse classic tokens"

// notMissing closes every withheld answer: a model told an action is withheld
// reads it as a capability the server lacks unless it is told otherwise.
const notMissing = "Do not report the capability as missing."

// The stable texts a withheld answer carries right after the action it names,
// one per phase. They are what a client matches a refusal on, and what the
// tenant register declares as the refusal's prefix (AUT-007 for the first), so
// each is written once, as a constant the sentence is built around.
const (
	// withheldUnavailable follows the action ID when no fine-grained token can
	// reach the action at the recorded version (phase A).
	withheldUnavailable = "exists but is not available to a fine-grained personal access token"
	// withheldNotGranted follows the action ID when this token's grant does
	// not reach the action (phase B).
	withheldNotGranted = "exists but this fine-grained personal access token was not granted what it needs"
)

// WithheldText is the one sentence every surface answers a withheld call
// with, or "" for a decision whose call passes. It names the action by its
// canonical ID and never by a tool name, names permissions in the words the
// token creation page offers, and says what GitLab does to the answer the way
// the row's effect records it, so a read GitLab answers with null is not
// described as a refusal.
func (a *Authority) WithheldText(id string, decision Decision) string {
	if decision.Callable {
		return ""
	}
	version := a.table.DisplayVersion()
	if decision.Cause == CauseNotGranted {
		return a.notGrantedText(id, decision, version)
	}
	row := a.table.Requirement(id)
	reason := "GitLab " + version + " declares no fine-grained permission this action can be granted"
	if row != nil && row.Denied != nil {
		reason = deniedReason(*row.Denied, version)
	}
	return fmt.Sprintf("action %q %s: %s. "+
		"Use a classic personal access token with read_api for reads or api for writes %s.%s %s",
		id, withheldUnavailable, reason, classicWayOut, a.fallbackText(), notMissing)
}

// deniedReason says why no fine-grained token reaches an action, and what
// GitLab does to the answer.
func deniedReason(denial Denial, version string) string {
	gitlab := "GitLab " + version
	effect := effectPhrase(denial.Cause, denial.Effect)
	switch denial.Cause {
	case CauseMutationUndeclared:
		return fmt.Sprintf("%s declares no fine-grained permission on the GraphQL mutation %s this action sends, and %s",
			gitlab, denial.Element, effect)
	case CausePayloadUndeclared:
		return fmt.Sprintf("%s declares no fine-grained permission on the GraphQL type %s this action's write answers with, and %s",
			gitlab, denial.Element, effect)
	case CauseBoundaryUnresolvable:
		kind := "type"
		if denial.Effect == EffectRefused {
			kind = "mutation"
		}
		return fmt.Sprintf("%s declares the GraphQL %s %s at a boundary the object this action reaches never resolves to, and %s",
			gitlab, kind, denial.Element, effect)
	case CauseRESTTodo:
		return fmt.Sprintf("%s has deferred the fine-grained permission of the route %s this action calls, and %s",
			gitlab, denial.Element, effect)
	case CauseRESTUndeclared:
		return fmt.Sprintf("%s declares no fine-grained permission on the route %s this action calls, and %s",
			gitlab, denial.Element, effect)
	default:
		return fmt.Sprintf("%s declares no fine-grained permission on the GraphQL type %s this action reads, and %s",
			gitlab, denial.Element, effect)
	}
}

// effectPhrase says what GitLab does to the answer, from the row's effect.
func effectPhrase(cause Cause, effect Effect) string {
	switch effect {
	case EffectNullOrEmpty:
		return "answers such a read with null or an empty list"
	case EffectRemoved:
		return "removes the items from such a list"
	case EffectListNull:
		return "answers such a list with null"
	case EffectCommittedThenNull:
		return "commits such a write and answers null"
	case EffectRefused:
		if cause.GraphQL() {
			return "refuses such a write"
		}
		return "refuses such a request"
	default:
		return "answers such a read with null"
	}
}

// notGrantedText is the phase B answer: the groups the grant fails, named in
// the words a user grants them by.
func (a *Authority) notGrantedText(id string, decision Decision, version string) string {
	parts := make([]string, 0, len(decision.Missing))
	count := 0
	for _, index := range decision.Missing {
		words, perms := a.table.groupWords(index)
		count += perms
		parts = append(parts, words)
	}
	pronoun := "it"
	if count > 1 {
		pronoun = "them"
	}
	return fmt.Sprintf("action %q %s: %s, "+
		"as GitLab %s declares it. Create a fine-grained token that grants %s, or use a classic token with the api scope %s.%s %s",
		id, withheldNotGranted, strings.Join(parts, ", and "), version, pronoun, classicWayOut, a.fallbackText(), notMissing)
}

// groupWords names one group the way a refusal reads it, "the project
// permission [Merge Request: Approve]", and says how many permissions it
// named, so a sentence about several groups can choose its pronoun.
//
//gitlab:allow-unescaped strings.Join(names, ", "): the names are the table's, generated from GitLab's source at the recorded version and compiled into the binary rather than read from any instance, and a bracketed list followed by no link destination is not a link.
func (t *Table) groupWords(index uint32) (words string, count int) {
	group := t.Groups[index]
	names := make([]string, 0, len(group.Perms))
	for _, perm := range group.Perms {
		names = append(names, t.displayOf(perm))
	}
	noun := "permission"
	if len(names) > 1 {
		noun = "permissions"
	}
	return fmt.Sprintf("the %s %s [%s]", group.Any, noun, strings.Join(names, ", ")), len(names)
}

// displayOf names one raw permission the way the token creation page offers
// it, or by its raw name when no assignable a token can hold expands to it.
func (t *Table) displayOf(perm uint16) string {
	if display := t.Display[perm]; display != "" {
		return display
	}
	return t.Permissions[perm]
}

// fallbackText is the sentence a phase A answer appends to say why the grant
// was not evaluated, with its leading space, or "" when there is nothing to
// say.
func (a *Authority) fallbackText() string {
	const notEvaluated = ", so the grant was not evaluated."
	switch a.fallback {
	case FallbackGrantUnreadable:
		return " The token cannot read its own grant (a token created with Personal Access Token: Read can)" + notEvaluated
	case FallbackGrantTooLarge:
		return " The token's grant is larger than this server reads" + notEvaluated
	case FallbackGrantShape:
		return " The token's grant holds a scope this server cannot read without guessing" + notEvaluated
	case FallbackVersionUnreadable:
		return " The instance did not report a version this server can read (a token granted Metadata: Read lets it)" + notEvaluated
	case FallbackVersionOutside:
		return fmt.Sprintf(" The instance reports GitLab %s and the permissions are recorded for %s only%s",
			a.reported, a.table.Bucket, notEvaluated)
	case FallbackUnknownPermission:
		return fmt.Sprintf(" The token's grant names a permission GitLab %s does not define%s",
			a.table.DisplayVersion(), notEvaluated)
	default:
		return ""
	}
}
