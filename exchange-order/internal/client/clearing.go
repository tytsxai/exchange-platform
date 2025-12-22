package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClearingClient handles balance freeze/unfreeze requests.
type ClearingClient struct {
	baseURL string
	client  *http.Client
}

func NewClearingClient(baseURL string) *ClearingClient {
	return &ClearingClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type FreezeRequest struct {
	IdempotencyKey string `json:"IdempotencyKey"`
	UserID         int64  `json:"UserID"`
	Asset          string `json:"Asset"`
	Amount         int64  `json:"Amount"`
	RefType        string `json:"RefType"`
	RefID          string `json:"RefID"`
}

type FreezeResponse struct {
	Success   bool   `json:"Success"`
	ErrorCode string `json:"ErrorCode"`
}

type UnfreezeRequest struct {
	IdempotencyKey string `json:"IdempotencyKey"`
	UserID         int64  `json:"UserID"`
	Asset          string `json:"Asset"`
	Amount         int64  `json:"Amount"`
	RefType        string `json:"RefType"`
	RefID          string `json:"RefID"`
}

type UnfreezeResponse struct {
	Success   bool   `json:"Success"`
	ErrorCode string `json:"ErrorCode"`
}

func (c *ClearingClient) FreezeBalance(ctx context.Context, userID int64, asset string, amount int64, idempotencyKey string) (*FreezeResponse, error) {
	req := &FreezeRequest{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		Asset:          asset,
		Amount:         amount,
		RefType:        "ORDER",
		RefID:          idempotencyKey,
	}
	return c.postFreeze(ctx, "/internal/freeze", req)
}

func (c *ClearingClient) UnfreezeBalance(ctx context.Context, userID int64, asset string, amount int64, idempotencyKey string) (*UnfreezeResponse, error) {
	req := &UnfreezeRequest{
		IdempotencyKey: idempotencyKey,
		UserID:         userID,
		Asset:          asset,
		Amount:         amount,
		RefType:        "ORDER",
		RefID:          idempotencyKey,
	}
	return c.postUnfreeze(ctx, "/internal/unfreeze", req)
}

func (c *ClearingClient) postFreeze(ctx context.Context, path string, body interface{}) (*FreezeResponse, error) {
	respBody, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}

	var resp FreezeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

func (c *ClearingClient) postUnfreeze(ctx context.Context, path string, body interface{}) (*UnfreezeResponse, error) {
	respBody, err := c.post(ctx, path, body)
	if err != nil {
		return nil, err
	}

	var resp UnfreezeResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

func (c *ClearingClient) post(ctx context.Context, path string, body interface{}) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code: %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return respBody, nil
}
