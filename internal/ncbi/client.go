// Package ncbi provides a shared base HTTP client for NCBI E-utilities.
// Both eutils and mesh clients embed or reference this to share rate limiting,
// common parameters, and response size guards.
package ncbi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"
)

const (
	// DefaultBaseURL is the NCBI E-utilities base URL.
	DefaultBaseURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
	// DefaultTool identifies this application to NCBI.
	DefaultTool = "pubmed-cli"
	// DefaultEmail is the contact email sent to NCBI.
	DefaultEmail = "pubmed-cli@users.noreply.github.com"

	// Rate limits per NCBI policy.
	RateWithoutKey = 3  // requests per second without API key
	RateWithKey    = 10 // requests per second with API key

	// DefaultMaxResponseBytes is the maximum response body size (50 MB).
	DefaultMaxResponseBytes int64 = 50 * 1024 * 1024
)

// BaseClient is a shared HTTP client for NCBI E-utilities with proper
// rate limiting, common parameter injection, and response size guards.
type BaseClient struct {
	BaseURL    string
	APIKey     string
	Tool       string
	Email      string
	HTTPClient *http.Client
	Limiter    *rate.Limiter
	MaxBytes   int64
}

// Option configures a BaseClient.
type Option func(*BaseClient)

// WithBaseURL sets the base URL for requests.
func WithBaseURL(u string) Option {
	return func(c *BaseClient) { c.BaseURL = u }
}

// WithAPIKey sets the NCBI API key and adjusts the rate limit accordingly.
func WithAPIKey(key string) Option {
	return func(c *BaseClient) {
		c.APIKey = key
		if key != "" {
			c.Limiter = rate.NewLimiter(rate.Limit(RateWithKey), 1)
		}
	}
}

// WithTool sets the tool parameter for NCBI requests.
func WithTool(tool string) Option {
	return func(c *BaseClient) { c.Tool = tool }
}

// WithEmail sets the email parameter for NCBI requests.
func WithEmail(email string) Option {
	return func(c *BaseClient) { c.Email = email }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *BaseClient) { c.HTTPClient = hc }
}

// WithMaxResponseBytes sets the maximum allowed response body size.
func WithMaxResponseBytes(n int64) Option {
	return func(c *BaseClient) { c.MaxBytes = n }
}

// NewBaseClient creates a new NCBI base client with the given options.
func NewBaseClient(opts ...Option) *BaseClient {
	c := &BaseClient{
		BaseURL:  DefaultBaseURL,
		Tool:     DefaultTool,
		Email:    DefaultEmail,
		MaxBytes: DefaultMaxResponseBytes,
		Limiter:  rate.NewLimiter(rate.Limit(RateWithoutKey), 1),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// DoGet performs a rate-limited GET request with common NCBI parameters
// and response size limits. Returns the response body.
func (c *BaseClient) DoGet(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
	// Wait for rate limiter token (respects context cancellation).
	if err := c.Limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	// Add common NCBI params.
	if c.APIKey != "" {
		params.Set("api_key", c.APIKey)
	}
	if c.Tool != "" {
		params.Set("tool", c.Tool)
	}
	if c.Email != "" {
		params.Set("email", c.Email)
	}

	u, err := url.JoinPath(c.BaseURL, endpoint)
	if err != nil {
		return nil, fmt.Errorf("building URL: %w", err)
	}
	fullURL := u + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("NCBI rate limit exceeded (HTTP 429). Consider using an API key with --api-key or NCBI_API_KEY env var")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NCBI returned HTTP %d for %s", resp.StatusCode, endpoint)
	}

	// Guard against unbounded reads: read up to MaxBytes+1 to detect oversized responses.
	r := io.LimitReader(resp.Body, c.MaxBytes+1)
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if int64(len(body)) > c.MaxBytes {
		return nil, fmt.Errorf("response exceeds maximum size of %d bytes", c.MaxBytes)
	}

	return body, nil
}
