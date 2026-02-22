package interceptors

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthInterceptor выполняет аутентификацию через JWT токен в metadata
type AuthInterceptor struct {
	jwtValidator JWTValidator
	publicMethods map[string]bool
}


// JWTValidator интерфейс для валидации JWT токенов
type JWTValidator interface {
	ValidateToken(token string) (*Claims, error)
}

// Claims представляет claims из JWT токена
type Claims struct {
	UserID   string   `json:"user_id"`
	NodeID   string   `json:"node_id"`
	Roles    []string `json:"roles"`
	Email    string   `json:"email"`
	IsActive bool     `json:"is_active"`
}

// NewAuthInterceptor создает новый интерцептор для аутентификации
func NewAuthInterceptor(jwtValidator JWTValidator, publicMethods ...string) *AuthInterceptor {
	public := make(map[string]bool)
	for _, method := range publicMethods {
		public[method] = true
	}
	
	return &AuthInterceptor{
		jwtValidator: jwtValidator,
		publicMethods: public,
	}
}

// UnaryInterceptor возвращает unary interceptor для аутентификации
func (ai *AuthInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		// Проверяем, является ли метод публичным
		if ai.isPublicMethod(info.FullMethod) {
			return handler(ctx, req)
		}

		// Извлекаем токен из metadata
		token, err := ai.extractTokenFromMetadata(ctx)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Валидируем токен
		claims, err := ai.jwtValidator.ValidateToken(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Добавляем claims в контекст
		newCtx := ai.setClaimsToContext(ctx, claims)
		
		return handler(newCtx, req)
	}
}

// StreamInterceptor возвращает stream interceptor для аутентификации
func (ai *AuthInterceptor) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		// Проверяем, является ли метод публичным
		if ai.isPublicMethod(info.FullMethod) {
			return handler(srv, ss)
		}

		// Извлекаем токен из metadata
		token, err := ai.extractTokenFromMetadata(ss.Context())
		if err != nil {
			return status.Errorf(codes.Unauthenticated, "authentication failed: %v", err)
		}

		// Валидируем токен
		claims, err := ai.jwtValidator.ValidateToken(token)
		if err != nil {
			return status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}

		// Создаем новый стрим с обновленным контекстом
		wrappedStream := &authServerStream{
			ServerStream: ss,
			ctx:          ai.setClaimsToContext(ss.Context(), claims),
		}
		
		return handler(srv, wrappedStream)
	}
}

// isPublicMethod проверяет, является ли метод публичным
func (ai *AuthInterceptor) isPublicMethod(fullMethod string) bool {
	// Извлекаем имя метода из полного пути (например, "/syncvault.v1.health.HealthService/Check" -> "Check")
	parts := strings.Split(fullMethod, "/")
	if len(parts) > 0 {
		methodName := parts[len(parts)-1]
		return ai.publicMethods[methodName]
	}
	return false
}

// extractTokenFromMetadata извлекает JWT токен из metadata
func (ai *AuthInterceptor) extractTokenFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", errors.New("metadata not found")
	}

	// Ищем токен в различных заголовках
	tokenHeaders := []string{"authorization", "x-auth-token", "grpc-metadata-token"}
	
	for _, header := range tokenHeaders {
		values := md.Get(header)
		if len(values) > 0 {
			token := values[0]
			
			// Удаляем префикс "Bearer " если он есть
			if strings.HasPrefix(strings.ToLower(token), "bearer ") {
				return strings.TrimSpace(token[7:]), nil
			}
			
			return strings.TrimSpace(token), nil
		}
	}

	return "", errors.New("authorization token not found")
}

// setClaimsToContext добавляет claims в контекст
func (ai *AuthInterceptor) setClaimsToContext(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey{}, claims)
}

// GetClaimsFromContext извлекает claims из контекста
func GetClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*Claims)
	return claims, ok
}

// RequireRole проверяет наличие роли у пользователя
func RequireRole(ctx context.Context, requiredRole string) error {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "claims not found in context")
	}

	for _, role := range claims.Roles {
		if role == requiredRole {
			return nil
		}
	}

	return status.Errorf(codes.PermissionDenied, "required role %s not found", requiredRole)
}

// RequireUserID проверяет совпадение ID пользователя
func RequireUserID(ctx context.Context, requiredUserID string) error {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "claims not found in context")
	}

	if claims.UserID != requiredUserID {
		return status.Errorf(codes.PermissionDenied, "user ID mismatch: expected %s, got %s", requiredUserID, claims.UserID)
	}

	return nil
}

// RequireNodeID проверяет совпадение ID узла
func RequireNodeID(ctx context.Context, requiredNodeID string) error {
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "claims not found in context")
	}

	if claims.NodeID != requiredNodeID {
		return status.Errorf(codes.PermissionDenied, "node ID mismatch: expected %s, got %s", requiredNodeID, claims.NodeID)
	}

	return nil
}

// claimsContextKey используется для хранения claims в контексте
type claimsContextKey struct{}

// authServerStream обертка для grpc.ServerStream с обновленным контекстом
type authServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context возвращает обновленный контекст
func (w *authServerStream) Context() context.Context {
	return w.ctx
}

// AuthConfig представляет конфигурацию аутентификации
type AuthConfig struct {
	SecretKey       string   `json:"secret_key"`
	TokenExpiry     int64    `json:"token_expiry"`
	RefreshExpiry   int64    `json:"refresh_expiry"`
	Issuer          string   `json:"issuer"`
	PublicMethods    []string `json:"public_methods"`
	RequiredRoles    []string `json:"required_roles"`
	SkipAuthMethods []string `json:"skip_auth_methods"`
}

// DefaultAuthConfig возвращает конфигурацию по умолчанию
func DefaultAuthConfig() *AuthConfig {
	return &AuthConfig{
		TokenExpiry:  3600, // 1 час
		RefreshExpiry: 86400, // 24 часа
		Issuer:       "syncvault",
		PublicMethods: []string{
			"Check",
			"Watch", 
			"HealthCheck",
			"Live",
			"Ready",
			"Startup",
		},
	}
}
