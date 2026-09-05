// Command audit_gateway_chars scans everything a client receives from
// tools/list, prompts/list and resources/list, on every tool surface and at the
// widest tier, for characters that MCP gateway validators reject.
//
// It exists because of a real rejection: a gateway introspecting this server
// refused onboarding with "Description contains unsafe characters: ';'". The
// semicolons were ordinary English punctuation, but the gateway is the door,
// and the door's rules win. This audit measures the served surface rather than
// grepping the source, because what matters is what crosses the wire: a
// description is assembled from several source strings, and a semicolon that
// survives assembly is a rejection wherever it came from.
//
// The policy it enforces is pure ASCII prose plus a short list of rejected
// ASCII characters (the semicolon): a validator that refuses "unsafe
// characters" usually matches a character class, so holding a class is the
// only version of clean that the next gateway cannot surprise.
//
// With -check it exits non-zero when any offending character is served, which
// is the CI gate; without it, it prints every offender with enough context to
// find the source string.
package main
