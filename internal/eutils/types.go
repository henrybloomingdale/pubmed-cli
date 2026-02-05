// Package eutils provides a client for NCBI E-utilities API.
package eutils

// SearchResult represents the result of an ESearch query.
type SearchResult struct {
	Count            int      `json:"count"`
	IDs              []string `json:"ids"`
	QueryTranslation string   `json:"query_translation"`
	WebEnv           string   `json:"web_env,omitempty"`
	QueryKey         string   `json:"query_key,omitempty"`
}

// Article represents a PubMed article with parsed fields.
type Article struct {
	PMID             string           `json:"pmid"`
	Title            string           `json:"title"`
	Abstract         string           `json:"abstract"`
	AbstractSections []AbstractSection `json:"abstract_sections,omitempty"`
	Authors          []Author         `json:"authors"`
	Journal          string           `json:"journal"`
	JournalAbbrev    string           `json:"journal_abbrev"`
	Volume           string           `json:"volume,omitempty"`
	Issue            string           `json:"issue,omitempty"`
	Pages            string           `json:"pages,omitempty"`
	Year             string           `json:"year"`
	Month            string           `json:"month,omitempty"`
	DOI              string           `json:"doi,omitempty"`
	PMCID            string           `json:"pmcid,omitempty"`
	MeSHTerms        []MeSHTerm       `json:"mesh_terms,omitempty"`
	PublicationTypes []string         `json:"publication_types"`
	Language         string           `json:"language"`
}

// AbstractSection represents a labeled section of a structured abstract.
type AbstractSection struct {
	Label string `json:"label,omitempty"`
	Text  string `json:"text"`
}

// Author represents an article author.
type Author struct {
	LastName       string `json:"last_name"`
	ForeName       string `json:"fore_name"`
	Initials       string `json:"initials"`
	CollectiveName string `json:"collective_name,omitempty"`
	Affiliation    string `json:"affiliation,omitempty"`
}

// FullName returns "ForeName LastName", or CollectiveName if present.
func (a Author) FullName() string {
	if a.CollectiveName != "" {
		return a.CollectiveName
	}
	if a.ForeName == "" {
		return a.LastName
	}
	return a.ForeName + " " + a.LastName
}

// MeSHTerm represents a MeSH heading with optional qualifiers.
type MeSHTerm struct {
	Descriptor   string   `json:"descriptor"`
	DescriptorUI string   `json:"descriptor_ui"`
	MajorTopic   bool     `json:"major_topic"`
	Qualifiers   []string `json:"qualifiers,omitempty"`
}

// LinkResult represents the result of an ELink query.
type LinkResult struct {
	SourceID string     `json:"source_id"`
	Links    []LinkItem `json:"links"`
}

// LinkItem represents a single linked article, optionally with a relevance score.
type LinkItem struct {
	ID    string `json:"id"`
	Score int    `json:"score,omitempty"`
}

// SearchOptions configures a search query.
type SearchOptions struct {
	Limit   int    `json:"limit,omitempty"`
	Sort    string `json:"sort,omitempty"`
	MinDate string `json:"min_date,omitempty"`
	MaxDate string `json:"max_date,omitempty"`
}
