# Changelog

All notable changes to pubmed-cli will be documented in this file.

## [0.3.0] - 2026-02-07

### Added
- **`pubmed qa` command** — Answer biomedical yes/no questions with adaptive retrieval
  - Novelty detection (scans for 2024+ year patterns and recency keywords)
  - Confidence-gated retrieval (only fetches from PubMed when model is uncertain)
  - Abstract minification (extracts key sentences, ~74% token savings)
  - `--explain` flag for reasoning trace with sources
  - `--retrieve` / `--parametric` flags to force strategy
  - `--claude` flag for Claude CLI integration
- LLM client abstraction (`internal/llm/`) supporting OpenAI-compatible APIs
- Adaptive retrieval engine (`internal/qa/`) with configurable confidence threshold
- `make release` target for cross-compilation

### Changed
- README completely rewritten with comprehensive documentation
- Added Homebrew installation instructions

## [0.2.0] - 2026-02-05

### Fixed
- ELink commands (`cited-by`, `references`, `related`) now correctly parse NCBI JSON format
- MeSH lookup uses `esummary` JSON instead of broken `efetch` text parser

### Changed
- Improved documentation with badges and architecture section

## [0.1.1] - 2026-02-05

### Fixed
- Rate limiter now uses `golang.org/x/time/rate` for correct concurrent behavior
- MeSH client shares rate limiter with eutils (NCBI compliance)
- Response size limits to prevent memory exhaustion
- XML parsing handles `MedlineDate`, `CollectiveName`, and nested tags
- Context propagation from CLI commands (enables Ctrl-C cancellation)
- Publication type filters properly quoted for multi-word types

## [0.1.0] - 2026-02-04

### Added
- Initial release
- 6 commands: `search`, `fetch`, `cited-by`, `references`, `related`, `mesh`
- JSON and human-readable output modes
- NCBI API key support
- Rate limiting (3 req/s default, 10 with API key)
- Year and publication type filters
