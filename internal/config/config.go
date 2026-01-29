package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPPort        int
	SMTPPort        int
	DBPath          string
	AuthSecret      string
	SMTPAuthEnabled bool
	SMTPUsername    string
	SMTPPassword    string
}

func Load() Config {
	return Config{
		HTTPPort:        getEnvInt("HTTP_PORT", 3025),
		SMTPPort:        getEnvInt("SMTP_PORT", 2025),
		DBPath:          getEnvString("DB_PATH", ""),
		AuthSecret:      getEnvString("AUTH_SECRET", ""),
		SMTPAuthEnabled: getEnvBool("SMTP_AUTH_ENABLED", true),
		SMTPUsername:    getEnvString("SMTP_USERNAME", "localsmtp"),
		SMTPPassword:    getEnvString("SMTP_PASSWORD", "localsmtp"),
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

func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed
		}
	}
	return fallback
}
