// Command kimiburn is a read-only terminal monitor for Kimi Code membership
// quotas. This slice provides the CLI skeleton, strict configuration and
// platform paths, and one-token credential setup/logout.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
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

// run executes the CLI with explicit streams and production dependencies.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runWithDeps(args, stdin, stdout, stderr, defaultDeps())
}

// options is the parsed command line. command is one of "help", "version",
// "config-path", "config-reset", "setup", "logout", or "" when none was given.
type options struct {
	command    string
	configPath string
}

// usageError marks failures caused by the command line rather than the
// environment, so they exit 2 with an explanatory line.
type usageError struct{ msg string }

func (e *usageError) Error() string { return e.msg }

// deps carries the injectable seams the CLI needs. Production builds get real
// implementations; tests inject fakes so no test touches the keyring, the
// network, or the real user configuration.
type deps struct {
	// validate checks a token with one live read. nil means "unavailable in
	// this build", which must never be treated as success.
	validate func(context.Context, string) error
	// keyring is the OS secret store, or nil to force the file fallback.
	keyring credentials.Keyring
	// interactive reports whether stdin is a terminal.
	interactive func(io.Reader) bool
	// readSecret reads a secret without echoing it.
	readSecret func(prompt string, stdin io.Reader, stdout io.Writer) (string, error)
}

// runWithDeps is the testable entry point.
func runWithDeps(args []string, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
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
	case "":
		fmt.Fprint(stderr, "kimiburn: missing command\n\n")
		printUsage(stderr)
		return exitUsage
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", credentials.Redact(err.Error()))
		return exitFailure
	}

	switch opts.command {
	case "config-path":
		path := paths.ConfigFile
		if opts.configPath != "" {
			path = opts.configPath
		}
		fmt.Fprintln(stdout, path)
		return exitOK
	case "config-reset":
		return doConfigReset(paths, opts, stdout, stderr)
	case "setup":
		return doSetup(paths, opts, stdin, stdout, stderr, d)
	case "logout":
		return doLogout(paths, opts, stdout, stderr, d)
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
	var opts options
	showVersion := fs.Bool("version", false, "print version and exit")
	fs.StringVar(&opts.configPath, "config", "", "use an explicit configuration file")
	if err := fs.Parse(args); err != nil {
		return options{}, &usageError{msg: err.Error()}
	}
	if *showVersion {
		return options{command: "version"}, nil
	}

	rest := fs.Args()
	switch {
	case len(rest) == 0:
		return options{}, nil
	case rest[0] == "config" && len(rest) == 1:
		return options{}, &usageError{msg: "usage: kimiburn config [path|reset]"}
	case rest[0] == "config" && len(rest) == 2 && rest[1] == "path":
		opts.command = "config-path"
	case rest[0] == "config" && len(rest) == 2 && rest[1] == "reset":
		opts.command = "config-reset"
	case rest[0] == "setup" && len(rest) == 1:
		opts.command = "setup"
	case rest[0] == "logout" && len(rest) == 1:
		opts.command = "logout"
	default:
		return options{}, &usageError{msg: fmt.Sprintf("unknown command %q", joinArgs(rest))}
	}
	return opts, nil
}

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += " "
		}
		out += a
	}
	return out
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, "Usage:\n"+
		"  kimiburn config path      Print the configuration file path\n"+
		"  kimiburn config reset     Restore the sample configuration\n"+
		"  kimiburn setup            Store the Kimi Code token\n"+
		"  kimiburn logout           Remove locally stored credentials\n"+
		"  kimiburn [--version] [--help]\n"+
		"\nFlags:\n"+
		"  --config <path>           Use an explicit configuration file\n"+
		"  --version                 Print version and exit\n"+
		"  --help, -h                Print this help and exit\n"+
		"\nExit codes:\n"+
		"  0 success   1 runtime or API failure   2 usage error   3 missing credentials\n")
}
