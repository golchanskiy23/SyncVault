package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

// AuthMiddleware создает middleware для JWT аутентификации
func (s *JWTService) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Извлекаем токен из Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Проверяем формат Bearer token
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		tokenString := tokenParts[1]

		// Валидируем access token
		claims, err := s.ValidateToken(tokenString, AccessTokenType)
		if err != nil {
			http.Error(w, "Invalid token: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Добавляем claims в context
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, EmailKey, claims.Email)
		ctx = context.WithValue(ctx, RoleKey, claims.Role)
		ctx = context.WithValue(ctx, TokenIDKey, claims.TokenID)
		ctx = context.WithValue(ctx, ClaimsKey, claims)

		// Продолжаем обработку с обновленным context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OptionalAuthMiddleware создает middleware для опциональной аутентификации
// Если токен предоставлен - проверяет его, если нет - продолжает без аутентификации
func (s *JWTService) OptionalAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Пробуем извлечь токен
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			tokenParts := strings.Split(authHeader, " ")
			if len(tokenParts) == 2 && tokenParts[0] == "Bearer" {
				tokenString := tokenParts[1]
				
				if claims, err := s.ValidateToken(tokenString, AccessTokenType); err == nil {
					// Токен валиден - добавляем данные в context
					ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
					ctx = context.WithValue(ctx, EmailKey, claims.Email)
					ctx = context.WithValue(ctx, RoleKey, claims.Role)
					ctx = context.WithValue(ctx, TokenIDKey, claims.TokenID)
					ctx = context.WithValue(ctx, ClaimsKey, claims)
				}
				// Если токен невалиден - просто игнорируем, продолжаем без аутентификации
			}
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRoleMiddleware создает middleware для проверки роли пользователя
func RequireRoleMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := r.Context().Value(RoleKey).(string)
			if !ok {
				http.Error(w, "User role not found in context", http.StatusUnauthorized)
				return
			}

			if role != requiredRole {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAdminMiddleware требует права администратора
func RequireAdminMiddleware(next http.Handler) http.Handler {
	return RequireRoleMiddleware("admin")(next)
}

// RequireUserMiddleware требует права обычного пользователя
func RequireUserMiddleware(next http.Handler) http.Handler {
	return RequireRoleMiddleware("user")(next)
}

// GetUserIDFromContext извлекает ID пользователя из context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok
}

// GetEmailFromContext извлекает email из context
func GetEmailFromContext(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(EmailKey).(string)
	return email, ok
}

// GetRoleFromContext извлекает роль из context
func GetRoleFromContext(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(RoleKey).(string)
	return role, ok
}

// GetClaimsFromContext извлекает все claims из context
func GetClaimsFromContext(ctx context.Context) (*JWTClaims, bool) {
	claims, ok := ctx.Value(ClaimsKey).(*JWTClaims)
	return claims, ok
}

// GetTokenIDFromContext извлекает ID токена из context
func GetTokenIDFromContext(ctx context.Context) (string, bool) {
	tokenID, ok := ctx.Value(TokenIDKey).(string)
	return tokenID, ok
}

// RequestLoggerMiddleware расширенный логгер запросов с информацией о пользователе
func RequestLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Получаем информацию о пользователе для логирования
		if userID, ok := GetUserIDFromContext(r.Context()); ok {
			r.Header.Set("X-User-ID", userID)
		}
		if email, ok := GetEmailFromContext(r.Context()); ok {
			r.Header.Set("X-User-Email", email)
		}
		if role, ok := GetRoleFromContext(r.Context()); ok {
			r.Header.Set("X-User-Role", role)
		}

		// Используем стандартный middleware логгер
		middleware.Logger(next).ServeHTTP(w, r)
	})
}
