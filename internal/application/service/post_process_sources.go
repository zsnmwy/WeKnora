package service

import (
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	graphDefaultChunkTypes = []types.ChunkType{
		types.ChunkTypeText,
		types.ChunkTypeImageOCR,
		types.ChunkTypeImageCaption,
	}
	tableSemanticChunkTypes = []types.ChunkType{
		types.ChunkTypeTableSummary,
		types.ChunkTypeTableColumn,
	}
	postProcessChunkTypes = append(append([]types.ChunkType{}, graphDefaultChunkTypes...), tableSemanticChunkTypes...)
)

type sourceChunkSelection struct {
	Chunks               []*types.Chunk
	Mode                 string
	IsTableDocument      bool
	MissingTableSemantic bool
}

func isTableDocumentKnowledge(knowledge *types.Knowledge) bool {
	if knowledge == nil {
		return false
	}
	fileType := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(knowledge.FileType), "."))
	if fileType == "" {
		fileType = getFileType(knowledge.FileName)
	}
	switch fileType {
	case "csv", "xlsx", "xls":
		return true
	default:
		return false
	}
}

func selectSemanticSourceChunks(knowledge *types.Knowledge, chunks []*types.Chunk) sourceChunkSelection {
	if isTableDocumentKnowledge(knowledge) {
		selected := filterChunksByTypes(chunks, tableSemanticChunkTypes...)
		return sourceChunkSelection{
			Chunks:               selected,
			Mode:                 "table_semantic",
			IsTableDocument:      true,
			MissingTableSemantic: len(selected) == 0,
		}
	}

	return sourceChunkSelection{
		Chunks: filterChunksByTypes(chunks, graphDefaultChunkTypes...),
		Mode:   "document_text",
	}
}

func selectSummarySourceChunks(selection sourceChunkSelection, chunks []*types.Chunk) []*types.Chunk {
	if selection.IsTableDocument {
		textChunks := filterChunksByTypes(chunks, graphDefaultChunkTypes...)
		if len(textChunks) > 0 {
			return textChunks
		}
		return selection.Chunks
	}
	return filterChunksByTypes(chunks, graphDefaultChunkTypes...)
}

func shouldGenerateQuestionsFromPostProcess(selection sourceChunkSelection) bool {
	return !selection.IsTableDocument
}

func filterChunksByTypes(chunks []*types.Chunk, chunkTypes ...types.ChunkType) []*types.Chunk {
	if len(chunks) == 0 || len(chunkTypes) == 0 {
		return nil
	}
	allowed := make(map[types.ChunkType]bool, len(chunkTypes))
	for _, chunkType := range chunkTypes {
		allowed[chunkType] = true
	}
	out := make([]*types.Chunk, 0, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		if allowed[chunk.ChunkType] || (chunk.ChunkType == "" && allowed[types.ChunkTypeText]) {
			out = append(out, chunk)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ChunkIndex == out[j].ChunkIndex {
			return out[i].StartAt < out[j].StartAt
		}
		return out[i].ChunkIndex < out[j].ChunkIndex
	})
	return out
}

func buildMinimalTableWikiContent(docTitle string) (summaryLine string, summaryBody string) {
	title := strings.TrimSpace(docTitle)
	if title == "" {
		title = "表格文档"
	}
	return title, "# " + title + "\n\n该表格文档尚未生成表结构摘要。Wiki 暂不扫描行级数据；请等待表格摘要任务完成后重新处理。\n"
}
