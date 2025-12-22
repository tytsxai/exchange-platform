package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestPriceMatchingClient_GetLastPrice(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		resp := map[string]interface{}{
			"bids": []map[string]int64{
				{"price": 100 * 1e8, "qty": 1},
			},
			"asks": []map[string]int64{
				{"price": 110 * 1e8, "qty": 2},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewMatchingClient(server.URL)

	price, err := client.GetLastPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := int64(105 * 1e8)
	if price != expected {
		t.Fatalf("expected %d, got %d", expected, price)
	}

	_, err = client.GetLastPrice("BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected cache hit, got %d http calls", hits)
	}
}

func TestPriceMatchingClient_GetLastPrice_NoDepth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"bids": []map[string]int64{},
			"asks": []map[string]int64{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewMatchingClient(server.URL)
	if _, err := client.GetLastPrice("BTCUSDT"); err == nil {
		t.Fatal("expected error for empty orderbook")
	}
}

func TestPriceMatchingClient_GetLastPrice_HTTPStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewMatchingClient(server.URL)
	if _, err := client.GetLastPrice("BTCUSDT"); err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestPriceMatchingClient_GetLastPrice_CacheExpiry(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		resp := map[string]interface{}{
			"bids": []map[string]int64{
				{"price": 100 * 1e8, "qty": 1},
			},
			"asks": []map[string]int64{
				{"price": 110 * 1e8, "qty": 2},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewMatchingClient(server.URL)
	if _, err := client.GetLastPrice("BTCUSDT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	client.mu.Lock()
	entry := client.cache["BTCUSDT"]
	entry.expiresAt = time.Now().Add(-time.Millisecond)
	client.cache["BTCUSDT"] = entry
	client.mu.Unlock()

	if _, err := client.GetLastPrice("BTCUSDT"); err != nil {
		t.Fatalf("unexpected error after expiry: %v", err)
	}
	if atomic.LoadInt32(&hits) != 2 {
		t.Fatalf("expected refresh after expiry, got %d http calls", hits)
	}
}

func TestPriceMatchingClient_GetLastPrice_ConcurrentCache(t *testing.T) {
	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		resp := map[string]interface{}{
			"bids": []map[string]int64{
				{"price": 100 * 1e8, "qty": 1},
			},
			"asks": []map[string]int64{
				{"price": 110 * 1e8, "qty": 2},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewMatchingClient(server.URL)
	if _, err := client.GetLastPrice("BTCUSDT"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const workers = 20
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			_, err := client.GetLastPrice("BTCUSDT")
			errCh <- err
		}()
	}
	for i := 0; i < workers; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("unexpected error in concurrent call: %v", err)
		}
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected cached responses only, got %d http calls", hits)
	}
}

func TestPriceMatchingClient_GetLastPrice_HTTPError(t *testing.T) {
	client := NewMatchingClient("http://127.0.0.1:0")
	if _, err := client.GetLastPrice("BTCUSDT"); err == nil {
		t.Fatal("expected http error")
	}
}

func TestPriceMatchingClient_GetLastPrice_SymbolRequired(t *testing.T) {
	client := NewMatchingClient("http://127.0.0.1")
	if _, err := client.GetLastPrice(""); err == nil || err.Error() != "symbol required" {
		t.Fatalf("expected symbol required error, got %v", err)
	}
}
