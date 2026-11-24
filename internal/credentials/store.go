// Package credentials resolves and stores Kimiburn's single Kimi Code token,
// and redacts secrets from anything the user can see. It is read-only with
// respect to Kimi: it never revokes, rotates, or transmits a token itself.
package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Kimiburn owns exactly one credential: the Kimi Code token. It is stored in
// the OS keyring under ServiceName/AccountName or, as a fallback, in a
// permission-restricted file.
const (
	ServiceName = "kimiburn"
	AccountName = "token"
	EnvToken    = "KIMIBURN_TOKEN"
)

// Sentinel errors surfaced by the credential store.
var (
	ErrKeyringUnavailable  = errors.New("credentials: the OS keyring is unavailable")
	ErrKeyringNotFound     = errors.New("credentials: the keyring holds no Kimiburn entry")
	ErrInsecurePermissions = errors.New("credentials: the token file must be readable only by its owner (0600)")
	ErrEmptyToken          = errors.New("credentials: refusing to store an empty token")
	ErrNoTokenFile         = errors.New("credentials: no token file path is configured")
)

// Keyring is the subset of the go-keyring API that the store uses.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// Options configures a Store. Tests inject a fake keyring and a temp path.
type Options struct {
	Keyring        Keyring
	TokenFile      string
	KeyringEnabled bool
	Env            map[string]string
}

// Store resolves and persists the Kimi Code token. Only Kimiburn's own
// surfaces are consulted: KIMIBURN_TOKEN, the keyring entry
// kimiburn/token, and the restricted fallback file.
type Store struct {
	keyring        Keyring
	tokenFile      string
	keyringEnabled bool

	// Env overrides the process environment; nil means os.Getenv.
	Env map[string]string
}

// NewStore builds a store from explicit options.
func NewStore(opts Options) *Store {
	return &Store{
		keyring:        opts.Keyring,
		tokenFile:      opts.TokenFile,
		keyringEnabled: opts.KeyringEnabled && opts.Keyring != nil,
		Env:            opts.Env,
	}
}

// Resolve returns the token the process should use right now, or "" when none
// is configured. Resolution order is environment, keyring, fallback file.
func (s *Store) Resolve(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	// The environment always wins and is never persisted.
	if token := s.lookupEnv(EnvToken); token != "" {
		return token, nil
	}

	if s.keyringEnabled {
		if v, err := s.keyring.Get(ServiceName, AccountName); err == nil {
			if token := strings.TrimSpace(v); token != "" {
				return token, nil
			}
		}
	}

	return readSecretFile(s.tokenFile)
}

// EnvironmentToken returns the trimmed KIMIBURN_TOKEN value, or "". It lets
// setup explain that the environment wins without persisting it.
func (s *Store) EnvironmentToken() string { return s.lookupEnv(EnvToken) }

// Save persists the token to the keyring, falling back to a 0600 file only
// when the keyring is disabled or unavailable. Because resolution prefers the
// keyring over the file, a backend transition must never leave two different
// tokens readable: a successful keyring save clears the old file, and a file
// fallback is refused while an older keyring entry still survives.
func (s *Store) Save(ctx context.Context, token string) error {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return ErrEmptyToken
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if s.keyringEnabled {
		if err := s.keyring.Set(ServiceName, AccountName, trimmed); err == nil {
			// The keyring now wins resolution, so a leftover fallback file would
			// resurrect the previous token if the keyring later went away.
			return s.removeStaleFile()
		}
		// The keyring could not take the token. Falling back to the file is safe
		// only when no older keyring entry survives, since that entry would keep
		// winning resolution over the file we are about to write.
		if s.keyringHoldsToken() {
			if err := s.keyring.Delete(ServiceName, AccountName); err != nil && !errors.Is(err, ErrKeyringNotFound) {
				return fmt.Errorf("credentials: the keyring still holds a previous token: %w", err)
			}
		}
	} else if s.keyring != nil {
		// The keyring is disabled by configuration, yet an entry from an earlier
		// run would still outrank the file. Clearing it is best effort: the file
		// save must not depend on keyring availability.
		_ = s.keyring.Delete(ServiceName, AccountName)
	}

	if s.tokenFile == "" {
		return ErrNoTokenFile
	}
	return writeSecretFile(s.tokenFile, trimmed)
}

// keyringHoldsToken reports whether the keyring currently stores a Kimiburn
// token. A keyring that cannot be read counts as holding none, which keeps the
// documented file fallback available when the keyring is simply unreachable.
func (s *Store) keyringHoldsToken() bool {
	if s.keyring == nil {
		return false
	}
	v, err := s.keyring.Get(ServiceName, AccountName)
	return err == nil && strings.TrimSpace(v) != ""
}

// removeStaleFile deletes a fallback file that a keyring save has superseded.
func (s *Store) removeStaleFile() error {
	if s.tokenFile == "" {
		return nil
	}
	if err := os.Remove(s.tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("credentials: remove superseded token file %s: %w", s.tokenFile, err)
	}
	return nil
}

// Logout removes only Kimiburn-owned credentials: its keyring entry and its
// fallback token file. Cache and configuration are never touched. It always
// attempts the keyring entry, even when the `keyring` preference is disabled,
// so flipping that setting cannot strand a previously stored token.
func (s *Store) Logout(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var failures []error
	if s.keyring != nil {
		// Not-found means there is nothing to remove, which is the expected
		// state. Any other failure means a stored credential may survive, so it
		// is reported instead of being reported as success.
		err := s.keyring.Delete(ServiceName, AccountName)
		if err != nil && !errors.Is(err, ErrKeyringNotFound) {
			failures = append(failures, fmt.Errorf("credentials: remove keyring entry: %w", err))
		}
	}
	if s.tokenFile != "" {
		if err := os.Remove(s.tokenFile); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Errorf("credentials: remove %s: %w", s.tokenFile, err))
		}
	}
	return errors.Join(failures...)
}

func (s *Store) lookupEnv(key string) string {
	if s.Env != nil {
		return strings.TrimSpace(s.Env[key])
	}
	return strings.TrimSpace(os.Getenv(key))
}
