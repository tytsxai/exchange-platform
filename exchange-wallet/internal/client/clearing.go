package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type ClearingClient struct {
	baseURL       string
	internalToken string
	client        *http.Client
}

func NewClearingClient(baseURL, internalToken string) *ClearingClient {
	return &ClearingClient{
		baseURL:       baseURL,
		internalToken: internalToken,
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

func (c *ClearingClient) Freeze(ctx context.Context, req *FreezeRequest) error {
	return c.post(ctx, "/internal/freeze", req)
}

type UnfreezeRequest struct {
	IdempotencyKey string `json:"IdempotencyKey"`
	UserID         int64  `json:"UserID"`
	Asset          string `json:"Asset"`
	Amount         int64  `json:"Amount"`
	RefType        string `json:"RefType"`
	RefID          string `json:"RefID"`
}

func (c *ClearingClient) Unfreeze(ctx context.Context, req *UnfreezeRequest) error {
	return c.post(ctx, "/internal/unfreeze", req)
}

type DeductRequest struct {
	IdempotencyKey string `json:"IdempotencyKey"`
	UserID         int64  `json:"UserID"`
	Asset          string `json:"Asset"`
	Amount         int64  `json:"Amount"`
	RefType        string `json:"RefType"`
	RefID          string `json:"RefID"`
}

func (c *ClearingClient) Deduct(ctx context.Context, req *DeductRequest) error {
	return c.post(ctx, "/internal/deduct", req)
}

type CreditRequest struct {
	IdempotencyKey string `json:"IdempotencyKey"`
	UserID         int64  `json:"UserID"`
	Asset          string `json:"Asset"`
	Amount         int64  `json:"Amount"`
	RefType        string `json:"RefType"`
	RefID          string `json:"RefID"`
}

func (c *ClearingClient) Credit(ctx context.Context, req *CreditRequest) error {
	return c.post(ctx, "/internal/credit", req)
}

func (c *ClearingClient) post(ctx context.Context, path string, body interface{}) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.internalToken != "" {
		req.Header.Set("X-Internal-Token", c.internalToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code: %d", resp.StatusCode)
	}

	var result struct {
		Success   bool   `json:"Success"`
		ErrorCode string `json:"ErrorCode"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("clearing error: %s", result.ErrorCode)
	}

	return nil
}
