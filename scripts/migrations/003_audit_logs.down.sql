-- Migration: 003_audit_logs (rollback)
-- Description: Remove audit logging table

DROP VIEW IF EXISTS audit_logs_recent;

DROP INDEX IF EXISTS idx_audit_logs_blocked;
DROP INDEX IF EXISTS idx_audit_logs_client_ip;
DROP INDEX IF EXISTS idx_audit_logs_request_id;
DROP INDEX IF EXISTS idx_audit_logs_status_code;
DROP INDEX IF EXISTS idx_audit_logs_route_id;
DROP INDEX IF EXISTS idx_audit_logs_user_id;
DROP INDEX IF EXISTS idx_audit_logs_created_at;
DROP TABLE IF EXISTS audit_logs;
