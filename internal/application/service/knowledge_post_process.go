package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/tracing/langfuse"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// KnowledgePostProcessService acts as an orchestrator for all post-processing tasks
// after a document has been parsed and split into chunks (including multimodal OCR/Caption).
type KnowledgePostProcessService struct {
	knowledgeRepo interfaces.KnowledgeRepository
	kbService     interfaces.KnowledgeBaseService
	chunkService  interfaces.ChunkService
	taskEnqueuer  interfaces.TaskEnqueuer
	redisClient   *redis.Client
}

func NewKnowledgePostProcessService(
	knowledgeRepo interfaces.KnowledgeRepository,
	kbService interfaces.KnowledgeBaseService,
	chunkService interfaces.ChunkService,
	taskEnqueuer interfaces.TaskEnqueuer,
	redisClient *redis.Client,
) interfaces.TaskHandler {
	return &KnowledgePostProcessService{
		knowledgeRepo: knowledgeRepo,
		kbService:     kbService,
		chunkService:  chunkService,
		taskEnqueuer:  taskEnqueuer,
		redisClient:   redisClient,
	}
}

// Handle implements asynq handler for TypeKnowledgePostProcess.
func (s *KnowledgePostProcessService) Handle(ctx context.Context, task *asynq.Task) error {
	var payload types.KnowledgePostProcessPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal knowledge post process payload: %w", err)
	}

	logger.Infof(ctx, "[KnowledgePostProcess] Orchestrating post processing for knowledge: %s", payload.KnowledgeID)

	ctx = context.WithValue(ctx, types.TenantIDContextKey, payload.TenantID)
	if payload.Language != "" {
		ctx = context.WithValue(ctx, types.LanguageContextKey, payload.Language)
	}

	// 1. Fetch Knowledge and KB
	knowledge, err := s.knowledgeRepo.GetKnowledgeByIDOnly(ctx, payload.KnowledgeID)
	if err != nil {
		return fmt.Errorf("get knowledge %s: %w", payload.KnowledgeID, err)
	}
	if knowledge == nil {
		logger.Warnf(ctx, "[KnowledgePostProcess] Knowledge %s not found, aborting.", payload.KnowledgeID)
		return nil
	}

	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, payload.KnowledgeBaseID)
	if err != nil || kb == nil {
		return fmt.Errorf("get knowledge base %s: %w", payload.KnowledgeBaseID, err)
	}

	// 2. Fetch chunks needed by post-process pipelines. Text/image chunks
	// drive summary/question generation, while table_summary/table_column
	// chunks are the semantic source for spreadsheet graph/wiki extraction.
	chunks, err := s.chunkService.ListChunksByKnowledgeIDAndTypes(ctx, payload.KnowledgeID, postProcessChunkTypes)
	if err != nil {
		return fmt.Errorf("list chunks for knowledge %s: %w", payload.KnowledgeID, err)
	}

	semanticSelection := selectSemanticSourceChunks(knowledge, chunks)
	summarySourceChunks := selectSummarySourceChunks(semanticSelection, chunks)

	// 3. Update ParseStatus to Completed
	// (Except if it's already completed or if it was marked as failed/deleting, but we'll just set it to completed if it's processing)
	if knowledge.ParseStatus == types.ParseStatusProcessing {
		knowledge.ParseStatus = types.ParseStatusCompleted
		knowledge.UpdatedAt = time.Now()

		// Setup summary status
		if len(summarySourceChunks) > 0 {
			knowledge.SummaryStatus = types.SummaryStatusPending
		} else {
			knowledge.SummaryStatus = types.SummaryStatusNone
		}

		if err := s.knowledgeRepo.UpdateKnowledge(ctx, knowledge); err != nil {
			logger.Warnf(ctx, "[KnowledgePostProcess] Failed to update knowledge status to completed: %v", err)
		} else {
			logger.Infof(ctx, "[KnowledgePostProcess] Knowledge %s marked as completed.", payload.KnowledgeID)
		}
	}

	// 4. Spawn Summary and Question Tasks
	if len(summarySourceChunks) > 0 {
		s.enqueueSummaryGenerationTask(ctx, payload)
		// Question generation only makes sense for RAG indexing (improves chunk recall).
		// Skip when only Wiki/Graph is enabled without vector/keyword search.
		if kb.NeedsEmbeddingModel() && shouldGenerateQuestionsFromPostProcess(semanticSelection) {
			s.enqueueQuestionGenerationIfEnabled(ctx, payload, kb)
		}
	}

	// 5. Spawn Graph RAG Tasks — only when graph indexing is enabled in IndexingStrategy
	if kb.IsGraphEnabled() {
		if semanticSelection.MissingTableSemantic {
			logger.Warnf(ctx, "[KnowledgePostProcess] Spreadsheet knowledge %s has no table_summary/table_column chunks; skip graph extraction instead of scanning row-level text chunks", payload.KnowledgeID)
		} else {
			logger.Infof(ctx, "[KnowledgePostProcess] Spawning Graph RAG extract tasks for %d %s chunks", len(semanticSelection.Chunks), semanticSelection.Mode)
		}
		for _, chunk := range semanticSelection.Chunks {
			err := NewChunkExtractTask(ctx, s.taskEnqueuer, payload.TenantID, chunk.ID, kb.SummaryModelID)
			if err != nil {
				logger.Errorf(ctx, "[KnowledgePostProcess] Failed to create chunk extract task for %s: %v", chunk.ID, err)
			}
		}
	}

	// 6. Spawn Wiki Ingest Task if wiki indexing is enabled in IndexingStrategy
	if kb.IndexingStrategy.WikiEnabled && (len(summarySourceChunks) > 0 || len(semanticSelection.Chunks) > 0 || semanticSelection.MissingTableSemantic) {
		EnqueueWikiIngest(ctx, s.taskEnqueuer, s.redisClient, payload.TenantID, payload.KnowledgeBaseID, payload.KnowledgeID)
		logger.Infof(ctx, "[KnowledgePostProcess] Enqueued wiki ingest task for %s", payload.KnowledgeID)
	}
	return nil
}

func (s *KnowledgePostProcessService) enqueueSummaryGenerationTask(ctx context.Context, payload types.KnowledgePostProcessPayload) {
	if s.taskEnqueuer == nil {
		return
	}

	taskPayload := types.SummaryGenerationPayload{
		TenantID:        payload.TenantID,
		KnowledgeBaseID: payload.KnowledgeBaseID,
		KnowledgeID:     payload.KnowledgeID,
		Language:        payload.Language,
	}
	langfuse.InjectTracing(ctx, &taskPayload)
	payloadBytes, err := json.Marshal(taskPayload)
	if err != nil {
		logger.Warnf(ctx, "[KnowledgePostProcess] Failed to marshal summary generation payload: %v", err)
		return
	}

	task := asynq.NewTask(types.TypeSummaryGeneration, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	if _, err := s.taskEnqueuer.Enqueue(task); err != nil {
		logger.Warnf(ctx, "[KnowledgePostProcess] Failed to enqueue summary generation for %s: %v", payload.KnowledgeID, err)
	} else {
		logger.Infof(ctx, "[KnowledgePostProcess] Enqueued summary generation task for %s", payload.KnowledgeID)
	}
}

func (s *KnowledgePostProcessService) enqueueQuestionGenerationIfEnabled(ctx context.Context, payload types.KnowledgePostProcessPayload, kb *types.KnowledgeBase) {
	if s.taskEnqueuer == nil {
		return
	}

	if kb.QuestionGenerationConfig == nil || !kb.QuestionGenerationConfig.Enabled {
		return
	}

	questionCount := kb.QuestionGenerationConfig.QuestionCount
	if questionCount <= 0 {
		questionCount = 3
	}
	if questionCount > 10 {
		questionCount = 10
	}

	taskPayload := types.QuestionGenerationPayload{
		TenantID:        payload.TenantID,
		KnowledgeBaseID: payload.KnowledgeBaseID,
		KnowledgeID:     payload.KnowledgeID,
		QuestionCount:   questionCount,
		Language:        payload.Language,
	}
	langfuse.InjectTracing(ctx, &taskPayload)
	payloadBytes, err := json.Marshal(taskPayload)
	if err != nil {
		logger.Warnf(ctx, "[KnowledgePostProcess] Failed to marshal question generation payload: %v", err)
		return
	}

	task := asynq.NewTask(types.TypeQuestionGeneration, payloadBytes, asynq.Queue("low"), asynq.MaxRetry(3))
	if _, err := s.taskEnqueuer.Enqueue(task); err != nil {
		logger.Warnf(ctx, "[KnowledgePostProcess] Failed to enqueue question generation for %s: %v", payload.KnowledgeID, err)
	} else {
		logger.Infof(ctx, "[KnowledgePostProcess] Enqueued question generation task for %s (count=%d)", payload.KnowledgeID, questionCount)
	}
}
