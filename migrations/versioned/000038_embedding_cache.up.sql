-- Migration: 000038_embedding_cache
-- Description: Persist reusable embedding vectors independently from live index rows.

DO $$
BEGIN
    IF current_setting('app.skip_embedding', true) = 'true' THEN
        RAISE NOTICE 'Skipping migration 000038_embedding_cache (app.skip_embedding=true)';
        RETURN;
    END IF;

    CREATE EXTENSION IF NOT EXISTS vector;

    CREATE TABLE IF NOT EXISTS embedding_cache (
        id SERIAL PRIMARY KEY,
        created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
        model_id VARCHAR(128) NOT NULL DEFAULT '',
        model_name VARCHAR(255) NOT NULL,
        dimension INTEGER NOT NULL,
        input_hash VARCHAR(64) NOT NULL,
        embedding halfvec NOT NULL,
        last_used_at TIMESTAMP WITH TIME ZONE,
        reuse_hit_count BIGINT NOT NULL DEFAULT 0
    );

    CREATE UNIQUE INDEX IF NOT EXISTS embedding_cache_unique_input
        ON embedding_cache(model_id, model_name, dimension, input_hash);

    CREATE INDEX IF NOT EXISTS idx_embedding_cache_model
        ON embedding_cache(model_id, model_name, dimension);

    RAISE NOTICE '[Migration 000038] embedding_cache table ready';
END $$;
