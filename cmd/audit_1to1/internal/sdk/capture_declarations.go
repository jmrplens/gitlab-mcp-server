// Copyright (c) 2026 José M. Requena Plens
// SPDX-License-Identifier: MIT

package sdk

import (
	"slices"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/cmd/audit_1to1/internal/waits"
)

// The reasons a key both a captured struct and the client-go struct beside
// it read is kept in the capture. Each names what has to stay true of the
// SDK for the reason to hold, which a predicate of its declaration says, and
// the predicate it requires is what [captureCategories] checks.
const (
	// keptForNull: the SDK decodes the key into a number or a string that is
	// not a pointer, so a null GitLab sends reads as zero, which the capture,
	// a pointer, tells apart.
	keptForNull = "kept-for-null"
	// keptForPresence: the SDK decodes the key into a value that is not a
	// pointer, so a key GitLab leaves out reads as zero, which the capture,
	// a pointer, tells apart.
	keptForPresence = "kept-for-presence"
	// keptForValue: the SDK decodes the key into a field whose value the
	// converters are not to read: a type that cannot hold what GitLab sends,
	// or a field client-go marks deprecated, which decodes the same value
	// but would need a lint exemption at each converter that read it.
	keptForValue = "kept-for-value"
	// keptForSiblingPairing: the captured struct is paired with another
	// client-go struct that does not model the key, so the capture is where
	// that pairing reads it from.
	keptForSiblingPairing = "kept-for-sibling-pairing"
	// keptWithItsObject: the key is read inside an object the capture reads
	// whole, beside a key of the same object the SDK's struct does not
	// model, so reading it off the SDK would split one object between two
	// decodes. It is written on the object's key and answers every key
	// below it.
	keptWithItsObject = "kept-with-its-object"
)

// captureDeclaration answers a key a captured struct reads that a client-go
// struct it is paired with models too.
type captureDeclaration struct {
	// Type is the captured struct, spelled with its package's name, and
	// Key the key, dotted below each key both read as an object. A
	// declaration of a category that answers an object is written on the
	// object's key and answers every key below it.
	Type, Key string
	// Category is one of the kept-for categories.
	Category string
	// Predicates are what is true of the pinned client-go while the key is
	// read twice for the declared reason. The category says which one has
	// to be among them. Some of them failing leaves a predicate to edit,
	// since the key is still read twice for the rest; none of them holding
	// is the SDK now reading the key the way the capture does, so the
	// capture is to retire.
	Predicates []waits.Predicate
	Reason     string
}

// id names the key a declaration is written on.
func (d captureDeclaration) id() string { return d.Type + "." + d.Key }

// captureJudging is what holding the declarations to the shared keys needs.
type captureJudging struct {
	sdk waits.SDK
	// keys indexes every shared key by its type and key.
	keys map[string]*capturedKey
	// pairs are the client-go structs each captured type is paired with,
	// and lacks the keys it reads that the client-go struct at their depth
	// does not model, both by the type's spelling.
	pairs map[string][]string
	lacks map[string][]waits.Lack
}

// captureCategory is what a category requires of a declaration: whether it
// answers an object, and the predicate it has to carry, checked against the
// keys it answers.
type captureCategory struct {
	object  bool
	require func(in captureJudging, declaration captureDeclaration, answered []*capturedKey) bool
	// lacks says what the declaration needs when require is false.
	lacks string
}

// captureCategories are the categories a capture declaration takes.
var captureCategories = map[string]captureCategory{
	keptForNull: {
		require: func(_ captureJudging, declaration captureDeclaration, answered []*capturedKey) bool {
			return answered[0].pointer() && hasFieldKind(declaration.Predicates, answered[0], isNumberOrString)
		},
		lacks: "a capture read as a pointer and a FieldKind holding the field that models the key to a number or a string that is not a pointer",
	},
	keptForPresence: {
		require: func(_ captureJudging, declaration captureDeclaration, answered []*capturedKey) bool {
			return answered[0].pointer() && hasFieldKind(declaration.Predicates, answered[0], isNotPointer)
		},
		lacks: "a capture read as a pointer and a FieldKind holding the field that models the key to a type that is not a pointer",
	},
	keptForValue: {
		require: func(_ captureJudging, declaration captureDeclaration, answered []*capturedKey) bool {
			return hasFieldKind(declaration.Predicates, answered[0], func(string) bool { return true })
		},
		lacks: "a FieldKind holding the field that models the key to its type",
	},
	keptForSiblingPairing: {
		object:  true,
		require: hasSiblingLack,
		lacks:   "a LacksKey, on the key or an object above it, on a client-go struct the type is paired with that does not model the key",
	},
	keptWithItsObject: {
		object:  true,
		require: hasObjectLack,
		lacks:   "to be written on an object above the keys it answers, and a LacksKey on a client-go struct below it naming a key the captured object reads there",
	},
}

// captureDeclarations answers every key the captures share with the
// client-go struct beside them on the pinned SDK.
var captureDeclarations = slices.Concat(trailerCaptures, nullCaptures, presenceCaptures, valueCaptures, siblingCaptures, objectCaptures)

// trailerReason is why a commit's extended_trailers is read off the capture.
const trailerReason = "client-go's Commit declares extended_trailers a map of strings where GitLab sends each trailer's list of values, so its decoder fails on a commit carrying one (register row 69)"

// trailerKind is the type client-go declares extended_trailers with.
var trailerKind = waits.FieldKind{Type: "Commit", Field: "ExtendedTrailers", Kind: "map[string]string"}

// trailerCaptures are the commits read whole off a capture for their
// extended_trailers, alone or inside the answer that carries them.
var trailerCaptures = []captureDeclaration{
	{Type: "branches.capturedBranch", Key: "commit.extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
	{Type: "commits.Captured", Key: "extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
	{Type: "mrchanges.capturedDiffVersion", Key: "commits.extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
	{Type: "mrcontextcommits.capturedCommit", Key: "extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
	{Type: "repository.capturedCompare", Key: "commit.extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
	{Type: "repository.capturedCompare", Key: "commits.extended_trailers", Category: keptForValue, Predicates: []waits.Predicate{trailerKind}, Reason: trailerReason},
}

// measureReason is why a measure GitLab sends as null is read off the
// capture.
const measureReason = "GitLab sends null for a measure that was never taken, which client-go decodes as 0, a measure of zero (register row 116)"

// nullCapture declares a key kept for its null, with a FieldKind per field
// client-go models it with.
func nullCapture(typeName, key, reason string, kinds ...waits.FieldKind) captureDeclaration {
	predicates := make([]waits.Predicate, 0, len(kinds))
	for _, kind := range kinds {
		predicates = append(predicates, kind)
	}
	return captureDeclaration{Type: typeName, Key: key, Category: keptForNull, Predicates: predicates, Reason: reason}
}

// nullCaptures are the keys GitLab sends as null where client-go decodes a
// number that is not a pointer.
var nullCaptures = []captureDeclaration{
	nullCapture("dorametrics.capturedMetric", "value", "GitLab sends null for a data point with no rows, which client-go decodes as 0, the best score a lead time can have (register row 116)",
		waits.FieldKind{Type: "DORAMetric", Field: "Value", Kind: "float64"}),
	nullCapture("mergetrains.trainMeasures", "duration", measureReason, waits.FieldKind{Type: "MergeTrain", Field: "Duration", Kind: "int64"}),
	nullCapture("mergetrains.trainMeasures", "pipeline.duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "Duration", Kind: "int64"}),
	nullCapture("mergetrains.trainMeasures", "pipeline.queued_duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "QueuedDuration", Kind: "int64"}),
	nullCapture("resourceevents.capturedWeight", "weight", "GitLab sends null for an event that took the weight away, which client-go decodes as a weight of 0 (register row 116)",
		waits.FieldKind{Type: "WeightEvent", Field: "Weight", Kind: "int64"}),
	nullCapture("toolutil.BoardExtra", "weight", "GitLab sends null for a board with no weight scope, which client-go decodes as a board scoped to weight 0 (register row 116)",
		waits.FieldKind{Type: "IssueBoard", Field: "Weight", Kind: "int64"}),
	nullCapture("toolutil.BridgeExtra", "coverage", measureReason,
		waits.FieldKind{Type: "Bridge", Field: "Coverage", Kind: "float64"}, waits.FieldKind{Type: "TriggerJob", Field: "Coverage", Kind: "float64"}),
	nullCapture("toolutil.BridgeExtra", "duration", measureReason,
		waits.FieldKind{Type: "Bridge", Field: "Duration", Kind: "float64"}, waits.FieldKind{Type: "TriggerJob", Field: "Duration", Kind: "float64"}),
	nullCapture("toolutil.BridgeExtra", "queued_duration", measureReason,
		waits.FieldKind{Type: "Bridge", Field: "QueuedDuration", Kind: "float64"}, waits.FieldKind{Type: "TriggerJob", Field: "QueuedDuration", Kind: "float64"}),
	nullCapture("toolutil.JobMeasures", "coverage", measureReason, waits.FieldKind{Type: "Job", Field: "Coverage", Kind: "float64"}),
	nullCapture("toolutil.JobMeasures", "duration", measureReason, waits.FieldKind{Type: "Job", Field: "Duration", Kind: "float64"}),
	nullCapture("toolutil.JobMeasures", "queued_duration", measureReason, waits.FieldKind{Type: "Job", Field: "QueuedDuration", Kind: "float64"}),
	nullCapture("toolutil.MergeRequestExtra", "head_pipeline.duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "Duration", Kind: "int64"}),
	nullCapture("toolutil.MergeRequestExtra", "head_pipeline.queued_duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "QueuedDuration", Kind: "int64"}),
	nullCapture("toolutil.NamespaceExtra", "additional_purchased_storage_size", namespaceLimitReason,
		waits.FieldKind{Type: "Namespace", Field: "AdditionalPurchasedStorageSize", Kind: "int64"}),
	nullCapture("toolutil.NamespaceExtra", "extra_shared_runners_minutes_limit", namespaceLimitReason,
		waits.FieldKind{Type: "Namespace", Field: "ExtraSharedRunnersMinutesLimit", Kind: "int64"}),
	nullCapture("toolutil.NamespaceExtra", "shared_runners_minutes_limit", namespaceLimitReason,
		waits.FieldKind{Type: "Namespace", Field: "SharedRunnersMinutesLimit", Kind: "int64"}),
	nullCapture("toolutil.PipelineExtra", "duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "Duration", Kind: "int64"}),
	nullCapture("toolutil.PipelineExtra", "queued_duration", measureReason, waits.FieldKind{Type: "Pipeline", Field: "QueuedDuration", Kind: "int64"}),
}

// namespaceLimitReason is why a namespace's limits are read off the capture.
const namespaceLimitReason = "GitLab sends null for a limit that is not set, which client-go decodes as a limit of 0, so no limit would read as limited to zero"

// approvalStateReason is why a scalar of the Enterprise Edition approval
// state is read off the capture.
const approvalStateReason = "only an Enterprise Edition instance sends the approval state, and client-go decodes the key a Community Edition answer leaves out as zero, a value an Enterprise answer also carries"

// presenceCapture declares a key of the Enterprise Edition approval state
// kept for its presence.
func presenceCapture(key, field, kind string) captureDeclaration {
	return captureDeclaration{
		Type: "mrapprovals.enterpriseApprovalCapture", Key: key, Category: keptForPresence,
		Predicates: []waits.Predicate{waits.FieldKind{Type: "MergeRequestApprovals", Field: field, Kind: kind}},
		Reason:     approvalStateReason,
	}
}

// presenceCaptures are the keys a Community Edition answer leaves out where
// client-go decodes a value that is not a pointer.
var presenceCaptures = []captureDeclaration{
	presenceCapture("approvals_left", "ApprovalsLeft", "int64"),
	presenceCapture("approvals_required", "ApprovalsRequired", "int64"),
	presenceCapture("description", "Description", "string"),
	presenceCapture("has_approval_rules", "HasApprovalRules", "bool"),
	presenceCapture("id", "ID", "int64"),
	presenceCapture("iid", "IID", "int64"),
	presenceCapture("merge_request_approvers_available", "MergeRequestApproversAvailable", "bool"),
	presenceCapture("merge_status", "MergeStatus", "string"),
	presenceCapture("multiple_approval_rules_available", "MultipleApprovalRulesAvailable", "bool"),
	presenceCapture("project_id", "ProjectID", "int64"),
	presenceCapture("require_password_to_approve", "RequirePasswordToApprove", "bool"),
	presenceCapture("state", "State", "string"),
	presenceCapture("title", "Title", "string"),
}

// confidentialReason is why a note's confidential is read off the capture.
const confidentialReason = "client-go decodes confidential, the same key with the same value, into a field it marks deprecated for internal; the converters once disagreed on whether to read that field or to mirror internal, and read the capture so that every one reads GitLab's value with no SA1019 exemption"

// confidentialKind is the deprecated field client-go decodes confidential
// into.
var confidentialKind = waits.FieldKind{Type: "Note", Field: "Confidential", Kind: "bool"}

// valueCaptures are the keys client-go decodes into a field the converters
// are not to read.
var valueCaptures = []captureDeclaration{
	{Type: "toolutil.DiscussionExtra", Key: "notes.confidential", Category: keptForValue, Predicates: []waits.Predicate{confidentialKind}, Reason: confidentialReason},
	{Type: "toolutil.NoteExtra", Key: "confidential", Category: keptForValue, Predicates: []waits.Predicate{confidentialKind}, Reason: confidentialReason},
}

// siblingCaptures are the keys one client-go struct a captured type is paired
// with models and another does not.
var siblingCaptures = []captureDeclaration{
	{
		Type: "toolutil.MemberExtra", Key: "group_saml_identity", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "ProjectMember", Key: "group_saml_identity"}},
		Reason:     "client-go's GroupMember models the SAML identity and ProjectMember does not, so a project member reads it from the capture",
	},
	{
		Type: "toolutil.MemberExtra", Key: "public_email", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "ProjectMember", Key: "public_email"}},
		Reason:     "client-go's GroupMember models the public email and ProjectMember does not, so a project member reads it from the capture",
	},
	{
		Type: "toolutil.MergeRequestExtra", Key: "work_in_progress", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "BasicMergeRequest", Key: "work_in_progress"}},
		Reason:     "client-go's MergeRequest models work_in_progress and BasicMergeRequest, which every list decodes into, does not",
	},
	{
		Type: "toolutil.RunnerExtra", Key: "token_expires_at", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "RunnerDetails", Key: "token_expires_at"}},
		Reason:     "client-go's Runner models token_expires_at and RunnerDetails, which a runner's details decode into, does not (register row 133)",
	},
	{
		Type: "toolutil.ServiceAccountExtra", Key: "public_email", Category: keptForSiblingPairing,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "ProjectServiceAccount", Key: "public_email"}},
		Reason:     "client-go's GroupServiceAccount models the public email and ProjectServiceAccount does not, so a project's account reads it from the capture",
	},
}

// userBasicReason is why a list of users is read whole off the capture.
const userBasicReason = "each user is GitLab's UserBasic, whose public_email and locked client-go's BasicUser does not model, so the list is read whole"

// objectCaptures are the objects read whole off the capture for a key the
// client-go struct inside them does not model.
var objectCaptures = []captureDeclaration{
	{
		Type: "mergerequests.approvalsExtra", Key: "approved_by", Category: keptWithItsObject,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "BasicUser", Key: "public_email"}, waits.LacksKey{Type: "BasicUser", Key: "locked"}},
		Reason:     "each approval's user is GitLab's UserBasic, whose public_email and locked client-go's BasicUser does not model, so each approval is read whole with the time it was given",
	},
	{
		Type: "mrapprovals.enterpriseApprovalCapture", Key: "approver_groups", Category: keptWithItsObject,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "MergeRequestApproverNestedGroup", Key: "parent_id"}},
		Reason:     "each group carries the parent_id client-go's MergeRequestApproverNestedGroup does not model, so the list is read whole",
	},
	{
		Type: "mrapprovals.enterpriseApprovalCapture", Key: "approvers", Category: keptWithItsObject,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "BasicUser", Key: "public_email"}, waits.LacksKey{Type: "BasicUser", Key: "locked"}},
		Reason:     userBasicReason,
	},
	{
		Type: "mrapprovals.enterpriseApprovalCapture", Key: "suggested_approvers", Category: keptWithItsObject,
		Predicates: []waits.Predicate{waits.LacksKey{Type: "BasicUser", Key: "public_email"}, waits.LacksKey{Type: "BasicUser", Key: "locked"}},
		Reason:     userBasicReason,
	},
}

// judgeCaptures answers each shared key of keys with its declaration, and
// returns what is wrong with the declarations: one that answers no shared
// key, two that answer one key, one of a category the table does not define,
// and one that lacks what its category requires.
func judgeCaptures(in captureJudging, keys []capturedKey, declared []captureDeclaration) []string {
	var stale []string
	for _, declaration := range declared {
		label := "capture declaration " + declaration.id()
		category, defined := captureCategories[declaration.Category]
		if !defined {
			stale = append(stale, label+": category "+strconv.Quote(declaration.Category)+" is not one the table defines")
			continue
		}
		answered := answeredKeys(keys, declaration, category.object)
		if len(answered) == 0 {
			stale = append(stale, label+" answers no key a capture shares with the client-go struct beside it")
			continue
		}
		verdicts, status := evaluateAll(declaration.Predicates, in.sdk)
		for _, key := range answered {
			if key.Declared != "" {
				stale = append(stale, label+" answers "+key.Key+", which the declaration on "+key.Declared+" answers too")
				continue
			}
			key.Declared, key.Category, key.Predicates, key.Reason = declaration.Key, declaration.Category, verdicts, declaration.Reason
			key.Status = status
		}
		if !category.require(in, declaration, answered) {
			stale = append(stale, label+": "+declaration.Category+" needs "+category.lacks)
		}
	}
	slices.Sort(stale)
	return stale
}

// answeredKeys returns the shared keys declaration answers: the one it is
// written on, and, for a category that answers an object, every key below
// it.
func answeredKeys(keys []capturedKey, declaration captureDeclaration, object bool) []*capturedKey {
	var out []*capturedKey
	for i := range keys {
		key := &keys[i]
		if key.Type != declaration.Type {
			continue
		}
		if key.Key == declaration.Key || (object && strings.HasPrefix(key.Key, declaration.Key+".")) {
			out = append(out, key)
		}
	}
	return out
}

// evaluateAll evaluates every predicate, and returns what the keys a
// declaration answers come to: kept while all hold, a predicate to edit while
// some do, and a capture to retire once none does.
func evaluateAll(predicates []waits.Predicate, sdk waits.SDK) (verdicts []waits.Verdict, status string) {
	verdicts = make([]waits.Verdict, 0, len(predicates))
	holding := 0
	for _, predicate := range predicates {
		verdict := waits.Evaluate(predicate, sdk)
		verdicts = append(verdicts, verdict)
		if verdict.Holds {
			holding++
		}
	}
	if holding == len(predicates) {
		return verdicts, captureKept
	}
	if holding > 0 {
		return verdicts, captureStalePredicate
	}
	return verdicts, captureObsolete
}

// pointer reports whether the capture reads the key as a pointer, which is
// what tells a null, or a key not sent, from a zero.
func (k *capturedKey) pointer() bool { return strings.HasPrefix(k.Captured, "*") }

// hasFieldKind reports whether, for every client-go struct that models key, a
// FieldKind among predicates names the field it models key with, spelled as
// kind accepts: a key two structs model is read twice for a reason each of
// them gives.
func hasFieldKind(predicates []waits.Predicate, key *capturedKey, kind func(string) bool) bool {
	named := map[waits.FieldKind]bool{}
	for _, predicate := range predicates {
		if fieldKind, isFieldKind := predicate.(waits.FieldKind); isFieldKind && kind(fieldKind.Kind) {
			named[waits.FieldKind{Type: fieldKind.Type, Field: fieldKind.Field}] = true
		}
	}
	for _, model := range key.Models {
		if !named[waits.FieldKind{Type: model.Owner, Field: model.Field}] {
			return false
		}
	}
	return true
}

// hasSiblingLack reports whether a LacksKey of declaration names the key it
// is written on, or an object above it, on a client-go struct the captured
// type is paired with and none of the keys it answers is modeled by.
func hasSiblingLack(in captureJudging, declaration captureDeclaration, answered []*capturedKey) bool {
	modeledBy := map[string]bool{}
	for _, key := range answered {
		for _, pair := range key.Pairs {
			modeledBy[pair] = true
		}
	}
	for _, predicate := range declaration.Predicates {
		lack, isLack := predicate.(waits.LacksKey)
		if isLack && dottedPrefix(lack.Key, declaration.Key) && slices.Contains(in.pairs[declaration.Type], lack.Type) && !modeledBy[lack.Type] {
			return true
		}
	}
	return false
}

// hasObjectLack reports whether declaration is written on an object above
// every key it answers, and a LacksKey of it names a key the captured type
// reads below that object on the client-go struct that does not model it.
func hasObjectLack(in captureJudging, declaration captureDeclaration, answered []*capturedKey) bool {
	for _, key := range answered {
		if key.Key == declaration.Key {
			return false
		}
	}
	for _, predicate := range declaration.Predicates {
		lack, isLack := predicate.(waits.LacksKey)
		if !isLack {
			continue
		}
		for _, read := range in.lacks[declaration.Type] {
			if strings.HasPrefix(read.Key, declaration.Key+".") && read.Owner == lack.Type && read.Name == lack.Key {
				return true
			}
		}
	}
	return false
}

// dottedPrefix reports whether prefix is key, or a key of an object above it.
func dottedPrefix(prefix, key string) bool {
	return key == prefix || strings.HasPrefix(key, prefix+".")
}

// numberOrString are the kinds, none a pointer, that a null reads as zero
// in.
var numberOrString = map[string]bool{
	"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
	"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
	"float32": true, "float64": true, "string": true,
}

// isNumberOrString reports whether kind spells a number or a string.
func isNumberOrString(kind string) bool { return numberOrString[kind] }

// isNotPointer reports whether kind spells a type that is not a pointer.
func isNotPointer(kind string) bool { return !strings.HasPrefix(kind, "*") }
