//go:build e2e

package modeleval

import (
	"example.com/e2efake/test/e2e/internal/harness"
)

// call uses the one harness export nothing else in the fixture touches, so the
// dead-export rule has to read this package to find its user.
func call() { harness.ModelOnly() }

// Run is the package's own entry point, here only so the function above is not
// itself unused.
func Run() { call() }
