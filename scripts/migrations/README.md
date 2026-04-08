# APICerebrus Database Migrations

This directory contains database migration scripts for APICerebrus using SQLite.

## Migration Files

Each migration consists of two files:
- `{version}_{name}.up.sql` - Applies the migration
- `{version}_{name}.down.sql` - Rolls back the migration

## Quick Start

### Check Migration Status
```bash
./migrate-status.sh
```

### Apply All Pending Migrations
```bash
./migrate-up.sh
```

### Roll Back Last Migration
```bash
./migrate-down.sh
```

### Create a New Migration
```bash
./migrate-create.sh add_user_preferences
```

## Available Scripts

### migrate-up.sh
Applies pending migrations to the database.

```bash
# Apply all pending migrations
./migrate-up.sh

# Apply to specific database
./migrate-up.sh -d /data/apicerberus.db

# Dry run (show what would be applied)
./migrate-up.sh -n

# Migrate to specific version
./migrate-up.sh -v 3
```

### migrate-down.sh
Rolls back migrations from the database.

```bash
# Roll back 1 migration
./migrate-down.sh

# Roll back 3 migrations
./migrate-down.sh -s 3

# Roll back to specific version
./migrate-down.sh -v 2

# Dry run
./migrate-down.sh -n

# Force without confirmation
./migrate-down.sh -f
```

### migrate-status.sh
Shows current migration status.

```bash
./migrate-status.sh
./migrate-status.sh -d /data/apicerberus.db
```

### migrate-create.sh
Creates a new migration file pair.

```bash
./migrate-create.sh add_user_preferences
./migrate-create.sh create_audit_table
```

## Migration Naming Convention

- Use `snake_case` for migration names
- Be descriptive: `add_user_preferences`, `create_audit_table`
- Don't include version numbers in names
- Version numbers are automatically assigned

## Writing Migrations

### Up Migration
```sql
-- Migration: 006_add_user_preferences
-- Description: Add user preferences column
-- Created: 2026-04-07

ALTER TABLE users ADD COLUMN preferences TEXT NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS idx_users_preferences ON users(preferences);
```

### Down Migration
```sql
-- Migration: 006_add_user_preferences (rollback)
-- Description: Rollback add user preferences column

DROP INDEX IF EXISTS idx_users_preferences;
-- Note: SQLite doesn't support DROP COLUMN directly
-- You may need to recreate the table without the column
```

## Best Practices

1. **Always write down migrations** - Even if complex, provide a way to roll back
2. **Test migrations** - Use dry run mode before applying
3. **Backup before rollback** - Rollbacks can cause data loss
4. **Keep migrations small** - Easier to debug and rollback
5. **Use transactions** - Each migration runs in a transaction automatically
6. **Add indexes separately** - Consider separate migrations for large tables

## SQLite Limitations

SQLite has some limitations to be aware of:
- `ALTER TABLE` is limited (can't DROP COLUMN directly)
- Some operations require table recreation
- Foreign key checks can be disabled during migrations if needed

### Example: Dropping a Column
```sql
-- Since SQLite doesn't support DROP COLUMN, recreate the table:

-- 1. Create new table without the column
CREATE TABLE users_new (
    id TEXT PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    -- ... other columns except the one being dropped
);

-- 2. Copy data
INSERT INTO users_new SELECT id, email, ... FROM users;

-- 3. Drop old table
DROP TABLE users;

-- 4. Rename new table
ALTER TABLE users_new RENAME TO users;

-- 5. Recreate indexes
CREATE INDEX ...
```

## Environment Variables

- `DB_PATH` - Path to SQLite database file
- `DRY_RUN` - Set to `true` for dry run mode
- `LOG_FILE` - Path to log file
- `FORCE` - Set to `true` to skip confirmations

## Troubleshooting

### Migration Failed
1. Check the error message
2. Fix the migration SQL
3. If partially applied, you may need to manually fix the database
4. Check `schema_migrations` table for applied versions

### Database Locked
- Ensure APICerebrus is not running
- Check for other processes accessing the database
- SQLite WAL files (`.db-wal`, `.db-shm`) may need cleanup

### Version Mismatch
- Check current version: `./migrate-status.sh`
- Force specific version: `./migrate-up.sh -v N`
- Manual fix: Edit `schema_migrations` table directly (careful!)
