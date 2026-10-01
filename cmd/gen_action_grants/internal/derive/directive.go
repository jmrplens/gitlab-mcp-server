package derive

import (
	"fmt"
	"go/token"
	"strings"

	"golang.org/x/tools/go/packages"
)

// DirectivePrefix is how a request directive starts.
const DirectivePrefix = "//gitlab:request"

// DirectiveKind is what a directive says about the requests it qualifies.
type DirectiveKind string

// The directive kinds.
const (
	// DirectiveOptional says the requests may not run on a call that
	// succeeds.
	DirectiveOptional DirectiveKind = "optional"
	// DirectiveMandatory says every request of the statement runs on every
	// call that succeeds: a loop or an if around it is taken, and a branch
	// that sends nothing is not a way the call succeeds.
	DirectiveMandatory DirectiveKind = "mandatory"
	// DirectiveAlternatives says exactly one of the statement's requests runs.
	DirectiveAlternatives DirectiveKind = "alternatives"
)

// Directive is one //gitlab:request comment, qualifying the statement written
// on the line after it.
type Directive struct {
	Kind   DirectiveKind
	Reason string
	// Position is where the comment is written, which is what a stale
	// directive is reported at.
	Position token.Position
	// used is set once the directive qualified a request some action reaches.
	used bool
}

// String renders the directive the way the record names it, without a line:
// a line moves with every unrelated edit above it.
func (d *Directive) String() string { return string(d.Kind) + ": " + d.Reason }

// directives is every directive of the loaded source, by the statement each
// qualifies.
type directives struct {
	// byTarget maps "file:line" of the qualified statement to its directive.
	byTarget map[string]*Directive
	all      []*Directive
	// malformed are comments that start like a directive and say nothing a
	// reader can apply, each already a finding.
	malformed []string
}

// collectDirectives reads every request directive out of the loaded packages.
//
// A directive qualifies the statement starting on the line after its comment
// group, so it can sit below an ordinary comment that explains the code; one
// with an unknown kind or without a reason is malformed, since a directive is
// read by a person deciding whether it still holds.
func collectDirectives(position func(token.Pos) token.Position, pkgs []*packages.Package) *directives {
	found := &directives{byTarget: map[string]*Directive{}}
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, group := range file.Comments {
				for _, comment := range group.List {
					rest, ok := strings.CutPrefix(comment.Text, DirectivePrefix)
					if !ok {
						continue
					}
					at := position(comment.Pos())
					directive, err := parseDirective(rest)
					if err != nil {
						found.malformed = append(found.malformed, fmt.Sprintf("%s: %v", at, err))
						continue
					}
					directive.Position = at
					target := position(group.End())
					found.byTarget[targetKey(target.Filename, target.Line+1)] = directive
					found.all = append(found.all, directive)
				}
			}
		}
	}
	return found
}

// parseDirective reads what follows the prefix: " kind: reason".
func parseDirective(rest string) (*Directive, error) {
	// A directive with no colon has no reason either: Cut answers an empty
	// one for it.
	kind, reason, _ := strings.Cut(strings.TrimSpace(rest), ":")
	kind, reason = strings.TrimSpace(kind), strings.TrimSpace(reason)
	switch DirectiveKind(kind) {
	case DirectiveOptional, DirectiveMandatory, DirectiveAlternatives:
	default:
		return nil, fmt.Errorf("%s names %q, which is not optional, mandatory or alternatives", DirectivePrefix, kind)
	}
	if reason == "" {
		return nil, fmt.Errorf("%s %s gives no reason", DirectivePrefix, kind)
	}
	return &Directive{Kind: DirectiveKind(kind), Reason: reason}, nil
}

// targetKey names one line of one file.
func targetKey(file string, line int) string { return fmt.Sprintf("%s:%d", file, line) }

// at returns the directive qualifying the statement at one position, if one
// does.
func (d *directives) at(position token.Position) *Directive {
	return d.byTarget[targetKey(position.Filename, position.Line)]
}

// unused lists every directive no action's requests went through, by where it
// is written, each a finding: a directive that qualifies nothing is one a
// later edit left behind, and it would quietly qualify whatever the next edit
// puts on that line.
func (d *directives) unused() []string {
	var found []string
	for _, directive := range d.all {
		if !directive.used {
			found = append(found, fmt.Sprintf("%s: %s qualifies no request any action reaches", directive.Position, DirectivePrefix))
		}
	}
	return found
}
