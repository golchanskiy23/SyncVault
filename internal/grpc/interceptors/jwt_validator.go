package interceptors

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWTValidatorImpl реализация JWT валидатора
type JWTValidatorImpl struct {
	secretKey     []byte
	issuer        string
	tokenExpiry   time.Duration
	refreshExpiry time.Duration
}

// NewJWTValidator создает новый JWT валидатор
func NewJWTValidator(secretKey string, issuer string, tokenExpiry, refreshExpiry time.Duration) *JWTValidatorImpl {
	return &JWTValidatorImpl{
		secretKey:     []byte(secretKey),
		issuer:        issuer,
		tokenExpiry:   tokenExpiry,
		refreshExpiry: refreshExpiry,
	}
}

// GenerateToken генерирует новый JWT токен формата header.payload.signature
func (j *JWTValidatorImpl) GenerateToken(claims *Claims, expiry time.Duration) (string, error) {
	now := time.Now()

	// Header
	header := map[string]string{
		"alg": "HS256",
		"typ": "JWT",
	}
	headerBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header: %w", err)
	}
	encodedHeader := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Payload
	payload := map[string]interface{}{
		"user_id":   claims.UserID,
		"node_id":   claims.NodeID,
		"email":     claims.Email,
		"roles":     claims.Roles,
		"is_active": claims.IsActive,
		"iss":       j.issuer,
		"iat":       now.Unix(),
		"exp":       now.Add(expiry).Unix(),
		"jti":       generateJTI(),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Signature подписываем header.payload
	signingInput := encodedHeader + "." + encodedPayload
	signature := j.createSignature(signingInput)
	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)

	return encodedHeader + "." + encodedPayload + "." + encodedSignature, nil
}

// ValidateToken валидирует JWT токен формата header.payload.signature
func (j *JWTValidatorImpl) ValidateToken(tokenString string) (*Claims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format: expected 3 parts, got %d", len(parts))
	}

	// Проверяем подпись
	signingInput := parts[0] + "." + parts[1]
	expectedSig := j.createSignature(signingInput)
	expectedEncoded := base64.RawURLEncoding.EncodeToString(expectedSig)
	if !hmac.Equal([]byte(parts[2]), []byte(expectedEncoded)) {
		return nil, fmt.Errorf("invalid token signature")
	}

	// Декодируем payload (parts[1])
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode token payload: %w", err)
	}

	var tokenData map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &tokenData); err != nil {
		return nil, fmt.Errorf("failed to parse token payload: %w", err)
	}

	// Проверяем срок действия
	if exp, ok := tokenData["exp"].(float64); ok {
		if time.Now().Unix() > int64(exp) {
			return nil, fmt.Errorf("token expired")
		}
	}

	// Проверяем issuer
	if iss, ok := tokenData["iss"].(string); ok && iss != j.issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", j.issuer, iss)
	}

	// Проверяем is_active
	if isActive, ok := tokenData["is_active"].(bool); ok && !isActive {
		return nil, fmt.Errorf("token is not active")
	}

	// Извлекаем claims
	claims := &Claims{}

	if v, ok := tokenData["user_id"].(string); ok {
		claims.UserID = v
	}
	if v, ok := tokenData["node_id"].(string); ok {
		claims.NodeID = v
	}
	if v, ok := tokenData["email"].(string); ok {
		claims.Email = v
	}
	if v, ok := tokenData["is_active"].(bool); ok {
		claims.IsActive = v
	}
	if roles, ok := tokenData["roles"].([]interface{}); ok {
		claims.Roles = make([]string, len(roles))
		for i, role := range roles {
			if r, ok := role.(string); ok {
				claims.Roles[i] = r
			}
		}
	}

	return claims, nil
}

// GenerateTokenPair генерирует access и refresh токены
func (j *JWTValidatorImpl) GenerateTokenPair(claims *Claims) (*TokenPair, error) {
	accessToken, err := j.GenerateToken(claims, j.tokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err := j.GenerateToken(claims, j.refreshExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresIn:        int64(j.tokenExpiry.Seconds()),
		RefreshExpiresIn: int64(j.refreshExpiry.Seconds()),
	}, nil
}

// RefreshToken обновляет токен используя refresh token
func (j *JWTValidatorImpl) RefreshToken(refreshTokenString string) (*TokenPair, error) {
	claims, err := j.ValidateToken(refreshTokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}
	return j.GenerateTokenPair(claims)
}

// createSignature создает HMAC-SHA256 подпись
func (j *JWTValidatorImpl) createSignature(input string) []byte {
	h := hmac.New(sha256.New, j.secretKey)
	h.Write([]byte(input))
	return h.Sum(nil)
}

// TokenPair представляет пару access и refresh токенов
type TokenPair struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshExpiresIn int64  `json:"refresh_expires_in"`
}

// generateJTI генерирует уникальный ID для токена
func generateJTI() string {
	h := hmac.New(sha256.New, []byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	h.Write([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
	return base64.URLEncoding.EncodeToString(h.Sum(nil))[:16]
}

// ValidateSecretKey проверяет валидность секретного ключа
func ValidateSecretKey(secretKey string) error {
	if len(secretKey) < 32 {
		return fmt.Errorf("secret key must be at least 32 characters long")
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, char := range secretKey {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		case strings.ContainsRune("!@#$%^&*()_+-=[]{}|;:,.<>?", char):
			hasSpecial = true
		}
	}

	if !hasUpper || !hasLower || !hasDigit || !hasSpecial {
		return fmt.Errorf("secret key must contain uppercase, lowercase, digits, and special characters")
	}

	return nil
}

// ExtractTokenFromBearerString извлекает токен из Bearer строки
func ExtractTokenFromBearerString(bearerString string) string {
	if strings.HasPrefix(strings.ToLower(bearerString), "bearer ") {
		return strings.TrimSpace(bearerString[7:])
	}
	return strings.TrimSpace(bearerString)
}

// MockJWTValidator мок JWT валидатора для тестов
type MockJWTValidator struct {
	validTokens map[string]*Claims
}

// NewMockJWTValidator создает мок JWT валидатор
func NewMockJWTValidator() *MockJWTValidator {
	return &MockJWTValidator{
		validTokens: make(map[string]*Claims),
	}
}

// AddValidToken добавляет валидный токен в мок
func (m *MockJWTValidator) AddValidToken(token string, claims *Claims) {
	m.validTokens[token] = claims
}

// ValidateToken валидирует токен в моке
func (m *MockJWTValidator) ValidateToken(token string) (*Claims, error) {
	if claims, ok := m.validTokens[token]; ok {
		return claims, nil
	}
	return nil, fmt.Errorf("invalid token")
}
