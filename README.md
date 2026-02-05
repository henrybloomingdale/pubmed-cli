<p align="center">
  <h1 align="center">🔬 pubmed-cli</h1>
  <p align="center">
    <strong>PubMed from your terminal. Built for humans and AI agents.</strong>
  </p>
  <p align="center">
    <a href="https://github.com/henrybloomingdale/pubmed-cli/actions"><img src="https://img.shields.io/github/actions/workflow/status/henrybloomingdale/pubmed-cli/ci.yml?branch=main&style=flat-square&label=tests" alt="CI"></a>
    <a href="https://goreportcard.com/report/github.com/henrybloomingdale/pubmed-cli"><img src="https://goreportcard.com/badge/github.com/henrybloomingdale/pubmed-cli?style=flat-square" alt="Go Report Card"></a>
    <img src="https://img.shields.io/badge/go-1.25-00ADD8?style=flat-square&logo=go" alt="Go 1.25">
    <img src="https://img.shields.io/badge/license-MIT-green?style=flat-square" alt="MIT License">
  </p>
</p>

---

Search PubMed, fetch abstracts, traverse citation networks, and look up MeSH terms -- all from the command line. Outputs structured JSON for piping into scripts, dashboards, or LLM tool-use loops.

**Why this exists:** Standard RAG retrieves what's *similar*. Agentic tool use retrieves what's *relevant*. This CLI gives any LLM agent direct, composable access to the 37M+ articles in PubMed via NCBI E-utilities -- no vector database, no embedding model, no retrieval corpus to maintain.

## ✨ Features

- **6 commands** -- `search`, `fetch`, `cited-by`, `references`, `related`, `mesh`
- **Dual output** -- `--json` for machines, `--human` for rich terminal display
- **Rate-limited** -- respects NCBI guidelines (3 req/s default, 10 with API key)
- **Zero dependencies** -- single static binary, ~5ms startup
- **Pipe-friendly** -- compose with `jq`, `xargs`, or any scripting language
- **Agent-ready** -- designed as a tool for LLM function calling / agentic workflows

## 📦 Installation

```bash
# Go install
go install github.com/henrybloomingdale/pubmed-cli/cmd/pubmed@latest

# Or build from source
git clone https://github.com/henrybloomingdale/pubmed-cli.git
cd pubmed-cli
make build
```

## ⚙️ Configuration

### NCBI API Key (recommended)

Without a key you're limited to 3 requests/second. With one, you get 10. Free at [ncbi.nlm.nih.gov/account/settings](https://www.ncbi.nlm.nih.gov/account/settings/).

```bash
# Environment variable (preferred)
export NCBI_API_KEY="your-key-here"

# Or .env file in working directory
echo 'NCBI_API_KEY=your-key-here' > .env

# Or per-command
pubmed search --api-key "your-key" "fragile x syndrome"
```

## 🚀 Usage

### Search

```bash
# Basic search
pubmed search "fragile x syndrome"

# MeSH + date filter
pubmed search '"fragile x syndrome"[MeSH] AND "electroencephalography"[MeSH]' --year 2020-2025

# Filters and sorting
pubmed search "ADHD treatment" --type review --limit 10 --sort date

# JSON for scripting
pubmed search "autism biomarkers" --json | jq '.ids[]'

# Rich terminal output
pubmed search "CRISPR therapy" --human
```

### Fetch

```bash
# Single article (full abstract, authors, MeSH, DOI)
pubmed fetch 38123456

# Multiple articles
pubmed fetch 38123456 37987654 37876543

# JSON with jq
pubmed fetch 38123456 --json | jq '{title: .title, doi: .doi}'
```

### Citation Network

```bash
# Who cited this paper?
pubmed cited-by 38123456

# What does this paper cite?
pubmed references 38123456

# Similar articles (NCBI relevance scores)
pubmed related 38123456
```

### MeSH Terms

```bash
# Look up a MeSH descriptor
pubmed mesh "Fragile X Syndrome"

# JSON output
pubmed mesh "Electroencephalography" --json
```

## 🤖 Agent Tool Use

The CLI is designed to be called by LLM agents via function calling. Define tools that map to commands:

```python
tools = [
    {"name": "pubmed_search",  "exec": "pubmed --json search {query}"},
    {"name": "pubmed_fetch",   "exec": "pubmed --json fetch {pmid}"},
    {"name": "pubmed_cited_by","exec": "pubmed --json cited-by {pmid}"},
    # ...
]
```

An agent can then autonomously:
1. **Search** -- formulate and refine PubMed queries
2. **Fetch** -- read abstracts for relevant hits
3. **Traverse** -- follow citation networks to find seminal or recent work
4. **Synthesize** -- answer questions grounded in real literature

This approach outperforms standard RAG on biomedical QA benchmarks. See our [MIRAGE evaluation](docs/benchmark.md) (coming soon).

## 📋 Reference

| Flag | Description | Default |
|------|-------------|---------|
| `--json` | Structured JSON output | `false` |
| `--human` | Rich terminal display | `false` |
| `--csv` | CSV export | `false` |
| `--limit N` | Max results | `20` |
| `--sort` | `relevance` \| `date` \| `cited` | `relevance` |
| `--year` | Year range (e.g. `2020-2025`) | -- |
| `--type` | `review` \| `trial` \| `meta-analysis` | -- |
| `--api-key` | NCBI API key | `$NCBI_API_KEY` |

## 🏗️ Architecture

```
pubmed-cli/
├── cmd/pubmed/          # Cobra CLI entry point
├── internal/
│   ├── eutils/          # NCBI E-utilities client
│   │   ├── client.go    # Rate-limited HTTP transport
│   │   ├── search.go    # ESearch
│   │   ├── fetch.go     # EFetch + XML parsing
│   │   ├── link.go      # ELink (citations, related)
│   │   └── types.go     # Domain types
│   ├── mesh/            # MeSH descriptor lookup
│   └── output/          # JSON / human / CSV formatters
├── testdata/            # Canned NCBI responses for unit tests
├── Makefile
└── go.mod
```

## 🧪 Development

```bash
make build           # Build binary
make test            # Unit tests (26 tests, offline)
make test-integration # Integration tests (real NCBI API)
make lint            # golangci-lint
make coverage        # Coverage report
```

Tests use `net/http/httptest` with fixture responses in `testdata/`. TDD throughout.

## 📄 License

MIT

## 🙏 Acknowledgments

Built on the [NCBI E-utilities API](https://www.ncbi.nlm.nih.gov/books/NBK25501/). Please respect their [usage guidelines](https://www.ncbi.nlm.nih.gov/books/NBK25497/).

---

<p align="center">
  <sub>Made with 🧬 for biomedical research</sub>
</p>
