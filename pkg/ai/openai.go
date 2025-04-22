package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sashabaranov/go-openai"
)

type OpenAI struct {
	APIKey     string
	Model      string
	SystemMsgs string
	Client     *openai.Client
}

func PrepareSystemPrompt(systemPrompt string, filePaths []string) (string, error) {
	// Load files and include their contents directly in the prompt

	//
	// problem 1: directyly including file contents in the system prompt can lead to large prompts
	// problem 2: if the file is too large, it may exceed the token limit of the model
	//
	for _, path := range filePaths {
		content, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("cannot read %s: %w", path, err)
		}

		systemPrompt += fmt.Sprintf("\n\nContent of file '%s':\n%s",
			filepath.Base(path),
			string(content))
	}

	return systemPrompt, nil
}

// NewOpenAI returns an OpenAI client with file contents embedded in system prompt
func NewOpenAI(model, systemPrompt string, filePaths []string, temp float32) *OpenAI {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		panic("OPENAI_API_KEY is not set")
	}

	// Embed file contents into system prompt
	fullPrompt, err := PrepareSystemPrompt(systemPrompt, filePaths)
	if err != nil {
		fmt.Println("Error preparing system prompt:", err)
		os.Exit(1)
	}

	return &OpenAI{
		APIKey:     apiKey,
		Model:      model,
		Client:     openai.NewClient(apiKey),
		SystemMsgs: fullPrompt,
	}
}

// ChatStream implements streaming chat (simplified version)
func (o *OpenAI) ChatStream(ctx context.Context, prompt string, temp float32) (<-chan string, error) {
	stream, err := o.Client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: o.Model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: o.SystemMsgs,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: temp,
		// Stream:      false,
	})
	if err != nil {
		return nil, fmt.Errorf("streaming chat failed: %w", err)
	}
	fmt.Println(stream.RecvRaw())
	out := make(chan string)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				fmt.Println("Stream error:", err)
				return
			}
			if len(msg.Choices) > 0 {
				out <- msg.Choices[0].Delta.Content
			}
		}
	}()
	return out, nil
}
