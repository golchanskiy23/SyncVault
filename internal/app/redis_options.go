package app

import (
	"fmt"
	"log"

	"syncvault/internal/cache"
	"syncvault/internal/config"
	"syncvault/internal/lock"
	"syncvault/internal/ratelimit"
	"syncvault/internal/redis"
	"syncvault/internal/session"
)

type RedisComponents struct {
	Client          *redis.Client
	FileCache       *cache.FileCache
	RateLimiter     *ratelimit.RateLimiter
	SessionStore    *session.SessionStore
	DistributedLock *lock.DistributedLock
}

// WithRedis инициализирует Redis компоненты
func WithRedis(cfg *config.Config) Option {
	return func(a *App) error {
		log.Printf("Initializing Redis components...")

		// Создаем Redis клиент
		redisConfig := redis.Config{
			Host:         cfg.Redis.Host,
			Port:         cfg.Redis.Port,
			Password:     cfg.Redis.Password,
			DB:           cfg.Redis.DB,
			PoolSize:     cfg.Redis.PoolSize,
			MinIdleConns: cfg.Redis.MinIdleConns,
			MaxRetries:   cfg.Redis.MaxRetries,
			DialTimeout:  cfg.Redis.DialTimeout,
			ReadTimeout:  cfg.Redis.ReadTimeout,
			WriteTimeout: cfg.Redis.WriteTimeout,
			PoolTimeout:  cfg.Redis.PoolTimeout,
		}

		redisClient := redis.NewClient(redisConfig)

		// Проверяем соединение с Redis
		if err := redisClient.Ping(a.ctx); err != nil {
			log.Printf("Failed to connect to Redis: %v", err)
			return fmt.Errorf("failed to connect to Redis: %w", err)
		}
		log.Printf("✓ Connected to Redis at %s:%d", cfg.Redis.Host, cfg.Redis.Port)

		// Инициализируем компоненты
		fileCache := cache.NewFileCache(redisClient.Redis(), cfg.Cache.FileMetadataTTL)
		rateLimiter := ratelimit.NewRateLimiter(redisClient.Redis())
		sessionStore := session.NewSessionStore(
			redisClient.Redis(),
			cfg.Session.TokenExpiry,
			cfg.Session.MaxSessions,
		)
		distributedLock := lock.NewDistributedLock(redisClient.Redis(), "syncvault")

		// Сохраняем в App
		a.redis.Client = redisClient
		a.redis.FileCache = fileCache
		a.redis.RateLimiter = rateLimiter
		a.redis.SessionStore = sessionStore
		a.redis.DistributedLock = distributedLock

		log.Printf("✓ Redis components initialized successfully")
		return nil
	}
}
