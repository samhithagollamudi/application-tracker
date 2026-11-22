// Package config reads all runtime configuration from environment variables.
package config

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	DatabaseURL string
	SessionKey  []byte // 32 bytes; signs/derives session and CSRF secrets
	Port        int
	Env         string // "development" or "production"
}

func (c Config) IsProd() bool { return c.Env == "production" }

// Load parses the environment. DATABASE_URL is always required; SESSION_KEY
// is required in production (a random one is generated in development).
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
	if k := os.Getenv("SESSION_KEY"); k != "" {
		b, err := hex.DecodeString(k)
		if err != nil || len(b) != 32 {
			return c, errors.New("SESSION_KEY must be 64 hex characters (32 bytes); generate with: openssl rand -hex 32")
		}
		c.SessionKey = b
	} else if c.IsProd() {
		return c, errors.New("SESSION_KEY is required in production")
	}
	return c, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
