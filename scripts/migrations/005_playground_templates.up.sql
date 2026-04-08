-- Migration: 005_playground_templates
-- Description: Add playground templates table
-- Created: 2026-04-07

CREATE TABLE IF NOT EXISTS playground_templates (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    method TEXT NOT NULL DEFAULT 'GET',
    path TEXT NOT NULL DEFAULT '/',
    headers TEXT NOT NULL DEFAULT '{}',
    query_params TEXT NOT NULL DEFAULT '{}',
    body TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_playground_templates_user_id ON playground_templates(user_id);
CREATE INDEX IF NOT EXISTS idx_playground_templates_method ON playground_templates(method);
