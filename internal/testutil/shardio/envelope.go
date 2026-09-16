// The one rule every record read back from a shard is held to, apart from
// whatever else its own package checks.

package shardio

import "fmt"

// ValidateEnvelope reports what is wrong with the envelope of a record read
// back from a shard: the schema it was written under, whether its type is one
// the reader knows, and whether it carries exactly the payload that type names.
//
// A shard is machine-written, so every one of these means the artifact is stale
// or truncated rather than that a caller made a mistake. Reporting it is what
// keeps a published figure from being computed over lines nobody can read.
//
// The table is the record's own: it answers, per line type, whether the record
// carries the payload that type names, and it is a table rather than a switch
// so that adding a line type is one entry in one place. Both questions the
// reader has are asked of it, whether the type is known and whether its payload
// is there, and it is walked once more to count the payloads carried.
//
// Exactly one payload is the envelope's contract, and the type naming one says
// nothing about the others: a line carrying two is two claims under one type,
// and a reader that took the named half would compute over a record it had only
// half read. Machine-written or not, that is a shard to refuse.
func ValidateEnvelope[R any](record R, schema, want int, lineType string, payloadPresent map[string]func(R) bool) error {
	if schema != want {
		return fmt.Errorf("schema %d is not %d: the shard was written by another version of this package", schema, want)
	}
	present, known := payloadPresent[lineType]
	if !known {
		return fmt.Errorf("unknown line type %q", lineType)
	}
	if !present(record) {
		return fmt.Errorf("line type %q carries no payload", lineType)
	}
	carried := 0
	for _, has := range payloadPresent {
		if has(record) {
			carried++
		}
	}
	if carried != 1 {
		return fmt.Errorf("line type %q carries %d payloads, want exactly one", lineType, carried)
	}
	return nil
}
