# pubmed-cli

A command-line interface for NCBI PubMed E-utilities. Search PubMed, fetch article details, explore citations, and look up MeSH terms — all from your terminal.

## Installation

```bash
go install github.com/henrybloomingdale/pubmed-cli/cmd/pubmed@latest
```

Or build from source:

```bash
git clone https://github.com/henrybloomingdale/pubmed-cli.git
cd pubmed-cli
make build
```

## Configuration

### NCBI API Key (recommended)

Without an API key, NCBI limits you to 3 requests/second. With one, you get 10 req/sec.

Get a free key at: https://www.ncbi.nlm.nih.gov/account/settings/

```bash
# Set as environment variable
export NCBI_API_KEY="your-key-here"

# Or pass per-command
pubmed search --api-key "your-key-here" "fragile x syndrome"
```

## Usage

### Search

```bash
# Basic search
pubmed search "fragile x syndrome"

# MeSH term search with date filter
pubmed search '"fragile x syndrome"[MeSH] AND "electroencephalography"[MeSH]' --year 2020-2025

# Search with filters
pubmed search "ADHD treatment" --type review --limit 10 --sort date

# JSON output (pipe-friendly)
pubmed search "autism biomarkers" --json | jq '.ids[]'
```

### Fetch Article Details

```bash
# Single article
pubmed fetch 38123456

# Multiple articles
pubmed fetch 38123456 37987654 37876543

# JSON output with full details
pubmed fetch 38123456 --json
```

### Citations

```bash
# Papers that cite this article
pubmed cited-by 38123456

# References in this article
pubmed references 38123456

# Similar articles (with relevance scores)
pubmed related 38123456
```

### MeSH Term Lookup

```bash
# Look up a MeSH term
pubmed mesh "Fragile X Syndrome"

# JSON output
pubmed mesh "Electroencephalography" --json
```

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--json` | Structured JSON output | `false` |
| `--limit N` | Maximum results | `20` |
| `--sort` | Sort: `relevance`, `date`, `cited` | `relevance` |
| `--year` | Year range filter (e.g., `2020-2025`) | — |
| `--type` | Publication type: `review`, `trial`, `meta-analysis` | — |
| `--api-key` | NCBI API key (or `NCBI_API_KEY` env var) | — |

## Examples

### Research Workflow

```bash
# 1. Find recent reviews on a topic
pubmed search "fragile x syndrome EEG biomarkers" --type review --year 2020-2025 --json

# 2. Get full details for interesting papers
pubmed fetch 38123456 --json | jq '{title: .title, doi: .doi, authors: [.authors[].full_name]}'

# 3. Explore citation network
pubmed cited-by 38123456 --json | jq '.links[].id'

# 4. Find related work
pubmed related 38123456 --limit 5

# 5. Look up MeSH terms for better searches
pubmed mesh "Electroencephalography"
```

### Piping and Scripting

```bash
# Search → Fetch pipeline
pubmed search "CRISPR therapy" --json | jq -r '.ids[:5][]' | xargs pubmed fetch --json

# Export citations
pubmed fetch 38123456 37987654 --json > papers.json
```

## Development

### Prerequisites

- Go 1.21+
- Make

### Build & Test

```bash
# Build
make build

# Run unit tests
make test

# Run integration tests (hits real NCBI API)
NCBI_API_KEY="your-key" make test-integration

# Lint
make lint

# Test coverage
make coverage
```

### Project Structure

```
pubmed-cli/
├── cmd/pubmed/          # CLI entry point
├── internal/
│   ├── eutils/          # E-utilities HTTP client
│   │   ├── client.go    # Rate-limited HTTP client
│   │   ├── search.go    # ESearch
│   │   ├── fetch.go     # EFetch (XML parsing)
│   │   ├── link.go      # ELink (citations, references, related)
│   │   └── types.go     # Shared types
│   ├── mesh/            # MeSH term lookup
│   └── output/          # JSON and human-readable formatting
├── testdata/            # Fixture files for unit tests
├── Makefile
└── README.md
```

### Test Strategy

- **Unit tests** use `net/http/httptest` with canned NCBI responses in `testdata/`
- **Integration tests** (`//go:build integration`) hit the real NCBI API
- TDD approach: tests written before implementation

## License

MIT

## NCBI E-utilities

This tool uses the [NCBI E-utilities API](https://www.ncbi.nlm.nih.gov/books/NBK25501/). Please respect their [usage guidelines](https://www.ncbi.nlm.nih.gov/books/NBK25497/):

- Include tool name and email in requests (handled automatically)
- Max 3 requests/second without API key, 10 with key
- Do not make concurrent requests from a single IP
