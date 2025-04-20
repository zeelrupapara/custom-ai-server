package ai

import "context"

// AIModel defines the interface for any provider
type AIModel interface {
	ChatStream(ctx context.Context, prompt, systemPrompt string, temp float64) (<-chan string, error)
}
