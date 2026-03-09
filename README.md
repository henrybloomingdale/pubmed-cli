# pubmed-cli

[![CI](https://github.com/drpedapati/pubmed-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/drpedapati/pubmed-cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/drpedapati/pubmed-cli)](https://github.com/drpedapati/pubmed-cli/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-green.svg)](./LICENSE)

`pubmed-cli` is a command-line interface for NCBI PubMed E-utilities.

It focuses on deterministic, scriptable literature workflows on the `main` branch:
- `search`
- `fetch`
- `cited-by`
- `references`
- `related`
- `mesh`
- `refcheck`

## Installation

### Homebrew (macOS and Linux)

```bash
brew tap henrybloomingdale/tools
brew install pubmed-cli
```

### Go

```bash
go install github.com/henrybloomingdale/pubmed-cli/cmd/pubmed@latest
```

### Build from source

```bash
git clone https://github.com/drpedapati/pubmed-cli.git
cd pubmed-cli
go build -o pubmed ./cmd/pubmed
```

## Configuration

Set your NCBI API key (recommended):

```bash
export NCBI_API_KEY="your-key"
```

NCBI rate limits:
- Without key: 3 requests/second
- With key: 10 requests/second

## Quick Start

```bash
# Basic search
pubmed search "fragile x syndrome" --limit 5 --human

# Fetch one PMID
pubmed fetch 38000001 --human --full

# Fetch multiple PMIDs (space or comma-separated)
pubmed fetch 38000001 38000002 --json
pubmed fetch "38000001,38000002" --json

# Export RIS for EndNote/Zotero import
pubmed fetch 38000001 38000002 --ris refs.ris

# Citation graph
pubmed cited-by 38000001 --limit 5 --json
pubmed references 38000001 --limit 5 --json
pubmed related 38000001 --limit 5 --human
pubmed related 38000001 --limit 10 --ris related.ris

# MeSH lookup
pubmed mesh "depression" --json

# Verify document references against PubMed
pubmed refcheck manuscript.docx --human
pubmed refcheck manuscript.docx --json
pubmed refcheck manuscript.docx --audit-text --csv-out report.csv --ris-out verified.ris
```

## Command Behavior

### Global Flags

| Flag | Description |
|------|-------------|
| `--json` | Structured JSON output |
| `--human`, `-H` | Rich terminal rendering |
| `--csv FILE` | Export current result to CSV |
| `--ris FILE` | Export citations in RIS format (fetch/link commands) |
| `--full` | Show full abstract text (human article output) |
| `--limit N` | Maximum results (must be `> 0`) |
| `--sort` | `relevance`, `date`, or `cited` |
| `--year` | `YYYY` or `YYYY-YYYY` |
| `--type` | Publication-type filter (`review`, `trial`, `meta-analysis`, `randomized`, `case-report`, or custom) |
| `--api-key` | NCBI API key override |

### Input Validation

The CLI now fails fast for common mistakes:
- Invalid `--limit` values (`<= 0`) are rejected.
- Invalid `--sort` values are rejected.
- Invalid year formats and descending ranges are rejected.
- Invalid PMIDs (non-digits) are rejected in `fetch`, `cited-by`, `references`, and `related`.
- `--ris` is supported on `fetch`, `cited-by`, `references`, and `related` (rejected for `search` and `mesh`).
- `refcheck` validates that the input file exists and that `docx-review` is installed.

## Production Reliability Notes

- Shared NCBI client with rate limiting and response-size guards.
- Automatic retry with backoff for transient NCBI `HTTP 429` responses.
- UTF-8 safe text truncation in human output.
- Tiered PubMed query strategy for reference verification (PMID → DOI → title → author+year → relaxed).
- Hallucination detection for potentially fabricated references.

## Development

```bash
# Build
go build ./...

# Test
go test ./...

# Vet
go vet ./...
```

Additional development docs:
- `docs/development/TDD.md`
- `docs/development/CODE_REVIEW.md`
- `docs/development/UX_TESTING.md`
- `docs/homebrew.md`
- `RELEASING.md`

Repository metadata:
- `CONTRIBUTING.md`
- `SECURITY.md`
- `.github/workflows/ci.yml`
- `.github/workflows/release-assets.yml`

## Branching Note

- `main`: non-AI command set listed above, including `refcheck` for document reference verification.
- `ai-features`: historical branch for AI/LLM workflows.

## License

MIT
