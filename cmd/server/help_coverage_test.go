package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// undocumentedFlags are the flags the curated help deliberately leaves out,
// each with the reason it is left out.
//
// A declaration here is a claim that an operator reading -h is not worse off
// for its absence, so each has to be a flag that is either an alias of
// something documented or one nobody types.
var undocumentedFlags = map[string]string{
	"h": "the short spelling of -help, which prints this help; documenting the alias inside its own output tells a reader nothing they are not already looking at",
}

// TestPrintHelp_DocumentsEveryFlag holds the curated help to the flags the
// binary actually registers.
//
// The help is hand-written, and the tests beside it check individual entries
// that were once wrong. Nothing checked the set, so a flag added without a help
// entry was discoverable only through the flag package's own output, which is
// the output -h exists to replace: an operator who does not already know the
// flag's name cannot find it. Four settings were added in exactly that state
// before this test existed.
//
// The names come from the source rather than from flag.CommandLine, because
// they are registered in main() and a test cannot call that. Reading the calls
// is what the audit commands in cmd/ do for the same reason.
func TestPrintHelp_DocumentsEveryFlag(t *testing.T) {
	stdout := captureStdout(t)
	printHelp()
	help := stdout()

	var missing []string
	for _, name := range registeredFlagNames(t) {
		if _, declared := undocumentedFlags[name]; declared {
			continue
		}
		// The help writes a flag as "-name", with its type or its value after
		// it, so the name followed by a space or a newline is the entry.
		if !strings.Contains(help, "-"+name+" ") && !strings.Contains(help, "-"+name+"\n") {
			missing = append(missing, name)
		}
	}

	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("-%s is registered but the curated help never names it; add an entry or declare it in undocumentedFlags", name)
	}
}

// TestUndocumentedFlags_StillExist keeps the declaration table honest in the
// other direction: an entry for a flag that has been removed or renamed
// excuses nothing and would quietly widen the check the day a new flag took
// that name.
func TestUndocumentedFlags_StillExist(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range registeredFlagNames(t) {
		registered[name] = true
	}

	for name, reason := range undocumentedFlags {
		if !registered[name] {
			t.Errorf("undocumentedFlags declares -%s (%s), which the binary no longer registers", name, reason)
		}
	}
}

// registeredFlagNames reads every flag name main.go registers, from the second
// argument of each flag.XxxVar call and of flag.Var.
func registeredFlagNames(t *testing.T) []string {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing main.go: %v", err)
	}

	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok || pkg.Name != "flag" {
			return true
		}
		if sel.Sel.Name != "Var" && !strings.HasSuffix(sel.Sel.Name, "Var") {
			return true
		}
		lit, ok := call.Args[1].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		name, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			t.Errorf("a flag name in main.go is not a plain string literal: %s", lit.Value)
			return true
		}
		names = append(names, name)
		return true
	})

	if len(names) == 0 {
		t.Fatal("no flag registrations found in main.go; the reader is looking at the wrong shape")
	}
	sort.Strings(names)
	return names
}
