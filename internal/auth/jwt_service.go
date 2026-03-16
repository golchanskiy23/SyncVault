package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

// JWTService предоставляет сервис для работы с JWT токенами
type JWTService struct {
	config *JWTConfig
	redis  *redis.Client
}

// NewJWTService создает новый JWT сервис
func NewJWTService(config *JWTConfig, redis *redis.Client) *JWTService {
	return &JWTService{
		config: config,
		redis:  redis,
	}
}

// GenerateTokenPair генерирует пару access и refresh токенов
func (s *JWTService) GenerateTokenPair(ctx context.Context, userID, email, role string) (*TokenPair, error) {
	now := time.Now()

	// Генерируем уникальный ID для токенов
	tokenID, err := generateTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token ID: %w", err)
	}

	// Создаем access token
	accessClaims := &JWTClaims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenID:   tokenID,
		TokenType: string(AccessTokenType),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   userID,
			Audience:  []string{"syncvault-api"},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.AccessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        tokenID,
		},
	}

	accessToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString([]byte(s.config.AccessSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Создаем refresh token
	refreshTokenID, err := generateTokenID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token ID: %w", err)
	}

	refreshClaims := &JWTClaims{
		UserID:    userID,
		Email:     email,
		Role:      role,
		TokenID:   refreshTokenID,
		TokenType: string(RefreshTokenType),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Subject:   userID,
			Audience:  []string{"syncvault-api"},
			ExpiresAt: jwt.NewNumericDate(now.Add(s.config.RefreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ID:        refreshTokenID,
		},
	}

	refreshToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString([]byte(s.config.RefreshSecret))
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	// Сохраняем refresh token в Redis
	err = s.storeRefreshToken(ctx, refreshTokenID, userID, now.Add(s.config.RefreshTTL))
	if err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(s.config.AccessTTL.Seconds()),
	}, nil
}

// ValidateToken валидирует JWT токен и возвращает claims
func (s *JWTService) ValidateToken(tokenString string, tokenType TokenType) (*JWTClaims, error) {
	var secret string

	switch tokenType {
	case AccessTokenType:
		secret = s.config.AccessSecret
	case RefreshTokenType:
		secret = s.config.RefreshSecret
	default:
		return nil, fmt.Errorf("unsupported token type: %s", tokenType)
	}

	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Проверяем тип токена
	if claims.TokenType != string(tokenType) {
		return nil, fmt.Errorf("token type mismatch: expected %s, got %s", tokenType, claims.TokenType)
	}

	return claims, nil
}

// RefreshTokens выполняет ротацию refresh токена
func (s *JWTService) RefreshTokens(ctx context.Context, refreshToken string) (*TokenPair, error) {
	// Валидируем refresh token
	claims, err := s.ValidateToken(refreshToken, RefreshTokenType)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Проверяем, что токен не отозван и существует в Redis
	tokenData, err := s.getRefreshToken(ctx, claims.TokenID)
	if err != nil {
		return nil, fmt.Errorf("refresh token not found or revoked: %w", err)
	}

	// Проверяем, что токен принадлежит тому же пользователю
	if tokenData.UserID != claims.UserID {
		return nil, fmt.Errorf("token user mismatch")
	}

	// Проверяем, что токен не был использован ранее (защита от кражи)
	if tokenData.IsRevoked {
		return nil, fmt.Errorf("refresh token has been revoked - possible theft detected")
	}

	// Отзываем старый refresh token
	err = s.revokeRefreshToken(ctx, claims.TokenID)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke old refresh token: %w", err)
	}

	// Генерируем новую пару токенов
	newTokenPair, err := s.GenerateTokenPair(ctx, claims.UserID, claims.Email, claims.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new tokens: %w", err)
	}

	return newTokenPair, nil
}

// RevokeRefreshToken отзывает refresh токен
func (s *JWTService) RevokeRefreshToken(ctx context.Context, tokenID string) error {
	return s.revokeRefreshToken(ctx, tokenID)
}

// Logout выполняет полный logout пользователя
func (s *JWTService) Logout(ctx context.Context, userID string) error {
	// Находим все активные refresh токены пользователя
	pattern := RefreshTokenKeyPrefix + "*"
	keys, err := s.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to find user tokens: %w", err)
	}

	// Отзываем все токены пользователя
	for _, key := range keys {
		tokenData, err := s.getRefreshTokenBykey(ctx, key)
		if err == nil && tokenData.UserID == userID {
			s.redis.Del(ctx, key)
		}
	}

	return nil
}

// Вспомогательные методы

func (s *JWTService) storeRefreshToken(ctx context.Context, tokenID, userID string, expiresAt time.Time) error {
	tokenData := RefreshTokenData{
		UserID:    userID,
		TokenID:   tokenID,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		IsRevoked: false,
	}

	key := RefreshTokenKeyPrefix + tokenID
	// Сериализуем в JSON для хранения в Redis
	data, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal token data: %w", err)
	}

	return s.redis.Set(ctx, key, data, s.config.RefreshTTL).Err()
}

func (s *JWTService) getRefreshToken(ctx context.Context, tokenID string) (*RefreshTokenData, error) {
	key := RefreshTokenKeyPrefix + tokenID
	return s.getRefreshTokenBykey(ctx, key)
}

func (s *JWTService) getRefreshTokenBykey(ctx context.Context, key string) (*RefreshTokenData, error) {
	data, err := s.redis.Get(ctx, key).Result()
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

func (s *JWTService) revokeRefreshToken(ctx context.Context, tokenID string) error {
	key := RefreshTokenKeyPrefix + tokenID
	tokenData, err := s.getRefreshToken(ctx, tokenID)
	if err != nil {
		return err
	}

	tokenData.IsRevoked = true
	// Сериализуем обновленные данные
	data, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal revoked token data: %w", err)
	}

	return s.redis.Set(ctx, key, data, time.Until(tokenData.ExpiresAt)).Err()
}

func generateTokenID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}
