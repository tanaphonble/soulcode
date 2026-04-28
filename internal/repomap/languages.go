package repomap

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// pattern defines how to extract symbols from a source file via regex.
type pattern struct {
	re     *regexp.Regexp
	format string // printf format; %s is replaced by the first capture group
}

// regexEntry extracts symbols from non-Go files using language-specific patterns.
func regexEntry(path, rel string, patterns []pattern) string {
	data, err := os.ReadFile(path) //nolint:gosec // path comes from WalkDir, not user input
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")

	var symbols []string
	for _, line := range lines {
		for _, p := range patterns {
			if m := p.re.FindStringSubmatch(line); m != nil {
				symbols = append(symbols, fmt.Sprintf("  "+p.format, m[1]))
				break
			}
		}
	}
	if len(symbols) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s\n", rel)
	for _, s := range symbols {
		sb.WriteString(s)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── Language patterns ────────────────────────────────────────────────────────

var pyPattern = []pattern{
	{regexp.MustCompile(`^class\s+(\w+)`), "class %s"},
	{regexp.MustCompile(`^def\s+(\w+)`), "def %s"},
	{regexp.MustCompile(`^    def\s+(\w+)`), "  def %s"},
	{regexp.MustCompile(`^(\w+)\s*=`), "var %s"},
}

var tsPattern = []pattern{
	{regexp.MustCompile(`^export\s+(?:default\s+)?(?:abstract\s+)?class\s+(\w+)`), "class %s"},
	{regexp.MustCompile(`^export\s+interface\s+(\w+)`), "interface %s"},
	{regexp.MustCompile(`^export\s+type\s+(\w+)`), "type %s"},
	{regexp.MustCompile(`^export\s+(?:async\s+)?function\s+(\w+)`), "function %s"},
	{regexp.MustCompile(`^export\s+const\s+(\w+)`), "const %s"},
	{regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+)?(?:async\s+)?(\w+)\s*\(`), "  method %s"},
}

var jsPattern = []pattern{
	{regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`), "function %s"},
	{regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`), "class %s"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)\s*=\s*(?:async\s+)?\(`), "const %s (fn)"},
	{regexp.MustCompile(`^(?:export\s+)?const\s+(\w+)`), "const %s"},
	{regexp.MustCompile(`^module\.exports\s*=`), "module.exports"},
}

var rustPattern = []pattern{
	{regexp.MustCompile(`^pub\s+(?:async\s+)?fn\s+(\w+)`), "pub fn %s"},
	{regexp.MustCompile(`^pub\s+struct\s+(\w+)`), "pub struct %s"},
	{regexp.MustCompile(`^pub\s+trait\s+(\w+)`), "pub trait %s"},
	{regexp.MustCompile(`^pub\s+enum\s+(\w+)`), "pub enum %s"},
	{regexp.MustCompile(`^impl(?:\s+\w+\s+for)?\s+(\w+)`), "impl %s"},
}
