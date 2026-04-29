package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/tanaphonble/soulcode/internal/provider"
	"github.com/tanaphonble/soulcode/internal/security"
)

const (
	bashTimeout = 30 * time.Second
	maxOutput   = 8 * 1024 // 8 KB — enough for errors; LLM can grep for more
)

// errBashDenied is returned when the user (or the approver) refuses a bash
// command. It is surfaced as a tool result so the LLM sees the rejection and
// can adjust its plan.
var errBashDenied = errors.New("user denied execution")

func bashTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "bash",
		Description: "Execute a shell command in the working directory and return its combined stdout and stderr. The user must approve the command unless they have pre-approved this exact prefix or run with SOULCODE_YOLO. Avoid commands that run indefinitely.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The shell command to run."
				}
			},
			"required": ["command"]
		}`),
	}, runBash
}

func runBash(ctx context.Context, input json.RawMessage, sec *SecurityContext) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("bash: invalid input: %w", err)
	}
	if args.Command == "" {
		return "", fmt.Errorf("bash: command is required")
	}

	dangerous := security.IsDangerousCommand(args.Command)
	approval := decideBashApproval(ctx, args.Command, dangerous, sec)
	if !approval.allowed {
		auditBash(sec, args.Command, approval.label, false, errBashDenied, "")
		return "", fmt.Errorf("bash: %w (command: %s)", errBashDenied, truncateOneLine(args.Command, 120))
	}

	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command) //nolint:gosec // bash tool intentionally runs user-approved commands
	if sec != nil && sec.Workdir != "" {
		cmd.Dir = sec.Workdir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()

	result := out.String()
	if len(result) > maxOutput {
		result = result[:maxOutput] + fmt.Sprintf("\n... (truncated, %d bytes total)", len(result))
	}

	if err != nil {
		exitCode := cmd.ProcessState.ExitCode()
		prefix := fmt.Sprintf("exit status %d", exitCode)
		if exitCode == -1 {
			prefix = fmt.Sprintf("error: command timed out after %s", bashTimeout)
		}
		if result != "" {
			result = fmt.Sprintf("%s\n%s", prefix, result)
		} else {
			result = prefix
		}
		auditBash(sec, args.Command, approval.label, false, err, result)
		return result, nil
	}

	auditBash(sec, args.Command, approval.label, true, nil, result)
	return result, nil
}

type bashApproval struct {
	allowed bool
	label   string // for audit log
}

func decideBashApproval(ctx context.Context, cmd string, dangerous bool, sec *SecurityContext) bashApproval {
	if sec == nil {
		return bashApproval{allowed: true, label: "no-security-context"}
	}
	// Project allow-list never overrides dangerous-pattern detection.
	if !dangerous && sec.AllowList != nil && sec.AllowList.Allows(cmd) {
		return bashApproval{allowed: true, label: "allow-list"}
	}
	if sec.Approver == nil {
		// No approver wired: default to allow for tests; main always wires one.
		return bashApproval{allowed: true, label: "no-approver"}
	}
	switch sec.Approver.Approve(ctx, "bash", cmd, dangerous) {
	case security.DecisionDeny:
		return bashApproval{allowed: false, label: "denied"}
	case security.DecisionAllowAlways:
		return bashApproval{allowed: true, label: "user-always"}
	default:
		return bashApproval{allowed: true, label: "user-once"}
	}
}

func auditBash(sec *SecurityContext, cmd, label string, ok bool, runErr error, result string) {
	if sec == nil || sec.Auditor == nil {
		return
	}
	entry := security.AuditEntry{
		Tool:       "bash",
		Args:       truncateOneLine(cmd, 200),
		Approval:   label,
		OK:         ok,
		ResultHash: security.HashResult(result),
	}
	if runErr != nil {
		entry.Err = runErr.Error()
	}
	sec.Auditor.Log(entry)
}

func truncateOneLine(s string, n int) string {
	for i, r := range s {
		if r == '\n' {
			s = s[:i] + " "
			break
		}
		_ = i
	}
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
