# CODE REVIEW — pubmed-cli

## Summary
Solid structure and a clean, testable layout, but there are **real correctness and compliance gaps**. The rate limiter is wrong under concurrency, MeSH requests ignore rate limiting entirely, and the XML parser misses common PubMed shapes (MedlineDate, collective authors, nested tags). You also have a few “paper cuts” like duplicated types and unused options.

**Grade: B-** (good base, but key correctness/robustness fixes needed before I’d call this production‑ready).

---

## Critical Issues (must fix)

1) **Rate limiting is incorrect under concurrency** — violates NCBI usage rules and can produce request bursts.  
   **File:** `internal/eutils/client.go:89–106`  
   The current approach releases the mutex before sleeping. If multiple goroutines call `Search/Fetch/Link` concurrently, they all sleep in parallel and then issue a burst of requests when the timer fires. That defeats the 3/10 rps limit.

   **Reproduction:**
   1. Create a client without API key.
   2. Spin up 10 goroutines calling `Search` against an `httptest.Server` that timestamps each request.
   3. Observe >3 requests within a single second.  

   **Fix:** Use a proper rate limiter (e.g., `golang.org/x/time/rate`) or serialize requests with a single shared token channel so only one request proceeds per interval.

---

## Major Suggestions (should fix)

1) **MeSH client has no rate limiting** (non‑compliant with NCBI policy).  
   **File:** `internal/mesh/mesh.go:34–148`  
   This client can burst arbitrarily fast, which is explicitly disallowed. Add the same rate limiter used by eutils (ideally shared implementation).

2) **No response size limits — potential memory exhaustion / DoS**.  
   **Files:** `internal/eutils/client.go:139` and `internal/mesh/mesh.go:148`  
   `io.ReadAll` on untrusted network responses is unbounded. A large response (or malicious proxy) can blow up memory. Use `io.LimitReader` + explicit max size. Add tests for oversized responses.

3) **Publication‑type filters are malformed for multi‑word types**.  
   **File:** `cmd/pubmed/main.go:78–90`  
   Strings like `clinical trial[pt]` and `randomized controlled trial[pt]` are not quoted. PubMed treats `[pt]` as applying only to the last term, changing the search semantics. This yields incorrect results. Use quotes: `"clinical trial"[pt]`, etc.

   **Reproduction:**
   - Run `pubmed search "asthma" --type trial --json` and inspect `query_translation`. You’ll see the split terms instead of a single publication type.

4) **XML parsing misses common PubMed shapes** (data loss).  
   **Files:** `internal/eutils/fetch.go:55–118`, `:75–85`  
   - `PubDate` often uses `<MedlineDate>` instead of `<Year>`/`<Month>`.
   - Authors can be `<CollectiveName>` with no `LastName`/`ForeName`.
   - `<AbstractText>` and `<ArticleTitle>` commonly include nested tags (`<i>`, `<sup>`, `<b>`). With `xml:",chardata"`, you lose content/formatting.

5) **Context propagation from CLI is ignored**.  
   **File:** `cmd/pubmed/main.go:129,147,165,183,201,220`  
   All commands use `context.Background()` instead of `cmd.Context()`. That prevents cancellation during rate‑limit waits or in‑flight HTTP calls (e.g., Ctrl‑C with Cobra). Use `cmd.Context()`.

6) **Search result count parse errors are silently dropped**.  
   **File:** `internal/eutils/search.go:64–65`  
   If NCBI returns a non‑numeric count, you silently treat it as 0. Wrap and surface the parsing error.

---

## Minor Suggestions (nice to have)

1) **Duplicate MeSHRecord type + unused SearchOptions.PubType**.  
   **File:** `internal/eutils/types.go:76–93` vs `internal/mesh/mesh.go:15–23`  
   This is dead weight and confusing. Either reuse a single type or delete the unused one. `SearchOptions.PubType` is never used.

2) **ELink parsing only reads first LinkSetDB**.  
   **File:** `internal/eutils/link.go:87–94`  
   If NCBI returns multiple `linksetdbs` entries, you silently ignore all but the first. Prefer filtering by `linkname` or merge.

3) **URL construction should use `url.JoinPath`**.  
   **Files:** `internal/eutils/client.go:119`, `internal/mesh/mesh.go:131`  
   Avoid double slashes if base URL ever has a trailing `/`.

4) **Output formatting for missing year**.  
   **File:** `internal/output/format.go:71–83`  
   Produces `Journal: XYZ ()` when `Year` is empty. Guard before adding parentheses.

---

## Positive Observations (what’s done well)

- Clear package separation (`eutils`, `mesh`, `output`); responsibilities are easy to follow.
- Tests are thorough for happy paths + some failure modes, and fixtures are realistic in structure.
- Good use of `httptest` for deterministic unit tests.
- Integration tests are isolated behind build tags.
- JSON output is clean and pipe‑friendly with deterministic fields.

---

## Suggested Refactors (with examples)

### 1) Shared NCBI client + proper rate limiter
Consolidate common HTTP + rate limit logic for `eutils` and `mesh`.

```go
// internal/ncbi/client.go
import "golang.org/x/time/rate"

type BaseClient struct {
    baseURL string
    http    *http.Client
    limiter *rate.Limiter
    apiKey, tool, email string
    maxBytes int64
}

func (c *BaseClient) get(ctx context.Context, endpoint string, params url.Values) ([]byte, error) {
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, err
    }
    if c.apiKey != "" { params.Set("api_key", c.apiKey) }
    if c.tool != ""   { params.Set("tool", c.tool) }
    if c.email != ""  { params.Set("email", c.email) }

    u, _ := url.JoinPath(c.baseURL, endpoint)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u+"?"+params.Encode(), nil)
    resp, err := c.http.Do(req)
    if err != nil { return nil, fmt.Errorf("request failed: %w", err) }
    defer resp.Body.Close()

    r := io.LimitReader(resp.Body, c.maxBytes+1)
    body, err := io.ReadAll(r)
    if err != nil { return nil, fmt.Errorf("read failed: %w", err) }
    if int64(len(body)) > c.maxBytes {
        return nil, fmt.Errorf("response too large")
    }
    return body, nil
}
```

### 2) Robust PubMed XML handling
Add support for `MedlineDate` and `CollectiveName`, and preserve nested tags.

```go
// Add MedlineDate and CollectiveName

type xmlPubDate struct {
    Year        string `xml:"Year"`
    Month       string `xml:"Month"`
    MedlineDate string `xml:"MedlineDate"`
}

type xmlAuthor struct {
    CollectiveName string `xml:"CollectiveName"`
    LastName       string `xml:"LastName"`
    ForeName       string `xml:"ForeName"`
    // ...
}
```

### 3) Centralize query building
Move CLI query building into a reusable builder or into `SearchOptions` to avoid duplicate date filtering.

```go
func BuildQuery(base string, opts SearchOptions) string {
    // append pub type + year filters consistently
}
```

---

## Testing Gaps

- **Concurrency rate‑limiting test** (simulate 10 concurrent calls; assert <= N per second).  
- **XML edge cases:** `MedlineDate`, `CollectiveName`, nested tags inside `AbstractText`.  
- **Oversized response handling** for both eutils and mesh clients.  
- **CLI query builder** tests for `--type` and `--year` behavior.

---

If you fix only one thing: **fix the rate limiter**. That’s the most likely to get you throttled or blocked by NCBI and it’s a correctness bug under concurrency.
