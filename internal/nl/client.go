package nl

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/ms/agent-daemon/internal/store"
)

// NLClient is the interface for natural language chat clients.
type NLClient interface {
	Chat(ctx context.Context, message string) (string, []string, error)
}

const systemPrompt = `You are the agent-daemon assistant. You help users manage their scheduled jobs.

Current jobs will be provided in each message. You can:
- Create new jobs (create_job)
- Update existing jobs (update_job)
- Delete jobs (delete_job)

## Trigger types
- "cron": repeating schedule via cron expression with seconds field (e.g. "0 */5 * * * *" = every 5 min, "0 30 9 * * 1-5" = 9:30am weekdays)
- "loop": repeating interval as a Go duration (e.g. "30s", "5m", "1h")
- "once": runs exactly once then auto-disables. trigger_schedule is an optional delay (e.g. "10m", "2h"). Leave empty to run right now.
- "watch": fires when a file or directory changes. Requires watch_path. Optional: watch_recursive (bool), watch_events (array: "create","write","remove","rename","chmod"), watch_mode ("notify" for OS-level, "poll" for polling), watch_poll_interval (e.g. "2s"), watch_debounce (quiet window, e.g. "500ms").

## Executor types
Every job has an executor that controls how it runs:

- "shell": runs a shell command. Requires shell_command.
- "claude-code": runs the Claude Code CLI (` + "`" + `claude -p` + "`" + `) in a working directory. Requires claude_prompt. Optionally: claude_steps (array of follow-up prompts for multi-turn), claude_model (e.g. "sonnet", "opus", "claude-sonnet-4-6"), claude_max_turns, claude_allowed_tools (array of tool names).
- "amplifier": runs the Amplifier CLI. Use amplifier_prompt for free-form prompts, or amplifier_recipe_path for a YAML recipe file. Optionally: amplifier_steps (multi-turn follow-ups), amplifier_bundle (e.g. "foundation", "recipes"), amplifier_model, amplifier_context (key-value map for recipe variables).

## Routing guidance
"run once" / "immediately" / "one time" → trigger "once", no schedule.
"in 10 minutes" / "after 2h" → trigger "once", schedule "10m" / "2h".
"every X" → "loop" or "cron".
"at 3pm" / specific time → "cron".
"watch", "monitor", "when file changes", "when folder changes" → trigger "watch", set watch_path.
"run claude" / "ask claude code to" / "have claude code..." → executor "claude-code".
"run amplifier" / "use amplifier" / "run recipe" → executor "amplifier".
No executor mentioned → default "shell".

Be concise. Confirm what actions you took.`

// AnthropicClient wraps the Anthropic SDK for job management conversations.
type AnthropicClient struct {
	client anthropic.Client
	model  anthropic.Model
	store  store.Store
}

func NewAnthropicClient(apiKey, model string, s store.Store) *AnthropicClient {
	c := anthropic.NewClient(option.WithAPIKey(apiKey))
	m := anthropic.Model(model)
	if m == "" {
		m = anthropic.ModelClaudeSonnet4_6
	}
	return &AnthropicClient{client: c, model: m, store: s}
}

// Chat processes a natural language message, executes any tool calls, and returns the response.
func (c *AnthropicClient) Chat(ctx context.Context, userMessage string) (string, []string, error) {
	jobs, _ := c.store.ListJobs(ctx)
	jobsJSON, _ := json.MarshalIndent(jobs, "", "  ")

	systemWithJobs := systemPrompt + "\n\nCurrent jobs:\n```json\n" + string(jobsJSON) + "\n```"

	tools := buildTools()

	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(userMessage)),
	}

	var actions []string
	var finalText string

	for {
		resp, err := c.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     c.model,
			MaxTokens: 1024,
			System: []anthropic.TextBlockParam{
				{Text: systemWithJobs},
			},
			Tools:    tools,
			Messages: messages,
		})
		if err != nil {
			return "", nil, fmt.Errorf("anthropic API: %w", err)
		}

		// Collect text from this response
		for _, block := range resp.Content {
			if block.Type == "text" {
				finalText += block.Text
			}
		}

		if resp.StopReason == anthropic.StopReasonEndTurn || len(resp.Content) == 0 {
			break
		}

		if resp.StopReason != anthropic.StopReasonToolUse {
			break
		}

		// Process tool calls
		var toolResults []anthropic.ContentBlockParamUnion

		for _, block := range resp.Content {
			if block.Type != "tool_use" {
				continue
			}
			toolUse := block.AsToolUse()

			result, action, execErr := executeTool(ctx, c.store, toolUse.Name, toolUse.Input)
			if execErr != nil {
				result = fmt.Sprintf("Error: %v", execErr)
			} else {
				actions = append(actions, action)
			}

			toolResults = append(toolResults, anthropic.NewToolResultBlock(toolUse.ID, result, execErr != nil))
		}

		if len(toolResults) == 0 {
			break
		}

		// Convert resp.Content to params for message history
		assistantBlocks := make([]anthropic.ContentBlockParamUnion, len(resp.Content))
		for i, b := range resp.Content {
			assistantBlocks[i] = b.ToParam()
		}

		messages = append(messages,
			anthropic.NewAssistantMessage(assistantBlocks...),
			anthropic.NewUserMessage(toolResults...),
		)
	}

	return finalText, actions, nil
}

// executeTool is a package-level function so both AnthropicClient and OpenAIClient can use it.
func executeTool(ctx context.Context, s store.Store, toolName string, input json.RawMessage) (string, string, error) {
	switch toolName {
	case "create_job":
		return executeCreateJob(ctx, s, input)
	case "update_job":
		return executeUpdateJob(ctx, s, input)
	case "delete_job":
		return executeDeleteJob(ctx, s, input)
	default:
		return "", "", fmt.Errorf("unknown tool: %s", toolName)
	}
}
