package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisConfig содержит конфигурацию Redis
type RedisConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// DefaultRedisConfig возвращает конфигурацию Redis по умолчанию
func DefaultRedisConfig() *RedisConfig {
	return &RedisConfig{
		Host:     "localhost",
		Port:     6379,
		Password: "",
		DB:       0,
	}
}

// NewRedisClient создает новый Redis клиент
func NewRedisClient(config *RedisConfig) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", config.Host, config.Port),
		Password: config.Password,
		DB:       config.DB,
	})

	// Проверяем соединение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return rdb, nil
}

// RedisTokenRepository реализует хранение токенов в Redis
type RedisTokenRepository struct {
	client *redis.Client
	config *JWTConfig
}

// NewRedisTokenRepository создает новый репозиторий токенов в Redis
func NewRedisTokenRepository(client *redis.Client, config *JWTConfig) *RedisTokenRepository {
	return &RedisTokenRepository{
		client: client,
		config: config,
	}
}

// StoreRefreshToken сохраняет refresh токен в Redis
func (r *RedisTokenRepository) StoreRefreshToken(ctx context.Context, tokenData *RefreshTokenData) error {
	key := RefreshTokenKeyPrefix + tokenData.TokenID

	// Используем JSON для хранения структуры
	data, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal token data: %w", err)
	}

	return r.client.Set(ctx, key, data, time.Until(tokenData.ExpiresAt)).Err()
}

// GetRefreshToken получает refresh токен из Redis
func (r *RedisTokenRepository) GetRefreshToken(ctx context.Context, tokenID string) (*RefreshTokenData, error) {
	key := RefreshTokenKeyPrefix + tokenID

	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var tokenData RefreshTokenData
	err = json.Unmarshal([]byte(data), &tokenData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal token data: %w", err)
	}

	return &tokenData, nil
}

// RevokeRefreshToken отзывает refresh токен
func (r *RedisTokenRepository) RevokeRefreshToken(ctx context.Context, tokenID string) error {
	key := RefreshTokenKeyPrefix + tokenID

	// Получаем текущие данные токена
	tokenData, err := r.GetRefreshToken(ctx, tokenID)
	if err != nil {
		return err
	}

	// Помечаем как отозванный
	tokenData.IsRevoked = true

	// Сериализуем обновленные данные
	data, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal revoked token data: %w", err)
	}

	// Обновляем с тем же TTL
	return r.client.Set(ctx, key, data, time.Until(tokenData.ExpiresAt)).Err()
}

// RevokeAllUserTokens отзывает все refresh токены пользователя
func (r *RedisTokenRepository) RevokeAllUserTokens(ctx context.Context, userID string) error {
	// Находим все токены пользователя
	pattern := RefreshTokenKeyPrefix + "*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to find user tokens: %w", err)
	}

	// Отзываем все токены пользователя
	for _, key := range keys {
		tokenData, err := r.GetRefreshTokenBykey(ctx, key)
		if err == nil && tokenData.UserID == userID {
			tokenData.IsRevoked = true
			// Сериализуем и обновляем
			data, marshalErr := json.Marshal(tokenData)
			if marshalErr == nil {
				r.client.Set(ctx, key, data, time.Until(tokenData.ExpiresAt))
			}
		}
	}

	return nil
}

// GetRefreshTokenBykey получает токен по ключу Redis
func (r *RedisTokenRepository) GetRefreshTokenBykey(ctx context.Context, key string) (*RefreshTokenData, error) {
	data, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var tokenData RefreshTokenData
	err = json.Unmarshal([]byte(data), &tokenData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal token data: %w", err)
	}

	return &tokenData, nil
}

// CleanupExpiredTokens очищает истекшие токены
func (r *RedisTokenRepository) CleanupExpiredTokens(ctx context.Context) error {
	// Redis автоматически удаляет истекшие ключи благодаря TTL
	// Этот метод может быть использован для принудительной очистки
	pattern := RefreshTokenKeyPrefix + "*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to scan tokens: %w", err)
	}

	now := time.Now()
	for _, key := range keys {
		tokenData, err := r.GetRefreshTokenBykey(ctx, key)
		if err == nil && tokenData.ExpiresAt.Before(now) {
			r.client.Del(ctx, key)
		}
	}

	return nil
}

// GetActiveTokensCount возвращает количество активных токенов пользователя
func (r *RedisTokenRepository) GetActiveTokensCount(ctx context.Context, userID string) (int64, error) {
	pattern := RefreshTokenKeyPrefix + "*"
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to scan tokens: %w", err)
	}

	count := int64(0)
	for _, key := range keys {
		tokenData, err := r.GetRefreshTokenBykey(ctx, key)
		if err == nil && tokenData.UserID == userID && !tokenData.IsRevoked {
			count++
		}
	}

	return count, nil
}

// HealthCheck проверяет доступность Redis
func (r *RedisTokenRepository) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}
