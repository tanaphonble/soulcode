package security

import "regexp"

// dangerousCommandPatterns identify shell commands that should always require
// fresh approval, even if the user previously chose "always allow" for the
// session. The intent is to catch obviously destructive or exfiltration-style
// commands; it is NOT a comprehensive sandbox.
var dangerousCommandPatterns = []*regexp.Regexp{
	// rm -rf / and friends, including --no-preserve-root variants
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\b.*\s/(\s|$)`),
	regexp.MustCompile(`\brm\s+.*--no-preserve-root\b`),
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\s+~`),
	regexp.MustCompile(`\brm\s+(-[a-zA-Z]*r[a-zA-Z]*f|-[a-zA-Z]*f[a-zA-Z]*r)\s+\$HOME`),

	// curl|bash, wget|sh — classic remote-execution pipelines
	regexp.MustCompile(`\b(curl|wget|fetch)\b[^|]*\|\s*(ba)?sh\b`),
	regexp.MustCompile(`\b(curl|wget|fetch)\b[^|]*\|\s*python\b`),
	regexp.MustCompile(`\b(curl|wget|fetch)\b[^|]*\|\s*perl\b`),

	// dd writing to a block device
	regexp.MustCompile(`\bdd\b[^|]*\bof=/dev/(sd|nvme|hd|disk|mmcblk)`),

	// mkfs / format
	regexp.MustCompile(`\bmkfs(\.\w+)?\b`),

	// fork bomb (the classic, plus a few variations)
	regexp.MustCompile(`:\(\)\s*\{\s*:\|:&\s*\}\s*;\s*:`),

	// chmod / chown on root or home
	regexp.MustCompile(`\bchmod\s+-R?\s+\d+\s+/(\s|$)`),
	regexp.MustCompile(`\bchown\s+-R?\s+\S+\s+/(\s|$)`),

	// History wipes — typical post-exploit cleanup
	regexp.MustCompile(`\bhistory\s+-c\b`),
	regexp.MustCompile(`>\s*~/\.bash_history\b`),
	regexp.MustCompile(`>\s*~/\.zsh_history\b`),

	// sudo / su escalations
	regexp.MustCompile(`\bsudo\s+(rm|dd|mkfs|chmod|chown|cp|mv)\b`),

	// Disabling network or firewall
	regexp.MustCompile(`\biptables\s+-F\b`),
	regexp.MustCompile(`\bufw\s+disable\b`),

	// Writes to /etc that are typically destructive
	regexp.MustCompile(`>\s*/etc/(passwd|shadow|sudoers|hosts)\b`),

	// Git force push to remote refs (common foot-gun even if not malicious)
	regexp.MustCompile(`\bgit\s+push\s+(--force|-f)\b`),
	regexp.MustCompile(`\bgit\s+reset\s+--hard\b`),
}

// IsDangerousCommand reports whether cmd matches any pattern that should
// always be confirmed regardless of session-level "always allow".
func IsDangerousCommand(cmd string) bool {
	for _, re := range dangerousCommandPatterns {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}
