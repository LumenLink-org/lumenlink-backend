# Database Migrations

This directory contains SQL migration files for the LumenLink database schema.

## Migration Files

- `0001_initial_schema.up.sql` - Creates all core tables (gateways, config_packs, attestations, etc.)
- `0001_initial_schema.down.sql` - Rolls back the initial schema
- `0002_add_timescale.up.sql` - Converts operator_metrics to TimescaleDB hypertable
- `0002_add_timescale.down.sql` - Removes TimescaleDB features

## Running Migrations

### Automatically (Recommended)

Migrations run automatically when the rendezvous server starts:

```bash
cd server/rendezvous
go run cmd/server/main.go
```

### Manually

Use the migration tool:

```bash
# Run all pending migrations
go run cmd/migrate/main.go -command up

# Rollback last migration
go run cmd/migrate/main.go -command down

# Check current version
go run cmd/migrate/main.go -command version
```

### Using Environment Variables

```bash
export DATABASE_URL="postgres://user:password@localhost:5432/lumenlink_dev?sslmode=disable"
go run cmd/migrate/main.go -command up
```

## Migration Naming Convention

Migrations follow the pattern: `NNNN_description.up.sql` and `NNNN_description.down.sql`

- `NNNN` - Sequential number (0001, 0002, etc.)
- `description` - Brief description of what the migration does
- `.up.sql` - Migration forward (applies changes)
- `.down.sql` - Migration backward (rolls back changes)

## Schema Overview

### Core Tables

1. **gateways** - Relay server information
   - Stores gateway IP, port, transport types, status
   - Indexed by region, status, last_seen

2. **config_packs** - Signed configuration packs
   - JSONB content with gateway lists
   - Indexed by region and version

3. **attestations** - Device attestation records
   - Play Integrity (Android) / DCAppAttest (iOS) tokens
   - Indexed by device_id and platform

4. **operator_metrics** - Time-series performance data
   - Converted to TimescaleDB hypertable
   - Automatic compression and retention policies
   - Continuous aggregates for hourly/daily stats

5. **discovery_logs** - Discovery channel analytics
   - Success/failure rates per channel
   - Latency measurements

6. **gateway_status_history** - Status change tracking
   - Audit trail of gateway status changes

7. **rate_limits** - API rate limiting
   - Per-device/IP rate limit tracking

## TimescaleDB Features

The `operator_metrics` table uses TimescaleDB for:

- **Automatic Partitioning**: Data partitioned by time (1 day chunks)
- **Compression**: Data older than 7 days is compressed
- **Retention**: Data older than 90 days is automatically deleted
- **Continuous Aggregates**: Pre-computed hourly and daily statistics

## Adding New Migrations

1. Create new migration files:
   ```bash
   touch migrations/0003_new_feature.up.sql
   touch migrations/0003_new_feature.down.sql
   ```

2. Write the SQL in `.up.sql` file
3. Write the rollback SQL in `.down.sql` file
4. Test both directions:
   ```bash
   go run cmd/migrate/main.go -command up
   go run cmd/migrate/main.go -command down
   go run cmd/migrate/main.go -command up
   ```

## Best Practices

1. **Always write rollback migrations** - Every `.up.sql` needs a `.down.sql`
2. **Test migrations** - Test both up and down directions
3. **Use transactions** - Migrations run in transactions automatically
4. **Add indexes** - Create indexes for frequently queried columns
5. **Document changes** - Add comments explaining complex migrations
6. **Never modify existing migrations** - Create new migrations instead

## Troubleshooting

### Migration fails with "dirty" state

If a migration fails partway through, the database will be marked as "dirty". You need to manually fix the issue and then:

```sql
-- Check migration version table
SELECT * FROM schema_migrations;

-- If dirty, manually fix the database state
-- Then update the version table
UPDATE schema_migrations SET dirty = false;
```

### TimescaleDB extension not found

Make sure TimescaleDB is installed:

```sql
CREATE EXTENSION IF NOT EXISTS timescaledb;
```

### Migration version mismatch

If you see version errors, check the current version:

```bash
go run cmd/migrate/main.go -command version
```

Then manually fix or rollback as needed.

## Production Considerations

1. **Backup before migrations** - Always backup production database
2. **Test in staging first** - Never run untested migrations in production
3. **Monitor migration time** - Large migrations may take time
4. **Use maintenance windows** - Schedule migrations during low traffic
5. **Have rollback plan** - Know how to rollback if something goes wrong
