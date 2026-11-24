// Package cache persists Kimiburn's private, versioned state envelope.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/acevedomiguel/kimiburn/internal/domain"
)

var (
	// ErrMalformed identifies invalid cache JSON or shape without exposing the
	// cached provider payload in the error text.
	ErrMalformed = errors.New("cache: malformed state")
	// ErrUnsupportedVersion prevents unknown cache or snapshot schemas from
	// being rendered as zero-valued data.
	ErrUnsupportedVersion = errors.New("cache: unsupported schema version")
)

// StateStore is the application-facing cache boundary.
type StateStore interface {
	Load() (domain.StoredState, error)
	Save(domain.StoredState) error
}

// Store is an atomic file-backed StateStore.
type Store struct{ path string }

// NewStore returns a file-backed store rooted at path.
func NewStore(path string) *Store { return &Store{path: path} }

// Load returns an empty v1 state when no cache exists.
func (s *Store) Load() (domain.StoredState, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.EmptyStoredState(), nil
		}
		return domain.StoredState{}, fmt.Errorf("cache: read state: %w", err)
	}
	var state domain.StoredState
	if err := json.Unmarshal(data, &state); err != nil {
		return domain.StoredState{}, ErrMalformed
	}
	if err := validateState(state); err != nil {
		return domain.StoredState{}, err
	}
	if state.Samples == nil {
		state.Samples = make(map[domain.QuotaKey][]domain.Sample)
	}
	return state, nil
}

// Save writes and fsyncs a same-directory temporary file before replacing the
// current cache. Provider values are never included in returned errors.
func (s *Store) Save(state domain.StoredState) error {
	if err := validateState(state); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return errors.New("cache: encode state")
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cache: create directory: %w", err)
	}
	if permissionsAreEnforced() {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("cache: secure directory: %w", err)
		}
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("cache: create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if permissionsAreEnforced() {
		if err := tmp.Chmod(0o600); err != nil {
			cleanup()
			return fmt.Errorf("cache: secure temporary file: %w", err)
		}
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("cache: write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("cache: sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: close state: %w", err)
	}
	if err := replaceFile(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("cache: replace state: %w", err)
	}
	return nil
}

func validateState(state domain.StoredState) error {
	if state.CacheVersion != domain.CacheSchemaVersion {
		return fmt.Errorf("%w: cache schema %d", ErrUnsupportedVersion, state.CacheVersion)
	}
	if state.Snapshot != nil && state.Snapshot.SchemaVersion != domain.SnapshotSchemaVersion {
		return fmt.Errorf("%w: snapshot schema %d", ErrUnsupportedVersion, state.Snapshot.SchemaVersion)
	}
	return nil
}

// DisabledStore implements --no-cache semantics without holding a path or
// performing any filesystem operation.
type DisabledStore struct{}

// NewDisabledStore returns an inert store for disabled cache operation.
func NewDisabledStore() DisabledStore { return DisabledStore{} }

// Load returns cold-start state without reading storage.
func (DisabledStore) Load() (domain.StoredState, error) { return domain.EmptyStoredState(), nil }

// Save discards state without writing storage.
func (DisabledStore) Save(domain.StoredState) error { return nil }
