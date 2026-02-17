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

	"github.com/exchange/admin/internal/config"
	"github.com/exchange/admin/internal/repository"
	"github.com/exchange/admin/internal/service"
	commonauth "github.com/exchange/common/pkg/auth"
	commonerrors "github.com/exchange/common/pkg/errors"
	commonresp "github.com/exchange/common/pkg/response"
	"github.com/exchange/common/pkg/snowflake"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	tokenManager, err := commonauth.NewTokenManager(cfg.AuthTokenSecret, cfg.AuthTokenTTL)
	if err != nil {
		log.Fatalf("Invalid auth token config: %v", err)
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

	// 创建服务
	idGen := snowflakeIDGen{}
	repo := repository.NewAdminRepository(db)
	svc := service.NewAdminService(repo, idGen)

	// HTTP 服务
	mux := http.NewServeMux()

	// 健康检查
	mux.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
		}
		writeHealth(w, deps)
	})
	metricsHandler := promhttp.Handler()
	if token := os.Getenv("METRICS_TOKEN"); token != "" {
		metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !metricsAuthorized(r, token) {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
			promhttp.Handler().ServeHTTP(w, r)
		})
	}
	mux.Handle("/metrics", metricsHandler)
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
		}
		writeHealth(w, deps)
	})

	// Swagger UI - API 文档
	// 访问 /docs 查看交互式 API 文档，支持在线测试
	// 访问 /openapi.yaml 获取 OpenAPI 3.0 规范文件
	if cfg.EnableDocs {
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, "api/openapi.yaml")
		})
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Exchange Admin API Documentation</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui.css">
    <style>
        body { margin: 0; padding: 0; }
        .swagger-ui .topbar { display: none; }
        .swagger-ui .info { margin: 20px 0; }
        .swagger-ui .info .title { font-size: 2em; }
        .swagger-ui .scheme-container { background: #fafafa; padding: 15px 0; }
        .swagger-ui .btn.authorize { background-color: #49cc90; border-color: #49cc90; }
    </style>
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-bundle.js"></script>
    <script src="https://unpkg.com/swagger-ui-dist@5.11.0/swagger-ui-standalone-preset.js"></script>
    <script>
        window.onload = () => {
            window.ui = SwaggerUIBundle({
                url: '/openapi.yaml',
                dom_id: '#swagger-ui',
                deepLinking: true,
                presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
                plugins: [SwaggerUIBundle.plugins.DownloadUrl],
                layout: "StandaloneLayout",
                tryItOutEnabled: true,
                persistAuthorization: true,
                filter: true
            });
        };
    </script>
</body>
</html>`
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(html))
		})
	} else {
		mux.HandleFunc("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
		mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}

	// 系统状态
	mux.HandleFunc("/admin/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := svc.GetSystemStatus(r.Context())
		if err != nil {
			writeInternalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	// ========== 交易对管理 ==========
	mux.HandleFunc("/admin/symbols", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			symbols, err := svc.ListSymbols(r.Context())
			if err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(symbols)

		case http.MethodPost:
			actorID := getActorID(r)
			var cfg repository.SymbolConfig
			if !decodeJSON(w, r, &cfg) {
				return
			}
			if err := svc.CreateSymbol(r.Context(), actorID, r.RemoteAddr, &cfg); err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
		}
	})

	mux.HandleFunc("/admin/symbols/", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Path[len("/admin/symbols/"):]
		if symbol == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "symbol required")
			return
		}

		switch r.Method {
		case http.MethodGet:
			cfg, err := svc.GetSymbol(r.Context(), symbol)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			if cfg == nil {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeNotFound, "not found")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfg)

		case http.MethodPatch:
			actorID := getActorID(r)
			var cfg repository.SymbolConfig
			if !decodeJSON(w, r, &cfg) {
				return
			}
			cfg.Symbol = symbol
			if err := svc.UpdateSymbol(r.Context(), actorID, r.RemoteAddr, &cfg); err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
		}
	})

	// ========== Kill Switch ==========
	mux.HandleFunc("/admin/killSwitch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		actorID := getActorID(r)
		var req struct {
			Action string `json:"action"` // halt, cancelOnly, resume
			Symbol string `json:"symbol"` // 可选，不传则全局
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		var err error
		if req.Symbol != "" {
			// 单个交易对
			var status int
			switch req.Action {
			case "halt":
				status = service.StatusHalt
			case "cancelOnly":
				status = service.StatusCancelOnly
			case "resume":
				status = service.StatusTrading
			default:
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "invalid action")
				return
			}
			err = svc.SetSymbolStatus(r.Context(), actorID, r.RemoteAddr, req.Symbol, status)
		} else {
			// 全局
			switch req.Action {
			case "halt":
				err = svc.GlobalHalt(r.Context(), actorID, r.RemoteAddr)
			case "cancelOnly":
				err = svc.GlobalCancelOnly(r.Context(), actorID, r.RemoteAddr)
			case "resume":
				err = svc.GlobalResume(r.Context(), actorID, r.RemoteAddr)
			default:
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "invalid action")
				return
			}
		}

		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// ========== 审计日志 ==========
	mux.HandleFunc("/admin/auditLogs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		targetType := r.URL.Query().Get("targetType")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}

		logs, err := svc.ListAuditLogs(r.Context(), targetType, limit)
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	})

	// ========== RBAC ==========
	mux.HandleFunc("/admin/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
			return
		}

		roles, err := svc.ListRoles(r.Context())
		if err != nil {
			writeInternalError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(roles)
	})

	mux.HandleFunc("/admin/userRoles", func(w http.ResponseWriter, r *http.Request) {
		actorID := getActorID(r)

		switch r.Method {
		case http.MethodGet:
			userID, _ := strconv.ParseInt(r.URL.Query().Get("userId"), 10, 64)
			if userID == 0 {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeInvalidParam, "userId required")
				return
			}
			roles, err := svc.GetUserRoles(r.Context(), userID)
			if err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"userId": userID, "roleIds": roles})

		case http.MethodPost:
			var req struct {
				UserID int64 `json:"userId"`
				RoleID int64 `json:"roleId"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			if err := svc.AssignRole(r.Context(), actorID, r.RemoteAddr, req.UserID, req.RoleID); err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		case http.MethodDelete:
			var req struct {
				UserID int64 `json:"userId"`
				RoleID int64 `json:"roleId"`
			}
			if !decodeJSON(w, r, &req) {
				return
			}
			if err := svc.RemoveRole(r.Context(), actorID, r.RemoteAddr, req.UserID, req.RoleID); err != nil {
				writeInternalError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			commonresp.WriteStatusError(w, r, http.StatusMethodNotAllowed, commonerrors.CodeInvalidRequest, "method not allowed")
		}
	})

	// 中间件链
	var handler http.Handler = mux
	handler = adminPermissionMiddleware(repo, handler)
	handler = authMiddleware(tokenManager, handler)
	handler = adminTokenMiddleware(cfg.AdminToken, handler)
	handler = limitBodyMiddleware(maxBodyBytes, handler)
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	log.Println("Shutdown complete")
}

type snowflakeIDGen struct{}

func (g snowflakeIDGen) NextID() int64 {
	return snowflake.MustNextID()
}

func getActorID(r *http.Request) int64 {
	actorID, _ := strconv.ParseInt(r.Header.Get("X-Actor-ID"), 10, 64)
	return actorID
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

func authMiddleware(tokenManager *commonauth.TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. 跳过健康检查和文档
		if r.URL.Path == "/live" || r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/docs" || r.URL.Path == "/openapi.yaml" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// 2. 获取 Token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "authorization required")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "invalid authorization format")
			return
		}

		token := parts[1]

		userID, err := tokenManager.Verify(token)
		if err != nil {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "invalid token")
			return
		}

		// 4. 设置 Header (供后续 handler 使用)
		r.Header.Set("X-Actor-ID", fmt.Sprintf("%d", userID))
		next.ServeHTTP(w, r)
	})
}

func adminTokenMiddleware(adminToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin") {
			if r.Header.Get("X-Admin-Token") != adminToken {
				commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type adminPermissionReader interface {
	GetUserPermissions(ctx context.Context, userID int64) ([]string, error)
}

type adminRoutePermission struct {
	Method      string
	Path        string
	PrefixMatch bool
	AnyOf       []string
}

var adminRoutePermissions = []adminRoutePermission{
	{Method: http.MethodGet, Path: "/admin/status", AnyOf: []string{"dashboard:read", "symbol:read", "risk:read", "audit:read"}},
	{Method: http.MethodGet, Path: "/admin/symbols", AnyOf: []string{"symbol:read", "symbol:write", "risk:read"}},
	{Method: http.MethodPost, Path: "/admin/symbols", AnyOf: []string{"symbol:write", "risk:write"}},
	{Method: http.MethodGet, Path: "/admin/symbols/", PrefixMatch: true, AnyOf: []string{"symbol:read", "symbol:write", "risk:read"}},
	{Method: http.MethodPatch, Path: "/admin/symbols/", PrefixMatch: true, AnyOf: []string{"symbol:write", "risk:write"}},
	{Method: http.MethodPost, Path: "/admin/killSwitch", AnyOf: []string{"killswitch:execute", "risk:write"}},
	{Method: http.MethodGet, Path: "/admin/auditLogs", AnyOf: []string{"audit:read"}},
	{Method: http.MethodGet, Path: "/admin/roles", AnyOf: []string{"rbac:read", "user:read", "risk:read"}},
	{Method: http.MethodGet, Path: "/admin/userRoles", AnyOf: []string{"rbac:read", "user:read", "risk:read"}},
	{Method: http.MethodPost, Path: "/admin/userRoles", AnyOf: []string{"rbac:write", "user:write", "risk:write"}},
	{Method: http.MethodDelete, Path: "/admin/userRoles", AnyOf: []string{"rbac:write", "user:write", "risk:write"}},
}

func adminPermissionMiddleware(reader adminPermissionReader, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/admin") || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		requiredPerms, matched := matchAdminRoutePermission(r.Method, r.URL.Path)
		if !matched {
			commonresp.WriteErrorCode(w, r, commonerrors.CodePermissionDenied, "permission denied")
			return
		}

		actorID := getActorID(r)
		if actorID <= 0 {
			commonresp.WriteErrorCode(w, r, commonerrors.CodeUnauthenticated, "unauthorized")
			return
		}

		perms, err := reader.GetUserPermissions(r.Context(), actorID)
		if err != nil {
			log.Printf("load admin permissions failed: user=%d err=%v", actorID, err)
			commonresp.WriteErrorCode(w, r, commonerrors.CodeInternal, "internal error")
			return
		}
		if !hasAnyPermission(perms, requiredPerms) {
			commonresp.WriteErrorCode(w, r, commonerrors.CodePermissionDenied, "permission denied")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func matchAdminRoutePermission(method, path string) ([]string, bool) {
	for _, rule := range adminRoutePermissions {
		if rule.Method != method {
			continue
		}
		if rule.PrefixMatch {
			if strings.HasPrefix(path, rule.Path) {
				return rule.AnyOf, true
			}
			continue
		}
		if path == rule.Path {
			return rule.AnyOf, true
		}
	}
	return nil, false
}

func hasAnyPermission(granted []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	if len(granted) == 0 {
		return false
	}
	grantedSet := make(map[string]struct{}, len(granted))
	for _, perm := range granted {
		perm = strings.TrimSpace(perm)
		if perm == "" {
			continue
		}
		grantedSet[perm] = struct{}{}
	}
	if _, ok := grantedSet["*"]; ok {
		return true
	}
	for _, need := range required {
		if _, ok := grantedSet[need]; ok {
			return true
		}
	}
	return false
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
