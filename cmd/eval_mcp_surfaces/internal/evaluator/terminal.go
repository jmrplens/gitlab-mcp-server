package evaluator

import (
	"github.com/jmrplens/gitlab-mcp-server/v2/cmd/eval_mcp_surfaces/internal/termio"
)

func terminalPrintf(format string, args ...any) {
	termio.Printf(format, args...)
}

func terminalPrint(content string) {
	termio.Print(content)
}

func terminalLogPrintf(format string, args ...any) {
	termio.LogPrintf(format, args...)
}

func configureTerminalOutput(opts options) (options, func() error, error) {
	if opts.TerminalLog == "" {
		opts.TerminalLog = defaultTerminalLogPath(opts.Output)
	}
	closeOutput, err := termio.Configure(opts.TerminalLog, opts.PrintOutput)
	if err != nil {
		return opts, nil, err
	}
	return opts, closeOutput, nil
}

func shouldConfigureTerminalOutput(opts options) bool {
	return termio.ShouldConfigure(opts.TerminalLog, opts.PrintOutput, opts.CheckDocs, len(opts.CheckEfficiency), len(opts.CompareTraces))
}

// stringList holds string list data for the evaluator package.
