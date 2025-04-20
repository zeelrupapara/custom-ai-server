package main

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/zeelrupapara/custom-ai-server/pkg/db"
	"github.com/zeelrupapara/custom-ai-server/pkg/gpt"
	"github.com/zeelrupapara/custom-ai-server/pkg/logger"
	"github.com/zeelrupapara/custom-ai-server/pkg/migration"

	"github.com/zeelrupapara/custom-ai-server/internal/routes"
)

func main() {
	// 1. Load env
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️  .env not found, using system ENV")
	}

	// 2. Init logger
	logg, _ := logger.New()
	defer logg.Sync()
	logg.Info("Starting custom-ai-server")

	if err := migration.Up(); err != nil {
		logg.Fatal("Migration failed", zap.Error(err))
	}

	// 3. Connect to services
	if err := db.ConnectPostgres(); err != nil {
		logg.Fatal("Postgres connect failed", zap.Error(err))
	}
	if err := db.ConnectRedis(); err != nil {
		logg.Fatal("Redis connect failed", zap.Error(err))
	}

	// 4. Load GPT configs
	if err := gpt.LoadConfigs("configs/gpts"); err != nil {
		logg.Fatal("Failed to load GPT configs", zap.Error(err))
	}

	// 5. Start HTTP server & routes
	app := routes.NewRouter(logg)
	port := os.Getenv("PORT")
	logg.Info("Listening", zap.String("port", port))
	if err := app.Listen(":" + port); err != nil {
		logg.Fatal("Server failed", zap.Error(err))
	}
}
