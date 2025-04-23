package handlers

import (
	"context"
	"fmt"

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
	model, err := ai.NewAI(context.Background(), cfg.Name, cfg.Model, cfg.SystemPrompt, cfg.Files)
	if err != nil {
		c.WriteMessage(websocket.TextMessage, []byte("AI error: "+err.Error()))
		return
	}
	c.WriteMessage(websocket.TextMessage, []byte("Your assistant is ready, ask anything to "+cfg.Name))
	for {
		_, msg, err := c.ReadMessage()
		if err != nil {
			break
		}
		fmt.Println("Received message:", string(msg))
		stream, err := model.Chat(context.Background(), string(msg))
		if err != nil {
			c.WriteMessage(websocket.TextMessage, []byte("AI error: "+err.Error()))
			continue
		}
		c.WriteMessage(websocket.TextMessage, []byte(stream))
	}
}
