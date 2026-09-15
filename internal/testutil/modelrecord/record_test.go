package modelrecord

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"
)

// TestRefusedOutcome_CarriesThePrefixAndTheReason verifies that a refusal is
// spelled as the prefix a scorer classifies on followed by the reason the
// server gave.
//
// It matters because a mutating step declined on a read-only surface is
// recognized by that prefix and the reason after it, and declining correctly is
// the whole of what a protective-mode row measures. A second hand-written
// concatenation anywhere would be how the writer and the scorer come to
// disagree about what a refusal looks like, so there is one function and this
// test pins what it produces.
func TestRefusedOutcome_CarriesThePrefixAndTheReason(t *testing.T) {
	got := RefusedOutcome("unknown_action")

	if want := "refused:unknown_action"; got != want {
		t.Errorf("RefusedOutcome = %q, want %q", got, want)
	}
	if !strings.HasPrefix(got, OutcomeRefusedPrefix) {
		t.Errorf("RefusedOutcome = %q, want it to start with %q", got, OutcomeRefusedPrefix)
	}
}

// TestCapResult_Cases verifies what the result cap keeps, what it replaces and
// what it reports.
//
// The boundary is the whole of it. A result that fits must come back byte for
// byte, or a later step's binding into it would be checked against something
// the server did not say; a result that does not must come back as valid JSON,
// or the one long answer in a run would make its shard unreadable and take
// every attempt beside it down; and the flag is what a scorer reads, so a
// truncation that reported nothing would look like a value the model got wrong.
func TestCapResult_Cases(t *testing.T) {
	// A JSON string of n bytes of content is n+2 bytes of document, so the
	// payload is sized against the cap rather than the quotes.
	fits := json.RawMessage(`"` + strings.Repeat("a", MaxResultBytes-2) + `"`)
	over := json.RawMessage(`"` + strings.Repeat("a", MaxResultBytes) + `"`)

	cases := []struct {
		name      string
		result    json.RawMessage
		want      json.RawMessage
		truncated bool
	}{
		{name: "nothing at all", result: nil, want: nil},
		{name: "a small object", result: json.RawMessage(`{"id":7}`), want: json.RawMessage(`{"id":7}`)},
		{name: "exactly the cap", result: fits, want: fits},
		{name: "one byte over", result: over, truncated: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, truncated := CapResult(testCase.result)

			if truncated != testCase.truncated {
				t.Fatalf("CapResult(%d bytes) truncated = %t, want %t", len(testCase.result), truncated, testCase.truncated)
			}
			if !truncated {
				if string(got) != string(testCase.want) {
					t.Errorf("CapResult returned %d bytes, want the %d it was given unchanged", len(got), len(testCase.want))
				}
				return
			}
			// The point of replacing rather than cutting: what comes back has
			// to be a document a reader can parse, or the shard holding it is
			// a shard nobody can read at all. It is not necessarily shorter
			// than what it replaced, since escaping costs bytes and a result
			// barely over the cap can come back barely longer; the bound that
			// matters is on the content, and the line cap is sized for the
			// escaping.
			var head string
			if err := json.Unmarshal(got, &head); err != nil {
				t.Fatalf("the truncated result is not valid JSON: %v", err)
			}
			if len(head) != MaxResultBytes {
				t.Errorf("the truncated result holds %d bytes, want %d", len(head), MaxResultBytes)
			}
			if !strings.HasPrefix(head, `"aaa`) {
				t.Errorf("the truncated result = %.16q, want the head of what it was given", head)
			}
		})
	}
}

// TestCapResult_KeepsTheHeadOfWhatItWasGiven verifies that the replacement
// carries the first MaxResultBytes bytes and not fewer.
//
// The head is what a person triaging a failed attempt reads, and a cap that
// quietly kept a fraction of it would make that reading useless while still
// reporting the truncation honestly.
func TestCapResult_KeepsTheHeadOfWhatItWasGiven(t *testing.T) {
	body := strings.Repeat("z", MaxResultBytes*2)

	got, truncated := CapResult(json.RawMessage(body))

	if !truncated {
		t.Fatalf("CapResult(%d bytes) truncated = false, want true", len(body))
	}
	var head string
	if err := json.Unmarshal(got, &head); err != nil {
		t.Fatalf("the truncated result is not valid JSON: %v", err)
	}
	if len(head) != MaxResultBytes {
		t.Errorf("the truncated result holds %d bytes, want %d", len(head), MaxResultBytes)
	}
}

// TestCapText_Cases verifies what the text cap keeps, what it cuts and what it
// reports.
//
// The rendered answer is the longest thing a call line carries, because it is
// whatever the server printed: a raw file, a diff, a job trace. Keeping the head
// rather than replacing it is the difference from CapResult, and it is safe for
// the same reason it would not be there: a string cut in half is still a string.
func TestCapText_Cases(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		want      int
		truncated bool
	}{
		{name: "nothing at all", text: "", want: 0},
		{name: "a rendered table", text: "| iid | title |", want: len("| iid | title |")},
		{name: "exactly the cap", text: strings.Repeat("a", MaxTextBytes), want: MaxTextBytes},
		{name: "one byte over", text: strings.Repeat("a", MaxTextBytes+1), want: MaxTextBytes, truncated: true},
		{name: "a job trace", text: strings.Repeat("b", MaxTextBytes*3), want: MaxTextBytes, truncated: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, truncated := CapText(testCase.text)

			if truncated != testCase.truncated {
				t.Fatalf("CapText(%d bytes) truncated = %t, want %t", len(testCase.text), truncated, testCase.truncated)
			}
			if len(got) != testCase.want {
				t.Errorf("CapText(%d bytes) returned %d, want %d", len(testCase.text), len(got), testCase.want)
			}
			if !strings.HasPrefix(testCase.text, got) {
				t.Errorf("CapText returned %.16q, want the head of what it was given", got)
			}
		})
	}
}

// TestRecordInvalidRawField_NamesTheFieldThatDoesNotParse verifies that each of
// the three raw fields is checked, that the field is named, and that an absent
// one is not a finding.
//
// It is the boundary the writer asks at. All three are filled from a model's own
// output, so text that does not parse is ordinary rather than exceptional, and
// the encoder's own message for one names a type and neither the field nor what
// to do instead.
func TestRecordInvalidRawField_NamesTheFieldThatDoesNotParse(t *testing.T) {
	// The fragment a model emits when it stops mid-object, which is the case
	// this whole check exists for.
	broken := json.RawMessage(`{"action": "issue.list", "params": {`)

	cases := []struct {
		name   string
		record Record
		want   string
	}{
		{name: "nothing raw at all", record: (&Verify{Attempt: "a1"}).record()},
		{name: "a turn whose blocks carry none", record: (&Turn{Blocks: []Block{{Kind: BlockText, Text: "hello"}}}).record()},
		{
			name:   "a block carrying valid arguments",
			record: (&Turn{Blocks: []Block{{Kind: BlockToolCall, Arguments: json.RawMessage(`{"action":"issue.list"}`)}}}).record(),
		},
		{
			name:   "a malformed tool call in the second block",
			record: (&Turn{Blocks: []Block{{Kind: BlockText, Text: "listing"}, {Kind: BlockToolCall, Arguments: broken}}}).record(),
			want:   "block 2",
		},
		{name: "call arguments", record: (&Call{Attempt: "a1", Arguments: broken}).record(), want: "the call arguments"},
		{name: "a call result", record: (&Call{Attempt: "a1", Result: broken}).record(), want: "the call result"},
		{
			name:   "a call carrying both, which reports the first",
			record: (&Call{Attempt: "a1", Arguments: broken, Result: broken}).record(),
			want:   "the call arguments",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := testCase.record.invalidRawField()

			if testCase.want == "" {
				if got != "" {
					t.Errorf("invalidRawField = %q, want nothing: every raw field of this line parses", got)
				}
				return
			}
			if !strings.Contains(got, testCase.want) {
				t.Errorf("invalidRawField = %q, want it to name %q", got, testCase.want)
			}
		})
	}
}

// TestRecordInvalidRawField_KnowsEveryRawFieldOfTheRecord verifies that the
// fields invalidRawField checks are all the json.RawMessage fields the six line
// types have.
//
// The check is written out by hand, so a fourth raw field would be added to the
// record and silently not checked: the writer would then report it as the
// encoder's message about a type name, and a run recording a malformed call into
// it would lose the line. The paths are enumerated from the types themselves, so
// adding one fails here until it is answered above.
func TestRecordInvalidRawField_KnowsEveryRawFieldOfTheRecord(t *testing.T) {
	checked := map[string]bool{
		"turn.Blocks[].Arguments": true,
		"call.Arguments":          true,
		"call.Result":             true,
	}
	rawType := reflect.TypeFor[json.RawMessage]()

	found := map[string]bool{}
	for lineType, payload := range payloadTypes(t) {
		for _, leaf := range leafValues(payload, lineType) {
			if leaf.typ == rawType {
				found[leaf.path] = true
			}
		}
	}

	for path := range found {
		t.Run(path, func(t *testing.T) {
			if !checked[path] {
				t.Errorf("%s holds JSON a model produced and invalidRawField does not check it", path)
			}
		})
	}
	for path := range checked {
		t.Run(path+" is still there", func(t *testing.T) {
			if !found[path] {
				t.Errorf("invalidRawField checks %s, which is no longer a raw field of the record", path)
			}
		})
	}
}

// TestLineRecord_EachLineWrapsItselfInItsOwnEnvelope verifies that every line
// type names itself, carries itself and carries nothing else.
//
// The envelope is what the reader dispatches on, so a line whose type and
// payload disagree is a line that would be dropped or read as another shape.
// Each of the six is checked rather than one standing for the rest, because the
// six wrappers are six separate one-line functions and nothing but this holds
// them to the same rule.
func TestLineRecord_EachLineWrapsItselfInItsOwnEnvelope(t *testing.T) {
	cases := []struct {
		name string
		line Line
		want string
	}{
		{name: TypeRun, line: &Run{Package: "modeleval"}, want: TypeRun},
		{name: TypeSession, line: &Session{Label: "dynamic/default"}, want: TypeSession},
		{name: TypeAttempt, line: &Attempt{ID: "a1"}, want: TypeAttempt},
		{name: TypeTurn, line: &Turn{Attempt: "a1"}, want: TypeTurn},
		{name: TypeCall, line: &Call{Attempt: "a1"}, want: TypeCall},
		{name: TypeVerify, line: &Verify{Attempt: "a1"}, want: TypeVerify},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := testCase.line.record()

			if got.Schema != SchemaVersion {
				t.Errorf("record().Schema = %d, want %d", got.Schema, SchemaVersion)
			}
			if got.Type != testCase.want {
				t.Errorf("record().Type = %q, want %q", got.Type, testCase.want)
			}
			if err := got.validate(); err != nil {
				t.Errorf("the envelope a %s line wraps itself in does not validate: %v", testCase.want, err)
			}
		})
	}
}

// TestRecordValidate_AcceptsOnlyTheNamedPayload verifies, per line type, that a
// record carrying that type's own payload is accepted and one carrying another
// type's payload is refused.
//
// This is where the envelope's contract lives. A reader that took a mislabelled
// line would score an attempt against lines from another one, and a shard is
// machine-written, so a mislabelled line means a stale artifact rather than a
// caller's mistake.
func TestRecordValidate_AcceptsOnlyTheNamedPayload(t *testing.T) {
	cases := []struct {
		name  string
		lineT string
		own   Record
	}{
		{name: TypeRun, lineT: TypeRun, own: Record{Run: &Run{}}},
		{name: TypeSession, lineT: TypeSession, own: Record{Session: &Session{}}},
		{name: TypeAttempt, lineT: TypeAttempt, own: Record{Attempt: &Attempt{}}},
		{name: TypeTurn, lineT: TypeTurn, own: Record{Turn: &Turn{}}},
		{name: TypeCall, lineT: TypeCall, own: Record{Call: &Call{}}},
		{name: TypeVerify, lineT: TypeVerify, own: Record{Verify: &Verify{}}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			accepted := testCase.own
			accepted.Schema = SchemaVersion
			accepted.Type = testCase.lineT
			if err := accepted.validate(); err != nil {
				t.Errorf("a %s line carrying its own payload did not validate: %v", testCase.lineT, err)
			}

			// A verify payload stands in for "somebody else's", except on the
			// verify line itself, where a run payload does.
			foreign := Record{Schema: SchemaVersion, Type: testCase.lineT, Verify: &Verify{}}
			if testCase.lineT == TypeVerify {
				foreign = Record{Schema: SchemaVersion, Type: testCase.lineT, Run: &Run{}}
			}
			err := foreign.validate()
			if err == nil {
				t.Fatalf("a %s line carrying another type's payload validated, want an error", testCase.lineT)
			}
			if !strings.Contains(err.Error(), "carries no payload") {
				t.Errorf("validate error = %v, want it to say the line carries no payload", err)
			}
		})
	}
}

// TestRecordValidate_RefusesWhatCannotBeRead verifies the three refusals that
// are not about which payload a type names: a schema this package did not
// write, a type it has never heard of, and a line making two claims at once.
//
// Each is a stale or corrupt artifact rather than a caller's mistake, and each
// is refused rather than half-read: a published model figure computed over
// lines nobody could read is worse than no figure, which is the position the
// pages this record feeds are in today.
func TestRecordValidate_RefusesWhatCannotBeRead(t *testing.T) {
	cases := []struct {
		name   string
		record Record
		want   string
	}{
		{
			name:   "a schema from another version of this package",
			record: Record{Schema: SchemaVersion + 1, Type: TypeRun, Run: &Run{}},
			want:   "another version",
		},
		{
			name:   "no schema at all",
			record: Record{Type: TypeRun, Run: &Run{}},
			want:   "another version",
		},
		{
			name:   "a line type this package does not know",
			record: Record{Schema: SchemaVersion, Type: "verdict", Run: &Run{}},
			want:   `unknown line type "verdict"`,
		},
		{
			name:   "no line type",
			record: Record{Schema: SchemaVersion, Run: &Run{}},
			want:   "unknown line type",
		},
		{
			name:   "two payloads under one type",
			record: Record{Schema: SchemaVersion, Type: TypeCall, Call: &Call{}, Turn: &Turn{}},
			want:   "carries 2 payloads",
		},
		{
			name: "every payload at once",
			record: Record{
				Schema: SchemaVersion, Type: TypeRun,
				Run: &Run{}, Session: &Session{}, Attempt: &Attempt{},
				Turn: &Turn{}, Call: &Call{}, Verify: &Verify{},
			},
			want: "carries 6 payloads",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.record.validate()

			if err == nil {
				t.Fatal("validate = nil, want an error")
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("validate error = %v, want it to say %q", err, testCase.want)
			}
		})
	}
}

// recordLeaf is one value a line writes: where it sits, as a path from the line
// type down through the fields that reach it, and what it is.
type recordLeaf struct {
	path string
	typ  reflect.Type
}

// payloadTypes returns the payload type behind each line type.
//
// It is taken from an empty line of each rather than from a table of its own:
// the six record wrappers are what say which payload a type names, and a second
// spelling of that is a second thing to keep current. A line type without an
// entry here fails the run, since every walk below would pass over its fields in
// silence.
func payloadTypes(t *testing.T) map[string]reflect.Type {
	t.Helper()

	types := map[string]reflect.Type{}
	for _, line := range []Line{&Run{}, &Session{}, &Attempt{}, &Turn{}, &Call{}, &Verify{}} {
		types[line.record().Type] = reflect.TypeOf(line).Elem()
	}
	if len(types) != len(payloadPresent) {
		t.Fatalf("this test knows %d line type(s) and the record declares %d", len(types), len(payloadPresent))
	}
	return types
}

// leafValues enumerates every value a payload type writes, as a path from the
// given root down to each field JSON carries a value for.
//
// A slice or a map contributes its element's paths under "[]", so a field added
// inside a [Block] or a [Provider] is enumerated even though no line is obliged
// to carry more than one of them. A byte slice, which is what a
// [encoding/json.RawMessage] is, and a [time.Time] end the walk: each is written
// as one JSON value and neither has an exported shape underneath worth
// descending into.
func leafValues(typ reflect.Type, root string) []recordLeaf {
	switch {
	case typ == reflect.TypeFor[time.Time]():
		return []recordLeaf{{path: root, typ: typ}}
	case typ.Kind() == reflect.Pointer:
		return leafValues(typ.Elem(), root)
	case typ.Kind() == reflect.Struct:
		var leaves []recordLeaf
		for field := range typ.Fields() {
			if !field.IsExported() {
				continue
			}
			leaves = append(leaves, leafValues(field.Type, root+"."+field.Name)...)
		}
		return leaves
	case typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8:
		return []recordLeaf{{path: root, typ: typ}}
	case typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map:
		return leafValues(typ.Elem(), root+"[]")
	default:
		return []recordLeaf{{path: root, typ: typ}}
	}
}

// filledValues records the path of every leaf of value that is not its zero,
// walking it the way leafValues walks its type.
func filledValues(value reflect.Value, root string, filled map[string]bool) {
	switch {
	case value.Type() == reflect.TypeFor[time.Time]():
		if !value.IsZero() {
			filled[root] = true
		}
	case value.Kind() == reflect.Pointer:
		if !value.IsNil() {
			filledValues(value.Elem(), root, filled)
		}
	case value.Kind() == reflect.Struct:
		for index := range value.NumField() {
			field := value.Type().Field(index)
			if !field.IsExported() {
				continue
			}
			filledValues(value.Field(index), root+"."+field.Name, filled)
		}
	case value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8:
		if value.Len() > 0 {
			filled[root] = true
		}
	case value.Kind() == reflect.Slice:
		for index := range value.Len() {
			filledValues(value.Index(index), root+"[]", filled)
		}
	case value.Kind() == reflect.Map:
		for _, key := range value.MapKeys() {
			filledValues(value.MapIndex(key), root+"[]", filled)
		}
	default:
		if !value.IsZero() {
			filled[root] = true
		}
	}
}

// fieldName is one field of the record: where it sits, what Go calls it and
// what a shard calls it.
type fieldName struct {
	path     string
	goName   string
	jsonName string
}

// jsonNames collects every field the record writes, from the envelope down
// through the payloads, the structures inside them and the elements of their
// slices and maps.
//
// It walks the types the way leafValues does and stops where that does, at a
// [time.Time] and at a byte slice, because neither writes a name of this
// package's own.
func jsonNames(typ reflect.Type, root string) []fieldName {
	switch {
	case typ == reflect.TypeFor[time.Time]():
		return nil
	case typ.Kind() == reflect.Pointer:
		return jsonNames(typ.Elem(), root)
	case typ.Kind() == reflect.Slice && typ.Elem().Kind() == reflect.Uint8:
		return nil
	case typ.Kind() == reflect.Slice, typ.Kind() == reflect.Map:
		return jsonNames(typ.Elem(), root+"[]")
	case typ.Kind() == reflect.Struct:
		var names []fieldName
		for field := range typ.Fields() {
			if !field.IsExported() {
				continue
			}
			declared, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			names = append(names, fieldName{path: root + "." + field.Name, goName: field.Name, jsonName: declared})
			names = append(names, jsonNames(field.Type, root+"."+field.Name)...)
		}
		return names
	default:
		return nil
	}
}

// snakeCase spells a Go field name the way this record spells it in JSON: words
// separated by underscores, with a run of capitals read as one word, so ID is
// id and LatencyMS is latency_ms.
func snakeCase(name string) string {
	runes := []rune(name)
	var spelled strings.Builder
	for index, letter := range runes {
		if index > 0 && unicode.IsUpper(letter) {
			startsAWord := !unicode.IsUpper(runes[index-1])
			endsARun := index+1 < len(runes) && unicode.IsLower(runes[index+1])
			if startsAWord || endsARun {
				spelled.WriteRune('_')
			}
		}
		spelled.WriteRune(unicode.ToLower(letter))
	}
	return spelled.String()
}

// TestRecordFieldNames_AreTheGoNamesInSnakeCase verifies that every field of
// every line is written under the snake case of its Go name, with the one
// exception declared below.
//
// It exists because nothing else in this package can see a misspelled name, and
// the round trip least of all: one struct both writes and reads a shard, so a
// tag that drops a letter from a name writes the misspelling, reads the same
// misspelling back, and the comparison matches. That was measured rather than
// assumed: with the tags of [Verify.Passed], [Attempt.Reason], [Turn.Detail]
// and [Call.RefusalReason] misspelled, every test in this package passed,
// including the round trip over a fixture that fills all four.
//
// The names are the contract with cmd/gen_model_results, with the scorer that
// reads a refusal out of [Call.RefusalReason] and a check out of
// [Verify.Passed], and with anyone reading a shard by hand months later. A rule
// is the only oracle for them that is not the tags themselves: a list of
// expected names copied from the tags would agree with any typo it was copied
// from.
func TestRecordFieldNames_AreTheGoNamesInSnakeCase(t *testing.T) {
	// GitLab is one word, and the Go identifier capitalizes it the way the
	// company does, so the rule below would spell it git_lab_version.
	exceptions := map[string]string{"record.Run.GitLabVersion": "gitlab_version"}

	excused := map[string]bool{}
	for _, field := range jsonNames(reflect.TypeFor[Record](), "record") {
		t.Run(field.path, func(t *testing.T) {
			want, declared := exceptions[field.path]
			if declared {
				excused[field.path] = true
			} else {
				want = snakeCase(field.goName)
			}
			if field.jsonName == "" {
				t.Fatalf("%s is written under no name of its own, so a shard carries it as %q", field.path, field.goName)
			}
			if field.jsonName != want {
				t.Errorf("%s is written as %q, want %q", field.path, field.jsonName, want)
			}
		})
	}
	for path := range exceptions {
		t.Run(path+" is still there", func(t *testing.T) {
			if !excused[path] {
				t.Errorf("%s is declared an exception to the naming rule and is no longer a field of the record", path)
			}
		})
	}
}

// TestSnakeCase_SpellsTheNamesThisRecordUses verifies the rule the field-name
// test is judged by, on the shapes this record's own names take.
//
// The rule is what decides whether a tag is right, so a rule that read ID as
// i_d or LatencyMS as latency_m_s would excuse the typos it exists to catch by
// making every correct name look wrong and every check noisy.
func TestSnakeCase_SpellsTheNamesThisRecordUses(t *testing.T) {
	cases := map[string]string{
		"Passed":             "passed",
		"Reason":             "reason",
		"RefusalReason":      "refusal_reason",
		"ID":                 "id",
		"RunID":              "run_id",
		"CallID":             "call_id",
		"LatencyMS":          "latency_ms",
		"BudgetUSD":          "budget_usd",
		"InputPerMillionUSD": "input_per_million_usd",
		"DispatchObserved":   "dispatch_observed",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			if got := snakeCase(name); got != want {
				t.Errorf("snakeCase(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

// TestPayloadPresent_CoversEveryLineType verifies that the table the reader
// dispatches on holds exactly the six declared types.
//
// The table is the single place a seventh line type would have to be added, and
// a type declared without an entry would be refused as unknown at read time,
// long after the run that wrote it was paid for.
func TestPayloadPresent_CoversEveryLineType(t *testing.T) {
	declared := []string{TypeRun, TypeSession, TypeAttempt, TypeTurn, TypeCall, TypeVerify}

	if len(payloadPresent) != len(declared) {
		t.Errorf("payloadPresent holds %d entr(ies), want %d", len(payloadPresent), len(declared))
	}
	for _, lineType := range declared {
		t.Run(lineType, func(t *testing.T) {
			if _, known := payloadPresent[lineType]; !known {
				t.Errorf("payloadPresent has no entry for %q", lineType)
			}
		})
	}
}
