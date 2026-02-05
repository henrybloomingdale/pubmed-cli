package ncbi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewBaseClient_Defaults(t *testing.T) {
	c := NewBaseClient()
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("expected base URL %q, got %q", DefaultBaseURL, c.BaseURL)
	}
	if c.Tool != DefaultTool {
		t.Errorf("expected tool %q, got %q", DefaultTool, c.Tool)
	}
	if c.Email != DefaultEmail {
		t.Errorf("expected email %q, got %q", DefaultEmail, c.Email)
	}
	if c.MaxBytes != DefaultMaxResponseBytes {
		t.Errorf("expected max bytes %d, got %d", DefaultMaxResponseBytes, c.MaxBytes)
	}
	if c.Limiter == nil {
		t.Error("expected non-nil limiter")
	}
}

func TestNewBaseClient_WithOptions(t *testing.T) {
	c := NewBaseClient(
		WithBaseURL("http://localhost:9999"),
		WithAPIKey("test-key-123"),
		WithTool("my-tool"),
		WithEmail("test@example.com"),
		WithMaxResponseBytes(1024),
	)
	if c.BaseURL != "http://localhost:9999" {
		t.Errorf("expected base URL %q, got %q", "http://localhost:9999", c.BaseURL)
	}
	if c.APIKey != "test-key-123" {
		t.Errorf("expected API key %q, got %q", "test-key-123", c.APIKey)
	}
	if c.Tool != "my-tool" {
		t.Errorf("expected tool %q, got %q", "my-tool", c.Tool)
	}
	if c.Email != "test@example.com" {
		t.Errorf("expected email %q, got %q", "test@example.com", c.Email)
	}
	if c.MaxBytes != 1024 {
		t.Errorf("expected max bytes 1024, got %d", c.MaxBytes)
	}
}

func TestDoGet_CommonParams(t *testing.T) {
	var receivedParams map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedParams = make(map[string]string)
		for k, v := range r.URL.Query() {
			receivedParams[k] = v[0]
		}
		w.Write([]byte(`OK`))
	}))
	defer srv.Close()

	c := NewBaseClient(
		WithBaseURL(srv.URL),
		WithAPIKey("my-api-key"),
		WithTool("pubmed-cli"),
		WithEmail("user@example.com"),
	)

	_, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedParams["api_key"] != "my-api-key" {
		t.Errorf("expected api_key %q, got %q", "my-api-key", receivedParams["api_key"])
	}
	if receivedParams["tool"] != "pubmed-cli" {
		t.Errorf("expected tool %q, got %q", "pubmed-cli", receivedParams["tool"])
	}
	if receivedParams["email"] != "user@example.com" {
		t.Errorf("expected email %q, got %q", "user@example.com", receivedParams["email"])
	}
}

func TestDoGet_RateLimitSequential(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rate limit test in short mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`OK`))
	}))
	defer srv.Close()

	// Client without API key: max 3 req/sec
	c := NewBaseClient(WithBaseURL(srv.URL))

	start := time.Now()
	for i := 0; i < 4; i++ {
		_, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}
	elapsed := time.Since(start)

	// 4 requests at 3/sec should take at least ~900ms (3 intervals of 333ms)
	if elapsed < 900*time.Millisecond {
		t.Errorf("rate limiting too fast: 4 requests completed in %v (expected >= 900ms)", elapsed)
	}
}

// TestDoGet_ConcurrentRateLimitNoKey is the critical concurrency test.
// It spins up 10 goroutines and asserts ≤4 requests in any 1-second window
// (rate=3/sec, burst=1 allows at most 4 due to timing).
func TestDoGet_ConcurrentRateLimitNoKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent rate limit test in short mode")
	}

	var mu sync.Mutex
	var timestamps []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		w.Write([]byte(`OK`))
	}))
	defer srv.Close()

	c := NewBaseClient(WithBaseURL(srv.URL)) // no API key = 3 req/sec

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
		}()
	}
	wg.Wait()

	if len(timestamps) != 10 {
		t.Fatalf("expected 10 requests, got %d", len(timestamps))
	}

	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})

	// With rate=3/sec and burst=1, no more than 4 requests should land
	// in any 1-second sliding window.
	for i := 0; i < len(timestamps); i++ {
		count := 1
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[j].Sub(timestamps[i]) < time.Second {
				count++
			}
		}
		if count > 4 {
			t.Errorf("rate limit violated: %d requests within 1 second starting at index %d (max 4 expected)", count, i)
			// Log timestamps for debugging
			for k, ts := range timestamps {
				t.Logf("  request %d: %v", k, ts.Sub(timestamps[0]))
			}
			break
		}
	}
}

// TestDoGet_ConcurrentRateLimitWithKey tests with API key (10 req/sec).
func TestDoGet_ConcurrentRateLimitWithKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent rate limit test in short mode")
	}

	var mu sync.Mutex
	var timestamps []time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		timestamps = append(timestamps, time.Now())
		mu.Unlock()
		w.Write([]byte(`OK`))
	}))
	defer srv.Close()

	c := NewBaseClient(WithBaseURL(srv.URL), WithAPIKey("test-key"))

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
		}()
	}
	wg.Wait()

	if len(timestamps) != 10 {
		t.Fatalf("expected 10 requests, got %d", len(timestamps))
	}

	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})

	// With rate=10/sec and burst=1, no more than 11 requests should land
	// in any 1-second window. With only 10 total requests, this should be fine.
	for i := 0; i < len(timestamps); i++ {
		count := 1
		for j := i + 1; j < len(timestamps); j++ {
			if timestamps[j].Sub(timestamps[i]) < time.Second {
				count++
			}
		}
		if count > 11 {
			t.Errorf("rate limit violated: %d requests within 1 second (max 11 expected)", count)
			break
		}
	}
}

func TestDoGet_ResponseTooLarge(t *testing.T) {
	// Server returns a response larger than MaxBytes
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write 2KB of data
		w.Write([]byte(strings.Repeat("X", 2048)))
	}))
	defer srv.Close()

	c := NewBaseClient(
		WithBaseURL(srv.URL),
		WithAPIKey("test"),
		WithMaxResponseBytes(1024), // 1KB limit
	)

	_, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
	if err == nil {
		t.Error("expected error for oversized response, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum size") {
		t.Errorf("expected 'exceeds maximum size' error, got: %v", err)
	}
}

func TestDoGet_ResponseWithinLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("small response"))
	}))
	defer srv.Close()

	c := NewBaseClient(
		WithBaseURL(srv.URL),
		WithAPIKey("test"),
		WithMaxResponseBytes(1024),
	)

	body, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(body) != "small response" {
		t.Errorf("expected 'small response', got %q", string(body))
	}
}

func TestDoGet_ContextCancellation(t *testing.T) {
	c := NewBaseClient(
		WithBaseURL("http://127.0.0.1:1"),
		WithAPIKey("test"),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.DoGet(ctx, "test.fcgi", make(map[string][]string))
	if err == nil {
		t.Error("expected error from cancelled context, got nil")
	}
}

func TestDoGet_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewBaseClient(WithBaseURL(srv.URL), WithAPIKey("test"))
	_, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestDoGet_HTTP429(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := NewBaseClient(WithBaseURL(srv.URL), WithAPIKey("test"))
	_, err := c.DoGet(context.Background(), "test.fcgi", make(map[string][]string))
	if err == nil {
		t.Error("expected error for HTTP 429, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("expected '429' in error message, got: %v", err)
	}
}

func TestDoGet_URLJoinPath(t *testing.T) {
	// Ensure trailing slash on base URL doesn't cause double-slash
	var receivedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Write([]byte(`OK`))
	}))
	defer srv.Close()

	// Base URL with trailing slash
	c := NewBaseClient(WithBaseURL(srv.URL+"/"), WithAPIKey("test"))
	_, err := c.DoGet(context.Background(), "esearch.fcgi", make(map[string][]string))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(receivedPath, "//") {
		t.Errorf("double slash in path: %q", receivedPath)
	}

	fmt.Println("received path:", receivedPath)
}
