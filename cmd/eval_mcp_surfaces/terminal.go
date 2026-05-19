package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

type terminalOutput struct {
	file io.Writer
	echo bool
}

func (out *terminalOutput) Write(p []byte) (int, error) {
	if out.file != nil {
		if _, err := out.file.Write(p); err != nil {
			return 0, err
		}
	}
	if out.echo {
		_, _ = os.Stdout.Write(p)
	}
	return len(p), nil
}

var commandOutput terminalOutput

func terminalPrintf(format string, args ...any) {
	if commandOutput.file != nil {
		_, _ = fmt.Fprintf(commandOutput.file, format, args...)
	}
	if commandOutput.echo {
		fmt.Printf(format, args...)
	}
}

func terminalPrint(content string) {
	if commandOutput.file != nil {
		_, _ = fmt.Fprint(commandOutput.file, content)
	}
	if commandOutput.echo {
		fmt.Print(content)
	}
}

func terminalLogPrintf(format string, args ...any) {
	if commandOutput.file != nil {
		_, _ = fmt.Fprintf(commandOutput.file, format, args...)
	}
}

func configureTerminalOutput(opts options) (options, func() error, error) {
	if opts.TerminalLog == "" {
		opts.TerminalLog = defaultTerminalLogPath(opts.Output)
	}
	if err := os.MkdirAll(filepath.Dir(opts.TerminalLog), 0o750); err != nil {
		return opts, nil, fmt.Errorf("create terminal log directory: %w", err)
	}
	file, err := os.OpenFile(opts.TerminalLog, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600) // #nosec G304 -- evaluator log path is an explicit CLI/default artifact path.
	if err != nil {
		return opts, nil, fmt.Errorf("open terminal log: %w", err)
	}
	commandOutput = terminalOutput{file: file, echo: opts.PrintOutput}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&commandOutput, nil)))
	terminalLogPrintf("eval_mcp_surfaces terminal output\n")
	terminalLogPrintf("terminal_log=%s\n", opts.TerminalLog)
	if opts.PrintOutput {
		terminalLogPrintf("print_output=true\n")
	}
	return opts, func() error {
		slog.SetDefault(previousLogger)
		commandOutput = terminalOutput{}
		return file.Close()
	}, nil
}

func shouldConfigureTerminalOutput(opts options) bool {
	if opts.TerminalLog != "" || opts.PrintOutput {
		return true
	}
	return !opts.CheckDocs && len(opts.CheckEfficiency) == 0 && len(opts.CompareTraces) == 0
}

// stringList holds string list data for the main package.
