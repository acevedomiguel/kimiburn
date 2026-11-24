package credentials

import (
	"net/url"
	"strings"
	"testing"
)

// syntheticToken is invented for tests. It is never a real credential.
const syntheticToken = "km_synthetic_0123456789abcdef"

func TestRedactorRemovesExactConfiguredToken(t *testing.T) {
	r := NewRedactor(syntheticToken)
	for _, in := range []string{
		"validation failed for " + syntheticToken,
		"Authorization: Bearer " + syntheticToken,
		"GET /usages?token=" + syntheticToken,
		"error: " + syntheticToken + " expired",
	} {
		out := r.Redact(in)
		if strings.Contains(out, syntheticToken) {
			t.Errorf("Redact(%q) = %q, still contains the token", in, out)
		}
		if !strings.Contains(out, redactionMarker) {
			t.Errorf("Redact(%q) = %q, want the redaction marker", in, out)
		}
	}
}

func TestRedactorRemovesURLEncodedForm(t *testing.T) {
	token := "km/token+with=chars"
	r := NewRedactor(token)
	for _, form := range []string{url.QueryEscape(token), url.PathEscape(token)} {
		in := "request to /usages?access_token=" + form
		if out := r.Redact(in); strings.Contains(out, form) {
			t.Errorf("Redact(%q) = %q, still contains the encoded token", in, out)
		}
	}
}

func TestRedactorRemovesGenericSecretShapesWithoutConfiguration(t *testing.T) {
	cases := map[string]string{
		"bearer":      "Authorization: Bearer abcd1234efgh",
		"sk-prefix":   "token sk-abcdef123456 rejected",
		"tokenquery":  "GET /usages?token=abcdef123456",
		"accessquery": "GET /usages?access_token=abcdef123456",
		"jsonbody":    `{"token":"abcdef123456"}`,
	}
	for name, in := range cases {
		out := Redact(in)
		if !strings.Contains(out, redactionMarker) {
			t.Errorf("%s: Redact(%q) = %q, want the redaction marker", name, in, out)
		}
		for _, leak := range []string{"abcd1234efgh", "sk-abcdef123456", "abcdef123456"} {
			if strings.Contains(out, leak) {
				t.Errorf("%s: Redact(%q) = %q, still contains %q", name, in, out, leak)
			}
		}
	}
}

func TestRedactorDoesNotMangleOrdinaryText(t *testing.T) {
	in := "The token limit is documented; see the token page."
	if out := Redact(in); out != in {
		t.Errorf("Redact(%q) = %q, want it unchanged", in, out)
	}
}

func TestNilRedactorStillRedactsGenericShapes(t *testing.T) {
	var r *Redactor
	if out := r.Redact("Authorization: Bearer abcd1234"); !strings.Contains(out, redactionMarker) {
		t.Errorf("a nil redactor must still scrub generic shapes: %q", out)
	}
}

func TestRedactAllHandlesNilAndValues(t *testing.T) {
	if got := RedactAll(nil); got != nil {
		t.Errorf("RedactAll(nil) = %v, want nil", got)
	}
	got := RedactAll([]string{"Bearer abcd1234", "plain"})
	if strings.Contains(got[0], "abcd1234") || got[1] != "plain" {
		t.Errorf("RedactAll = %v", got)
	}
}

// A configured secret embedded next to a JSON token field must not survive
// even when the two overlap.
func TestRedactorHandlesAdjacentSecrets(t *testing.T) {
	r := NewRedactor(syntheticToken)
	in := `{"access_token":"` + syntheticToken + `","other":1}`
	out := r.Redact(in)
	if strings.Contains(out, syntheticToken) {
		t.Errorf("Redact(%q) = %q, leaked the token", in, out)
	}
}
