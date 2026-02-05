package eutils

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
)

// XML structures for parsing PubMed EFetch responses.

type pubmedArticleSet struct {
	XMLName  xml.Name        `xml:"PubmedArticleSet"`
	Articles []pubmedArticle `xml:"PubmedArticle"`
}

type pubmedArticle struct {
	Citation  medlineCitation `xml:"MedlineCitation"`
	PubmedData pubmedData    `xml:"PubmedData"`
}

type medlineCitation struct {
	PMID              xmlPMID           `xml:"PMID"`
	Article           xmlArticle        `xml:"Article"`
	MeshHeadingList   xmlMeshHeadingList `xml:"MeshHeadingList"`
}

type xmlPMID struct {
	Value string `xml:",chardata"`
}

type xmlArticle struct {
	Journal             xmlJournal            `xml:"Journal"`
	ArticleTitle        string                `xml:"ArticleTitle"`
	Abstract            xmlAbstract           `xml:"Abstract"`
	AuthorList          xmlAuthorList         `xml:"AuthorList"`
	Language            []string              `xml:"Language"`
	PublicationTypeList xmlPublicationTypeList `xml:"PublicationTypeList"`
	Pagination          xmlPagination         `xml:"Pagination"`
}

type xmlJournal struct {
	JournalIssue    xmlJournalIssue `xml:"JournalIssue"`
	Title           string          `xml:"Title"`
	ISOAbbreviation string          `xml:"ISOAbbreviation"`
}

type xmlJournalIssue struct {
	Volume  string      `xml:"Volume"`
	Issue   string      `xml:"Issue"`
	PubDate xmlPubDate  `xml:"PubDate"`
}

type xmlPubDate struct {
	Year  string `xml:"Year"`
	Month string `xml:"Month"`
	Day   string `xml:"Day"`
}

type xmlAbstract struct {
	AbstractTexts []xmlAbstractText `xml:"AbstractText"`
}

type xmlAbstractText struct {
	Label string `xml:"Label,attr"`
	Text  string `xml:",chardata"`
}

type xmlAuthorList struct {
	Complete string      `xml:"CompleteYN,attr"`
	Authors  []xmlAuthor `xml:"Author"`
}

type xmlAuthor struct {
	ValidYN         string             `xml:"ValidYN,attr"`
	LastName        string             `xml:"LastName"`
	ForeName        string             `xml:"ForeName"`
	Initials        string             `xml:"Initials"`
	AffiliationInfo []xmlAffiliationInfo `xml:"AffiliationInfo"`
}

type xmlAffiliationInfo struct {
	Affiliation string `xml:"Affiliation"`
}

type xmlPublicationTypeList struct {
	Types []xmlPublicationType `xml:"PublicationType"`
}

type xmlPublicationType struct {
	UI   string `xml:"UI,attr"`
	Name string `xml:",chardata"`
}

type xmlPagination struct {
	MedlinePgn string `xml:"MedlinePgn"`
}

type xmlMeshHeadingList struct {
	MeshHeadings []xmlMeshHeading `xml:"MeshHeading"`
}

type xmlMeshHeading struct {
	Descriptor xmlDescriptorName   `xml:"DescriptorName"`
	Qualifiers []xmlQualifierName  `xml:"QualifierName"`
}

type xmlDescriptorName struct {
	UI         string `xml:"UI,attr"`
	MajorTopic string `xml:"MajorTopicYN,attr"`
	Name       string `xml:",chardata"`
}

type xmlQualifierName struct {
	UI         string `xml:"UI,attr"`
	MajorTopic string `xml:"MajorTopicYN,attr"`
	Name       string `xml:",chardata"`
}

type pubmedData struct {
	ArticleIDList xmlArticleIDList `xml:"ArticleIdList"`
}

type xmlArticleIDList struct {
	ArticleIDs []xmlArticleID `xml:"ArticleId"`
}

type xmlArticleID struct {
	IDType string `xml:"IdType,attr"`
	Value  string `xml:",chardata"`
}

// Fetch retrieves full article details for the given PMIDs.
func (c *Client) Fetch(ctx context.Context, pmids []string) ([]Article, error) {
	if len(pmids) == 0 {
		return nil, fmt.Errorf("at least one PMID is required")
	}

	params := url.Values{}
	params.Set("db", "pubmed")
	params.Set("id", strings.Join(pmids, ","))
	params.Set("rettype", "xml")
	params.Set("retmode", "xml")

	body, err := c.DoGet(ctx, "efetch.fcgi", params)
	if err != nil {
		return nil, fmt.Errorf("fetch request failed: %w", err)
	}

	return parseArticles(body)
}

// parseArticles parses PubMed XML into Article structs.
func parseArticles(data []byte) ([]Article, error) {
	var articleSet pubmedArticleSet
	if err := xml.Unmarshal(data, &articleSet); err != nil {
		return nil, fmt.Errorf("parsing PubMed XML: %w", err)
	}

	articles := make([]Article, 0, len(articleSet.Articles))
	for _, pa := range articleSet.Articles {
		article := convertArticle(pa)
		articles = append(articles, article)
	}

	return articles, nil
}

func convertArticle(pa pubmedArticle) Article {
	mc := pa.Citation
	xa := mc.Article

	a := Article{
		PMID:          mc.PMID.Value,
		Title:         xa.ArticleTitle,
		Journal:       xa.Journal.Title,
		JournalAbbrev: xa.Journal.ISOAbbreviation,
		Volume:        xa.Journal.JournalIssue.Volume,
		Issue:         xa.Journal.JournalIssue.Issue,
		Pages:         xa.Pagination.MedlinePgn,
		Year:          xa.Journal.JournalIssue.PubDate.Year,
		Month:         xa.Journal.JournalIssue.PubDate.Month,
	}

	// Language
	if len(xa.Language) > 0 {
		a.Language = xa.Language[0]
	}

	// Abstract sections
	for _, at := range xa.Abstract.AbstractTexts {
		a.AbstractSections = append(a.AbstractSections, AbstractSection{
			Label: at.Label,
			Text:  at.Text,
		})
	}

	// Build full abstract text
	if len(a.AbstractSections) > 0 {
		var parts []string
		for _, s := range a.AbstractSections {
			if s.Label != "" {
				parts = append(parts, s.Label+": "+s.Text)
			} else {
				parts = append(parts, s.Text)
			}
		}
		a.Abstract = strings.Join(parts, "\n\n")
	}

	// Authors
	for _, au := range xa.AuthorList.Authors {
		if au.ValidYN == "N" {
			continue
		}
		author := Author{
			LastName: au.LastName,
			ForeName: au.ForeName,
			Initials: au.Initials,
		}
		if len(au.AffiliationInfo) > 0 {
			author.Affiliation = au.AffiliationInfo[0].Affiliation
		}
		a.Authors = append(a.Authors, author)
	}

	// Article IDs (DOI, PMCID)
	for _, aid := range pa.PubmedData.ArticleIDList.ArticleIDs {
		switch aid.IDType {
		case "doi":
			a.DOI = aid.Value
		case "pmc":
			a.PMCID = aid.Value
		}
	}

	// MeSH terms
	for _, mh := range mc.MeshHeadingList.MeshHeadings {
		term := MeSHTerm{
			Descriptor:   mh.Descriptor.Name,
			DescriptorUI: mh.Descriptor.UI,
			MajorTopic:   mh.Descriptor.MajorTopic == "Y",
		}
		for _, q := range mh.Qualifiers {
			term.Qualifiers = append(term.Qualifiers, q.Name)
		}
		a.MeSHTerms = append(a.MeSHTerms, term)
	}

	// Publication types
	for _, pt := range xa.PublicationTypeList.Types {
		a.PublicationTypes = append(a.PublicationTypes, pt.Name)
	}

	return a
}
