// Package mesh provides MeSH term lookup via NCBI E-utilities.
package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/ncbi"
)

// MeSHRecord represents a MeSH descriptor record.
type MeSHRecord struct {
	UI          string   `json:"ui"`
	Name        string   `json:"name"`
	ScopeNote   string   `json:"scope_note"`
	TreeNumbers []string `json:"tree_numbers"`
	EntryTerms  []string `json:"entry_terms"`
	Annotation  string   `json:"annotation,omitempty"`
}

// Client provides MeSH lookup functionality.
// It embeds ncbi.BaseClient for shared rate limiting and common parameters.
type Client struct {
	*ncbi.BaseClient
}

// NewClient creates a new MeSH lookup client using an existing NCBI base client.
func NewClient(base *ncbi.BaseClient) *Client {
	return &Client{BaseClient: base}
}

// esearchResult for parsing MeSH search.
type meshSearchResponse struct {
	Result meshSearchResult `json:"esearchresult"`
}

type meshSearchResult struct {
	Count  string   `json:"count"`
	IDList []string `json:"idlist"`
}

// Lookup searches for a MeSH term and returns its record.
func (c *Client) Lookup(ctx context.Context, term string) (*MeSHRecord, error) {
	if term == "" {
		return nil, fmt.Errorf("MeSH term cannot be empty")
	}

	// Step 1: Search for the term in MeSH database
	ids, err := c.searchMeSH(ctx, term)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("MeSH term %q not found", term)
	}

	// Step 2: Fetch the full record
	record, err := c.fetchMeSH(ctx, ids[0])
	if err != nil {
		return nil, err
	}

	return record, nil
}

func (c *Client) searchMeSH(ctx context.Context, term string) ([]string, error) {
	params := make(map[string][]string)
	vals := make(map[string]string)
	vals["db"] = "mesh"
	vals["term"] = term
	vals["retmode"] = "json"
	for k, v := range vals {
		params[k] = []string{v}
	}

	resp, err := c.DoGet(ctx, "esearch.fcgi", params)
	if err != nil {
		return nil, fmt.Errorf("MeSH search failed: %w", err)
	}

	var result meshSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("parsing MeSH search response: %w", err)
	}

	return result.Result.IDList, nil
}

func (c *Client) fetchMeSH(ctx context.Context, uid string) (*MeSHRecord, error) {
	params := make(map[string][]string)
	vals := map[string]string{
		"db":      "mesh",
		"id":      uid,
		"rettype": "full",
		"retmode": "text",
	}
	for k, v := range vals {
		params[k] = []string{v}
	}

	body, err := c.DoGet(ctx, "efetch.fcgi", params)
	if err != nil {
		return nil, fmt.Errorf("MeSH fetch failed: %w", err)
	}

	record := parseMeSHRecord(string(body))
	return &record, nil
}

// parseMeSHRecord parses the NCBI MeSH full text format into a MeSHRecord.
func parseMeSHRecord(text string) MeSHRecord {
	record := MeSHRecord{}

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "*NEWRECORD" {
			continue
		}

		parts := strings.SplitN(line, " = ", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "MH":
			record.Name = value
		case "UI":
			record.UI = value
		case "MS":
			record.ScopeNote = value
		case "MN":
			record.TreeNumbers = append(record.TreeNumbers, value)
		case "AN":
			record.Annotation = value
		case "ENTRY":
			// Entry terms have format: "Term|T047|..."
			entryParts := strings.SplitN(value, "|", 2)
			record.EntryTerms = append(record.EntryTerms, strings.TrimSpace(entryParts[0]))
		}
	}

	return record
}
