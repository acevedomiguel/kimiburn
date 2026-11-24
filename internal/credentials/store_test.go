package credentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeKeyring is an in-memory keyring so tests never touch the real one.
type fakeKeyring struct {
	values     map[string]string
	fail       bool
	failSet    bool
	failDelete bool
}

func newFakeKeyring() *fakeKeyring { return &fakeKeyring{values: map[string]string{}} }

func (f *fakeKeyring) Get(service, user string) (string, error) {
	if f.fail {
		return "", ErrKeyringUnavailable
	}
	v, ok := f.values[service+"/"+user]
	if !ok {
		return "", ErrKeyringNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Set(service, user, secret string) error {
	if f.fail || f.failSet {
		return ErrKeyringUnavailable
	}
	f.values[service+"/"+user] = secret
	return nil
}

func (f *fakeKeyring) Delete(service, user string) error {
	if f.fail || f.failDelete {
		return ErrKeyringUnavailable
	}
	key := service + "/" + user
	if _, ok := f.values[key]; !ok {
		return ErrKeyringNotFound
	}
	delete(f.values, key)
	return nil
}

func newStore(t *testing.T, kr Keyring) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s := NewStore(Options{
		Keyring:        kr,
		TokenFile:      filepath.Join(dir, "token"),
		KeyringEnabled: kr != nil,
	})
	return s, dir
}

func TestResolutionOrderEnvironmentBeatsKeyringAndFile(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	if err := kr.Set(ServiceName, AccountName, "keyring-token"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.Env = map[string]string{EnvToken: "env-token"}

	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "env-token" {
		t.Errorf("token = %q, want the environment token", token)
	}
}

func TestResolutionOrderKeyringBeatsFile(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	if err := kr.Set(ServiceName, AccountName, "keyring-token"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "keyring-token" {
		t.Errorf("token = %q, want the keyring token", token)
	}
}

func TestResolutionFallsBackToRestrictedFile(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("file-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "file-token" {
		t.Errorf("token = %q, want the trimmed file token", token)
	}
}

func TestResolutionUsesKeyringEntryKimiburnToken(t *testing.T) {
	kr := newFakeKeyring()
	s, _ := newStore(t, kr)
	if err := kr.Set("kimiburn", "token", "keyring-token"); err != nil {
		t.Fatal(err)
	}

	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "keyring-token" {
		t.Errorf("token = %q, want the kimiburn/token entry", token)
	}
	if _, err := kr.Get("routeburn", "management"); err == nil {
		t.Error("resolution must never read another application's keyring entry")
	}
}

func TestMissingTokenIsNotAnError(t *testing.T) {
	s, _ := newStore(t, newFakeKeyring())
	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty", token)
	}
}

func TestEnvironmentTokenIsNeverPersisted(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	s.Env = map[string]string{EnvToken: "env-token"}

	if _, err := s.Resolve(context.Background()); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(kr.values) != 0 {
		t.Errorf("resolution must not write the environment token to the keyring: %v", kr.values)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("resolution must not create the fallback file: %v", entries)
	}
}

func TestSavePrefersKeyring(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)

	if err := s.Save(context.Background(), "new-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if kr.values["kimiburn/token"] != "new-token" {
		t.Errorf("keyring = %v, want the saved token", kr.values)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); !os.IsNotExist(err) {
		t.Errorf("an available keyring must not write the fallback file: %v", err)
	}
}

func TestSaveFallsBackToFileWhenKeyringUnavailable(t *testing.T) {
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)

	if err := s.Save(context.Background(), "new-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path := filepath.Join(dir, "token")
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "new-token" {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
	// Windows synthesises file modes, so POSIX bits are asserted only where the
	// OS actually enforces them. On Windows the file is protected by the ACL
	// inherited from the per-user profile directory, not by a mode bit.
	if PermissionsAreEnforced {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat fallback file: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("fallback mode = %o, want 0600", perm)
		}
		if dirInfo, err := os.Stat(dir); err != nil || dirInfo.Mode().Perm() != 0o700 {
			t.Errorf("fallback directory mode = %v (err %v), want 0700", dirInfo.Mode().Perm(), err)
		}
	}
}

func TestSaveFallsBackToFileWhenKeyringDisabled(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(Options{TokenFile: filepath.Join(dir, "token"), KeyringEnabled: false})

	if err := s.Save(context.Background(), "file-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "token")); err != nil {
		t.Errorf("disabled keyring must use the fallback file: %v", err)
	}
}

func TestSaveRejectsEmptyToken(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	if err := s.Save(context.Background(), "   "); !errors.Is(err, ErrEmptyToken) {
		t.Fatalf("Save err = %v, want ErrEmptyToken", err)
	}
	if len(kr.values) != 0 {
		t.Errorf("an empty token must not reach the keyring: %v", kr.values)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("an empty token must not create a file: %v", entries)
	}
}

func TestReadFileRefusesGroupOrWorldReadableToken(t *testing.T) {
	if !PermissionsAreEnforced {
		t.Skip("POSIX permission bits are not enforced on this platform")
	}
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("loose-token"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Resolve(context.Background()); !errors.Is(err, ErrInsecurePermissions) {
		t.Fatalf("Resolve err = %v, want ErrInsecurePermissions", err)
	}
}

func TestReadFileRefusesDirectory(t *testing.T) {
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)
	if err := os.Mkdir(filepath.Join(dir, "token"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(context.Background()); err == nil {
		t.Fatal("a directory in place of the token file must be an error")
	}
}

func TestSaveTightensAnExistingLooseFile(t *testing.T) {
	if !PermissionsAreEnforced {
		t.Skip("POSIX permission bits are not enforced on this platform")
	}
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.Save(context.Background(), "new-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
}

func TestLogoutRemovesOnlyKimiburnCredentialsAndKeepsCache(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	if err := kr.Set(ServiceName, AccountName, "token"); err != nil {
		t.Fatal(err)
	}
	if err := kr.Set("other-app", "token", "keep"); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache", "state.json")
	if err := os.MkdirAll(filepath.Dir(cache), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := kr.Get(ServiceName, AccountName); err == nil {
		t.Error("Logout must remove Kimiburn's keyring entry")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("Logout must remove the fallback token file: %v", err)
	}
	if v, err := kr.Get("other-app", "token"); err != nil || v != "keep" {
		t.Errorf("Logout must not touch another application's entry: %q err=%v", v, err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Errorf("Logout must not delete the cache: %v", err)
	}
}

// A keyring with no entry reports not-found, which is the expected "nothing to
// remove" state and must not fail logout.
func TestLogoutSucceedsWhenTheKeyringHasNoEntry(t *testing.T) {
	s, _ := newStore(t, newFakeKeyring())
	if err := s.Logout(context.Background()); err != nil {
		t.Fatalf("Logout with nothing stored: %v", err)
	}
}

// A keyring that cannot be reached is a genuine failure: logout must report it
// while still removing the fallback file rather than silently claiming success.
func TestLogoutReportsUnavailableKeyringButRemovesFile(t *testing.T) {
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := s.Logout(context.Background())
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("Logout err = %v, want ErrKeyringUnavailable", err)
	}
	if _, statErr := os.Stat(tokenPath); !os.IsNotExist(statErr) {
		t.Errorf("logout must still remove the fallback file: %v", statErr)
	}
}

// Once the keyring holds the token, a stale fallback file must not survive:
// otherwise a later keyring outage would silently resurrect the old token.
func TestSaveReportsStaleFileCleanupFailure(t *testing.T) {
	kr := newFakeKeyring()
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token")
	// A non-empty directory cannot be removed, so cleanup must fail.
	if err := os.MkdirAll(filepath.Join(tokenPath, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	s := NewStore(Options{Keyring: kr, TokenFile: tokenPath, KeyringEnabled: true})

	err := s.Save(context.Background(), "new-token")
	if err == nil {
		t.Fatal("Save must report a failure to remove the stale fallback file")
	}
	if kr.values[ServiceName+"/"+AccountName] != "new-token" {
		t.Errorf("the keyring save itself already succeeded: %v", kr.values)
	}
}

// A disabled keyring must not leave an accessible stale entry that would keep
// outranking the file we are about to write.
func TestSaveWithDisabledKeyringClearsAccessibleStaleEntry(t *testing.T) {
	kr := newFakeKeyring()
	if err := kr.Set(ServiceName, AccountName, "old-keyring-token"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s := NewStore(Options{
		Keyring:        kr,
		TokenFile:      filepath.Join(dir, "token"),
		KeyringEnabled: false,
	})

	if err := s.Save(context.Background(), "file-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := kr.Get(ServiceName, AccountName); err == nil {
		t.Error("a disabled-keyring save should clear an accessible stale entry")
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "token")); err != nil || string(raw) != "file-token" {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
}

// A successful save must not leave a temporary file behind.
func TestWriteSecretFileLeavesNoTemporaryFilesOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := writeSecretFile(path, "secret-value"); err != nil {
		t.Fatalf("writeSecretFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "token" {
		t.Errorf("directory should hold only the token file: %v", entries)
	}
}

// A failed replacement must not leave a temporary file that holds the secret.
func TestWriteSecretFileCleansUpTemporaryFileOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory at the target path makes the final rename fail.
	path := filepath.Join(dir, "token")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := writeSecretFile(path, "secret-value"); err == nil {
		t.Fatal("writing over a directory must fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "token" {
			t.Errorf("a failed write leaked a temporary file: %s", e.Name())
		}
	}
}

// Replacement must overwrite an existing token rather than reuse its mode.
func TestWriteSecretFileReplacesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("old-token"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeSecretFile(path, "new-token"); err != nil {
		t.Fatalf("writeSecretFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "new-token" {
		t.Errorf("contents = %q err=%v, want the replacement", raw, err)
	}
}

// A successful write must produce an owner-only file on POSIX systems.
func TestWriteSecretFileIsOwnerOnly(t *testing.T) {
	if !PermissionsAreEnforced {
		t.Skip("POSIX permission bits are not enforced on this platform")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := writeSecretFile(path, "secret-value"); err != nil {
		t.Fatalf("writeSecretFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
}

// Logout must be independent of the keyring resolution preference: turning the
// keyring off in config must not strand a previously stored entry.
func TestLogoutDeletesKeyringEntryEvenWhenDisabled(t *testing.T) {
	kr := newFakeKeyring()
	if err := kr.Set(ServiceName, AccountName, "stored"); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	s := NewStore(Options{
		Keyring:        kr,
		TokenFile:      filepath.Join(dir, "token"),
		KeyringEnabled: false,
	})
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("file-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.Logout(context.Background()); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := kr.Get(ServiceName, AccountName); err == nil {
		t.Error("logout must clear Kimiburn's keyring entry even with the keyring disabled")
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("logout must remove the fallback file: %v", err)
	}
}

// A disabled keyring must not stop file save from succeeding.
func TestSaveWithDisabledKeyringIgnoresUnreachableKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.fail = true
	dir := t.TempDir()
	s := NewStore(Options{
		Keyring:        kr,
		TokenFile:      filepath.Join(dir, "token"),
		KeyringEnabled: false,
	})

	if err := s.Save(context.Background(), "file-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if raw, err := os.ReadFile(filepath.Join(dir, "token")); err != nil || string(raw) != "file-token" {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
}

// Saving to the keyring must not leave a stale fallback file behind, because a
// later keyring outage would silently resurrect the old token.
func TestSaveRemovesStaleFallbackFileAfterKeyringSave(t *testing.T) {
	kr := newFakeKeyring()
	s, dir := newStore(t, kr)
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte("stale-token"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := s.Save(context.Background(), "new-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Errorf("a keyring save must clear the stale fallback file: %v", err)
	}
	if kr.values[ServiceName+"/"+AccountName] != "new-token" {
		t.Errorf("keyring = %v, want the new token", kr.values)
	}
}

// Falling back to file while an old keyring entry exists would leave two
// different tokens readable, so the save must fail instead of claiming success.
func TestSaveFailsWhenStaleKeyringEntryCannotBeCleared(t *testing.T) {
	kr := newFakeKeyring()
	if err := kr.Set(ServiceName, AccountName, "old-keyring-token"); err != nil {
		t.Fatal(err)
	}
	kr.failSet = true
	kr.failDelete = true
	s, dir := newStore(t, kr)

	err := s.Save(context.Background(), "new-token")
	if err == nil {
		t.Fatal("Save must not report success while the old keyring entry survives")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "token")); !os.IsNotExist(statErr) {
		t.Errorf("a failed save must not write the fallback file: %v", statErr)
	}
	if kr.values[ServiceName+"/"+AccountName] != "old-keyring-token" {
		t.Errorf("the stale entry must be left untouched for the user to inspect: %v", kr.values)
	}
}

// When the keyring is simply unavailable and no entry exists, the documented
// file fallback stays available.
func TestSaveStillFallsBackToFileWhenKeyringHasNoEntry(t *testing.T) {
	kr := newFakeKeyring()
	kr.fail = true
	s, dir := newStore(t, kr)

	if err := s.Save(context.Background(), "new-token"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "token"))
	if err != nil || string(raw) != "new-token" {
		t.Errorf("fallback contents = %q err=%v", raw, err)
	}
}

// The store must never consult names owned by other applications.
func TestStoreNeverReadsForeignEnvironment(t *testing.T) {
	s, _ := newStore(t, nil)
	s.Env = map[string]string{
		"KIMI_API_KEY":       "foreign",
		"OPENROUTER_API_KEY": "foreign",
	}
	token, err := s.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if token != "" {
		t.Errorf("token = %q, want empty; foreign variables must be ignored", token)
	}
	if got := s.EnvironmentToken(); got != "" {
		t.Errorf("EnvironmentToken = %q, want empty", got)
	}
	if strings.Contains(EnvToken, "KIMI_API") {
		t.Errorf("the supported environment variable must stay %s", EnvToken)
	}
}
