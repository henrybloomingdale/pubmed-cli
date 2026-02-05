# CODE REVIEW — pubmed-cli

## Summary
Solid structure and a clean, testable layout, but there are **real correctness and compliance gaps**. The rate limiter is wrong under concurrency, MeSH requests ignore rate limiting entirely, and the XML parser misses common PubMed shapes (MedlineDate, collective authors, nested tags). You also have a few "paper cuts" like duplicated types and unused options.

**Grade: B-** (good base, but key correctness/robustness fixes needed before I'd call this production‑ready).

---

## Critical Issues (must fix)

1) **Rate limiting is incorrect under concurrency** — violates NCBI usage rules and can produce request bursts. **✅ FIXED**
   **File:** `internal/eutils/client.go:89–106`
   The current approach releases the mutex before sleeping. If multiple goroutines call `Search/Fetch/Link` concurrently, they all sleep in parallel and then issue a burst of requests when the timer fires. That defeats the 3/10 rps limit.

   **Fix applied:** Replaced hand-rolled rate limiter with `golang.org/x/time/rate.Limiter` in shared `internal/ncbi/client.go`. Added concurrency tests (10 goroutines, ≤4 req/sec without key, ≤11/sec with key).

---

## Major Suggestions (should fix)

1) **MeSH client has no rate limiting** (non‑compliant with NCBI policy). **✅ FIXED**
   **File:** `internal/mesh/mesh.go:34–148`
   This client can burst arbitrarily fast, which is explicitly disallowed.

   **Fix applied:** Created shared `internal/ncbi/client.go` with `BaseClient` struct. Both `eutils.Client` and `mesh.Client` now embed `*ncbi.BaseClient`, sharing the same rate limiter implementation. Architecture: `internal/ncbi/client.go` with `BaseClient`, rate limiter, common params, and response size guards.

2) **No response size limits — potential memory exhaustion / DoS**. **✅ FIXED**
   **Files:** `internal/eutils/client.go:139` and `internal/mesh/mesh.go:148`
   `io.ReadAll` on untrusted network responses is unbounded.

   **Fix applied:** `BaseClient.DoGet()` uses `io.LimitReader` with configurable max (default 50MB). Returns clear error "response exceeds maximum size" when exceeded. Tests added for both eutils and mesh clients.

3) **Publication‑type filters are malformed for multi‑word types**. **✅ FIXED**
   **File:** `cmd/pubmed/main.go:78–90`
   Strings like `clinical trial[pt]` and `randomized controlled trial[pt]` are not quoted.

   **Fix applied:** All publication types in the typeMap are now properly quoted: `"clinical trial"[pt]`, `"randomized controlled trial"[pt]`, etc. Custom types are also quoted. Comprehensive tests added in `cmd/pubmed/main_test.go`.

4) **XML parsing misses common PubMed shapes** (data loss). **✅ FIXED**
   **Files:** `internal/eutils/fetch.go:55–118`, `:75–85`
   - `PubDate` often uses `<MedlineDate>` instead of `<Year>`/`<Month>`.
   - Authors can be `<CollectiveName>` with no `LastName`/`ForeName`.
   - `<AbstractText>` and `<ArticleTitle>` commonly include nested tags.

   **Fix applied:** Added `MedlineDate` support with year extraction fallback. Added `CollectiveName` field to `Author` type with updated `FullName()`. Changed `ArticleTitle` and `AbstractText` to use `xml:",innerxml"` with tag stripping via `cleanInnerXML()`. Testdata fixtures and tests added for all three edge cases.

5) **Context propagation from CLI is ignored**. **✅ FIXED**
   **File:** `cmd/pubmed/main.go:129,147,165,183,201,220`
   All commands use `context.Background()` instead of `cmd.Context()`.

   **Fix applied:** All six command `RunE` functions now use `cmd.Context()` instead of `context.Background()`, enabling Ctrl‑C cancellation during rate‑limit waits and in‑flight HTTP calls.

6) **Search result count parse errors are silently dropped**. **✅ FIXED**
   **File:** `internal/eutils/search.go:64–65`
   If NCBI returns a non‑numeric count, you silently treat it as 0.

   **Fix applied:** Non-numeric count now returns a descriptive error: `parsing search result count "...": ...`. Empty string count gracefully returns 0. Test added.

---

## Minor Suggestions (nice to have)

1) **Duplicate MeSHRecord type + unused SearchOptions.PubType**. **✅ FIXED**
   **File:** `internal/eutils/types.go:76–93` vs `internal/mesh/mesh.go:15–23`

   **Fix applied:** Removed `MeSHRecord` from `internal/eutils/types.go` (only `mesh.MeSHRecord` remains). Removed unused `SearchOptions.PubType` field.

2) **ELink parsing only reads first LinkSetDB**. **✅ FIXED**
   **File:** `internal/eutils/link.go:87–94`

   **Fix applied:** Now filters `LinkSetDBs` by the requested `linkname` instead of blindly taking the first entry. Test added with multiple LinkSetDB entries.

3) **URL construction should use `url.JoinPath`**. **✅ FIXED**
   **Files:** `internal/eutils/client.go:119`, `internal/mesh/mesh.go:131`

   **Fix applied:** `BaseClient.DoGet()` uses `url.JoinPath` to avoid double slashes. Test added.

4) **Output formatting for missing year**. **✅ FIXED**
   **File:** `internal/output/format.go:71–83`
   Produces `Journal: XYZ ()` when `Year` is empty.

   **Fix applied:** Year parentheses only added when `Year` is non-empty. Test added.

---

## Positive Observations (what's done well)

- Clear package separation (`eutils`, `mesh`, `output`); responsibilities are easy to follow.
- Tests are thorough for happy paths + some failure modes, and fixtures are realistic in structure.
- Good use of `httptest` for deterministic unit tests.
- Integration tests are isolated behind build tags.
- JSON output is clean and pipe‑friendly with deterministic fields.

---

## Architecture Improvements Made

### Shared NCBI Base Client
Created `internal/ncbi/client.go` with `BaseClient` struct that provides:
- Proper token-bucket rate limiter via `golang.org/x/time/rate`
- Common NCBI parameter injection (api_key, tool, email)
- Response size guards via `io.LimitReader`
- URL construction via `url.JoinPath`

Both `eutils.Client` and `mesh.Client` now embed `*ncbi.BaseClient`, sharing rate limiting and common functionality. The `eutils` package re-exports `ncbi.Option` types for backward compatibility.

### Test Coverage Added
- Concurrent rate limiting (10 goroutines, sliding window assertion)
- Oversized response handling
- MedlineDate XML edge case
- CollectiveName XML edge case
- Nested tags in ArticleTitle/AbstractText
- Multi-word publication type quoting
- ELink linkname filtering with multiple LinkSetDBs
- Invalid search count parsing
- Empty year output formatting
