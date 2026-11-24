package credentials

import (
	"errors"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// The adapter must keep "no entry" distinct from "the keyring could not be
// reached": logout ignores only the former, so conflating them would make a
// failed deletion look like success.
func TestMapKeyringErrorDistinguishesNotFound(t *testing.T) {
	if err := mapKeyringError(nil); err != nil {
		t.Fatalf("mapKeyringError(nil) = %v, want nil", err)
	}
	if err := mapKeyringError(keyring.ErrNotFound); !errors.Is(err, ErrKeyringNotFound) {
		t.Errorf("mapKeyringError(ErrNotFound) = %v, want ErrKeyringNotFound", err)
	}
}

func TestMapKeyringErrorMarksOtherFailuresUnavailable(t *testing.T) {
	boom := errors.New("secret service is not reachable")
	err := mapKeyringError(boom)
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("mapKeyringError(boom) = %v, want ErrKeyringUnavailable", err)
	}
	if errors.Is(err, ErrKeyringNotFound) {
		t.Error("a real failure must never look like a missing entry")
	}
	if !errors.Is(err, boom) {
		t.Errorf("the mapped error should retain its cause: %v", err)
	}
}
