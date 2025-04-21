package ai

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sashabaranov/go-openai"
)

type OpenAI struct {
	APIKey     string
	Model      string
	SystemMsgs []openai.ChatCompletionMessage
	Client     *openai.Client
}

func PrepareSystemPrompt(client *openai.Client, model, systemPrompt string, filePaths []string) ([]openai.ChatCompletionMessage, error) {
	// load system messages
	msgs := []openai.ChatCompletionMessage{{
		Role:    openai.ChatMessageRoleSystem,
		Content: systemPrompt,
	}}

	fmt.Println("System prompt:", systemPrompt)

	// check files
	if len(filePaths) > 0 {
		// upload the files
		for _, path := range filePaths {
			f, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("cannot open %s: %w", path, err)
			}
			upl := openai.FileBytesRequest{
				Name:    filepath.Base(path),
				Bytes:   f,
				Purpose: openai.PurposeFineTune,
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

	fmt.Println("Initial response:", initResp.Choices[0].Message.Content)

	// Append assistant's initial response to messages
	// if len(initResp.Choices) > 0 {
	// 	msgs = append(msgs, openai.ChatCompletionMessage{
	// 		Role:    openai.ChatMessageRoleAssistant,
	// 		Content: initResp.Choices[0].Message.Content,
	// 	})
	// }

	return initReq.Messages, nil
}

// NewOpenAI returns an OpenAI client
func NewOpenAI(model, systemPrompt string, filePaths []string, temp float32) *OpenAI {
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
		fmt.Println("Error while load the System Prompt", err)
	}

	return &OpenAI{
		APIKey:     apiKey,
		Model:      model,
		Client:     client,
		SystemMsgs: msgs,
	}
}

// ChatStream implements streaming chat (non‑stream stub here)
func (o *OpenAI) ChatStream(ctx context.Context, prompt string, temp float32) (<-chan string, error) {
	stream, err := o.Client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       o.Model,
		Messages:    append(o.SystemMsgs, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: prompt}),
		Temperature: temp,
		Stream:      true,
	})
	if err != nil {
		return nil, fmt.Errorf("streaming chat failed: %w", err)
	}

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
				fmt.Println(err)
				return
			}
			fmt.Println("Received message:", msg.Choices[0], msg.Choices[0].Delta.Content)
			out <- msg.Choices[0].Delta.Content
			time.Sleep(time.Millisecond * 100)
		}
	}()
	return out, nil
}

func (o *OpenAI) SystemMsgsString() string {
	var out string
	for _, msg := range o.SystemMsgs {
		out += msg.Content + "\n"
	}
	return out
}
