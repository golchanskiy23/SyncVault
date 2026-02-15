package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJWTService тестирует основной JWT сервис
func TestJWTService(t *testing.T) {
	// Настройка тестового Redis
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	// Конфигурация JWT
	config := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars",
		RefreshSecret: "test-refresh-secret-key-32-chars",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "test-issuer",
	}

	jwtService := NewJWTService(config, redisClient)

	t.Run("GenerateTokenPair", func(t *testing.T) {
		ctx := context.Background()

		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)
		assert.NotEmpty(t, tokenPair.AccessToken)
		assert.NotEmpty(t, tokenPair.RefreshToken)
		assert.Equal(t, "Bearer", tokenPair.TokenType)
		assert.Greater(t, tokenPair.ExpiresIn, int64(0))
	})

	t.Run("ValidateAccessToken", func(t *testing.T) {
		ctx := context.Background()

		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		claims, err := jwtService.ValidateToken(tokenPair.AccessToken, AccessTokenType)
		require.NoError(t, err)
		assert.Equal(t, "user123", claims.UserID)
		assert.Equal(t, "test@example.com", claims.Email)
		assert.Equal(t, "user", claims.Role)
		assert.Equal(t, string(AccessTokenType), claims.TokenType)
	})

	t.Run("ValidateRefreshToken", func(t *testing.T) {
		ctx := context.Background()

		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		claims, err := jwtService.ValidateToken(tokenPair.RefreshToken, RefreshTokenType)
		require.NoError(t, err)
		assert.Equal(t, "user123", claims.UserID)
		assert.Equal(t, "test@example.com", claims.Email)
		assert.Equal(t, "user", claims.Role)
		assert.Equal(t, string(RefreshTokenType), claims.TokenType)
	})

	t.Run("RefreshTokenRotation", func(t *testing.T) {
		ctx := context.Background()

		// Генерируем первую пару токенов
		tokenPair1, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		// Ротируем токены
		tokenPair2, err := jwtService.RefreshTokens(ctx, tokenPair1.RefreshToken)
		require.NoError(t, err)

		// Новые токены должны быть разными
		assert.NotEqual(t, tokenPair1.AccessToken, tokenPair2.AccessToken)
		assert.NotEqual(t, tokenPair1.RefreshToken, tokenPair2.RefreshToken)

		// Старый refresh токен должен быть недействительным
		_, err = jwtService.RefreshTokens(ctx, tokenPair1.RefreshToken)
		assert.Error(t, err) // Должна быть ошибка, т.к. токен отозван

		// Новый refresh токен должен работать
		tokenPair3, err := jwtService.RefreshTokens(ctx, tokenPair2.RefreshToken)
		require.NoError(t, err)
		assert.NotEmpty(t, tokenPair3.AccessToken)
		assert.NotEmpty(t, tokenPair3.RefreshToken)
	})

	t.Run("TokenTheftDetection", func(t *testing.T) {
		ctx := context.Background()

		// Генерируем токены
		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		// Ротируем токены
		_, err = jwtService.RefreshTokens(ctx, tokenPair.RefreshToken)
		require.NoError(t, err)

		// Пытаемся использовать старый refresh токен (симуляция кражи)
		_, err = jwtService.RefreshTokens(ctx, tokenPair.RefreshToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "revoked")
	})

	t.Run("Logout", func(t *testing.T) {
		ctx := context.Background()

		// Генерируем несколько токенов для пользователя
		tokenPair1, err := jwtService.GenerateTokenPair(ctx, "user456", "logout@test.com", "user")
		require.NoError(t, err)

		tokenPair2, err := jwtService.GenerateTokenPair(ctx, "user456", "logout@test.com", "user")
		require.NoError(t, err)

		// Выполняем logout
		err = jwtService.Logout(ctx, "user456")
		require.NoError(t, err)

		// Оба refresh токена должны быть недействительными
		_, err = jwtService.RefreshTokens(ctx, tokenPair1.RefreshToken)
		assert.Error(t, err)

		_, err = jwtService.RefreshTokens(ctx, tokenPair2.RefreshToken)
		assert.Error(t, err)
	})
}

// TestAuthMiddleware тестирует middleware
func TestAuthMiddleware(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	config := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars",
		RefreshSecret: "test-refresh-secret-key-32-chars",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "test-issuer",
	}

	jwtService := NewJWTService(config, redisClient)

	t.Run("ValidToken", func(t *testing.T) {
		ctx := context.Background()

		// Генерируем токен
		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		// Создаем тестовый запрос с токеном
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+tokenPair.AccessToken)
		req = req.WithContext(ctx)

		// Создаем middleware
		middleware := jwtService.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Проверяем, что данные добавлены в context
			userID, ok := GetUserIDFromContext(r.Context())
			assert.True(t, ok)
			assert.Equal(t, "user123", userID)

			email, ok := GetEmailFromContext(r.Context())
			assert.True(t, ok)
			assert.Equal(t, "test@example.com", email)

			role, ok := GetRoleFromContext(r.Context())
			assert.True(t, ok)
			assert.Equal(t, "user", role)
		}))

		// Выполняем middleware
		middleware.ServeHTTP(&httptest.ResponseRecorder{}, req)
	})

	t.Run("MissingToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req = req.WithContext(context.Background())

		middleware := jwtService.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("Handler should not be called")
		}))

		recorder := &httptest.ResponseRecorder{}
		middleware.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("InvalidToken", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		req = req.WithContext(context.Background())

		middleware := jwtService.AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("Handler should not be called")
		}))

		recorder := &httptest.ResponseRecorder{}
		middleware.ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code)
	})
}

// TestRedisTokenRepository тестирует работу с Redis
func TestRedisTokenRepository(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	config := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars",
		RefreshSecret: "test-refresh-secret-key-32-chars",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "test-issuer",
	}

	repo := NewRedisTokenRepository(redisClient, config)

	t.Run("StoreAndGetRefreshToken", func(t *testing.T) {
		ctx := context.Background()

		tokenData := &RefreshTokenData{
			UserID:    "user123",
			TokenID:   "token123",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			IsRevoked: false,
		}

		err := repo.StoreRefreshToken(ctx, tokenData)
		require.NoError(t, err)

		retrieved, err := repo.GetRefreshToken(ctx, "token123")
		require.NoError(t, err)
		assert.Equal(t, tokenData.UserID, retrieved.UserID)
		assert.Equal(t, tokenData.TokenID, retrieved.TokenID)
		assert.False(t, retrieved.IsRevoked)
	})

	t.Run("RevokeRefreshToken", func(t *testing.T) {
		ctx := context.Background()

		tokenData := &RefreshTokenData{
			UserID:    "user123",
			TokenID:   "token456",
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(24 * time.Hour),
			IsRevoked: false,
		}

		err := repo.StoreRefreshToken(ctx, tokenData)
		require.NoError(t, err)

		err = repo.RevokeRefreshToken(ctx, "token456")
		require.NoError(t, err)

		retrieved, err := repo.GetRefreshToken(ctx, "token456")
		require.NoError(t, err)
		assert.True(t, retrieved.IsRevoked)
	})
}

// TestAuthHandlers тестирует HTTP обработчики
func TestAuthHandlers(t *testing.T) {
	s, err := miniredis.Run()
	require.NoError(t, err)
	defer s.Close()

	redisClient := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	config := &JWTConfig{
		AccessSecret:  "test-access-secret-key-32-chars",
		RefreshSecret: "test-refresh-secret-key-32-chars",
		AccessTTL:     15 * time.Minute,
		RefreshTTL:    30 * 24 * time.Hour,
		Issuer:        "test-issuer",
	}

	jwtService := NewJWTService(config, redisClient)
	userService := NewTestUserService()
	authHandler := NewAuthHandler(jwtService, userService)

	t.Run("Login", func(t *testing.T) {
		// Создаем тестовый запрос
		body := strings.NewReader(`{"email":"test@example.com","password":"password"}`)
		req := httptest.NewRequest("POST", "/api/v1/auth/login", body)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		authHandler.Login(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response AuthResponse
		err := json.NewDecoder(recorder.Body).Decode(&response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.AccessToken)
		assert.NotEmpty(t, response.RefreshToken)
		assert.Equal(t, "test@example.com", response.User.Email)
	})

	t.Run("Refresh", func(t *testing.T) {
		// Сначала логинимся для получения токенов
		ctx := context.Background()
		tokenPair, err := jwtService.GenerateTokenPair(ctx, "user123", "test@example.com", "user")
		require.NoError(t, err)

		// Создаем запрос на обновление
		body := strings.NewReader(fmt.Sprintf(`{"refresh_token":"%s"}`, tokenPair.RefreshToken))
		req := httptest.NewRequest("POST", "/api/v1/auth/refresh", body)
		req.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		authHandler.Refresh(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]interface{}
		err = json.NewDecoder(recorder.Body).Decode(&response)
		require.NoError(t, err)

		tokens, ok := response["tokens"].(map[string]interface{})
		require.True(t, ok)
		assert.NotEmpty(t, tokens["access_token"])
		assert.NotEmpty(t, tokens["refresh_token"])
	})
}

// TestUserService тестовая реализация UserService
type TestUserService struct {
	users map[string]*UserInfo
}

func NewTestUserService() *TestUserService {
	users := make(map[string]*UserInfo)

	hashedPassword, _ := HashPassword("password")
	users["test@example.com"] = &UserInfo{
		ID:           "user123",
		Email:        "test@example.com",
		Role:         "user",
		PasswordHash: hashedPassword,
	}

	return &TestUserService{users: users}
}

func (s *TestUserService) GetUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	user, exists := s.users[email]
	if !exists {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *TestUserService) VerifyPassword(hashedPassword, password string) bool {
	return CheckPassword(hashedPassword, password)
}
