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
