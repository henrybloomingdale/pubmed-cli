package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/henrybloomingdale/pubmed-cli/internal/llm"
	"github.com/henrybloomingdale/pubmed-cli/internal/synth"
	"github.com/spf13/cobra"
)

// WizardConfig holds user preferences.
type WizardConfig struct {
	DefaultPapers    int    `json:"default_papers"`
	DefaultWords     int    `json:"default_words"`
	DefaultRelevance int    `json:"default_relevance"`
	OutputFolder     string `json:"output_folder"`
	PreferDocx       bool   `json:"prefer_docx"`
	PreferRIS        bool   `json:"prefer_ris"`
	LLMModel         string `json:"llm_model,omitempty"`
	UseClaude        bool   `json:"use_claude"`
}

// wizardInputs holds collected user inputs from the wizard form.
type wizardInputs struct {
	Question     string
	Papers       int
	Words        int
	OutputFormat string
	OutputName   string
}

// DefaultWizardConfig returns sensible defaults.
func DefaultWizardConfig() WizardConfig {
	return WizardConfig{
		DefaultPapers:    5,
		DefaultWords:     250,
		DefaultRelevance: 7,
		OutputFolder:     getDefaultOutputFolder(),
		PreferDocx:       true,
		PreferRIS:        true,
		UseClaude:        true, // Default to Claude (handles auth via CLI OAuth)
	}
}

func init() {
	rootCmd.AddCommand(wizardCmd)
}

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Interactive literature synthesis wizard",
	Long: `Beautiful interactive wizard for literature synthesis.

Walk through the process step-by-step with sensible defaults.
Creates Word documents and RIS files for your reference manager.

Run without arguments to start the wizard:
  pubmed wizard`,
	RunE: runWizard,
}

// Styles.
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("99")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Italic(true)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("42"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("99")).
			Padding(1, 2).
			MarginTop(1)
)

func runWizard(cmd *cobra.Command, args []string) error {
	// Load config.
	cfg := loadWizardConfig()
	if strings.TrimSpace(cfg.OutputFolder) == "" {
		cfg.OutputFolder = getDefaultOutputFolder()
	}

	// Show welcome.
	printWizardWelcome()

	// Collect user inputs via interactive form.
	inputs, cancelled, err := collectWizardInputs(&cfg)
	if err != nil {
		return err
	}
	if cancelled {
		fmt.Println(dimStyle.Render("\nCancelled."))
		return nil
	}

	// Ensure output folder exists.
	if err := os.MkdirAll(cfg.OutputFolder, 0o755); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}

	// Execute synthesis.
	result, err := executeWizardSynthesis(cmd.Context(), &cfg, inputs)
	if err != nil {
		return err
	}

	// Handle output based on format.
	return handleWizardOutput(cmd.Context(), result, inputs, &cfg)
}

// printWizardWelcome clears the screen and displays the welcome banner.
func printWizardWelcome() {
	fmt.Print("\033[H\033[2J")
	fmt.Println(titleStyle.Render("🔬 PubMed Literature Synthesis"))
	fmt.Println(subtitleStyle.Render("Create a research synthesis with citations in seconds"))
	fmt.Println()
}

// collectWizardInputs displays the interactive form and collects user inputs.
// Returns the inputs, a cancelled flag, and any error.
func collectWizardInputs(cfg *WizardConfig) (*wizardInputs, bool, error) {
	var (
		question     string
		papersStr    string
		wordsStr     string
		outputFormat string
		outputName   string
		confirm      bool
	)
	outputFormat = defaultOutputFormat(*cfg)

	// Build the form.
	form := huh.NewForm(
		// Page 1: Research Question
		huh.NewGroup(
			huh.NewText().
				Title("What's your research question?").
				Description("Enter a topic or question to synthesize literature on").
				Placeholder("e.g., SGLT-2 inhibitors in liver fibrosis").
				Value(&question).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("please enter a research question")
					}
					return nil
				}),
		).Title("Research Question"),

		// Page 2: Settings
		huh.NewGroup(
			huh.NewInput().
				Title("Number of papers to include").
				Description("More papers = broader synthesis, but slower").
				Placeholder(fmt.Sprintf("%d", cfg.DefaultPapers)).
				Value(&papersStr).
				Validate(validatePositiveInt),

			huh.NewInput().
				Title("Target word count").
				Description("Approximate length of synthesis").
				Placeholder(fmt.Sprintf("%d", cfg.DefaultWords)).
				Value(&wordsStr).
				Validate(validatePositiveInt),
		).Title("Synthesis Settings"),

		// Page 3: Output
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Output format").
				Options(
					huh.NewOption("Word document (.docx) + References (.ris)", "docx+ris"),
					huh.NewOption("Word document only (.docx)", "docx"),
					huh.NewOption("Markdown (terminal)", "markdown"),
					huh.NewOption("JSON (for pipelines)", "json"),
				).
				Value(&outputFormat),

			huh.NewInput().
				Title("Output filename (without extension)").
				Description(fmt.Sprintf("Files saved to: %s", cfg.OutputFolder)).
				Placeholder("synthesis").
				Value(&outputName).
				Validate(validateOutputName),
		).Title("Output Options"),

		// Page 4: Confirm
		huh.NewGroup(
			huh.NewConfirm().
				Title("Ready to synthesize?").
				Description("This will search PubMed, score papers for relevance, and generate your synthesis.").
				Affirmative("Let's go!").
				Negative("Cancel").
				Value(&confirm),
		).Title("Confirm"),
	).WithTheme(huh.ThemeCatppuccin())

	if err := form.Run(); err != nil {
		return nil, false, err
	}
	if !confirm {
		return nil, true, nil
	}

	// Parse form values into inputs struct.
	return parseWizardFormValues(cfg, question, papersStr, wordsStr, outputFormat, outputName)
}

// parseWizardFormValues converts raw form strings into validated wizardInputs.
func parseWizardFormValues(cfg *WizardConfig, question, papersStr, wordsStr, outputFormat, outputName string) (*wizardInputs, bool, error) {
	inputs := &wizardInputs{
		Question:     question,
		Papers:       cfg.DefaultPapers,
		Words:        cfg.DefaultWords,
		OutputFormat: outputFormat,
	}

	// Parse papers count.
	if papersStr = strings.TrimSpace(papersStr); papersStr != "" {
		p, err := strconv.Atoi(papersStr)
		if err != nil {
			return nil, false, fmt.Errorf("parse papers: %w", err)
		}
		inputs.Papers = p
	}

	// Parse word count.
	if wordsStr = strings.TrimSpace(wordsStr); wordsStr != "" {
		w, err := strconv.Atoi(wordsStr)
		if err != nil {
			return nil, false, fmt.Errorf("parse words: %w", err)
		}
		inputs.Words = w
	}

	// Parse output name.
	if strings.TrimSpace(outputName) == "" {
		outputName = "synthesis"
	}
	name, err := sanitizeOutputName(outputName)
	if err != nil {
		return nil, false, err
	}
	inputs.OutputName = name

	return inputs, false, nil
}

// executeWizardSynthesis builds the LLM client and runs the synthesis engine.
func executeWizardSynthesis(ctx context.Context, cfg *WizardConfig, inputs *wizardInputs) (*synth.Result, error) {
	fmt.Println()

	// Build LLM client.
	llmClient, err := buildLLMClient(cfg)
	if err != nil {
		return nil, err
	}

	// Build synth config.
	synthCfg := synth.DefaultConfig()
	synthCfg.PapersToUse = inputs.Papers
	synthCfg.TargetWords = inputs.Words
	synthCfg.RelevanceThreshold = cfg.DefaultRelevance

	engine := synth.NewEngine(llmClient, newEutilsClient(), synthCfg)

	// Run synthesis with spinner.
	var (
		result   *synth.Result
		synthErr error
	)
	action := func() {
		result, synthErr = engine.Synthesize(ctx, inputs.Question)
	}

	spinErr := spinner.New().
		Title("Synthesizing literature...").
		Action(action).
		Run()

	if spinErr != nil {
		return nil, spinErr
	}
	if synthErr != nil {
		return nil, synthErr
	}
	if result == nil {
		return nil, errors.New("synthesis returned nil result")
	}

	return result, nil
}

// buildLLMClient creates the appropriate LLM client based on config.
func buildLLMClient(cfg *WizardConfig) (synth.LLMClient, error) {
	if cfg.UseClaude {
		client, err := llm.NewClaudeClient(cfg.LLMModel)
		if err != nil {
			return nil, fmt.Errorf("claude setup: %w", err)
		}
		return client, nil
	}

	var opts []llm.Option
	if cfg.LLMModel != "" {
		opts = append(opts, llm.WithModel(cfg.LLMModel))
	}
	return llm.NewClient(opts...), nil
}

// handleWizardOutput saves outputs based on the selected format and prints success.
func handleWizardOutput(ctx context.Context, result *synth.Result, inputs *wizardInputs, cfg *WizardConfig) error {
	// Build file paths.
	docxPath := filepath.Join(cfg.OutputFolder, inputs.OutputName+".docx")
	risPath := filepath.Join(cfg.OutputFolder, inputs.OutputName+".ris")
	mdPath := filepath.Join(cfg.OutputFolder, inputs.OutputName+".md")

	// Handle format-specific output.
	switch inputs.OutputFormat {
	case "markdown":
		printMarkdownResult(result)
		return nil

	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	case "docx", "docx+ris":
		savedFiles, err := saveDocxOutput(ctx, result, mdPath, docxPath, risPath, inputs.OutputFormat)
		if err != nil {
			return err
		}
		printWizardSuccess(result, savedFiles)
		return nil

	default:
		return fmt.Errorf("unknown output format: %q", inputs.OutputFormat)
	}
}

// saveDocxOutput handles saving docx and optionally ris files.
func saveDocxOutput(ctx context.Context, result *synth.Result, mdPath, docxPath, risPath, format string) ([]string, error) {
	var savedFiles []string

	// Save markdown first (needed for pandoc conversion).
	if err := saveMarkdownFile(mdPath, result); err != nil {
		return nil, err
	}

	// Convert to docx.
	if err := convertToDocxContext(ctx, mdPath, docxPath); err != nil {
		fmt.Fprintln(os.Stderr, dimStyle.Render(fmt.Sprintf("Pandoc conversion failed (%v). Keeping markdown output.", err)))
		savedFiles = append(savedFiles, mdPath)
	} else {
		_ = os.Remove(mdPath) // best-effort cleanup
		savedFiles = append(savedFiles, docxPath)
	}

	// Save RIS if requested.
	if format == "docx+ris" {
		if err := os.WriteFile(risPath, []byte(result.RIS), 0o644); err != nil {
			return nil, fmt.Errorf("write RIS: %w", err)
		}
		savedFiles = append(savedFiles, risPath)
	}

	return savedFiles, nil
}

// printWizardSuccess displays the success message and summary.
func printWizardSuccess(result *synth.Result, savedFiles []string) {
	fmt.Println()
	fmt.Println(successStyle.Render("✓ Synthesis complete!"))
	fmt.Println()

	summary := fmt.Sprintf(`📊 %d papers searched → %d scored → %d used
📝 %d words generated
💰 ~%d tokens used`,
		result.PapersSearched,
		result.PapersScored,
		result.PapersUsed,
		countWords(result.Synthesis),
		result.Tokens.Total)

	fmt.Println(boxStyle.Render(summary))
	fmt.Println()

	fmt.Println(dimStyle.Render("Files created:"))
	for _, f := range savedFiles {
		fmt.Printf("  📄 %s\n", f)
	}
	fmt.Println()

	snippet := result.Synthesis
	if r := []rune(snippet); len(r) > 300 {
		snippet = string(r[:300]) + "..."
	}
	fmt.Println(dimStyle.Render("Preview:"))
	fmt.Println(boxStyle.Render(snippet))
}

func defaultOutputFormat(cfg WizardConfig) string {
	if cfg.PreferDocx {
		if cfg.PreferRIS {
			return "docx+ris"
		}
		return "docx"
	}
	return "markdown"
}

func validatePositiveInt(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil // Allow empty for defaults.
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("please enter a whole number")
	}
	if n < 1 {
		return fmt.Errorf("must be at least 1")
	}
	return nil
}

func validateOutputName(s string) error {
	if strings.TrimSpace(s) == "" {
		return nil // default will be used
	}
	// We intentionally disallow path separators so the wizard can't write outside OutputFolder.
	if strings.ContainsAny(s, "/\\") {
		return fmt.Errorf("filename must not contain path separators")
	}
	return nil
}

func sanitizeOutputName(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("output name is required")
	}
	if strings.ContainsAny(s, "/\\") {
		return "", fmt.Errorf("filename must not contain path separators")
	}
	// Strip a user-provided extension, if any.
	if ext := filepath.Ext(s); ext != "" {
		s = strings.TrimSuffix(s, ext)
	}
	if s == "" {
		return "", fmt.Errorf("output name is required")
	}
	return s, nil
}

func saveMarkdownFile(path string, result *synth.Result) error {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", result.Question))
	sb.WriteString(result.Synthesis)
	sb.WriteString("\n\n## References\n\n")
	for i, ref := range result.References {
		sb.WriteString(fmt.Sprintf("%d. %s\n\n", i+1, ref.CitationAPA))
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func convertToDocxContext(ctx context.Context, mdPath, docxPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	pandocPath, err := exec.LookPath("pandoc")
	if err != nil {
		// Check common locations.
		for _, p := range []string{"/opt/homebrew/bin/pandoc", "/usr/local/bin/pandoc", "/usr/bin/pandoc"} {
			if _, err := os.Stat(p); err == nil {
				pandocPath = p
				break
			}
		}
	}
	if pandocPath == "" {
		return fmt.Errorf("pandoc not found - saved as markdown instead")
	}

	cmd := exec.CommandContext(ctx, pandocPath, mdPath, "-o", docxPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return fmt.Errorf("pandoc: %w", err)
		}
		return fmt.Errorf("pandoc: %w: %s", err, msg)
	}
	return nil
}

func printMarkdownResult(result *synth.Result) {
	fmt.Println()
	fmt.Println(titleStyle.Render(result.Question))
	fmt.Println()
	fmt.Println(result.Synthesis)
	fmt.Println()
	fmt.Println(dimStyle.Render("References:"))
	for i, ref := range result.References {
		fmt.Printf("%d. %s\n", i+1, ref.CitationAPA)
	}
	fmt.Println()
	fmt.Println(dimStyle.Render(fmt.Sprintf("Tokens: ~%d", result.Tokens.Total)))
}

func countWords(s string) int {
	return len(strings.Fields(s))
}

// Config file handling.

func getConfigPath() string {
	var configDir string
	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			configDir = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
	default: // linux, darwin
		configDir = os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			configDir = filepath.Join(os.Getenv("HOME"), ".config")
		}
	}
	if strings.TrimSpace(configDir) == "" {
		configDir = "."
	}
	return filepath.Join(configDir, "pubmed-cli", "config.json")
}

func getDefaultOutputFolder() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		// Fall back to a relative folder so we don't accidentally join against an empty home dir.
		return "pubmed-syntheses"
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return filepath.Join(home, "Documents", "PubMed Syntheses")
	default:
		return filepath.Join(home, "pubmed-syntheses")
	}
}

func loadWizardConfig() WizardConfig {
	cfg := DefaultWizardConfig()

	configPath := getConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Treat invalid JSON as "no config".
		return DefaultWizardConfig()
	}
	return cfg
}

func saveWizardConfig(cfg WizardConfig) error {
	configPath := getConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, 0o644)
}
