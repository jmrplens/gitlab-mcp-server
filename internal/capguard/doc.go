// Package capguard keeps the methods this server answers in step with the
// capabilities it declares.
//
// The go-sdk wires a handler for every method in the protocol, whether or
// not the server opted into the feature behind it. Two mismatches result
// here: logging/setLevel succeeds with an empty result although the
// handshake never declares the logging capability (this server logs to
// stderr and never emits notifications/message to a session), and on the
// minimal capability surface prompts/list answers a successful empty page
// while the handshake declares no prompts capability. Both pairs have no
// honest reading — a success invites the client to keep asking for a
// feature the handshake already said does not exist, while -32601 says so
// once and for all.
//
// The pattern (and the package name) is shared with the sibling
// libgen-mcp's capguard, which draws the same line for its resource
// methods.
package capguard
