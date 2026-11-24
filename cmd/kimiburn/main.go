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
	"time"

	"github.com/acevedomiguel/kimiburn/internal/cache"
	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
	"github.com/acevedomiguel/kimiburn/internal/domain"
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
	command         string
	configPath      string
	jsonOutput      bool
	refreshInterval time.Duration
	refreshSet      bool
	noCache         bool
	noColor         bool
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
	// this build", which must never be treated as success. Production leaves it
	// nil and sets validateFactory instead, so the validator can honour the
	// explicit --config path.
	validate func(context.Context, string) error
	// validateFactory builds the production validator for the parsed command
	// line. Tests inject validate directly and leave this nil.
	validateFactory func(configPath string) func(context.Context, string) error
	// keyring is the OS secret store, or nil to force the file fallback.
	keyring credentials.Keyring
	// interactive reports whether stdin is a terminal.
	interactive func(io.Reader) bool
	// readSecret reads a secret without echoing it.
	readSecret func(prompt string, stdin io.Reader, stdout io.Writer) (string, error)
	// newUsage builds the one-shot provider reader. Tests inject a fake.
	newUsage func(baseURL, token string) (domain.UsageSource, error)
	// stateStore overrides the cache for tests. nil selects the real file store,
	// or the disabled store when --no-cache is set.
	stateStore cache.StateStore
	// now supplies cached-age timestamps in tests.
	now func() time.Time
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
		if !opts.jsonOutput {
			fmt.Fprintln(stderr, "kimiburn: dashboard is not available in this build")
			return exitFailure
		}
	case "demo":
		fmt.Fprintln(stderr, "kimiburn: demo is not available in this build")
		return exitFailure
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}

	if opts.command == "" && opts.jsonOutput {
		return doJSON(paths, opts, stdout, stderr, d)
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
		// Production builds supply a factory rather than a frozen validator so
		// the bounded client honours the explicit --config path.
		if d.validate == nil && d.validateFactory != nil {
			d.validate = d.validateFactory(opts.configPath)
		}
		return doSetup(paths, opts, stdin, stdout, stderr, d)
	case "logout":
		return doLogout(paths, stdout, stderr, d)
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
	fs.BoolVar(&opts.jsonOutput, "json", false, "emit one schema-1 JSON snapshot")
	fs.DurationVar(&opts.refreshInterval, "refresh", config.MinRefreshInterval, "automatic refresh interval (0 disables; otherwise at least 30s)")
	fs.BoolVar(&opts.noCache, "no-cache", false, "disable cache reads and writes")
	fs.BoolVar(&opts.noColor, "no-color", false, "disable terminal color")
	if err := fs.Parse(args); err != nil {
		return options{}, &usageError{msg: err.Error()}
	}
	if *showVersion {
		return options{command: "version"}, nil
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "refresh" {
			opts.refreshSet = true
		}
	})

	rest := fs.Args()
	switch {
	case len(rest) == 0:
		opts.command = ""
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
	case rest[0] == "demo" && len(rest) == 1:
		opts.command = "demo"
	default:
		return options{}, &usageError{msg: fmt.Sprintf("unknown command %q", joinArgs(rest))}
	}
	if opts.jsonOutput && opts.command != "" {
		return options{}, &usageError{msg: "--json cannot be combined with a command"}
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
		"  kimiburn                  Open the live dashboard\n"+
		"  kimiburn demo             Open the synthetic dashboard\n"+
		"  kimiburn --json           Print one schema-1 JSON snapshot\n"+
		"  kimiburn config path      Print the configuration file path\n"+
		"  kimiburn config reset     Restore the sample configuration\n"+
		"  kimiburn setup            Store the Kimi Code token\n"+
		"  kimiburn logout           Remove locally stored credentials\n"+
		"  kimiburn [--version] [--help]\n"+
		"\nFlags:\n"+
		"  --config <path>           Use an explicit configuration file\n"+
		"  --json                    Emit one JSON snapshot and exit\n"+
		"  --refresh <duration>      Refresh interval: 0 or at least 30s\n"+
		"  --no-cache                Disable cache reads and writes\n"+
		"  --no-color                Disable terminal color\n"+
		"  --version                 Print version and exit\n"+
		"  --help, -h                Print this help and exit\n"+
		"\nExit codes:\n"+
		"  0 success   1 runtime or API failure   2 usage error   3 missing credentials\n")
}
