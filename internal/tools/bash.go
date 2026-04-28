package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"soulcode/internal/provider"
)

const (
	bashTimeout = 30 * time.Second
	maxOutput   = 32 * 1024 // 32 KB
)

func bashTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name:        "bash",
		Description: "Execute a shell command and return its combined stdout and stderr. Avoid commands that run indefinitely.",
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

func runBash(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("bash: invalid input: %w", err)
	}
	if args.Command == "" {
		return "", fmt.Errorf("bash: command is required")
	}

	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", args.Command) //nolint:gosec // intentional: bash tool executes user-provided commands
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
			return fmt.Sprintf("%s\n%s", prefix, result), nil
		}
		return prefix, nil
	}
	return result, nil
}
