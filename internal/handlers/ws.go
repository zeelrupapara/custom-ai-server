package handlers

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"github.com/zeelrupapara/custom-ai-server/pkg/ai"
	"github.com/zeelrupapara/custom-ai-server/pkg/gpt"
)

// WSUpgrade rejects non‑WebSocket requests
func WSUpgrade(c *fiber.Ctx) error {
	if websocket.IsWebSocketUpgrade(c) {
		// allow next() to call the actual websocket handler
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}

// HandleWS is the WebSocket entrypoint
func HandleWS(c *websocket.Conn) {
	slug := c.Params("slug")
	cfg, ok := gpt.Configs[slug]
	if !ok {
		c.WriteMessage(websocket.TextMessage, []byte("unknown GPT"))
		return
	}
	model := ai.NewOpenAI(cfg.Model)
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			break
		}
		stream, err := model.ChatStream(context.Background(), string(msg), cfg.SystemPrompt, cfg.Temperature)
		if err != nil {
			c.WriteMessage(websocket.TextMessage, []byte("AI error: "+err.Error()))
			continue
		}
		for chunk := range stream {
			c.WriteMessage(websocket.TextMessage, []byte(chunk))
		}
	}
}
