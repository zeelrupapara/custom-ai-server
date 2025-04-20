package config

import (
	"os"
	"strconv"
	"time"
)

// AppConfig holds all configuration loaded from ENV
type AppConfig struct {
	Port          string
	DBUrl         string
	RedisAddr     string
	JWTSecret     string
	OpenAIAPIKey  string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
}

// Load reads ENV vars into AppConfig
func Load() *AppConfig {
	readTimeout, _ := strconv.Atoi(os.Getenv("HTTP_READ_TIMEOUT_SEC"))
	writeTimeout, _ := strconv.Atoi(os.Getenv("HTTP_WRITE_TIMEOUT_SEC"))
	idleTimeout, _ := strconv.Atoi(os.Getenv("HTTP_IDLE_TIMEOUT_SEC"))
	return &AppConfig{
		Port:         os.Getenv("PORT"),
		DBUrl:        os.Getenv("DB_URL"),
		RedisAddr:    os.Getenv("REDIS_ADDR"),
		JWTSecret:    os.Getenv("JWT_SECRET"),
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		ReadTimeout:  time.Duration(readTimeout) * time.Second,
		WriteTimeout: time.Duration(writeTimeout) * time.Second,
		IdleTimeout:  time.Duration(idleTimeout) * time.Second,
	}
}
