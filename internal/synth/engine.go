// Package synth provides literature synthesis from PubMed searches.
package synth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// LLMClient is the interface for LLM completions.
type LLMClient interface {
	Complete(ctx context.Context, prompt string, maxTokens int) (string, error)
}

// Config controls synthesis behavior.
type Config struct {
	PapersToUse        int    // How many papers to include (default: 5)
	PapersToSearch     int    // How many to search before filtering (default: 30)
	RelevanceThreshold int    // Minimum relevance score 1-10 (default: 7)
	TargetWords        int    // Target word count (default: 250)
	CitationStyle      string // Citation style (default: apa)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		PapersToUse:        5,
		PapersToSearch:     30,
		RelevanceThreshold: 7,
		TargetWords:        250,
		CitationStyle:      "apa",
	}
}

func (c Config) validate() error {
	if c.PapersToUse < 1 {
		return fmt.Errorf("papers_to_use must be >= 1")
	}
	if c.PapersToSearch < 1 {
		return fmt.Errorf("papers_to_search must be >= 1")
	}
	if c.TargetWords < 1 {
		return fmt.Errorf("target_words must be >= 1")
	}
	if c.RelevanceThreshold < 1 || c.RelevanceThreshold > 10 {
		return fmt.Errorf("relevance_threshold must be 1-10")
	}
	// Allow PapersToUse > PapersToSearch, but it's almost certainly a misconfig.
	return nil
}

// ScoredPaper holds a paper with its relevance score.
type ScoredPaper struct {
	Article        eutils.Article
	RelevanceScore int
}

// Reference holds citation information.
type Reference struct {
	Key            string `json:"key"`
	PMID           string `json:"pmid"`
	CitationAPA    string `json:"citation_apa"`
	RelevanceScore int    `json:"relevance_score"`
	DOI            string `json:"doi,omitempty"`
	Title          string `json:"title"`
	Abstract       string `json:"abstract,omitempty"`
	Year           string `json:"year"`
	Authors        string `json:"authors"`

	// AuthorsList holds the full author list in BibTeX-friendly "Last, First" form.
	// It is internal-only and intentionally excluded from JSON output.
	AuthorsList []string `json:"-"`

	Journal string `json:"journal"`
}

// Result contains the synthesis output.
type Result struct {
	Question       string      `json:"question"`
	Synthesis      string      `json:"synthesis"`
	PapersSearched int         `json:"papers_searched"`
	PapersScored   int         `json:"papers_scored"`
	PapersUsed     int         `json:"papers_used"`
	References     []Reference `json:"references"`
	RIS            string      `json:"ris,omitempty"`
	Tokens         TokenUsage  `json:"tokens"`
}

// TokenUsage tracks token consumption.
type TokenUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Total  int `json:"total"`
}

// ProgressPhase indicates where we are in the synthesis pipeline.
type ProgressPhase string

const (
	ProgressSearch    ProgressPhase = "search"
	ProgressFetch     ProgressPhase = "fetch"
	ProgressScore     ProgressPhase = "score"
	ProgressFilter    ProgressPhase = "filter"
	ProgressSynthesis ProgressPhase = "synthesis"
	ProgressRIS       ProgressPhase = "ris"
)

// ProgressUpdate is emitted as the engine advances through the workflow.
// Current/Total are primarily used for per-paper scoring updates.
type ProgressUpdate struct {
	Phase   ProgressPhase
	Message string
	Current int
	Total   int
}

// ProgressCallback receives progress updates from the engine.
// It must be fast and must not block.
type ProgressCallback func(ProgressUpdate)

// Engine performs literature synthesis.
type Engine struct {
	llm      LLMClient
	eutils   *eutils.Client
	cfg      Config
	progress ProgressCallback
}

// NewEngine creates a new synthesis engine.
func NewEngine(llmClient LLMClient, eutilsClient *eutils.Client, cfg Config) *Engine {
	return &Engine{
		llm:    llmClient,
		eutils: eutilsClient,
		cfg:    cfg,
	}
}

// WithProgress sets an optional progress callback.
//
// The callback must be fast and must not block.
func (e *Engine) WithProgress(cb ProgressCallback) *Engine {
	if e == nil {
		return nil
	}
	e.progress = cb
	return e
}

func (e *Engine) report(update ProgressUpdate) {
	if e == nil || e.progress == nil {
		return
	}
	e.progress(update)
}

// Synthesize performs the full synthesis workflow.
func (e *Engine) Synthesize(ctx context.Context, question string) (*Result, error) {
	if e == nil {
		return nil, errors.New("synth engine is nil")
	}
	if e.llm == nil {
		return nil, errors.New("LLM client is nil")
	}
	if e.eutils == nil {
		return nil, errors.New("eutils client is nil")
	}
	if err := e.cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	question = strings.TrimSpace(question)
	if question == "" {
		return nil, errors.New("question is required")
	}

	result := &Result{Question: question}

	// Step 1: Search PubMed
	e.report(ProgressUpdate{Phase: ProgressSearch, Message: "Searching PubMed..."})
	searchResult, err := e.eutils.Search(ctx, question, &eutils.SearchOptions{Limit: e.cfg.PapersToSearch})
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if searchResult == nil {
		return nil, errors.New("search: nil result")
	}
	ids := searchResult.IDs
	result.PapersSearched = len(ids)
	if len(ids) == 0 {
		return nil, fmt.Errorf("no papers found for query: %s", question)
	}

	// Step 2: Fetch articles
	e.report(ProgressUpdate{Phase: ProgressFetch, Message: "Fetching paper metadata..."})
	articles, err := e.eutils.Fetch(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("fetch: no articles returned for query: %s", question)
	}

	// Step 3: Score relevance
	if n := len(articles); n > 0 {
		e.report(ProgressUpdate{Phase: ProgressScore, Message: fmt.Sprintf("Scoring paper %d/%d for relevance...", 1, n), Current: 0, Total: n})
	}
	scored, scoringTokens, err := e.scoreRelevance(ctx, question, articles)
	if err != nil {
		return nil, fmt.Errorf("relevance scoring: %w", err)
	}
	result.PapersScored = len(scored)
	result.Tokens.Input += scoringTokens.Input
	result.Tokens.Output += scoringTokens.Output

	// Step 4: Filter and sort by relevance
	e.report(ProgressUpdate{Phase: ProgressFilter, Message: fmt.Sprintf("Filtering to top %d papers...", e.cfg.PapersToUse)})
	var relevant []ScoredPaper
	for _, sp := range scored {
		if sp.RelevanceScore >= e.cfg.RelevanceThreshold {
			relevant = append(relevant, sp)
		}
	}

	sort.Slice(relevant, func(i, j int) bool {
		return relevant[i].RelevanceScore > relevant[j].RelevanceScore
	})

	if len(relevant) > e.cfg.PapersToUse {
		relevant = relevant[:e.cfg.PapersToUse]
	}
	if len(relevant) == 0 {
		return nil, fmt.Errorf("no papers met relevance threshold (%d) for: %s", e.cfg.RelevanceThreshold, question)
	}
	result.PapersUsed = len(relevant)

	// Step 5: Build references
	result.References = make([]Reference, 0, len(relevant))
	for i, sp := range relevant {
		ref := buildReference(sp.Article, i+1, sp.RelevanceScore)
		result.References = append(result.References, ref)
	}

	// Step 6: Generate synthesis
	e.report(ProgressUpdate{Phase: ProgressSynthesis, Message: "Generating synthesis..."})
	synthesis, tokens, err := e.generateSynthesis(ctx, question, relevant)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}
	result.Synthesis = synthesis
	result.Tokens.Input += tokens.Input
	result.Tokens.Output += tokens.Output

	// Step 7: Generate RIS
	e.report(ProgressUpdate{Phase: ProgressRIS, Message: "Generating RIS..."})
	result.RIS = GenerateRIS(result.References)
	result.Tokens.Total = result.Tokens.Input + result.Tokens.Output
	return result, nil
}

// SynthesizePMID performs deep dive on a single paper.
func (e *Engine) SynthesizePMID(ctx context.Context, pmid string) (*Result, error) {
	if e == nil {
		return nil, errors.New("synth engine is nil")
	}
	if e.llm == nil {
		return nil, errors.New("LLM client is nil")
	}
	if e.eutils == nil {
		return nil, errors.New("eutils client is nil")
	}
	if err := e.cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	pmid = strings.TrimSpace(pmid)
	if pmid == "" {
		return nil, errors.New("pmid is required")
	}

	result := &Result{
		Question:       fmt.Sprintf("Deep dive: PMID %s", pmid),
		PapersSearched: 1,
		PapersScored:   1,
		PapersUsed:     1,
	}

	articles, err := e.eutils.Fetch(ctx, []string{pmid})
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("article not found: %s", pmid)
	}

	article := articles[0]
	ref := buildReference(article, 1, 10)
	result.References = []Reference{ref}

	citeKey := inTextCiteKey(article)
	title := strings.TrimSpace(article.Title)
	if title == "" {
		title = "(no title available)"
	}
	abstract := strings.TrimSpace(article.Abstract)
	if abstract == "" {
		abstract = "(no abstract available)"
	}
	abstract = truncate(abstract, 2500)

	prompt := fmt.Sprintf(`Summarize this research paper in approximately %d words. Include:
- Main objective/question
- Key methods
- Primary findings
- Implications/conclusions

Title: %s

Abstract:
%s

Write a cohesive summary paragraph. Cite as (%s).`,
		e.cfg.TargetWords, title, abstract, citeKey)

	synthesis, err := e.llm.Complete(ctx, prompt, e.cfg.TargetWords*2)
	if err != nil {
		return nil, fmt.Errorf("synthesis: %w", err)
	}
	synthesis = strings.TrimSpace(synthesis)
	if synthesis == "" {
		return nil, errors.New("synthesis: empty response")
	}
	result.Synthesis = synthesis

	result.Tokens.Input = len(prompt) / 4
	result.Tokens.Output = len(synthesis) / 4
	result.Tokens.Total = result.Tokens.Input + result.Tokens.Output
	result.RIS = GenerateRIS(result.References)
	return result, nil
}

func (e *Engine) scoreRelevance(ctx context.Context, question string, articles []eutils.Article) ([]ScoredPaper, TokenUsage, error) {
	if e == nil || e.llm == nil {
		return nil, TokenUsage{}, errors.New("LLM client is nil")
	}
	question = strings.TrimSpace(question)

	scored := make([]ScoredPaper, 0, len(articles))
	totalTokens := TokenUsage{}
	var firstErr error
	errCount := 0
	total := len(articles)
	for i := range articles {
		// Emit a progress event *before* each call so the UI can show what we're about to score.
		e.report(ProgressUpdate{Phase: ProgressScore, Message: fmt.Sprintf("Scoring paper %d/%d for relevance...", i+1, total), Current: i, Total: total})

		article := &articles[i]
		score, tokens, err := scoreArticleRelevance(ctx, e.llm, question, article)
		if err != nil {
			// Never swallow cancellation/timeouts: callers expect prompt termination.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, totalTokens, ctxErr
			}
			// Continue with a neutral score; don't fail the whole run on a single scoring failure.
			// But if *all* scoring calls fail, surface the underlying error.
			errCount++
			if firstErr == nil {
				firstErr = err
			}
			score = 5
		}
		totalTokens.Input += tokens.Input
		totalTokens.Output += tokens.Output
		scored = append(scored, ScoredPaper{Article: *article, RelevanceScore: score})

		// Emit a second event after scoring so the progress bar can advance.
		e.report(ProgressUpdate{Phase: ProgressScore, Message: fmt.Sprintf("Scoring paper %d/%d for relevance...", i+1, total), Current: i + 1, Total: total})
	}
	if len(articles) > 0 && errCount == len(articles) {
		return nil, totalTokens, fmt.Errorf("relevance scoring failed for all %d articles: %w", errCount, firstErr)
	}
	return scored, totalTokens, nil
}

func (e *Engine) generateSynthesis(ctx context.Context, question string, papers []ScoredPaper) (string, TokenUsage, error) {
	if e == nil || e.llm == nil {
		return "", TokenUsage{}, errors.New("LLM client is nil")
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return "", TokenUsage{}, errors.New("question is required")
	}
	if len(papers) == 0 {
		return "", TokenUsage{}, errors.New("no papers provided")
	}

	contextParts := make([]string, 0, len(papers))
	citeKeys := make([]string, 0, len(papers))

	for i, sp := range papers {
		citeKey := inTextCiteKey(sp.Article)
		citeKeys = append(citeKeys, citeKey)

		abstract := sp.Article.Abstract
		if abstract == "" {
			abstract = "(no abstract available)"
		}
		abstract = truncate(abstract, 1500)

		contextParts = append(contextParts, fmt.Sprintf(`[%d] %s (%s)
Title: %s
Abstract: %s
`, i+1, citeKey, sp.Article.PMID, sp.Article.Title, abstract))
	}

	prompt := fmt.Sprintf(`You are a scientific writer. Synthesize the following research papers to answer this question:

Question: %s

Papers:
%s

Write a synthesis of approximately %d words that:
1. Directly addresses the question
2. Integrates findings across papers
3. Uses inline citations like (Smith et al., 2024)
4. Maintains academic tone
5. Notes any conflicting findings

Available citations: %s

Write the synthesis:`,
		question,
		strings.Join(contextParts, "\n---\n"),
		e.cfg.TargetWords,
		strings.Join(citeKeys, "; "))

	synthesis, err := e.llm.Complete(ctx, prompt, e.cfg.TargetWords*3)
	if err != nil {
		return "", TokenUsage{}, err
	}
	synthesis = strings.TrimSpace(synthesis)
	if synthesis == "" {
		return "", TokenUsage{}, errors.New("LLM returned empty synthesis")
	}

	return synthesis, TokenUsage{Input: len(prompt) / 4, Output: len(synthesis) / 4}, nil
}

func buildReference(article eutils.Article, num int, relevance int) Reference {
	// Build author string
	authorStr := "Unknown"
	if len(article.Authors) > 0 {
		switch len(article.Authors) {
		case 1:
			authorStr = article.Authors[0].FullName()
		case 2:
			authorStr = article.Authors[0].FullName() + " & " + article.Authors[1].FullName()
		default:
			authorStr = article.Authors[0].FullName() + " et al."
		}
	}

	authorsList := make([]string, 0, len(article.Authors))
	for _, a := range article.Authors {
		authorsList = append(authorsList, bibtexAuthorFromName(a.FullName()))
	}

	apa := formatAPA(article)

	key := fmt.Sprintf("%d", num)
	if len(article.Authors) > 0 {
		name := firstAuthorKeyName(article)
		year := normalizedYear(article.Year)
		if name != "" {
			key = fmt.Sprintf("%s %s", name, year)
		}
	}

	return Reference{
		Key:            key,
		PMID:           article.PMID,
		CitationAPA:    apa,
		RelevanceScore: relevance,
		DOI:            article.DOI,
		Title:          article.Title,
		Abstract:       article.Abstract,
		Year:           article.Year,
		Authors:        authorStr,
		AuthorsList:    authorsList,
		Journal:        article.Journal,
	}
}

func formatAPA(article eutils.Article) string {
	// Build author list for APA
	var authors string
	switch {
	case len(article.Authors) == 0:
		authors = "Unknown"
	case len(article.Authors) == 1:
		a := article.Authors[0]
		authors = apaAuthor(a)
	case len(article.Authors) <= 7:
		parts := make([]string, 0, len(article.Authors))
		for i, a := range article.Authors {
			if i == len(article.Authors)-1 {
				parts = append(parts, fmt.Sprintf("& %s", apaAuthor(a)))
			} else {
				parts = append(parts, apaAuthor(a))
			}
		}
		authors = strings.Join(parts, ", ")
	default:
		// More than 7 authors: first 6, ..., last
		parts := make([]string, 0, 8)
		for i := 0; i < 6; i++ {
			a := article.Authors[i]
			parts = append(parts, apaAuthor(a))
		}
		last := article.Authors[len(article.Authors)-1]
		parts = append(parts, "...")
		parts = append(parts, fmt.Sprintf("& %s", apaAuthor(last)))
		authors = strings.Join(parts, ", ")
	}

	year := normalizedYear(article.Year)
	citation := fmt.Sprintf("%s (%s). %s. %s.", authors, year, article.Title, article.Journal)
	if article.DOI != "" {
		citation += fmt.Sprintf(" https://doi.org/%s", article.DOI)
	}
	return citation
}

func apaAuthor(a eutils.Author) string {
	name := strings.TrimSpace(a.CollectiveName)
	if name == "" {
		last := strings.TrimSpace(a.LastName)
		fore := strings.TrimSpace(a.ForeName)
		switch {
		case last != "" && fore != "":
			name = fmt.Sprintf("%s, %s.", last, initials(fore))
		case last != "":
			name = last
		case fore != "":
			name = fore
		default:
			name = "Unknown"
		}
	}
	if name != "Unknown" && !strings.HasSuffix(name, ".") {
		name += "."
	}
	return name
}

func initials(foreName string) string {
	parts := strings.Fields(foreName)
	inits := make([]string, 0, len(parts))
	for _, p := range parts {
		r := []rune(p)
		if len(r) > 0 {
			inits = append(inits, string(r[0]))
		}
	}
	return strings.Join(inits, ". ")
}

func lastToken(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

func normalizedYear(year string) string {
	year = strings.TrimSpace(year)
	if year == "" {
		return "n.d."
	}
	return year
}

func firstAuthorKeyName(article eutils.Article) string {
	if len(article.Authors) == 0 {
		return ""
	}
	a := article.Authors[0]
	if s := strings.TrimSpace(a.CollectiveName); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.LastName); s != "" {
		return s
	}
	if s := strings.TrimSpace(a.FullName()); s != "" {
		return lastToken(s)
	}
	return ""
}

func inTextCiteKey(article eutils.Article) string {
	name := firstAuthorKeyName(article)
	if name == "" {
		name = "Unknown"
	}
	year := normalizedYear(article.Year)
	if len(article.Authors) <= 1 {
		return fmt.Sprintf("%s, %s", name, year)
	}
	return fmt.Sprintf("%s et al., %s", name, year)
}
