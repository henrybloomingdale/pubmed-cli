package output

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/henrybloomingdale/pubmed-cli/internal/eutils"
)

// writeArticlesRIS exports article details to RIS format for citation managers.
func writeArticlesRIS(path string, articles []eutils.Article) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating RIS file: %w", err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for i, a := range articles {
		writeRISTag(w, "TY", "JOUR")
		writeRISTag(w, "TI", a.Title)

		for _, au := range a.Authors {
			writeRISTag(w, "AU", risAuthor(au))
		}

		writeRISTag(w, "PY", a.Year)
		writeRISTag(w, "JO", a.Journal)
		writeRISTag(w, "VL", a.Volume)
		writeRISTag(w, "IS", a.Issue)

		startPage, endPage := splitPages(a.Pages)
		writeRISTag(w, "SP", startPage)
		writeRISTag(w, "EP", endPage)

		writeRISTag(w, "DO", a.DOI)
		writeRISTag(w, "AB", a.Abstract)
		if a.PMID != "" {
			writeRISTag(w, "ID", "PMID:"+a.PMID)
			writeRISTag(w, "UR", "https://pubmed.ncbi.nlm.nih.gov/"+a.PMID+"/")
		}
		writeRISTag(w, "ER", "")

		if i < len(articles)-1 {
			if _, err := w.WriteString("\n"); err != nil {
				return fmt.Errorf("writing RIS separator: %w", err)
			}
		}
	}

	if err := w.Flush(); err != nil {
		return fmt.Errorf("flushing RIS output: %w", err)
	}

	return nil
}

func writeRISTag(w *bufio.Writer, tag, value string) {
	if tag == "" {
		return
	}
	if tag != "ER" && strings.TrimSpace(value) == "" {
		return
	}
	if tag == "ER" {
		_, _ = w.WriteString("ER  -\n")
		return
	}
	_, _ = w.WriteString(tag + "  - " + sanitizeRISValue(value) + "\n")
}

func sanitizeRISValue(v string) string {
	v = strings.ReplaceAll(v, "\r\n", " ")
	v = strings.ReplaceAll(v, "\n", " ")
	v = strings.ReplaceAll(v, "\r", " ")
	return strings.TrimSpace(v)
}

func risAuthor(a eutils.Author) string {
	if a.CollectiveName != "" {
		return a.CollectiveName
	}
	last := strings.TrimSpace(a.LastName)
	fore := strings.TrimSpace(a.ForeName)
	if last == "" {
		return fore
	}
	if fore == "" {
		return last
	}
	return last + ", " + fore
}

func splitPages(pages string) (string, string) {
	pages = strings.TrimSpace(pages)
	if pages == "" {
		return "", ""
	}

	rangeSeparators := []string{"-", "–", "—"}
	for _, sep := range rangeSeparators {
		if strings.Contains(pages, sep) {
			parts := strings.SplitN(pages, sep, 2)
			start := strings.TrimSpace(parts[0])
			end := strings.TrimSpace(parts[1])
			return start, end
		}
	}

	return pages, ""
}
