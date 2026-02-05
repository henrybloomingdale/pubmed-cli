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
	DbFrom     string           `json:"dbfrom"`
	IDs        []string         `json:"ids"`
	LinkSetDBs []elinkLinkSetDB `json:"linksetdbs"`
}

type elinkLinkSetDB struct {
	DbTo     string          `json:"dbto"`
	LinkName string          `json:"linkname"`
	Links    []elinkLink     `json:"links"`
}

// elinkLink handles both plain string IDs and {id, score} objects.
// NCBI returns plain strings for cited-by/refs and objects with scores for related.
type elinkLink struct {
	id    string
	score string
}

func (e *elinkLink) UnmarshalJSON(data []byte) error {
	// Try plain string first (cited-by, refs)
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		e.id = s
		return nil
	}
	// Try object with id/score (related with neighbor_score)
	// Score can be string or number depending on NCBI endpoint
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("cannot parse elink link: %s", string(data))
	}
	if raw, ok := obj["id"]; ok {
		var id string
		if err := json.Unmarshal(raw, &id); err != nil {
			// Try as number
			var num int
			if err2 := json.Unmarshal(raw, &num); err2 != nil {
				return fmt.Errorf("cannot parse elink link id: %s", string(raw))
			}
			id = strconv.Itoa(num)
		}
		e.id = id
	}
	if raw, ok := obj["score"]; ok {
		var scoreStr string
		if err := json.Unmarshal(raw, &scoreStr); err != nil {
			// Try as number
			var num int
			if err2 := json.Unmarshal(raw, &num); err2 == nil {
				scoreStr = strconv.Itoa(num)
			}
		}
		e.score = scoreStr
	}
	return nil
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

	if len(resp.LinkSets) > 0 {
		for _, lsdb := range resp.LinkSets[0].LinkSetDBs {
			if lsdb.LinkName != linkName {
				continue
			}
			for _, link := range lsdb.Links {
				item := LinkItem{
					ID: link.id,
				}
				if link.score != "" {
					item.Score, _ = strconv.Atoi(link.score)
				}
				result.Links = append(result.Links, item)
			}
		}
	}

	// Ensure Links is non-nil empty slice for JSON serialization
	if result.Links == nil {
		result.Links = []LinkItem{}
	}

	return result, nil
}
