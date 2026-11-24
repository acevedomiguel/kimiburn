package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acevedomiguel/kimiburn/internal/config"
	"github.com/acevedomiguel/kimiburn/internal/credentials"
)

// cliKeyring is an in-memory keyring so CLI tests never touch the real one.
type cliKeyring struct {
	values map[string]string
	fail   bool
}

func newCLIKeyring() *cliKeyring { return &cliKeyring{values: map[string]string{}} }

func (k *cliKeyring) Get(service, user string) (string, error) {
	if k.fail {
		return "", credentials.ErrKeyringUnavailable
	}
	v, ok := k.values[service+"/"+user]
	if !ok {
		return "", credentials.ErrKeyringUnavailable
	}
	return v, nil
}

func (k *cliKeyring) Set(service, user, secret string) error {
	if k.fail {
		return credentials.ErrKeyringUnavailable
	}
	k.values[service+"/"+user] = secret
	return nil
}

func (k *cliKeyring) Delete(service, user string) error {
	if k.fail {
		return credentials.ErrKeyringUnavailable
	}
	delete(k.values, service+"/"+user)
	return nil
}

// setupEnv isolates config/cache paths and the credential environment and
// returns the resolved config dir. It never reads real user state.
func setupEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, "config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", configDir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv(credentials.EnvToken, "")
	t.Setenv("KIMIBURN_KEYRING", "")
	return filepath.Join(configDir, "kimiburn")
}

// testDeps builds injectable CLI dependencies for setup/logout tests.
func testDeps(validate func(context.Context, string) error, kr credentials.Keyring) deps {
	return deps{
		validate:    validate,
		keyring:     kr,
		interactive: func(io.Reader) bool { return false },
		readSecret:  readSecretNoEcho,
	}
}

func runWith(args []string, stdin io.Reader, d deps) (int, string, string) {
	var out, errOut strings.Builder
	code := runWithDeps(args, stdin, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

const cliToken = "km_cli_synthetic_abcdef"

func TestSetupNonInteractiveStoresValidatedToken(t *testing.T) {
	configDir := setupEnv(t)
	kr := newCLIKeyring()
	validated := ""
	d := testDeps(func(_ context.Context, token string) error {
		validated = token
		return nil
	}, kr)

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if validated != cliToken {
		t.Errorf("validated = %q, want the piped token", validated)
	}
	if kr.values["kimiburn/token"] != cliToken {
		t.Errorf("keyring = %v, want the stored token", kr.values)
	}
	if !strings.Contains(stdout, "Kimi Code token") {
		t.Errorf("stdout should prompt for the token: %q", stdout)
	}
	if strings.Contains(stdout+stderr, cliToken) {
		t.Errorf("the token must never be echoed: stdout=%q stderr=%q", stdout, stderr)
	}
	cfgPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("successful setup should write the sample config: %v", err)
	}
	if !strings.Contains(stdout, cfgPath) {
		t.Errorf("stdout should name the config path: %q", stdout)
	}
}

func TestSetupValidatesBeforeStorageAndRedactsFailure(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	d := testDeps(func(_ context.Context, token string) error {
		return errors.New("server rejected " + token)
	}, kr)

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if len(kr.values) != 0 {
		t.Errorf("a failed validation must store nothing: %v", kr.values)
	}
	if strings.Contains(stdout+stderr, cliToken) {
		t.Errorf("the token leaked into output: stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "[redacted]") {
		t.Errorf("stderr should carry the redaction marker: %q", stderr)
	}
}

func TestSetupExplainsEnvironmentTokenWithoutPersisting(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	t.Setenv(credentials.EnvToken, "env-token")
	d := testDeps(func(context.Context, string) error {
		t.Error("an environment token must not be validated or stored")
		return nil
	}, kr)

	code, stdout, stderr := runWith([]string{"setup"}, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if len(kr.values) != 0 {
		t.Errorf("the environment token must not be persisted: %v", kr.values)
	}
	combined := stdout + stderr
	if !strings.Contains(strings.ToLower(combined), "environment") {
		t.Errorf("setup should explain the environment token wins: %q", combined)
	}
	if strings.Contains(combined, "env-token") {
		t.Errorf("the environment token must never be printed: %q", combined)
	}
}

func TestSetupMissingTokenExitsThreeAndStoresNothing(t *testing.T) {
	configDir := setupEnv(t)
	kr := newCLIKeyring()
	d := testDeps(func(context.Context, string) error { return nil }, kr)

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(""), d)
	if code != exitCredentials {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitCredentials, stderr)
	}
	if len(kr.values) != 0 {
		t.Errorf("missing token must store nothing: %v", kr.values)
	}
	if _, err := os.Stat(filepath.Join(configDir, "token")); !os.IsNotExist(err) {
		t.Errorf("missing token must not create the fallback file: %v", err)
	}
	if strings.Contains(stdout, cliToken) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestSetupFallsBackToRestrictedFileWhenKeyringDisabled(t *testing.T) {
	configDir := setupEnv(t)
	t.Setenv("KIMIBURN_KEYRING", "false")
	d := testDeps(func(context.Context, string) error { return nil }, newCLIKeyring())

	code, _, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	path := filepath.Join(configDir, "token")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("fallback file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("token file mode = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(configDir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("config dir mode = %o, want 0700", perm)
	}
}

// A pre-existing group/world-readable fallback file must be refused before it
// is read, so a loose token never becomes usable.
func TestResolutionRefusesLooseTokenFileBeforeReading(t *testing.T) {
	configDir := setupEnv(t)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "token"), []byte("loose"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := testDeps(nil, nil)
	store, err := storeFor(config.Paths{ConfigDir: configDir}, options{}, d)
	if err != nil {
		t.Fatalf("storeFor: %v", err)
	}
	if _, err := store.Resolve(context.Background()); !errors.Is(err, credentials.ErrInsecurePermissions) {
		t.Fatalf("resolve err = %v, want ErrInsecurePermissions", err)
	}
}

func TestSetupInteractiveUsesNoEchoReader(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	secretRead := false
	d := testDeps(func(_ context.Context, token string) error {
		if token != cliToken {
			t.Errorf("validated = %q", token)
		}
		return nil
	}, kr)
	d.interactive = func(io.Reader) bool { return true }
	d.readSecret = func(prompt string, _ io.Reader, stdout io.Writer) (string, error) {
		secretRead = true
		if !strings.Contains(prompt, "Kimi Code token") {
			t.Errorf("prompt = %q", prompt)
		}
		io.WriteString(stdout, prompt)
		return cliToken, nil
	}

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(""), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if !secretRead {
		t.Error("interactive setup must read the token through the no-echo reader")
	}
	if strings.Contains(stdout, cliToken) {
		t.Errorf("interactive setup echoed the token: %q", stdout)
	}
}

func TestSetupReportsValidationUnavailableWithoutStoring(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	d := testDeps(nil, kr)

	code, _, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code == exitOK {
		t.Fatal("setup must not report success when validation is unavailable")
	}
	if len(kr.values) != 0 {
		t.Errorf("unvalidated token must not be stored: %v", kr.values)
	}
	if strings.Contains(stderr, cliToken) {
		t.Errorf("stderr leaked the token: %q", stderr)
	}
}

func TestLogoutRemovesOnlyKimiburnCredentials(t *testing.T) {
	configDir := setupEnv(t)
	kr := newCLIKeyring()
	if err := kr.Set("kimiburn", "token", "stored"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("other-app", "token", "keep"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(configDir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(filepath.Dir(configDir), "cache", "state.json")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	d := testDeps(func(context.Context, string) error { return nil }, kr)
	code, stdout, stderr := runWith([]string{"logout"}, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if _, err := kr.Get("kimiburn", "token"); err == nil {
		t.Error("logout must remove Kimiburn's keyring entry")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback token file: %v", err)
	}
	if v, err := kr.Get("other-app", "token"); err != nil || v != "keep" {
		t.Errorf("logout touched another app: %q err=%v", v, err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("logout must not delete the cache: %v", err)
	}
	combined := strings.ToLower(stdout + stderr)
	if !strings.Contains(combined, "remain") && !strings.Contains(combined, "valid") {
		t.Errorf("logout should explain the token stays valid at Kimi: %q", combined)
	}
	if strings.Contains(combined, "revoke") && !strings.Contains(combined, "not revoke") && !strings.Contains(combined, "does not revoke") {
		t.Errorf("logout must clarify it does not revoke remotely: %q", combined)
	}
}

// An unavailable keyring is not a logout failure: the fallback file is still
// removed and logout reports success.
func TestLogoutSucceedsWhenKeyringIsUnavailable(t *testing.T) {
	configDir := setupEnv(t)
	kr := newCLIKeyring()
	kr.fail = true
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(configDir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := testDeps(func(context.Context, string) error { return nil }, kr)
	code, _, stderr := runWith([]string{"logout"}, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback file even without a keyring: %v", err)
	}
}

// A logout that genuinely fails (a directory where the token file should be)
// must scrub a secret-like path fragment from the diagnostic.
func TestLogoutFailureDiagnosticIsRedacted(t *testing.T) {
	configDir := setupEnv(t)
	if err := os.MkdirAll(filepath.Join(configDir, "token", "sk-directorytoken"), 0o700); err != nil {
		t.Fatal(err)
	}
	d := testDeps(func(context.Context, string) error { return nil }, nil)
	code, _, stderr := runWith([]string{"logout"}, errorReader{}, d)
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitFailure, stderr)
	}
	if strings.Contains(stderr, "sk-directorytoken") {
		t.Errorf("logout failure leaked a secret-like path: %q", stderr)
	}
}

func TestSetupHelpMentionsSetupAndLogout(t *testing.T) {
	setupEnv(t)
	code, stdout, _ := runWith([]string{"--help"}, errorReader{}, testDeps(nil, nil))
	if code != exitOK {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"setup", "logout"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing %q in:\n%s", want, stdout)
		}
	}
}

func TestConfigErrorIsRedacted(t *testing.T) {
	configDir := setupEnv(t)
	// Point --config at a path whose error message contains a sk-like fragment;
	// the diagnostic must not echo the raw path unfiltered.
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "sk-directorytoken")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runWith([]string{"--config", dir, "config", "reset"}, errorReader{}, testDeps(nil, nil))
	if code == exitOK {
		t.Fatalf("writing a config over a directory should fail (stderr=%q)", stderr)
	}
	if strings.Contains(stderr, "sk-directorytoken") {
		t.Errorf("config error leaked a secret-like path: %q", stderr)
	}
}
