package main

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// gateRefusals are the gate literals G8 reads: a Retry-After built from the
// longest block, one from GitLab's own delay or the fixed one, one from the
// fixed one alone, a challenge, a text built in one function and written in
// another, and headers G8 cannot read.
const gateRefusals = `
const upstreamRetryAfter = 30

func retryAfterSeconds(n int) string { return "" }

func itoa(n int) string { return "" }

type upstreamError struct{ RetryAfter int }

func (g *gate) blocked(n int) *gateFailure {
	return &gateFailure{status: 429, code: -42900, message: "Too many attempts.", header: newHeader("Retry-After", retryAfterSeconds(n))}
}

func (g *gate) missing() *gateFailure {
	return &gateFailure{status: 401, code: -40100, message: "Authentication required.", header: newHeader("WWW-Authenticate", "Bearer")}
}

func (g *gate) classify(err *upstreamError) *gateFailure {
	if err != nil {
		delay := err.RetryAfter
		if delay <= 0 {
			delay = upstreamRetryAfter
		}
		return &gateFailure{status: 503, code: -50300, message: "Verify later;", header: newHeader("Retry-After", itoa(delay))}
	}
	var fixed, unused = upstreamRetryAfter, 0
	_ = unused
	return &gateFailure{status: 503, code: -50300, message: "Verify later.", header: newHeader("Retry-After", itoa(fixed))}
}

func (g *gate) missingURLMessage() string { return "Several instances" + " are published." }

func (g *gate) resolveURL(n int) *gateFailure {
	if n > 0 {
		return &gateFailure{status: 400, code: -32600, message: describe(n)}
	}
	return &gateFailure{status: 400, code: -32600, message: g.missingURLMessage()}
}

func (g *gate) resolveNoMessage() *gateFailure {
	return &gateFailure{status: 400, code: -32600}
}

var sharedHeader map[string]string

func (g *gate) unreadableHeader() *gateFailure {
	return &gateFailure{status: 400, code: -32600, message: "Shared.", header: sharedHeader}
}

func (g *gate) headerNameNotConstant(name string) *gateFailure {
	return &gateFailure{status: 400, code: -32600, message: "Named.", header: newHeader(name, "x")}
}

func (g *gate) nilHeader() *gateFailure {
	return &gateFailure{status: 404, code: -32600, message: "Gone.", header: nil}
}

func (g *gate) codeNotConstant(code int) *gateFailure {
	return &gateFailure{status: 404, code: code, message: "Gone."}
}

type holder struct{}
`

// rpcSource is the site package's JSON-RPC and tool-result refusals, in a
// file of its own since the shared header takes the imports.
const rpcSource = `package site

import (
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

const codeBusy = -32000

var ErrTooMany = errors.New("subscriptions: too many active subscriptions")

var errUnbound = &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "this subscription could not be attributed"}

func busy(scope string) error {
	return &jsonrpc.Error{Code: codeBusy, Message: fmt.Sprintf("too many open streams (%s limit %d); close one", scope, 5)}
}

func wire(err error) error {
	var code int64
	switch {
	case errors.Is(err, ErrTooMany):
		code = codeBusy
	default:
		code = jsonrpc.CodeInternalError
	}
	return &jsonrpc.Error{Code: code, Message: err.Error()}
}

var packageCode int64 = -32000

func noCode() error { return &jsonrpc.Error{Message: "no code"} }

func globalCode() error { return &jsonrpc.Error{Code: packageCode} }

func noLiteral() error { return errors.New("plain") }

type result struct {
	IsError bool
	Text    string
}

func errorResult(msg string) *result { return &result{IsError: true, Text: msg} }

func refusedResult() *result { return errorResult("rate limit exceeded for tools/call") }

func unflagged() *result { return &result{Text: "x"} }

func positional() *result { return &result{true, "x"} }

func variableFlag(b bool) *result { return &result{IsError: b} }

func fromValue(f func() *result) *result { return f() }

func fromVariable(r *result) *result { return r }

func bare() (r *result) { return }

func throughUnflagged() *result { return unflagged() }

func withClosure() *result {
	later := func() *result { return nil }
	_ = later
	return &result{IsError: true}
}

func recursive(n int) *result {
	if n > 0 {
		return recursive(n - 1)
	}
	return nil
}

func noResult() error { return nil }
`

// refusalFixture runs rows of refusals over the gate and RPC fixtures.
func refusalFixture(t *testing.T, refusals ...tenancy.Refusal) Report {
	t.Helper()
	d := row("ROW-001")
	d.Refusals = refusals
	return fixture{
		files: map[string]string{"site/site.go": gateSource + gateRefusals, "site/rpc.go": rpcSource},
		rows:  []tenancy.Decision{d},
	}.run(t)
}

// gateAt is a gate refusal whose text and literal are in the site function
// named at.
func gateAt(at string, status, code int, prefix string) tenancy.Refusal {
	return tenancy.Refusal{
		Methods: []string{tenancy.MethodGate}, Channel: tenancy.Gate, Status: status, Code: code, Prefix: prefix,
		At: site(at, tenancy.Refuse),
	}
}

// TestCheckRefusals_GateLiteralsThatCarryTheirRow_Pass: each Retry-After
// source reads what it names, a challenge is carried where the row says, a
// text built in one function passes to a literal in another, and a refusal
// with no text passes on its status and code alone.
func TestCheckRefusals_GateLiteralsThatCarryTheirRow_Pass(t *testing.T) {
	blocked := gateAt("gate.blocked", 429, -42900, "Too many attempts.")
	blocked.RetryAfter = tenancy.RetryAfterLongestBlock
	missing := gateAt("gate.missing", 401, -40100, "Authentication required.")
	missing.Challenge = true
	upstream := gateAt("gate.classify", 503, -50300, "Verify later;")
	upstream.RetryAfter = tenancy.RetryAfterUpstreamOrFixed
	fixed := gateAt("gate.classify", 503, -50300, "Verify later.")
	fixed.RetryAfter = tenancy.RetryAfterFixed
	via := gateAt("gate.missingURLMessage", 400, -32600, "Several instances are")
	via.Via = site("gate.resolveURL", tenancy.Refuse)
	// A literal that carries no message cannot be the one the text reached,
	// and is still the only candidate its function holds.
	viaNoMessage := gateAt("gate.missingURLMessage", 400, -32600, "Several")
	viaNoMessage.Via = site("gate.resolveNoMessage", tenancy.Refuse)
	report := refusalFixture(t, blocked, missing, upstream, fixed, via, viaNoMessage,
		gateAt("gate.nilHeader", 404, -32600, ""), gateAt("gate.resolveURL", 400, -32600, ""))
	assertFindings(t, report, "G8")
	if report.Summary.Refusals != 8 {
		t.Fatalf("refusals read = %d, want 8", report.Summary.Refusals)
	}
}

// TestCheckRefusals_AGateLiteralThatDrifted_IsAFinding: reworded text, a
// changed status, a changed code, a Retry-After gained or lost or read from
// the wrong source, a challenge gained or lost, and headers the gate cannot
// read each fail until the row changes with them.
func TestCheckRefusals_AGateLiteralThatDrifted_IsAFinding(t *testing.T) {
	missingRetry := gateAt("gate.blocked", 429, -42900, "Too many attempts.")
	wrongSource := gateAt("gate.blocked", 429, -42900, "Too many attempts.")
	wrongSource.RetryAfter = tenancy.RetryAfterFixed
	gainedRetry := gateAt("gate.missing", 401, -40100, "Authentication required.")
	gainedRetry.RetryAfter = tenancy.RetryAfterFixed
	gainedRetry.Challenge = true
	lostChallenge := gateAt("gate.missing", 401, -40100, "Authentication required.")
	report := refusalFixture(t,
		gateAt("gate.blocked", 429, -42900, "Too many failed"),
		gateAt("gate.blocked", 403, -42900, ""),
		gateAt("gate.nilHeader", 404, -40300, ""),
		gateAt("gate.codeNotConstant", 404, -32600, ""),
		missingRetry, wrongSource, gainedRetry, lostChallenge,
		gateAt("gate.unreadableHeader", 400, -32600, "Shared."),
		gateAt("gate.headerNameNotConstant", 400, -32600, "Named."),
		gateAt("gate.gone", 400, -32600, ""),
		gateAt("holder", 400, -32600, ""),
	)
	key := func(name string) string { return siteDir + ":" + name }
	assertFindings(t, report, "G8",
		"ROW-001 refusal 1 (http gate): no string "+key("gate.blocked")+" folds begins with \"Too many failed\"",
		"ROW-001 refusal 2 (http gate): "+key("gate.blocked")+" holds no gate refusal with status 403 whose text begins \"\"",
		"ROW-001 refusal 3 (http gate): "+key("gate.nilHeader")+" its code is -32600, and the register says -40300",
		"ROW-001 refusal 4 (http gate): "+key("gate.codeNotConstant")+" its code is not a constant, and the register says -32600",
		"ROW-001 refusal 5 (http gate): "+key("gate.blocked")+" it carries Retry-After true, and the register says false",
		"ROW-001 refusal 6 (http gate): "+key("gate.blocked")+" its Retry-After does not read upstreamRetryAfter",
		"ROW-001 refusal 7 (http gate): "+key("gate.missing")+" it carries Retry-After false, and the register says true",
		"ROW-001 refusal 8 (http gate): "+key("gate.missing")+" it carries WWW-Authenticate true, and the register says false",
		"ROW-001 refusal 9 (http gate): "+key("gate.unreadableHeader")+" its headers are built where the gate cannot read them",
		"ROW-001 refusal 10 (http gate): "+key("gate.headerNameNotConstant")+" its headers are built where the gate cannot read them",
		"ROW-001 refusal 12 (http gate): "+key("holder")+" holds no gate refusal with status 400 whose text begins \"\"",
	)
}

// TestCheckRefusals_RPCRefusals: a code that folds, a code held in a variable
// and counted as every constant the function assigns it, and a holder that
// is a variable, pass; a holder with no JSON-RPC literal, one whose literals
// carry another code, set none, or read a package variable, fail.
func TestCheckRefusals_RPCRefusals(t *testing.T) {
	rpc := func(at string, code int, prefix, via string) tenancy.Refusal {
		r := tenancy.Refusal{
			Methods: []string{"resources/read"}, Channel: tenancy.RPC, Code: code, Prefix: prefix,
			At: site(at, tenancy.Refuse),
		}
		if via != "" {
			r.Via = site(via, tenancy.Refuse)
		}
		return r
	}
	report := refusalFixture(t,
		rpc("busy", -32000, "too many open streams (", ""),
		rpc("ErrTooMany", -32000, "subscriptions: too many active", "wire"),
		rpc("errUnbound", -32603, "this subscription could not", ""),
		rpc("busy", -32000, "too many open streams (%s", ""),
		rpc("noLiteral", -32000, "", ""),
		rpc("busy", -32602, "", ""),
		rpc("noCode", -32000, "", ""),
		rpc("globalCode", -32000, "", ""),
		rpc("gate.missing", -40100, "", ""),
	)
	assertFindings(t, report, "G8",
		"ROW-001 refusal 4 (resources/read rpc): no string "+siteDir+":busy folds begins with \"too many open streams (%s\"",
		"ROW-001 refusal 5 (resources/read rpc): "+siteDir+":noLiteral builds no JSON-RPC error",
		"ROW-001 refusal 6 (resources/read rpc): "+siteDir+":busy builds no JSON-RPC error carrying code -32602 (it carries [-32000])",
		"ROW-001 refusal 7 (resources/read rpc): "+siteDir+":noCode builds no JSON-RPC error carrying code -32000 (it carries [])",
		"ROW-001 refusal 8 (resources/read rpc): "+siteDir+":globalCode builds no JSON-RPC error carrying code -32000 (it carries [])",
		"ROW-001 refusal 9 (resources/read rpc): "+siteDir+":gate.missing builds no JSON-RPC error",
	)
}

// TestCheckRefusals_ToolErrorRefusals: a result flagged as an error, directly
// or through the function it returns, passes, and so does a holder that
// returns no tool result; one that returns a result it does not flag, flags
// with a value that is not constant, returns through a value or a variable,
// or writes it positionally, fails.
func TestCheckRefusals_ToolErrorRefusals(t *testing.T) {
	r := fixtureRules()
	toolError := func(at string) tenancy.Refusal {
		return tenancy.Refusal{Methods: []string{"tools/call"}, Channel: tenancy.ToolError, At: site(at, tenancy.Refuse)}
	}
	d := row("ROW-001")
	d.Refusals = []tenancy.Refusal{
		toolError("refusedResult"), toolError("errorResult"), toolError("noResult"), toolError("codeBusy"),
		toolError("bare"), toolError("recursive"),
		toolError("unflagged"), toolError("variableFlag"), toolError("fromValue"), toolError("fromVariable"), toolError("positional"),
		toolError("withClosure"), toolError("throughUnflagged"),
	}
	report := fixture{
		files: map[string]string{"site/site.go": gateSource + gateRefusals, "site/rpc.go": rpcSource},
		rows:  []tenancy.Decision{d},
		rules: &r,
	}.run(t)
	at := func(line string) string {
		return fmt.Sprintf("%s/site/rpc.go:%d", fixtureDir, lineOf(t, rpcSource, line))
	}
	assertFindings(t, report, "G8",
		"ROW-001 refusal 7 (tools/call tool-error): "+siteDir+":unflagged returns a tool result at "+at(`func unflagged() *result { return &result{Text: "x"} }`)+" without IsError set",
		"ROW-001 refusal 8 (tools/call tool-error): "+siteDir+":variableFlag returns a tool result at "+at(`func variableFlag(b bool) *result { return &result{IsError: b} }`)+" without IsError set",
		"ROW-001 refusal 9 (tools/call tool-error): "+siteDir+":fromValue returns a tool result from f, which the gate cannot read",
		"ROW-001 refusal 10 (tools/call tool-error): "+siteDir+":fromVariable returns r, which the gate cannot read as a tool result",
		"ROW-001 refusal 11 (tools/call tool-error): "+siteDir+":positional returns a tool result at "+at(`func positional() *result { return &result{true, "x"} }`)+" without IsError set",
		"ROW-001 refusal 13 (tools/call tool-error): "+siteDir+":throughUnflagged returns a tool result at "+at(`func unflagged() *result { return &result{Text: "x"} }`)+" without IsError set",
	)
}

// TestCheckRefusals_AnUnresolvedSite_IsLeftToG1: a refusal whose text or
// literal names nothing is not judged here, since G1 already says so.
func TestCheckRefusals_AnUnresolvedSite_IsLeftToG1(t *testing.T) {
	via := gateAt("gate.missing", 401, -40100, "")
	via.Via = site("gone", tenancy.Refuse)
	report := refusalFixture(t, gateAt("gone", 401, -40100, ""), via)
	assertFindings(t, report, "G8")
	if report.Summary.Refusals != 1 {
		t.Fatalf("refusals read = %d, want the one whose text resolved", report.Summary.Refusals)
	}
}

// TestBeforeVerb_ReadsAFormatUpToItsFirstVerb: an escaped percent is the
// percent it prints, and the first real verb ends the text.
func TestBeforeVerb_ReadsAFormatUpToItsFirstVerb(t *testing.T) {
	for format, want := range map[string]string{
		"plain":               "plain",
		"streams (%s limit)":  "streams (",
		"100%% sure %d times": "100% sure ",
		"trailing %":          "trailing ",
	} {
		t.Run(format, func(t *testing.T) {
			if got := beforeVerb(format); got != want {
				t.Fatalf("beforeVerb(%q) = %q, want %q", format, got, want)
			}
		})
	}
}

// TestLeadingText_ReadsWhatAnExpressionBeginsWith: a constant whole, the left
// operand of a concatenation, a format up to its verb, and nothing for a call
// that is not a format or a format that does not fold.
func TestLeadingText_ReadsWhatAnExpressionBeginsWith(t *testing.T) {
	source := `package site

import "fmt"

var format = "%s"

var (
	whole       = "Whole."
	concatenated = "Left " + fmt.Sprint(1) + " right"
	formatted   = fmt.Sprintf("Streams (%s)", "x")
	notAFormat  = fmt.Sprint("x")
	notFolded   = fmt.Sprintf(format, "x")
	errorf      = fmt.Errorf("Refused %d", 1)
)
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}})
	g := &gate{p: p, rules: fixtureRules()}
	for name, want := range map[string]string{
		"whole": "Whole.", "concatenated": "Left ", "formatted": "Streams (", "notAFormat": "", "notFolded": "", "errorf": "Refused ",
	} {
		t.Run(name, func(t *testing.T) {
			decl := p.decls[siteDir+":"+name]
			if got := g.leadingText(decl.info(), decl.initializer()); got != want {
				t.Fatalf("leadingText = %q, want %q", got, want)
			}
		})
	}
}

// TestFoldedTexts_ReadsAFormatOnlyUpToItsVerb: the format of a formatting
// call is read up to its verb and never whole, while its other arguments are
// read like anything else.
func TestFoldedTexts_ReadsAFormatOnlyUpToItsVerb(t *testing.T) {
	source := `package site

import "fmt"

const suffix = "; retry"

func Refuse(n int) string {
	return fmt.Sprintf("Refused (%d)"+suffix, n, "argument")
}
`
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}})
	g := &gate{p: p, rules: fixtureRules()}
	texts := g.foldedTexts(p.decls[siteDir+":Refuse"])
	if !slices.Contains(texts, "Refused (") || !slices.Contains(texts, "argument") || slices.Contains(texts, "Refused (%d); retry") {
		t.Fatalf("texts = %q, want the format up to its verb and the argument, never the format whole", texts)
	}
	if g.foldedTexts(&declaration{typ: &ast.TypeSpec{}}) != nil {
		t.Fatal("a declaration with no body folded texts")
	}
}

// TestRefusalLiteral_ReadsKeyedAndPositionalFields: a positional literal is
// read by field order, and a literal of a type that is no struct is read as
// far as its keyed elements go.
func TestRefusalLiteral_ReadsKeyedAndPositionalFields(t *testing.T) {
	source := `package site

type failure struct {
	status int
	code   int
}

type codes []int

var (
	keyed      = failure{status: 400, code: -32600}
	positional = failure{403, -40300}
	slice      = codes{0: -32000}
)
`
	r := fixtureRules()
	r.refusalTypes = []refusalType{
		{name: sitePath + ".failure", code: "code", status: "status"},
		{name: sitePath + ".codes", code: "code"},
	}
	p := programOf(t, fixture{files: map[string]string{"site/site.go": source}, rules: &r})
	g := &gate{p: p, rules: r}
	for name, want := range map[string][2]int{"keyed": {400, -32600}, "positional": {403, -40300}, "slice": {0, 0}} {
		t.Run(name, func(t *testing.T) {
			decl := p.decls[siteDir+":"+name]
			rl, ok := g.refusalLiteral(decl.info(), decl.initializer().(*ast.CompositeLit))
			if !ok {
				t.Fatal("not read as a refusal literal")
			}
			status, _ := rl.intField(decl.info(), rl.typ.status)
			code, _ := rl.intField(decl.info(), rl.typ.code)
			if [2]int{status, code} != want {
				t.Fatalf("status and code = %d, %d, want %v", status, code, want)
			}
		})
	}
}

// TestSymbolsRead_FollowsLocalsAndSkipsWhatIsUnresolved: an identifier with
// no object is passed over rather than followed.
func TestSymbolsRead_FollowsLocalsAndSkipsWhatIsUnresolved(t *testing.T) {
	p := programOf(t, fixture{files: map[string]string{"site/site.go": gateSource + gateRefusals}})
	g := &gate{p: p, rules: fixtureRules()}
	decl := p.decls[siteDir+":gate.classify"]
	read := g.symbolsRead(decl, &ast.BinaryExpr{X: ast.NewIdent("unresolved"), Op: token.ADD, Y: ast.NewIdent("alsoUnresolved")})
	if len(read) != 0 {
		t.Fatalf("read = %v, want nothing from identifiers with no object", read)
	}
}

// TestTypeName_NamesANamedTypeThroughAPointer, and nothing for a type with no
// name or no package.
func TestTypeName_NamesANamedTypeThroughAPointer(t *testing.T) {
	p := programOf(t, fixture{files: map[string]string{"site/site.go": gateSource}})
	obj := p.byDir[siteDir].Types.Scope().Lookup("gateFailure")
	if got := typeName(obj.Type()); got != sitePath+".gateFailure" {
		t.Fatalf("typeName = %q", got)
	}
	if got := typeName(p.byDir[siteDir].Types.Scope().Lookup("describe").Type()); got != "" {
		t.Fatalf("typeName of a signature = %q, want empty", got)
	}
	if got := typeName(types.Universe.Lookup("error").Type()); got != "" {
		t.Fatalf("typeName of error = %q, want empty: it has no package", got)
	}
}
