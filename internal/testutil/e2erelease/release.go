package e2erelease

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/finegrained"
)

// Verdict is what a run does on the release it found.
type Verdict int

// The verdicts, in no order a run reads anything into. The zero value is
// Unreadable, so a decision nobody made stops a run rather than passing it.
const (
	// Unreadable is a version, the instance's or the table's bucket, that does
	// not start with a major.minor as GitLab writes one, two numbers with no
	// leading zero. The run stops: nothing about it can be judged.
	Unreadable Verdict = iota
	// Recorded is a release in the major.minor the table was recorded from,
	// where a fine-grained grant decides the listing and the calls (phase B).
	// The run goes ahead.
	Recorded
	// ListingOnly is the prerelease of the release right after the table's,
	// where a grant decides the listing alone. The run goes ahead.
	ListingOnly
	// Older is a release older than the table's, which is a deliberate pin
	// rather than a stale table: no grant is judged on it (phase A), and the
	// run goes ahead saying so in one line.
	Older
	// Newer is a release past both states a grant is judged in. The table has
	// fallen behind the GitLab a user installs, and the run stops until it is
	// re-recorded.
	Newer
)

// String names the verdict, as a word a log line can carry.
func (v Verdict) String() string {
	switch v {
	case Unreadable:
		return "unreadable"
	case Recorded:
		return "recorded"
	case ListingOnly:
		return "listing-only"
	case Older:
		return "older"
	case Newer:
		return "newer"
	default:
		return fmt.Sprintf("verdict(%d)", int(v))
	}
}

// Decision is one release measured against one table: the verdict, and the
// three values it was taken from, which is what its text names.
type Decision struct {
	// Verdict is what the run does.
	Verdict Verdict
	// Version is the release the instance reported, as it reported it.
	Version string
	// Table is the release the table was recorded from ("19.4.1-ee").
	Table string
	// Bucket is the table's major.minor ("19.4").
	Bucket string
	// display is the table's release as a Docker tag spells it ("19.4.1"),
	// for the pin a stop suggests.
	display string
}

// Decide measures the release an instance reported against the table a run
// carries. version is what GET /api/v4/version answered ("19.4.1-ee",
// "19.5.0-pre"); table is non-nil.
//
// The major.minor is read with [finegrained.Bucket], the parse the table is
// chosen by, and compared as two numbers, so the verdicts are a total order
// with the listing-only prerelease the one release past the table that runs.
func Decide(version string, table *finegrained.Table) Decision {
	decision := Decision{Version: version, Table: table.Version, Bucket: table.Bucket, display: table.DisplayVersion()}
	recorded, recordedOK := parseBucket(table.Bucket)
	found, foundOK := parseBucket(finegrained.Bucket(version))
	if !recordedOK || !foundOK {
		decision.Verdict = Unreadable
		return decision
	}
	// Separate ifs rather than a tagless switch: a case expression carries no
	// statement counter, so the mutation gate reports every mutant of one as
	// not covered whatever the tests assert.
	order := found.compare(recorded)
	if order < 0 {
		decision.Verdict = Older
		return decision
	}
	if order == 0 {
		decision.Verdict = Recorded
		return decision
	}
	if PrereleaseAfter(version, table.Bucket) {
		decision.Verdict = ListingOnly
		return decision
	}
	decision.Verdict = Newer
	return decision
}

// Runs reports whether the run goes ahead: on the table's release, on the
// prerelease right after it, and on an older release. Anything else stops it,
// a verdict nobody defined included.
func (d Decision) Runs() bool {
	switch d.Verdict {
	case Recorded, ListingOnly, Older:
		return true
	default:
		return false
	}
}

// Text is what the run says about the decision: one line for a release it
// runs on, and for one it stops on, which releases they are and what to do
// first.
func (d Decision) Text() string {
	switch d.Verdict {
	case Recorded:
		return fmt.Sprintf("GitLab %s is in the release the fine-grained action table was recorded from (%s): "+
			"a fine-grained grant decides the listing and the calls (phase B)", d.Version, d.Table)
	case ListingOnly:
		return fmt.Sprintf("GitLab %s is the prerelease right after the release the fine-grained action table was recorded from (%s): "+
			"a fine-grained grant decides the listing alone, and every call phase A allows is handed to GitLab", d.Version, d.Table)
	case Older:
		return fmt.Sprintf("GitLab %s is older than the release the fine-grained action table was recorded from (%s), "+
			"so no fine-grained grant is judged on it: every fine-grained session runs in phase A, "+
			"withholding only what no fine-grained token reaches", d.Version, d.Table)
	case Newer:
		return d.newerText()
	case Unreadable:
		return d.unreadableText()
	default:
		return fmt.Sprintf("GitLab %s measured against the fine-grained action table (%s) took %s, which is no verdict this rule defines",
			d.Version, d.Table, d.Verdict)
	}
}

// newerText is the stop for a release the table has fallen behind: both
// releases, why it matters, the three steps the issue names, and the pin that
// runs the table's own release meanwhile.
func (d Decision) newerText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "GitLab %s is newer than the release the fine-grained action table was recorded from (%s): "+
		"it is in neither that release's %s nor the prerelease right after it (%s), the two in which a fine-grained grant is judged. "+
		"On it every fine-grained session would run in phase A, and the scenarios that hold what a grant lists and may call "+
		"would pass without checking either.\n\n", d.Version, d.Table, d.Bucket, NextPrerelease(d.Bucket))
	b.WriteString("Re-record the table before running against this release:\n")
	b.WriteString("  1. make gen-api-live, which boots gitlab/gitlab-ee:latest and rewrites docs/development/gitlab-api-live.json " +
		"(go run ./cmd/gen_api_live/ -image <image> boots another release);\n")
	b.WriteString("  2. make gen-action-grants, which rewrites internal/tools/actiongrants/table_gen.go, " +
		"docs/development/action-requests.json and site/src/content/docs/reference/fine-grained-permissions.mdx with its Spanish twin, " +
		"and writes nothing while the derivation has a finding, which is where a release that changed an authorization shows up;\n")
	b.WriteString("  3. commit what both rewrote, then start the run again.\n\n")
	fmt.Fprintf(&b, "To run against the release the table records meanwhile, pin it: "+
		"GITLAB_IMAGE=gitlab/gitlab-ce:%[1]s-ce.0 or gitlab/gitlab-ee:%[1]s-ee.0 for a Docker run.", d.display)
	return b.String()
}

// unreadableText is the stop for a version nothing can be read from, the
// table's bucket first since a table that cannot be measured against stops
// every run whatever the instance answers.
func (d Decision) unreadableText() string {
	if _, ok := parseBucket(d.Bucket); !ok {
		return fmt.Sprintf("The fine-grained action table (%s) carries the bucket %q, which is no major.minor as GitLab writes one, "+
			"so no release can be measured against it. The run stops: regenerate the table with make gen-action-grants.", d.Table, d.Bucket)
	}
	return fmt.Sprintf("GitLab reported the version %q, which does not start with a major.minor as GitLab writes one, so nothing can say whether "+
		"the fine-grained action table (recorded from %s) judges a grant on it. The run stops rather than pass without knowing: "+
		"GET /api/v4/version has to answer the instance's release.", d.Version, d.Table)
}

// NextPrerelease spells the prerelease of the release right after bucket: the
// next minor of the same major ("19.5.0-pre" after "19.4"), or the next major's
// first after a major's last minor ("20.0.0-pre" after "19.11"). It is "" for
// a bucket that is no major.minor as GitLab writes one.
func NextPrerelease(bucket string) string {
	recorded, ok := parseBucket(bucket)
	if !ok {
		return ""
	}
	return recorded.next().String() + ".0-pre"
}

// PrereleaseAfter reports whether version is the prerelease of the release
// right after the major.minor bucket ([NextPrerelease]), which is what GitLab.com
// and a nightly image report while releases stop at the table's minor, and the
// one release past the table whose grant decides the listing.
func PrereleaseAfter(version, bucket string) bool {
	recorded, ok := parseBucket(bucket)
	return ok && strings.HasSuffix(version, "-pre") && finegrained.Bucket(version) == recorded.next().String()
}

// lastMinorOfAMajor is the last minor GitLab releases in a major: a minor each
// month and a major each May, so a major runs from .0 to .11 and the release
// after x.11 is (x+1).0.
const lastMinorOfAMajor = 11

// release is a major.minor as two numbers.
type release struct {
	major, minor int
}

// parseBucket reads a major.minor bucket as two numbers. It refuses anything
// [finegrained.Bucket] would not have written, which keeps "19", "19.4.1" and
// "x.4" out, a number no int holds, and a number written with a leading zero
// ("19.04"). GitLab never writes the last, and [finegrained.Judge] compares a
// bucket as text, so reading "19.04" as the release "19.4" would let a run go
// ahead in a state the server does not take.
func parseBucket(bucket string) (release, bool) {
	if finegrained.Bucket(bucket) != bucket {
		return release{}, false
	}
	// Both halves are runs of digits now, so the one error Atoi can return is
	// a range error, and the value it returns then is clamped to the largest
	// int, which never spells the text it was read from. Spelling the release
	// back is therefore what refuses a number no int holds, as it refuses a
	// leading zero, and the errors need no test of their own.
	majorText, minorText, _ := strings.Cut(bucket, ".")
	major, _ := strconv.Atoi(majorText)
	minor, _ := strconv.Atoi(minorText)
	parsed := release{major: major, minor: minor}
	if parsed.String() != bucket {
		return release{}, false
	}
	return parsed, true
}

// compare orders two releases, major first: negative when r is older than o,
// zero when they are the same, positive when r is newer.
func (r release) compare(o release) int {
	return cmp.Or(cmp.Compare(r.major, o.major), cmp.Compare(r.minor, o.minor))
}

// next is the release right after r.
func (r release) next() release {
	if r.minor >= lastMinorOfAMajor {
		return release{major: r.major + 1}
	}
	return release{major: r.major, minor: r.minor + 1}
}

// String spells r as its bucket.
func (r release) String() string {
	return strconv.Itoa(r.major) + "." + strconv.Itoa(r.minor)
}
