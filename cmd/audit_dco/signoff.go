package main

import "strings"

// identity is a person or an app as git records one: a name and an email.
type identity struct{ name, email string }

// String writes the identity the way git and a Signed-off-by trailer do.
func (id identity) String() string { return id.name + " <" + id.email + ">" }

// parseIdentity reads a trailer value as a name followed by an email in angle
// brackets that end the value, the space around each trimmed. It names nobody
// unless both are there, the name holds no closing bracket (it ends at the
// first opening one, and git keeps neither in a name, so such a name could
// never be an author's), and the email holds an at sign and no angle bracket
// or blank.
func parseIdentity(value string) (identity, bool) {
	name, rest, _ := strings.Cut(strings.TrimSpace(value), "<")
	email, closed := strings.CutSuffix(rest, ">")
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if !closed || name == "" || strings.ContainsRune(name, '>') ||
		!strings.Contains(email, "@") || strings.ContainsAny(email, "<> \t") {
		return identity{}, false
	}
	return identity{name: name, email: email}, true
}

// sameIdentity reports whether a and b name the same identity: the same name
// and the same email, each compared without regard to case, since neither is
// case-sensitive to the person it names and git commit --signoff copies both
// from the configuration that wrote the author.
func sameIdentity(a, b identity) bool {
	return strings.EqualFold(a.name, b.name) && strings.EqualFold(a.email, b.email)
}

// shortHash is how many characters of a hash the report prints.
const shortHash = 12

// commit is one commit of the range, as git records it: its hash, how many
// parents it has, its author, its subject, the values of its Signed-off-by
// trailers in the order they appear, and whether it is a merge of two
// parents that brings no change of its own (markCleanMerges decides).
type commit struct {
	hash       string
	parents    int
	author     identity
	subject    string
	signoffs   []string
	cleanMerge bool
}

// short is the commit's hash abbreviated for the report.
func (c commit) short() string { return c.hash[:min(len(c.hash), shortHash)] }

// outcome is what judging one commit decided.
type outcome int

// The outcomes. Only unsigned and signedByOthers fail a run.
const (
	// signedOff is a commit one of whose trailers names its author.
	signedOff outcome = iota
	// unsigned is a commit with no Signed-off-by trailer.
	unsigned
	// signedByOthers is a commit whose trailers name only other people.
	signedByOthers
	// exempt is a declared bot's commit in a pull request that bot opened.
	exempt
	// merge is a merge commit that brings no change of its own, which is
	// not judged.
	merge
)

// verdict is one judged commit, its outcome, and the declared bot that
// exempts it or whose identity a failing commit carries.
type verdict struct {
	commit  commit
	outcome outcome
	bot     *declaredBot
}

// failed reports whether the verdict fails the run.
func (v verdict) failed() bool { return v.outcome == unsigned || v.outcome == signedByOthers }

// judge decides one commit of a pull request openedBy opened. A merge that
// brings no change of its own is not judged, a declared bot's commit in a
// pull request that bot opened is exempt, and any other commit, any other
// merge included, passes only when a Signed-off-by trailer names its author.
// A failing commit that carries a declared bot's identity keeps the bot, so
// the report can say why the exemption did not hold.
func judge(c commit, openedBy string) verdict {
	if c.cleanMerge {
		return verdict{commit: c, outcome: merge}
	}
	if bot, ok := exemptingBot(c.author, openedBy); ok {
		return verdict{commit: c, outcome: exempt, bot: &bot}
	}
	for _, value := range c.signoffs {
		if signer, ok := parseIdentity(value); ok && sameIdentity(signer, c.author) {
			return verdict{commit: c, outcome: signedOff}
		}
	}
	failing := verdict{commit: c, outcome: unsigned}
	if len(c.signoffs) > 0 {
		failing.outcome = signedByOthers
	}
	if bot, ok := claimedBot(c.author); ok {
		failing.bot = &bot
	}
	return failing
}
