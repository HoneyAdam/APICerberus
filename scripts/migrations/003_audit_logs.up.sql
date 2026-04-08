-- Migration: 003_audit_logs
-- Description: Add audit logging table
-- Created: 2026-04-07

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL DEFAULT '',
    route_id TEXT NOT NULL DEFAULT '',
    route_name TEXT NOT NULL DEFAULT '',
    service_name TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    consumer_name TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT '',
    host TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    query TEXT NOT NULL DEFAULT '',
    status_code INTEGER NOT NULL DEFAULT 0,
    latency_ms INTEGER NOT NULL DEFAULT 0,
    bytes_in INTEGER NOT NULL DEFAULT 0,
    bytes_out INTEGER NOT NULL DEFAULT 0,
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    blocked INTEGER NOT NULL DEFAULT 0,
    block_reason TEXT NOT NULL DEFAULT '',
    request_headers TEXT NOT NULL DEFAULT '{}',
    request_body TEXT NOT NULL DEFAULT '',
    response_headers TEXT NOT NULL DEFAULT '{}',
    response_body TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_route_id ON audit_logs(route_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_status_code ON audit_logs(status_code);
CREATE INDEX IF NOT EXISTS idx_audit_logs_request_id ON audit_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_client_ip ON audit_logs(client_ip);
CREATE INDEX IF NOT EXISTS idx_audit_logs_blocked ON audit_logs(blocked);

-- Create a view for recent audit logs (last 24 hours)
CREATE VIEW IF NOT EXISTS audit_logs_recent AS
SELECT * FROM audit_logs
WHERE created_at > datetime('now', '-1 day')
ORDER BY created_at DESC;
