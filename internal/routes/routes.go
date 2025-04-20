package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"
	"go.uber.org/zap"

	"github.com/zeelrupapara/custom-ai-server/internal/handlers"
	"github.com/zeelrupapara/custom-ai-server/pkg/auth"
	"github.com/zeelrupapara/custom-ai-server/pkg/config"
)

// NewRouter wires up all routes and middleware
func NewRouter(log *zap.Logger) *fiber.App {
	cfg := config.Load()
	app := fiber.New(fiber.Config{
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{"error": err.Error()})
		},
	})

	// Metrics endpoint
	// app.Get("/metrics", fiber.WrapHandler(promhttp.Handler()))

	// Auth
	app.Post("/register", auth.Register)
	app.Post("/login", auth.Login)

	// File upload (authenticated)
	app.Post("/upload", auth.Protect(false), handlers.UploadFile)

	// Admin only
	app.Post("/admin/reload", auth.Protect(true), handlers.ReloadGPTs)

	// WebSocket chat
	app.Use("/ws/:slug", auth.Protect(false), handlers.WSUpgrade)
	app.Get("/ws/:slug", websocket.New(handlers.HandleWS))

	return app
}
