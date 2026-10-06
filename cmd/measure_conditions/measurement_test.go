package main

import (
	"reflect"
	"testing"
)

// TestParseMeasurement_Outputs_ReadTheFigureAndTheConditions verifies the
// reading of a run that measured: gobco's figure, every condition it names
// in each of its three shapes with the file written with slashes and the
// code unquoted, the held ones marked from the lines the gate printed under
// its verdict, and everything else in the output passed over, a condition
// evaluated both ways, an indented line that is not a condition and a
// carriage return included.
func TestParseMeasurement_Outputs_ReadTheFigureAndTheConditions(t *testing.T) {
	t.Parallel()
	output := "go: downloading example.com/x v1.0.0\n" +
		"ok  \texample.com/m/cmd/server\t1.2s\r\n" +
		"\n" +
		"Condition coverage: 2236/2240\r\n" +
		`main.go:91:5: condition "x != \"\"" was 3 times true but never false` + "\n" +
		`cmd\server\conn_windows.go:12:9: condition "errors.Is(err, errRefused)" was once false but never true` + "\r\n" +
		`probe.go:7:2: condition "a | b == c" was never evaluated` + "\n" +
		`main.go:95:5: condition "y" was once true and once false` + "\n" +
		"gobco: GOBCO_GATE=all: 2 condition(s) in the files were not evaluated both ways:\n" +
		`  main.go:91:5: condition "x != \"\"" was 3 times true but never false` + "\n" +
		`  probe.go:7:2: condition "a | b == c" was never evaluated` + "\r\n" +
		"  --- FAIL: an indented line that is not a condition\n"
	got, err := parseMeasurement(output, statusGateHeld)
	if err != nil {
		t.Fatalf("parseMeasurement() error = %v", err)
	}
	want := measurement{
		covered: "2236",
		total:   "2240",
		held:    2,
		conditions: []condition{
			{
				text: `main.go:91:5: condition "x != \"\"" was 3 times true but never false`,
				file: "main.go", line: "91", code: `x != ""`, says: "3 times true but never false", held: true,
			},
			{
				text: `cmd\server\conn_windows.go:12:9: condition "errors.Is(err, errRefused)" was once false but never true`,
				file: "cmd/server/conn_windows.go", line: "12", code: "errors.Is(err, errRefused)", says: "once false but never true",
			},
			{
				text: `probe.go:7:2: condition "a | b == c" was never evaluated`,
				file: "probe.go", line: "7", code: "a | b == c", says: "never evaluated", held: true,
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseMeasurement() =\n%+v\nwant\n%+v", got, want)
	}
}

// TestParseMeasurement_StatusAndOutput_MustAgree verifies that a measurement
// is refused when its output is not what the script writes for its status:
// no figure, a figure of zero (which the script refuses), a held line
// gobco's report does not name, a gate status with nothing held, and a
// passing status with something held. The last figure printed is the one
// read, and a passing run that holds nothing is accepted.
func TestParseMeasurement_StatusAndOutput_MustAgree(t *testing.T) {
	t.Parallel()
	const never = `main.go:1:2: condition "x" was never evaluated`
	cases := []struct {
		name    string
		output  string
		status  int
		wantErr string
	}{
		{
			name: "no figure", output: never + "\n", status: statusMeasured,
			wantErr: "the script's status says it measured, and its output carries no Condition coverage figure",
		},
		{
			name: "a figure of zero", output: "Condition coverage: 0/0\n", status: statusMeasured,
			wantErr: "the script's status says it measured, and its output carries no Condition coverage figure",
		},
		{
			name: "a held line gobco did not name", output: "Condition coverage: 1/2\n  " + never + "\n", status: statusGateHeld,
			wantErr: "the script held 1 condition(s) and gobco's report names 0 of them",
		},
		{
			name: "a gate status with nothing held", output: "Condition coverage: 1/2\n" + never + "\n", status: statusGateHeld,
			wantErr: "the script exited 3 and its gate held 0 condition(s)",
		},
		{
			name: "a passing status with something held", output: "Condition coverage: 1/2\n" + never + "\n  " + never + "\n", status: statusMeasured,
			wantErr: "the script exited 0 and its gate held 1 condition(s)",
		},
		{name: "a passing run, the last figure read", output: "Condition coverage: 1/9\nCondition coverage: 2/2\n", status: statusMeasured},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseMeasurement(tc.output, tc.status)
			if tc.wantErr == "" {
				if err != nil || got.covered != "2" || got.total != "2" || got.held != 0 {
					t.Errorf("parseMeasurement() = %+v, %v; want 2 of 2 and nothing held", got, err)
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("parseMeasurement() error = %v, want %q", err, tc.wantErr)
			}
			if !reflect.DeepEqual(got, measurement{}) {
				t.Errorf("parseMeasurement() = %+v alongside its error, want the zero measurement", got)
			}
		})
	}
}

// TestUnquoteCode_Quotings_AreUndoneWhereGoReadsThem verifies that the code
// gobco quoted is shown as written, on one line, and a quoting Go cannot read
// is shown as printed rather than dropped. A condition written across lines
// is quoted with its line breaks and their indentation, and each break comes
// back as one space with the spaces and tabs around it, a carriage return
// alone or before a line feed included. A tab or two spaces inside the code,
// and a tab the source wrote as an escape, are kept as they are.
func TestUnquoteCode_Quotings_AreUndoneWhereGoReadsThem(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`"a == \"b\""`:                        `a == "b"`,
		`"\q"`:                                `"\q"`,
		`"strings.Contains(s,\n\t\t\"x\t\")"`: "strings.Contains(s, \"x\t\")",
		`"a &&  \r\n\tb ||  c"`:               `a && b ||  c`,
		`"a ||\rb"`:                           `a || b`,
		`"s == \"\\t\""`:                      `s == "\t"`,
	}
	for quoted, want := range cases {
		t.Run(quoted, func(t *testing.T) {
			t.Parallel()
			if got := unquoteCode(quoted); got != want {
				t.Errorf("unquoteCode(%q) = %q, want %q", quoted, got, want)
			}
		})
	}
}
