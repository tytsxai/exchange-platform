package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
)

type EventType string

const (
	// 用户认证
	EventLogin       EventType = "USER_LOGIN"
	EventLogout      EventType = "USER_LOGOUT"
	EventLoginFailed EventType = "USER_LOGIN_FAILED"

	// API Key 管理
	EventAPIKeyCreated EventType = "API_KEY_CREATED"
	EventAPIKeyDeleted EventType = "API_KEY_DELETED"

	// 交易操作
	EventOrderCreated  EventType = "ORDER_CREATED"
	EventOrderCanceled EventType = "ORDER_CANCELED"

	// 资金操作
	EventWithdrawRequest  EventType = "WITHDRAW_REQUEST"
	EventWithdrawApproved EventType = "WITHDRAW_APPROVED"
	EventWithdrawRejected EventType = "WITHDRAW_REJECTED"
	EventDepositConfirmed EventType = "DEPOSIT_CONFIRMED"

	// 管理员操作
	EventAdminAction   EventType = "ADMIN_ACTION"
	EventConfigChanged EventType = "CONFIG_CHANGED"
	EventUserFrozen    EventType = "USER_FROZEN"
	EventUserUnfrozen  EventType = "USER_UNFROZEN"
)

type AuditLog struct {
	ID         int64     `json:"id"`
	EventType  EventType `json:"eventType"`
	UserID     int64     `json:"userId"`
	ActorID    int64     `json:"actorId"`    // 操作者（管理员审批时）
	IP         string    `json:"ip"`
	UserAgent  string    `json:"userAgent"`
	Resource   string    `json:"resource"`   // 操作的资源类型
	ResourceID string    `json:"resourceId"` // 资源ID
	Action     string    `json:"action"`     // 具体动作
	Params     string    `json:"params"`     // JSON格式的参数（脱敏后）
	Result     string    `json:"result"`     // SUCCESS/FAILED
	ErrorMsg   string    `json:"errorMsg"`
	Timestamp  int64     `json:"timestamp"`
	RequestID  string    `json:"requestId"`
}

type Logger interface {
	Log(ctx context.Context, log *AuditLog) error
	Query(ctx context.Context, filter *QueryFilter) ([]*AuditLog, error)
}

type QueryFilter struct {
	UserID    int64
	EventType EventType
	StartTime int64
	EndTime   int64
	Limit     int
	Offset    int
}

const (
	ResultSuccess = "SUCCESS"
	ResultFailed  = "FAILED"
)

// NewLog 创建审计日志。Timestamp 使用 Unix 毫秒。
func NewLog(eventType EventType, userID int64) *AuditLog {
	return &AuditLog{
		EventType: eventType,
		UserID:    userID,
		Timestamp: time.Now().UnixMilli(),
		Result:    ResultSuccess,
		Params:    "{}",
	}
}

// WithIP 设置IP。
func (l *AuditLog) WithIP(ip string) *AuditLog {
	if l == nil {
		return nil
	}
	l.IP = ip
	return l
}

// WithResource 设置资源。
func (l *AuditLog) WithResource(resource, resourceID string) *AuditLog {
	if l == nil {
		return nil
	}
	l.Resource = resource
	l.ResourceID = resourceID
	return l
}

// WithParams 设置参数（自动脱敏敏感字段）。
func (l *AuditLog) WithParams(params map[string]interface{}) *AuditLog {
	if l == nil {
		return nil
	}
	safe := SanitizeParams(params)
	b, err := json.Marshal(safe)
	if err != nil {
		l.Params = "{}"
		return l
	}
	l.Params = string(b)
	return l
}

// WithResult 设置结果。
func (l *AuditLog) WithResult(success bool, errMsg string) *AuditLog {
	if l == nil {
		return nil
	}
	if success {
		l.Result = ResultSuccess
		l.ErrorMsg = ""
		return l
	}
	l.Result = ResultFailed
	l.ErrorMsg = errMsg
	return l
}

// SanitizeParams 脱敏敏感参数。
func SanitizeParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return map[string]interface{}{}
	}

	out := make(map[string]interface{}, len(params))
	for k, v := range params {
		out[k] = sanitizeValue(k, v)
	}
	return out
}

func sanitizeValue(key string, value interface{}) interface{} {
	if isSensitiveKey(key) {
		return "***"
	}

	switch typed := value.(type) {
	case map[string]interface{}:
		return SanitizeParams(typed)
	case []interface{}:
		cp := make([]interface{}, 0, len(typed))
		for i, item := range typed {
			// 数组元素使用索引作为 key，避免父级 key 误判
			elemKey := fmt.Sprintf("[%d]", i)
			if m, ok := item.(map[string]interface{}); ok {
				cp = append(cp, SanitizeParams(m))
			} else {
				cp = append(cp, sanitizeValue(elemKey, item))
			}
		}
		return cp
	case string:
		if shouldMaskPartial(key, typed) {
			return maskPreserveEnds(typed, 2, 2)
		}
		return typed
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if k == "" {
		return false
	}
	return strings.Contains(k, "password") ||
		strings.Contains(k, "passwd") ||
		strings.Contains(k, "pwd") ||
		strings.Contains(k, "secret") ||
		strings.Contains(k, "token") ||
		strings.Contains(k, "apikey") ||
		strings.Contains(k, "api_key") ||
		(k == "key") ||
		strings.HasSuffix(k, "_key") ||
		strings.Contains(k, "privatekey") ||
		strings.Contains(k, "private_key")
}

func shouldMaskPartial(key, value string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	if strings.Contains(k, "phone") || strings.Contains(k, "mobile") || strings.Contains(k, "tel") {
		return true
	}

	// 值本身看起来像手机号/账号：数字占比高且长度足够
	digits := 0
	for _, r := range value {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if len(value) >= 7 && digits >= len(value)-2 {
		return true
	}
	return false
}

func maskPreserveEnds(s string, prefixKeep, suffixKeep int) string {
	runes := []rune(s)
	if prefixKeep < 0 {
		prefixKeep = 0
	}
	if suffixKeep < 0 {
		suffixKeep = 0
	}
	if len(runes) <= prefixKeep+suffixKeep {
		return "***"
	}
	maskedLen := len(runes) - prefixKeep - suffixKeep
	return string(runes[:prefixKeep]) + strings.Repeat("*", maskedLen) + string(runes[len(runes)-suffixKeep:])
}

// DBLogger 使用 PostgreSQL（database/sql）实现审计日志存储，默认异步写入以避免影响主业务流程。
//
// 说明：
// - 表名固定为 audit_logs（append-only）
// - 应用需自行 import PostgreSQL driver（如 github.com/lib/pq）
type DBLogger struct {
	db *sql.DB

	insertQueue chan *AuditLog
	cancel      context.CancelFunc
	wg          sync.WaitGroup

	onError func(error)
}

type DBLoggerOption func(*dbLoggerOptions)

type dbLoggerOptions struct {
	queueSize  int
	workers    int
	onError    func(error)
	timeNow    func() time.Time
	skipWorker bool
}

func WithQueueSize(size int) DBLoggerOption {
	return func(o *dbLoggerOptions) {
		if size > 0 {
			o.queueSize = size
		}
	}
}

func WithWorkers(n int) DBLoggerOption {
	return func(o *dbLoggerOptions) {
		if n > 0 {
			o.workers = n
		}
	}
}

func WithErrorHandler(fn func(error)) DBLoggerOption {
	return func(o *dbLoggerOptions) {
		if fn != nil {
			o.onError = fn
		}
	}
}

// WithSynchronousWrite 让 Log() 直接写数据库（不推荐在主链路使用）。
func WithSynchronousWrite() DBLoggerOption {
	return func(o *dbLoggerOptions) {
		o.skipWorker = true
	}
}

func NewDBLogger(db *sql.DB, opts ...DBLoggerOption) (*DBLogger, error) {
	if db == nil {
		return nil, errors.New("audit: db is nil")
	}

	cfg := dbLoggerOptions{
		queueSize: 4096,
		workers:   2,
		onError:   func(error) {},
		timeNow:   time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	l := &DBLogger{
		db:      db,
		onError: cfg.onError,
	}

	if cfg.skipWorker {
		return l, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.insertQueue = make(chan *AuditLog, cfg.queueSize)

	workers := cfg.workers
	for i := 0; i < workers; i++ {
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item := <-l.insertQueue:
					if item == nil {
						continue
					}
					if err := l.insert(ctx, item); err != nil {
						l.onError(err)
					}
				}
			}
		}()
	}

	return l, nil
}

// Close 关闭后台写入协程（可选调用）。
func (l *DBLogger) Close() {
	if l == nil {
		return
	}
	if l.cancel != nil {
		l.cancel()
	}
	l.wg.Wait()
}

func (l *DBLogger) Log(ctx context.Context, log *AuditLog) error {
	if l == nil || l.db == nil || log == nil {
		return nil
	}

	// 兜底：确保 Params 已脱敏且为 JSON 字符串
	if strings.TrimSpace(log.Params) == "" {
		log.Params = "{}"
	}
	if log.Timestamp == 0 {
		log.Timestamp = time.Now().UnixMilli()
	}

	if l.insertQueue == nil {
		// 同步写入模式：失败返回错误（调用方可选择忽略）
		return l.insert(ctx, log)
	}

	select {
	case l.insertQueue <- log:
	default:
		// 队列满：通知错误处理器，但不阻塞主流程
		if l.onError != nil {
			l.onError(errors.New("audit: queue full, log dropped"))
		}
	}
	return nil
}

func (l *DBLogger) Query(ctx context.Context, filter *QueryFilter) ([]*AuditLog, error) {
	if l == nil || l.db == nil {
		return nil, errors.New("audit: db logger not initialized")
	}

	var (
		where  []string
		args   []interface{}
		argIdx = 1
	)
	if filter != nil {
		if filter.UserID != 0 {
			where = append(where, fmt.Sprintf("user_id = $%d", argIdx))
			args = append(args, filter.UserID)
			argIdx++
		}
		if filter.EventType != "" {
			where = append(where, fmt.Sprintf("event_type = $%d", argIdx))
			args = append(args, filter.EventType)
			argIdx++
		}
		if filter.StartTime != 0 {
			where = append(where, fmt.Sprintf("timestamp >= $%d", argIdx))
			args = append(args, filter.StartTime)
			argIdx++
		}
		if filter.EndTime != 0 {
			where = append(where, fmt.Sprintf("timestamp <= $%d", argIdx))
			args = append(args, filter.EndTime)
			argIdx++
		}
	}

	query := `
SELECT id, event_type, user_id, actor_id, ip, user_agent, resource, resource_id, action, params, result, error_msg, timestamp, request_id
FROM audit_logs
`
	if len(where) > 0 {
		query += "WHERE " + strings.Join(where, " AND ") + "\n"
	}
	query += "ORDER BY timestamp DESC, id DESC\n"

	limit := 100
	offset := 0
	if filter != nil {
		if filter.Limit > 0 {
			limit = filter.Limit
		}
		if filter.Offset > 0 {
			offset = filter.Offset
		}
	}
	query += fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*AuditLog
	for rows.Next() {
		var item AuditLog
		if err := rows.Scan(
			&item.ID,
			&item.EventType,
			&item.UserID,
			&item.ActorID,
			&item.IP,
			&item.UserAgent,
			&item.Resource,
			&item.ResourceID,
			&item.Action,
			&item.Params,
			&item.Result,
			&item.ErrorMsg,
			&item.Timestamp,
			&item.RequestID,
		); err != nil {
			return nil, err
		}
		logs = append(logs, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (l *DBLogger) insert(ctx context.Context, log *AuditLog) error {
	const stmt = `
INSERT INTO audit_logs (
  id, event_type, user_id, actor_id, ip, user_agent, resource, resource_id, action, params, result, error_msg, timestamp, request_id
) VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14
)
`
	_, err := l.db.ExecContext(ctx, stmt,
		log.ID,
		log.EventType,
		log.UserID,
		log.ActorID,
		log.IP,
		log.UserAgent,
		log.Resource,
		log.ResourceID,
		log.Action,
		log.Params,
		log.Result,
		log.ErrorMsg,
		log.Timestamp,
		log.RequestID,
	)
	return err
}

// CreateTableSQL 提供 audit_logs 表结构（可用于初始化/迁移）。
//
// 分区：可在业务侧使用 PostgreSQL PARTITION BY RANGE(timestamp) 扩展。
const CreateTableSQL = `
CREATE TABLE IF NOT EXISTS audit_logs (
  id BIGINT PRIMARY KEY,
  event_type VARCHAR(64) NOT NULL,
  user_id BIGINT NOT NULL DEFAULT 0,
  actor_id BIGINT NOT NULL DEFAULT 0,
  ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  resource VARCHAR(128) NOT NULL DEFAULT '',
  resource_id VARCHAR(128) NOT NULL DEFAULT '',
  action VARCHAR(128) NOT NULL DEFAULT '',
  params JSONB NOT NULL DEFAULT '{}'::jsonb,
  result VARCHAR(16) NOT NULL DEFAULT 'SUCCESS',
  error_msg TEXT NOT NULL DEFAULT '',
  timestamp BIGINT NOT NULL,
  request_id VARCHAR(128) NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_ts ON audit_logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_ts ON audit_logs(user_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_event_ts ON audit_logs(event_type, timestamp DESC);
`

