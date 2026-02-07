package synth

import (
	"context"
	"errors"
	"testing"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// mockLLMClient implements LLMClient for testing.
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(ctx context.Context, prompt string, maxTokens int) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestParseScore(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"simple digit", "7", 7},
		{"digit with whitespace", "  8  ", 8},
		{"digit in text", "The score is 9", 9},
		{"digit 10", "10", 10},
		{"digit 10 in text", "I would rate this 10 out of 10", 10},
		{"minimum score", "1", 1},
		{"maximum score", "10", 10},
		{"invalid zero", "0", 5},
		{"invalid negative", "-3", 3}, // regex matches "3" in "-3"
		{"invalid too high", "11", 5},
		{"invalid too high 15", "15", 5},
		{"no digit", "high relevance", 5},
		{"empty string", "", 5},
		{"only whitespace", "   ", 5},
		{"multiple digits picks first valid", "2 then 8", 2},
		{"score with period", "7.", 7},
		{"score in parentheses", "(6)", 6},
		{"mixed content", "Rating: 5/10", 5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseScore(tc.input)
			if got != tc.expected {
				t.Errorf("parseScore(%q) = %d, want %d", tc.input, got, tc.expected)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"longer than max", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
		{"zero max length", "hello", 0, ""},
		{"negative max length", "hello", -1, ""},
		{"unicode chars", "日本語テスト", 3, "日本語..."},
		{"unicode exact", "日本語", 3, "日本語"},
		{"single char", "a", 1, "a"},
		{"truncate to 1", "hello", 1, "h..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.expected {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.expected)
			}
		})
	}
}

func TestScoreArticleRelevance(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		llm           LLMClient
		question      string
		article       *eutils.Article
		expectedScore int
		expectError   bool
	}{
		{
			name:     "nil LLM client",
			llm:      nil,
			question: "test question",
			article: &eutils.Article{
				Title:    "Test Article",
				Abstract: "Test abstract",
			},
			expectError: true,
		},
		{
			name:        "nil article",
			llm:         &mockLLMClient{response: "8"},
			question:    "test question",
			article:     nil,
			expectError: true,
		},
		{
			name:     "successful scoring",
			llm:      &mockLLMClient{response: "8"},
			question: "What is the effect of caffeine on sleep?",
			article: &eutils.Article{
				Title:    "Caffeine and Sleep Quality",
				Abstract: "This study examines the relationship between caffeine consumption and sleep quality...",
			},
			expectedScore: 8,
			expectError:   false,
		},
		{
			name:     "LLM returns text with score",
			llm:      &mockLLMClient{response: "I would rate this as 9 out of 10"},
			question: "test question",
			article: &eutils.Article{
				Title:    "Test",
				Abstract: "Test abstract",
			},
			expectedScore: 9,
			expectError:   false,
		},
		{
			name:     "LLM returns invalid response defaults to 5",
			llm:      &mockLLMClient{response: "very relevant"},
			question: "test question",
			article: &eutils.Article{
				Title:    "Test",
				Abstract: "Test abstract",
			},
			expectedScore: 5,
			expectError:   false,
		},
		{
			name:     "LLM error",
			llm:      &mockLLMClient{err: errors.New("API error")},
			question: "test question",
			article: &eutils.Article{
				Title:    "Test",
				Abstract: "Test abstract",
			},
			expectError: true,
		},
		{
			name:     "empty abstract",
			llm:      &mockLLMClient{response: "6"},
			question: "test question",
			article: &eutils.Article{
				Title:    "Test Article",
				Abstract: "",
			},
			expectedScore: 6,
			expectError:   false,
		},
		{
			name:     "very long abstract gets truncated",
			llm:      &mockLLMClient{response: "7"},
			question: "test question",
			article: &eutils.Article{
				Title:    "Test Article",
				Abstract: string(make([]byte, 1000)), // Long abstract
			},
			expectedScore: 7,
			expectError:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score, tokens, err := scoreArticleRelevance(ctx, tc.llm, tc.question, tc.article)

			if tc.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if score != tc.expectedScore {
				t.Errorf("score = %d, want %d", score, tc.expectedScore)
			}

			if tokens <= 0 {
				t.Error("expected positive token count")
			}
		})
	}
}

func TestScoreArticleRelevance_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	llm := &mockLLMClient{err: ctx.Err()}
	article := &eutils.Article{
		Title:    "Test",
		Abstract: "Test abstract",
	}

	_, _, err := scoreArticleRelevance(ctx, llm, "question", article)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}
