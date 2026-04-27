package service

import (
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestIsKnowledgeReadyForHashSkip_RequiresCompletedEnabledKnowledge(t *testing.T) {
	knowledge := &types.Knowledge{
		FileType:     "pdf",
		ParseStatus:  types.ParseStatusProcessing,
		EnableStatus: "enabled",
	}
	if isKnowledgeReadyForHashSkip(knowledge, nil, nil) {
		t.Fatal("processing knowledge must not be skipped by file hash")
	}

	knowledge.ParseStatus = types.ParseStatusCompleted
	knowledge.EnableStatus = "disabled"
	if isKnowledgeReadyForHashSkip(knowledge, nil, nil) {
		t.Fatal("disabled knowledge must not be skipped by file hash")
	}
}

func TestIsKnowledgeReadyForHashSkip_TableRequiresSemanticChunks(t *testing.T) {
	knowledge := &types.Knowledge{
		FileName:     "orders.xlsx",
		ParseStatus:  types.ParseStatusCompleted,
		EnableStatus: "enabled",
	}
	if isKnowledgeReadyForHashSkip(knowledge, nil, nil) {
		t.Fatal("table knowledge without semantic chunks must not be skipped by file hash")
	}
	if isKnowledgeReadyForHashSkip(knowledge, []*types.Chunk{
		{ID: "row", ChunkType: types.ChunkTypeText, Content: "row text"},
	}, nil) {
		t.Fatal("table knowledge with only row text chunks must not be skipped by file hash")
	}
	if isKnowledgeReadyForHashSkip(knowledge, []*types.Chunk{
		{ID: "summary", ChunkType: types.ChunkTypeTableSummary, Content: "table summary"},
	}, errors.New("repository failed")) {
		t.Fatal("table knowledge must not be skipped when semantic chunk lookup fails")
	}
	if !isKnowledgeReadyForHashSkip(knowledge, []*types.Chunk{
		{ID: "summary", ChunkType: types.ChunkTypeTableSummary, Content: "table summary"},
	}, nil) {
		t.Fatal("completed table knowledge with semantic chunks should be skipped by file hash")
	}
}

func TestIsKnowledgeReadyForHashSkip_NonTableCompletedEnabled(t *testing.T) {
	knowledge := &types.Knowledge{
		FileType:     "pdf",
		ParseStatus:  types.ParseStatusCompleted,
		EnableStatus: "enabled",
	}
	if !isKnowledgeReadyForHashSkip(knowledge, nil, nil) {
		t.Fatal("completed non-table knowledge should be skipped by file hash")
	}
}
