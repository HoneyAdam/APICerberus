-- Migration: 002_endpoint_permissions (rollback)
-- Description: Remove endpoint permissions table

DROP INDEX IF EXISTS idx_endpoint_permissions_allowed;
DROP INDEX IF EXISTS idx_endpoint_permissions_route_id;
DROP INDEX IF EXISTS idx_endpoint_permissions_user_id;
DROP TABLE IF EXISTS endpoint_permissions;
