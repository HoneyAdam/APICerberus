-- Migration: 005_playground_templates (rollback)
-- Description: Remove playground templates table

DROP INDEX IF EXISTS idx_playground_templates_method;
DROP INDEX IF EXISTS idx_playground_templates_user_id;
DROP TABLE IF EXISTS playground_templates;
