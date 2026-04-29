package security

import (
	"strings"
	"testing"
)

func TestRedact_KnownPatterns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		wantHit string // expected pattern name
	}{
		{"AWS access key", "use AKIAIOSFODNN7EXAMPLE for the key", "AWS access key"},
		{"GitHub classic", "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", "GitHub token"},
		{"OpenAI key", "OPENAI_API_KEY=sk-proj-AbCdEfGhIjKlMnOpQrStUv123456789", "OpenAI API key"},
		{"Anthropic key", "key=sk-ant-api03-AAA_BBB_CCC_DDDDDDDDDDDDDDDDDD", "Anthropic API key"},
		{"Google key", "GOOGLE=AIzaSyA1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6Q", "Google API key"}, // 35 chars after AIza
		{"PEM private key", "-----BEGIN RSA PRIVATE KEY-----\nfoo\n-----END...", "PEM private key"},
		{"Slack token", "xoxb-1234567890-abc-defghijklmn", "Slack token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			redacted, hits := Redact(tc.input)
			if !strings.Contains(redacted, "[REDACTED:") {
				t.Errorf("expected redaction marker, got %q", redacted)
			}
			found := false
			for _, h := range hits {
				if h == tc.wantHit {
					found = true
				}
			}
			if !found {
				t.Errorf("expected hit %q, got %v", tc.wantHit, hits)
			}
		})
	}
}

func TestRedact_NoChangeOnCleanString(t *testing.T) {
	t.Parallel()
	in := "package main\nfunc Hello() string { return \"world\" }"
	out, hits := Redact(in)
	if out != in {
		t.Errorf("clean input mutated: %q", out)
	}
	if len(hits) != 0 {
		t.Errorf("clean input reported hits: %v", hits)
	}
}

func TestHasSecretShape(t *testing.T) {
	t.Parallel()
	if !HasSecretShape("AKIAIOSFODNN7EXAMPLE") {
		t.Error("expected AKIA... to look like a secret")
	}
	if HasSecretShape("just a normal string") {
		t.Error("normal string should not look like a secret")
	}
}
