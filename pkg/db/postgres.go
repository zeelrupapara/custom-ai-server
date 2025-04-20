package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zeelrupapara/custom-ai-server/pkg/config"
)

var PG *pgxpool.Pool

// ConnectPostgres initializes the global PG pool
func ConnectPostgres() error {
	cfg := config.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, cfg.DBUrl)
	if err != nil {
		return err
	}
	PG = pool
	return nil
}
