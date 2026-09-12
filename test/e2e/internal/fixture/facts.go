//go:build e2e

// facts.go holds what the suite this replaces learned about the GitLab
// release it runs against and that a test cannot discover for itself. Each
// fact is a function or a constant a test names, so the reason lives in one
// place and the test that relies on it reads as a claim rather than as a
// number.
//
// The deployment facts, which are requests rather than answers, are stated
// on the deployment builder beside the request they shape.

package fixture

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// NonSQLMetricsUnserved reports whether err is the answer GitLab 19 CE gives
// on the usage-data non-SQL metrics endpoint: a 404, even with its feature
// flag enabled, verified by live probing. A test of that action asserts the
// documented error path with this, and passes unchanged on a future GitLab
// that serves the endpoint, since it then gets no error to classify.
func NonSQLMetricsUnserved(err error) bool {
	return IsStatus(err, http.StatusNotFound)
}

// legacyEpicRESTRemovedMajor is the GitLab release that removed the epic
// REST endpoints: epics became work items, and the routes the resource
// event actions of an epic reach answer 404 there whatever the tier.
const legacyEpicRESTRemovedMajor = 19

// LegacyEpicRESTRemoved reports whether the instance this run targets has
// no epic REST endpoints any more. A test of an epic's resource events
// asserts the documented refusal on such a release and the events
// themselves on an earlier one, and says which it did.
func LegacyEpicRESTRemoved(e *harness.Env) bool {
	return MajorVersion(e.Runtime().Version) >= legacyEpicRESTRemovedMajor
}

// MajorVersion reads the major number off a GitLab version string such as
// "19.3.1-ee" or "18.11.0". A string with no leading number reads as zero,
// which is below every release and so claims nothing about it.
func MajorVersion(version string) int {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	number, err := strconv.Atoi(major)
	if err != nil || number < 0 {
		return 0
	}
	return number
}
