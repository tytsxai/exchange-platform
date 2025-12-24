// Package client matching engine http client
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	cacheTTL       = time.Second
	defaultTimeout = 2 * time.Second
)

var ErrNoReferencePrice = errors.New("no reference price")

// MatchingClient 调用撮合引擎接口
type MatchingClient struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	price     int64
	expiresAt time.Time
}

// NewMatchingClient 创建撮合客户端
func NewMatchingClient(baseURL, internalToken string) *MatchingClient {
	return &MatchingClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		internalToken: internalToken,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cache: make(map[string]cacheEntry),
	}
}

// GetLastPrice 获取参考价
func (c *MatchingClient) GetLastPrice(symbol string) (int64, error) {
	if symbol == "" {
		return 0, errors.New("symbol required")
	}

	now := time.Now()
	if price, ok := c.getCached(symbol, now); ok {
		return price, nil
	}

	url := fmt.Sprintf("%s/depth?symbol=%s", c.baseURL, symbol)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	if c.internalToken != "" {
		req.Header.Set("X-Internal-Token", c.internalToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("get depth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("depth status: %d", resp.StatusCode)
	}

	var payload depthResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode depth: %w", err)
	}

	// 允许单边报价作为参考价格
	if len(payload.Bids) == 0 && len(payload.Asks) == 0 {
		return 0, ErrNoReferencePrice
	}

	var ref int64
	if len(payload.Bids) > 0 && len(payload.Asks) > 0 {
		// 双边报价：取中间价
		ref = (payload.Bids[0].Price + payload.Asks[0].Price) / 2
	} else if len(payload.Bids) > 0 {
		// 只有买单：用最高买价
		ref = payload.Bids[0].Price
	} else {
		// 只有卖单：用最低卖价
		ref = payload.Asks[0].Price
	}

	c.setCached(symbol, ref, now.Add(cacheTTL))
	return ref, nil
}

func (c *MatchingClient) getCached(symbol string, now time.Time) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[symbol]
	if !ok || now.After(entry.expiresAt) {
		return 0, false
	}
	return entry.price, true
}

func (c *MatchingClient) setCached(symbol string, price int64, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[symbol] = cacheEntry{
		price:     price,
		expiresAt: expiresAt,
	}
}

type depthResponse struct {
	Bids []priceLevel `json:"bids"`
	Asks []priceLevel `json:"asks"`
}

type priceLevel struct {
	Price int64 `json:"price"`
	Qty   int64 `json:"qty"`
}
