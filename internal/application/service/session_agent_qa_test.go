package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestApplyModelSpecificAgentDefaults_DeepSeekUsesOneMillionContext(t *testing.T) {
	cfg := &types.AgentConfig{}
	model := &types.Model{
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			Provider: "deepseek",
			BaseURL:  "https://api.deepseek.com",
		},
	}

	applyModelSpecificAgentDefaults(context.Background(), cfg, model)

	if cfg.MaxContextTokens != types.DefaultDeepSeekMaxContextTokens {
		t.Fatalf("MaxContextTokens = %d, want %d", cfg.MaxContextTokens, types.DefaultDeepSeekMaxContextTokens)
	}
}

func TestApplyModelSpecificAgentDefaults_DoesNotAffectOtherProviders(t *testing.T) {
	cfg := &types.AgentConfig{}
	model := &types.Model{
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			Provider: "openai",
			BaseURL:  "https://api.openai.com/v1",
		},
	}

	applyModelSpecificAgentDefaults(context.Background(), cfg, model)

	if cfg.MaxContextTokens != types.DefaultMaxContextTokens {
		t.Fatalf("MaxContextTokens = %d, want %d", cfg.MaxContextTokens, types.DefaultMaxContextTokens)
	}
}

func TestApplyModelSpecificAgentDefaults_ExplicitConfigWins(t *testing.T) {
	cfg := &types.AgentConfig{MaxContextTokens: 12345}
	model := &types.Model{
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			Provider: "deepseek",
		},
	}

	applyModelSpecificAgentDefaults(context.Background(), cfg, model)

	if cfg.MaxContextTokens != 12345 {
		t.Fatalf("MaxContextTokens = %d, want explicit value 12345", cfg.MaxContextTokens)
	}
}

func TestIsDeepSeekModel_DetectsDeepSeekBaseURLWhenProviderEmpty(t *testing.T) {
	model := &types.Model{
		Source: types.ModelSourceRemote,
		Parameters: types.ModelParameters{
			BaseURL: "https://api.deepseek.com",
		},
	}

	if !isDeepSeekModel(model) {
		t.Fatal("expected DeepSeek model to be detected from base URL")
	}
}

func TestSelectAgentRerankModelID_PrefersDefaultActiveRerank(t *testing.T) {
	models := []*types.Model{
		{ID: "first", Type: types.ModelTypeRerank, Status: types.ModelStatusActive},
		{ID: "default", Type: types.ModelTypeRerank, Status: types.ModelStatusActive, IsDefault: true},
	}

	if got := selectAgentRerankModelID(models); got != "default" {
		t.Fatalf("selectAgentRerankModelID() = %q, want default", got)
	}
}

func TestSelectAgentRerankModelID_FallsBackToFirstActiveRerank(t *testing.T) {
	models := []*types.Model{
		{ID: "chat", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsDefault: true},
		{ID: "failed", Type: types.ModelTypeRerank, Status: types.ModelStatusDownloadFailed, IsDefault: true},
		{ID: "rerank", Type: types.ModelTypeRerank, Status: types.ModelStatusActive},
	}

	if got := selectAgentRerankModelID(models); got != "rerank" {
		t.Fatalf("selectAgentRerankModelID() = %q, want rerank", got)
	}
}

func TestSelectAgentRerankModelID_ReturnsEmptyWithoutActiveRerank(t *testing.T) {
	models := []*types.Model{
		nil,
		{ID: "chat", Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive},
		{ID: "downloading", Type: types.ModelTypeRerank, Status: types.ModelStatusDownloading},
	}

	if got := selectAgentRerankModelID(models); got != "" {
		t.Fatalf("selectAgentRerankModelID() = %q, want empty", got)
	}
}
