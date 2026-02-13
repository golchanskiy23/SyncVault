package google

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log"
	"time"

	"syncvault/internal/config"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

// OAuthService предоставляет сервис для работы с Google OAuth
type OAuthService struct {
	config *GoogleDriveConfig
	db     *pgxpool.Pool
	oauth2 *oauth2.Config
}

// NewOAuthService создает новый OAuth сервис
func NewOAuthService(config *config.GoogleDriveConfig, db *pgxpool.Pool) *OAuthService {
	// Конвертируем config
	googleConfig := &GoogleDriveConfig{
		OAuth: &GoogleOAuthConfig{
			ClientID:     config.OAuth.ClientID,
			ClientSecret: config.OAuth.ClientSecret,
			RedirectURL:  config.OAuth.RedirectURL,
			Scopes:       config.OAuth.Scopes,
		},
		APIBaseURL:  config.APIBaseURL,
		UploadURL:   config.UploadURL,
		MaxFileSize: config.MaxFileSize,
		ChunkSize:   config.ChunkSize,
		RetryCount:  config.RetryCount,
		RetryDelay:  config.RetryDelay,
	}

	return &OAuthService{
		config: googleConfig,
		db:     db,
		oauth2: googleConfig.OAuth.ToOAuth2Config(),
	}
}

// GeneratePKCE генерирует PKCE verifier и challenge
func (s *OAuthService) GeneratePKCE() (*PKCEData, error) {
	// Генерируем code verifier (43-128 символов)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}

	codeVerifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(b)

	// Генерируем state
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(stateBytes)

	return &PKCEData{
		CodeVerifier: codeVerifier,
		State:        state,
		CreatedAt:    time.Now(),
	}, nil
}

// SaveState сохраняет состояние OAuth flow
func (s *OAuthService) SaveState(ctx context.Context, userID, state, codeVerifier string) error {
	query := `
		INSERT INTO oauth_states (state, user_id, code_verifier, provider, created_at, expires_at, is_used)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.db.Exec(ctx, query, state, userID, codeVerifier, ProviderGoogle, time.Now(), time.Now().Add(StateTTL), false)
	if err != nil {
		return fmt.Errorf("failed to save oauth state: %w", err)
	}

	return nil
}

// GetState получает состояние OAuth flow
func (s *OAuthService) GetState(ctx context.Context, state string) (*OAuthState, error) {
	var oauthState OAuthState
	query := `
		SELECT state, user_id, code_verifier, provider, created_at, expires_at, is_used
		FROM oauth_states
		WHERE state = $1 AND provider = $2
	`

	err := s.db.QueryRow(ctx, query, state, ProviderGoogle).Scan(
		&oauthState.State,
		&oauthState.UserID,
		&oauthState.CodeVerifier,
		&oauthState.Provider,
		&oauthState.CreatedAt,
		&oauthState.ExpiresAt,
		&oauthState.IsUsed,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get oauth state: %w", err)
	}

	return &oauthState, nil
}

// MarkStateAsUsed помечает состояние как использованное
func (s *OAuthService) MarkStateAsUsed(ctx context.Context, state string) error {
	query := `UPDATE oauth_states SET is_used = true WHERE state = $1`
	_, err := s.db.Exec(ctx, query, state)
	if err != nil {
		return fmt.Errorf("failed to mark state as used: %w", err)
	}
	return nil
}

// GetAuthURL генерирует URL для авторизации
func (s *OAuthService) GetAuthURL(ctx context.Context, userID string) (string, *PKCEData, error) {
	// Генерируем PKCE
	pkce, err := s.GeneratePKCE()
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate PKCE: %w", err)
	}

	// Сохраняем состояние
	err = s.SaveState(ctx, userID, pkce.State, pkce.CodeVerifier)
	if err != nil {
		return "", nil, fmt.Errorf("failed to save state: %w", err)
	}

	// Генерируем auth URL с PKCE
	_ = s.generateCodeChallenge(pkce.CodeVerifier) // для совместимости с PKCE
	authURL := s.oauth2.AuthCodeURL(pkce.State,
		oauth2.SetAuthURLParam("code_challenge", s.generateCodeChallenge(pkce.CodeVerifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)

	return authURL, pkce, nil
}

// ExchangeCode обменивает authorization code на токен
func (s *OAuthService) ExchangeCode(ctx context.Context, state, code string) (*oauth2.Token, error) {
	// Получаем сохраненное состояние
	oauthState, err := s.GetState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("invalid state: %w", err)
	}

	// Проверяем что состояние не использовано и не истекло
	if oauthState.IsUsed {
		return nil, fmt.Errorf("state already used")
	}

	if time.Now().After(oauthState.ExpiresAt) {
		return nil, fmt.Errorf("state expired")
	}

	// Помечаем состояние как использованное
	err = s.MarkStateAsUsed(ctx, state)
	if err != nil {
		log.Printf("Warning: failed to mark state as used: %v", err)
	}

	// Обмениваем code на токен с code verifier
	token, err := s.oauth2.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", oauthState.CodeVerifier),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	return token, nil
}

// SaveToken сохраняет OAuth токен пользователя
func (s *OAuthService) SaveToken(ctx context.Context, userID string, token *oauth2.Token) error {
	// Удаляем существующие токены для этого пользователя и провайдера
	err := s.RevokeTokens(ctx, userID, ProviderGoogle)
	if err != nil {
		log.Printf("Warning: failed to revoke existing tokens: %v", err)
	}

	// Создаем новую запись токена
	oauthToken := FromOAuth2Token(userID, ProviderGoogle, token)

	query := `
		INSERT INTO oauth_tokens (user_id, provider, access_token, refresh_token, token_type, expiry, scope, created_at, updated_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`

	err = s.db.QueryRow(ctx, query,
		oauthToken.UserID,
		oauthToken.Provider,
		oauthToken.AccessToken,
		oauthToken.RefreshToken,
		oauthToken.TokenType,
		oauthToken.Expiry,
		oauthToken.Scope,
		oauthToken.CreatedAt,
		oauthToken.UpdatedAt,
		oauthToken.IsActive,
	).Scan(&oauthToken.ID)

	if err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	return nil
}

// GetToken получает активный токен пользователя
func (s *OAuthService) GetToken(ctx context.Context, userID string) (*OAuthToken, error) {
	var token OAuthToken
	query := `
		SELECT id, user_id, provider, access_token, refresh_token, token_type, expiry, scope, created_at, updated_at, is_active
		FROM oauth_tokens
		WHERE user_id = $1 AND provider = $2 AND is_active = true
		ORDER BY created_at DESC
		LIMIT 1
	`

	err := s.db.QueryRow(ctx, query, userID, ProviderGoogle).Scan(
		&token.ID,
		&token.UserID,
		&token.Provider,
		&token.AccessToken,
		&token.RefreshToken,
		&token.TokenType,
		&token.Expiry,
		&token.Scope,
		&token.CreatedAt,
		&token.UpdatedAt,
		&token.IsActive,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no active token found for user %s", userID)
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return &token, nil
}

// RefreshToken обновляет токен
func (s *OAuthService) RefreshToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	// Получаем текущий токен
	currentToken, err := s.GetToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current token: %w", err)
	}

	// Проверяем наличие refresh token
	if currentToken.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	// Создаем oauth2.Token из сохраненного
	token := currentToken.ToOAuth2Token()

	// Обновляем токен
	tokenSource := s.oauth2.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}

	// Сохраняем новый токен
	err = s.SaveToken(ctx, userID, newToken)
	if err != nil {
		return nil, fmt.Errorf("failed to save new token: %w", err)
	}

	return newToken, nil
}

// GetValidToken получает валидный токен (с автоматическим обновлением)
func (s *OAuthService) GetValidToken(ctx context.Context, userID string) (*oauth2.Token, error) {
	// Получаем токен
	token, err := s.GetToken(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	oauth2Token := token.ToOAuth2Token()

	// Проверяем нужно ли обновить токен
	if oauth2Token.Expiry.Before(time.Now().Add(RefreshThreshold)) {
		log.Printf("Token expires soon, refreshing for user %s", userID)
		newToken, err := s.RefreshToken(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		return newToken, nil
	}

	return oauth2Token, nil
}

// RevokeTokens отзывает все токены пользователя
func (s *OAuthService) RevokeTokens(ctx context.Context, userID, provider string) error {
	query := `UPDATE oauth_tokens SET is_active = false, updated_at = $1 WHERE user_id = $2 AND provider = $3`
	_, err := s.db.Exec(ctx, query, time.Now(), userID, provider)
	if err != nil {
		return fmt.Errorf("failed to revoke tokens: %w", err)
	}
	return nil
}

// DeleteToken удаляет токен
func (s *OAuthService) DeleteToken(ctx context.Context, tokenID int64) error {
	query := `DELETE FROM oauth_tokens WHERE id = $1`
	_, err := s.db.Exec(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}

// CleanupExpiredStates очищает истекшие состояния
func (s *OAuthService) CleanupExpiredStates(ctx context.Context) error {
	query := `DELETE FROM oauth_states WHERE expires_at < $1`
	_, err := s.db.Exec(ctx, query, time.Now())
	if err != nil {
		return fmt.Errorf("failed to cleanup expired states: %w", err)
	}
	return nil
}

// generateCodeChallenge генерирует code challenge из verifier
func (s *OAuthService) generateCodeChallenge(codeVerifier string) string {
	hash := sha256.Sum256([]byte(codeVerifier))
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(hash[:])
}

// ValidateToken проверяет валидность токена
func (s *OAuthService) ValidateToken(ctx context.Context, token *oauth2.Token) error {
	// Проверяем expiry
	if !token.Valid() {
		return fmt.Errorf("token is expired")
	}

	// Можно добавить дополнительную валидацию через Google API
	// Например, проверку userinfo endpoint

	return nil
}
