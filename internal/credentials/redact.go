package credentials

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// redactionMarker is what replaces anything that looks like a secret.
const redactionMarker = "[redacted]"

// Generic secret shapes. They catch leaks that occur before the configured
// token is known, and tokens that were never configured on this machine.
var (
	bearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	tokenQueryPattern = regexp.MustCompile(`(?i)\b(token|access_token)=[^&\s"']+`)
	jsonTokenPattern  = regexp.MustCompile(`(?i)"(token|access_token)"\s*:\s*"[^"]*"`)
	skPattern         = regexp.MustCompile(`\bsk-[A-Za-z0-9._-]{4,}`)
)

// Redactor scrubs one configured secret plus generic secret shapes. The
// configured token is matched literally and in its URL-escaped form.
type Redactor struct {
	secrets []string
}

// NewRedactor returns a redactor aware of the given secrets. Empty values are
// ignored, and the longest secret is matched first so overlapping tokens are
// fully removed.
func NewRedactor(secrets ...string) *Redactor {
	r := &Redactor{}
	seen := map[string]bool{}
	for _, s := range secrets {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		for _, form := range []string{s, url.QueryEscape(s), url.PathEscape(s)} {
			if form == "" || seen[form] {
				continue
			}
			seen[form] = true
			r.secrets = append(r.secrets, form)
		}
	}
	sort.Slice(r.secrets, func(i, j int) bool { return len(r.secrets[i]) > len(r.secrets[j]) })
	return r
}

// Redact scrubs the receiver's secrets and generic secret shapes from s. A nil
// receiver still scrubs generic shapes, so a missing configuration never
// disables redaction.
func (r *Redactor) Redact(s string) string {
	if s == "" {
		return s
	}
	if r != nil {
		for _, secret := range r.secrets {
			s = strings.ReplaceAll(s, secret, redactionMarker)
		}
	}
	return redactGeneric(s)
}

// Redact scrubs generic secret shapes from s without knowing the token.
func Redact(s string) string { return redactGeneric(s) }

// RedactAll applies Redact to every value.
func RedactAll(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = Redact(v)
	}
	return out
}

func redactGeneric(s string) string {
	if s == "" {
		return s
	}
	s = bearerPattern.ReplaceAllString(s, "Bearer "+redactionMarker)
	s = tokenQueryPattern.ReplaceAllString(s, "$1="+redactionMarker)
	s = jsonTokenPattern.ReplaceAllString(s, `"$1":"`+redactionMarker+`"`)
	s = skPattern.ReplaceAllString(s, redactionMarker)
	return s
}
