package eutils

import (
	"github.com/henrybloomingdale/pubmed-cli/internal/ncbi"
)

const (
	// DefaultBaseURL is the NCBI E-utilities base URL.
	DefaultBaseURL = ncbi.DefaultBaseURL
	// DefaultTool identifies this application to NCBI.
	DefaultTool = ncbi.DefaultTool
	// DefaultEmail is the contact email sent to NCBI.
	DefaultEmail = ncbi.DefaultEmail
)

// Client is an HTTP client for NCBI E-utilities.
// It embeds ncbi.BaseClient for shared rate limiting, common parameters,
// and response size guards.
type Client struct {
	*ncbi.BaseClient
}

// Option configures a Client (alias for ncbi.Option).
type Option = ncbi.Option

// Re-export ncbi options for backward compatibility.
var (
	WithBaseURL    = ncbi.WithBaseURL
	WithAPIKey     = ncbi.WithAPIKey
	WithTool       = ncbi.WithTool
	WithEmail      = ncbi.WithEmail
	WithHTTPClient = ncbi.WithHTTPClient
)

// NewClient creates a new E-utilities client with the given options.
// Options configure the underlying NCBI base client.
func NewClient(opts ...Option) *Client {
	return &Client{BaseClient: ncbi.NewBaseClient(opts...)}
}

// NewClientWithBase creates a new E-utilities client using an existing base client.
// Use this to share rate limiters across eutils and mesh clients.
func NewClientWithBase(base *ncbi.BaseClient) *Client {
	return &Client{BaseClient: base}
}
