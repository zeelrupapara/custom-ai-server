package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

type OpenAI struct {
	APIKey string
	Model  string
}

// NewOpenAI returns an OpenAI client
func NewOpenAI(model string) *OpenAI {
	return &OpenAI{
		APIKey: os.Getenv("OPENAI_API_KEY"),
		Model:  model,
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

	fmt.Println(body)

	reqHTTP, _ := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/completions", bytes.NewBuffer(b))
	reqHTTP.Header.Set("Authorization", "Bearer "+o.APIKey)
	reqHTTP.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(reqHTTP)
	if err != nil {
		return nil, err
	}
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
