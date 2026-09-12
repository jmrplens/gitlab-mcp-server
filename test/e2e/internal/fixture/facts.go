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

import "net/http"

// NonSQLMetricsUnserved reports whether err is the answer GitLab 19 CE gives
// on the usage-data non-SQL metrics endpoint: a 404, even with its feature
// flag enabled, verified by live probing. A test of that action asserts the
// documented error path with this, and passes unchanged on a future GitLab
// that serves the endpoint, since it then gets no error to classify.
func NonSQLMetricsUnserved(err error) bool {
	return IsStatus(err, http.StatusNotFound)
}
