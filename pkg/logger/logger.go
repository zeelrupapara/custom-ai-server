package logger

import (
	"go.uber.org/zap"
)

// New returns a production Zap logger
func New() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.LevelKey = "level"
	cfg.EncoderConfig.MessageKey = "msg"
	return cfg.Build()
}
