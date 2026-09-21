// Package legacy sits under test/e2e/gitlab and is none of the three runtime
// packages, which is the placement finding: the gate reads the runtime off
// the directory name, so a fourth one is a package whose runtime nothing can
// say, and every id in it is judged against a placement that does not exist.
package legacy
