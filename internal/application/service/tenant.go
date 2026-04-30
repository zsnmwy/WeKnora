package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/config"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
)

var apiKeySecret = func() []byte {
	return []byte(os.Getenv("TENANT_AES_KEY"))
}

// ListTenantsParams defines parameters for listing tenants with filtering and pagination
type ListTenantsParams struct {
	Page     int    // Page number for pagination
	PageSize int    // Number of items per page
	Status   string // Filter by tenant status
	Name     string // Filter by tenant name
}

// tenantService implements the TenantService interface
type tenantService struct {
	repo                  interfaces.TenantRepository // Repository for tenant data operations
	modelRepo             interfaces.ModelRepository
	webSearchProviderRepo interfaces.WebSearchProviderRepository
	config                *config.Config
}

// NewTenantService creates a new tenant service instance
func NewTenantService(
	repo interfaces.TenantRepository,
	modelRepo interfaces.ModelRepository,
	webSearchProviderRepo interfaces.WebSearchProviderRepository,
	configInfo *config.Config,
) interfaces.TenantService {
	return &tenantService{
		repo:                  repo,
		modelRepo:             modelRepo,
		webSearchProviderRepo: webSearchProviderRepo,
		config:                configInfo,
	}
}

// CreateTenant creates a new tenant
func (s *tenantService) CreateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	logger.Info(ctx, "Start creating tenant")

	if tenant.Name == "" {
		logger.Error(ctx, "Tenant name cannot be empty")
		return nil, errors.New("tenant name cannot be empty")
	}

	logger.Infof(ctx, "Creating tenant, name: %s", tenant.Name)

	// Create tenant with initial values
	tenant.APIKey = s.generateApiKey(0)
	tenant.Status = "active"
	tenant.CreatedAt = time.Now()
	tenant.UpdatedAt = time.Now()

	if err := s.validateStorageBucketUniqueness(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_name": tenant.Name,
		})
		return nil, err
	}

	logger.Info(ctx, "Saving tenant information to database")
	if err := s.repo.CreateTenant(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_name": tenant.Name,
		})
		return nil, err
	}

	logger.Infof(ctx, "Tenant created successfully, ID: %d, generating official API Key", tenant.ID)
	tenant.APIKey = s.generateApiKey(tenant.ID)
	s.applyDefaultTenantSettings(ctx, tenant)

	// Manually encrypt APIKey before update, because db.Updates() does not trigger BeforeSave hook
	if key := utils.GetAESKey(); key != nil && tenant.APIKey != "" {
		if encrypted, err := utils.EncryptAESGCM(tenant.APIKey, key); err == nil {
			tenant.APIKey = encrypted
		}
	}

	if err := s.repo.UpdateTenant(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id":   tenant.ID,
			"tenant_name": tenant.Name,
		})
		return nil, err
	}

	if err := s.ensureDefaultWebSearchProviders(ctx, tenant.ID); err != nil {
		logger.Warnf(ctx, "Failed to initialize default web search providers for tenant %d: %v", tenant.ID, err)
	}

	logger.Infof(ctx, "Tenant creation and update completed, ID: %d, name: %s", tenant.ID, tenant.Name)
	return tenant, nil
}

const defaultBuiltinTenantID uint64 = 10000

var (
	defaultChatModelCandidates = []string{
		"builtin-deepseek-v4-pro",
		"builtin-deepseek-v4-flash",
	}
	defaultEmbeddingModelCandidates = []string{
		"builtin-embedding-3",
		"builtin-embedding-001",
	}
	defaultRerankModelCandidates = []string{
		"builtin-rerank",
		"builtin-rerank-001",
	}
)

func (s *tenantService) applyDefaultTenantSettings(ctx context.Context, tenant *types.Tenant) {
	if tenant == nil {
		return
	}

	defaultChatModelID := s.resolveDefaultModelID(ctx, tenant.ID, types.ModelTypeKnowledgeQA, defaultChatModelCandidates...)
	defaultEmbeddingModelID := s.resolveDefaultModelID(ctx, tenant.ID, types.ModelTypeEmbedding, defaultEmbeddingModelCandidates...)
	defaultRerankModelID := s.resolveDefaultModelID(ctx, tenant.ID, types.ModelTypeRerank, defaultRerankModelCandidates...)

	if tenant.ConversationConfig == nil {
		tenant.ConversationConfig = s.buildDefaultConversationConfig(defaultChatModelID, defaultRerankModelID)
	} else {
		s.fillConversationDefaults(tenant.ConversationConfig, defaultChatModelID, defaultRerankModelID)
	}

	if tenant.WebSearchConfig == nil {
		tenant.WebSearchConfig = buildDefaultWebSearchConfig(defaultEmbeddingModelID, defaultRerankModelID)
	} else {
		fillWebSearchDefaults(tenant.WebSearchConfig, defaultEmbeddingModelID, defaultRerankModelID)
	}

	if tenant.RetrievalConfig == nil {
		tenant.RetrievalConfig = s.buildDefaultRetrievalConfig(defaultRerankModelID)
	} else {
		s.fillRetrievalDefaults(tenant.RetrievalConfig, defaultRerankModelID)
	}
}

func (s *tenantService) buildDefaultConversationConfig(summaryModelID, rerankModelID string) *types.ConversationConfig {
	cfg := &types.ConversationConfig{
		Temperature:          0.7,
		MaxCompletionTokens:  2048,
		MaxRounds:            5,
		EmbeddingTopK:        30,
		KeywordThreshold:     0.3,
		VectorThreshold:      0.2,
		RerankTopK:           30,
		RerankThreshold:      0.3,
		EnableRewrite:        true,
		EnableQueryExpansion: true,
		FallbackStrategy:     string(types.FallbackStrategyModel),
		FallbackResponse:     "Sorry, I am unable to answer this question.",
		SummaryModelID:       summaryModelID,
		RerankModelID:        rerankModelID,
	}

	if s.config != nil && s.config.Conversation != nil {
		conv := s.config.Conversation
		cfg.MaxRounds = positiveIntOr(conv.MaxRounds, cfg.MaxRounds)
		cfg.EmbeddingTopK = positiveIntOr(conv.EmbeddingTopK, cfg.EmbeddingTopK)
		cfg.KeywordThreshold = positiveFloatOr(conv.KeywordThreshold, cfg.KeywordThreshold)
		cfg.VectorThreshold = positiveFloatOr(conv.VectorThreshold, cfg.VectorThreshold)
		cfg.RerankTopK = positiveIntOr(conv.RerankTopK, cfg.RerankTopK)
		cfg.RerankThreshold = conv.RerankThreshold
		cfg.EnableRewrite = conv.EnableRewrite
		cfg.EnableQueryExpansion = conv.EnableQueryExpansion
		if conv.FallbackStrategy != "" {
			cfg.FallbackStrategy = conv.FallbackStrategy
		}
		if conv.FallbackResponse != "" {
			cfg.FallbackResponse = conv.FallbackResponse
		}
		cfg.FallbackPrompt = conv.FallbackPrompt
		cfg.RewritePromptSystem = conv.RewritePromptSystem
		cfg.RewritePromptUser = conv.RewritePromptUser
		if conv.Summary != nil {
			cfg.Prompt = conv.Summary.Prompt
			cfg.ContextTemplate = conv.Summary.ContextTemplate
			cfg.Temperature = conv.Summary.Temperature
			cfg.MaxCompletionTokens = positiveIntOr(conv.Summary.MaxCompletionTokens, cfg.MaxCompletionTokens)
		}
	}

	return cfg
}

func (s *tenantService) fillConversationDefaults(cfg *types.ConversationConfig, summaryModelID, rerankModelID string) {
	if cfg == nil {
		return
	}
	defaults := s.buildDefaultConversationConfig(summaryModelID, rerankModelID)
	if cfg.Prompt == "" {
		cfg.Prompt = defaults.Prompt
	}
	if cfg.ContextTemplate == "" {
		cfg.ContextTemplate = defaults.ContextTemplate
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = defaults.Temperature
	}
	if cfg.MaxCompletionTokens <= 0 {
		cfg.MaxCompletionTokens = defaults.MaxCompletionTokens
	}
	if cfg.MaxRounds <= 0 {
		cfg.MaxRounds = defaults.MaxRounds
	}
	if cfg.EmbeddingTopK <= 0 {
		cfg.EmbeddingTopK = defaults.EmbeddingTopK
	}
	if cfg.KeywordThreshold <= 0 {
		cfg.KeywordThreshold = defaults.KeywordThreshold
	}
	if cfg.VectorThreshold <= 0 {
		cfg.VectorThreshold = defaults.VectorThreshold
	}
	if cfg.RerankTopK <= 0 {
		cfg.RerankTopK = defaults.RerankTopK
	}
	if cfg.RerankThreshold == 0 {
		cfg.RerankThreshold = defaults.RerankThreshold
	}
	if cfg.SummaryModelID == "" {
		cfg.SummaryModelID = defaults.SummaryModelID
	}
	if cfg.RerankModelID == "" {
		cfg.RerankModelID = defaults.RerankModelID
	}
	if cfg.FallbackStrategy == "" {
		cfg.FallbackStrategy = defaults.FallbackStrategy
	}
	if cfg.FallbackResponse == "" {
		cfg.FallbackResponse = defaults.FallbackResponse
	}
	if cfg.FallbackPrompt == "" {
		cfg.FallbackPrompt = defaults.FallbackPrompt
	}
	if cfg.RewritePromptSystem == "" {
		cfg.RewritePromptSystem = defaults.RewritePromptSystem
	}
	if cfg.RewritePromptUser == "" {
		cfg.RewritePromptUser = defaults.RewritePromptUser
	}
}

func buildDefaultWebSearchConfig(embeddingModelID, rerankModelID string) *types.WebSearchConfig {
	return &types.WebSearchConfig{
		MaxResults:        5,
		IncludeDate:       true,
		CompressionMethod: "none",
		Blacklist:         []string{},
		EmbeddingModelID:  embeddingModelID,
		RerankModelID:     rerankModelID,
	}
}

func fillWebSearchDefaults(cfg *types.WebSearchConfig, embeddingModelID, rerankModelID string) {
	if cfg == nil {
		return
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	if cfg.CompressionMethod == "" {
		cfg.CompressionMethod = "none"
	}
	if cfg.Blacklist == nil {
		cfg.Blacklist = []string{}
	}
	if cfg.EmbeddingModelID == "" {
		cfg.EmbeddingModelID = embeddingModelID
	}
	if cfg.RerankModelID == "" {
		cfg.RerankModelID = rerankModelID
	}
}

func (s *tenantService) buildDefaultRetrievalConfig(rerankModelID string) *types.RetrievalConfig {
	cfg := &types.RetrievalConfig{
		EmbeddingTopK:    50,
		VectorThreshold:  0.15,
		KeywordThreshold: 0.3,
		RerankTopK:       10,
		RerankThreshold:  0.2,
		RerankModelID:    rerankModelID,
	}
	if s.config != nil && s.config.Conversation != nil {
		conv := s.config.Conversation
		cfg.EmbeddingTopK = positiveIntOr(conv.EmbeddingTopK, cfg.EmbeddingTopK)
		cfg.VectorThreshold = positiveFloatOr(conv.VectorThreshold, cfg.VectorThreshold)
		cfg.KeywordThreshold = positiveFloatOr(conv.KeywordThreshold, cfg.KeywordThreshold)
		cfg.RerankTopK = positiveIntOr(conv.RerankTopK, cfg.RerankTopK)
		cfg.RerankThreshold = conv.RerankThreshold
	}
	return cfg
}

func (s *tenantService) fillRetrievalDefaults(cfg *types.RetrievalConfig, rerankModelID string) {
	if cfg == nil {
		return
	}
	defaults := s.buildDefaultRetrievalConfig(rerankModelID)
	if cfg.EmbeddingTopK <= 0 {
		cfg.EmbeddingTopK = defaults.EmbeddingTopK
	}
	if cfg.VectorThreshold <= 0 {
		cfg.VectorThreshold = defaults.VectorThreshold
	}
	if cfg.KeywordThreshold <= 0 {
		cfg.KeywordThreshold = defaults.KeywordThreshold
	}
	if cfg.RerankTopK <= 0 {
		cfg.RerankTopK = defaults.RerankTopK
	}
	if cfg.RerankThreshold == 0 {
		cfg.RerankThreshold = defaults.RerankThreshold
	}
	if cfg.RerankModelID == "" {
		cfg.RerankModelID = defaults.RerankModelID
	}
}

func (s *tenantService) resolveDefaultModelID(
	ctx context.Context,
	tenantID uint64,
	modelType types.ModelType,
	preferredIDs ...string,
) string {
	if s.modelRepo == nil {
		if len(preferredIDs) > 0 {
			return preferredIDs[0]
		}
		return ""
	}

	for _, id := range preferredIDs {
		if id == "" {
			continue
		}
		model, err := s.modelRepo.GetByID(ctx, tenantID, id)
		if err == nil && model != nil && model.Type == modelType && model.Status == types.ModelStatusActive {
			return model.ID
		}
	}

	models, err := s.modelRepo.List(ctx, tenantID, modelType, "")
	if err != nil {
		logger.Warnf(ctx, "Failed to list default %s models for tenant %d: %v", modelType, tenantID, err)
		return ""
	}
	for _, model := range models {
		if model != nil && model.Type == modelType && model.Status == types.ModelStatusActive && model.IsDefault {
			return model.ID
		}
	}
	for _, model := range models {
		if model != nil && model.Type == modelType && model.Status == types.ModelStatusActive {
			return model.ID
		}
	}
	return ""
}

func (s *tenantService) ensureDefaultWebSearchProviders(ctx context.Context, tenantID uint64) error {
	if s.webSearchProviderRepo == nil || tenantID == 0 {
		return nil
	}

	existing, err := s.webSearchProviderRepo.List(ctx, tenantID)
	if err != nil {
		return err
	}

	existingTypes := make(map[types.WebSearchProviderType]bool, len(existing))
	hasDefault := false
	for _, provider := range existing {
		if provider == nil {
			continue
		}
		existingTypes[provider.Provider] = true
		if provider.IsDefault {
			hasDefault = true
		}
	}

	sourceProviders := []*types.WebSearchProviderEntity(nil)
	if tenantID != defaultBuiltinTenantID {
		if providers, err := s.webSearchProviderRepo.List(ctx, defaultBuiltinTenantID); err == nil {
			sourceProviders = providers
		} else {
			logger.Warnf(ctx, "Failed to list template web search providers from tenant %d: %v", defaultBuiltinTenantID, err)
		}
	}

	now := time.Now()
	createdAny := false
	for _, provider := range sourceProviders {
		if provider == nil || existingTypes[provider.Provider] {
			continue
		}
		copyProvider := *provider
		copyProvider.ID = ""
		copyProvider.TenantID = tenantID
		copyProvider.IsDefault = provider.IsDefault && !hasDefault
		copyProvider.CreatedAt = now
		copyProvider.UpdatedAt = now
		copyProvider.DeletedAt.Valid = false
		if err := s.webSearchProviderRepo.Create(ctx, &copyProvider); err != nil {
			return err
		}
		existingTypes[copyProvider.Provider] = true
		if copyProvider.IsDefault {
			hasDefault = true
		}
		createdAny = true
	}

	if len(existing) == 0 && !createdAny {
		provider := &types.WebSearchProviderEntity{
			TenantID:    tenantID,
			Name:        "DuckDuckGo",
			Provider:    types.WebSearchProviderTypeDuckDuckGo,
			Description: "Built-in DuckDuckGo web search",
			Parameters:  types.WebSearchProviderParameters{},
			IsDefault:   true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		return s.webSearchProviderRepo.Create(ctx, provider)
	}

	return nil
}

func positiveIntOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func positiveFloatOr(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

// GetTenantByID retrieves a tenant by their ID
func (s *tenantService) GetTenantByID(ctx context.Context, id uint64) (*types.Tenant, error) {
	if id == 0 {
		logger.Error(ctx, "Tenant ID cannot be 0")
		return nil, errors.New("tenant ID cannot be 0")
	}

	tenant, err := s.repo.GetTenantByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": id,
		})
		return nil, err
	}

	return tenant, nil
}

// ListTenants retrieves a list of all tenants
func (s *tenantService) ListTenants(ctx context.Context) ([]*types.Tenant, error) {
	tenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	logger.Infof(ctx, "Tenant list retrieved successfully, total: %d", len(tenants))
	return tenants, nil
}

// UpdateTenant updates an existing tenant's information
func (s *tenantService) UpdateTenant(ctx context.Context, tenant *types.Tenant) (*types.Tenant, error) {
	if tenant.ID == 0 {
		logger.Error(ctx, "Tenant ID cannot be 0")
		return nil, errors.New("tenant ID cannot be 0")
	}

	logger.Infof(ctx, "Updating tenant, ID: %d, name: %s", tenant.ID, tenant.Name)

	if err := s.validateStorageBucketUniqueness(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenant.ID,
		})
		return nil, err
	}

	// Generate new API key if empty
	if tenant.APIKey == "" {
		logger.Info(ctx, "API Key is empty, generating new API Key")
		tenant.APIKey = s.generateApiKey(tenant.ID)
	}

	tenant.UpdatedAt = time.Now()
	logger.Info(ctx, "Saving tenant information to database")

	if err := s.repo.UpdateTenant(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenant.ID,
		})
		return nil, err
	}

	logger.Infof(ctx, "Tenant updated successfully, ID: %d", tenant.ID)
	return tenant, nil
}

// DeleteTenant removes a tenant by their ID
func (s *tenantService) DeleteTenant(ctx context.Context, id uint64) error {
	logger.Info(ctx, "Start deleting tenant")

	if id == 0 {
		logger.Error(ctx, "Tenant ID cannot be 0")
		return errors.New("tenant ID cannot be 0")
	}

	logger.Infof(ctx, "Deleting tenant, ID: %d", id)

	// Get tenant information for logging
	tenant, err := s.repo.GetTenantByID(ctx, id)
	if err != nil {
		if err.Error() == "record not found" {
			logger.Warnf(ctx, "Tenant to be deleted does not exist, ID: %d", id)
		} else {
			logger.ErrorWithFields(ctx, err, map[string]interface{}{
				"tenant_id": id,
			})
			return err
		}
	} else {
		logger.Infof(ctx, "Deleting tenant, ID: %d, name: %s", id, tenant.Name)
	}

	err = s.repo.DeleteTenant(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": id,
		})
		return err
	}

	logger.Infof(ctx, "Tenant deleted successfully, ID: %d", id)
	return nil
}

// UpdateAPIKey updates the API key for a specific tenant
func (s *tenantService) UpdateAPIKey(ctx context.Context, id uint64) (string, error) {
	logger.Info(ctx, "Start updating tenant API Key")

	if id == 0 {
		logger.Error(ctx, "Tenant ID cannot be 0")
		return "", errors.New("tenant ID cannot be 0")
	}

	tenant, err := s.repo.GetTenantByID(ctx, id)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": id,
		})
		return "", err
	}

	logger.Infof(ctx, "Generating new API Key for tenant, ID: %d", id)
	tenant.APIKey = s.generateApiKey(tenant.ID)

	// Manually encrypt APIKey before update, because db.Updates() does not trigger BeforeSave hook
	if key := utils.GetAESKey(); key != nil && tenant.APIKey != "" {
		if encrypted, err := utils.EncryptAESGCM(tenant.APIKey, key); err == nil {
			tenant.APIKey = encrypted
		}
	}

	if err := s.repo.UpdateTenant(ctx, tenant); err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": id,
		})
		return "", err
	}

	logger.Infof(ctx, "Tenant API Key updated successfully, ID: %d", id)
	return tenant.APIKey, nil
}

// generateApiKey generates a secure API key for tenant authentication
func (r *tenantService) generateApiKey(tenantID uint64) string {
	// 1. Convert tenant_id to bytes
	idBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(idBytes, uint64(tenantID))

	// 2. Encrypt tenant_id using AES-GCM
	block, err := aes.NewCipher(apiKeySecret())
	if err != nil {
		panic("Failed to create AES cipher: " + err.Error())
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic("Failed to create GCM cipher: " + err.Error())
	}

	ciphertext := aesgcm.Seal(nil, nonce, idBytes, nil)

	// 3. Combine nonce and ciphertext, then encode with base64
	combined := append(nonce, ciphertext...)
	encoded := base64.RawURLEncoding.EncodeToString(combined)

	// Create final API Key in format: sk-{encrypted_part}
	return "sk-" + encoded
}

// ExtractTenantIDFromAPIKey extracts the tenant ID from an API key
func (r *tenantService) ExtractTenantIDFromAPIKey(apiKey string) (uint64, error) {
	// 1. Validate format and extract encrypted part
	parts := strings.SplitN(apiKey, "-", 2)
	if len(parts) != 2 || parts[0] != "sk" {
		return 0, errors.New("invalid API key format")
	}

	// 2. Decode the base64 part
	encryptedData, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return 0, errors.New("invalid API key encoding")
	}

	// 3. Separate nonce and ciphertext
	if len(encryptedData) < 12 {
		return 0, errors.New("invalid API key length")
	}
	nonce, ciphertext := encryptedData[:12], encryptedData[12:]

	// 4. Decrypt
	block, err := aes.NewCipher(apiKeySecret())
	if err != nil {
		return 0, errors.New("decryption error")
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, errors.New("decryption error")
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return 0, errors.New("API key is invalid or has been tampered with")
	}

	// 5. Convert back to tenant_id
	tenantID := binary.LittleEndian.Uint64(plaintext)

	return tenantID, nil
}

// ListAllTenants lists all tenants (for users with cross-tenant access permission)
// This method returns all tenants without filtering, intended for admin users
func (s *tenantService) ListAllTenants(ctx context.Context) ([]*types.Tenant, error) {
	tenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		logger.ErrorWithFields(ctx, err, nil)
		return nil, err
	}

	logger.Infof(ctx, "All tenants list retrieved successfully, total: %d", len(tenants))
	return tenants, nil
}

// SearchTenants searches tenants with pagination and filters
func (s *tenantService) SearchTenants(ctx context.Context, keyword string, tenantID uint64, page, pageSize int) ([]*types.Tenant, int64, error) {
	tenants, total, err := s.repo.SearchTenants(ctx, keyword, tenantID, page, pageSize)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"keyword":  keyword,
			"tenantID": tenantID,
			"page":     page,
			"pageSize": pageSize,
		})
		return nil, 0, err
	}

	logger.Infof(ctx, "Tenants search completed, keyword: %s, tenantID: %d, page: %d, pageSize: %d, total: %d, found: %d",
		keyword, tenantID, page, pageSize, total, len(tenants))
	return tenants, total, nil
}

// GetTenantByIDForUser gets a tenant by ID with permission check
// This method verifies that the user has permission to access the tenant
func (s *tenantService) GetTenantByIDForUser(ctx context.Context, tenantID uint64, userID string) (*types.Tenant, error) {
	tenant, err := s.repo.GetTenantByID(ctx, tenantID)
	if err != nil {
		logger.ErrorWithFields(ctx, err, map[string]interface{}{
			"tenant_id": tenantID,
			"user_id":   userID,
		})
		return nil, err
	}

	return tenant, nil
}

func (s *tenantService) GetWeKnoraCloudCredentials(ctx context.Context) *types.WeKnoraCloudCredentials {
	// Try to get tenant info from context first (already loaded by middleware).
	// CredentialsConfig.Scan handles decryption, so credentials are ready to use.
	if tenant, ok := types.TenantInfoFromContext(ctx); ok {
		if creds := tenant.Credentials.GetWeKnoraCloud(); creds != nil {
			return creds
		}
	}

	// Fallback: load tenant from repo by tenantID
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok {
		return nil
	}

	tenant, err := s.repo.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return nil
	}
	return tenant.Credentials.GetWeKnoraCloud()
}

func (s *tenantService) validateStorageBucketUniqueness(ctx context.Context, tenant *types.Tenant) error {
	if tenant.StorageEngineConfig == nil {
		return nil
	}

	// Fetch existing tenant from DB to compare
	var oldTenant *types.Tenant
	if tenant.ID != 0 {
		var err error
		oldTenant, err = s.repo.GetTenantByID(ctx, tenant.ID)
		if err != nil && err.Error() != "tenant not found" && err.Error() != "record not found" {
			return err
		}
	}

	// Fetch ALL tenants to check for collision.
	allTenants, err := s.repo.ListTenants(ctx)
	if err != nil {
		return err
	}

	// Helper to get bucket names from a StorageEngineConfig
	getBuckets := func(cfg *types.StorageEngineConfig) map[string]string {
		if cfg == nil {
			return nil
		}
		res := make(map[string]string)
		if cfg.MinIO != nil && cfg.MinIO.BucketName != "" {
			res["minio"] = cfg.MinIO.BucketName
		}
		if cfg.COS != nil && cfg.COS.BucketName != "" {
			res["cos"] = cfg.COS.BucketName
		}
		if cfg.TOS != nil && cfg.TOS.BucketName != "" {
			res["tos"] = cfg.TOS.BucketName
		}
		if cfg.S3 != nil && cfg.S3.BucketName != "" {
			res["s3"] = cfg.S3.BucketName
		}
		if cfg.OSS != nil && cfg.OSS.BucketName != "" {
			res["oss"] = cfg.OSS.BucketName
		}
		return res
	}

	var oldBuckets map[string]string
	if oldTenant != nil {
		oldBuckets = getBuckets(oldTenant.StorageEngineConfig)
	}
	newBuckets := getBuckets(tenant.StorageEngineConfig)

	// Collect buckets used by other tenants
	usedByOthers := make(map[string]map[string]bool) // provider -> set of bucket names
	for _, t := range allTenants {
		if t.ID == tenant.ID {
			continue
		}
		tb := getBuckets(t.StorageEngineConfig)
		for p, b := range tb {
			if usedByOthers[p] == nil {
				usedByOthers[p] = make(map[string]bool)
			}
			usedByOthers[p][b] = true
		}
	}

	// Check if any NEW bucket is already used by someone else, AND it's different from the OLD bucket
	for p, b := range newBuckets {
		oldB := oldBuckets[p]
		if b != oldB { // User is trying to change their bucket name or set a new one
			if usedByOthers[p] != nil && usedByOthers[p][b] {
				return werrors.NewBadRequestError("存储桶名称「" + b + "」已被其他租户使用，为保证数据隔离，请使用其他名称")
			}
		}
	}

	return nil
}
