// rpc_code.go classifies errors with the JSON-RPC codes the MCP specification
// names, so a client can tell what it is expected to do about a failure.
package toolutil

import (
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

// CodedError carries a JSON-RPC code alongside the original error.
//
// It exists because the go-sdk derives the wire code from the error value: its
// toWireError leaves Code at 0 unless the error is, or wraps, a
// *jsonrpc.Error. An unclassified error therefore reaches the client as code 0,
// which is not a JSON-RPC error code at all, and a client cannot tell "you sent
// the wrong arguments" from "the upstream is down" — the two failures it would
// act on differently.
//
// Unwrap lists the cause before the code, so errors.As finds the innermost code
// first. That ordering is deliberate in both directions: a handler that wraps
// an argument failure in its own context still reports the argument code, and a
// cause that already carries a specific code (a not-found, say) keeps it rather
// than being flattened into the generic one.
type CodedError struct {
	cause error
	code  int64
}

func (e *CodedError) Error() string { return e.cause.Error() }

func (e *CodedError) Unwrap() []error {
	return []error{e.cause, &jsonrpc.Error{Code: e.code}}
}

// InvalidParams marks an error as JSON-RPC -32602: the caller sent something
// missing, empty or unparseable, and has to change it.
func InvalidParams(err error) error {
	return coded(err, jsonrpc.CodeInvalidParams)
}

// InternalError marks an error as JSON-RPC -32603: an upstream call failed or a
// response would not decode, and there is nothing the caller can change.
func InternalError(err error) error {
	return coded(err, jsonrpc.CodeInternalError)
}

func coded(err error, code int64) error {
	if err == nil {
		return nil
	}
	return &CodedError{cause: err, code: code}
}
