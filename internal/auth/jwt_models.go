package auth

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims содержит утверждения для токена
type JWTClaims struct {
	UserID    string `json:"sub"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TokenID   string `json:"jti"`
	TokenType string `json:"type"` // "access" или "refresh"
	jwt.RegisteredClaims
}

// TokenPair представляет пару access и refresh токенов
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"` // в секундах
}

// RefreshTokenData содержит данные для хранения в Redis
type RefreshTokenData struct {
	UserID    string    `json:"user_id"`
	TokenID   string    `json:"token_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	IsRevoked bool      `json:"is_revoked"`
}

// LoginRequest представляет запрос на вход
type LoginRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

// RefreshRequest представляет запрос на обновление токена
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// AuthResponse представляет ответ аутентификации
type AuthResponse struct {
	TokenPair `json:"tokens"`
	User      UserInfo `json:"user"`
}

// UserInfo содержит информацию о пользователе
type UserInfo struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"` // не сериализуется в JSON
}

// JWTConfig содержит конфигурацию JWT
type JWTConfig struct {
	AccessSecret  string        `json:"access_secret"`
	RefreshSecret string        `json:"refresh_secret"`
	AccessTTL     time.Duration `json:"access_ttl"`
	RefreshTTL    time.Duration `json:"refresh_ttl"`
	Issuer        string        `json:"issuer"`
}

// DefaultJWTConfig возвращает конфигурацию по умолчанию
func DefaultJWTConfig() *JWTConfig {
	return &JWTConfig{
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 30 * 24 * time.Hour, // 30 дней
		Issuer:     "syncvault",
	}
}

// TokenType типы токенов
type TokenType string

const (
	AccessTokenType  TokenType = "access"
	RefreshTokenType TokenType = "refresh"
)

// Redis ключи
const (
	RefreshTokenKeyPrefix = "refresh_token:"
	BlacklistKeyPrefix    = "blacklist:"
)

// Context ключи для передачи данных через middleware
const (
	UserIDKey  = "user_id"
	EmailKey   = "email"
	RoleKey    = "role"
	TokenIDKey = "token_id"
	ClaimsKey  = "claims"
)
