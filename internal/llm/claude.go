// Claude CLI wrapper for LLM inference.
//
// This uses a unique integration approach: instead of calling the Anthropic API
// directly (which requires an ANTHROPIC_API_KEY), we shell out to the Claude Code
// CLI binary. The CLI handles OAuth authentication internally via the user's
// Anthropic account.
//
// Benefits:
//   - No API key management required
//   - Works with existing Claude Code / Max subscriptions
//   - Respects Anthropic's CLI-level rate limits
//
// The claude binary must be installed: npm install -g @anthropic-ai/claude-code
//
// Security Model:
//   - Prompts are NOT shell-escaped because they are passed as arguments to exec.Command,
//     not through a shell. Go's exec.Command uses execve() directly.
//   - Input validation rejects prompts with null bytes (which could truncate strings in C code).
//   - Output is captured from stdout which contains only the text response in text mode.
package llm

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Default timeout values
const (
	DefaultClaudeTimeout = 60 * time.Second
	DefaultOpusTimeout   = 90 * time.Second
)

// ClaudeClient wraps the Claude CLI for LLM inference.
type ClaudeClient struct {
	model      string
	binaryPath string
	maxTurns   int
	useOpus    bool
	timeout    time.Duration
	security   SecurityConfig
}

// ClaudeOption configures the Claude client.
type ClaudeOption func(*ClaudeClient)

// WithClaudeModel sets the model for Claude.
func WithClaudeModel(model string) ClaudeOption {
	return func(c *ClaudeClient) { c.model = model }
}

// WithMaxTurns sets the maximum number of turns (default 1).
// Higher values allow more complex multi-step reasoning.
func WithMaxTurns(turns int) ClaudeOption {
	return func(c *ClaudeClient) { c.maxTurns = turns }
}

// WithOpus enables Claude Opus model (claude-sonnet-4-20250514 -> opus).
func WithOpus(enabled bool) ClaudeOption {
	return func(c *ClaudeClient) { c.useOpus = enabled }
}

// WithTimeout sets the timeout for Claude CLI calls.
// Default is 60s for Sonnet, 90s for Opus.
func WithTimeout(d time.Duration) ClaudeOption {
	return func(c *ClaudeClient) { c.timeout = d }
}

// WithClaudeSecurityConfig sets the security configuration for sandbox and limits.
func WithClaudeSecurityConfig(cfg SecurityConfig) ClaudeOption {
	return func(c *ClaudeClient) { c.security = cfg }
}

// NewClaudeClient creates a client that shells out to the claude CLI.
// Deprecated: Use NewClaudeClientWithOptions for new code.
func NewClaudeClient(model string) (*ClaudeClient, error) {
	opts := []ClaudeOption{}
	if model != "" {
		opts = append(opts, WithClaudeModel(model))
	}
	return NewClaudeClientWithOptions(opts...)
}

// NewClaudeClientWithOptions creates a client with functional options.
func NewClaudeClientWithOptions(opts ...ClaudeOption) (*ClaudeClient, error) {
	binaryPath, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found (install: npm install -g @anthropic-ai/claude-code)")
	}

	c := &ClaudeClient{
		model:      "sonnet", // Default to Sonnet
		binaryPath: binaryPath,
		maxTurns:   1, // Default single turn
		useOpus:    false,
		timeout:    DefaultClaudeTimeout,    // Default 60s for Sonnet
		security:   DefaultSecurityConfig(), // Safe defaults
	}

	for _, opt := range opts {
		opt(c)
	}

	// Apply Opus override if enabled and no explicit model was set
	if c.useOpus && c.model == "sonnet" {
		c.model = "opus"
		// Use longer timeout for Opus if not explicitly set
		if c.timeout == DefaultClaudeTimeout {
			c.timeout = DefaultOpusTimeout
		}
	}

	return c, nil
}

// Complete sends a prompt to Claude CLI and returns the response.
func (c *ClaudeClient) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	// Sanitize and validate input before passing to CLI using client's security config
	sanitizedPrompt, err := SanitizePromptWithConfig(prompt, c.security)
	if err != nil {
		return "", fmt.Errorf("invalid prompt: %w", err)
	}

	// Set timeout via context
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Build command arguments with appropriate sandbox mode.
	// See internal/llm/security.go for threat model documentation.
	args := []string{
		"-p", // Print mode (non-interactive)
		"--output-format", "text",
		"--model", c.model,
		"--max-turns", strconv.Itoa(c.maxTurns),
	}

	// Apply sandbox mode based on security config.
	// Claude Code uses --dangerously-skip-permissions for full access,
	// otherwise we rely on its built-in permission system.
	switch c.security.SandboxMode {
	case SandboxFullAccess:
		// DANGEROUS: Skip all permission checks.
		// Only use when user explicitly requests --unsafe.
		args = append(args, "--dangerously-skip-permissions")
	case SandboxWorkspace, SandboxReadOnly:
		// Claude Code doesn't have granular sandbox flags like Codex,
		// but without --dangerously-skip-permissions it will prompt
		// for permission before dangerous operations. In non-interactive
		// mode (-p), this means it will fail safely instead of executing.
		// This is the desired behavior for read-only/workspace modes.
	}

	args = append(args, "--", sanitizedPrompt)
	cmd := exec.CommandContext(ctx, c.binaryPath, args...)

	output, err := cmd.Output()
	if err != nil {
		return "", c.handleError(err, ctx)
	}

	text := strings.TrimSpace(string(output))
	if text == "" {
		return "", fmt.Errorf("empty response from claude CLI")
	}

	return text, nil
}

// handleError converts CLI errors into user-friendly messages.
func (c *ClaudeClient) handleError(err error, ctx context.Context) error {
	// Check for context timeout/cancellation
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("claude CLI timed out after %d seconds", int(c.timeout.Seconds()))
	}
	if ctx.Err() == context.Canceled {
		return fmt.Errorf("claude CLI request was cancelled")
	}

	// Check for exit error with stderr
	if exitErr, ok := err.(*exec.ExitError); ok {
		stderr := strings.ToLower(string(exitErr.Stderr))

		// Check for authentication issues
		if strings.Contains(stderr, "not authenticated") ||
			strings.Contains(stderr, "login") ||
			strings.Contains(stderr, "unauthorized") ||
			strings.Contains(stderr, "auth") ||
			exitErr.ExitCode() == 1 && strings.Contains(stderr, "account") {
			return fmt.Errorf("claude CLI not authenticated - run 'claude login'")
		}

		// Check for rate limiting
		if strings.Contains(stderr, "rate limit") || strings.Contains(stderr, "too many requests") {
			return fmt.Errorf("claude CLI rate limited - please wait and retry")
		}

		// Generic exit error with details
		return fmt.Errorf("claude CLI failed (exit %d): %s", exitErr.ExitCode(), string(exitErr.Stderr))
	}

	return fmt.Errorf("claude CLI error: %w", err)
}

// CompleteMessages implements multi-turn for compatibility.
func (c *ClaudeClient) CompleteMessages(ctx context.Context, messages []Message, maxTokens int) (string, error) {
	var parts []string
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return c.Complete(ctx, strings.Join(parts, "\n"), maxTokens)
}
