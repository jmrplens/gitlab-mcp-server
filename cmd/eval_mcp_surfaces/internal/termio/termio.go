package termio

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Output mirrors command progress to an optional log file and stdout.
type Output struct {
	file io.Writer
	echo bool
}

// NewOutput creates an output sink for tests or custom terminal routing.
func NewOutput(file io.Writer, echo bool) Output {
	return Output{file: file, echo: echo}
}

// Write writes p to the configured file sink and optionally to stdout.
func (out Output) Write(p []byte) (int, error) {
	if out.file != nil {
		if _, err := out.file.Write(p); err != nil {
			return 0, err
		}
	}
	if out.echo {
		n, err := os.Stdout.Write(p)
		if err != nil {
			return n, err
		}
	}
	return len(p), nil
}

var commandOutput Output

// SetOutputForTest replaces the package output sink and returns a restore function.
func SetOutputForTest(out Output) func() {
	previous := commandOutput
	commandOutput = out
	return func() { commandOutput = previous }
}

// Printf writes formatted command output to the configured terminal sink.
func Printf(format string, args ...any) {
	if commandOutput.file != nil {
		if _, err := fmt.Fprintf(commandOutput.file, format, args...); err != nil {
			reportWriteError("Printf", err)
		}
	}
	if commandOutput.echo {
		fmt.Printf(format, args...)
	}
}

// Print writes command output to the configured terminal sink.
func Print(content string) {
	if commandOutput.file != nil {
		if _, err := fmt.Fprint(commandOutput.file, content); err != nil {
			reportWriteError("Print", err)
		}
	}
	if commandOutput.echo {
		fmt.Print(content)
	}
}

// LogPrintf writes formatted output only to the terminal log sink.
func LogPrintf(format string, args ...any) {
	if commandOutput.file != nil {
		if _, err := fmt.Fprintf(commandOutput.file, format, args...); err != nil {
			reportWriteError("LogPrintf", err)
		}
	}
}

func reportWriteError(function string, err error) {
	fmt.Fprintf(os.Stderr, "termio.%s: write terminal log: %v\n", function, err)
}

// Configure creates a terminal log at logPath and routes command output to it.
func Configure(logPath string, printOutput bool) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0o750); err != nil {
		return nil, fmt.Errorf("create terminal log directory: %w", err)
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- evaluator log path is an explicit CLI/default artifact path.
	if err != nil {
		return nil, fmt.Errorf("open terminal log: %w", err)
	}
	commandOutput = Output{file: file, echo: printOutput}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(commandOutput, nil)))
	LogPrintf("eval_mcp_surfaces terminal output\n")
	LogPrintf("terminal_log=%s\n", logPath)
	if printOutput {
		LogPrintf("print_output=true\n")
	}
	return func() error {
		slog.SetDefault(previousLogger)
		commandOutput = Output{}
		return file.Close()
	}, nil
}

// ShouldConfigure reports whether command output should be routed to a log file.
func ShouldConfigure(terminalLog string, printOutput, checkDocs bool, checkEfficiencyCount, compareTraceCount int) bool {
	if terminalLog != "" || printOutput {
		return true
	}
	return !checkDocs && checkEfficiencyCount == 0 && compareTraceCount == 0
}
