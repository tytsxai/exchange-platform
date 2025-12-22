// Package signature API 签名验证工具
package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	// 默认时间窗口（30秒）
	DefaultTimeWindow = 30 * time.Second
)

// Signer 签名器
type Signer struct {
	secret []byte
}

// NewSigner 创建签名器
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// Sign 生成签名
func (s *Signer) Sign(canonicalString string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(canonicalString))
	return hex.EncodeToString(h.Sum(nil))
}

// Verify 验证签名
func (s *Signer) Verify(canonicalString, signature string) bool {
	expected := s.Sign(canonicalString)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// BuildCanonicalString 构建规范字符串
// 格式：timestampMs\nnonce\nmethod\npath\ncanonicalQuery
// body 参数预留（当前签名不包含 body）
func BuildCanonicalString(timestampMs int64, nonce, method, path string, query url.Values, body []byte) string {
	parts := []string{
		fmt.Sprintf("%d", timestampMs),
		nonce,
		strings.ToUpper(method),
		path,
		canonicalQuery(query),
	}
	return strings.Join(parts, "\n")
}

// canonicalQuery 构建规范查询字符串（按 key 排序）
func canonicalQuery(query url.Values) string {
	if len(query) == 0 {
		return ""
	}

	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		values := query[k]
		sort.Strings(values)
		for _, v := range values {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(pairs, "&")
}

// Verifier 签名验证器
type Verifier struct {
	signer     *Signer
	timeWindow time.Duration
	nonceStore NonceStore
}

// NonceStore nonce 存储接口（用于防重放）
type NonceStore interface {
	// Exists 检查 nonce 是否存在，如果不存在则存储并返回 false
	// 返回 true 表示 nonce 已存在（重放攻击）
	Exists(apiKey, nonce string, expireAt time.Time) (bool, error)
}

// VerifierOption 验证器选项
type VerifierOption func(*Verifier)

// WithTimeWindow 设置时间窗口
func WithTimeWindow(d time.Duration) VerifierOption {
	return func(v *Verifier) {
		v.timeWindow = d
	}
}

// WithNonceStore 设置 nonce 存储
func WithNonceStore(store NonceStore) VerifierOption {
	return func(v *Verifier) {
		v.nonceStore = store
	}
}

// NewVerifier 创建验证器
func NewVerifier(secret string, opts ...VerifierOption) *Verifier {
	v := &Verifier{
		signer:     NewSigner(secret),
		timeWindow: DefaultTimeWindow,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// VerifyRequest 验证请求
func (v *Verifier) VerifyRequest(req *Request) error {
	// 1. 验证时间戳
	now := time.Now().UnixMilli()
	diff := now - req.TimestampMs
	if diff < 0 {
		diff = -diff
	}
	if diff > v.timeWindow.Milliseconds() {
		return ErrInvalidTimestamp
	}

	// 2. 验证 nonce（防重放）
	if v.nonceStore != nil {
		expireAt := time.Now().Add(v.timeWindow * 2)
		exists, err := v.nonceStore.Exists(req.ApiKey, req.Nonce, expireAt)
		if err != nil {
			return fmt.Errorf("nonce store error: %w", err)
		}
		if exists {
			return ErrNonceReused
		}
	}

	// 3. 验证签名
	canonical := BuildCanonicalString(
		req.TimestampMs,
		req.Nonce,
		req.Method,
		req.Path,
		req.Query,
		req.Body,
	)
	if !v.signer.Verify(canonical, req.Signature) {
		return ErrInvalidSignature
	}

	return nil
}

// Request 待验证的请求
type Request struct {
	ApiKey      string
	TimestampMs int64
	Nonce       string
	Signature   string
	Method      string
	Path        string
	Query       url.Values
	Body        []byte
}

// 错误定义
var (
	ErrInvalidTimestamp = fmt.Errorf("invalid timestamp")
	ErrNonceReused      = fmt.Errorf("nonce reused")
	ErrInvalidSignature = fmt.Errorf("invalid signature")
)

// GenerateNonce 生成 nonce（示例实现）
func GenerateNonce() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
