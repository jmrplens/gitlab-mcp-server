package main

import (
	"slices"
	"testing"

	"github.com/jmrplens/gitlab-mcp-server/v3/internal/tenancy"
)

// tripwireRules lists the fixture's own constructor and options type beside
// the production ones, so part (a) has something of the site package's to
// find.
func tripwireRules() *rules {
	r := fixtureRules()
	r.constructors = append(r.constructors, sitePath+".NewLimiter")
	r.limitTypes = append(r.limitTypes, sitePath+".Options")
	return &r
}

// constructorSource builds limits inside a declared site and outside one,
// with arguments of every shape part (a) reads.
const constructorSource = siteHeader + `
type config struct {
	Rate  int
	Other int
}

type limiter struct{}

func (limiter) Size() int { return 1 }

func NewLimiter(rate, burst int, strict bool) *limiter { return &limiter{} }

type Options struct {
	Max   int
	Lease time.Duration
}

const declared = 10

const undeclared = 20

var declaredLimiter = NewLimiter(declared, 1, false)

var settings config

func Build(cfg config, l limiter) {
	n := 3
	_ = NewLimiter(cfg.Rate, n, true)
	_ = NewLimiter(cfg.Other, l.Size(), false)
	_ = NewLimiter(settings.Rate, n, true)
	_ = NewLimiter(settings.Other, n, false)
	_ = NewLimiter(undeclared, 2, false)
	_ = make(chan struct{}, undeclared)
	_ = make(chan struct{}, 5)
	_ = NewLimiter(int(time.Nanosecond), n, false)
	_ = Options{Max: cfg.Rate, Lease: time.Duration(n)}
}

func Loose(n int) {
	_ = NewLimiter(n, n, false)
	_ = Options{Max: 7}
}

func semaphores(n int) any {
	a := make(chan struct{}, n)
	b := make(chan struct{}, 1)
	c := make(chan struct{}, 0)
	d := make(chan struct{})
	e := make(chan int, n)
	f := make(chan struct{ x int }, n)
	g := make([]int, n)
	return []any{a, b, c, d, e, f, g}
}
`

// shadowSource declares a function called make in a package of its own,
// whose calls are not the builtin's even where they take what a semaphore
// would.
const shadowSource = `package other

func make(ch chan struct{}, n int) chan struct{} { return ch }

func Use() chan struct{} {
	var ch chan struct{}
	return make(ch, 5)
}
`

// TestCheckTripwire_Constructors: a limit built inside a declared Enforce site
// from declared inputs, a configuration field (of a package variable too) and
// locals passes, and so does a literal of a limit's options type built the
// same way; one built outside every Enforce site (a site declared in another
// role among them), a literal argument or field and an undeclared
// package-level argument, a package variable's undeclared field among them,
// fail; a make of a channel of empty structs with a capacity other than 0 or 1
// is a semaphore, and a function the program declares called make is not the
// builtin.
func TestCheckTripwire_Constructors(t *testing.T) {
	d := row("ROW-001", site("declared", tenancy.Enforce), site("declaredLimiter", tenancy.Enforce), site("Build", tenancy.Enforce),
		site("Loose", tenancy.Refuse))
	d.Config = []string{"Rate"}
	report := fixture{
		files: map[string]string{"site/site.go": constructorSource, "other/other.go": shadowSource},
		rows:  []tenancy.Decision{d},
		rules: tripwireRules(),
	}.run(t)
	assertFindings(t, report, "G10",
		siteDir+":Build: passes "+siteDir+":undeclared, which no row declares to "+sitePath+".NewLimiter",
		siteDir+":Build: passes "+siteDir+":undeclared, which no row declares to make",
		siteDir+":Build: passes the literal 2 to "+sitePath+".NewLimiter",
		siteDir+":Build: passes the literal 5 to make",
		siteDir+":Build: passes "+siteDir+":settings, which no row declares to "+sitePath+".NewLimiter",
		siteDir+":Loose: builds a limit with "+sitePath+".NewLimiter outside every declared Enforce site",
		siteDir+":Loose: builds a limit with "+sitePath+".Options outside every declared Enforce site",
		siteDir+":Loose: passes the literal 7 to "+sitePath+".Options",
		siteDir+":declaredLimiter: passes the literal 1 to "+sitePath+".NewLimiter",
		siteDir+":semaphores: builds a limit with make outside every declared Enforce site",
	)
}

// TestCheckTripwire_AnExemptedConstructor_IsAnswered: the exemption table
// answers part (a) as it answers the others, and records which part it did.
func TestCheckTripwire_AnExemptedConstructor_IsAnswered(t *testing.T) {
	report := fixture{
		files:  map[string]string{"site/site.go": constructorSource},
		rows:   []tenancy.Decision{row("ROW-001", site("Build", tenancy.Enforce), site("declared", tenancy.Enforce))},
		rules:  tripwireRules(),
		exempt: map[string]exemption{siteDir + ":Loose": {categoryParsing, "a fixture"}, siteDir + ":undeclared": {categoryParsing, "a fixture"}},
	}.run(t)
	for _, f := range findings(report, "G10") {
		if f == siteDir+":Loose: builds a limit with "+sitePath+".NewLimiter outside every declared site" {
			t.Fatalf("an exempted constructor call was reported: %q", f)
		}
	}
	want := []Excuse{
		{Key: siteDir + ":Loose", Part: partConstructors, Category: categoryParsing, Reason: "a fixture"},
		{Key: siteDir + ":undeclared", Part: partConstructors, Category: categoryParsing, Reason: "a fixture"},
	}
	if !slices.Equal(report.Excused, want) {
		t.Fatalf("excused = %+v, want %+v", report.Excused, want)
	}
}

// literalSource builds refusals of every shape part (b) tells apart.
const literalSource = gateSource + `
func (g *gate) declared() *gateFailure {
	return &gateFailure{status: 429, code: -42900}
}

func (g *gate) outside(code int) []*gateFailure {
	return []*gateFailure{
		{status: 400, code: -40300},
		{status: 400, code: code},
		{status: 503, code: -32603},
		{status: 400, code: -32600},
		{status: 404},
	}
}

func (g *gate) exempted() *gateFailure {
	return &gateFailure{status: 400, code: -32000}
}
`

// literalMessage is what part (b) says of a refusal literal no row declares.
const literalMessage = "builds a refusal that reads as a limit's (a policy code, a code that is not constant, or a 429 or 503) that no refusal a row declares there carries"

// TestCheckTripwire_RefusalLiterals: a policy code, a code that is not
// constant and a 503 outside every site fail; a protocol code under a 400 and
// a literal that sets no code pass; a literal the refusal a row declares
// there carries, or an exemption, is answered.
func TestCheckTripwire_RefusalLiterals(t *testing.T) {
	d := row("ROW-001", site("gate.declared", tenancy.Refuse))
	// A gate refusal of another status in the same function declares none of
	// gate.outside's literals.
	d.Refusals = []tenancy.Refusal{
		{Channel: tenancy.Gate, Status: 429, At: site("gate.declared", tenancy.Refuse)},
		{Channel: tenancy.Gate, Status: 429, At: site("gate.outside", tenancy.Refuse)},
	}
	report := fixture{
		files:  map[string]string{"site/site.go": literalSource},
		rows:   []tenancy.Decision{d},
		exempt: map[string]exemption{siteDir + ":gate.exempted": {categoryServerState, "a fixture"}},
	}.run(t)
	// The shared gate's two constructors build a 401 and a 503 outside any
	// site of this fixture's register, and are held to it like the rest.
	assertFindings(t, report, "G10",
		siteDir+":gate.outside: "+literalMessage, siteDir+":gate.outside: "+literalMessage, siteDir+":gate.outside: "+literalMessage,
		siteDir+":gate.rejected: "+literalMessage, siteDir+":gate.upstream: "+literalMessage)
	if len(report.Excused) != 1 || report.Excused[0].Part != partLiterals {
		t.Fatalf("excused = %+v, want the one literal exemption", report.Excused)
	}
}

// writerSource writes statuses outside a gate literal, and builds a JSON-RPC
// refusal in a function a row declares a refusal of another code in.
const writerSource = `package site

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func tooMany(w http.ResponseWriter) { http.Error(w, "slow down", http.StatusTooManyRequests) }

func draining(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) }

func throttled(w http.ResponseWriter) { w.WriteHeader(http.StatusServiceUnavailable) }

func fine(w http.ResponseWriter) { w.WriteHeader(http.StatusOK) }

func echoed(w http.ResponseWriter, status int) { w.WriteHeader(status) }

func overLimit() error { return &jsonrpc.Error{Code: -42900, Message: "over"} }
`

// TestCheckTripwire_ARefusalInADeclaredFunction_MustBeTheOneItDeclares: part
// (b) asks whether the refusal a row declares in a function is the one its
// literal builds, not whether the function is declared at all. A policy code
// no refusal there carries fails even in a declared function, and so does a
// code that is not constant beside a refusal that is no JSON-RPC one; the code
// and the variable code a declared JSON-RPC refusal carries pass. A 429 or 503
// written with http.Error or WriteHeader is held to a gate refusal of that
// status the same way; another status, or one that is not constant, is not
// read.
func TestCheckTripwire_ARefusalInADeclaredFunction_MustBeTheOneItDeclares(t *testing.T) {
	rpc := func(at, via string, channel tenancy.Channel, code int) tenancy.Refusal {
		r := tenancy.Refusal{Channel: channel, Code: code, At: site(at, tenancy.Refuse)}
		if via != "" {
			r.Via = site(via, tenancy.Refuse)
		}
		return r
	}
	d := row("ROW-001", site("overLimit", tenancy.Enforce))
	d.Refusals = []tenancy.Refusal{
		rpc("busy", "", tenancy.RPC, -32000),
		rpc("ErrTooMany", "wire", tenancy.RPC, -32000),
		rpc("callCode", "", tenancy.ToolError, 0),
		rpc("overLimit", "", tenancy.RPC, -32000),
		{Channel: tenancy.Gate, Status: 503, At: site("draining", tenancy.Refuse)},
		{Channel: tenancy.Gate, Status: 429, At: site("throttled", tenancy.Refuse)},
	}
	report := fixture{
		files: map[string]string{"site/site.go": siteHeader, "site/rpc.go": rpcSource, "site/writers.go": writerSource},
		rows:  []tenancy.Decision{d},
	}.run(t)
	assertFindings(t, report, "G10",
		siteDir+":callCode: "+literalMessage,
		siteDir+":globalCode: "+literalMessage,
		siteDir+":overLimit: "+literalMessage,
		siteDir+":tooMany: writes a 429 response that no refusal a row declares there carries",
		siteDir+":throttled: writes a 503 response that no refusal a row declares there carries",
	)
}

// nameSource declares package-level names, some reading as a limit.
const nameSource = siteHeader + `
const (
	maxWidgets     = 5
	widgetTTL      = 6
	exemptedWindow = 7
	declaredLimit  = 8
	plainName      = 9
	_              = 10
)

var httpReadTimeout = 11

func Enforce() {}
`

// TestCheckTripwire_LimitNamedNames: a name with a lexicon word that no row
// declares and no exemption answers fails, in a package a row's value or
// enforcing site is in; a declared or exempted one, and one with no lexicon
// word, passes.
func TestCheckTripwire_LimitNamedNames(t *testing.T) {
	report := fixture{
		files:  map[string]string{"site/site.go": nameSource},
		rows:   []tenancy.Decision{row("ROW-001", site("Enforce", tenancy.Enforce), site("declaredLimit", tenancy.Refuse))},
		exempt: map[string]exemption{siteDir + ":exemptedWindow": {categoryTransport, "a fixture"}},
	}.run(t)
	message := "is named like a limit, and no row declares it and no exemption answers it"
	assertFindings(t, report, "G10",
		siteDir+":httpReadTimeout: "+message, siteDir+":maxWidgets: "+message, siteDir+":widgetTTL: "+message)
	if len(report.Excused) != 1 || report.Excused[0].Part != partNames {
		t.Fatalf("excused = %+v, want the one name exemption", report.Excused)
	}
}

// TestCheckTripwire_OnlyThePackagesTheRegisterNames_AreRead: a package holding
// only a request bound's sites, or only refusal sites, is not read by part
// (c), and a package the register names that the program does not hold is
// passed over.
func TestCheckTripwire_OnlyThePackagesTheRegisterNames_AreRead(t *testing.T) {
	bound := row("RQB-001", site("Enforce", tenancy.Enforce))
	bound.Disposition = tenancy.RequestBound
	report := fixture{
		files: map[string]string{"site/site.go": nameSource},
		rows: []tenancy.Decision{
			bound,
			row("ROW-001", site("declaredLimit", tenancy.Refuse), tenancy.Site{Pkg: "internal/nowhere", Name: "X", Role: tenancy.Enforce}),
		},
	}.run(t)
	assertFindings(t, report, "G10")
}

// TestNamedPackages_AreDerivedFromTheRegisterOnce: each package once, sorted,
// from the value and enforcing sites of rows that are not request bounds.
func TestNamedPackages_AreDerivedFromTheRegisterOnce(t *testing.T) {
	g := &gate{reg: register{decisions: []tenancy.Decision{
		row("ROW-001", tenancy.Site{Pkg: "b", Role: tenancy.Alias}, tenancy.Site{Pkg: "a", Role: tenancy.Pin}, tenancy.Site{Pkg: "b", Role: tenancy.Arg}),
		row("ROW-002", tenancy.Site{Pkg: "c", Role: tenancy.Refuse}, tenancy.Site{Pkg: "d", Role: tenancy.Enforce}),
	}}}
	if got := g.namedPackages(); !slices.Equal(got, []string{"a", "b", "d"}) {
		t.Fatalf("namedPackages = %v", got)
	}
}

// TestCamelWords_SplitsAnIdentifierIntoItsWords: at a lower-to-upper step, at
// the end of an acronym a lower-case run follows, and at a digit-to-upper
// step.
func TestCamelWords_SplitsAnIdentifierIntoItsWords(t *testing.T) {
	for name, want := range map[string][]string{
		"baseHTTPMaxHeaderBytes":    {"base", "HTTP", "Max", "Header", "Bytes"},
		"maxListenStreamsPerServer": {"max", "Listen", "Streams", "Per", "Server"},
		"staticListTTLMs":           {"static", "List", "TTL", "Ms"},
		"base64Size":                {"base64", "Size"},
		"ID":                        {"ID"},
		"x":                         {"x"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := camelWords(name); !slices.Equal(got, want) {
				t.Fatalf("camelWords(%q) = %q, want %q", name, got, want)
			}
		})
	}
}
