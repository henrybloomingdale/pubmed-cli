package eutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

// esearchResponse represents the raw JSON response from ESearch.
type esearchResponse struct {
	Result esearchResult `json:"esearchresult"`
}

type esearchResult struct {
	Count            string   `json:"count"`
	RetMax           string   `json:"retmax"`
	RetStart         string   `json:"retstart"`
	IDList           []string `json:"idlist"`
	QueryTranslation string   `json:"querytranslation"`
	WebEnv           string   `json:"webenv"`
	QueryKey         string   `json:"querykey"`
}

// Search performs an ESearch query against PubMed.
func (c *Client) Search(ctx context.Context, query string, opts *SearchOptions) (*SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("search query cannot be empty")
	}

	params := url.Values{}
	params.Set("db", "pubmed")
	params.Set("term", query)
	params.Set("retmode", "json")
	params.Set("usehistory", "y")

	limit := 20
	if opts != nil {
		if opts.Limit > 0 {
			limit = opts.Limit
		}
		if opts.Sort != "" {
			params.Set("sort", opts.Sort)
		}
		if opts.MinDate != "" && opts.MaxDate != "" {
			params.Set("datetype", "pdat")
			params.Set("mindate", opts.MinDate)
			params.Set("maxdate", opts.MaxDate)
		}
	}
	params.Set("retmax", strconv.Itoa(limit))

	body, err := c.DoGet(ctx, "esearch.fcgi", params)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}

	var resp esearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing search response: %w", err)
	}

	count, _ := strconv.Atoi(resp.Result.Count)

	return &SearchResult{
		Count:            count,
		IDs:              resp.Result.IDList,
		QueryTranslation: resp.Result.QueryTranslation,
		WebEnv:           resp.Result.WebEnv,
		QueryKey:         resp.Result.QueryKey,
	}, nil
}
