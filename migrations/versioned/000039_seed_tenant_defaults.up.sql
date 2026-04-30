-- Migration: 000039_seed_tenant_defaults
-- Description: Backfill tenant-level defaults so existing tenants can use builtin models and web search.
DO $$ BEGIN RAISE NOTICE '[Migration 000039] Seeding tenant default configs and web search providers'; END $$;

WITH defaults AS (
    SELECT
        COALESCE(
            (SELECT conversation_config FROM tenants WHERE id = 10000 AND conversation_config IS NOT NULL LIMIT 1),
            jsonb_build_object(
                'prompt', '',
                'context_template', '{{contexts}}',
                'temperature', 0.7,
                'max_completion_tokens', 2048,
                'max_rounds', 5,
                'embedding_top_k', 30,
                'keyword_threshold', 0.3,
                'vector_threshold', 0.2,
                'rerank_top_k', 30,
                'rerank_threshold', 0.3,
                'enable_rewrite', true,
                'enable_query_expansion', true,
                'summary_model_id', 'builtin-deepseek-v4-pro',
                'rerank_model_id', 'builtin-rerank',
                'fallback_strategy', 'model',
                'fallback_response', 'Sorry, I am unable to answer this question.',
                'fallback_prompt', '',
                'rewrite_prompt_system', '',
                'rewrite_prompt_user', ''
            )
        ) AS conversation_config,
        COALESCE(
            (SELECT web_search_config FROM tenants WHERE id = 10000 AND web_search_config IS NOT NULL LIMIT 1),
            jsonb_build_object(
                'max_results', 5,
                'include_date', true,
                'compression_method', 'none',
                'blacklist', jsonb_build_array(),
                'embedding_model_id', 'builtin-embedding-3',
                'rerank_model_id', 'builtin-rerank'
            )
        ) AS web_search_config,
        COALESCE(
            (SELECT retrieval_config FROM tenants WHERE id = 10000 AND retrieval_config IS NOT NULL LIMIT 1),
            jsonb_build_object(
                'embedding_top_k', 30,
                'vector_threshold', 0.2,
                'keyword_threshold', 0.3,
                'rerank_top_k', 30,
                'rerank_threshold', 0.3,
                'rerank_model_id', 'builtin-rerank'
            )
        ) AS retrieval_config
)
UPDATE tenants t
SET
    conversation_config = COALESCE(t.conversation_config, defaults.conversation_config),
    web_search_config = COALESCE(t.web_search_config, defaults.web_search_config),
    retrieval_config = COALESCE(t.retrieval_config, defaults.retrieval_config),
    updated_at = NOW()
FROM defaults
WHERE t.deleted_at IS NULL
  AND (t.conversation_config IS NULL OR t.web_search_config IS NULL OR t.retrieval_config IS NULL);

INSERT INTO web_search_providers (
    id,
    tenant_id,
    name,
    provider,
    description,
    parameters,
    is_default,
    created_at,
    updated_at
)
SELECT
    uuid_generate_v4()::VARCHAR(36),
    t.id,
    p.name,
    p.provider,
    p.description,
    p.parameters,
    CASE
        WHEN EXISTS (
            SELECT 1 FROM web_search_providers existing_default
            WHERE existing_default.tenant_id = t.id
              AND existing_default.is_default = true
              AND existing_default.deleted_at IS NULL
        ) THEN false
        ELSE p.is_default
    END,
    NOW(),
    NOW()
FROM tenants t
JOIN web_search_providers p
  ON p.tenant_id = 10000
 AND p.deleted_at IS NULL
WHERE t.deleted_at IS NULL
  AND t.id <> 10000
  AND NOT EXISTS (
      SELECT 1 FROM web_search_providers existing
      WHERE existing.tenant_id = t.id
        AND existing.provider = p.provider
        AND existing.deleted_at IS NULL
  );

INSERT INTO web_search_providers (
    id,
    tenant_id,
    name,
    provider,
    description,
    parameters,
    is_default,
    created_at,
    updated_at
)
SELECT
    uuid_generate_v4()::VARCHAR(36),
    t.id,
    'DuckDuckGo',
    'duckduckgo',
    'Built-in DuckDuckGo web search',
    '{}'::jsonb,
    true,
    NOW(),
    NOW()
FROM tenants t
WHERE t.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM web_search_providers existing
      WHERE existing.tenant_id = t.id
        AND existing.deleted_at IS NULL
  );

DO $$ BEGIN RAISE NOTICE '[Migration 000039] Tenant defaults seeded successfully'; END $$;
