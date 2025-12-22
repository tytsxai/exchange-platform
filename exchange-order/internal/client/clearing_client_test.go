package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClearingClient_FreezeBalance(t *testing.T) {
	var got FreezeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/freeze" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Success":   true,
			"ErrorCode": "",
		})
	}))
	defer server.Close()

	c := NewClearingClient(server.URL)
	resp, err := c.FreezeBalance(context.Background(), 10, "USDT", 100, "freeze:order:1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}
	if got.UserID != 10 || got.Asset != "USDT" || got.Amount != 100 {
		t.Fatalf("unexpected request payload: %+v", got)
	}
	if got.IdempotencyKey != "freeze:order:1" {
		t.Fatalf("unexpected idempotency key: %s", got.IdempotencyKey)
	}
}

func TestClearingClient_UnfreezeBalance(t *testing.T) {
	var got UnfreezeRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/unfreeze" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"Success":   true,
			"ErrorCode": "",
		})
	}))
	defer server.Close()

	c := NewClearingClient(server.URL)
	resp, err := c.UnfreezeBalance(context.Background(), 22, "BTC", 5, "unfreeze:order:9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}
	if got.UserID != 22 || got.Asset != "BTC" || got.Amount != 5 {
		t.Fatalf("unexpected request payload: %+v", got)
	}
	if got.IdempotencyKey != "unfreeze:order:9" {
		t.Fatalf("unexpected idempotency key: %s", got.IdempotencyKey)
	}
}

func TestClearingClient_FreezeBalance_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := NewClearingClient(server.URL)
	if _, err := c.FreezeBalance(context.Background(), 10, "USDT", 100, "freeze:order:1"); err == nil {
		t.Fatal("expected status error")
	}
}

func TestClearingClient_FreezeBalance_DecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{"))
	}))
	defer server.Close()

	c := NewClearingClient(server.URL)
	if _, err := c.FreezeBalance(context.Background(), 10, "USDT", 100, "freeze:order:1"); err == nil {
		t.Fatal("expected decode error")
	}
}

func TestClearingClient_Post_MarshalError(t *testing.T) {
	c := NewClearingClient("http://127.0.0.1")
	if _, err := c.post(context.Background(), "/internal/freeze", make(chan int)); err == nil {
		t.Fatal("expected marshal error")
	}
}
