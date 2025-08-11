package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v2"
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

func (c *Config) Address() string {
	if c.HTTP.Host == "" {
		return fmt.Sprintf(":%d", c.HTTP.Port)
	}

	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}

func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
	}

	return &cfg, nil
}
