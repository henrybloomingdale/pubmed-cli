package eutils

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

const (
	linkCitedIn  = "pubmed_pubmed_citedin"
	linkRefs     = "pubmed_pubmed_refs"
	linkRelated  = "pubmed_pubmed"
)

// ELink JSON response structures.
type elinkResponse struct {
	LinkSets []elinkLinkSet `json:"linksets"`
}

type elinkLinkSet struct {
	DbFrom     string          `json:"dbfrom"`
	IDs        []elinkID       `json:"ids"`
	LinkSetDBs []elinkLinkSetDB `json:"linksetdbs"`
}

type elinkID struct {
	Value string `json:"value"`
}

type elinkLinkSetDB struct {
	DbTo     string      `json:"dbto"`
	LinkName string      `json:"linkname"`
	Links    []elinkLink `json:"links"`
}

type elinkLink struct {
	ID    string `json:"id"`
	Score string `json:"score,omitempty"`
}

// CitedBy returns papers that cite the given PMID.
func (c *Client) CitedBy(ctx context.Context, pmid string) (*LinkResult, error) {
	return c.link(ctx, pmid, linkCitedIn, false)
}

// References returns papers referenced by the given PMID.
func (c *Client) References(ctx context.Context, pmid string) (*LinkResult, error) {
	return c.link(ctx, pmid, linkRefs, false)
}

// Related returns similar articles for the given PMID with relevance scores.
func (c *Client) Related(ctx context.Context, pmid string) (*LinkResult, error) {
	return c.link(ctx, pmid, linkRelated, true)
}

func (c *Client) link(ctx context.Context, pmid, linkName string, withScores bool) (*LinkResult, error) {
	if pmid == "" {
		return nil, fmt.Errorf("PMID cannot be empty")
	}

	params := url.Values{}
	params.Set("dbfrom", "pubmed")
	params.Set("db", "pubmed")
	params.Set("id", pmid)
	params.Set("linkname", linkName)
	params.Set("retmode", "json")
	if withScores {
		params.Set("cmd", "neighbor_score")
	}

	body, err := c.DoGet(ctx, "elink.fcgi", params)
	if err != nil {
		return nil, fmt.Errorf("link request failed: %w", err)
	}

	var resp elinkResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parsing link response: %w", err)
	}

	result := &LinkResult{
		SourceID: pmid,
	}

	if len(resp.LinkSets) > 0 && len(resp.LinkSets[0].LinkSetDBs) > 0 {
		for _, link := range resp.LinkSets[0].LinkSetDBs[0].Links {
			item := LinkItem{
				ID: link.ID,
			}
			if link.Score != "" {
				item.Score, _ = strconv.Atoi(link.Score)
			}
			result.Links = append(result.Links, item)
		}
	}

	// Ensure Links is non-nil empty slice for JSON serialization
	if result.Links == nil {
		result.Links = []LinkItem{}
	}

	return result, nil
}
