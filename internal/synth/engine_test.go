package synth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PapersToUse != 5 {
		t.Errorf("PapersToUse = %d, want 5", cfg.PapersToUse)
	}
	if cfg.PapersToSearch != 30 {
		t.Errorf("PapersToSearch = %d, want 30", cfg.PapersToSearch)
	}
	if cfg.RelevanceThreshold != 7 {
		t.Errorf("RelevanceThreshold = %d, want 7", cfg.RelevanceThreshold)
	}
	if cfg.TargetWords != 250 {
		t.Errorf("TargetWords = %d, want 250", cfg.TargetWords)
	}
	if cfg.CitationStyle != "apa" {
		t.Errorf("CitationStyle = %q, want %q", cfg.CitationStyle, "apa")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		expectErr bool
	}{
		{
			name:      "valid default config",
			cfg:       DefaultConfig(),
			expectErr: false,
		},
		{
			name: "PapersToUse < 1",
			cfg: Config{
				PapersToUse:        0,
				PapersToSearch:     10,
				TargetWords:        100,
				RelevanceThreshold: 5,
			},
			expectErr: true,
		},
		{
			name: "PapersToSearch < 1",
			cfg: Config{
				PapersToUse:        5,
				PapersToSearch:     0,
				TargetWords:        100,
				RelevanceThreshold: 5,
			},
			expectErr: true,
		},
		{
			name: "TargetWords < 1",
			cfg: Config{
				PapersToUse:        5,
				PapersToSearch:     10,
				TargetWords:        0,
				RelevanceThreshold: 5,
			},
			expectErr: true,
		},
		{
			name: "RelevanceThreshold < 1",
			cfg: Config{
				PapersToUse:        5,
				PapersToSearch:     10,
				TargetWords:        100,
				RelevanceThreshold: 0,
			},
			expectErr: true,
		},
		{
			name: "RelevanceThreshold > 10",
			cfg: Config{
				PapersToUse:        5,
				PapersToSearch:     10,
				TargetWords:        100,
				RelevanceThreshold: 11,
			},
			expectErr: true,
		},
		{
			name: "boundary values valid",
			cfg: Config{
				PapersToUse:        1,
				PapersToSearch:     1,
				TargetWords:        1,
				RelevanceThreshold: 1,
			},
			expectErr: false,
		},
		{
			name: "max RelevanceThreshold valid",
			cfg: Config{
				PapersToUse:        5,
				PapersToSearch:     10,
				TargetWords:        100,
				RelevanceThreshold: 10,
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.validate()
			if tc.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestNewEngine(t *testing.T) {
	llm := &mockLLMClient{}
	eu := &eutils.Client{}
	cfg := DefaultConfig()

	engine := NewEngine(llm, eu, cfg)

	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if engine.llm != llm {
		t.Error("llm not set correctly")
	}
	if engine.eutils != eu {
		t.Error("eutils not set correctly")
	}
	if engine.cfg != cfg {
		t.Error("cfg not set correctly")
	}
}

func TestEngine_WithProgress(t *testing.T) {
	engine := NewEngine(&mockLLMClient{}, &eutils.Client{}, DefaultConfig())

	called := false
	engine.WithProgress(func(update ProgressUpdate) {
		called = true
	})

	engine.report(ProgressUpdate{Phase: ProgressSearch, Message: "test"})

	if !called {
		t.Error("progress callback was not called")
	}
}

func TestEngine_WithProgress_NilEngine(t *testing.T) {
	var engine *Engine
	result := engine.WithProgress(func(update ProgressUpdate) {})

	if result != nil {
		t.Error("WithProgress on nil engine should return nil")
	}
}

func TestEngine_Report_NilCallback(t *testing.T) {
	engine := NewEngine(&mockLLMClient{}, &eutils.Client{}, DefaultConfig())
	// Should not panic
	engine.report(ProgressUpdate{Phase: ProgressSearch, Message: "test"})
}

func TestEngine_Report_NilEngine(t *testing.T) {
	var engine *Engine
	// Should not panic
	engine.report(ProgressUpdate{Phase: ProgressSearch, Message: "test"})
}

func TestEngine_Synthesize_NilEngine(t *testing.T) {
	var engine *Engine
	_, err := engine.Synthesize(context.Background(), "test question")

	if err == nil {
		t.Error("expected error for nil engine")
	}
}

func TestEngine_Synthesize_NilLLM(t *testing.T) {
	engine := &Engine{
		llm:    nil,
		eutils: &eutils.Client{},
		cfg:    DefaultConfig(),
	}

	_, err := engine.Synthesize(context.Background(), "test question")

	if err == nil {
		t.Error("expected error for nil LLM")
	}
	if !strings.Contains(err.Error(), "LLM client is nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEngine_Synthesize_NilEutils(t *testing.T) {
	engine := &Engine{
		llm:    &mockLLMClient{},
		eutils: nil,
		cfg:    DefaultConfig(),
	}

	_, err := engine.Synthesize(context.Background(), "test question")

	if err == nil {
		t.Error("expected error for nil eutils")
	}
	if !strings.Contains(err.Error(), "eutils client is nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEngine_Synthesize_InvalidConfig(t *testing.T) {
	engine := &Engine{
		llm:    &mockLLMClient{},
		eutils: &eutils.Client{},
		cfg:    Config{PapersToUse: 0}, // Invalid
	}

	_, err := engine.Synthesize(context.Background(), "test question")

	if err == nil {
		t.Error("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "invalid config") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEngine_Synthesize_EmptyQuestion(t *testing.T) {
	engine := &Engine{
		llm:    &mockLLMClient{},
		eutils: &eutils.Client{},
		cfg:    DefaultConfig(),
	}

	tests := []string{"", "   ", "\n\t"}

	for _, q := range tests {
		_, err := engine.Synthesize(context.Background(), q)
		if err == nil {
			t.Errorf("expected error for question %q", q)
		}
		if !strings.Contains(err.Error(), "question is required") {
			t.Errorf("unexpected error message for %q: %v", q, err)
		}
	}
}

func TestEngine_SynthesizePMID_NilEngine(t *testing.T) {
	var engine *Engine
	_, err := engine.SynthesizePMID(context.Background(), "12345678")

	if err == nil {
		t.Error("expected error for nil engine")
	}
}

func TestEngine_SynthesizePMID_NilLLM(t *testing.T) {
	engine := &Engine{
		llm:    nil,
		eutils: &eutils.Client{},
		cfg:    DefaultConfig(),
	}

	_, err := engine.SynthesizePMID(context.Background(), "12345678")

	if err == nil {
		t.Error("expected error for nil LLM")
	}
}

func TestEngine_SynthesizePMID_NilEutils(t *testing.T) {
	engine := &Engine{
		llm:    &mockLLMClient{},
		eutils: nil,
		cfg:    DefaultConfig(),
	}

	_, err := engine.SynthesizePMID(context.Background(), "12345678")

	if err == nil {
		t.Error("expected error for nil eutils")
	}
}

func TestEngine_SynthesizePMID_EmptyPMID(t *testing.T) {
	engine := &Engine{
		llm:    &mockLLMClient{},
		eutils: &eutils.Client{},
		cfg:    DefaultConfig(),
	}

	tests := []string{"", "   ", "\n\t"}

	for _, pmid := range tests {
		_, err := engine.SynthesizePMID(context.Background(), pmid)
		if err == nil {
			t.Errorf("expected error for pmid %q", pmid)
		}
		if !strings.Contains(err.Error(), "pmid is required") {
			t.Errorf("unexpected error message for %q: %v", pmid, err)
		}
	}
}

func TestEngine_ScoreRelevance_NilEngine(t *testing.T) {
	var engine *Engine
	_, _, err := engine.scoreRelevance(context.Background(), "question", nil)

	if err == nil {
		t.Error("expected error for nil engine")
	}
}

func TestEngine_ScoreRelevance_NilLLM(t *testing.T) {
	engine := &Engine{llm: nil}
	_, _, err := engine.scoreRelevance(context.Background(), "question", nil)

	if err == nil {
		t.Error("expected error for nil LLM")
	}
}

func TestEngine_GenerateSynthesis_NilEngine(t *testing.T) {
	var engine *Engine
	_, _, err := engine.generateSynthesis(context.Background(), "question", nil)

	if err == nil {
		t.Error("expected error for nil engine")
	}
}

func TestEngine_GenerateSynthesis_NilLLM(t *testing.T) {
	engine := &Engine{llm: nil}
	_, _, err := engine.generateSynthesis(context.Background(), "question", nil)

	if err == nil {
		t.Error("expected error for nil LLM")
	}
}

func TestEngine_GenerateSynthesis_EmptyQuestion(t *testing.T) {
	engine := &Engine{llm: &mockLLMClient{}}
	_, _, err := engine.generateSynthesis(context.Background(), "", nil)

	if err == nil {
		t.Error("expected error for empty question")
	}
}

func TestEngine_GenerateSynthesis_NoPapers(t *testing.T) {
	engine := &Engine{llm: &mockLLMClient{}, cfg: DefaultConfig()}
	_, _, err := engine.generateSynthesis(context.Background(), "question", []ScoredPaper{})

	if err == nil {
		t.Error("expected error for no papers")
	}
}

func TestEngine_GenerateSynthesis_LLMError(t *testing.T) {
	engine := &Engine{
		llm: &mockLLMClient{err: errors.New("API error")},
		cfg: DefaultConfig(),
	}

	papers := []ScoredPaper{
		{
			Article:        eutils.Article{Title: "Test", Abstract: "Abstract"},
			RelevanceScore: 8,
		},
	}

	_, _, err := engine.generateSynthesis(context.Background(), "question", papers)

	if err == nil {
		t.Error("expected error for LLM failure")
	}
}

func TestEngine_GenerateSynthesis_EmptyResponse(t *testing.T) {
	engine := &Engine{
		llm: &mockLLMClient{response: "   "},
		cfg: DefaultConfig(),
	}

	papers := []ScoredPaper{
		{
			Article:        eutils.Article{Title: "Test", Abstract: "Abstract"},
			RelevanceScore: 8,
		},
	}

	_, _, err := engine.generateSynthesis(context.Background(), "question", papers)

	if err == nil {
		t.Error("expected error for empty LLM response")
	}
}

func TestBuildReference(t *testing.T) {
	article := eutils.Article{
		PMID:     "12345678",
		Title:    "Test Article Title",
		Abstract: "This is the abstract.",
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Jones", ForeName: "Jane"},
		},
		Journal: "Nature",
		Year:    "2024",
		DOI:     "10.1234/test",
	}

	ref := buildReference(article, 1, 9)

	if ref.PMID != "12345678" {
		t.Errorf("PMID = %q, want %q", ref.PMID, "12345678")
	}
	if ref.Title != "Test Article Title" {
		t.Errorf("Title = %q, want %q", ref.Title, "Test Article Title")
	}
	if ref.RelevanceScore != 9 {
		t.Errorf("RelevanceScore = %d, want 9", ref.RelevanceScore)
	}
	if ref.Year != "2024" {
		t.Errorf("Year = %q, want %q", ref.Year, "2024")
	}
	if ref.DOI != "10.1234/test" {
		t.Errorf("DOI = %q, want %q", ref.DOI, "10.1234/test")
	}
	if ref.Journal != "Nature" {
		t.Errorf("Journal = %q, want %q", ref.Journal, "Nature")
	}

	// Authors should be formatted as "First Last & Second Last" for two authors
	if !strings.Contains(ref.Authors, "John Smith") {
		t.Errorf("Authors should contain 'John Smith': %q", ref.Authors)
	}
	if !strings.Contains(ref.Authors, "&") {
		t.Errorf("Authors should contain '&' for two authors: %q", ref.Authors)
	}

	// AuthorsList should be in BibTeX format
	if len(ref.AuthorsList) != 2 {
		t.Errorf("AuthorsList should have 2 authors, got %d", len(ref.AuthorsList))
	}
}

func TestBuildReference_SingleAuthor(t *testing.T) {
	article := eutils.Article{
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
		},
	}

	ref := buildReference(article, 1, 8)

	if ref.Authors != "John Smith" {
		t.Errorf("Authors = %q, want %q", ref.Authors, "John Smith")
	}
}

func TestBuildReference_ManyAuthors(t *testing.T) {
	article := eutils.Article{
		Authors: []eutils.Author{
			{LastName: "Smith", ForeName: "John"},
			{LastName: "Jones", ForeName: "Jane"},
			{LastName: "Brown", ForeName: "Bob"},
		},
	}

	ref := buildReference(article, 1, 8)

	if !strings.Contains(ref.Authors, "et al.") {
		t.Errorf("Authors should contain 'et al.' for 3+ authors: %q", ref.Authors)
	}
}

func TestBuildReference_NoAuthors(t *testing.T) {
	article := eutils.Article{
		Authors: []eutils.Author{},
	}

	ref := buildReference(article, 1, 8)

	if ref.Authors != "Unknown" {
		t.Errorf("Authors = %q, want %q", ref.Authors, "Unknown")
	}
}

func TestFormatAPA(t *testing.T) {
	tests := []struct {
		name     string
		article  eutils.Article
		contains []string
	}{
		{
			name: "single author",
			article: eutils.Article{
				Authors: []eutils.Author{{LastName: "Smith", ForeName: "John"}},
				Year:    "2024",
				Title:   "Test Title",
				Journal: "Nature",
			},
			contains: []string{"Smith, J.", "2024", "Test Title", "Nature"},
		},
		{
			name: "two authors",
			article: eutils.Article{
				Authors: []eutils.Author{
					{LastName: "Smith", ForeName: "John"},
					{LastName: "Jones", ForeName: "Jane"},
				},
				Year: "2024",
			},
			contains: []string{"Smith, J.", "& Jones, J."},
		},
		{
			name: "more than 7 authors uses ellipsis",
			article: eutils.Article{
				Authors: []eutils.Author{
					{LastName: "A", ForeName: "A"},
					{LastName: "B", ForeName: "B"},
					{LastName: "C", ForeName: "C"},
					{LastName: "D", ForeName: "D"},
					{LastName: "E", ForeName: "E"},
					{LastName: "F", ForeName: "F"},
					{LastName: "G", ForeName: "G"},
					{LastName: "H", ForeName: "H"},
				},
				Year: "2024",
			},
			contains: []string{"...", "& H, H."},
		},
		{
			name: "no authors",
			article: eutils.Article{
				Authors: []eutils.Author{},
				Year:    "2024",
			},
			contains: []string{"Unknown"},
		},
		{
			name: "no year",
			article: eutils.Article{
				Authors: []eutils.Author{{LastName: "Smith"}},
				Year:    "",
			},
			contains: []string{"n.d."},
		},
		{
			name: "with DOI",
			article: eutils.Article{
				Authors: []eutils.Author{{LastName: "Smith"}},
				Year:    "2024",
				DOI:     "10.1234/test",
			},
			contains: []string{"https://doi.org/10.1234/test"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatAPA(tc.article)
			for _, expected := range tc.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("formatAPA should contain %q\nGot: %s", expected, result)
				}
			}
		})
	}
}

func TestApaAuthor(t *testing.T) {
	tests := []struct {
		name     string
		author   eutils.Author
		expected string
	}{
		{
			name:     "full name",
			author:   eutils.Author{LastName: "Smith", ForeName: "John"},
			expected: "Smith, J.",
		},
		{
			name:     "last name only",
			author:   eutils.Author{LastName: "Smith"},
			expected: "Smith.",
		},
		{
			name:     "fore name only",
			author:   eutils.Author{ForeName: "John"},
			expected: "John.",
		},
		{
			name:     "collective name",
			author:   eutils.Author{CollectiveName: "WHO"},
			expected: "WHO.",
		},
		{
			name:     "multiple fore names",
			author:   eutils.Author{LastName: "Smith", ForeName: "John Paul"},
			expected: "Smith, J. P.",
		},
		{
			name:     "empty author",
			author:   eutils.Author{},
			expected: "Unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := apaAuthor(tc.author)
			if got != tc.expected {
				t.Errorf("apaAuthor() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestInitials(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John", "J"},
		{"John Paul", "J. P"},
		{"", ""},
		{"A B C", "A. B. C"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := initials(tc.input)
			if got != tc.expected {
				t.Errorf("initials(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestLastToken(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Smith", "Smith"},
		{"Smith", "Smith"},
		{"", ""},
		{"A B C", "C"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := lastToken(tc.input)
			if got != tc.expected {
				t.Errorf("lastToken(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestNormalizedYear(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2024", "2024"},
		{"", "n.d."},
		{"  2024  ", "2024"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := normalizedYear(tc.input)
			if got != tc.expected {
				t.Errorf("normalizedYear(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestFirstAuthorKeyName(t *testing.T) {
	tests := []struct {
		name     string
		article  eutils.Article
		expected string
	}{
		{
			name:     "no authors",
			article:  eutils.Article{Authors: []eutils.Author{}},
			expected: "",
		},
		{
			name:     "with last name",
			article:  eutils.Article{Authors: []eutils.Author{{LastName: "Smith"}}},
			expected: "Smith",
		},
		{
			name:     "collective name",
			article:  eutils.Article{Authors: []eutils.Author{{CollectiveName: "WHO"}}},
			expected: "WHO",
		},
		{
			name:     "full name fallback",
			article:  eutils.Article{Authors: []eutils.Author{{ForeName: "John Paul"}}},
			expected: "Paul",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := firstAuthorKeyName(tc.article)
			if got != tc.expected {
				t.Errorf("firstAuthorKeyName() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestInTextCiteKey(t *testing.T) {
	tests := []struct {
		name     string
		article  eutils.Article
		expected string
	}{
		{
			name:     "single author",
			article:  eutils.Article{Authors: []eutils.Author{{LastName: "Smith"}}, Year: "2024"},
			expected: "Smith, 2024",
		},
		{
			name: "two authors",
			article: eutils.Article{
				Authors: []eutils.Author{{LastName: "Smith"}, {LastName: "Jones"}},
				Year:    "2024",
			},
			expected: "Smith et al., 2024",
		},
		{
			name:     "no author",
			article:  eutils.Article{Year: "2024"},
			expected: "Unknown, 2024",
		},
		{
			name:     "no year",
			article:  eutils.Article{Authors: []eutils.Author{{LastName: "Smith"}}},
			expected: "Smith, n.d.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := inTextCiteKey(tc.article)
			if got != tc.expected {
				t.Errorf("inTextCiteKey() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestProgressPhase_Constants(t *testing.T) {
	// Verify all phases exist
	phases := []ProgressPhase{
		ProgressSearch,
		ProgressFetch,
		ProgressScore,
		ProgressFilter,
		ProgressSynthesis,
		ProgressRIS,
	}

	for _, phase := range phases {
		if phase == "" {
			t.Error("phase should not be empty")
		}
	}
}

func TestTokenUsage_Fields(t *testing.T) {
	usage := TokenUsage{
		Input:  100,
		Output: 50,
		Total:  150,
	}

	if usage.Input != 100 {
		t.Errorf("Input = %d, want 100", usage.Input)
	}
	if usage.Output != 50 {
		t.Errorf("Output = %d, want 50", usage.Output)
	}
	if usage.Total != 150 {
		t.Errorf("Total = %d, want 150", usage.Total)
	}
}

func TestResult_Fields(t *testing.T) {
	result := Result{
		Question:       "Test question",
		Synthesis:      "Test synthesis",
		PapersSearched: 30,
		PapersScored:   25,
		PapersUsed:     5,
		References:     []Reference{{Title: "Test"}},
		RIS:            "TY  - JOUR",
		Tokens:         TokenUsage{Total: 100},
	}

	if result.Question != "Test question" {
		t.Error("Question not set correctly")
	}
	if result.PapersSearched != 30 {
		t.Error("PapersSearched not set correctly")
	}
	if len(result.References) != 1 {
		t.Error("References not set correctly")
	}
}

func TestScoredPaper_Fields(t *testing.T) {
	sp := ScoredPaper{
		Article:        eutils.Article{Title: "Test"},
		RelevanceScore: 8,
	}

	if sp.Article.Title != "Test" {
		t.Error("Article not set correctly")
	}
	if sp.RelevanceScore != 8 {
		t.Error("RelevanceScore not set correctly")
	}
}
