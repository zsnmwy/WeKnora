package service

import (
	"context"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type mockRetrieveEngineRegistry struct {
	service interfaces.RetrieveEngineService
}

func (m *mockRetrieveEngineRegistry) Register(indexService interfaces.RetrieveEngineService) error {
	m.service = indexService
	return nil
}

func (m *mockRetrieveEngineRegistry) GetRetrieveEngineService(engineType types.RetrieverEngineType) (interfaces.RetrieveEngineService, error) {
	return m.service, nil
}

func (m *mockRetrieveEngineRegistry) GetAllRetrieveEngineServices() []interfaces.RetrieveEngineService {
	if m.service == nil {
		return nil
	}
	return []interfaces.RetrieveEngineService{m.service}
}

type mockRetrieveEngineService struct{}

func (m *mockRetrieveEngineService) EngineType() types.RetrieverEngineType {
	return types.PostgresRetrieverEngineType
}
func (m *mockRetrieveEngineService) Support() []types.RetrieverType {
	return []types.RetrieverType{types.VectorRetrieverType, types.KeywordsRetrieverType}
}
func (m *mockRetrieveEngineService) Retrieve(context.Context, types.RetrieveParams) ([]*types.RetrieveResult, error) {
	return nil, nil
}
func (m *mockRetrieveEngineService) Index(context.Context, embedding.Embedder, *types.IndexInfo, []types.RetrieverType) error {
	return nil
}
func (m *mockRetrieveEngineService) BatchIndex(context.Context, embedding.Embedder, []*types.IndexInfo, []types.RetrieverType) error {
	return nil
}
func (m *mockRetrieveEngineService) EstimateStorageSize(context.Context, embedding.Embedder, []*types.IndexInfo, []types.RetrieverType) int64 {
	return 0
}
func (m *mockRetrieveEngineService) CopyIndices(context.Context, string, map[string]string, map[string]string, string, int, string) error {
	return nil
}
func (m *mockRetrieveEngineService) DeleteByChunkIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveEngineService) DeleteBySourceIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveEngineService) DeleteByKnowledgeIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveEngineService) BatchUpdateChunkEnabledStatus(context.Context, map[string]bool) error {
	return nil
}
func (m *mockRetrieveEngineService) BatchUpdateChunkTagID(context.Context, map[string]string) error {
	return nil
}

func TestBuildRetrievalParamsAppliesDefaultIndexingStrategy(t *testing.T) {
	registry := &mockRetrieveEngineRegistry{service: &mockRetrieveEngineService{}}
	engine, err := retriever.NewCompositeRetrieveEngine(registry, []types.RetrieverEngineParams{
		{RetrieverType: types.VectorRetrieverType, RetrieverEngineType: types.PostgresRetrieverEngineType},
		{RetrieverType: types.KeywordsRetrieverType, RetrieverEngineType: types.PostgresRetrieverEngineType},
	})
	require.NoError(t, err)

	svc := &knowledgeBaseService{}
	kb := &types.KnowledgeBase{
		ID:               "kb-1",
		Type:             types.KnowledgeBaseTypeDocument,
		EmbeddingModelID: "embedding-model",
	}

	params, err := svc.buildRetrievalParams(
		context.WithValue(context.Background(), types.TenantIDContextKey, uint64(10000)),
		engine,
		kb,
		types.SearchParams{
			QueryText:        "效果复盘",
			QueryEmbedding:   []float32{0.1, 0.2},
			VectorThreshold:  0.15,
			KeywordThreshold: 0.3,
			MatchCount:       10,
		},
		[]string{"kb-1"},
		50,
	)
	require.NoError(t, err)
	require.Len(t, params, 2)
	require.Equal(t, types.VectorRetrieverType, params[0].RetrieverType)
	require.Equal(t, types.KeywordsRetrieverType, params[1].RetrieverType)
	require.True(t, kb.IsVectorEnabled())
	require.True(t, kb.IsKeywordEnabled())
}
