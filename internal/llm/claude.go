// Claude CLI wrapper for LLM inference.
// Uses the claude binary which handles OAuth authentication internally.
package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ClaudeClient wraps the Claude CLI for LLM inference.
type ClaudeClient struct {
	model      string
	binaryPath string
}

// NewClaudeClient creates a client that shells out to the claude CLI.
func NewClaudeClient(model string) (*ClaudeClient, error) {
	binaryPath, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found (install: npm install -g @anthropic-ai/claude-code)")
	}

	if model == "" {
		model = "sonnet"
	}

	return &ClaudeClient{
		model:      model,
		binaryPath: binaryPath,
	}, nil
}

// Complete sends a prompt to Claude CLI and returns the response.
func (c *ClaudeClient) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// Set timeout via context
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// Build command with text output
	cmd := exec.CommandContext(ctx, c.binaryPath,
		"-p", // Print mode (non-interactive)
		"--dangerously-skip-permissions",
		"--output-format", "text",
		"--model", c.model,
		"--max-turns", "1",
		"--", prompt,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("claude CLI failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("claude CLI error: %w", err)
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return "", fmt.Errorf("empty response from claude CLI")
	}

	return text, nil
}

// CompleteMessages implements multi-turn for compatibility.
func (c *ClaudeClient) CompleteMessages(ctx context.Context, messages []Message, maxTokens int) (string, error) {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return c.Complete(ctx, strings.Join(parts, "\n"), maxTokens)
}
