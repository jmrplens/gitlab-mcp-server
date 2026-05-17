package tools

import "testing"

func TestRegisterMetaCatalog_NilInputs(t *testing.T) {
	RegisterMetaCatalog(nil, nil)
}
