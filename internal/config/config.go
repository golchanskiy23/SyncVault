package config

import (
	"fmt"
	"time"
)

type Config struct {
	HTTP struct {
		Host         string        `json:"host" yaml:"host"`
		Port         int           `json:"port" yaml:"port"`
		ReadTimeout  time.Duration `json:"readTimeout" yaml:"readTimeout"`
		WriteTimeout time.Duration `json:"writeTimeout" yaml:"writeTimeout"`
		IdleTimeout  time.Duration `json:"idleTimeout" yaml:"idleTimeout"`
	} `json:"http" yaml:"http"`

	Shutdown struct {
		Timeout time.Duration `json:"timeout" yaml:"timeout"`
	} `json:"shutdown" yaml:"shutdown"`

	// Другие секции для Kafka, NATS, БД и т.д.
}

func Default() *Config {
	cfg := &Config{}

	cfg.HTTP.Host = ""
	cfg.HTTP.Port = 8080
	cfg.HTTP.ReadTimeout = 15 * time.Second
	cfg.HTTP.WriteTimeout = 15 * time.Second
	cfg.HTTP.IdleTimeout = 60 * time.Second
	cfg.Shutdown.Timeout = 30 * time.Second

	return cfg
}

func (c *Config) Address() string {
	if c.HTTP.Host == "" {
		return fmt.Sprintf(":%d", c.HTTP.Port)
	}

	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}
