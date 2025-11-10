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

	Database struct {
		Host            string        `json:"host" yaml:"host"`
		Port            int           `json:"port" yaml:"port"`
		User            string        `json:"user" yaml:"user"`
		Password        string        `json:"password" yaml:"password"`
		DBName          string        `json:"dbname" yaml:"dbname"`
		SSLMode         string        `json:"sslmode" yaml:"sslmode"`
		MaxOpenConns    int           `json:"maxOpenConns" yaml:"maxOpenConns"`
		MaxIdleConns    int           `json:"maxIdleConns" yaml:"maxIdleConns"`
		ConnMaxLifetime time.Duration `json:"connMaxLifetime" yaml:"connMaxLifetime"`
	} `json:"database" yaml:"database"`

	Redis struct {
		Host         string        `json:"host" yaml:"host"`
		Port         int           `json:"port" yaml:"port"`
		Password     string        `json:"password" yaml:"password"`
		DB           int           `json:"db" yaml:"db"`
		PoolSize     int           `json:"poolSize" yaml:"poolSize"`
		MinIdleConns int           `json:"minIdleConns" yaml:"minIdleConns"`
		MaxRetries   int           `json:"maxRetries" yaml:"maxRetries"`
		DialTimeout  time.Duration `json:"dialTimeout" yaml:"dialTimeout"`
		ReadTimeout  time.Duration `json:"readTimeout" yaml:"readTimeout"`
		WriteTimeout time.Duration `json:"writeTimeout" yaml:"writeTimeout"`
		PoolTimeout  time.Duration `json:"poolTimeout" yaml:"poolTimeout"`
	} `json:"redis" yaml:"redis"`

	Cache struct {
		FileMetadataTTL time.Duration `json:"fileMetadataTTL" yaml:"fileMetadataTTL"`
	} `json:"cache" yaml:"cache"`

	Session struct {
		TokenExpiry time.Duration `json:"tokenExpiry" yaml:"tokenExpiry"`
		MaxSessions int           `json:"maxSessions" yaml:"maxSessions"`
	} `json:"session" yaml:"session"`

	RateLimit struct {
		RequestsPerMinute int `json:"requestsPerMinute" yaml:"requestsPerMinute"`
		RequestsPerHour   int `json:"requestsPerHour" yaml:"requestsPerHour"`
	} `json:"rateLimit" yaml:"rateLimit"`

	Lock struct {
		DefaultTTL time.Duration `json:"defaultTTL" yaml:"defaultTTL"`
		RetryCount int           `json:"retryCount" yaml:"retryCount"`
		RetryDelay time.Duration `json:"retryDelay" yaml:"retryDelay"`
	} `json:"lock" yaml:"lock"`

	// Другие секции для Kafka, NATS, БД и т.д.
}

func (c *Config) Address() string {
	if c.HTTP.Host == "" {
		return fmt.Sprintf(":%d", c.HTTP.Port)
	}

	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}

// Default возвращает конфигурацию по умолчанию
func Default() *Config {
	cfg := &Config{}

	cfg.HTTP.Host = ""
	cfg.HTTP.Port = 8080
	cfg.HTTP.ReadTimeout = 15 * time.Second
	cfg.HTTP.WriteTimeout = 15 * time.Second
	cfg.HTTP.IdleTimeout = 60 * time.Second
	cfg.Shutdown.Timeout = 30 * time.Second

	// PostgreSQL настройки по умолчанию
	cfg.Database.Host = "localhost"
	cfg.Database.Port = 5432
	cfg.Database.User = "postgres"
	cfg.Database.Password = ""
	cfg.Database.DBName = "syncvault"
	cfg.Database.SSLMode = "disable"
	cfg.Database.MaxOpenConns = 25
	cfg.Database.MaxIdleConns = 5
	cfg.Database.ConnMaxLifetime = 5 * time.Minute

	// Redis настройки по умолчанию
	cfg.Redis.Host = "localhost"
	cfg.Redis.Port = 6379
	cfg.Redis.Password = ""
	cfg.Redis.DB = 0
	cfg.Redis.PoolSize = 10
	cfg.Redis.MinIdleConns = 5
	cfg.Redis.MaxRetries = 3
	cfg.Redis.DialTimeout = 5 * time.Second
	cfg.Redis.ReadTimeout = 3 * time.Second
	cfg.Redis.WriteTimeout = 3 * time.Second
	cfg.Redis.PoolTimeout = 4 * time.Second

	// Cache настройки
	cfg.Cache.FileMetadataTTL = 5 * time.Minute

	// Session настройки
	cfg.Session.TokenExpiry = 24 * time.Hour
	cfg.Session.MaxSessions = 5

	// Rate limiting настройки
	cfg.RateLimit.RequestsPerMinute = 60
	cfg.RateLimit.RequestsPerHour = 1000

	// Lock настройки
	cfg.Lock.DefaultTTL = 30 * time.Second
	cfg.Lock.RetryCount = 3
	cfg.Lock.RetryDelay = 100 * time.Millisecond

	return cfg
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
