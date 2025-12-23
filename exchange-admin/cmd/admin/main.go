package main

import (
	"context"
	"database/sql"
	"encoding/json"
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
	"github.com/exchange/common/pkg/snowflake"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	cfg := config.Load()
	log.Printf("Starting %s...", cfg.ServiceName)

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

	if err := db.Ping(); err != nil {
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
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
		}
		writeHealth(w, deps)
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		deps := []dependencyStatus{
			checkPostgres(r.Context(), db),
		}
		writeHealth(w, deps)
	})

	// Swagger UI - API 文档
	// 访问 /docs 查看交互式 API 文档，支持在线测试
	// 访问 /openapi.yaml 获取 OpenAPI 3.0 规范文件
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

	// 系统状态
	mux.HandleFunc("/admin/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := svc.GetSystemStatus(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(symbols)

		case http.MethodPost:
			actorID := getActorID(r)
			var cfg repository.SymbolConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := svc.CreateSymbol(r.Context(), actorID, r.RemoteAddr, &cfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/admin/symbols/", func(w http.ResponseWriter, r *http.Request) {
		symbol := r.URL.Path[len("/admin/symbols/"):]
		if symbol == "" {
			http.Error(w, "symbol required", http.StatusBadRequest)
			return
		}

		switch r.Method {
		case http.MethodGet:
			cfg, err := svc.GetSymbol(r.Context(), symbol)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if cfg == nil {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(cfg)

		case http.MethodPatch:
			actorID := getActorID(r)
			var cfg repository.SymbolConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			cfg.Symbol = symbol
			if err := svc.UpdateSymbol(r.Context(), actorID, r.RemoteAddr, &cfg); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ========== Kill Switch ==========
	mux.HandleFunc("/admin/killSwitch", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		actorID := getActorID(r)
		var req struct {
			Action string `json:"action"` // halt, cancelOnly, resume
			Symbol string `json:"symbol"` // 可选，不传则全局
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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
				http.Error(w, "invalid action", http.StatusBadRequest)
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
				http.Error(w, "invalid action", http.StatusBadRequest)
				return
			}
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	})

	// ========== 审计日志 ==========
	mux.HandleFunc("/admin/auditLogs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		targetType := r.URL.Query().Get("targetType")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 100
		}

		logs, err := svc.ListAuditLogs(r.Context(), targetType, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(logs)
	})

	// ========== RBAC ==========
	mux.HandleFunc("/admin/roles", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		roles, err := svc.ListRoles(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
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
				http.Error(w, "userId required", http.StatusBadRequest)
				return
			}
			roles, err := svc.GetUserRoles(r.Context(), userID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"userId": userID, "roleIds": roles})

		case http.MethodPost:
			var req struct {
				UserID int64 `json:"userId"`
				RoleID int64 `json:"roleId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := svc.AssignRole(r.Context(), actorID, r.RemoteAddr, req.UserID, req.RoleID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		case http.MethodDelete:
			var req struct {
				UserID int64 `json:"userId"`
				RoleID int64 `json:"roleId"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := svc.RemoveRole(r.Context(), actorID, r.RemoteAddr, req.UserID, req.RoleID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// 中间件链
	handler := authMiddleware(tokenManager, mux)

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
		if r.URL.Path == "/health" || r.URL.Path == "/ready" || r.URL.Path == "/docs" || r.URL.Path == "/openapi.yaml" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		// 2. 获取 Token
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "invalid authorization format", http.StatusUnauthorized)
			return
		}

		token := parts[1]

		userID, err := tokenManager.Verify(token)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}

		// 4. 设置 Header (供后续 handler 使用)
		r.Header.Set("X-Actor-ID", fmt.Sprintf("%d", userID))
		next.ServeHTTP(w, r)
	})
}
