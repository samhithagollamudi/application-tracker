// Package config reads all runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL string
	Port        int
	Env         string // "development" or "production"
}

func (c Config) IsProd() bool { return c.Env == "production" }

// Load parses the environment. Only DATABASE_URL is required. Sessions live
// server-side in Postgres, so no signing key is needed.
func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Env:         getenv("APP_ENV", "development"),
		Port:        8080,
	}
	if c.Env != "development" && c.Env != "production" {
		return c, fmt.Errorf("APP_ENV must be development or production, got %q", c.Env)
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if p := os.Getenv("PORT"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n <= 0 || n > 65535 {
			return c, fmt.Errorf("PORT must be a valid port number, got %q", p)
		}
		c.Port = n
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
