package db

import (
	"context"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/zeelrupapara/custom-ai-server/pkg/config"
)

var RDB *redis.Client

// ConnectRedis sets up the global Redis client
func ConnectRedis() error {
	cfg := config.Load()
	RDB = redis.NewClient(&redis.Options{
		Addr:        cfg.RedisAddr,
		DialTimeout: 5 * time.Second,
	})
	return RDB.Ping(context.Background()).Err()
}
