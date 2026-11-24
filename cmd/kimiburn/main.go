// Command kimiburn is a read-only terminal monitor for Kimi Code membership
// quotas. This slice provides the CLI skeleton: version and help only.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

// Exit codes documented in the CLI contract.
const (
	exitOK          = 0
	exitFailure     = 1
	exitUsage       = 2
	exitCredentials = 3
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options is the parsed command line.
type options struct {
	// command is one of "help", "version", or "" when no command was given.
	command string
}

// usageError marks failures caused by the command line rather than the
// environment, so they exit 2 with an explanatory line.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// run executes the CLI with explicit streams so tests never touch the real
// terminal, process exit status, or user state. The help and version paths
// return before any configuration, credential, or network access, and never
// read stdin.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	opts, err := parse(args)
	if err != nil {
		var ue *usageError
		if errors.As(err, &ue) {
			fmt.Fprintf(stderr, "kimiburn: %s\n\n", ue.msg)
			printUsage(stderr)
		}
		return exitUsage
	}

	switch opts.command {
	case "version":
		fmt.Fprintf(stdout, "kimiburn version %s\n", version)
		return exitOK
	case "help":
		printUsage(stdout)
		return exitOK
	}

	fmt.Fprint(stderr, "kimiburn: missing command\n\n")
	printUsage(stderr)
	return exitUsage
}

// parse turns argv into options, honoring help requests before flag errors.
func parse(args []string) (options, error) {
	for _, a := range args {
		if a == "-h" || a == "-help" || a == "--help" {
			return options{command: "help"}, nil
		}
	}

	fs := flag.NewFlagSet("kimiburn", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	showVersion := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return options{}, &usageError{msg: err.Error()}
	}
	if *showVersion {
		return options{command: "version"}, nil
	}
	if rest := fs.Args(); len(rest) > 0 {
		return options{}, &usageError{msg: fmt.Sprintf("unexpected argument %q", rest[0])}
	}
	return options{}, nil
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, "Usage:\n"+
		"  kimiburn [--version] [--help]\n"+
		"\nFlags:\n"+
		"  --version                 Print version and exit\n"+
		"  --help, -h                Print this help and exit\n"+
		"\nExit codes:\n"+
		"  0 success   1 runtime or API failure   2 usage error   3 missing credentials\n")
}
