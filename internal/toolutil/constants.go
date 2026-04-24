// constants.go defines shared error message constants used by tool handlers
// across all domain sub-packages.

package toolutil

// ErrMsgContextCanceled is the operation label passed to WrapErr when a tool
// handler detects context cancellation before calling the GitLab API.
const ErrMsgContextCanceled = "context canceled"
