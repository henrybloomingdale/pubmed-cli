// Security configuration for LLM CLI integrations.
//
// Threat Model:
//
// These LLM CLI tools (Claude Code, OpenAI Codex) can execute arbitrary code
// on the host system. While they have their own permission systems, we add an
// additional layer of defense-in-depth by configuring sandbox modes.
//
// Risks mitigated:
//   - Prompt injection: Malicious content in PubMed abstracts could attempt
//     to manipulate the LLM into executing harmful commands.
//   - Model hallucination: The LLM might incorrectly decide to modify files
//     or execute commands based on misunderstood instructions.
//   - Supply chain: Compromised LLM binaries could attempt unauthorized access.
//
// Defense layers:
//  1. Sandbox mode restricts filesystem and command access at the CLI level
//  2. Input validation rejects obviously malicious prompts (see sanitize.go)
//  3. Prompt length limits prevent context overflow attacks
//  4. Network restrictions can limit exfiltration vectors
//
// Recommended usage:
//   - QA tasks: SandboxReadOnly (safe for answering questions)
//   - Synth tasks: SandboxReadOnly by default, allow workspace writes if needed
//   - Full access: Only with explicit --unsafe flag and user warning
package llm

// SandboxMode controls what the LLM CLI can do on the system.
// Maps to Codex --sandbox flag and Claude permission modes.
type SandboxMode string

const (
	// SandboxReadOnly prevents file writes and destructive commands.
	// This is the safest mode - the LLM can only read and respond.
	// Use for: QA, analysis, summarization tasks.
	SandboxReadOnly SandboxMode = "read-only"

	// SandboxWorkspace allows writes only within the current workspace.
	// Safer than full access but allows file creation/modification.
	// Use for: Report generation, export tasks.
	SandboxWorkspace SandboxMode = "workspace-write"

	// SandboxFullAccess bypasses all sandbox restrictions.
	// DANGEROUS: The LLM can execute any command and modify any file.
	// Only use when explicitly requested with --unsafe flag.
	SandboxFullAccess SandboxMode = "danger-full-access"
)

// IsValid returns true if the sandbox mode is recognized.
func (m SandboxMode) IsValid() bool {
	switch m {
	case SandboxReadOnly, SandboxWorkspace, SandboxFullAccess:
		return true
	default:
		return false
	}
}

// IsDangerous returns true if the mode allows potentially destructive operations.
func (m SandboxMode) IsDangerous() bool {
	return m == SandboxFullAccess
}

// String returns the mode value for CLI flags.
func (m SandboxMode) String() string {
	return string(m)
}

// SecurityConfig holds security settings for LLM CLI integrations.
type SecurityConfig struct {
	// SandboxMode controls filesystem and command restrictions.
	SandboxMode SandboxMode

	// AllowNetworkCalls permits the LLM to make network requests.
	// Currently informational; actual enforcement depends on CLI capabilities.
	AllowNetworkCalls bool

	// MaxPromptLength limits prompt size to prevent context overflow attacks.
	// Prompts exceeding this length will be rejected.
	MaxPromptLength int

	// AllowToolUse permits the LLM to use tools/functions.
	// When false, the LLM is restricted to text-only responses.
	AllowToolUse bool

	// AllowedDomains restricts URL references to specific domains.
	// Empty slice means no domain restrictions.
	AllowedDomains []string

	// AllowShellMetachars permits shell metacharacters in prompts.
	// Even when true, they won't be interpreted (exec.Command bypasses shell).
	AllowShellMetachars bool

	// BlockPromptInjection enables detection of prompt injection patterns.
	BlockPromptInjection bool
}

// DefaultSecurityConfig returns a safe default configuration.
// Defaults to read-only sandbox with reasonable limits.
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		SandboxMode:          SandboxReadOnly,
		AllowNetworkCalls:    true,
		MaxPromptLength:      100 * 1024, // 100KB
		AllowToolUse:         false,      // Text responses only by default
		AllowedDomains:       nil,        // No domain restrictions
		AllowShellMetachars:  false,      // Block shell chars by default
		BlockPromptInjection: true,       // Enable injection detection
	}
}

// ForQA returns a security config optimized for question-answering tasks.
// Uses the most restrictive settings since QA only needs to read and respond.
func ForQA() SecurityConfig {
	return SecurityConfig{
		SandboxMode:         SandboxReadOnly,
		AllowNetworkCalls:   true,
		MaxPromptLength:     50 * 1024, // 50KB is plenty for QA
		AllowToolUse:        false,
		AllowedDomains:      nil,
		AllowShellMetachars: true, // PubMed abstracts contain $, &, | in scientific notation
	}
}

// ForSynthesis returns a security config for literature synthesis.
// Read-only by default but with higher prompt limits for context.
func ForSynthesis() SecurityConfig {
	return SecurityConfig{
		SandboxMode:         SandboxReadOnly,
		AllowNetworkCalls:   true,
		MaxPromptLength:     200 * 1024, // Synthesis needs more context
		AllowToolUse:        false,
		AllowedDomains:      nil,
		AllowShellMetachars: true, // PubMed abstracts contain $, &, | in scientific notation
	}
}

// WithFullAccess returns a copy with full access enabled.
// This should only be used when the user explicitly requests --unsafe.
func (c SecurityConfig) WithFullAccess() SecurityConfig {
	c.SandboxMode = SandboxFullAccess
	c.AllowToolUse = true
	return c
}

// WithWorkspaceWrite returns a copy with workspace write access.
func (c SecurityConfig) WithWorkspaceWrite() SecurityConfig {
	c.SandboxMode = SandboxWorkspace
	return c
}

// PermissiveSecurityConfig returns a less restrictive configuration.
// Useful for trusted inputs or testing. Still uses read-only sandbox.
func PermissiveSecurityConfig() SecurityConfig {
	return SecurityConfig{
		SandboxMode:          SandboxReadOnly,
		AllowNetworkCalls:    true,
		MaxPromptLength:      1024 * 1024, // 1MB - very permissive
		AllowToolUse:         false,
		AllowedDomains:       nil,
		AllowShellMetachars:  true,  // Allow shell chars
		BlockPromptInjection: false, // Disable injection blocking
	}
}

// WithAllowedDomains returns a copy with allowed URL domains for prompts.
func (c SecurityConfig) WithAllowedDomains(domains []string) SecurityConfig {
	c.AllowedDomains = domains
	return c
}
