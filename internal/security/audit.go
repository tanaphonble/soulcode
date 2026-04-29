package security

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEntry is a single row in the audit log. It is intentionally compact so
// the file remains readable with `tail -f`.
type AuditEntry struct {
	Time       time.Time `json:"t"`
	Tool       string    `json:"tool"`
	Args       string    `json:"args,omitempty"`     // truncated
	Approval   string    `json:"approval,omitempty"` // "auto", "user", "yolo", "denied"
	OK         bool      `json:"ok"`
	Err        string    `json:"err,omitempty"`
	ResultHash string    `json:"result_sha256,omitempty"`
	SecretHits []string  `json:"redacted,omitempty"`
}

// Auditor appends entries to ~/.soulcode/audit.log (mode 0600). It is safe
// for concurrent use.
type Auditor struct {
	mu   sync.Mutex
	path string
}

// NewAuditor opens (or creates) the audit log at ~/.soulcode/audit.log.
// Returns a non-nil Auditor even if the underlying file cannot be opened
// yet; failures are deferred to Log calls so a missing $HOME never breaks
// the agent loop.
func NewAuditor() *Auditor {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return &Auditor{}
	}
	return &Auditor{path: filepath.Join(home, ".soulcode", "audit.log")}
}

// Log appends an entry. Errors are swallowed; the audit log is best-effort
// and must never block tool execution.
func (a *Auditor) Log(e AuditEntry) {
	if a == nil || a.path == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(a.path), 0700); err != nil {
		return
	}
	f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
}

// HashResult returns a short hex SHA-256 of the tool result, suitable for
// audit.ResultHash. Empty input yields an empty string so the audit row stays
// compact.
func HashResult(s string) string {
	if s == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8]) // 16 hex chars; collision-safe for audit
}

// ErrAuditPath is returned by openLog when the audit path is unset.
var ErrAuditPath = errors.New("audit: path is not set")

// truncate caps a string at n bytes for the args field.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("…[+%d]", len(s)-n)
}

// FormatArgs returns a short representation of tool input suitable for the
// audit log. Long inputs are truncated.
func FormatArgs(raw []byte) string {
	const max = 200
	return truncate(string(raw), max)
}
