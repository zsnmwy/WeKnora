package service

import (
	"context"
	"strings"
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

func TestBuildAgentQueryIncludesAttachments(t *testing.T) {
	req := &types.QARequest{
		Query: "总结这个文档",
		Attachments: types.MessageAttachments{
			{
				FileName: "交付中心api对接规范(1).pdf",
				FileType: ".pdf",
				FileSize: 206171,
				Content:  "交付中心 api 对接规范\n创建订单规范",
			},
		},
	}

	query, imageURLs := buildAgentQuery(req, false)

	if len(imageURLs) != 0 {
		t.Fatalf("imageURLs = %v, want empty", imageURLs)
	}
	for _, want := range []string{
		"总结这个文档",
		"<attachments>",
		`name="交付中心api对接规范(1).pdf"`,
		"<type>.pdf</type>",
		"交付中心 api 对接规范",
		"创建订单规范",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("agent query does not contain %q:\n%s", want, query)
		}
	}
}

func TestBuildAgentQueryPreservesVisionImagesAndAttachments(t *testing.T) {
	req := &types.QARequest{
		Query:     "看一下这个文件",
		ImageURLs: []string{"local://10000/image.png"},
		Attachments: types.MessageAttachments{
			{
				FileName: "notes.txt",
				FileType: ".txt",
				FileSize: 12,
				Content:  "hello",
			},
		},
	}

	query, imageURLs := buildAgentQuery(req, true)

	if len(imageURLs) != 1 || imageURLs[0] != "local://10000/image.png" {
		t.Fatalf("imageURLs = %v, want direct image URL", imageURLs)
	}
	if !strings.Contains(query, "<attachments>") || !strings.Contains(query, "hello") {
		t.Fatalf("agent query should include attachments:\n%s", query)
	}
}
