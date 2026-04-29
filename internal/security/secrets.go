package security

import (
	"regexp"
	"strings"
)

// secretPattern identifies a kind of high-risk credential that we never want
// to send to a third-party LLM provider as part of a tool result.
type secretPattern struct {
	name string
	re   *regexp.Regexp
}

// Order matters: more specific patterns must come first so they win
// over more permissive ones (e.g. Anthropic's sk-ant- prefix would also match
// the OpenAI sk- pattern if checked second).
var secretPatterns = []secretPattern{
	{"PEM private key", regexp.MustCompile(`-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY-----`)},
	{"AWS access key", regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`)},
	{"AWS secret", regexp.MustCompile(`\baws_secret_access_key\s*=\s*[A-Za-z0-9/+=]{40}\b`)},
	{"GitHub fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{82}\b`)},
	{"GitHub token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`)},
	{"GitLab token", regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	{"Slack token", regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)},
	{"Stripe live secret", regexp.MustCompile(`\bsk_live_[A-Za-z0-9]{20,}\b`)},
	{"Stripe restricted", regexp.MustCompile(`\brk_live_[A-Za-z0-9]{20,}\b`)},
	{"Anthropic API key", regexp.MustCompile(`\bsk-ant-(?:api03|admin01)-[A-Za-z0-9_-]{20,}\b`)},
	{"OpenAI API key", regexp.MustCompile(`\bsk-(?:proj-|svcacct-)?[A-Za-z0-9_-]{20,}\b`)},
	{"Google API key", regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{"JWT", regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`)},
	{"npm token", regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{"PyPI token", regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}\b`)},
	{"Heroku key", regexp.MustCompile(`\bheroku[a-z_-]*[\s:=]+[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)},
}

// Redact replaces any secret-like substring in s with a placeholder that
// preserves the secret's kind so the model still understands the structure.
// It returns the redacted string and the list of secret kinds that were hit
// (so the caller can warn the user).
func Redact(s string) (string, []string) {
	if s == "" {
		return s, nil
	}
	var hits []string
	out := s
	for _, p := range secretPatterns {
		matched := false
		out = p.re.ReplaceAllStringFunc(out, func(_ string) string {
			matched = true
			return "[REDACTED:" + p.name + "]"
		})
		if matched {
			hits = append(hits, p.name)
		}
	}
	return out, dedupe(hits)
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// Looks like a secret if it has high entropy in a single token? We deliberately
// avoid an entropy-based scanner here: it produces false positives on hex
// hashes, base64 build IDs, and long tokens that are normal in code. The named
// patterns above are conservative and catch the most damaging leaks.

// HasSecretShape is exported for tests and for callers that want to detect
// without redacting.
func HasSecretShape(s string) bool {
	for _, p := range secretPatterns {
		if p.re.MatchString(s) {
			return true
		}
	}
	return false
}

// SummariseHits formats a slice of pattern names for user-facing messages.
func SummariseHits(hits []string) string {
	if len(hits) == 0 {
		return ""
	}
	return strings.Join(hits, ", ")
}
