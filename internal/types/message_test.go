package types

import (
	"strings"
	"testing"
)

func TestMessageLLMContextContentIncludesStoredAttachments(t *testing.T) {
	msg := &Message{
		Content: "那里面有多少个API",
		Images: MessageImages{
			{Caption: "图片里写着接口清单"},
		},
		Attachments: MessageAttachments{
			{
				FileName: "交付中心api对接规范(1).pdf",
				FileType: ".pdf",
				FileSize: 2048,
				Content:  "创建订单接口 createOrder",
			},
		},
	}

	got := msg.LLMContextContent()
	for _, want := range []string{
		"那里面有多少个API",
		"[用户上传图片内容]",
		"图片里写着接口清单",
		"<attachments>",
		"交付中心api对接规范(1).pdf",
		"创建订单接口 createOrder",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("LLMContextContent() missing %q in:\n%s", want, got)
		}
	}
}

func TestMessageLLMContextContentPrefersRenderedContent(t *testing.T) {
	msg := &Message{
		Content:         "raw",
		RenderedContent: "rendered with retrieval context",
		Attachments: MessageAttachments{
			{FileName: "ignored.pdf", Content: "should not duplicate"},
		},
	}

	if got := msg.LLMContextContent(); got != "rendered with retrieval context" {
		t.Fatalf("LLMContextContent() = %q", got)
	}
}
