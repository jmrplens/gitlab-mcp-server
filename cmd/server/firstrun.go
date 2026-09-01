package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// isInteractiveTerminal reports whether stdin is a terminal rather than a pipe.
//
// It is the test for "a person started this", and it is the same test an MCP
// client fails by construction: a client connects pipes, so this is false for
// every real session. Cygwin is checked as well as the character device,
// because mintty on Windows is not a character device and a double-click there
// is exactly the case this exists for.
func isInteractiveTerminal() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd())
}

// firstRunGuidance explains what this program is to somebody who has started it
// by hand, and waits before returning.
//
// The wait is the point. On Windows a double-clicked console program opens a
// window that closes the instant it exits, so a message printed and returned
// from is a message nobody reads. This blocks on a line from stdin, which is
// safe because it only runs when stdin is a terminal.
//
// It writes to stderr rather than stdout, and that is not cosmetic. On the
// stdio transport stdout carries JSON-RPC and nothing else, and a single stray
// line ends the session. Writing the one message that exists to be read by a
// human onto the one stream reserved for a machine would be a defect waiting
// for the guard above it to be widened by accident. A console window shows
// stderr just as readily.
//
// It replaced an interactive setup wizard that offered a web UI, a TUI and a
// prompt flow, and wrote a configuration file. That was twelve thousand lines
// and forty-five packages to configure two environment variables that almost
// nobody sets here anyway: MCP configuration lives in the client's own JSON,
// which is where the documentation puts it and where a wizard writing a dotfile
// on this machine could not help.
func firstRunGuidance(out io.Writer, in io.Reader, serverVersion string) {
	fmt.Fprintf(out, `gitlab-mcp-server %s

This is a Model Context Protocol server. It is not meant to be run directly:
an MCP client such as Claude Desktop, Claude Code, VS Code or Cursor starts it
and speaks to it over this program's standard input and output.

Nothing is configured yet. The server needs two values:

  GITLAB_URL     your GitLab instance, for example https://gitlab.com
  GITLAB_TOKEN   a personal access token, created under
                 User settings > Access tokens on that instance

Set them in your MCP client's configuration rather than in this terminal. The
per-client JSON, and the OAuth alternative if you would rather not paste a
token at all, are here:

  %s/getting-started/
  %s/configuration/

For every flag and environment variable this build accepts:

  %s --help

Press Enter to close.
`, serverVersion, projectWebsite, projectWebsite, executableName())

	// Errors are ignored: the only reader is a person, the only outcome is
	// that the window closes sooner, and there is nothing left to report to.
	_, _ = bufio.NewReader(in).ReadString('\n')
}

// executableName is how to spell this program on the reader's command line.
//
// Taken from the running binary rather than hardcoded, so the line can be
// copied as it appears: someone who renamed the download, or who reached this
// through a launcher, is told the name that will actually work.
func executableName() string {
	path, err := os.Executable()
	if err != nil || path == "" {
		return "gitlab-mcp-server"
	}
	return path
}
