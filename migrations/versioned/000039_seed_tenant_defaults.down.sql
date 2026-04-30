-- Rollback migration: 000039_seed_tenant_defaults
-- This migration backfills operational defaults for existing tenants. There is
-- no safe destructive rollback because the seeded values may be edited later.
DO $$ BEGIN RAISE NOTICE '[Migration 000039] No-op rollback for tenant default backfill'; END $$;
