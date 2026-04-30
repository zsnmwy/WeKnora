package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/config"
	"github.com/Tencent/WeKnora/internal/types"
)

func TestCreateTenantAppliesDefaultsAndCopiesWebSearchProviders(t *testing.T) {
	t.Setenv("TENANT_AES_KEY", "12345678901234567890123456789012")

	ctx := context.Background()
	tenantRepo := newFakeTenantRepo(10001)
	modelRepo := fakeModelRepo{
		models: map[string]*types.Model{
			"builtin-deepseek-v4-pro": {
				ID: "builtin-deepseek-v4-pro", TenantID: 10000, Name: "deepseek-v4-pro",
				Type: types.ModelTypeKnowledgeQA, Status: types.ModelStatusActive, IsBuiltin: true,
			},
			"builtin-embedding-3": {
				ID: "builtin-embedding-3", TenantID: 10000, Name: "embedding-3",
				Type: types.ModelTypeEmbedding, Status: types.ModelStatusActive, IsBuiltin: true,
			},
			"builtin-rerank": {
				ID: "builtin-rerank", TenantID: 10000, Name: "rerank",
				Type: types.ModelTypeRerank, Status: types.ModelStatusActive, IsBuiltin: true,
			},
		},
	}
	webRepo := newFakeWebSearchProviderRepo()
	webRepo.providers[defaultBuiltinTenantID] = []*types.WebSearchProviderEntity{
		{
			ID: "template-tavily", TenantID: defaultBuiltinTenantID, Name: "Tavily Search",
			Provider: types.WebSearchProviderTypeTavily, IsDefault: true,
			Parameters: types.WebSearchProviderParameters{APIKey: "template-key"},
		},
		{
			ID: "template-duckduckgo", TenantID: defaultBuiltinTenantID, Name: "DuckDuckGo",
			Provider: types.WebSearchProviderTypeDuckDuckGo, IsDefault: false,
		},
	}

	svc := &tenantService{
		repo:                  tenantRepo,
		modelRepo:             modelRepo,
		webSearchProviderRepo: webRepo,
		config: &config.Config{
			Conversation: &config.ConversationConfig{
				MaxRounds:            5,
				EmbeddingTopK:        30,
				KeywordThreshold:     0.3,
				VectorThreshold:      0.2,
				RerankTopK:           30,
				RerankThreshold:      0.3,
				EnableRewrite:        true,
				EnableQueryExpansion: true,
				FallbackStrategy:     string(types.FallbackStrategyModel),
				Summary: &config.SummaryConfig{
					Temperature:         0.3,
					MaxCompletionTokens: 2048,
				},
			},
		},
	}

	tenant, err := svc.CreateTenant(ctx, &types.Tenant{Name: "member workspace"})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}

	if tenant.ConversationConfig == nil || tenant.ConversationConfig.SummaryModelID != "builtin-deepseek-v4-pro" {
		t.Fatalf("conversation default model not seeded: %+v", tenant.ConversationConfig)
	}
	if tenant.WebSearchConfig == nil || tenant.WebSearchConfig.MaxResults != 5 || !tenant.WebSearchConfig.IncludeDate {
		t.Fatalf("web search config not seeded: %+v", tenant.WebSearchConfig)
	}
	if tenant.RetrievalConfig == nil || tenant.RetrievalConfig.RerankModelID != "builtin-rerank" {
		t.Fatalf("retrieval config not seeded: %+v", tenant.RetrievalConfig)
	}

	providers := webRepo.providers[tenant.ID]
	if len(providers) != 2 {
		t.Fatalf("copied providers = %d, want 2: %+v", len(providers), providers)
	}
	if providers[0].TenantID != tenant.ID || providers[0].ID == "template-tavily" {
		t.Fatalf("provider copy did not reset identity: %+v", providers[0])
	}
	defaultCount := 0
	for _, provider := range providers {
		if provider.IsDefault {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Fatalf("default provider count = %d, want 1", defaultCount)
	}

	if err := svc.ensureDefaultWebSearchProviders(ctx, tenant.ID); err != nil {
		t.Fatalf("ensureDefaultWebSearchProviders() second pass error = %v", err)
	}
	if got := len(webRepo.providers[tenant.ID]); got != 2 {
		t.Fatalf("providers duplicated on second pass: got %d", got)
	}
}

func TestApplyDefaultTenantSettingsPreservesExplicitValues(t *testing.T) {
	ctx := context.Background()
	svc := &tenantService{
		modelRepo: fakeModelRepo{
			models: map[string]*types.Model{
				"builtin-deepseek-v4-pro": {
					ID: "builtin-deepseek-v4-pro", Type: types.ModelTypeKnowledgeQA,
					Status: types.ModelStatusActive, IsBuiltin: true,
				},
				"builtin-rerank": {
					ID: "builtin-rerank", Type: types.ModelTypeRerank,
					Status: types.ModelStatusActive, IsBuiltin: true,
				},
			},
		},
		config: &config.Config{
			Conversation: &config.ConversationConfig{
				MaxRounds:        5,
				EmbeddingTopK:    30,
				VectorThreshold:  0.2,
				KeywordThreshold: 0.3,
				RerankTopK:       30,
				RerankThreshold:  0.3,
				Summary:          &config.SummaryConfig{Temperature: 0.3, MaxCompletionTokens: 2048},
			},
		},
	}
	tenant := &types.Tenant{
		ID: 10002,
		ConversationConfig: &types.ConversationConfig{
			SummaryModelID:      "custom-chat",
			MaxCompletionTokens: 8192,
		},
		WebSearchConfig: &types.WebSearchConfig{
			MaxResults: 12,
		},
	}

	svc.applyDefaultTenantSettings(ctx, tenant)

	if tenant.ConversationConfig.SummaryModelID != "custom-chat" {
		t.Fatalf("explicit summary model was overwritten: %+v", tenant.ConversationConfig)
	}
	if tenant.ConversationConfig.MaxCompletionTokens != 8192 {
		t.Fatalf("explicit max completion tokens was overwritten: %+v", tenant.ConversationConfig)
	}
	if tenant.WebSearchConfig.MaxResults != 12 {
		t.Fatalf("explicit web search max results was overwritten: %+v", tenant.WebSearchConfig)
	}
	if tenant.ConversationConfig.RerankModelID != "builtin-rerank" {
		t.Fatalf("missing rerank model was not filled: %+v", tenant.ConversationConfig)
	}
}

type fakeTenantRepo struct {
	nextID  uint64
	tenants map[uint64]*types.Tenant
}

func newFakeTenantRepo(nextID uint64) *fakeTenantRepo {
	return &fakeTenantRepo{nextID: nextID, tenants: map[uint64]*types.Tenant{}}
}

func (r *fakeTenantRepo) CreateTenant(_ context.Context, tenant *types.Tenant) error {
	tenant.ID = r.nextID
	r.tenants[tenant.ID] = tenant
	return nil
}

func (r *fakeTenantRepo) GetTenantByID(_ context.Context, id uint64) (*types.Tenant, error) {
	tenant := r.tenants[id]
	if tenant == nil {
		return nil, fmt.Errorf("tenant not found")
	}
	return tenant, nil
}

func (r *fakeTenantRepo) ListTenants(_ context.Context) ([]*types.Tenant, error) {
	tenants := make([]*types.Tenant, 0, len(r.tenants))
	for _, tenant := range r.tenants {
		tenants = append(tenants, tenant)
	}
	return tenants, nil
}

func (r *fakeTenantRepo) SearchTenants(_ context.Context, _ string, _ uint64, _, _ int) ([]*types.Tenant, int64, error) {
	return nil, 0, nil
}

func (r *fakeTenantRepo) UpdateTenant(_ context.Context, tenant *types.Tenant) error {
	r.tenants[tenant.ID] = tenant
	return nil
}

func (r *fakeTenantRepo) DeleteTenant(_ context.Context, id uint64) error {
	delete(r.tenants, id)
	return nil
}

func (r *fakeTenantRepo) AdjustStorageUsed(_ context.Context, _ uint64, _ int64) error {
	return nil
}

type fakeModelRepo struct {
	models map[string]*types.Model
}

func (r fakeModelRepo) Create(_ context.Context, model *types.Model) error {
	r.models[model.ID] = model
	return nil
}

func (r fakeModelRepo) GetByID(_ context.Context, tenantID uint64, id string) (*types.Model, error) {
	model := r.models[id]
	if model == nil {
		return nil, nil
	}
	if model.TenantID == tenantID || model.IsBuiltin {
		return model, nil
	}
	return nil, nil
}

func (r fakeModelRepo) List(_ context.Context, tenantID uint64, modelType types.ModelType, source types.ModelSource) ([]*types.Model, error) {
	models := make([]*types.Model, 0, len(r.models))
	for _, model := range r.models {
		if model == nil {
			continue
		}
		if model.TenantID != tenantID && !model.IsBuiltin {
			continue
		}
		if modelType != "" && model.Type != modelType {
			continue
		}
		if source != "" && model.Source != source {
			continue
		}
		models = append(models, model)
	}
	return models, nil
}

func (r fakeModelRepo) Update(_ context.Context, model *types.Model) error {
	r.models[model.ID] = model
	return nil
}

func (r fakeModelRepo) Delete(_ context.Context, _ uint64, id string) error {
	delete(r.models, id)
	return nil
}

func (r fakeModelRepo) ClearDefaultByType(_ context.Context, _ uint, _ types.ModelType, _ string) error {
	return nil
}

type fakeWebSearchProviderRepo struct {
	providers map[uint64][]*types.WebSearchProviderEntity
	nextID    int
}

func newFakeWebSearchProviderRepo() *fakeWebSearchProviderRepo {
	return &fakeWebSearchProviderRepo{providers: map[uint64][]*types.WebSearchProviderEntity{}}
}

func (r *fakeWebSearchProviderRepo) Create(_ context.Context, provider *types.WebSearchProviderEntity) error {
	if provider.ID == "" {
		r.nextID++
		provider.ID = fmt.Sprintf("provider-%d", r.nextID)
	}
	copyProvider := *provider
	r.providers[provider.TenantID] = append(r.providers[provider.TenantID], &copyProvider)
	return nil
}

func (r *fakeWebSearchProviderRepo) GetByID(_ context.Context, tenantID uint64, id string) (*types.WebSearchProviderEntity, error) {
	for _, provider := range r.providers[tenantID] {
		if provider.ID == id {
			return provider, nil
		}
	}
	return nil, nil
}

func (r *fakeWebSearchProviderRepo) GetDefault(_ context.Context, tenantID uint64) (*types.WebSearchProviderEntity, error) {
	for _, provider := range r.providers[tenantID] {
		if provider.IsDefault {
			return provider, nil
		}
	}
	return nil, nil
}

func (r *fakeWebSearchProviderRepo) List(_ context.Context, tenantID uint64) ([]*types.WebSearchProviderEntity, error) {
	return append([]*types.WebSearchProviderEntity(nil), r.providers[tenantID]...), nil
}

func (r *fakeWebSearchProviderRepo) Update(_ context.Context, provider *types.WebSearchProviderEntity) error {
	for i, existing := range r.providers[provider.TenantID] {
		if existing.ID == provider.ID {
			copyProvider := *provider
			r.providers[provider.TenantID][i] = &copyProvider
			return nil
		}
	}
	return nil
}

func (r *fakeWebSearchProviderRepo) Delete(_ context.Context, tenantID uint64, id string) error {
	providers := r.providers[tenantID]
	for i, provider := range providers {
		if provider.ID == id {
			r.providers[tenantID] = append(providers[:i], providers[i+1:]...)
			return nil
		}
	}
	return nil
}

func (r *fakeWebSearchProviderRepo) ClearDefault(_ context.Context, tenantID uint64, excludeID string) error {
	for _, provider := range r.providers[tenantID] {
		if provider.ID != excludeID {
			provider.IsDefault = false
		}
	}
	return nil
}
