package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusUp       Status = "up"
	StatusDown     Status = "down"
	StatusDegraded Status = "degraded"
)

type Checker interface {
	Name() string
	Check(ctx context.Context) CheckResult
}

type CheckResult struct {
	Status  Status        `json:"status"`
	Latency time.Duration `json:"latency"`
	Message string        `json:"message,omitempty"`
}

type Response struct {
	Status       Status                 `json:"status"`
	Dependencies map[string]CheckResult `json:"dependencies,omitempty"`
}

type Health struct {
	checkers []Checker
	ready    atomic.Bool
}

const defaultCheckTimeout = 2 * time.Second

func New() *Health {
	return &Health{}
}

func (h *Health) Register(c Checker) {
	if c == nil {
		return
	}
	h.checkers = append(h.checkers, c)
}

func (h *Health) SetReady(ready bool) {
	h.ready.Store(ready)
}

func (h *Health) IsReady() bool {
	return h.ready.Load()
}

// Live 存活检查（只检查进程是否响应）
func (h *Health) Live() Response {
	return Response{Status: StatusUp}
}

// Ready 就绪检查（检查所有依赖）
func (h *Health) Ready(ctx context.Context) Response {
	if !h.IsReady() {
		r := Response{Status: StatusDown}
		if len(h.checkers) > 0 {
			r.Dependencies = h.runChecks(ctx)
		}
		return r
	}

	deps := h.runChecks(ctx)
	return Response{
		Status:       summarize(deps),
		Dependencies: deps,
	}
}

// Health 完整健康检查
func (h *Health) Health(ctx context.Context) Response {
	deps := h.runChecks(ctx)
	status := summarize(deps)
	if !h.IsReady() && status == StatusUp {
		status = StatusDown
	}
	return Response{
		Status:       status,
		Dependencies: deps,
	}
}

func (h *Health) runChecks(ctx context.Context) map[string]CheckResult {
	checkers := append([]Checker(nil), h.checkers...)
	if len(checkers) == 0 {
		return nil
	}

	parent := ctx
	if parent == nil {
		parent = context.Background()
	}

	results := make(map[string]CheckResult, len(checkers))
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(checkers))

	for _, c := range checkers {
		c := c
		go func() {
			defer wg.Done()
			name := c.Name()
			if name == "" {
				name = "unknown"
			}

			start := time.Now()
			depCtx, cancel := context.WithTimeout(parent, defaultCheckTimeout)
			defer cancel()

			resCh := make(chan CheckResult, 1)
			go func() {
				resCh <- c.Check(depCtx)
			}()

			var res CheckResult
			select {
			case res = <-resCh:
			case <-depCtx.Done():
				res = CheckResult{
					Status:  StatusDown,
					Latency: time.Since(start),
					Message: "timeout",
				}
			// 异步消费结果，确保 goroutine 能退出
			go func() { <-resCh }()
			}

			if res.Latency <= 0 {
				res.Latency = time.Since(start)
			}
			if res.Status == "" {
				res.Status = StatusDown
			}

			mu.Lock()
			results[name] = res
			mu.Unlock()
		}()
	}

	wg.Wait()
	return results
}

func summarize(deps map[string]CheckResult) Status {
	if len(deps) == 0 {
		return StatusUp
	}

	overall := StatusUp
	for _, r := range deps {
		switch r.Status {
		case StatusDown:
			return StatusDegraded // 任一依赖 down 则整体 degraded
		case StatusDegraded:
			overall = StatusDegraded
		}
	}
	return overall
}

func statusCode(s Status) int {
	if s == StatusUp {
		return http.StatusOK
	}
	return http.StatusServiceUnavailable
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Health) LiveHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := h.Live()
		writeJSON(w, statusCode(resp.Status), resp)
	}
}

func (h *Health) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := h.Ready(r.Context())
		writeJSON(w, statusCode(resp.Status), resp)
	}
}

func (h *Health) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := h.Health(r.Context())
		writeJSON(w, statusCode(resp.Status), resp)
	}
}

type postgresChecker struct {
	db *sql.DB
}

func NewPostgresChecker(db *sql.DB) Checker {
	return &postgresChecker{db: db}
}

func (c *postgresChecker) Name() string { return "postgres" }

func (c *postgresChecker) Check(ctx context.Context) CheckResult {
	if c == nil || c.db == nil {
		return CheckResult{Status: StatusDown, Message: "nil db"}
	}
	start := time.Now()
	err := c.db.PingContext(ctx)
	lat := time.Since(start)
	if err != nil {
		return CheckResult{Status: StatusDown, Latency: lat, Message: err.Error()}
	}
	return CheckResult{Status: StatusUp, Latency: lat}
}

type RedisPingCmd interface {
	Err() error
}

type RedisClient interface {
	Ping(ctx context.Context) RedisPingCmd
}

type redisChecker struct {
	client RedisClient
}

func NewRedisChecker(client RedisClient) Checker {
	return &redisChecker{client: client}
}

func (c *redisChecker) Name() string { return "redis" }

func (c *redisChecker) Check(ctx context.Context) CheckResult {
	if c == nil || c.client == nil {
		return CheckResult{Status: StatusDown, Message: "nil redis client"}
	}
	start := time.Now()
	cmd := c.client.Ping(ctx)
	lat := time.Since(start)
	if cmd == nil {
		return CheckResult{Status: StatusDown, Latency: lat, Message: "nil ping response"}
	}
	if err := cmd.Err(); err != nil {
		return CheckResult{Status: StatusDown, Latency: lat, Message: err.Error()}
	}
	return CheckResult{Status: StatusUp, Latency: lat}
}

type httpChecker struct {
	name   string
	url    string
	client *http.Client
}

func NewHTTPChecker(name, url string) Checker {
	if name == "" {
		name = "http"
	}
	return &httpChecker{
		name: name,
		url:  url,
		client: &http.Client{
			Timeout: defaultCheckTimeout,
		},
	}
}

func (c *httpChecker) Name() string { return c.name }

func (c *httpChecker) Check(ctx context.Context) CheckResult {
	if c == nil || c.url == "" {
		return CheckResult{Status: StatusDown, Message: "empty url"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		return CheckResult{Status: StatusDown, Message: err.Error()}
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return CheckResult{Status: StatusDown, Latency: lat, Message: err.Error()}
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return CheckResult{
			Status:  StatusDown,
			Latency: lat,
			Message: resp.Status,
		}
	}

	return CheckResult{Status: StatusUp, Latency: lat, Message: resp.Status}
}
