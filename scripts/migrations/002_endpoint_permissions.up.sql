-- Migration: 002_endpoint_permissions
-- Description: Add endpoint permissions table
-- Created: 2026-04-07

CREATE TABLE IF NOT EXISTS endpoint_permissions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    route_id TEXT NOT NULL,
    methods TEXT NOT NULL DEFAULT '[]',
    allowed INTEGER NOT NULL DEFAULT 1,
    rate_limits TEXT NOT NULL DEFAULT '{}',
    credit_cost TEXT NOT NULL DEFAULT '',
    valid_from TEXT NOT NULL DEFAULT '',
    valid_until TEXT NOT NULL DEFAULT '',
    allowed_days TEXT NOT NULL DEFAULT '[]',
    allowed_hours TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, route_id)
);

CREATE INDEX IF NOT EXISTS idx_endpoint_permissions_user_id ON endpoint_permissions(user_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_permissions_route_id ON endpoint_permissions(route_id);
CREATE INDEX IF NOT EXISTS idx_endpoint_permissions_allowed ON endpoint_permissions(allowed);
