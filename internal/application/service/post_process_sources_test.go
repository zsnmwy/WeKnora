package service

import (
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestSelectSemanticSourceChunks_TableUsesOnlySemanticChunks(t *testing.T) {
	knowledge := &types.Knowledge{FileType: "xlsx", Title: "orders.xlsx"}
	chunks := make([]*types.Chunk, 0, 2292)
	for i := 0; i < 2290; i++ {
		chunks = append(chunks, &types.Chunk{
			ID:         fmt.Sprintf("row-%d", i),
			ChunkType:  types.ChunkTypeText,
			Content:    "row text",
			ChunkIndex: i,
		})
	}
	chunks = append(chunks,
		&types.Chunk{ID: "summary", ChunkType: types.ChunkTypeTableSummary, Content: "table summary", ChunkIndex: 2290},
		&types.Chunk{ID: "columns", ChunkType: types.ChunkTypeTableColumn, Content: "column descriptions", ChunkIndex: 2291},
	)

	got := selectSemanticSourceChunks(knowledge, chunks)
	if got.MissingTableSemantic {
		t.Fatal("table semantic chunks should be present")
	}
	if got.Mode != "table_semantic" {
		t.Fatalf("mode = %q, want table_semantic", got.Mode)
	}
	if len(got.Chunks) != 2 {
		t.Fatalf("selected %d chunks, want 2", len(got.Chunks))
	}
	for _, chunk := range got.Chunks {
		if chunk.ChunkType != types.ChunkTypeTableSummary && chunk.ChunkType != types.ChunkTypeTableColumn {
			t.Fatalf("selected row-level chunk type %q", chunk.ChunkType)
		}
	}
}

func TestSelectSemanticSourceChunks_TableMissingSemanticSkipsRows(t *testing.T) {
	knowledge := &types.Knowledge{FileName: "orders.csv"}
	got := selectSemanticSourceChunks(knowledge, []*types.Chunk{
		{ID: "row-1", ChunkType: types.ChunkTypeText, Content: "row text", ChunkIndex: 1},
	})
	if !got.MissingTableSemantic {
		t.Fatal("expected missing table semantic signal")
	}
	if len(got.Chunks) != 0 {
		t.Fatalf("selected %d chunks, want 0", len(got.Chunks))
	}
}

func TestSelectSemanticSourceChunks_DocumentKeepsTextAndImageSources(t *testing.T) {
	knowledge := &types.Knowledge{FileType: "pdf"}
	got := selectSemanticSourceChunks(knowledge, []*types.Chunk{
		{ID: "text", ChunkType: types.ChunkTypeText, Content: "text", ChunkIndex: 1},
		{ID: "ocr", ChunkType: types.ChunkTypeImageOCR, Content: "ocr", ChunkIndex: 2},
		{ID: "caption", ChunkType: types.ChunkTypeImageCaption, Content: "caption", ChunkIndex: 3},
		{ID: "table", ChunkType: types.ChunkTypeTableSummary, Content: "table", ChunkIndex: 4},
	})
	if got.MissingTableSemantic {
		t.Fatal("non-table document should not report missing table semantic chunks")
	}
	if len(got.Chunks) != 3 {
		t.Fatalf("selected %d chunks, want 3", len(got.Chunks))
	}
}

func TestSelectSummarySourceChunks_TableUsesSemanticChunks(t *testing.T) {
	knowledge := &types.Knowledge{FileName: "orders.xlsx"}
	chunks := []*types.Chunk{
		{ID: "row-1", ChunkType: types.ChunkTypeText, Content: "row text", ChunkIndex: 1},
		{ID: "table-summary", ChunkType: types.ChunkTypeTableSummary, Content: "table summary", ChunkIndex: 2},
	}

	selection := selectSemanticSourceChunks(knowledge, chunks)
	got := selectSummarySourceChunks(selection, chunks)
	if len(got) != 1 {
		t.Fatalf("selected %d summary chunks, want 1", len(got))
	}
	if got[0].ID != "table-summary" {
		t.Fatalf("selected %q, want table semantic chunk", got[0].ID)
	}
	if shouldGenerateQuestionsFromPostProcess(selection) {
		t.Fatal("table documents must not generate row-level recall questions")
	}
}

func TestSelectSummarySourceChunks_DocumentKeepsTextLikeChunks(t *testing.T) {
	knowledge := &types.Knowledge{FileName: "report.pdf"}
	chunks := []*types.Chunk{
		{ID: "text", ChunkType: types.ChunkTypeText, Content: "text", ChunkIndex: 1},
		{ID: "ocr", ChunkType: types.ChunkTypeImageOCR, Content: "ocr", ChunkIndex: 2},
		{ID: "table-summary", ChunkType: types.ChunkTypeTableSummary, Content: "table summary", ChunkIndex: 3},
	}

	selection := selectSemanticSourceChunks(knowledge, chunks)
	got := selectSummarySourceChunks(selection, chunks)
	if len(got) != 2 {
		t.Fatalf("selected %d summary chunks, want 2", len(got))
	}
	if !shouldGenerateQuestionsFromPostProcess(selection) {
		t.Fatal("non-table documents should keep question generation eligibility")
	}
}
