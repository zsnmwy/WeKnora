package service

import (
	"testing"

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
