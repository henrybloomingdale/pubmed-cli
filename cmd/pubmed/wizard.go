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

	// Clear screen and show welcome.
	fmt.Print("\033[H\033[2J")
	fmt.Println(titleStyle.Render("🔬 PubMed Literature Synthesis"))
	fmt.Println(subtitleStyle.Render("Create a research synthesis with citations in seconds"))
	fmt.Println()

	// Form values.
	var (
		question     string
		papersStr    string
		wordsStr     string
		outputFormat string
		outputName   string
		confirm      bool
	)
	outputFormat = defaultOutputFormat(cfg)

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
		return err
	}
	if !confirm {
		fmt.Println(dimStyle.Render("\nCancelled."))
		return nil
	}

	// Parse values with defaults.
	papers := cfg.DefaultPapers
	papersStr = strings.TrimSpace(papersStr)
	if papersStr != "" {
		p, err := strconv.Atoi(papersStr)
		if err != nil {
			return fmt.Errorf("parse papers: %w", err)
		}
		papers = p
	}

	words := cfg.DefaultWords
	wordsStr = strings.TrimSpace(wordsStr)
	if wordsStr != "" {
		w, err := strconv.Atoi(wordsStr)
		if err != nil {
			return fmt.Errorf("parse words: %w", err)
		}
		words = w
	}

	if strings.TrimSpace(outputName) == "" {
		outputName = "synthesis"
	}
	name, err := sanitizeOutputName(outputName)
	if err != nil {
		return err
	}
	outputName = name

	// Ensure output folder exists.
	if err := os.MkdirAll(cfg.OutputFolder, 0o755); err != nil {
		return fmt.Errorf("create output folder: %w", err)
	}

	// Build file paths.
	docxPath := filepath.Join(cfg.OutputFolder, outputName+".docx")
	risPath := filepath.Join(cfg.OutputFolder, outputName+".ris")
	mdPath := filepath.Join(cfg.OutputFolder, outputName+".md")

	fmt.Println()

	// Build LLM client.
	var llmClient synth.LLMClient
	if cfg.UseClaude {
		llmClient, err = llm.NewClaudeClient(cfg.LLMModel)
		if err != nil {
			return fmt.Errorf("claude setup: %w", err)
		}
	} else {
		var opts []llm.Option
		if cfg.LLMModel != "" {
			opts = append(opts, llm.WithModel(cfg.LLMModel))
		}
		llmClient = llm.NewClient(opts...)
	}

	// Build synth config.
	synthCfg := synth.DefaultConfig()
	synthCfg.PapersToUse = papers
	synthCfg.TargetWords = words
	synthCfg.RelevanceThreshold = cfg.DefaultRelevance

	engine := synth.NewEngine(llmClient, newEutilsClient(), synthCfg)

	// Run synthesis with spinner.
	var (
		result   *synth.Result
		synthErr error
	)
	action := func() {
		result, synthErr = engine.Synthesize(cmd.Context(), question)
	}

	spinErr := spinner.New().
		Title("Synthesizing literature...").
		Action(action).
		Run()

	if spinErr != nil {
		return spinErr
	}
	if synthErr != nil {
		return synthErr
	}
	if result == nil {
		return errors.New("synthesis returned nil result")
	}

	// Save outputs based on format.
	var savedFiles []string

	switch outputFormat {
	case "docx+ris":
		if err := saveMarkdownFile(mdPath, result); err != nil {
			return err
		}
		if err := convertToDocxContext(cmd.Context(), mdPath, docxPath); err != nil {
			fmt.Fprintln(os.Stderr, dimStyle.Render(fmt.Sprintf("Pandoc conversion failed (%v). Keeping markdown output.", err)))
			// Fall back to just markdown.
			savedFiles = append(savedFiles, mdPath)
		} else {
			_ = os.Remove(mdPath) // best-effort cleanup
			savedFiles = append(savedFiles, docxPath)
		}
		if err := os.WriteFile(risPath, []byte(result.RIS), 0o644); err != nil {
			return fmt.Errorf("write RIS: %w", err)
		}
		savedFiles = append(savedFiles, risPath)

	case "docx":
		if err := saveMarkdownFile(mdPath, result); err != nil {
			return err
		}
		if err := convertToDocxContext(cmd.Context(), mdPath, docxPath); err != nil {
			fmt.Fprintln(os.Stderr, dimStyle.Render(fmt.Sprintf("Pandoc conversion failed (%v). Keeping markdown output.", err)))
			savedFiles = append(savedFiles, mdPath)
		} else {
			_ = os.Remove(mdPath)
			savedFiles = append(savedFiles, docxPath)
		}

	case "markdown":
		printMarkdownResult(result)
		return nil

	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)

	default:
		return fmt.Errorf("unknown output format: %q", outputFormat)
	}

	// Print success.
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
	return nil
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
