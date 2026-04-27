package retriever

import (
	"context"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/models/embedding"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

type mockEmbedder struct {
	mu     sync.Mutex
	inputs []string
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	embeddings, err := m.BatchEmbed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (m *mockEmbedder) BatchEmbed(_ context.Context, texts []string) ([][]float32, error) {
	m.mu.Lock()
	m.inputs = append(m.inputs, texts...)
	m.mu.Unlock()

	embeddings := make([][]float32, len(texts))
	for i, text := range texts {
		embeddings[i] = []float32{float32(len(text)), float32(i + 1)}
	}
	return embeddings, nil
}

func (m *mockEmbedder) BatchEmbedWithPool(ctx context.Context, model embedding.Embedder, texts []string) ([][]float32, error) {
	return model.BatchEmbed(ctx, texts)
}

func (m *mockEmbedder) GetModelName() string { return "mock-embedding" }
func (m *mockEmbedder) GetDimensions() int   { return 2 }
func (m *mockEmbedder) GetModelID() string   { return "mock-model-id" }

type mockRetrieveRepository struct {
	mu              sync.Mutex
	savedIndexInfos []*types.IndexInfo
	savedEmbeddings map[string][]float32
}

func (m *mockRetrieveRepository) Save(_ context.Context, indexInfo *types.IndexInfo, params map[string]any) error {
	return m.BatchSave(context.Background(), []*types.IndexInfo{indexInfo}, params)
}

func (m *mockRetrieveRepository) BatchSave(_ context.Context, indexInfoList []*types.IndexInfo, params map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.savedIndexInfos = append(m.savedIndexInfos, indexInfoList...)
	if m.savedEmbeddings == nil {
		m.savedEmbeddings = make(map[string][]float32)
	}
	if embeddingMap, ok := params["embedding"].(map[string][]float32); ok {
		for sourceID, embedding := range embeddingMap {
			m.savedEmbeddings[sourceID] = embedding
		}
	}
	return nil
}

func (m *mockRetrieveRepository) EstimateStorageSize(context.Context, []*types.IndexInfo, map[string]any) int64 {
	return 0
}
func (m *mockRetrieveRepository) DeleteByChunkIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveRepository) DeleteBySourceIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveRepository) CopyIndices(context.Context, string, map[string]string, map[string]string, string, int, string) error {
	return nil
}
func (m *mockRetrieveRepository) DeleteByKnowledgeIDList(context.Context, []string, int, string) error {
	return nil
}
func (m *mockRetrieveRepository) BatchUpdateChunkEnabledStatus(context.Context, map[string]bool) error {
	return nil
}
func (m *mockRetrieveRepository) BatchUpdateChunkTagID(context.Context, map[string]string) error {
	return nil
}
func (m *mockRetrieveRepository) EngineType() types.RetrieverEngineType {
	return types.PostgresRetrieverEngineType
}
func (m *mockRetrieveRepository) Retrieve(context.Context, types.RetrieveParams) ([]*types.RetrieveResult, error) {
	return nil, nil
}
func (m *mockRetrieveRepository) Support() []types.RetrieverType {
	return []types.RetrieverType{types.KeywordsRetrieverType, types.VectorRetrieverType}
}

type mockCachedRetrieveRepository struct {
	*mockRetrieveRepository
	cache      map[string][]float32
	savedCache map[string][]float32
}

func (m *mockCachedRetrieveRepository) FindEmbeddingCache(
	_ context.Context,
	_ string,
	_ string,
	_ int,
	inputHashes []string,
) (map[string][]float32, error) {
	result := make(map[string][]float32)
	for _, inputHash := range inputHashes {
		if embedding, ok := m.cache[inputHash]; ok {
			result[inputHash] = embedding
		}
	}
	return result, nil
}

func (m *mockCachedRetrieveRepository) SaveEmbeddingCache(
	_ context.Context,
	_ string,
	_ string,
	_ int,
	embeddings map[string][]float32,
) error {
	if m.savedCache == nil {
		m.savedCache = make(map[string][]float32)
	}
	for inputHash, embedding := range embeddings {
		m.savedCache[inputHash] = embedding
	}
	return nil
}

func TestBatchIndexReusesCachedEmbeddingsAndDeduplicatesNewInputs(t *testing.T) {
	cachedHash := calculateEmbeddingInputHash("cached")
	newHash := calculateEmbeddingInputHash("new")
	repo := &mockCachedRetrieveRepository{
		mockRetrieveRepository: &mockRetrieveRepository{},
		cache: map[string][]float32{
			cachedHash: {9, 9},
		},
	}
	embedder := &mockEmbedder{}
	service := NewKVHybridRetrieveEngine(repo, types.PostgresRetrieverEngineType)

	err := service.BatchIndex(context.Background(), embedder, []*types.IndexInfo{
		{SourceID: "s1", Content: "cached", SourceType: types.ChunkSourceType},
		{SourceID: "s2", Content: "new", SourceType: types.ChunkSourceType},
		{SourceID: "s3", Content: "new", SourceType: types.ChunkSourceType},
	}, []types.RetrieverType{types.VectorRetrieverType})
	require.NoError(t, err)

	require.Equal(t, []string{"new"}, embedder.inputs)
	require.Len(t, repo.savedIndexInfos, 3)
	require.Equal(t, []float32{9, 9}, repo.savedEmbeddings["s1"])
	require.Equal(t, repo.savedEmbeddings["s2"], repo.savedEmbeddings["s3"])
	require.Contains(t, repo.savedCache, newHash)
	require.NotContains(t, repo.savedCache, cachedHash)
}

func TestBatchIndexDeduplicatesInputsWithoutCacheRepository(t *testing.T) {
	repo := &mockRetrieveRepository{}
	embedder := &mockEmbedder{}
	service := NewKVHybridRetrieveEngine(repo, types.PostgresRetrieverEngineType)

	err := service.BatchIndex(context.Background(), embedder, []*types.IndexInfo{
		{SourceID: "s1", Content: "same", SourceType: types.ChunkSourceType},
		{SourceID: "s2", Content: "same", SourceType: types.ChunkSourceType},
	}, []types.RetrieverType{types.VectorRetrieverType})
	require.NoError(t, err)

	require.Equal(t, []string{"same"}, embedder.inputs)
	require.Len(t, repo.savedIndexInfos, 2)
	require.Equal(t, repo.savedEmbeddings["s1"], repo.savedEmbeddings["s2"])
}
