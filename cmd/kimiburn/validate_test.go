package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/acevedomiguel/burnblocks/kimi"
)

// Production setup must validate through the bounded Kimi client rather than a
// nil validator. The factory is resolved per invocation so the explicit
// --config path reaches the validator.
func TestDefaultDepsProvidesTokenValidator(t *testing.T) {
	d := defaultDeps()
	if d.validateFactory == nil {
		t.Fatal("defaultDeps must supply a production token validator factory")
	}
	if d.validate != nil {
		t.Error("the factory must be resolved per invocation, not frozen at construction")
	}
}

// runWithDeps must build the validator from the factory with the parsed
// --config path, and setup must call it exactly once before persisting.
func TestRunWithDepsBuildsValidatorFromFactory(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	explicit := t.TempDir() + "/config.yaml"

	var factoryCalls int32
	var validated []string
	d := deps{
		keyring:     kr,
		interactive: func(io.Reader) bool { return false },
		readSecret:  readSecretNoEcho,
		validateFactory: func(configPath string) func(context.Context, string) error {
			atomic.AddInt32(&factoryCalls, 1)
			if configPath != explicit {
				t.Errorf("factory config path = %q, want %q", configPath, explicit)
			}
			return func(_ context.Context, token string) error {
				validated = append(validated, token)
				return nil
			}
		},
	}

	code, _, stderr := runWith([]string{"--config", explicit, "setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr=%q)", code, exitOK, stderr)
	}
	if got := atomic.LoadInt32(&factoryCalls); got != 1 {
		t.Errorf("factory calls = %d, want 1", got)
	}
	if len(validated) != 1 || validated[0] != cliToken {
		t.Errorf("validated = %v, want exactly the piped token", validated)
	}
	if kr.values["kimiburn/token"] != cliToken {
		t.Errorf("keyring = %v, want the validated token stored", kr.values)
	}
	if strings.Contains(stderr, cliToken) {
		t.Errorf("stderr leaked the token: %q", stderr)
	}
}

// A failing production validator must prevent persistence, exactly like an
// injected validator.
func TestFactoryValidatorFailurePreventsStorage(t *testing.T) {
	setupEnv(t)
	kr := newCLIKeyring()
	d := deps{
		keyring:     kr,
		interactive: func(io.Reader) bool { return false },
		readSecret:  readSecretNoEcho,
		validateFactory: func(string) func(context.Context, string) error {
			return func(context.Context, string) error {
				return errors.New("kimi: token is invalid or expired (401 Unauthorized) at /usages")
			}
		},
	}

	code, _, stderr := runWith([]string{"setup"}, strings.NewReader(cliToken+"\n"), d)
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if len(kr.values) != 0 {
		t.Errorf("a failed validation must store nothing: %v", kr.values)
	}
	if !strings.Contains(stderr, "validation failed") {
		t.Errorf("stderr should report the validation failure: %q", stderr)
	}
}

// readUsageOnce must perform exactly one request to /usages with the documented
// headers. It is the seam the production validator uses, and it accepts a
// loopback base only because tests inject one.
func TestReadUsageOncePerformsExactlyOneRead(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.Method != http.MethodGet || r.URL.Path != "/usages" {
			t.Errorf("request = %s %s, want GET /usages", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+cliToken {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("accept = %q", got)
		}
		if got := r.Header.Get("User-Agent"); !strings.HasPrefix(got, "Kimiburn/") {
			t.Errorf("user agent = %q", got)
		}
		io.WriteString(w, `{"usages":{}}`)
	}))
	defer srv.Close()

	if err := readUsageOnce(context.Background(), srv.URL, cliToken, "test"); err != nil {
		t.Fatalf("readUsageOnce: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("requests = %d, want exactly 1", got)
	}
}

// The validator must never reach a non-loopback host that is not one of the two
// documented bases, and it must never send a request there.
func TestReadUsageOnceRejectsOtherProducts(t *testing.T) {
	for _, base := range []string{
		"https://api.moonshot.ai/v1",
		"https://openrouter.ai/api/v1",
		"http://api.kimi.com/coding/v1",
	} {
		err := readUsageOnce(context.Background(), base, cliToken, "test")
		if err == nil {
			t.Fatalf("readUsageOnce(%q) = nil error, want a refusal", base)
		}
		if strings.Contains(err.Error(), cliToken) {
			t.Errorf("refusal leaked the token: %v", err)
		}
	}
}

// validateToken must resolve the configured base URL and fail safely on an
// invalid configuration without contacting the network.
func TestValidateTokenRejectsInvalidConfigurationWithoutNetwork(t *testing.T) {
	setupEnv(t)
	t.Setenv("KIMIBURN_BASE_URL", "https://api.moonshot.ai/v1")

	err := validateToken("")(context.Background(), cliToken)
	if err == nil {
		t.Fatal("validateToken must fail on a disallowed base URL")
	}
	if strings.Contains(err.Error(), cliToken) {
		t.Errorf("configuration error leaked the token: %v", err)
	}
}

// validateToken must build its client with the configured base URL, so a valid
// production configuration is accepted by the allowlist.
func TestValidateTokenAcceptsAllowedBase(t *testing.T) {
	setupEnv(t)
	t.Setenv("KIMIBURN_BASE_URL", kimi.OverseasBaseURL)

	// The client is constructed successfully; the read itself is not attempted
	// here because that would reach the real network. A canceled context proves
	// the client is reached and stops before any request.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := validateToken("")(ctx, cliToken)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled from the bounded client", err)
	}
}

// The production validator must never be mistaken for a credential store: it
// only reads usage. This test proves the seam is wired to the real client type
// by checking a synthetic server response is accepted end to end.
func TestReadUsageOnceAcceptsObjectWithoutQuotas(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"goods_version":"v1"}`)
	}))
	defer srv.Close()

	if err := readUsageOnce(context.Background(), srv.URL, cliToken, "test"); err != nil {
		t.Fatalf("readUsageOnce: %v", err)
	}
}
