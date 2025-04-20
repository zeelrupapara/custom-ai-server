package handlers

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofiber/fiber/v2"
	"github.com/ledongthuc/pdf"
	"github.com/zeelrupapara/custom-ai-server/pkg/db"
)

// UploadFile handles PDF/TXT uploads; extracts text and stores in Redis
func UploadFile(c *fiber.Ctx) error {
	userID := c.Locals("userID").(int)
	file, err := c.FormFile("file")
	if err != nil {
		return fiber.ErrBadRequest
	}
	dst := filepath.Join("uploads", file.Filename)
	if err := c.SaveFile(file, dst); err != nil {
		return fiber.ErrInternalServerError
	}
	f, err := os.Open(dst)
	if err != nil {
		return fiber.ErrInternalServerError
	}
	defer f.Close()

	reader, _ := pdf.NewReader(f, file.Size)
	var buf bytes.Buffer
	for i := 1; i <= reader.NumPage(); i++ {
		p := reader.Page(i)
		txt, _ := p.GetPlainText(nil)
		buf.WriteString(txt)
	}

	key := fmt.Sprintf("filectx:%d", userID)
	if err := db.RDB.Set(context.Background(), key, buf.String(), 0).Err(); err != nil {
		return fiber.ErrInternalServerError
	}
	return c.JSON(fiber.Map{"message": "uploaded"})
}
