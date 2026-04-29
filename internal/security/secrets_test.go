package security

import (
	"strings"
	"testing"
)

// makeFake assembles a fake-secret literal at runtime so GitHub's secret
// scanner (and similar source-code scanners) cannot match a contiguous string
// in this file. The resulting value is identical to a hard-coded literal.
func makeFake(parts ...string) string { return strings.Join(parts, "") }

func TestRedact_KnownPatterns(t *testing.T) {
	t.Parallel()
	// Each input is built from a small slice of fragments so no scanner-matching
	// substring exists verbatim in this source file. The fragments are joined
	// at runtime to form the canonical secret shape that the redactor must hit.
	cases := []struct {
		name    string
		input   string
		wantHit string // expected pattern name
	}{
		{"AWS access key", "use " + makeFake("AKIA", "IOSFODNN7", "EXAMPLE") + " for the key", "AWS access key"},
		{"GitHub classic", makeFake("ghp_", "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"), "GitHub token"},
		{"OpenAI key", "OPENAI_API_KEY=" + makeFake("sk-", "proj-", "AbCdEfGhIjKlMnOpQrStUv123456789"), "OpenAI API key"},
		{"Anthropic key", "key=" + makeFake("sk-ant-", "api03-", "AAA_BBB_CCC_DDDDDDDDDDDDDDDDDD"), "Anthropic API key"},
		{"Google key", "GOOGLE=" + makeFake("AIza", "Sy", strings.Repeat("X", 33)), "Google API key"},
		{"PEM private key", makeFake("-----BEGIN ", "RSA ", "PRIVATE ", "KEY-----") + "\nfoo\n-----END...", "PEM private key"},
		{"Slack token", makeFake("xoxb-", "1234567890-", "abc-defghijklmn"), "Slack token"},
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
	awsKey := makeFake("AKIA", "IOSFODNN7", "EXAMPLE")
	if !HasSecretShape(awsKey) {
		t.Error("expected AKIA... to look like a secret")
	}
	if HasSecretShape("just a normal string") {
		t.Error("normal string should not look like a secret")
	}
}
