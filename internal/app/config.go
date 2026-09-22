package app

import (
	"errors"
	"os"
	"strconv"
	"time"
)

var ErrConfig = errors.New("invalid application configuration")

type Config struct {
	DatabaseURL      string
	MasterKeyVersion int
	HTTPAddr         string
	SSHTimeout       time.Duration
	ProviderTimeout  time.Duration
}

func LoadConfig() (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), HTTPAddr: os.Getenv("HTTP_ADDR"), MasterKeyVersion: 1, SSHTimeout: 8 * time.Second, ProviderTimeout: 30 * time.Second}
	if c.HTTPAddr == "" {
		c.HTTPAddr = "127.0.0.1:18080"
	}
	if v := os.Getenv("MASTER_KEY_VERSION"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, ErrConfig
		}
		c.MasterKeyVersion = n
	}
	if c.DatabaseURL == "" {
		return Config{}, ErrConfig
	}
	return c, nil
}
