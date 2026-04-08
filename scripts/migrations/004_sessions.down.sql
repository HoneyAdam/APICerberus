-- Migration: 004_sessions (rollback)
-- Description: Remove portal sessions table

DROP VIEW IF EXISTS sessions_active;

DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_token_hash;
DROP INDEX IF EXISTS idx_sessions_user_id;
DROP TABLE IF EXISTS sessions;
