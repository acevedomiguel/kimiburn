package kimiburn

import (
	"net/http"
	"time"

	"github.com/acevedomiguel/kimiburn/internal/kimi"
)

type sourceOptions struct {
	httpClient *http.Client
	now        func() time.Time
}

// SourceOption configures a usage source without changing its provider-neutral
// boundary.
type SourceOption func(*sourceOptions)

// WithHTTPClient injects the HTTP client used by a source. The source always
// refuses redirects, including when the supplied client permits them. Nil keeps
// the default client.
func WithHTTPClient(client *http.Client) SourceOption {
	return func(options *sourceOptions) { options.httpClient = client }
}

// WithClock injects the clock used for observation and retry timestamps. Nil
// keeps the system clock.
func WithClock(now func() time.Time) SourceOption {
	return func(options *sourceOptions) { options.now = now }
}

// NewSource constructs a bounded Kimi Code usage source from an explicitly
// selected base and caller-supplied token. It performs no credential discovery
// and no I/O until ReadUsage is called.
func NewSource(baseURL, token string, opts ...SourceOption) (UsageSource, error) {
	var options sourceOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&options)
		}
	}
	clientOptions := []kimi.Option{kimi.WithBaseURL(baseURL), kimi.WithToken(token)}
	if options.httpClient != nil {
		clientOptions = append(clientOptions, kimi.WithHTTPClient(options.httpClient))
	}
	if options.now != nil {
		clientOptions = append(clientOptions, kimi.WithClock(options.now))
	}
	return kimi.NewClient(clientOptions...)
}
