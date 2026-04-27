package chat

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRemoteChat(t *testing.T) *RemoteAPIChat {
	t.Helper()

	chat, err := NewRemoteAPIChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "",
		ModelName: "test-model",
		APIKey:    "test-key",
		ModelID:   "test-model",
	})
	require.NoError(t, err)
	return chat
}

func TestBuildChatCompletionRequest_ParallelToolCalls(t *testing.T) {
	chat := newTestRemoteChat(t)
	messages := []Message{{Role: "user", Content: "hello"}}

	t.Run("nil ParallelToolCalls leaves default", func(t *testing.T) {
		opts := &ChatOptions{Temperature: 0.7}
		req := chat.BuildChatCompletionRequest(messages, opts, false)
		assert.Nil(t, req.ParallelToolCalls, "should be nil when not set")
	})

	t.Run("ParallelToolCalls true is propagated", func(t *testing.T) {
		ptc := true
		opts := &ChatOptions{
			Temperature:       0.7,
			ParallelToolCalls: &ptc,
			Tools: []Tool{{
				Type: "function",
				Function: FunctionDef{
					Name:        "mcp_weather_getforecast",
					Description: "Get weather",
					Parameters:  json.RawMessage(`{"type":"object"}`),
				},
			}},
		}
		req := chat.BuildChatCompletionRequest(messages, opts, true)
		assert.NotNil(t, req.ParallelToolCalls)

		val, ok := req.ParallelToolCalls.(bool)
		if ok {
			assert.Equal(t, true, val)
		} else {
			assert.Equal(t, true, req.ParallelToolCalls)
		}

		assert.Len(t, req.Tools, 1)
		assert.Equal(t, "mcp_weather_getforecast", req.Tools[0].Function.Name)
	})

	t.Run("ParallelToolCalls false is propagated", func(t *testing.T) {
		ptc := false
		opts := &ChatOptions{
			Temperature:       0.7,
			ParallelToolCalls: &ptc,
		}
		req := chat.BuildChatCompletionRequest(messages, opts, false)
		assert.NotNil(t, req.ParallelToolCalls)

		val, ok := req.ParallelToolCalls.(bool)
		if ok {
			assert.Equal(t, false, val)
		} else {
			assert.Equal(t, false, req.ParallelToolCalls)
		}
	})
}

func TestBuildChatCompletionRequest_MCPToolsFormat(t *testing.T) {
	chat := newTestRemoteChat(t)
	messages := []Message{{Role: "user", Content: "查询乙醇的理化性质"}}

	mcpTools := []Tool{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "mcp_hazardous_chemicals_gethazardouschemicals",
				Description: "[MCP Service: hazardous_chemicals (external)] Get hazardous chemicals list",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "mcp_hazardous_chemicals_gethazardouschemicalbybizid",
				Description: "[MCP Service: hazardous_chemicals (external)] Get hazardous chemical by biz ID",
				Parameters:  json.RawMessage(`{"type":"object","properties":{"bizId":{"type":"string"}},"required":["bizId"]}`),
			},
		},
	}

	ptc := true
	opts := &ChatOptions{
		Temperature:       0.7,
		Tools:             mcpTools,
		ParallelToolCalls: &ptc,
	}

	req := chat.BuildChatCompletionRequest(messages, opts, true)

	assert.Len(t, req.Tools, 2)
	assert.Equal(t, "mcp_hazardous_chemicals_gethazardouschemicals", req.Tools[0].Function.Name)
	assert.Equal(t, "mcp_hazardous_chemicals_gethazardouschemicalbybizid", req.Tools[1].Function.Name)
	assert.Equal(t, true, req.ParallelToolCalls)
	assert.True(t, req.Stream)

	for _, tool := range req.Tools {
		name := tool.Function.Name
		assert.NotContains(t, name, "ed606721", "tool name must use service name, not UUID")
		assert.Regexp(t, `^[a-zA-Z0-9_-]+$`, name, "tool name must match OpenAI pattern")
		assert.LessOrEqual(t, len(name), 64, "tool name must be <= 64 chars")
	}
}

func TestBuildChatCompletionRequest_ToolChoice(t *testing.T) {
	chat := newTestRemoteChat(t)
	messages := []Message{{Role: "user", Content: "test"}}

	t.Run("auto tool choice", func(t *testing.T) {
		opts := &ChatOptions{ToolChoice: "auto"}
		req := chat.BuildChatCompletionRequest(messages, opts, false)
		assert.Equal(t, "auto", req.ToolChoice)
	})

	t.Run("specific tool choice", func(t *testing.T) {
		opts := &ChatOptions{ToolChoice: "mcp_svc_tool"}
		req := chat.BuildChatCompletionRequest(messages, opts, false)
		assert.NotNil(t, req.ToolChoice)
	})
}

func TestDeepSeekThinkingRequestCustomizer(t *testing.T) {
	chat, err := NewRemoteAPIChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "",
		ModelName: "deepseek-v4-pro",
		APIKey:    "test-key",
		ModelID:   "deepseek-v4-pro",
		Provider:  "deepseek",
		ExtraConfig: map[string]string{
			"thinking":              "enabled",
			"reasoning_effort":      "max",
			"max_completion_tokens": "393216",
		},
	})
	require.NoError(t, err)

	opts := chat.effectiveChatOptions(&ChatOptions{ToolChoice: "auto"})
	require.NotNil(t, opts.Thinking)
	assert.True(t, *opts.Thinking)

	req := chat.BuildChatCompletionRequest([]Message{{Role: "user", Content: "test"}}, opts, true)
	assert.Equal(t, "max", req.ReasoningEffort)
	assert.Equal(t, types.MaxConversationCompletionTokens, req.MaxCompletionTokens)
	assert.Equal(t, "auto", req.ToolChoice)

	customReq, useRawHTTP := deepseekRequestCustomizer(&req, opts, true)
	require.True(t, useRawHTTP)
	require.NotNil(t, customReq)

	deepseekReq, ok := customReq.(ThinkingChatCompletionRequest)
	require.True(t, ok)
	require.NotNil(t, deepseekReq.Thinking)
	assert.Equal(t, "enabled", deepseekReq.Thinking.Type)
	assert.Nil(t, deepseekReq.ToolChoice, "DeepSeek provider should strip tool_choice")
}

func TestDeepSeekThinkingRequestCustomizer_DisabledFromOptions(t *testing.T) {
	thinking := false
	opts := &ChatOptions{Thinking: &thinking}
	req := openAICompatibleTestRequest("deepseek-v4-pro")

	customReq, useRawHTTP := deepseekRequestCustomizer(&req, opts, true)
	require.True(t, useRawHTTP)

	deepseekReq, ok := customReq.(ThinkingChatCompletionRequest)
	require.True(t, ok)
	require.NotNil(t, deepseekReq.Thinking)
	assert.Equal(t, "disabled", deepseekReq.Thinking.Type)
}

func TestConvertMessages_DeepSeekReasoningContentOnlyForDeepSeek(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "weather?"},
		{
			Role:             "assistant",
			Content:          "I will check.",
			ReasoningContent: "Need date first.",
			ToolCalls: []ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: FunctionCall{
					Name:      "get_date",
					Arguments: "{}",
				},
			}},
		},
	}

	deepseekChat, err := NewRemoteAPIChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "",
		ModelName: "deepseek-v4-pro",
		APIKey:    "test-key",
		ModelID:   "deepseek-v4-pro",
		Provider:  "deepseek",
	})
	require.NoError(t, err)
	deepseekMessages := deepseekChat.ConvertMessages(messages)
	require.Len(t, deepseekMessages, 2)
	assert.Equal(t, "Need date first.", deepseekMessages[1].ReasoningContent)
	assert.Equal(t, "I will check.", deepseekMessages[1].Content)
	require.Len(t, deepseekMessages[1].ToolCalls, 1)

	openAIChat, err := NewRemoteAPIChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "",
		ModelName: "gpt-test",
		APIKey:    "test-key",
		ModelID:   "gpt-test",
		Provider:  "openai",
	})
	require.NoError(t, err)
	openAIMessages := openAIChat.ConvertMessages(messages)
	require.Len(t, openAIMessages, 2)
	assert.Empty(t, openAIMessages[1].ReasoningContent)
	assert.Equal(t, "I will check.", openAIMessages[1].Content)
}

func TestParseCompletionResponse_PreservesReasoningContent(t *testing.T) {
	chat := newTestRemoteChat(t)
	resp := &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Role:             "assistant",
				Content:          "final answer",
				ReasoningContent: "private reasoning",
			},
			FinishReason: openai.FinishReasonStop,
		}},
		Usage: openai.Usage{
			PromptTokens:     2,
			CompletionTokens: 3,
			TotalTokens:      5,
		},
	}

	parsed, err := chat.parseCompletionResponse(resp)
	require.NoError(t, err)
	assert.Equal(t, "final answer", parsed.Content)
	assert.Equal(t, "private reasoning", parsed.ReasoningContent)
	assert.Equal(t, "stop", parsed.FinishReason)
}

func TestDeepSeekThinkingToolRoundTrip_Live(t *testing.T) {
	if os.Getenv("DEEPSEEK_LIVE_TESTS") != "1" {
		t.Skip("set DEEPSEEK_LIVE_TESTS=1 to run live DeepSeek API test")
	}
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		t.Skip("DEEPSEEK_API_KEY environment variable not set")
	}
	t.Setenv("SSRF_WHITELIST", "api.deepseek.com")

	chatModel, err := NewRemoteChat(&ChatConfig{
		Source:    types.ModelSourceRemote,
		BaseURL:   "https://api.deepseek.com",
		ModelName: "deepseek-v4-flash",
		APIKey:    apiKey,
		ModelID:   "deepseek-v4-flash",
		Provider:  "deepseek",
		ExtraConfig: map[string]string{
			"thinking":         "enabled",
			"reasoning_effort": "max",
		},
	})
	require.NoError(t, err)

	tools := []Tool{{
		Type: "function",
		Function: FunctionDef{
			Name:        "get_date",
			Description: "Get the current date. The assistant must call this before answering.",
			Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
		},
	}}
	messages := []Message{{
		Role:    "user",
		Content: "You must call get_date first. Then answer with the exact date returned by the tool.",
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	first, err := chatModel.Chat(ctx, messages, &ChatOptions{
		Tools:               tools,
		MaxCompletionTokens: 256,
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.ReasoningContent)
	require.NotEmpty(t, first.ToolCalls)

	assistantToolCalls := make([]ToolCall, 0, len(first.ToolCalls))
	for _, tc := range first.ToolCalls {
		assistantToolCalls = append(assistantToolCalls, ToolCall{
			ID:   tc.ID,
			Type: tc.Type,
			Function: FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	messages = append(messages, Message{
		Role:             "assistant",
		Content:          first.Content,
		ReasoningContent: first.ReasoningContent,
		ToolCalls:        assistantToolCalls,
	})
	messages = append(messages, Message{
		Role:       "tool",
		ToolCallID: first.ToolCalls[0].ID,
		Content:    "2026-04-27",
	})

	second, err := chatModel.Chat(ctx, messages, &ChatOptions{
		Tools:               tools,
		MaxCompletionTokens: 256,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, second.ReasoningContent)
	assert.NotEmpty(t, second.Content)
}

func openAICompatibleTestRequest(modelName string) openai.ChatCompletionRequest {
	return openai.ChatCompletionRequest{
		Model:  modelName,
		Stream: true,
		Messages: []openai.ChatCompletionMessage{
			{Role: "user", Content: "test"},
		},
	}
}

// TestRemoteAPIChat 综合测试 Remote API Chat 的所有功能
func TestRemoteAPIChat(t *testing.T) {
	// 获取环境变量
	deepseekAPIKey := os.Getenv("DEEPSEEK_API_KEY")
	aliyunAPIKey := os.Getenv("ALIYUN_API_KEY")

	// 定义测试配置
	testConfigs := []struct {
		name    string
		apiKey  string
		config  *ChatConfig
		skipMsg string
	}{
		{
			name:   "DeepSeek API",
			apiKey: deepseekAPIKey,
			config: &ChatConfig{
				Source:    types.ModelSourceRemote,
				BaseURL:   "https://api.deepseek.com/v1",
				ModelName: "deepseek-chat",
				APIKey:    deepseekAPIKey,
				ModelID:   "deepseek-chat",
			},
			skipMsg: "DEEPSEEK_API_KEY environment variable not set",
		},
		{
			name:   "Aliyun DeepSeek",
			apiKey: aliyunAPIKey,
			config: &ChatConfig{
				Source:    types.ModelSourceRemote,
				BaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
				ModelName: "deepseek-v3.1",
				APIKey:    aliyunAPIKey,
				ModelID:   "deepseek-v3.1",
			},
			skipMsg: "ALIYUN_API_KEY environment variable not set",
		},
		{
			name:   "Aliyun Qwen3-32b",
			apiKey: aliyunAPIKey,
			config: &ChatConfig{
				Source:    types.ModelSourceRemote,
				BaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
				ModelName: "qwen3-32b",
				APIKey:    aliyunAPIKey,
				ModelID:   "qwen3-32b",
			},
			skipMsg: "ALIYUN_API_KEY environment variable not set",
		},
		{
			name:   "Aliyun Qwen-max",
			apiKey: aliyunAPIKey,
			config: &ChatConfig{
				Source:    types.ModelSourceRemote,
				BaseURL:   "https://dashscope.aliyuncs.com/compatible-mode/v1",
				ModelName: "qwen-max",
				APIKey:    aliyunAPIKey,
				ModelID:   "qwen-max",
			},
			skipMsg: "ALIYUN_API_KEY environment variable not set",
		},
	}

	// 测试消息
	testMessages := []Message{
		{
			Role:    "user",
			Content: "test",
		},
	}

	// 测试选项
	testOptions := &ChatOptions{
		Temperature: 0.7,
		MaxTokens:   100,
	}

	// 创建上下文
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 遍历所有配置进行测试
	for _, tc := range testConfigs {
		t.Run(tc.name, func(t *testing.T) {
			// 检查 API Key
			if tc.apiKey == "" {
				t.Skip(tc.skipMsg)
			}

			// 创建聊天实例
			chat, err := NewRemoteAPIChat(tc.config)
			require.NoError(t, err)
			assert.Equal(t, tc.config.ModelName, chat.GetModelName())
			assert.Equal(t, tc.config.ModelID, chat.GetModelID())

			// 测试基本聊天功能
			t.Run("Basic Chat", func(t *testing.T) {
				response, err := chat.Chat(ctx, testMessages, testOptions)
				require.NoError(t, err)
				require.NotNil(t, response, "response should not be nil")
				assert.NotEmpty(t, response.Content)
				assert.Greater(t, response.Usage.TotalTokens, 0)
				assert.Greater(t, response.Usage.PromptTokens, 0)
				assert.Greater(t, response.Usage.CompletionTokens, 0)

				t.Logf("%s Response: %s", tc.name, response.Content)
				t.Logf("Usage: Prompt=%d, Completion=%d, Total=%d",
					response.Usage.PromptTokens,
					response.Usage.CompletionTokens,
					response.Usage.TotalTokens)
			})
		})
	}
}
