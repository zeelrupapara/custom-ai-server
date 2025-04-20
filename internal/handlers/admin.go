package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/zeelrupapara/custom-ai-server/pkg/gpt"
)

// ReloadGPTs lets an admin reload all YAML configs
func ReloadGPTs(c *fiber.Ctx) error {
	if err := gpt.LoadConfigs("configs/gpts"); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "reload failed")
	}
	return c.SendStatus(fiber.StatusOK)
}
