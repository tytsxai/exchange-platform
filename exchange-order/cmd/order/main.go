package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	commonerrors "github.com/exchange/common/pkg/errors"
	commonredis "github.com/exchange/common/pkg/redis"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/snowflake"
	"github.com/exchange/order/internal/client"
	"github.com/exchange/order/internal/config"
	"github.com/exchange/order/internal/metrics"
	"github.com/exchange/order/internal/repository"
	"github.com/exchange/order/internal/service"
	orderws "github.com/exchange/order/internal/ws"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	if err := snowflake.Init(cfg.WorkerID); err != nil {
		log.Fatalf("Failed to init snowflake: %v", err)
	}

	// 连接数据库
	db, err := sql.Open("postgres", cfg.DSN())
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(cfg.DBMaxOpenConns)
	db.SetMaxIdleConns(cfg.DBMaxIdleConns)
	db.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.DBConnMaxIdleTime)

	dbPingCtx, dbPingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dbPingCancel()
	if err := db.PingContext(dbPingCtx); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Printf("Connected to PostgreSQL")

	// 连接 Redis
	redisTLSConfig, err := commonredis.TLSConfigFromEnv()
	if err != nil {
		log.Fatalf("Invalid Redis TLS config: %v", err)
	}
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.RedisAddr,
		Password:     cfg.RedisPassword,
		TLSConfig:    redisTLSConfig,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
		PoolSize:     200,
		MinIdleConns: 20,
	})
	defer redisClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	redisPingCtx, redisPingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer redisPingCancel()
	if err := redisClient.Ping(redisPingCtx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis")

	metricsClient := metrics.New()

	// 创建服务
	idGen := snowflakeIDGen{}
	repo := repository.NewOrderRepository(db)
	matchingClient := client.NewMatchingClient(cfg.MatchingServiceURL, cfg.InternalToken)
	clearingClient := client.NewClearingClient(cfg.ClearingBaseURL, cfg.InternalToken)
	validator := service.NewPriceValidator(repo, matchingClient, service.PriceValidatorConfig{
		Enabled:          cfg.PriceProtection.Enabled,
		DefaultLimitRate: cfg.PriceProtection.DefaultLimitRate,
	})
	svc := service.NewOrderService(repo, redisClient, idGen, cfg.OrderStream, validator, clearingClient, metricsClient)
	wsPublisher := orderws.NewPublisher(redisClient, cfg.PrivateUserEventChannel)
	svc.SetPublisher(wsPublisher)

	tradeRepo := repository.NewTradeRepository(db)
	updater := service.NewOrderUpdater(redisClient, repo, tradeRepo, clearingClient, metricsClient, &service.UpdaterConfig{
		EventStream: cfg.EventStream,
		Group:       cfg.MatchingConsumerGroup,
		Consumer:    cfg.MatchingConsumerName,
	})
	updater.SetPublisher(wsPublisher)
	if err := updater.Start(ctx); err != nil {
		log.Fatalf("Failed to start order updater: %v", err)
	}

	// HTTP 服务
	mux := http.NewServeMux()
	healthHTTPClient := &http.Client{Timeout: 2 * time.Second}
	requireInternalAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-Internal-Token") != cfg.InternalToken {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			next(w, r)
		}
	}

	// 健康检查
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingBaseURL, healthHTTPClient),
			checkConsumeLoop(updater),
		}
		writeHealth(w, deps)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
			checkRedis(r.Context(), redisClient),
			checkHTTP(r.Context(), "matching", cfg.MatchingServiceURL, healthHTTPClient),
			checkHTTP(r.Context(), "clearing", cfg.ClearingBaseURL, healthHTTPClient),
			checkConsumeLoop(updater),
		}
		writeHealth(w, deps)
	})

	// 交易所信息
	mux.HandleFunc("/v1/exchangeInfo", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		configs, err := svc.GetExchangeInfo(r.Context())
		if err != nil {
			writeInternalError(w, err)
			return
		}
		symbols := make([]*symbolInfoResponse, 0, len(configs))
		for _, cfg := range configs {
			if cfg == nil {
				continue
			}
			symbols = append(symbols, toSymbolInfoResponse(cfg))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"serverTime": time.Now().UnixMilli(),
			"symbols":    symbols,
		})
	}))

	// Prometheus metrics
	metricsHandler := metricsClient.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			metricsClient.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)

	// 下单
	mux.HandleFunc("/v1/order", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handleCreateOrder(w, r, svc)
		case http.MethodDelete:
			handleCancelOrder(w, r, svc)
		case http.MethodGet:
			handleGetOrder(w, r, svc)
		default:
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
		}
	}))

	// 当前委托
	mux.HandleFunc("/v1/openOrders", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
			return
		}
		symbol := r.URL.Query().Get("symbol")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 {
			limit = 100
		}

		orders, err := svc.ListOpenOrders(r.Context(), userID, symbol, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toOrderResponses(orders))
	}))

	// 历史订单
	mux.HandleFunc("/v1/allOrders", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
			return
		}
		symbol := r.URL.Query().Get("symbol")
		startTime, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		endTime, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		orders, err := svc.ListOrders(r.Context(), userID, symbol, startTime, endTime, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toOrderResponses(orders))
	}))

	// 我的成交
	mux.HandleFunc("/v1/myTrades", requireInternalAuth(func(w http.ResponseWriter, r *http.Request) {
		userID, err := getUserIDFromHeader(r)
		if err != nil {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
			return
		}

		symbol := r.URL.Query().Get("symbol")
		if strings.TrimSpace(symbol) == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "symbol required")
			return
		}
		startTime, _ := strconv.ParseInt(r.URL.Query().Get("startTime"), 10, 64)
		endTime, _ := strconv.ParseInt(r.URL.Query().Get("endTime"), 10, 64)
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		trades, err := tradeRepo.ListTradesByUser(r.Context(), userID, symbol, startTime, endTime, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(toAccountTradeResponses(trades, userID))
	}))

	handler := limitBodyMiddleware(maxBodyBytes, mux)
	handler = commonresp.RequestIDMiddleware(handler)
	handler = commonresp.RecoveryMiddleware(handler)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.HTTPPort),
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	go func() {
		log.Printf("HTTP server listening on :%d", cfg.HTTPPort)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down...")
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
	log.Println("Shutdown complete")
}

type snowflakeIDGen struct{}

func (g snowflakeIDGen) NextID() int64 {
	return snowflake.MustNextID()
}

type dependencyStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Latency int64  `json:"latency"`
}

type healthResponse struct {
	Status       string             `json:"status"`
	Dependencies []dependencyStatus `json:"dependencies"`
}

func checkPostgres(ctx context.Context, db *sql.DB) dependencyStatus {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := db.PingContext(timeoutCtx)
	status := "ok"
	if err != nil {
		status = "down"
	}
	return dependencyStatus{
		Name:    "postgres",
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
}

func checkRedis(ctx context.Context, client *redis.Client) dependencyStatus {
	start := time.Now()
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := client.Ping(timeoutCtx).Err()
	status := "ok"
	if err != nil {
		status = "down"
	}
	return dependencyStatus{
		Name:    "redis",
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
}

func checkHTTP(ctx context.Context, name, baseURL string, client *http.Client) dependencyStatus {
	start := time.Now()
	status := "ok"
	if baseURL == "" {
		status = "down"
	} else {
		healthURL := strings.TrimRight(baseURL, "/") + "/health"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			status = "down"
		} else {
			resp, err := client.Do(req)
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				status = "down"
			}
			if resp != nil {
				resp.Body.Close()
			}
		}
	}
	return dependencyStatus{
		Name:    name,
		Status:  status,
		Latency: time.Since(start).Milliseconds(),
	}
}

func checkConsumeLoop(updater *service.OrderUpdater) dependencyStatus {
	now := time.Now()
	ok, age, _ := updater.ConsumeLoopHealthy(now, 45*time.Second)
	status := "ok"
	if !ok {
		status = "down"
	}
	return dependencyStatus{
		Name:    "eventStreamConsumer",
		Status:  status,
		Latency: age.Milliseconds(),
	}
}

func writeHealth(w http.ResponseWriter, deps []dependencyStatus) {
	status := "ok"
	for _, dep := range deps {
		if dep.Status != "ok" {
			status = "degraded"
			break
		}
	}
	code := http.StatusOK
	if status != "ok" {
		code = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(healthResponse{
		Status:       status,
		Dependencies: deps,
	})
}

// CreateOrderRequest 下单请求
type CreateOrderRequest struct {
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"timeInForce"`
	Price         int64  `json:"price"`
	Quantity      int64  `json:"quantity"`
	QuoteOrderQty int64  `json:"quoteOrderQty"`
	ClientOrderID string `json:"clientOrderId"`
}

func getUserIDFromHeader(r *http.Request) (int64, error) {
	userIDStr := strings.TrimSpace(r.Header.Get("X-User-Id"))
	if userIDStr == "" {
		return 0, fmt.Errorf("X-User-Id header required")
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("invalid X-User-Id")
	}
	return userID, nil
}

func handleCreateOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
		return
	}

	var req CreateOrderRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	resp, err := svc.CreateOrder(r.Context(), &service.CreateOrderRequest{
		UserID:        userID,
		Symbol:        req.Symbol,
		Side:          req.Side,
		Type:          req.Type,
		TimeInForce:   req.TimeInForce,
		Price:         req.Price,
		Quantity:      req.Quantity,
		QuoteOrderQty: req.QuoteOrderQty,
		ClientOrderID: req.ClientOrderID,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}

	if resp.ErrorCode != "" {
		commonresp.WriteErrorCode(w, r, commonerrors.Code(resp.ErrorCode), "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toOrderResponse(resp.Order))
}

func handleCancelOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
		return
	}
	symbol := r.URL.Query().Get("symbol")
	orderID, _ := strconv.ParseInt(r.URL.Query().Get("orderId"), 10, 64)
	clientOrderID := r.URL.Query().Get("clientOrderId")

	resp, err := svc.CancelOrder(r.Context(), &service.CancelOrderRequest{
		UserID:        userID,
		Symbol:        symbol,
		OrderID:       orderID,
		ClientOrderID: clientOrderID,
	})
	if err != nil {
		writeInternalError(w, err)
		return
	}

	if resp.ErrorCode != "" {
		commonresp.WriteErrorCode(w, r, commonerrors.Code(resp.ErrorCode), "")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toOrderResponse(resp.Order))
}

func handleGetOrder(w http.ResponseWriter, r *http.Request, svc *service.OrderService) {
	userID, err := getUserIDFromHeader(r)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, err.Error())
		return
	}
	orderID, _ := strconv.ParseInt(r.URL.Query().Get("orderId"), 10, 64)

	order, err := svc.GetOrder(r.Context(), userID, orderID)
	if err != nil {
		commonresp.WriteErrorCode(w, r, commonerrors.CodeOrderNotFound, "order not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(toOrderResponse(order))
}

func metricsAuthorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Metrics-Token")) == token {
		return true
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(auth, "Bearer ") && strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == token {
		return true
	}
	return false
}

const maxBodyBytes int64 = 4 << 20

func limitBodyMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && maxBytes > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		if isRequestTooLarge(err) {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeRequestTooLarge, "")
			return false
		}
		commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidRequest, "invalid request")
		return false
	}
	return true
}

func isRequestTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	commonresp.WriteErrorCode(w, nil, commonerrors.CodeInternal, "internal error")
}

type symbolInfoResponse struct {
	Symbol         string `json:"symbol"`
	BaseAsset      string `json:"baseAsset"`
	QuoteAsset     string `json:"quoteAsset"`
	Status         int    `json:"status"`
	BasePrecision  int    `json:"basePrecision"`
	QuotePrecision int    `json:"quotePrecision"`
	PricePrecision int    `json:"pricePrecision"`
	QtyPrecision   int    `json:"qtyPrecision"`
	PriceTick      string `json:"priceTick"`
	QtyStep        string `json:"qtyStep"`
	MinQty         string `json:"minQty"`
	MaxQty         string `json:"maxQty"`
	MinNotional    string `json:"minNotional"`
	PriceLimitRate string `json:"priceLimitRate,omitempty"`
	MakerFeeRate   string `json:"makerFeeRate,omitempty"`
	TakerFeeRate   string `json:"takerFeeRate,omitempty"`
}

func toSymbolInfoResponse(cfg *repository.SymbolConfig) *symbolInfoResponse {
	if cfg == nil {
		return nil
	}
	return &symbolInfoResponse{
		Symbol:         cfg.Symbol,
		BaseAsset:      cfg.BaseAsset,
		QuoteAsset:     cfg.QuoteAsset,
		Status:         cfg.Status,
		BasePrecision:  cfg.BasePrecision,
		QuotePrecision: cfg.QuotePrecision,
		PricePrecision: cfg.PricePrecision,
		QtyPrecision:   cfg.QtyPrecision,
		PriceTick:      cfg.PriceTick,
		QtyStep:        cfg.QtyStep,
		MinQty:         cfg.MinQty,
		MaxQty:         cfg.MaxQty,
		MinNotional:    cfg.MinNotional,
		PriceLimitRate: cfg.PriceLimitRate,
		MakerFeeRate:   cfg.MakerFeeRate,
		TakerFeeRate:   cfg.TakerFeeRate,
	}
}

type orderResponse struct {
	OrderID       int64  `json:"orderId"`
	ClientOrderID string `json:"clientOrderId,omitempty"`
	Symbol        string `json:"symbol"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"timeInForce"`
	Price         string `json:"price"`
	OrigQty       string `json:"origQty"`
	ExecutedQty   string `json:"executedQty"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"createdAt"`
	UpdatedAt     int64  `json:"updatedAt"`
}

func toOrderResponse(order *repository.Order) *orderResponse {
	if order == nil {
		return nil
	}
	return &orderResponse{
		OrderID:       order.OrderID,
		ClientOrderID: order.ClientOrderID,
		Symbol:        order.Symbol,
		Side:          sideToString(order.Side),
		Type:          typeToString(order.Type),
		TimeInForce:   tifToString(order.TimeInForce),
		Price:         order.Price,
		OrigQty:       order.OrigQty,
		ExecutedQty:   order.ExecutedQty,
		Status:        statusToString(order.Status),
		CreatedAt:     order.CreateTimeMs,
		UpdatedAt:     order.UpdateTimeMs,
	}
}

func toOrderResponses(orders []*repository.Order) []*orderResponse {
	if len(orders) == 0 {
		return []*orderResponse{}
	}
	resp := make([]*orderResponse, 0, len(orders))
	for _, order := range orders {
		if order == nil {
			continue
		}
		resp = append(resp, toOrderResponse(order))
	}
	return resp
}

type accountTradeResponse struct {
	TradeID         int64  `json:"tradeId"`
	OrderID         int64  `json:"orderId"`
	Symbol          string `json:"symbol"`
	Price           string `json:"price"`
	Qty             string `json:"qty"`
	Commission      string `json:"commission"`
	CommissionAsset string `json:"commissionAsset"`
	Time            int64  `json:"time"`
	IsBuyer         bool   `json:"isBuyer"`
	IsMaker         bool   `json:"isMaker"`
}

func toAccountTradeResponses(trades []*repository.Trade, userID int64) []*accountTradeResponse {
	if len(trades) == 0 {
		return []*accountTradeResponse{}
	}
	resp := make([]*accountTradeResponse, 0, len(trades))
	for _, trade := range trades {
		if trade == nil {
			continue
		}
		resp = append(resp, toAccountTradeResponse(trade, userID))
	}
	return resp
}

func toAccountTradeResponse(trade *repository.Trade, userID int64) *accountTradeResponse {
	if trade == nil {
		return nil
	}
	isMaker := trade.MakerUserID == userID
	orderID := trade.TakerOrderID
	commission := trade.TakerFee
	isBuyer := trade.TakerSide == repository.SideBuy
	if isMaker {
		orderID = trade.MakerOrderID
		commission = trade.MakerFee
		isBuyer = trade.TakerSide == repository.SideSell
	}
	return &accountTradeResponse{
		TradeID:         trade.TradeID,
		OrderID:         orderID,
		Symbol:          trade.Symbol,
		Price:           strconv.FormatInt(trade.Price, 10),
		Qty:             strconv.FormatInt(trade.Qty, 10),
		Commission:      strconv.FormatInt(commission, 10),
		CommissionAsset: trade.FeeAsset,
		Time:            trade.TimestampMs,
		IsBuyer:         isBuyer,
		IsMaker:         isMaker,
	}
}

func sideToString(side int) string {
	switch side {
	case repository.SideBuy:
		return "BUY"
	case repository.SideSell:
		return "SELL"
	default:
		return "UNKNOWN"
	}
}

func typeToString(orderType int) string {
	switch orderType {
	case repository.TypeLimit:
		return "LIMIT"
	case repository.TypeMarket:
		return "MARKET"
	default:
		return "UNKNOWN"
	}
}

func tifToString(tif int) string {
	switch tif {
	case 2:
		return "IOC"
	case 3:
		return "FOK"
	default:
		return "GTC"
	}
}

func statusToString(status int) string {
	switch status {
	case repository.StatusInit:
		return "INIT"
	case repository.StatusNew:
		return "NEW"
	case repository.StatusPartiallyFilled:
		return "PARTIALLY_FILLED"
	case repository.StatusFilled:
		return "FILLED"
	case repository.StatusCanceled:
		return "CANCELED"
	case repository.StatusRejected:
		return "REJECTED"
	case repository.StatusExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}
