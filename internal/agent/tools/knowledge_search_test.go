package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
)

func TestKnowledgeSearchGetEnrichedPassageIncludesImageInfo(t *testing.T) {
	tool := &KnowledgeSearchTool{}

	imageInfoJSON, err := json.Marshal([]types.ImageInfo{
		{
			URL:     "local://image-1.png",
			Caption: "流程图展示了内容运营的标准化路径",
			OCRText: "内容制作流程及人才标准",
		},
	})
	if err != nil {
		t.Fatalf("marshal image info: %v", err)
	}

	result := &types.SearchResult{
		Content:   "这是正文内容",
		ImageInfo: string(imageInfoJSON),
	}

	got := tool.getEnrichedPassage(context.Background(), result)

	wantParts := []string{
		"这是正文内容",
		"Image Caption: 流程图展示了内容运营的标准化路径",
		"Image Text: 内容制作流程及人才标准",
	}

	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("enriched passage missing %q, got: %s", want, got)
		}
	}
}
