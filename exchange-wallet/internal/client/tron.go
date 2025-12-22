package client

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"time"
)

// TronClient TRON 网络客户端
type TronClient struct {
	nodeURL string
	apiKey  string
	client  *http.Client
}

// NewTronClient 创建 Tron 客户端
func NewTronClient(nodeURL, apiKey string) *TronClient {
	return &TronClient{
		nodeURL: strings.TrimRight(nodeURL, "/"),
		apiKey:  apiKey,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// TrxAccount 账户信息
type TrxAccount struct {
	Balance int64 `json:"balance"`
	// 其他字段按需添加
}

// GetAccount 获取账户信息（余额）
func (c *TronClient) GetAccount(address string) (*TrxAccount, error) {
	url := fmt.Sprintf("%s/wallet/getaccount", c.nodeURL)
	payload := fmt.Sprintf(`{"address": "%s", "visible": true}`, address)

	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tron api error: status %d", resp.StatusCode)
	}

	// TRON 有时返回空 JSON 代表没激活
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 || string(body) == "{}" {
		return &TrxAccount{Balance: 0}, nil
	}

	var account TrxAccount
	if err := json.Unmarshal(body, &account); err != nil {
		return nil, err
	}

	return &account, nil
}

// Transaction TRON 交易结构简略版
type Transaction struct {
	TxID    string `json:"txID"`
	RawData struct {
		Contract []struct {
			Parameter struct {
				Value struct {
					Amount       int64  `json:"amount"`
					OwnerAddress string `json:"owner_address"`
					ToAddress    string `json:"to_address"`
				} `json:"value"`
			} `json:"parameter"`
			Type string `json:"type"`
		} `json:"contract"`
		Timestamp int64 `json:"timestamp"`
	} `json:"raw_data"`
}

type TrxTransactionsResponse struct {
	Data    []Transaction `json:"data"`
	Success bool          `json:"success"`
}

// GetTransactions 获取最近交易 (使用 TronGrid V1 API)
func (c *TronClient) GetTransactions(address string, limit int) ([]Transaction, error) {
	// 注意：TronGrid V1 API 路径
	url := fmt.Sprintf("%s/v1/accounts/%s/transactions?limit=%d", c.nodeURL, address, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tron api error: status %d", resp.StatusCode)
	}

	var result TrxTransactionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

// TRC20Transfer TRC20 转账（TronGrid v1 简化版）
type TRC20Transfer struct {
	TransactionID  string `json:"transaction_id"`
	From           string `json:"from"`
	To             string `json:"to"`
	Value          string `json:"value"`
	BlockTimestamp int64  `json:"block_timestamp"`
	EventIndex     int    `json:"event_index"`
	TokenInfo      struct {
		Symbol   string `json:"symbol"`
		Decimals int    `json:"decimals"`
		Address  string `json:"address"`
	} `json:"token_info"`
}

type trc20TransfersResponse struct {
	Data    []TRC20Transfer `json:"data"`
	Success bool            `json:"success"`
}

// GetTRC20Transfers 获取 TRC20 转账记录（TronGrid V1）
// contractAddress 可为空；如不为空，会追加 contract_address 过滤条件。
func (c *TronClient) GetTRC20Transfers(address string, limit int, contractAddress string) ([]TRC20Transfer, error) {
	url := fmt.Sprintf("%s/v1/accounts/%s/transactions/trc20?limit=%d", c.nodeURL, address, limit)
	if contractAddress != "" {
		url += "&contract_address=" + contractAddress
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tron api error: status %d", resp.StatusCode)
	}

	var result trc20TransfersResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// TransactionInfo 交易信息（用于确认数计算）
type TransactionInfo struct {
	ID          string `json:"id"`
	BlockNumber int64  `json:"blockNumber"`
}

// GetTransactionInfo 获取交易信息（Tron full node API）
func (c *TronClient) GetTransactionInfo(txid string) (*TransactionInfo, error) {
	url := fmt.Sprintf("%s/wallet/gettransactioninfobyid", c.nodeURL)
	payload := fmt.Sprintf(`{"value": "%s"}`, txid)

	req, err := http.NewRequest("POST", url, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tron api error: status %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 || string(body) == "{}" {
		return nil, fmt.Errorf("tron tx info empty")
	}

	var info TransactionInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.BlockNumber == 0 {
		return nil, fmt.Errorf("tron tx info missing blockNumber")
	}
	return &info, nil
}

type nowBlockResponse struct {
	BlockHeader struct {
		RawData struct {
			Number int64 `json:"number"`
		} `json:"raw_data"`
	} `json:"block_header"`
}

// GetNowBlockNumber 获取当前区块高度（Tron full node API）
func (c *TronClient) GetNowBlockNumber() (int64, error) {
	url := fmt.Sprintf("%s/wallet/getnowblock", c.nodeURL)
	req, err := http.NewRequest("POST", url, strings.NewReader(`{}`))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("TRON-PRO-API-KEY", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("tron api error: status %d", resp.StatusCode)
	}

	var result nowBlockResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	if result.BlockHeader.RawData.Number == 0 {
		return 0, fmt.Errorf("tron now block missing number")
	}
	return result.BlockHeader.RawData.Number, nil
}

// ParseTRC20Value 将 TRC20 value 字符串转为 int64（最小单位）
func ParseTRC20Value(value string) (int64, error) {
	v := new(big.Int)
	if _, ok := v.SetString(value, 10); !ok {
		return 0, fmt.Errorf("invalid trc20 value: %q", value)
	}
	if v.Sign() < 0 {
		return 0, fmt.Errorf("invalid trc20 value (negative): %q", value)
	}
	if v.BitLen() > 63 {
		return 0, fmt.Errorf("trc20 value overflow: %q", value)
	}
	n := v.Int64()
	return n, nil
}
