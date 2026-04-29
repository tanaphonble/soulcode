// Package security provides defensive primitives for soulcode's tool layer:
// path scoping, sensitive-file blocking, dangerous-command detection,
// secret redaction, audit logging, and human-in-the-loop approval.
package security

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrOutsideWorkdir is returned when a path resolves outside the working
// directory boundary.
var ErrOutsideWorkdir = errors.New("path is outside the working directory")

// ErrSensitivePath is returned when a path matches a known sensitive location
// (private keys, cloud credentials, soulcode's own config).
var ErrSensitivePath = errors.New("path is blocked by security policy")

// sensitiveDirs are absolute directory prefixes that are always off-limits.
// Populated lazily so tests with HOME=temp work.
func sensitiveDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".aws"),
		filepath.Join(home, ".gnupg"),
		filepath.Join(home, ".config", "gh"),
		filepath.Join(home, ".docker"),
		filepath.Join(home, ".kube"),
		filepath.Join(home, ".soulcode"), // protect our own API key
	}
}

// sensitiveFiles are basenames that are always off-limits regardless of
// location, on the grounds that they are essentially never legitimate to read
// from a coding agent's perspective.
var sensitiveFiles = map[string]bool{
	".netrc":       true,
	"id_rsa":       true,
	"id_dsa":       true,
	"id_ecdsa":     true,
	"id_ed25519":   true,
	"id_xmss":      true,
	".pgpass":      true,
	".npmrc":       true, // often holds auth tokens
	".pypirc":      true,
	"credentials":  true, // matches ~/.aws/credentials etc; also project files named this
	"shadow":       true,
	"master.key":   true,
	"secrets.yaml": true,
	"secrets.yml":  true,
}

// sensitiveExts are file extensions that typically mark private key material.
var sensitiveExts = map[string]bool{
	".pem":  true,
	".key":  true,
	".p12":  true,
	".pfx":  true,
	".jks":  true,
	".asc":  true, // GPG armoured key
	".gpg":  true,
	".kdbx": true, // KeePass
}

// dotEnvAllowed lists .env-style files that are safe to read because they
// contain templates/examples rather than real values.
var dotEnvAllowed = map[string]bool{
	".env.example":  true,
	".env.sample":   true,
	".env.template": true,
	".env.dist":     true,
}

// IsSensitive reports whether absPath matches any blocked pattern.
// absPath must already be absolute and cleaned.
func IsSensitive(absPath string) bool {
	base := filepath.Base(absPath)

	if sensitiveFiles[base] {
		return true
	}
	if sensitiveExts[filepath.Ext(base)] {
		return true
	}
	// .env, .env.local, .env.production, … but not .env.example
	if strings.HasPrefix(base, ".env") && !dotEnvAllowed[base] {
		return true
	}
	for _, prefix := range sensitiveDirs() {
		if absPath == prefix || strings.HasPrefix(absPath, prefix+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ResolveRead resolves a user-supplied path for a read-style operation,
// enforcing the workdir boundary and the sensitive-path blocklist.
//
// It accepts both relative and absolute inputs, normalises symlinks if the
// target exists, and rejects anything that resolves outside workdir or onto a
// sensitive location.
func ResolveRead(input, workdir string) (string, error) {
	wd := canonical(workdir)
	abs := absUnder(input, wd)
	if IsSensitive(abs) {
		return "", ErrSensitivePath
	}
	// EvalSymlinks fails if the path doesn't exist; that's fine for a read.
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = real
	}
	if err := checkWithin(abs, wd); err != nil {
		return "", err
	}
	if IsSensitive(abs) {
		return "", ErrSensitivePath
	}
	return abs, nil
}

// ResolveWrite resolves a user-supplied path for a write-style operation.
// The file itself need not exist, but its parent directory chain (after
// symlink resolution) must lie inside workdir.
func ResolveWrite(input, workdir string) (string, error) {
	wd := canonical(workdir)
	abs := absUnder(input, wd)
	if IsSensitive(abs) {
		return "", ErrSensitivePath
	}
	// Resolve the longest existing prefix to catch symlinked parents that
	// escape the workdir.
	resolved := abs
	for cur := abs; ; {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			resolved = filepath.Join(real, strings.TrimPrefix(abs, cur))
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	if err := checkWithin(resolved, wd); err != nil {
		return "", err
	}
	if IsSensitive(resolved) {
		return "", ErrSensitivePath
	}
	return resolved, nil
}

// canonical resolves symlinks in workdir so boundary checks compare apples to
// apples on systems where temp/home dirs are accessed via a symlink (macOS
// /var → /private/var, common on CI runners with shared caches).
func canonical(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}

// absUnder converts input to an absolute, cleaned path; relative paths are
// taken to be relative to workdir.
func absUnder(input, workdir string) string {
	if !filepath.IsAbs(input) {
		input = filepath.Join(workdir, input)
	}
	return filepath.Clean(input)
}

// checkWithin returns ErrOutsideWorkdir unless target is workdir or a
// descendant of it. Both inputs must be absolute and cleaned.
func checkWithin(target, workdir string) error {
	workdir = filepath.Clean(workdir)
	if target == workdir {
		return nil
	}
	rel, err := filepath.Rel(workdir, target)
	if err != nil || rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, "..") {
		return ErrOutsideWorkdir
	}
	return nil
}
