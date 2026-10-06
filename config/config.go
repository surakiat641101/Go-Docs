// Package config loads application settings from the environment, optionally
// seeded from a .env file. Variables already set in the environment win over
// .env, so Docker / production values are never overridden by the file.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port              string // PORT
	BodyLimitMB       int    // BODY_LIMIT_MB: max request size, room for base64 images
	AllowRemoteImages bool   // ALLOW_REMOTE_IMAGES: let <img src="http(s)://..."> be fetched
	CORSAllowOrigins  string // CORS_ALLOW_ORIGINS: comma-separated, "*" for any
}

// Load reads .env (if present) and then the environment.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("read .env: %w", err)
	}
	return FromEnv()
}

// FromEnv builds a Config from the current environment only.
func FromEnv() (Config, error) {
	cfg := Config{
		Port:             getenv("PORT", "3000"),
		CORSAllowOrigins: getenv("CORS_ALLOW_ORIGINS", "*"),
	}
	var err error
	if cfg.BodyLimitMB, err = strconv.Atoi(getenv("BODY_LIMIT_MB", "20")); err != nil || cfg.BodyLimitMB <= 0 {
		return Config{}, fmt.Errorf("BODY_LIMIT_MB must be a positive integer")
	}
	if cfg.AllowRemoteImages, err = strconv.ParseBool(getenv("ALLOW_REMOTE_IMAGES", "false")); err != nil {
		return Config{}, fmt.Errorf("ALLOW_REMOTE_IMAGES must be true or false")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
