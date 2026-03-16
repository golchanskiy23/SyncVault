package google

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// GoogleOAuthConfig содержит конфигурацию Google OAuth
type GoogleOAuthConfig struct {
	ClientID     string `yaml:"client_id" json:"client_id"`
	ClientSecret string `yaml:"client_secret" json:"client_secret"`
	RedirectURL  string `yaml:"redirect_url" json:"redirect_url"`
	Scopes       []string `yaml:"scopes" json:"scopes"`
}

// DefaultGoogleOAuthConfig возвращает конфигурацию по умолчанию
func DefaultGoogleOAuthConfig() *GoogleOAuthConfig {
	return &GoogleOAuthConfig{
		RedirectURL: "http://localhost:8080/auth/google/callback",
		Scopes: []string{
			"https://www.googleapis.com/auth/drive.readonly",
			"https://www.googleapis.com/auth/drive.file",
			"https://www.googleapis.com/auth/drive.metadata.readonly",
		},
	}
}

// ToOAuth2Config конвертирует в oauth2.Config
func (c *GoogleOAuthConfig) ToOAuth2Config() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     c.ClientID,
		ClientSecret: c.ClientSecret,
		RedirectURL:  c.RedirectURL,
		Scopes:       c.Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:  "https://oauth2.googleapis.com/token",
		},
	}
}

// OAuthToken представляет OAuth токен пользователя
type OAuthToken struct {
	ID           int64     `json:"id" db:"id"`
	UserID       string    `json:"user_id" db:"user_id"`
	Provider     string    `json:"provider" db:"provider"` // google
	AccessToken  string    `json:"access_token" db:"access_token"`
	RefreshToken string    `json:"refresh_token" db:"refresh_token"`
	TokenType    string    `json:"token_type" db:"token_type"`
	Expiry       time.Time `json:"expiry" db:"expiry"`
	Scope        string    `json:"scope" db:"scope"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
	IsActive     bool      `json:"is_active" db:"is_active"`
}

// Value реализует driver.Valuer для PostgreSQL
func (t OAuthToken) Value() (driver.Value, error) {
	return json.Marshal(t)
}

// Scan реализует sql.Scanner для PostgreSQL
func (t *OAuthToken) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, t)
	case string:
		return json.Unmarshal([]byte(v), t)
	default:
		return fmt.Errorf("cannot scan %T into OAuthToken", value)
	}
}

// ToOAuth2Token конвертирует в oauth2.Token
func (t *OAuthToken) ToOAuth2Token() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.Expiry,
	}
}

// FromOAuth2Token создает OAuthToken из oauth2.Token
func FromOAuth2Token(userID, provider string, token *oauth2.Token) *OAuthToken {
	return &OAuthToken{
		UserID:       userID,
		Provider:     provider,
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Expiry:       token.Expiry,
		Scope:        "drive.readonly drive.file drive.metadata.readonly",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		IsActive:     true,
	}
}

// PKCEData содержит данные для PKCE flow
type PKCEData struct {
	CodeVerifier string `json:"code_verifier"`
	State        string `json:"state"`
	CreatedAt    time.Time `json:"created_at"`
}

// OAuthState содержит состояние OAuth flow
type OAuthState struct {
	State        string    `json:"state" db:"state"`
	UserID       string    `json:"user_id" db:"user_id"`
	CodeVerifier string    `json:"code_verifier" db:"code_verifier"`
	Provider     string    `json:"provider" db:"provider"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	ExpiresAt    time.Time `json:"expires_at" db:"expires_at"`
	IsUsed       bool      `json:"is_used" db:"is_used"`
}

// Value реализует driver.Valuer для PostgreSQL
func (s OAuthState) Value() (driver.Value, error) {
	return json.Marshal(s)
}

// Scan реализует sql.Scanner для PostgreSQL
func (s *OAuthState) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("cannot scan %T into OAuthState", value)
	}
}

// GoogleDriveFile представляет файл в Google Drive
type GoogleDriveFile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	MimeType    string    `json:"mimeType"`
	Size        int64     `json:"size"`
	CreatedTime time.Time `json:"createdTime"`
	ModifiedTime time.Time `json:"modifiedTime"`
	Parents     []string  `json:"parents"`
	WebViewLink string    `json:"webViewLink"`
	DownloadURL string    `json:"downloadUrl,omitempty"`
}

// GoogleDriveFolder представляет папку в Google Drive
type GoogleDriveFolder struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CreatedTime time.Time `json:"createdTime"`
	ModifiedTime time.Time `json:"modifiedTime"`
	Parents     []string  `json:"parents"`
	WebViewLink string    `json:"webViewLink"`
}

// GoogleDriveResponse представляет ответ от Google Drive API
type GoogleDriveResponse struct {
	Files      []GoogleDriveFile `json:"files"`
	NextPageToken string         `json:"nextPageToken,omitempty"`
}

// Constants
const (
	ProviderGoogle = "google"
	
	// Google Drive scopes
	ScopeDriveReadonly           = "https://www.googleapis.com/auth/drive.readonly"
	ScopeDriveFile              = "https://www.googleapis.com/auth/drive.file"
	ScopeDriveMetadataReadonly  = "https://www.googleapis.com/auth/drive.metadata.readonly"
	
	// OAuth flow timeouts
	StateTTL = 10 * time.Minute
	
	// Token refresh thresholds
	RefreshThreshold = 5 * time.Minute
)

// GoogleDriveConfig содержит конфигурацию Google Drive
type GoogleDriveConfig struct {
	OAuth        *GoogleOAuthConfig `yaml:"oauth" json:"oauth"`
	APIBaseURL   string             `yaml:"api_base_url" json:"api_base_url"`
	UploadURL    string             `yaml:"upload_url" json:"upload_url"`
	MaxFileSize  int64              `yaml:"max_file_size" json:"max_file_size"`
	ChunkSize    int                `yaml:"chunk_size" json:"chunk_size"`
	RetryCount   int                `yaml:"retry_count" json:"retry_count"`
	RetryDelay   time.Duration      `yaml:"retry_delay" json:"retry_delay"`
}

// DefaultGoogleDriveConfig возвращает конфигурацию по умолчанию
func DefaultGoogleDriveConfig() *GoogleDriveConfig {
	return &GoogleDriveConfig{
		OAuth:       DefaultGoogleOAuthConfig(),
		APIBaseURL:  "https://www.googleapis.com/drive/v3",
		UploadURL:   "https://www.googleapis.com/upload/drive/v3",
		MaxFileSize: 100 * 1024 * 1024, // 100MB
		ChunkSize:   1024 * 1024,        // 1MB
		RetryCount:  3,
		RetryDelay:  time.Second,
	}
}

// Validate проверяет конфигурацию
func (c *GoogleDriveConfig) Validate() error {
	if c.OAuth == nil {
		return fmt.Errorf("oauth config is required")
	}
	
	if c.OAuth.ClientID == "" {
		return fmt.Errorf("oauth client_id is required")
	}
	
	if c.OAuth.ClientSecret == "" {
		return fmt.Errorf("oauth client_secret is required")
	}
	
	if len(c.OAuth.Scopes) == 0 {
		return fmt.Errorf("oauth scopes are required")
	}
	
	return nil
}
