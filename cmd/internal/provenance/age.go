package provenance

import (
	"fmt"
	"time"
)

// MaxAge is how long a pinned record may stand before the --check that reads
// it refuses it.
//
// GitLab ships monthly and narrows fields in place, so half a year is roughly
// six releases of drift: long enough not to ambush an unrelated change often,
// short enough that a narrowing is noticed within a release cycle or two. The
// window exists because the gate cannot ask the generator's question. A check
// that runs without the network can prove the record is readable, whole and
// provenanced, and can prove nothing about whether it still matches the GitLab
// it was taken from; an old record is therefore a gate that has quietly
// stopped asking, and the only honest way to say so is to fail.
//
// One window for all three records, because the reason is about GitLab's
// release cadence and not about any one of them: a reason to widen or narrow
// it is a reason to move all three.
const MaxAge = 180 * 24 * time.Hour

// Clock is now with a default, so a run that only checks a committed record
// does not have to supply one to ask how old it is.
//
// Every one of these commands takes its clock as a parameter for the same
// reason: the refusals below are otherwise reachable only from a process on
// the right day, and a test that cannot reach them is a gate nobody has seen
// work.
func Clock(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

// Age reports how long ago retrievedAt was, or the reason the date cannot say.
//
// Every record's decoder accepts any string in that field, so this is the only
// place a date nobody can read is noticed, and a date the age check cannot
// read is a record whose age nobody knows, which is exactly what the window
// exists to refuse.
func Age(retrievedAt string, now time.Time) (time.Duration, error) {
	retrieved, err := time.Parse(time.DateOnly, retrievedAt)
	if err != nil {
		return 0, err
	}
	return now.UTC().Sub(retrieved), nil
}

// Days renders an age the way every sentence about one spells it: whole days,
// truncated, because a record's date carries no time of day to be precise
// about.
func Days(age time.Duration) int {
	return int(age.Hours() / 24)
}

// Subject is what one command's record calls itself when its age is judged.
// The verdict is shared; the sentence stays the command's own, because a
// reader who meets it in CI output is reading about one artifact and not about
// the three that happen to share a window.
type Subject struct {
	// Noun names the artifact in a sentence: "record" for the two REST
	// records, "pin" for the GraphQL schema.
	Noun string
	// Consequence completes "the <noun> is N days old and the window is M: ",
	// saying what this particular record can no longer report. It is the half
	// of the message that is genuinely different between the three, since each
	// record is stale in its own way: one can no longer report a field that
	// changed, one a document that broke, one when a field is sent.
	Consequence string
}

// Problems reports the ways a record's retrieval date stops it being one a
// gate can rest on: a date nothing can parse, a date that has not happened,
// and a date further back than [MaxAge].
//
// It returns a slice rather than one string so a caller appends it to the
// problems it found itself, and so a date that is merely fine adds nothing.
func Problems(subject Subject, retrievedAt string, now time.Time) []string {
	switch age, err := Age(retrievedAt, now); {
	case err != nil:
		return []string{fmt.Sprintf(
			"the %s says it was taken on %q, which is not a date: nothing can then say how old the gate is",
			subject.Noun, retrievedAt,
		)}
	case age < 0:
		return []string{fmt.Sprintf(
			"the %s says it was taken on %s, which has not happened yet: no regeneration writes a day in the future",
			subject.Noun, retrievedAt,
		)}
	case age > MaxAge:
		return []string{fmt.Sprintf(
			"the %s is %d days old and the window is %d: %s",
			subject.Noun, Days(age), Days(MaxAge), subject.Consequence,
		)}
	default:
		return nil
	}
}
