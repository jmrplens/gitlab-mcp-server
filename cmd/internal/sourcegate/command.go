package sourcegate

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// Exit ends a gate's process with the code its run decided. It is [os.Exit]
// behind a seam, so a gate's test can run the gate's main and read that code
// as a value rather than as the end of the test binary; a test that replaces
// it puts it back in its cleanup.
var Exit = os.Exit

// Main is a gate's main function: it runs the gate's command line on the
// process's arguments, standard output and standard error, in that order, and
// ends the process through [Exit] with the code the command line decided.
func Main(runMain func(args []string, stdout, stderr io.Writer) int) {
	Exit(runMain(os.Args[1:], os.Stdout, os.Stderr))
}

// Run parses a gate's command line with the three flags every gate reads and
// hands what it read to start, whose answer is the exit code. When parsing
// ends the run, a request for help or arguments it cannot parse, start is not
// called and the code is the one [Command.Parse] decided.
func Run(name string, usage Usage, args []string, stderr io.Writer, start func(cmd *Command) int) int {
	cmd := NewCommand(name, usage, stderr)
	if code, goOn := cmd.Parse(args); !goOn {
		return code
	}
	return start(cmd)
}

// Usage is the help text of the three flags every gate reads, in the gate's
// own words, since what -check refuses and what -v adds are the gate's
// question rather than the scaffolding's.
type Usage struct {
	Dir     string
	Check   string
	Verbose string
}

// Command is one gate's command line: a flag set named after the gate that
// writes its complaints and its help to the gate's standard error, with -dir,
// -check and -v defined on it.
//
// The fields hold what [Command.Parse] read. A gate with a flag of its own
// defines it on Flags before parsing, and reads the arguments left after the
// flags from Flags as well.
type Command struct {
	Flags   *flag.FlagSet
	Dir     string
	Check   bool
	Verbose bool
}

// NewCommand returns the command line of the gate called name.
//
// The set is made with [flag.ContinueOnError] rather than the process's
// [flag.CommandLine], so a test can run a gate's whole command line any number
// of times and read the exit code it decided, rather than the end of the test
// binary.
func NewCommand(name string, usage Usage, stderr io.Writer) *Command {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	cmd := &Command{Flags: flags}
	flags.StringVar(&cmd.Dir, "dir", ".", usage.Dir)
	flags.BoolVar(&cmd.Check, "check", false, usage.Check)
	flags.BoolVar(&cmd.Verbose, "v", false, usage.Verbose)
	return cmd
}

// Parse parses a command line and reports whether the run goes on. When it
// does not, the code is what the process exits with: 0 for a request for help,
// which the flag package has already answered on standard error, and 2 for
// arguments it cannot parse, which it has already complained about there.
func (c *Command) Parse(args []string) (code int, goOn bool) {
	if err := c.Flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, false
		}
		return 2, false
	}
	return 0, true
}

// Patterns are the arguments left after the flags, or the gate's own defaults
// when there are none, so a run with no arguments audits what the gate is for.
func (c *Command) Patterns(defaults []string) []string {
	if c.Flags.NArg() == 0 {
		return defaults
	}
	return c.Flags.Args()
}

// Report is what a gate's audit hands back: a report that writes itself, with
// what its declarations excused when the run is verbose, and says whether the
// run found nothing to answer for.
type Report interface {
	// Write renders the report to out, with what the declarations excused
	// when verbose.
	Write(out io.Writer, verbose bool)
	// OK reports whether the run found nothing to answer for.
	OK() bool
}

// Finish ends a gate's run with what its audit returned, and returns the
// process exit code.
//
// Exit 1 covers two different things on purpose, told apart by which stream
// spoke: a run that could not be made at all says so on standard error, under
// the gate's name, and writes no report, and a run that found something says
// it on standard output and fails only under -check. A report is worth having
// without a gate, and a gate is worth having without ceremony.
func Finish(name string, report Report, err error, check, verbose bool, stdout, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", name, err)
		return 1
	}
	report.Write(stdout, verbose)
	if check && !report.OK() {
		return 1
	}
	return 0
}
