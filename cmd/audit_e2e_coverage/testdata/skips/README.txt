The go test -json streams of the two complete Docker runs issue 1014 describes,
cut down to what the skip gate reads: make test-e2e-ce and make test-e2e-ee of
main at de1ab3b49, run on 2026-09-27 against the cached gitlab/gitlab-ce 19.3.0
and gitlab/gitlab-ee 19.3.1-ee images.

Every line is copied verbatim from the stream gotestsum wrote to
dist/e2e-reports/e2e-<runtime>-log.json. Kept whole are the package-level
start and verdict events, every event of each test with a skipped test under
it, and every event of one test that passed with nothing skipped
(TestWorkItemTypes_List_HoldsTheIssueType), so a fixture holds skipped
subtests beside passing siblings and a parent that passed around them.

ce-de1ab3b49.json holds the CE run's eleven skips, ee-de1ab3b49.json the EE
run's ten. The EE run failed elsewhere, which is why its package verdicts are
fail; the gate reads skips whatever the run's verdict.
