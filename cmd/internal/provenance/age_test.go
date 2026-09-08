package provenance

import (
	"strings"
	"testing"
	"time"
)

// today is the day every test below measures from, so an age is a fact about
// the fixture rather than about the day the suite runs.
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

// TestAge_ADateItCanRead_MeasuresFromUTC verifies the arithmetic itself,
// including the conversion the three records depend on: a retrieval date is a
// UTC day, so the comparison has to be made in UTC whatever zone the machine
// running the gate is in.
func TestAge_ADateItCanRead_MeasuresFromUTC(t *testing.T) {
	cases := []struct {
		name        string
		retrievedAt string
		now         time.Time
		want        int
	}{
		{name: "the same day", retrievedAt: "2026-09-08", now: today, want: 0},
		{name: "a week ago", retrievedAt: "2026-09-01", now: today, want: 7},
		{
			name:        "a day recorded in another zone",
			retrievedAt: "2026-09-01",
			now:         today.In(time.FixedZone("UTC-11", -11*60*60)),
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
// an error rather than a zero age. Every record's decoder accepts any string
// in that field, so an age of zero would read as "taken today" and let a
// record with a hand-edited date through the one check that asks about it.
func TestAge_ADateNothingCanRead_ReportsWhy(t *testing.T) {
	if _, err := Age("one tuesday", today); err == nil {
		t.Error("Age accepted a string that is not a date")
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

// TestProblems_ADateAGateCanRestOn_ReportsNothing verifies the quiet case: a
// record taken inside the window adds no problem to the ones its command
// found for itself.
func TestProblems_ADateAGateCanRestOn_ReportsNothing(t *testing.T) {
	subject := Subject{Noun: "record", Consequence: "it says nothing"}

	if got := Problems(subject, today.Add(-MaxAge/2).Format(time.DateOnly), today); got != nil {
		t.Errorf("Problems reported %v for a record inside the window", got)
	}
}

// TestProblems_ADateAGateCannotRestOn_NamesTheSubject verifies the three
// refusals and that each is written in the calling command's own words: the
// verdict is shared, so a reader who meets one in CI output has to be able to
// tell which of the pinned records it is about.
func TestProblems_ADateAGateCannotRestOn_NamesTheSubject(t *testing.T) {
	subject := Subject{Noun: "pin", Consequence: "a document that broke since goes unreported"}

	cases := []struct {
		name        string
		retrievedAt string
		want        string
		// quotesDate is whether the message is about the date itself, and so
		// has to show it. The stale message is about how far back it is, and
		// shows the count of days and the window instead.
		quotesDate bool
	}{
		{name: "a date nothing can read", retrievedAt: "one tuesday", want: "is not a date", quotesDate: true},
		{name: "a date that has not happened", retrievedAt: "2026-09-09", want: "has not happened yet", quotesDate: true},
		{name: "a date past the window", retrievedAt: "2020-01-01", want: "days old and the window is 180"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			problems := Problems(subject, testCase.retrievedAt, today)

			if len(problems) != 1 {
				t.Fatalf("Problems returned %d problems, want 1: %v", len(problems), problems)
			}
			if !strings.Contains(problems[0], testCase.want) {
				t.Errorf("problem does not explain the refusal %q: %s", testCase.want, problems[0])
			}
			if !strings.HasPrefix(problems[0], "the pin ") {
				t.Errorf("problem does not name the subject it is about: %s", problems[0])
			}
			if testCase.quotesDate && !strings.Contains(problems[0], testCase.retrievedAt) {
				t.Errorf("problem does not quote the date it refused: %s", problems[0])
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
