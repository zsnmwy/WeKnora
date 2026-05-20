package llmcontext

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

type fakeMessageRepo struct {
	messages []*types.Message
}

func (r *fakeMessageRepo) CreateMessage(ctx context.Context, message *types.Message) (*types.Message, error) {
	return message, nil
}

func (r *fakeMessageRepo) GetMessage(ctx context.Context, sessionID string, id string) (*types.Message, error) {
	return nil, nil
}

func (r *fakeMessageRepo) GetMessagesBySession(ctx context.Context, sessionID string, page int, pageSize int) ([]*types.Message, error) {
	return r.messages, nil
}

func (r *fakeMessageRepo) GetRecentMessagesBySession(ctx context.Context, sessionID string, limit int) ([]*types.Message, error) {
	return r.messages, nil
}

func (r *fakeMessageRepo) GetMessagesBySessionBeforeTime(ctx context.Context, sessionID string, beforeTime time.Time, limit int) ([]*types.Message, error) {
	return r.messages, nil
}

func (r *fakeMessageRepo) UpdateMessage(ctx context.Context, message *types.Message) error {
	return nil
}

func (r *fakeMessageRepo) UpdateMessageImages(ctx context.Context, sessionID, messageID string, images types.MessageImages) error {
	return nil
}

func (r *fakeMessageRepo) UpdateMessageRenderedContent(ctx context.Context, sessionID, messageID string, renderedContent string) error {
	return nil
}

func (r *fakeMessageRepo) DeleteMessage(ctx context.Context, sessionID string, id string) error {
	return nil
}

func (r *fakeMessageRepo) DeleteMessagesBySessionID(ctx context.Context, sessionID string) error {
	return nil
}

func (r *fakeMessageRepo) GetFirstMessageOfUser(ctx context.Context, sessionID string) (*types.Message, error) {
	return nil, nil
}

func (r *fakeMessageRepo) SearchMessagesByKeyword(ctx context.Context, tenantID uint64, keyword string, sessionIDs []string, limit int) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (r *fakeMessageRepo) GetMessagesByKnowledgeIDs(ctx context.Context, knowledgeIDs []string) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (r *fakeMessageRepo) GetMessagesByRequestIDs(ctx context.Context, requestIDs []string) ([]*types.MessageWithSession, error) {
	return nil, nil
}

func (r *fakeMessageRepo) GetKnowledgeIDsBySessionID(ctx context.Context, sessionID string) ([]string, error) {
	return nil, nil
}

func (r *fakeMessageRepo) UpdateMessageKnowledgeID(ctx context.Context, messageID string, knowledgeID string) error {
	return nil
}

func TestGetContextRebuildsWhenCacheOnlyHasSystemPrompt(t *testing.T) {
	ctx := context.Background()
	sessionID := "s1"
	storage := NewMemoryStorage()
	repo := &fakeMessageRepo{
		messages: []*types.Message{
			{
				SessionID: sessionID,
				RequestID: "r1",
				Role:      "user",
				Content:   "请读这个文件",
				CreatedAt: time.Unix(1, 0),
				Attachments: types.MessageAttachments{
					{FileName: "交付中心api对接规范(1).pdf", FileType: ".pdf", FileSize: 2048, Content: "创建订单规范"},
				},
			},
			{
				SessionID: sessionID,
				RequestID: "r1",
				Role:      "assistant",
				Content:   "已读取PDF。",
				CreatedAt: time.Unix(2, 0),
			},
		},
	}
	manager := NewContextManager(storage, repo)

	if err := storage.Save(ctx, sessionID, []chat.Message{{Role: "system", Content: "system prompt"}}); err != nil {
		t.Fatal(err)
	}

	got, err := manager.GetContext(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected system plus rebuilt user/assistant messages, got %d: %#v", len(got), got)
	}
	if got[0].Role != "system" || got[1].Role != "user" || got[2].Role != "assistant" {
		t.Fatalf("unexpected roles: %#v", got)
	}
	for _, want := range []string{"<attachments>", "交付中心api对接规范(1).pdf", "创建订单规范"} {
		if !strings.Contains(got[1].Content, want) {
			t.Fatalf("rebuilt user context missing %q in:\n%s", want, got[1].Content)
		}
	}
}
