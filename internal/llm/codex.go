// Codex CLI wrapper for LLM inference.
//
// Similar to Claude integration: shells out to the OpenAI Codex CLI binary.
// The CLI handles OAuth authentication internally via the user's ChatGPT account.
//
// Benefits:
//   - No API key management required
//   - Works with existing ChatGPT Plus/Pro/Team/Enterprise subscriptions
//   - Respects OpenAI's CLI-level rate limits
//
// The codex binary must be installed: npm install -g @openai/codex
//
// Security Model:
//   - Prompts are NOT shell-escaped because they are passed as arguments to exec.Command,
//     not through a shell. Go's exec.Command uses execve() directly.
//   - Input validation rejects prompts with null bytes (which could truncate strings in C code).
//   - The --sandbox read-only flag prevents file system modifications.
//   - Output is read from a temp file, not parsed from stdout, to avoid injection via model output.
package llm

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// CodexClient wraps the Codex CLI for LLM inference.
type CodexClient struct {
	model           string
	binaryPath      string
	reasoningEffort string
	security        SecurityConfig
}

// CodexOption configures the Codex client.
type CodexOption func(*CodexClient)

// WithCodexModel sets the model for Codex.
func WithCodexModel(model string) CodexOption {
	return func(c *CodexClient) { c.model = model }
}

// WithReasoningEffort sets the reasoning effort level (low, medium, high).
func WithReasoningEffort(effort string) CodexOption {
	return func(c *CodexClient) { c.reasoningEffort = effort }
}

// WithSecurityConfig sets the security configuration for sandbox and limits.
func WithSecurityConfig(cfg SecurityConfig) CodexOption {
	return func(c *CodexClient) { c.security = cfg }
}

// NewCodexClient creates a client that shells out to the codex CLI.
func NewCodexClient(opts ...CodexOption) (*CodexClient, error) {
	binaryPath, err := exec.LookPath("codex")
	if err != nil {
		return nil, fmt.Errorf("codex CLI not found (install: npm install -g @openai/codex)")
	}

	c := &CodexClient{
		model:           "gpt-5.3-codex", // Default Codex model
		binaryPath:      binaryPath,
		reasoningEffort: "medium",                // Balanced reasoning by default
		security:        DefaultSecurityConfig(), // Safe defaults
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// NOTE: Input validation is handled by SanitizePrompt in security.go.
// The security model includes:
//   - Null byte rejection
//   - Prompt length limits (configurable via SecurityConfig)
//   - Sandbox mode enforcement via CLI flags

// Complete sends a prompt to Codex CLI and returns the response.
func (c *CodexClient) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// Sanitize and validate input before passing to CLI using client's security config
	sanitizedPrompt, err := SanitizePromptWithConfig(prompt, c.security)
	if err != nil {
		return "", fmt.Errorf("invalid prompt: %w", err)
	}
	prompt = sanitizedPrompt

	// Set timeout via context - Codex can be slower than Claude
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	// Create temp file for output (cleaner than parsing stdout which contains metadata)
	tmpFile, err := os.CreateTemp("", "codex-response-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Build command arguments with appropriate sandbox mode.
	// See internal/llm/security.go for threat model documentation.
	args := []string{
		"exec",
		"--skip-git-repo-check", // pubmed-cli may run outside git repos
		"--color", "never",      // Disable ANSI colors for clean output
		"--model", c.model,
		"-c", fmt.Sprintf("model_reasoning_effort=%q", c.reasoningEffort),
		"-o", tmpPath, // Write response to temp file
	}

	// Apply sandbox mode based on security config.
	// Only bypass sandbox for dangerous full-access mode.
	switch c.security.SandboxMode {
	case SandboxFullAccess:
		// DANGEROUS: Full access bypasses all sandbox restrictions.
		// Only use when user explicitly requests --unsafe.
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	case SandboxWorkspace:
		// Allow writes within workspace only.
		args = append(args, "--sandbox", "workspace-write")
	case SandboxReadOnly:
		fallthrough
	default:
		// Safe default: read-only sandbox prevents file modifications.
		args = append(args, "--sandbox", "read-only")
	}

	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("codex CLI failed (exit %d): %s",
				exitErr.ExitCode(), string(output))
		}
		return "", fmt.Errorf("codex CLI error: %w", err)
	}

	// Read response from temp file
	response, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	text := strings.TrimSpace(string(response))
	if text == "" {
		return "", fmt.Errorf("empty response from codex CLI")
	}

	return text, nil
}

// CompleteMessages implements multi-turn for compatibility.
func (c *CodexClient) CompleteMessages(ctx context.Context, messages []Message, maxTokens int) (string, error) {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return c.Complete(ctx, strings.Join(parts, "\n"), maxTokens)
}
