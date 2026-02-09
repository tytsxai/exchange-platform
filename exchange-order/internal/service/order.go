// Package service 订单服务
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	commondecimal "github.com/exchange/common/pkg/decimal"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/metrics"
	"github.com/exchange/order/internal/repository"
	"github.com/redis/go-redis/v9"
)

// OrderService 订单服务
type OrderService struct {
	repo        OrderStore
	redis       *redis.Client
	idGen       IDGenerator
	orderStream string
	validator   *PriceValidator
	clearing    *client.ClearingClient
	metrics     *metrics.Metrics
	publisher   orderPublisher
}

type orderPublisher interface {
	PublishOrderCreated(ctx context.Context, userID int64, order interface{}) error
	PublishOrderEvent(ctx context.Context, userID int64, event string, order interface{}) error
}

// OrderStore 订单数据接口
type OrderStore interface {
	GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
	GetOrderByClientID(ctx context.Context, userID int64, clientOrderID string) (*repository.Order, error)
	CreateOrder(ctx context.Context, order *repository.Order) error
	GetOrder(ctx context.Context, orderID int64) (*repository.Order, error)
	UpdateOrderStatus(ctx context.Context, orderID int64, status int, executedQty, cumulativeQuoteQty, updateTimeMs int64) error
	RejectOrder(ctx context.Context, orderID int64, reason string, updateTimeMs int64) error
	ListOpenOrders(ctx context.Context, userID int64, symbol string, limit int) ([]*repository.Order, error)
	ListOrders(ctx context.Context, userID int64, symbol string, startTime, endTime int64, limit int) ([]*repository.Order, error)
	ListSymbolConfigs(ctx context.Context) ([]*repository.SymbolConfig, error)
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// NewOrderService 创建订单服务
func NewOrderService(repo OrderStore, redisClient *redis.Client, idGen IDGenerator, orderStream string, validator *PriceValidator, clearingClient *client.ClearingClient, metricsClient *metrics.Metrics) *OrderService {
	return &OrderService{
		repo:        repo,
		redis:       redisClient,
		idGen:       idGen,
		orderStream: orderStream,
		validator:   validator,
		clearing:    clearingClient,
		metrics:     metricsClient,
	}
}

func (s *OrderService) SetPublisher(publisher orderPublisher) {
	s.publisher = publisher
}

// CreateOrderRequest 下单请求
type CreateOrderRequest struct {
	UserID        int64
	Symbol        string
	Side          string // BUY / SELL
	Type          string // LIMIT / MARKET
	TimeInForce   string // GTC / IOC / FOK / POST_ONLY
	Price         int64
	Quantity      int64
	QuoteOrderQty int64 // 市价买单：花多少钱
	ClientOrderID string
}

// CreateOrderResponse 下单响应
type CreateOrderResponse struct {
	Order     *repository.Order
	ErrorCode string
}

// CreateOrder 创建订单
func (s *OrderService) CreateOrder(ctx context.Context, req *CreateOrderRequest) (*CreateOrderResponse, error) {
	start := time.Now()
	if s.metrics != nil {
		defer func() { s.metrics.ObserveOrderLatency(time.Since(start)) }()
	}
	if req == nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INVALID_PARAM")
		}
		return &CreateOrderResponse{ErrorCode: "INVALID_PARAM"}, nil
	}

	normalizeCreateOrderRequest(req)

	reject := func(code string) *CreateOrderResponse {
		if s.metrics != nil && code != "" {
			s.metrics.IncOrderRejected(code)
		}
		return &CreateOrderResponse{ErrorCode: code}
	}

	// 1. 获取交易对配置
	cfg, err := s.repo.GetSymbolConfig(ctx, req.Symbol)
	if err != nil {
		return reject("SYMBOL_NOT_FOUND"), nil
	}

	// 2. 检查交易对状态
	if cfg.Status != 1 {
		return reject("SYMBOL_NOT_TRADING"), nil
	}

	// 3. 参数校验
	if err := s.validateOrder(req, cfg); err != nil {
		return reject(err.Error()), nil
	}

	// 4. 幂等检查
	if req.ClientOrderID != "" {
		existing, err := s.repo.GetOrderByClientID(ctx, req.UserID, req.ClientOrderID)
		if err == nil && existing != nil {
			if code, err := s.ensureOrderReady(ctx, existing, cfg); err != nil {
				return nil, err
			} else if code != "" {
				return reject(code), nil
			}
			return &CreateOrderResponse{Order: existing}, nil
		}
	}

	// 5. 价格保护（仅限价单）
	if req.Type == "LIMIT" && s.validator != nil {
		if err := s.validator.ValidatePrice(req.Symbol, req.Side, req.Price); err != nil {
			return reject(err.Error()), nil
		}
	}

	now := time.Now().UnixMilli()
	tif := parseTIF(req.TimeInForce)
	if req.Type == "MARKET" && tif == 1 {
		tif = 2 // MARKET 默认按 IOC 处理，避免挂单
	}
	order := &repository.Order{
		OrderID:            s.idGen.NextID(),
		ClientOrderID:      req.ClientOrderID,
		UserID:             req.UserID,
		Symbol:             req.Symbol,
		Side:               parseSide(req.Side),
		Type:               parseType(req.Type),
		TimeInForce:        tif,
		Price:              strconv.FormatInt(req.Price, 10),
		StopPrice:          "0",
		OrigQty:            strconv.FormatInt(req.Quantity, 10),
		ExecutedQty:        "0",
		CumulativeQuoteQty: "0",
		Status:             repository.StatusInit,
		CreateTimeMs:       now,
		UpdateTimeMs:       now,
	}

	// 6. 计算冻结金额
	var freezeAsset string
	var freezeAmount int64
	if order.Side == repository.SideBuy {
		freezeAsset = cfg.QuoteAsset
		if order.Type == repository.TypeMarket {
			bufferedPrice, quoteAmount, err := s.marketBuyQuoteAmount(ctx, req.Symbol, req.Quantity, cfg)
			if err != nil {
				return reject("NO_REFERENCE_PRICE"), nil
			}
			order.Price = strconv.FormatInt(bufferedPrice, 10)
			freezeAmount = quoteAmount
		} else {
			freezeAmount = quoteQty(req.Price, req.Quantity, cfg.QtyPrecision)
		}
	} else {
		freezeAsset = cfg.BaseAsset
		freezeAmount = req.Quantity
	}
	if s.clearing == nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INTERNAL_ERROR")
		}
		return nil, fmt.Errorf("clearing client not configured")
	}

	// 7. 保存订单（先落库，再冻结）
	if err := s.repo.CreateOrder(ctx, order); err != nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INTERNAL_ERROR")
		}
		if errors.Is(err, repository.ErrDuplicateClientOrderID) && req.ClientOrderID != "" {
			existing, fetchErr := s.repo.GetOrderByClientID(ctx, req.UserID, req.ClientOrderID)
			if fetchErr == nil && existing != nil {
				if code, err := s.ensureOrderReady(ctx, existing, cfg); err != nil {
					return nil, err
				} else if code != "" {
					return reject(code), nil
				}
				return &CreateOrderResponse{Order: existing}, nil
			}
		}
		return nil, fmt.Errorf("create order: %w", err)
	}

	// 8. 调用清算服务冻结资金（幂等键基于 orderId）
	freezeKey := fmt.Sprintf("freeze:order:%d", order.OrderID)
	freezeResp, err := s.clearing.FreezeBalance(ctx, order.UserID, freezeAsset, freezeAmount, freezeKey)
	if err != nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INTERNAL_ERROR")
		}
		return nil, fmt.Errorf("freeze balance: %w", err)
	}
	if freezeResp == nil || !freezeResp.Success {
		code := "FREEZE_FAILED"
		if freezeResp != nil && freezeResp.ErrorCode != "" {
			code = freezeResp.ErrorCode
		}
		if rejectErr := s.rejectOrder(ctx, order.OrderID, code); rejectErr != nil {
			return nil, fmt.Errorf("reject order: %w", rejectErr)
		}
		return reject(code), nil
	}

	// 9. 更新订单状态为 NEW
	updateTime := time.Now().UnixMilli()
	if err := s.repo.UpdateOrderStatus(ctx, order.OrderID, repository.StatusNew, 0, 0, updateTime); err != nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INTERNAL_ERROR")
		}
		if rollbackErr := s.compensateCreateOrderFailure(ctx, order, freezeAsset, freezeAmount, "update_status_failed"); rollbackErr != nil {
			log.Printf("compensate create order failure (update status) error: %v", rollbackErr)
		}
		return nil, fmt.Errorf("update order status: %w", err)
	}
	order.Status = repository.StatusNew
	order.UpdateTimeMs = updateTime

	// 10. 发送到撮合队列
	if err := s.sendToMatchingWithRetry(ctx, order); err != nil {
		if s.metrics != nil {
			s.metrics.IncOrderRejected("INTERNAL_ERROR")
		}
		if rollbackErr := s.compensateCreateOrderFailure(ctx, order, freezeAsset, freezeAmount, "send_matching_failed"); rollbackErr != nil {
			log.Printf("compensate create order failure (send matching) error: %v", rollbackErr)
		}
		return nil, fmt.Errorf("send to matching: %w", err)
	}

	if s.metrics != nil {
		s.metrics.IncOrderCreated(order.Symbol, sideToString(order.Side))
	}
	if s.publisher != nil {
		if err := s.publisher.PublishOrderCreated(ctx, order.UserID, order); err != nil {
			log.Printf("publish order created error: %v", err)
		}
	}

	return &CreateOrderResponse{Order: order}, nil
}

func (s *OrderService) sendToMatchingWithRetry(ctx context.Context, order *repository.Order) error {
	const maxAttempts = 3
	backoff := 50 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.sendToMatching(ctx, order); err == nil {
			return nil
		} else {
			lastErr = err
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if backoff < 500*time.Millisecond {
			backoff *= 2
		}
	}
	return lastErr
}

func (s *OrderService) ensureOrderReady(ctx context.Context, order *repository.Order, cfg *repository.SymbolConfig) (string, error) {
	if order == nil {
		return "", nil
	}
	switch order.Status {
	case repository.StatusInit:
		if s.clearing == nil {
			return "", fmt.Errorf("clearing client not configured")
		}
		freezeAsset, freezeAmount, err := s.freezeSpecFromOrder(order, cfg)
		if err != nil {
			return "", err
		}
		freezeKey := fmt.Sprintf("freeze:order:%d", order.OrderID)
		freezeResp, err := s.clearing.FreezeBalance(ctx, order.UserID, freezeAsset, freezeAmount, freezeKey)
		if err != nil {
			return "", fmt.Errorf("freeze balance: %w", err)
		}
		if freezeResp == nil || !freezeResp.Success {
			code := "FREEZE_FAILED"
			if freezeResp != nil && freezeResp.ErrorCode != "" {
				code = freezeResp.ErrorCode
			}
			if err := s.rejectOrder(ctx, order.OrderID, code); err != nil {
				return "", fmt.Errorf("reject order: %w", err)
			}
			return code, nil
		}
		updateTime := time.Now().UnixMilli()
		if err := s.repo.UpdateOrderStatus(ctx, order.OrderID, repository.StatusNew, 0, 0, updateTime); err != nil {
			return "", fmt.Errorf("update order status: %w", err)
		}
		order.Status = repository.StatusNew
		order.UpdateTimeMs = updateTime
	}

	if order.Status == repository.StatusNew && s.redis != nil {
		// best-effort re-send; matching side will dedupe.
		_ = s.sendToMatchingWithRetry(ctx, order)
	}

	return "", nil
}

func (s *OrderService) freezeSpecFromOrder(order *repository.Order, cfg *repository.SymbolConfig) (string, int64, error) {
	if order == nil || cfg == nil {
		return "", 0, fmt.Errorf("invalid order/config")
	}
	if order.Side == repository.SideBuy {
		price, err := parseInt64Compat(order.Price, "price")
		if err != nil {
			return "", 0, err
		}
		qty, err := parseInt64Compat(order.OrigQty, "orig_qty")
		if err != nil {
			return "", 0, err
		}
		return cfg.QuoteAsset, quoteQty(price, qty, cfg.QtyPrecision), nil
	}
	qty, err := parseInt64Compat(order.OrigQty, "orig_qty")
	if err != nil {
		return "", 0, err
	}
	return cfg.BaseAsset, qty, nil
}

func (s *OrderService) rejectOrder(ctx context.Context, orderID int64, reason string) error {
	if reason == "" {
		reason = "INTERNAL_ERROR"
	}
	err := s.repo.RejectOrder(ctx, orderID, reason, time.Now().UnixMilli())
	if err != nil && !errors.Is(err, repository.ErrOrderNotFound) {
		return err
	}
	return nil
}

func (s *OrderService) rollbackFreeze(ctx context.Context, order *repository.Order, asset string, amount int64, reason string) error {
	if s.clearing == nil {
		return fmt.Errorf("clearing client not configured")
	}
	if amount <= 0 {
		return nil
	}
	key := fmt.Sprintf("unfreeze:order:%d:%s", order.OrderID, reason)
	resp, err := s.clearing.UnfreezeBalance(ctx, order.UserID, asset, amount, key)
	if err != nil {
		return err
	}
	if resp != nil && !resp.Success {
		return fmt.Errorf("unfreeze failed: %s", resp.ErrorCode)
	}
	return nil
}

func (s *OrderService) compensateCreateOrderFailure(ctx context.Context, order *repository.Order, asset string, amount int64, rollbackReason string) error {
	if order == nil {
		return nil
	}

	var errs []string
	if err := s.rollbackFreeze(ctx, order, asset, amount, rollbackReason); err != nil {
		errs = append(errs, fmt.Sprintf("rollback freeze: %v", err))
	}
	if err := s.rejectOrder(ctx, order.OrderID, "INTERNAL_ERROR"); err != nil {
		errs = append(errs, fmt.Sprintf("reject order: %v", err))
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	order.Status = repository.StatusRejected
	order.UpdateTimeMs = time.Now().UnixMilli()
	return nil
}

// CancelOrderRequest 撤单请求
type CancelOrderRequest struct {
	UserID        int64
	Symbol        string
	OrderID       int64
	ClientOrderID string
}

// CancelOrderResponse 撤单响应
type CancelOrderResponse struct {
	Order     *repository.Order
	ErrorCode string
}

// CancelOrder 撤销订单
func (s *OrderService) CancelOrder(ctx context.Context, req *CancelOrderRequest) (*CancelOrderResponse, error) {
	// 1. 获取订单
	var order *repository.Order
	var err error
	if req.OrderID > 0 {
		order, err = s.repo.GetOrder(ctx, req.OrderID)
	} else if req.ClientOrderID != "" {
		order, err = s.repo.GetOrderByClientID(ctx, req.UserID, req.ClientOrderID)
	} else {
		return &CancelOrderResponse{ErrorCode: "INVALID_PARAM"}, nil
	}

	if err != nil {
		return &CancelOrderResponse{ErrorCode: "ORDER_NOT_FOUND"}, nil
	}

	// 2. 检查权限
	if order.UserID != req.UserID {
		return &CancelOrderResponse{ErrorCode: "ORDER_NOT_FOUND"}, nil
	}

	// 3. 检查状态
	if order.Status != repository.StatusNew && order.Status != repository.StatusPartiallyFilled {
		if order.Status == repository.StatusCanceled {
			return &CancelOrderResponse{Order: order}, nil // 幂等
		}
		return &CancelOrderResponse{ErrorCode: "ORDER_ALREADY_FILLED"}, nil
	}

	// 4. 发送撤单到撮合
	if err := s.sendCancelToMatching(ctx, order); err != nil {
		return nil, fmt.Errorf("send cancel to matching: %w", err)
	}

	return &CancelOrderResponse{Order: order}, nil
}

// GetOrder 获取订单
func (s *OrderService) GetOrder(ctx context.Context, userID, orderID int64) (*repository.Order, error) {
	order, err := s.repo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, repository.ErrOrderNotFound
	}
	return order, nil
}

// ListOpenOrders 查询当前委托
func (s *OrderService) ListOpenOrders(ctx context.Context, userID int64, symbol string, limit int) ([]*repository.Order, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.repo.ListOpenOrders(ctx, userID, symbol, limit)
}

// ListOrders 查询历史订单
func (s *OrderService) ListOrders(ctx context.Context, userID int64, symbol string, startTime, endTime int64, limit int) ([]*repository.Order, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	if endTime == 0 {
		endTime = time.Now().UnixMilli()
	}
	if startTime == 0 {
		startTime = endTime - 7*24*3600*1000 // 默认 7 天
	}
	return s.repo.ListOrders(ctx, userID, symbol, startTime, endTime, limit)
}

// GetExchangeInfo 获取交易所信息
func (s *OrderService) GetExchangeInfo(ctx context.Context) ([]*repository.SymbolConfig, error) {
	return s.repo.ListSymbolConfigs(ctx)
}

func (s *OrderService) validateOrder(req *CreateOrderRequest, cfg *repository.SymbolConfig) error {
	if req == nil || cfg == nil {
		return fmt.Errorf("INVALID_PARAM")
	}
	if !isValidSide(req.Side) {
		return fmt.Errorf("INVALID_SIDE")
	}
	if !isValidOrderType(req.Type) {
		return fmt.Errorf("INVALID_ORDER_TYPE")
	}
	if !isValidTimeInForce(req.TimeInForce) {
		return fmt.Errorf("INVALID_TIME_IN_FORCE")
	}
	if req.Type == "MARKET" && req.TimeInForce == "POST_ONLY" {
		return fmt.Errorf("INVALID_TIME_IN_FORCE")
	}

	if cfg.BasePrecision <= 0 || cfg.QuotePrecision <= 0 {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	if normalizePrecision(cfg.QtyPrecision) != normalizePrecision(cfg.BasePrecision) {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	if normalizePrecision(cfg.PricePrecision) != normalizePrecision(cfg.QuotePrecision) {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	// 解析配置值（兼容小数与最小单位整数）
	qtyPrecision := normalizePrecision(cfg.QtyPrecision)
	pricePrecision := normalizePrecision(cfg.PricePrecision)

	minQty, err := parseScaledValue(cfg.MinQty, qtyPrecision)
	if err != nil {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	maxQty, err := parseScaledValue(cfg.MaxQty, qtyPrecision)
	if err != nil {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	qtyStep, err := parseScaledValue(cfg.QtyStep, qtyPrecision)
	if err != nil {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	priceTick, err := parseScaledValue(cfg.PriceTick, pricePrecision)
	if err != nil {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}
	minNotional, err := parseScaledValue(cfg.MinNotional, pricePrecision)
	if err != nil {
		return fmt.Errorf("INVALID_SYMBOL_CONFIG")
	}

	// 数量校验
	if req.Quantity < minQty {
		return fmt.Errorf("QTY_TOO_SMALL")
	}
	if req.Quantity > maxQty {
		return fmt.Errorf("QTY_TOO_LARGE")
	}
	if qtyStep > 0 && req.Quantity%qtyStep != 0 {
		return fmt.Errorf("INVALID_QUANTITY")
	}

	// 限价单价格校验
	if req.Type == "LIMIT" {
		if req.Price <= 0 {
			return fmt.Errorf("INVALID_PRICE")
		}
		if priceTick > 0 && req.Price%priceTick != 0 {
			return fmt.Errorf("INVALID_PRICE")
		}
		// 最小成交额
		notional := quoteQty(req.Price, req.Quantity, cfg.QtyPrecision)
		if notional < minNotional {
			return fmt.Errorf("NOTIONAL_TOO_SMALL")
		}
	}

	return nil
}

func (s *OrderService) marketBuyQuoteAmount(ctx context.Context, symbol string, qty int64, cfg *repository.SymbolConfig) (int64, int64, error) {
	if qty <= 0 {
		return 0, 0, errors.New("invalid quantity")
	}
	if s.validator == nil {
		return 0, 0, errors.New("no reference price")
	}
	refPrice, err := s.validator.ReferencePrice(symbol)
	if err != nil || refPrice <= 0 {
		return 0, 0, errors.New("no reference price")
	}

	limitRate := resolveLimitRate(cfg)
	pricePrecision := normalizePrecision(cfg.PricePrecision)
	priceDec := commondecimal.FromIntWithScale(refPrice, pricePrecision)
	one := commondecimal.FromInt(1)
	buffered := priceDec.Mul(one.Add(&limitRate))
	bufferedPrice := buffered.ToInt(pricePrecision)
	if bufferedPrice <= 0 {
		return 0, 0, errors.New("invalid reference price")
	}

	quoteAmount := quoteQty(bufferedPrice, qty, cfg.QtyPrecision)
	if quoteAmount <= 0 {
		return 0, 0, errors.New("invalid quote amount")
	}

	return bufferedPrice, quoteAmount, nil
}

func resolveLimitRate(cfg *repository.SymbolConfig) commondecimal.Decimal {
	rate := *commondecimal.MustNew("0.05")
	if cfg == nil || cfg.PriceLimitRate == "" {
		return rate
	}
	if v, err := commondecimal.New(cfg.PriceLimitRate); err == nil && v.Cmp(commondecimal.Zero) > 0 {
		return *v
	}
	return rate
}

// OrderMessage 发送到撮合的消息
type OrderMessage struct {
	Type          string `json:"type"`
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId"`
	UserID        int64  `json:"userId"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	OrderType     string `json:"orderType"`
	TimeInForce   string `json:"timeInForce"`
	Price         int64  `json:"price"`
	Qty           int64  `json:"qty"`
}

func (s *OrderService) sendToMatching(ctx context.Context, order *repository.Order) error {
	if s.redis == nil {
		return fmt.Errorf("redis client not configured")
	}
	price, err := parseInt64Compat(order.Price, "price")
	if err != nil {
		return err
	}
	qty, err := parseInt64Compat(order.OrigQty, "orig_qty")
	if err != nil {
		return err
	}
	msg := &OrderMessage{
		Type:          "NEW",
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		UserID:        order.UserID,
		Symbol:        order.Symbol,
		Side:          sideToString(order.Side),
		OrderType:     typeToString(order.Type),
		TimeInForce:   tifToString(order.TimeInForce),
		Price:         price,
		Qty:           qty,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: s.orderStream,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()

	return err
}

func parseInt64Compat(value string, field string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	if strings.Contains(value, ".") {
		parts := strings.SplitN(value, ".", 2)
		if len(parts) == 2 {
			frac := strings.TrimRight(parts[1], "0")
			if frac != "" {
				return 0, fmt.Errorf("parse %s: has fractional part %q", field, parts[1])
			}
			value = parts[0]
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", field, err)
	}
	return parsed, nil
}

func (s *OrderService) sendCancelToMatching(ctx context.Context, order *repository.Order) error {
	if s.redis == nil {
		return fmt.Errorf("redis client not configured")
	}
	msg := &OrderMessage{
		Type:          "CANCEL",
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		UserID:        order.UserID,
		Symbol:        order.Symbol,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: s.orderStream,
		Values: map[string]interface{}{
			"data": string(data),
		},
	}).Result()

	return err
}

func normalizeCreateOrderRequest(req *CreateOrderRequest) {
	if req == nil {
		return
	}
	req.Symbol = strings.TrimSpace(req.Symbol)
	req.Side = strings.ToUpper(strings.TrimSpace(req.Side))
	req.Type = strings.ToUpper(strings.TrimSpace(req.Type))
	req.TimeInForce = strings.ToUpper(strings.TrimSpace(req.TimeInForce))
	req.ClientOrderID = strings.TrimSpace(req.ClientOrderID)
}

func isValidSide(side string) bool {
	return side == "BUY" || side == "SELL"
}

func isValidOrderType(orderType string) bool {
	return orderType == "LIMIT" || orderType == "MARKET"
}

func isValidTimeInForce(tif string) bool {
	switch tif {
	case "", "GTC", "IOC", "FOK", "POST_ONLY":
		return true
	default:
		return false
	}
}

func parseSide(s string) int {
	if s == "SELL" {
		return repository.SideSell
	}
	return repository.SideBuy
}

func parseType(t string) int {
	if t == "MARKET" {
		return repository.TypeMarket
	}
	return repository.TypeLimit
}

func parseTIF(tif string) int {
	switch tif {
	case "IOC":
		return 2
	case "FOK":
		return 3
	case "POST_ONLY":
		return 4
	default:
		return 1 // GTC
	}
}

func sideToString(side int) string {
	if side == repository.SideBuy {
		return "BUY"
	}
	return "SELL"
}

func typeToString(t int) string {
	if t == repository.TypeMarket {
		return "MARKET"
	}
	return "LIMIT"
}

func tifToString(tif int) string {
	switch tif {
	case 2:
		return "IOC"
	case 3:
		return "FOK"
	case 4:
		return "POST_ONLY"
	default:
		return "GTC"
	}
}
