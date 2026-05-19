package tools

import "testing"

// TestRegisterMetaCatalog_NilInputs verifies nil server or catalog inputs are
// ignored without panicking.
//
// Meta catalog registration is called from configurable startup paths; accepting
// nil inputs keeps defensive tests and partial setup flows from crashing.
func TestRegisterMetaCatalog_NilInputs(t *testing.T) {
	RegisterMetaCatalog(nil, nil)
}
