// Package qa implements adaptive retrieval for biomedical question answering.
package qa

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
	"github.com/henrybloomingdale/pubmed-cli/internal/llm"
)

// Strategy represents the retrieval decision.
type Strategy string

const (
	StrategyParametric Strategy = "parametric"
	StrategyRetrieval  Strategy = "retrieval"
)

// Result contains the QA result and metadata.
type Result struct {
	Question       string   `json:"question"`
	Answer         string   `json:"answer"`
	Confidence     int      `json:"confidence,omitempty"`
	Strategy       Strategy `json:"strategy"`
	NovelDetected  bool     `json:"novel_detected"`
	SourcePMIDs    []string `json:"source_pmids,omitempty"`
	MinifiedContext string  `json:"context,omitempty"`
}

// Config controls adaptive retrieval behavior.
type Config struct {
	ConfidenceThreshold int  // Default: 7
	ForceRetrieval      bool // Always retrieve
	ForceParametric     bool // Never retrieve
	MaxResults          int  // Papers to fetch
	Verbose             bool // Show reasoning
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ConfidenceThreshold: 7,
		MaxResults:          3,
	}
}

// Engine performs adaptive question answering.
type Engine struct {
	llm    *llm.Client
	eutils *eutils.Client
	cfg    Config
}

// NewEngine creates a new QA engine.
func NewEngine(llmClient *llm.Client, eutilsClient *eutils.Client, cfg Config) *Engine {
	return &Engine{
		llm:    llmClient,
		eutils: eutilsClient,
		cfg:    cfg,
	}
}

// Answer performs adaptive retrieval and returns an answer.
func (e *Engine) Answer(ctx context.Context, question string) (*Result, error) {
	result := &Result{
		Question: question,
	}

	// Step 1: Detect novelty
	result.NovelDetected = DetectNovelty(question)

	// Step 2: Decide strategy
	if e.cfg.ForceRetrieval || result.NovelDetected {
		result.Strategy = StrategyRetrieval
	} else if e.cfg.ForceParametric {
		result.Strategy = StrategyParametric
		return e.answerParametric(ctx, result)
	} else {
		// Check confidence
		answer, confidence, err := e.getConfidence(ctx, question)
		if err != nil {
			return nil, fmt.Errorf("confidence check: %w", err)
		}
		result.Confidence = confidence

		if confidence >= e.cfg.ConfidenceThreshold {
			result.Strategy = StrategyParametric
			result.Answer = answer
			return result, nil
		}
		result.Strategy = StrategyRetrieval
	}

	// Step 3: Retrieve and answer
	return e.answerWithRetrieval(ctx, result)
}

// DetectNovelty checks if the question requires post-training knowledge.
func DetectNovelty(question string) bool {
	q := strings.ToLower(question)

	// Check for recent year references
	yearPattern := regexp.MustCompile(`\b(202[4-9]|203\d)\b`)
	if yearPattern.MatchString(question) {
		return true
	}

	// Check for recency keywords
	recencyTerms := []string{
		"recent", "latest", "new study", "new research", "newly published",
		"this year", "last month", "just published",
	}
	for _, term := range recencyTerms {
		if strings.Contains(q, term) {
			return true
		}
	}

	return false
}

// ExpandQuery cleans up the question for PubMed search.
func ExpandQuery(question string) string {
	q := question

	// Remove question preambles
	preambles := []string{
		"According to a 2025 meta-analysis,",
		"According to a 2025 systematic review,",
		"According to a 2025 RCT,",
		"According to a 2025 study,",
		"According to 2025 studies,",
		"Based on a 2025 meta-analysis,",
		"Based on a 2025 RCT,",
		"Based on 2025 evidence,",
		"Based on 2025 studies,",
		"According to a", "Based on a", "Based on",
	}
	for _, p := range preambles {
		q = strings.Replace(q, p, "", 1)
		q = strings.Replace(q, strings.ToLower(p), "", 1)
	}
	
	// Trim before checking question words
	q = strings.TrimSpace(q)

	// Remove question words at start of query only
	questionWords := []string{"does ", "Does ", "do ", "Do ", "is ", "Is ", "can ", "Can "}
	for _, w := range questionWords {
		if strings.HasPrefix(q, w) {
			q = strings.TrimPrefix(q, w)
			break
		}
	}
	q = strings.TrimSuffix(q, "?")

	// Clean whitespace
	q = strings.TrimSpace(strings.Join(strings.Fields(q), " "))

	// Keep query reasonable length
	if len(q) > 150 {
		q = q[:150]
	}
	return q
}

// MinifyAbstract extracts key sentences from an abstract.
func MinifyAbstract(text string, maxChars int) string {
	if text == "" || len(text) <= maxChars {
		return text
	}

	// Split into sentences
	sentencePattern := regexp.MustCompile(`[.!?]+\s*`)
	sentences := sentencePattern.Split(text, -1)

	// Key terms for scoring
	keyTerms := []string{
		"conclusion", "result", "found", "showed", "demonstrated",
		"significant", "effective", "improved", "reduced", "increased",
		"associated", "compared", "outcome", "accuracy", "sensitivity",
		"specificity", "pooled", "meta-analysis",
	}

	// Score sentences
	type scored struct {
		score int
		text  string
	}
	var scoredSentences []scored

	labelPattern := regexp.MustCompile(`(?i)^(results?|conclusions?|findings?)\s*:`)
	statPattern := regexp.MustCompile(`\d+%|\d+\.\d+|95%\s*CI|p\s*[<=]`)

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) < 20 {
			continue
		}

		score := 0
		lower := strings.ToLower(s)

		// Score by key terms
		for _, term := range keyTerms {
			if strings.Contains(lower, term) {
				score++
			}
		}

		// Boost labeled sections
		if labelPattern.MatchString(s) {
			score += 3
		}

		// Boost sentences with statistics
		if statPattern.MatchString(s) {
			score += 2
		}

		scoredSentences = append(scoredSentences, scored{score, s})
	}

	// Sort by score descending
	sort.Slice(scoredSentences, func(i, j int) bool {
		return scoredSentences[i].score > scoredSentences[j].score
	})

	// Take best sentences up to maxChars
	var result []string
	total := 0
	for _, ss := range scoredSentences {
		if total+len(ss.text) > maxChars {
			break
		}
		result = append(result, ss.text)
		total += len(ss.text) + 2
	}

	if len(result) == 0 {
		if len(text) > maxChars {
			return text[:maxChars]
		}
		return text
	}

	return strings.Join(result, ". ") + "."
}

func (e *Engine) getConfidence(ctx context.Context, question string) (string, int, error) {
	prompt := fmt.Sprintf(`Answer this biomedical question.
CONFIDENCE (1-10):
ANSWER (yes/no):

Question: %s`, question)

	resp, err := e.llm.Complete(ctx, prompt, 50)
	if err != nil {
		return "", 0, err
	}

	// Parse response
	lower := strings.ToLower(resp)
	answer := "no"
	if strings.Contains(lower, "yes") {
		answer = "yes"
	}

	confidence := 5
	for _, line := range strings.Split(lower, "\n") {
		if strings.Contains(line, "confidence") && strings.Contains(line, ":") {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				numPattern := regexp.MustCompile(`\d+`)
				if match := numPattern.FindString(parts[len(parts)-1]); match != "" {
					fmt.Sscanf(match, "%d", &confidence)
					if confidence > 10 {
						confidence = 10
					}
				}
			}
			break
		}
	}

	return answer, confidence, nil
}

func (e *Engine) answerParametric(ctx context.Context, result *Result) (*Result, error) {
	prompt := fmt.Sprintf("Answer yes or no: %s\nANSWER:", result.Question)
	resp, err := e.llm.Complete(ctx, prompt, 10)
	if err != nil {
		return nil, err
	}

	if strings.Contains(strings.ToLower(resp), "yes") {
		result.Answer = "yes"
	} else {
		result.Answer = "no"
	}
	return result, nil
}

func (e *Engine) answerWithRetrieval(ctx context.Context, result *Result) (*Result, error) {
	// Expand and search
	query := ExpandQuery(result.Question)
	searchResult, err := e.eutils.Search(ctx, query, &eutils.SearchOptions{Limit: e.cfg.MaxResults})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	if len(searchResult.IDs) == 0 {
		// Fallback to parametric
		return e.answerParametric(ctx, result)
	}

	result.SourcePMIDs = searchResult.IDs

	// Fetch articles
	articles, err := e.eutils.Fetch(ctx, searchResult.IDs)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}

	// Build minified context
	var contextParts []string
	for _, a := range articles {
		minified := MinifyAbstract(a.Abstract, 400)
		contextParts = append(contextParts, fmt.Sprintf("**%s**\n%s", a.Title, minified))
	}
	context := strings.Join(contextParts, "\n\n")
	result.MinifiedContext = context

	// Answer with context
	prompt := fmt.Sprintf(`Question: %s

Evidence from PubMed:
%s

Based on this evidence, answer yes or no.
ANSWER:`, result.Question, context)

	resp, err := e.llm.Complete(ctx, prompt, 10)
	if err != nil {
		return nil, fmt.Errorf("answer: %w", err)
	}

	if strings.Contains(strings.ToLower(resp), "yes") {
		result.Answer = "yes"
	} else {
		result.Answer = "no"
	}

	return result, nil
}
