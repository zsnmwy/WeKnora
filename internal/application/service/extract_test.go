package service

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestBuildChunkExtractTemplateUsesBaseForDefaultSchema(t *testing.T) {
	base := &types.PromptTemplateStructured{
		Description: "base description",
		Tags:        []string{"BaseRelation"},
		Examples: []types.GraphData{
			{
				Text: "base example",
				Node: []*types.GraphNode{{Name: "Base A"}},
			},
		},
	}
	config := &types.ExtractConfig{Enabled: true}

	got := buildChunkExtractTemplate(base, config)

	if got != base {
		t.Fatal("buildChunkExtractTemplate() should use base template for empty enabled config")
	}
}

func TestBuildChunkExtractTemplateUsesCustomSchemaWhenProvided(t *testing.T) {
	base := &types.PromptTemplateStructured{
		Description: "base description",
		Tags:        []string{"BaseRelation"},
		Examples: []types.GraphData{
			{Text: "base example"},
		},
	}
	config := &types.ExtractConfig{
		Enabled: true,
		Text:    "custom example",
		Tags:    []string{"CustomRelation"},
		Nodes: []*types.GraphNode{
			{Name: "Custom A"},
		},
		Relations: []*types.GraphRelation{
			{Node1: "Custom A", Node2: "Custom B", Type: "CustomRelation"},
		},
	}

	got := buildChunkExtractTemplate(base, config)

	if got == base {
		t.Fatal("buildChunkExtractTemplate() should create a custom template")
	}
	if got.Description != base.Description {
		t.Fatalf("Description = %q, want %q", got.Description, base.Description)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "CustomRelation" {
		t.Fatalf("Tags = %+v, want custom tags", got.Tags)
	}
	if len(got.Examples) != 1 || got.Examples[0].Text != "custom example" {
		t.Fatalf("Examples = %+v, want custom example", got.Examples)
	}
}

func TestBuildSampleDataDescriptionSupportsStringRows(t *testing.T) {
	service := &DataTableSummaryService{}
	result := &types.ToolResult{
		Data: map[string]interface{}{
			"rows": []map[string]string{
				{
					"笔记类型": "企业号",
					"维护阶段": "新发布",
				},
			},
		},
	}

	got := service.buildSampleDataDescription(result, 10)

	for _, want := range []string{"笔记类型", "企业号", "维护阶段", "新发布"} {
		if !strings.Contains(got, want) {
			t.Fatalf("sample description missing %q:\n%s", want, got)
		}
	}
}

func TestBuildChunksIncludeDeterministicTableStructure(t *testing.T) {
	service := &DataTableSummaryService{}
	resources := &extractionResources{
		knowledge: &types.Knowledge{
			ID:              "knowledge-1",
			TenantID:        10000,
			KnowledgeBaseID: "kb-1",
		},
	}
	schemaDesc := "Table name: k_knowledge_1\nColumns: 2\nRows: 1\n\nColumn info:\n- 笔记类型 (VARCHAR)\n- 维护阶段 (VARCHAR)"
	sampleDesc := "Sample data (first 10 rows):\n{\"笔记类型\":\"企业号\",\"维护阶段\":\"新发布\"}\n"

	chunks := service.buildChunks(resources, "# Table Summary\n\nTable name: k_knowledge_1", "# Table Column Information\n\nTable name: k_knowledge_1", schemaDesc, sampleDesc)

	if len(chunks) != 2 {
		t.Fatalf("buildChunks() returned %d chunks, want 2", len(chunks))
	}
	summary := chunks[0].Content
	columns := chunks[1].Content
	for _, want := range []string{"Actual Table Schema", "Columns: 2", "Rows: 1", "笔记类型", "维护阶段"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary chunk missing %q:\n%s", want, summary)
		}
		if !strings.Contains(columns, want) {
			t.Fatalf("column chunk missing %q:\n%s", want, columns)
		}
	}
	if !strings.Contains(summary, "Sample Rows") || !strings.Contains(summary, "企业号") {
		t.Fatalf("summary chunk missing sample rows:\n%s", summary)
	}
}

func TestBuildChunksLimitTableSemanticContentForEmbedding(t *testing.T) {
	service := &DataTableSummaryService{}
	resources := &extractionResources{
		knowledge: &types.Knowledge{
			ID:              "knowledge-1",
			TenantID:        10000,
			KnowledgeBaseID: "kb-1",
		},
	}
	schemaDesc := "Table name: k_knowledge_1\nColumns: 2\nRows: 500\n\nColumn info:\n- 推广套餐名 (VARCHAR)\n- 推广套餐话术 (VARCHAR)"
	longSample := "Sample data (first 10 rows):\n" + strings.Repeat("{\"推广套餐话术\":\"成都联通千兆宽带长期优惠，安装费减免，套餐详情包含大量说明文本。\"}\n", 300)
	longColumnDescription := strings.Repeat("推广套餐话术字段用于描述宽带套餐卖点、价格、安装费和办理条件。", 240)

	chunks := service.buildChunks(resources, "该表记录宽带推广产品信息。", longColumnDescription, schemaDesc, longSample)

	summary := chunks[0].Content
	columns := chunks[1].Content
	if len(summary) > tableSemanticMaxEmbeddingBytes {
		t.Fatalf("summary chunk length = %d, want <= %d", len(summary), tableSemanticMaxEmbeddingBytes)
	}
	if len(columns) > tableSemanticMaxEmbeddingBytes {
		t.Fatalf("column chunk length = %d, want <= %d", len(columns), tableSemanticMaxEmbeddingBytes)
	}
	for _, got := range []string{summary, columns} {
		if !utf8.ValidString(got) {
			t.Fatalf("semantic chunk is not valid UTF-8:\n%s", got)
		}
		if !strings.Contains(got, "Actual Table Schema") {
			t.Fatalf("semantic chunk missing table schema:\n%s", got)
		}
	}
	if !strings.Contains(summary, "Content truncated") {
		t.Fatalf("summary chunk should mark truncated sample content:\n%s", summary)
	}
	if !strings.Contains(columns, "Content truncated") {
		t.Fatalf("column chunk should mark truncated column content:\n%s", columns)
	}
}
