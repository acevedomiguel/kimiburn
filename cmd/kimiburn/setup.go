package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
)

// isTerminal reports whether r is an interactive terminal.
func isTerminal(r io.Reader) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(file.Fd())
}

// readSecretNoEcho prompts on stdout and reads a line without echoing it. Only
// an interactive terminal reaches the no-echo path.
func readSecretNoEcho(prompt string, stdin io.Reader, stdout io.Writer) (string, error) {
	file, ok := stdin.(*os.File)
	if !ok {
		return "", fmt.Errorf("credentials: stdin is not a terminal")
	}
	fmt.Fprint(stdout, prompt)
	raw, err := term.ReadPassword(file.Fd())
	fmt.Fprintln(stdout)
	if err != nil {
		return "", fmt.Errorf("credentials: read token: %w", err)
	}
	return strings.TrimSpace(string(raw)), nil
}

// setupWelcome states Kimiburn's boundaries before the token is requested.
func setupWelcome() string {
	return "Kimiburn is an independent, read-only terminal monitor for Kimi Code\n" +
		"membership quotas. It is not affiliated with, and is not an official client of,\n" +
		"Kimi. Kimiburn never sends inference requests and never changes your quota.\n" +
		"It stores exactly one token, used only for the Kimi Code usage endpoint.\n\n"
}

// doSetup stores the Kimi Code token after validating it with one live read.
// The token is never echoed and never printed.
func doSetup(paths config.Paths, opts options, stdin io.Reader, stdout, stderr io.Writer, d deps) int {
	fmt.Fprint(stdout, setupWelcome())
	store, err := storeFor(paths, opts, d)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: setup: %s\n", earlyRedact(err.Error()))
		return exitFailure
	}

	// An environment token always wins and is never persisted.
	if envToken := store.EnvironmentToken(); envToken != "" {
		fmt.Fprintln(stderr, "kimiburn: KIMIBURN_TOKEN is set; nothing to set up.")
		fmt.Fprintln(stderr, "The environment token wins and is never written to disk.")
		fmt.Fprintln(stderr, "Unset it to store a token locally, or keep it to use it for this shell.")
		return exitOK
	}

	token, code := readSetupToken(stdin, stdout, stderr, d)
	if code != exitOK {
		return code
	}
	if token == "" {
		fmt.Fprintln(stderr, "kimiburn: no Kimi Code token was provided; nothing was saved.")
		fmt.Fprintln(stderr, "Run `kimiburn setup` again, or export KIMIBURN_TOKEN.")
		return exitCredentials
	}

	// Redact the entered token, plus any environment token, from everything
	// this command can print.
	redactor := credentials.NewRedactor(store.EnvironmentToken(), token)

	if d.validate == nil {
		fmt.Fprintln(stderr, "kimiburn: token validation is unavailable in this build; nothing was saved.")
		return exitFailure
	}
	ctx, cancel := context.WithTimeout(context.Background(), setupValidationTimeout)
	defer cancel()
	if err := d.validate(ctx, token); err != nil {
		fmt.Fprintf(stderr, "kimiburn: token validation failed: %s\n", redactor.Redact(err.Error()))
		fmt.Fprintln(stderr, "Nothing was saved. Run `kimiburn setup` again when the token works.")
		return exitFailure
	}

	if err := store.Save(ctx, token); err != nil {
		fmt.Fprintf(stderr, "kimiburn: could not store the token: %s\n", redactor.Redact(err.Error()))
		return exitFailure
	}

	cfgPath := paths.ConfigFile
	if opts.configPath != "" {
		cfgPath = opts.configPath
	}
	wroteConfig, err := writeSampleIfAbsent(cfgPath)
	if err != nil {
		fmt.Fprintf(stderr, "kimiburn: token saved, but the configuration file could not be written: %s\n", redactor.Redact(err.Error()))
		return exitFailure
	}
	if wroteConfig {
		fmt.Fprintf(stdout, "\nToken saved. Configuration written to %s\n", cfgPath)
	} else {
		// A hand-edited configuration encodes the user's own backend, theme, and
		// interval choices, so setup must never overwrite it.
		fmt.Fprintf(stdout, "\nToken saved. Existing configuration kept at %s\n", cfgPath)
	}
	fmt.Fprintln(stdout, "Run `kimiburn` to open the dashboard.")
	return exitOK
}

// readSetupToken reads the token from a terminal without echo, or as exactly
// one line when setup is non-interactive.
func readSetupToken(stdin io.Reader, stdout, stderr io.Writer, d deps) (string, int) {
	if d.interactive(stdin) {
		token, err := d.readSecret("Kimi Code token: ", stdin, stdout)
		if err != nil {
			fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
			return "", exitFailure
		}
		return strings.TrimSpace(token), exitOK
	}

	if _, err := fmt.Fprint(stdout, "Kimi Code token: "); err != nil {
		return "", exitFailure
	}
	scanner := bufio.NewScanner(stdin)
	token := ""
	if scanner.Scan() {
		token = strings.TrimSpace(scanner.Text())
	}
	fmt.Fprintln(stdout)
	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(stderr, "kimiburn: %s\n", earlyRedact(err.Error()))
		return "", exitFailure
	}
	return token, exitOK
}

// setupValidationTimeout bounds setup's single validation read.
const setupValidationTimeout = 20 * time.Second
