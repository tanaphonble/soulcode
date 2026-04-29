package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"soulcode/internal/provider"
)

func thinkTool() (provider.Tool, executeFn) {
	return provider.Tool{
		Name: "think",
		Description: "Use this tool to reason through a problem before taking action. " +
			"Think step-by-step about what to do, what files to check, and what the correct approach is. " +
			"This does not execute any code or read any files — it is purely for structured reasoning. " +
			"Use it before complex edits, when debugging, or when planning multi-file changes.",
		Schema: schema(`{
			"type": "object",
			"properties": {
				"thought": {
					"type": "string",
					"description": "Your step-by-step reasoning about the problem and the plan."
				}
			},
			"required": ["thought"]
		}`),
	}, runThink
}

func runThink(_ context.Context, input json.RawMessage, _ *SecurityContext) (string, error) {
	var args struct {
		Thought string `json:"thought"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", fmt.Errorf("think: %w", err)
	}
	return "thought recorded", nil
}
