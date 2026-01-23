package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort   int
	SMTPPort   int
	DBPath     string
	AuthSecret string
}

func Load() Config {
	return Config{
		HTTPPort:   getEnvInt("HTTP_PORT", 3025),
		SMTPPort:   getEnvInt("SMTP_PORT", 2025),
		DBPath:     getEnvString("DB_PATH", ""),
		AuthSecret: getEnvString("AUTH_SECRET", ""),
	}
}

func getEnvString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return fallback
}
