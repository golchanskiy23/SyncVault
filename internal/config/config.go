package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
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
		Address      string        `json:"address" yaml:"address"`
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

	MongoDB struct {
		URI         string        `json:"uri" yaml:"uri"`
		Database    string        `json:"database" yaml:"database"`
		Timeout     time.Duration `json:"timeout" yaml:"timeout"`
		MaxPoolSize uint64        `json:"maxPoolSize" yaml:"maxPoolSize"`
		MinPoolSize uint64        `json:"minPoolSize" yaml:"minPoolSize"`
	} `json:"mongodb" yaml:"mongodb"`

	Kafka struct {
		Brokers             []string      `json:"brokers" yaml:"brokers"`
		GroupID             string        `json:"groupId" yaml:"groupId"`
		FileEventsTopic     string        `json:"fileEventsTopic" yaml:"fileEventsTopic"`
		SyncEventsTopic     string        `json:"syncEventsTopic" yaml:"syncEventsTopic"`
		ConflictEventsTopic string        `json:"conflictEventsTopic" yaml:"conflictEventsTopic"`
		DLQTopic            string        `json:"dlqTopic" yaml:"dlqTopic"`
		ProducerTimeout     time.Duration `json:"producerTimeout" yaml:"producerTimeout"`
		ConsumerTimeout     time.Duration `json:"consumerTimeout" yaml:"consumerTimeout"`
		MaxRetries          int           `json:"maxRetries" yaml:"maxRetries"`
		RetryBackoff        time.Duration `json:"retryBackoff" yaml:"retryBackoff"`
		BatchSize           int           `json:"batchSize" yaml:"batchSize"`
		BatchTimeout        time.Duration `json:"batchTimeout" yaml:"batchTimeout"`
	} `json:"kafka" yaml:"kafka"`

	JWT struct {
		AccessSecret  string        `json:"accessSecret" yaml:"accessSecret"`
		RefreshSecret string        `json:"refreshSecret" yaml:"refreshSecret"`
		AccessTTL     time.Duration `json:"accessTTL" yaml:"accessTTL"`
		RefreshTTL    time.Duration `json:"refreshTTL" yaml:"refreshTTL"`
		Issuer        string        `json:"issuer" yaml:"issuer"`
	} `json:"jwt" yaml:"jwt"`
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
	cfg.Redis.Address = fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
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

	// MongoDB настройки по умолчанию
	cfg.MongoDB.URI = "mongodb://localhost:27017"
	cfg.MongoDB.Database = "syncvault_audit"
	cfg.MongoDB.Timeout = 10 * time.Second
	cfg.MongoDB.MaxPoolSize = 10
	cfg.MongoDB.MinPoolSize = 5

	// Kafka настройки по умолчанию
	cfg.Kafka.Brokers = []string{"localhost:9092"}
	cfg.Kafka.GroupID = "syncvault-group"
	cfg.Kafka.FileEventsTopic = "file.events"
	cfg.Kafka.SyncEventsTopic = "sync.events"
	cfg.Kafka.ConflictEventsTopic = "conflict.events"
	cfg.Kafka.DLQTopic = "dlq.file.events"
	cfg.Kafka.ProducerTimeout = 10 * time.Second
	cfg.Kafka.ConsumerTimeout = 10 * time.Second
	cfg.Kafka.MaxRetries = 3
	cfg.Kafka.RetryBackoff = 1 * time.Second
	cfg.Kafka.BatchSize = 100
	cfg.Kafka.BatchTimeout = 1 * time.Second

	// JWT настройки по умолчанию
	cfg.JWT.AccessSecret = os.Getenv("JWT_ACCESS_SECRET")
	if cfg.JWT.AccessSecret == "" {
		cfg.JWT.AccessSecret = "your-access-secret-key-change-in-production"
	}
	cfg.JWT.RefreshSecret = os.Getenv("JWT_REFRESH_SECRET")
	if cfg.JWT.RefreshSecret == "" {
		cfg.JWT.RefreshSecret = "your-refresh-secret-key-change-in-production"
	}
	cfg.JWT.AccessTTL = 15 * time.Minute
	cfg.JWT.RefreshTTL = 30 * 24 * time.Hour
	cfg.JWT.Issuer = "syncvault"

	return cfg
}

func LoadFromFile(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		// If config file doesn't exist, return default config
		return Default(), nil
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
	}

	// Apply defaults for any missing Kafka configuration
	if len(cfg.Kafka.Brokers) == 0 {
		// Check environment variable first
		if brokers := os.Getenv("KAFKA_BROKERS"); brokers != "" {
			cfg.Kafka.Brokers = []string{brokers}
		} else {
			cfg.Kafka.Brokers = []string{"localhost:9092"}
		}
	}
	if cfg.Kafka.GroupID == "" {
		if groupID := os.Getenv("KAFKA_GROUP_ID"); groupID != "" {
			cfg.Kafka.GroupID = groupID
		} else {
			cfg.Kafka.GroupID = "syncvault-group"
		}
	}
	if cfg.Kafka.FileEventsTopic == "" {
		cfg.Kafka.FileEventsTopic = "file.events"
	}
	if cfg.Kafka.SyncEventsTopic == "" {
		cfg.Kafka.SyncEventsTopic = "sync.events"
	}
	if cfg.Kafka.ConflictEventsTopic == "" {
		cfg.Kafka.ConflictEventsTopic = "conflict.events"
	}
	if cfg.Kafka.DLQTopic == "" {
		cfg.Kafka.DLQTopic = "dlq.file.events"
	}
	if cfg.Kafka.ProducerTimeout == 0 {
		cfg.Kafka.ProducerTimeout = 10 * time.Second
	}
	if cfg.Kafka.ConsumerTimeout == 0 {
		cfg.Kafka.ConsumerTimeout = 10 * time.Second
	}
	if cfg.Kafka.MaxRetries == 0 {
		if retries := os.Getenv("KAFKA_MAX_RETRIES"); retries != "" {
			if parsed, err := strconv.Atoi(retries); err == nil {
				cfg.Kafka.MaxRetries = parsed
			}
		} else {
			cfg.Kafka.MaxRetries = 3
		}
	}
	if cfg.Kafka.RetryBackoff == 0 {
		cfg.Kafka.RetryBackoff = 1 * time.Second
	}
	if cfg.Kafka.BatchSize == 0 {
		cfg.Kafka.BatchSize = 100
	}
	if cfg.Kafka.BatchTimeout == 0 {
		cfg.Kafka.BatchTimeout = 1 * time.Second
	}

	// Добавляем Redis адрес для удобства (если не установлен)
	if cfg.Redis.Address == "" {
		cfg.Redis.Address = fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	}

	return &cfg, nil
}

// LoadConfig загружает конфигурацию из файла или переменных окружения
func LoadConfig() *Config {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "internal/config/config.yml"
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		log.Printf("Failed to load config from file %s: %v, using defaults", configPath, err)
		return Default()
	}

	// Добавляем Redis адрес для удобства (если не установлен)
	if cfg.Redis.Address == "" {
		cfg.Redis.Address = fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)
	}

	return cfg
}
