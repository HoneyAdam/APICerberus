-- Migration: 001_init_core_tables (rollback)
-- Description: Remove initial core tables

DROP INDEX IF EXISTS idx_credit_transactions_created_at;
DROP INDEX IF EXISTS idx_credit_transactions_type;
DROP INDEX IF EXISTS idx_credit_transactions_user_id;
DROP TABLE IF EXISTS credit_transactions;

DROP INDEX IF EXISTS idx_api_keys_status;
DROP INDEX IF EXISTS idx_api_keys_key_hash;
DROP INDEX IF EXISTS idx_api_keys_user_id;
DROP TABLE IF EXISTS api_keys;

DROP INDEX IF EXISTS idx_users_status;
DROP INDEX IF EXISTS idx_users_role;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
