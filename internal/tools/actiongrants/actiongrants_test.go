package actiongrants

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestTable_IsTheSameTableOnEveryCall verifies every reader shares the one
// generated table rather than a copy, which is what lets each catalog action
// point into it for free.
func TestTable_IsTheSameTableOnEveryCall(t *testing.T) {
	first, second := Table(), Table()
	if first != second {
		t.Fatal("Table() answered two different tables")
	}
	if Table().Version == "" {
		t.Error("the generated table names no GitLab version; regenerate it with make gen-action-grants")
	}
}

// TestRequirement_AnswersByCanonicalID verifies a row is found by its action
// ID, as a pointer into the table, and that an ID with no row answers nil.
func TestRequirement_AnswersByCanonicalID(t *testing.T) {
	rows := Table().Actions
	if len(rows) == 0 {
		t.Fatal("the generated table holds no row; regenerate it with make gen-action-grants")
	}
	first := &rows[0]
	if got := Requirement(first.ID); got != first {
		t.Errorf("Requirement(%q) = %p, want the table's row %p", first.ID, got, first)
	}
	if got := Requirement("no.such_action"); got != nil {
		t.Errorf("Requirement(no.such_action) = %+v, want nil", got)
	}
}

// initSymbol matches, in `go tool nm -size` output, the init functions this
// package compiles to: the one the compiler writes for package-level values
// it could not lay out as data, and any declared init function.
var initSymbol = regexp.MustCompile(`^\s*[0-9a-f]+\s+(\d+)\s+T\s+\S*/internal/tools/actiongrants\.init(\.\d+)?$`)

// TestTable_CompilesToDataWithNoInitWork verifies the generated table is laid
// out by the compiler rather than built when the server starts: a value it
// cannot place statically gets code in the package's init function, and every
// start would pay for building a table of more than a thousand rows.
//
// The package is compiled twice and the init functions compared: once as it
// stands, and once with the generator's stand-in, an empty table, in
// table_gen.go's place. The compiler writes a one-instruction init for any
// package importing one with init work of its own, so the empty table is the
// baseline, and a generated table adding nothing to it is one the compiler
// laid out whole. Compiling the stand-in is also what holds it to compiling
// against the current finegrained types, which nothing else does: the
// generator only ever reads it through an overlay.
func TestTable_CompilesToDataWithNoInitWork(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	stub, err := filepath.Abs(filepath.Join(dir, "..", "..", "..", "cmd", "gen_action_grants", "table_stub.go.txt"))
	if err != nil {
		t.Fatalf("stand-in path: %v", err)
	}
	overlay := filepath.Join(t.TempDir(), "overlay.json")
	body, err := json.Marshal(map[string]map[string]string{"Replace": {filepath.Join(dir, "table_gen.go"): stub}})
	if err != nil {
		t.Fatalf("encode overlay: %v", err)
	}
	if err = os.WriteFile(overlay, body, 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}

	generated := initBytes(t, "go", "list", "-export", "-f", "{{.Export}}", ".")
	empty := initBytes(t, "go", "list", "-overlay="+overlay, "-export", "-f", "{{.Export}}", ".")
	if generated > empty {
		t.Errorf("the generated table compiles to %d bytes of init code against the empty table's %d: a literal in table_gen.go is no longer one the compiler lays out as data", generated, empty)
	}
}

// initBytes compiles the package with the go command given, through
// `go list -export`, and returns the size of its init functions.
func initBytes(t *testing.T, command ...string) int {
	t.Helper()
	exported, err := exec.CommandContext(t.Context(), command[0], command[1:]...).Output() // #nosec G204 -- the go command, with the arguments this test writes
	if err != nil {
		t.Fatalf("%s: %v", strings.Join(command, " "), err)
	}
	object := strings.TrimSpace(string(exported))
	symbols, err := exec.CommandContext(t.Context(), "go", "tool", "nm", "-size", object).Output()
	if err != nil {
		t.Fatalf("go tool nm -size %s: %v", object, err)
	}
	if !strings.Contains(string(symbols), "actiongrants.table") {
		t.Fatalf("the symbols of %s name no table, so they are not this package's", object)
	}
	total := 0
	for line := range strings.SplitSeq(string(symbols), "\n") {
		if match := initSymbol.FindStringSubmatch(line); match != nil {
			size, _ := strconv.Atoi(match[1])
			total += size
		}
	}
	return total
}
