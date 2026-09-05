// Command audit_e2e_gaps reports which canonical catalog actions are not
// exercised by the e2e suite under test/e2e/suite.
//
// It builds the Ultimate-tier action catalog offline and scans the suite
// sources for the three invocation shapes: individual tool names
// ("gitlab_branch_create"), meta calls (a "gitlab_branch" literal followed by
// an "action": "create" pair within a short window), and dynamic execute
// calls referencing canonical "domain.action" IDs. An action counts as
// exercised when any surface references it.
package main
