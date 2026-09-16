//go:build e2e

// main_test.go declares what this package needs of the instance it runs on:
// nothing in particular. The harness resolves the configuration here and
// contacts GitLab only when the first test asks it to, so a run that measures
// nothing never reaches an instance and never spends a token.

package modeleval

import (
	"os"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/test/e2e/internal/harness"
)

// TestMain hands the package to the harness with the Any requirement. Which
// cases a run may attempt is decided by their own declared needs, case by
// case, and not by a requirement over the whole package: a corpus holding a
// licensed case would otherwise refuse the whole run on a CE instance instead
// of skipping that case with a reason.
func TestMain(m *testing.M) {
	os.Exit(harness.Main(m, harness.Any))
}

// TestModelEval is the run: a model against the real binary, case by case, on
// each surface the configuration names.
//
// It reads the configuration and stops there. What is missing is the turn loop
// that composes a stimulus, asks a provider and feeds the answer back, which
// arrives with the providers; until then there is no model to ask and nothing
// to record, and a test that pretended otherwise would be the dry run this
// rebuild deleted, which reported six metrics at 100.0% while dispatching
// nothing.
//
// The configuration is still read, and read first. It is the one thing that
// can be wrong before a provider is involved, and a run refused here for a
// misspelled surface costs nothing, while one refused after the first request
// costs a request.
func TestModelEval(t *testing.T) {
	cfg, err := loadRunConfig()
	if err != nil {
		t.Fatalf("reading the run's configuration: %v", err)
	}

	t.Skipf("no turn loop yet: this package holds the seams, the server shape and the call adapter. "+
		"It would have run %d surface(s) in %s mode.", len(cfg.Surfaces), cfg.Mode)

	// Unreachable until the loop lands, and compiled rather than deleted: this
	// is the sequence the loop is built out of, and it is what makes the
	// harness exports it uses reachable from a package outside the harness,
	// which is the condition cmd/audit_e2e_coverage holds every export to.
	env := harness.New(t)
	session := openSession(env, cfg, cfg.Surfaces[0])
	answer, line := callAsModel(env.Ctx, session, callPosition{Attempt: t.Name(), Turn: 1, Index: 1},
		harness.ModelCall{Tool: "gitlab_find_action", Arguments: []byte(`{"query":"list issues"}`)}, "")
	t.Logf("%s answered %s and recorded %s", line.Tool, answer.Outcome, line.Outcome)
}
