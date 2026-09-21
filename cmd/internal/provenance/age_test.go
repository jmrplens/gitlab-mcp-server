package provenance

import (
	"strings"
	"testing"
	"time"
)

// today is the day the tests below measure from, so an age is a fact about the
// fixture rather than about the day the suite runs. It is noon, which is why
// the one test that needs an exact age anchors itself to midnight instead.
var today = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

// TestClock_NoFunctionSupplied_FallsBackToNow verifies the default every
// command's check half relies on: a run that only reads a committed record
// hands in no clock, and must still be able to ask how old the record is
// rather than dividing by a zero time.
func TestClock_NoFunctionSupplied_FallsBackToNow(t *testing.T) {
	before := time.Now()
	got := Clock(nil)
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("Clock(nil) = %s, want a time between %s and %s", got, before, after)
	}
}

// TestClock_FunctionSupplied_UsesIt verifies the other half of the same
// default: a test that hands in a day gets that day, which is what makes the
// age refusals reachable from a test at all.
func TestClock_FunctionSupplied_UsesIt(t *testing.T) {
	if got := Clock(func() time.Time { return today }); !got.Equal(today) {
		t.Errorf("Clock(fixed) = %s, want %s", got, today)
	}
}

// TestAge_ADateItCanRead_MeasuresElapsedTimeNotCalendarDays verifies the
// arithmetic itself, and that it is elapsed time between two instants rather
// than a difference of calendar dates. The distinction is what the three
// records depend on: a retrieval date is midnight UTC, and an age derived from
// the calendar date the clock reads locally would move with the zone of the
// machine running the gate. The last case is the one that can tell the two
// implementations apart, because the instant it names falls on the next day in
// the zone it is rendered in.
func TestAge_ADateItCanRead_MeasuresElapsedTimeNotCalendarDays(t *testing.T) {
	cases := []struct {
		name        string
		retrievedAt string
		now         time.Time
		want        int
	}{
		{name: "the same day", retrievedAt: "2026-09-08", now: today, want: 0},
		{name: "a week ago", retrievedAt: "2026-09-01", now: today, want: 7},
		{
			name:        "an instant rendered in a zone where it is already tomorrow",
			retrievedAt: "2026-09-01",
			now:         today.In(time.FixedZone("UTC+14", 14*60*60)),
			want:        7,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			age, err := Age(testCase.retrievedAt, testCase.now)
			if err != nil {
				t.Fatalf("Age(%q) returned %v", testCase.retrievedAt, err)
			}
			if got := Days(age); got != testCase.want {
				t.Errorf("Days(Age(%q)) = %d, want %d", testCase.retrievedAt, got, testCase.want)
			}
		})
	}
}

// TestAge_ADateNothingCanRead_ReportsWhy verifies that an unreadable date is
// an error, and that the age beside it is zero rather than whatever the failed
// parse left behind. Every record's decoder accepts any string in that field,
// and the empty one is what a record that simply omits it decodes to; a caller
// that read the duration without the error would otherwise be handed the
// distance from the zero time, saturated at the 292 years a Duration holds,
// and call the record impossibly stale.
func TestAge_ADateNothingCanRead_ReportsWhy(t *testing.T) {
	cases := []struct {
		name        string
		retrievedAt string
	}{
		{name: "a string that is not a date", retrievedAt: "one tuesday"},
		{name: "the empty string a missing field decodes to", retrievedAt: ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			age, err := Age(testCase.retrievedAt, today)

			if err == nil {
				t.Errorf("Age(%q) returned no error", testCase.retrievedAt)
			}
			if age != 0 {
				t.Errorf("Age(%q) = %s beside its error, want 0", testCase.retrievedAt, age)
			}
		})
	}
}

// TestDays_AnAgeInHours_TruncatesToWholeDays verifies the rendering every
// message about an age uses: a record's date carries no time of day, so a
// part-day is not a day.
func TestDays_AnAgeInHours_TruncatesToWholeDays(t *testing.T) {
	if got := Days(47 * time.Hour); got != 1 {
		t.Errorf("Days(47h) = %d, want 1", got)
	}
}

// TestProblems_ADateAGateCanRestOn_ReportsNothing verifies the quiet case and
// both of its edges: a record taken inside the window adds no problem to the
// ones its command found for itself, and neither edge is outside it. Both
// refusals are written as strict comparisons, so either one loosened by an
// instant would refuse a record it was meant to accept — the one taken as the
// gate reads it as a regeneration that has not happened, the one exactly
// [MaxAge] old as one that has aged out.
//
// The cases are measured from midnight rather than from today, because a
// retrieval date parses to midnight UTC and an age of exactly zero or exactly
// [MaxAge] cannot be expressed against an instant at noon.
func TestProblems_ADateAGateCanRestOn_ReportsNothing(t *testing.T) {
	subject := Subject{Noun: "record", Consequence: "it says nothing"}
	midnight := time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)
	retrievedAt := midnight.Format(time.DateOnly)

	cases := []struct {
		name string
		now  time.Time
	}{
		{name: "half the window ago", now: midnight.Add(MaxAge / 2)},
		{name: "at the very instant the gate reads it", now: midnight},
		{name: "exactly the window ago, which is still inside it", now: midnight.Add(MaxAge)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Problems(subject, retrievedAt, testCase.now); got != nil {
				t.Errorf("Problems reported %v for a record taken %s", got, testCase.name)
			}
		})
	}
}

// TestProblems_ADateAGateCannotRestOn_NamesTheSubject verifies the three
// refusals whole, the unreadable date in both shapes it arrives in, and that
// each is written in the calling command's own words: the verdict is shared,
// so a reader who meets one in CI output has to be able to tell which of the
// pinned records it is about.
//
// The sentence is compared entire rather than searched for a phrase, because
// every part of it is a value something could get wrong without a phrase
// search noticing: the stale message carries two day counts and a search for
// the window matches a message that printed the window twice and never said
// how old the record is, and the unreadable message quotes its date so that the
// empty one a missing field decodes to leaves a visible "" rather than a hole
// in the middle of the sentence.
func TestProblems_ADateAGateCannotRestOn_NamesTheSubject(t *testing.T) {
	subject := Subject{Noun: "pin", Consequence: "a document that broke since goes unreported"}

	cases := []struct {
		name        string
		retrievedAt string
		want        string
	}{
		{
			name:        "a date nothing can read",
			retrievedAt: "one tuesday",
			want:        `the pin says it was taken on "one tuesday", which is not a date: nothing can then say how old the gate is`,
		},
		{
			name:        "a date a missing field left empty",
			retrievedAt: "",
			want:        `the pin says it was taken on "", which is not a date: nothing can then say how old the gate is`,
		},
		{
			name:        "a date that has not happened",
			retrievedAt: "2026-09-09",
			want:        "the pin says it was taken on 2026-09-09, which has not happened yet: no regeneration writes a day in the future",
		},
		{
			name:        "a date past the window",
			retrievedAt: "2020-01-01",
			want:        "the pin is 2442 days old and the window is 180: a document that broke since goes unreported",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problems := Problems(subject, testCase.retrievedAt, today)

			if len(problems) != 1 {
				t.Fatalf("Problems returned %d problems, want 1: %v", len(problems), problems)
			}
			if problems[0] != testCase.want {
				t.Errorf("Problems(%q) = %q, want %q", testCase.retrievedAt, problems[0], testCase.want)
			}
		})
	}
}

// TestProblems_APastTheWindowDate_CarriesTheCommandsOwnConsequence verifies
// the half of the stale message that is genuinely each command's own: what
// this particular record can no longer report. Sharing that clause is what
// would turn three honest sentences into one vague one.
func TestProblems_APastTheWindowDate_CarriesTheCommandsOwnConsequence(t *testing.T) {
	subject := Subject{Noun: "record", Consequence: "one this old no longer says when a field is sent"}

	problems := Problems(subject, "2020-01-01", today)

	if len(problems) != 1 || !strings.HasSuffix(problems[0], subject.Consequence) {
		t.Errorf("problems do not end with the caller's consequence: %v", problems)
	}
}
