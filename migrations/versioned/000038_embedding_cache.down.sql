-- Migration: 000038_embedding_cache (rollback)
-- Description: Drop reusable embedding cache.

DROP INDEX IF EXISTS idx_embedding_cache_model;
DROP INDEX IF EXISTS embedding_cache_unique_input;
DROP TABLE IF EXISTS embedding_cache;
