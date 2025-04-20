package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
)

type OpenAI struct {
	APIKey string
	Model  string
	SystemMsgs []openai.ChatCompletionMessage
	Client *openai.Client
}

func PrepareSystemPrompt(client *openai.Client, model, systemPrompt string, filePaths []string) ([]openai.ChatCompletionMessage, error) {
	// load system messages
	msgs := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

	// check files
	if len(filePaths) > 0 {
		// upload the files
		for _, path := range filePaths {
			f, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("cannot open %s: %w", path, err)
			}
			upl := openai.FileBytesRequest{
				Name: filepath.Base(path),
				Bytes:    f,
				Purpose:  openai.PurposeFineTune,
			}
			resp, err := client.CreateFileBytes(context.Background(), upl)
			if err != nil {
				return nil, fmt.Errorf("upload failed for %s: %w", path, err)
			}

			msgs = append(msgs, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: fmt.Sprintf("Uploaded '%s' with ID %s", filepath.Base(path), resp.ID),
			})
		}
	}

	// Send initial prompt+file messages to GPT to create context
	initReq := openai.ChatCompletionRequest{
		Model:    model,
		Messages: msgs,
		// temperature 0 for deterministic system setup
		Temperature: 0,
	}
	initResp, err := client.CreateChatCompletion(context.Background(), initReq)
	if err != nil {
		return nil, fmt.Errorf("initial chat failed: %w", err)
	}

	// Append assistant's initial response to messages
	if len(initResp.Choices) > 0 {
		msgs = append(msgs, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: initResp.Choices[0].Message.Content,
		})
	}

	return initReq.Messages, nil
}

// NewOpenAI returns an OpenAI client
func NewOpenAI(model, systemPrompt string, filePaths []string, temp float64) *OpenAI {
	// check api key exist
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		panic("OPENAI_API_KEY is not set")
	}

	// connect with openai client session
	client := openai.NewClient(apiKey)

	// load the system prompt and files for custom gpt agents
	msgs, err := PrepareSystemPrompt(client, model, systemPrompt, filePaths)
	if err != nil {
		fmt.Println("Error while load the System Prompt")
	}

	return &OpenAI{
		APIKey: apiKey,
		Model:  model,
		Client: client,
		SystemMsgs: msgs,
	}
}

// ChatStream implements streaming chat (non‑stream stub here)
func (o *OpenAI) ChatStream(ctx context.Context, prompt, systemPrompt string, temp float64) (<-chan string, error) {
	type req struct {
		Model       string  `json:"model"`
		Prompt      string  `json:"prompt"`
		Temperature float64 `json:"temperature"`
		Stream      bool    `json:"stream"`
	}
	body := req{o.Model, systemPrompt + "\n\n" + prompt, temp, false}
	b, _ := json.Marshal(body)

	msgs := make([]openai.ChatCompletionMessage, len(o.SystemMsgs))
	copy(msgs, o.SystemMsgs)

	// Append the new user message
	msgs = append(msgs, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userMessage,
	})


	defer resp.Body.Close()

	var result struct {
		Choices []struct{ Text string } `json:"choices"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	out := make(chan string, 1)
	go func() {
		defer close(out)
		if len(result.Choices) > 0 {
			out <- result.Choices[0].Text
		}
	}()
	return out, nil
}
