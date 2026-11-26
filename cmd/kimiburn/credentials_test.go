package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acevedomiguel/burnblocks/kimi"
)

// cliKeyring is an in-memory keyring so CLI tests never touch the real one.
type cliKeyring struct {
	values map[string]string
	fail   bool
}

func newCLIKeyring() *cliKeyring { return &cliKeyring{values: map[string]string{}} }

func (k *cliKeyring) Get(service, user string) (string, error) {
	if k.fail {
		return "", kimi.ErrKeyringUnavailable
	}
	v, ok := k.values[service+"/"+user]
	if !ok {
		return "", kimi.ErrKeyringNotFound
	}
	return v, nil
}

func (k *cliKeyring) Set(service, user, secret string) error {
	if k.fail {
		return kimi.ErrKeyringUnavailable
	}
	k.values[service+"/"+user] = secret
	return nil
}

func (k *cliKeyring) Delete(service, user string) error {
	if k.fail {
		return kimi.ErrKeyringUnavailable
	}
	key := service + "/" + user
	if _, ok := k.values[key]; !ok {
		return kimi.ErrKeyringNotFound
	}
	delete(k.values, key)
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
	t.Setenv(kimi.TokenEnvVar, "")
	t.Setenv("KIMIBURN_KEYRING", "")
	return filepath.Join(configDir, "kimiburn")
}

// testDeps builds injectable CLI dependencies for setup/logout tests.
func testDeps(validate func(context.Context, string) error, kr kimi.Keyring) deps {
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

// cliExactToken has no sk-like shape, so only a redactor that knows the exact
// configured value can scrub it.
const cliExactToken = "km_exact_synthetic_0123456789abcdef"

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
	t.Setenv(kimi.TokenEnvVar, "env-token")
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
	if raw, err := os.ReadFile(path); err != nil || strings.TrimSpace(string(raw)) != cliToken {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
	// Windows synthesises file modes, so the POSIX bits are asserted only where
	// the OS enforces them; there the 0700 directory carries the protection.
	if kimi.PermissionsAreEnforced {
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
}

// A pre-existing group/world-readable fallback file must be refused before it
// is read, so a loose token never becomes usable.
func TestResolutionRefusesLooseTokenFileBeforeReading(t *testing.T) {
	if !kimi.PermissionsAreEnforced {
		t.Skip("POSIX permission bits are not enforced on this platform")
	}
	configDir := setupEnv(t)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "token"), []byte("loose"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := testDeps(nil, nil)
	store, err := storeFor(kimi.Paths{ConfigDir: configDir}, options{}, d)
	if err != nil {
		t.Fatalf("storeFor: %v", err)
	}
	if _, err := store.Resolve(context.Background()); !errors.Is(err, kimi.ErrInsecurePermissions) {
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
	// Assert against the real resolved cache location, not a guess.
	paths, err := kimi.DefaultPaths()
	if err != nil {
		t.Fatalf("DefaultPaths: %v", err)
	}
	cachePath := paths.CacheFile
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

// An unreachable keyring must not be reported as success: a stored entry may
// survive. Logout still removes the fallback file, and the diagnostic is a
// platform message rather than a secret.
func TestLogoutReportsUnavailableKeyringButStillRemovesFile(t *testing.T) {
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
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitFailure, stderr)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback file even when the keyring fails: %v", err)
	}
}

// A keyring with no Kimiburn entry is the ordinary "nothing to remove" state
// and must exit successfully.
func TestLogoutSucceedsWhenKimiburnHasNoKeyringEntry(t *testing.T) {
	setupEnv(t)
	d := testDeps(func(context.Context, string) error { return nil }, newCLIKeyring())
	code, _, stderr := runWith([]string{"logout"}, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
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

// The exact configured token must never survive an early configuration error.
// A non-sk shape proves the code cannot lean on the generic pattern alone.
func TestSetupEarlyConfigErrorRedactsExactEnvironmentToken(t *testing.T) {
	configDir := setupEnv(t)
	// The invalid base_url is itself the exact environment token, so a
	// generic-only redactor would echo it back in the config diagnostic.
	t.Setenv("KIMIBURN_BASE_URL", cliExactToken)
	t.Setenv(kimi.TokenEnvVar, cliExactToken)
	kr := newCLIKeyring()
	d := testDeps(func(context.Context, string) error {
		t.Error("setup must not validate when configuration is invalid")
		return nil
	}, kr)

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code == exitOK {
		t.Fatalf("invalid configuration must not succeed (stderr=%q)", stderr)
	}
	combined := stdout + stderr
	if strings.Contains(combined, cliExactToken) {
		t.Errorf("setup leaked the exact environment token: stdout=%q stderr=%q", stdout, stderr)
	}
	if len(kr.values) != 0 {
		t.Errorf("invalid configuration must store nothing: %v", kr.values)
	}
	if entries, err := os.ReadDir(configDir); err == nil && len(entries) != 0 {
		t.Errorf("invalid configuration must not write %s: %v", configDir, entries)
	}
}

// Setup must never overwrite an existing configuration, which may encode a
// deliberate backend, theme, or refresh choice.
func TestSetupPreservesExistingConfiguration(t *testing.T) {
	configDir := setupEnv(t)
	kr := newCLIKeyring()
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(configDir, "config.yaml")
	existing := []byte("# hand written\nversion: 1\nrefresh_interval: 45s\nbase_url: https://api.kimi.ai/coding/v1\ntheme: dark\ncolor: never\nkeyring: true\n")
	if err := os.WriteFile(cfgPath, existing, 0o600); err != nil {
		t.Fatal(err)
	}
	d := testDeps(func(context.Context, string) error { return nil }, kr)

	code, stdout, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, existing) {
		t.Errorf("setup must preserve the existing config byte for byte:\n%s", got)
	}
	if _, err := kr.Get("kimiburn", "token"); err != nil {
		t.Errorf("setup should still store the token: %v", err)
	}
	if strings.Contains(stdout+stderr, cliToken) {
		t.Errorf("the token must never be echoed: stdout=%q stderr=%q", stdout, stderr)
	}
}

// A disabled keyring must not persist a token while an older keyring entry
// survives and keeps outranking the file.
func TestSetupWithDisabledKeyringClearsStaleEntry(t *testing.T) {
	configDir := setupEnv(t)
	t.Setenv("KIMIBURN_KEYRING", "false")
	kr := newCLIKeyring()
	if err := kr.Set("kimiburn", "token", "stale"); err != nil {
		t.Fatal(err)
	}
	d := testDeps(func(context.Context, string) error { return nil }, kr)

	code, _, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if _, err := kr.Get("kimiburn", "token"); err == nil {
		t.Error("a disabled-keyring setup should clear the stale entry")
	}
	raw, err := os.ReadFile(filepath.Join(configDir, "token"))
	if err != nil || strings.TrimSpace(string(raw)) != cliToken {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
}

// Logout must remove credentials even when configuration disables the keyring,
// so flipping the preference cannot strand a previously stored entry.
func TestLogoutRemovesCredentialsWithKeyringDisabled(t *testing.T) {
	configDir := setupEnv(t)
	t.Setenv("KIMIBURN_KEYRING", "false")
	kr := newCLIKeyring()
	if err := kr.Set("kimiburn", "token", "stored"); err != nil {
		t.Fatal(err)
	}
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
	if _, err := kr.Get("kimiburn", "token"); err == nil {
		t.Error("logout must clear the keyring entry even when the setting disables it")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback token file: %v", err)
	}
}

// Logout must not depend on configuration being parseable: a broken config must
// not prevent clearing credentials, and the exact environment token must not
// reappear in the diagnostic.
func TestLogoutIgnoresInvalidConfigAndDoesNotLeakToken(t *testing.T) {
	configDir := setupEnv(t)
	t.Setenv("KIMIBURN_BASE_URL", cliExactToken)
	t.Setenv(kimi.TokenEnvVar, cliExactToken)
	kr := newCLIKeyring()
	if err := kr.Set("kimiburn", "token", "stored"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(configDir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := testDeps(func(context.Context, string) error { return nil }, kr)

	code, stdout, stderr := runWith([]string{"logout"}, errorReader{}, d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; logout must not depend on config validity (stderr=%q)", code, exitOK, stderr)
	}
	if _, err := kr.Get("kimiburn", "token"); err == nil {
		t.Error("logout must remove Kimiburn's keyring entry despite an invalid config")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback token file: %v", err)
	}
	if combined := stdout + stderr; strings.Contains(combined, cliExactToken) {
		t.Errorf("logout leaked the exact environment token: %q", combined)
	}
}

func TestSetupWelcomeStatesKimiburnIsIndependentAndReadOnly(t *testing.T) {
	welcome := strings.ToLower(setupWelcome())
	for _, want := range []string{"independent", "read-only", "never sends inference requests"} {
		if !strings.Contains(welcome, want) {
			t.Errorf("setup welcome must state %q:\n%s", want, setupWelcome())
		}
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
