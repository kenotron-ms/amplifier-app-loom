package nl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ms/agent-daemon/internal/store"
	"github.com/ms/agent-daemon/internal/types"
)

const openAIEndpoint = "https://api.openai.com/v1/chat/completions"

// OpenAIClient implements NLClient using the OpenAI Chat Completions API.
type OpenAIClient struct {
	apiKey string
	model  string
	store  store.Store
	http   *http.Client
}

func NewOpenAIClient(apiKey, model string, s store.Store) *OpenAIClient {
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		store:  s,
		http:   &http.Client{},
	}
}

// ── OpenAI API types ──────────────────────────────────────────────────────────

type oaiMessage struct {
	Role       string        `json:"role"`
	Content    interface{}   `json:"content,omitempty"` // string or null
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type oaiToolCall struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"` // "function"
	Function oaiToolFunction `json:"function"`
}

type oaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string
}

type oaiTool struct {
	Type     string     `json:"type"` // "function"
	Function oaiFuncDef `json:"function"`
}

type oaiFuncDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Parameters  interface{} `json:"parameters"`
}

type oaiRequest struct {
	Model     string       `json:"model"`
	Messages  []oaiMessage `json:"messages"`
	Tools     []oaiTool    `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens,omitempty"`
}

type oaiResponse struct {
	Choices []oaiChoice `json:"choices"`
	Error   *oaiError   `json:"error,omitempty"`
}

type oaiChoice struct {
	Message      oaiMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type oaiError struct {
	Message string `json:"message"`
}

// ── Chat ─────────────────────────────────────────────────────────────────────

func (c *OpenAIClient) Chat(ctx context.Context, userMessage string, history []types.ChatMessage) (string, []string, error) {
	jobs, _ := c.store.ListJobs(ctx)
	jobsJSON, _ := json.MarshalIndent(jobs, "", "  ")

	sysContent := buildSystemPrompt() + "\n\nCurrent jobs:\n```json\n" + string(jobsJSON) + "\n```"

	messages := []oaiMessage{{Role: "system", Content: sysContent}}
	for _, m := range history {
		messages = append(messages, oaiMessage{Role: m.Role, Content: m.Content})
	}
	messages = append(messages, oaiMessage{Role: "user", Content: userMessage})

	tools := buildOpenAITools()

	var actions []string
	var finalText string

	for {
		resp, err := c.callAPI(ctx, messages, tools)
		if err != nil {
			return "", nil, err
		}
		if len(resp.Choices) == 0 {
			break
		}

		choice := resp.Choices[0]
		msg := choice.Message

		// Collect text
		if s, ok := msg.Content.(string); ok && s != "" {
			finalText += s
		}

		if choice.FinishReason != "tool_calls" || len(msg.ToolCalls) == 0 {
			break
		}

		// Append assistant message with tool calls
		messages = append(messages, msg)

		// Execute each tool call
		for _, tc := range msg.ToolCalls {
			result, action, execErr := executeTool(ctx, c.store, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
			var content string
			if execErr != nil {
				content = fmt.Sprintf("Error: %v", execErr)
			} else {
				content = result
				actions = append(actions, action)
			}
			messages = append(messages, oaiMessage{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    content,
			})
		}
	}

	return finalText, actions, nil
}

func (c *OpenAIClient) callAPI(ctx context.Context, messages []oaiMessage, tools []oaiTool) (*oaiResponse, error) {
	body, err := json.Marshal(oaiRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai HTTP: %w", err)
	}
	defer res.Body.Close()

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var resp oaiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("openai parse: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("openai API: %s", resp.Error.Message)
	}
	return &resp, nil
}

// Ping verifies the API key by sending a minimal request.
func (c *OpenAIClient) Ping(ctx context.Context) error {
	_, err := c.callAPI(ctx, []oaiMessage{{Role: "user", Content: "hi"}}, nil)
	return err
}

// ── Tool definitions for OpenAI ───────────────────────────────────────────────

func buildOpenAITools() []oaiTool {
	anthropicTools := buildTools() // reuse schema from tools.go
	result := make([]oaiTool, 0, len(anthropicTools))
	for _, t := range anthropicTools {
		if t.OfTool == nil {
			continue
		}
		desc := ""
		if t.OfTool.Description.Valid() {
			desc = t.OfTool.Description.Value
		}
		result = append(result, oaiTool{
			Type: "function",
			Function: oaiFuncDef{
				Name:        t.OfTool.Name,
				Description: desc,
				Parameters:  t.OfTool.InputSchema,
			},
		})
	}
	return result
}
