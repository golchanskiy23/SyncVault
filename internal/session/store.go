package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	UserID       int64     `json:"user_id"`
	RefreshToken string    `json:"refresh_token"`
	CreatedAt    time.Time `json:"created_at"`
	ExpiresAt    time.Time `json:"expires_at"`
	LastUsed     time.Time `json:"last_used"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

type SessionStore struct {
	rdb           *redis.Client
	tokenExpiry   time.Duration
	maxSessions   int
	hashCost      int
	prefix        string
}

func NewSessionStore(rdb *redis.Client, tokenExpiry time.Duration, maxSessions int) *SessionStore {
	return &SessionStore{
		rdb:         rdb,
		tokenExpiry: tokenExpiry,
		maxSessions: maxSessions,
		hashCost:    bcrypt.DefaultCost,
		prefix:      "session:",
	}
}

func (s *SessionStore) generateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func (s *SessionStore) hashToken(token string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(token), s.hashCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash token: %w", err)
	}
	return string(hash), nil
}

func (s *SessionStore) verifyToken(hashedToken, token string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedToken), []byte(token))
}

func (s *SessionStore) userKey(userID int64) string {
	return fmt.Sprintf("%suser:%d", s.prefix, userID)
}

func (s *SessionStore) sessionKey(sessionID string) string {
	return fmt.Sprintf("%sid:%s", s.prefix, sessionID)
}

func (s *SessionStore) CreateSession(ctx context.Context, userID int64, metadata map[string]string) (*Session, error) {
	refreshToken, err := s.generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	hashedToken, err := s.hashToken(refreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to hash refresh token: %w", err)
	}

	now := time.Now()
	session := &Session{
		UserID:       userID,
		RefreshToken: hashedToken,
		CreatedAt:    now,
		ExpiresAt:    now.Add(s.tokenExpiry),
		LastUsed:     now,
		Metadata:     metadata,
	}

	// Generate session ID from token hash
	sessionID := fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))

	// Check user session limit and cleanup old sessions if needed
	if err := s.enforceSessionLimit(ctx, userID); err != nil {
		return nil, fmt.Errorf("failed to enforce session limit: %w", err)
	}

	// Store session data
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	pipe := s.rdb.TxPipeline()
	
	// Store session by ID
	pipe.Set(ctx, s.sessionKey(sessionID), sessionData, s.tokenExpiry)
	
	// Add to user's session list
	pipe.LPush(ctx, s.userKey(userID), sessionID)
	pipe.Expire(ctx, s.userKey(userID), s.tokenExpiry)
	
	// Trim session list to maxSessions
	pipe.LTrim(ctx, s.userKey(userID), 0, int64(s.maxSessions-1))

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("failed to store session: %w", err)
	}

	// Return session with unhashed token for client
	session.RefreshToken = refreshToken
	return session, nil
}

func (s *SessionStore) enforceSessionLimit(ctx context.Context, userID int64) error {
	sessions, err := s.rdb.LRange(ctx, s.userKey(userID), 0, -1).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	if len(sessions) >= s.maxSessions {
		// Remove oldest sessions
		toRemove := len(sessions) - s.maxSessions + 1
		for i := 0; i < toRemove; i++ {
			oldestSessionID, err := s.rdb.RPop(ctx, s.userKey(userID)).Result()
			if err != nil && err != redis.Nil {
				return fmt.Errorf("failed to remove old session: %w", err)
			}
			
			// Remove session data
			if oldestSessionID != "" {
				s.rdb.Del(ctx, s.sessionKey(oldestSessionID))
			}
		}
	}

	return nil
}

func (s *SessionStore) ValidateSession(ctx context.Context, sessionID, refreshToken string) (*Session, error) {
	sessionData, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	var session Session
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		s.DeleteSession(ctx, sessionID)
		return nil, fmt.Errorf("session expired")
	}

	// Verify refresh token
	if err := s.verifyToken(session.RefreshToken, refreshToken); err != nil {
		return nil, fmt.Errorf("invalid refresh token: %w", err)
	}

	// Update last used time
	session.LastUsed = time.Now()
	
	// Update session in Redis
	updatedData, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated session: %w", err)
	}

	if err := s.rdb.Set(ctx, s.sessionKey(sessionID), updatedData, s.tokenExpiry).Err(); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	return &session, nil
}

func (s *SessionStore) RotateToken(ctx context.Context, sessionID, currentRefreshToken string) (*Session, error) {
	// Validate current session first
	session, err := s.ValidateSession(ctx, sessionID, currentRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to validate session for rotation: %w", err)
	}

	// Generate new refresh token
	newRefreshToken, err := s.generateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate new refresh token: %w", err)
	}

	// Hash new token
	hashedNewToken, err := s.hashToken(newRefreshToken)
	if err != nil {
		return nil, fmt.Errorf("failed to hash new refresh token: %w", err)
	}

	// Update session with new token
	session.RefreshToken = hashedNewToken
	session.LastUsed = time.Now()
	session.ExpiresAt = time.Now().Add(s.tokenExpiry)

	// Update session in Redis
	sessionData, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := s.rdb.Set(ctx, s.sessionKey(sessionID), sessionData, s.tokenExpiry).Err(); err != nil {
		return nil, fmt.Errorf("failed to update session: %w", err)
	}

	// Return session with unhashed token
	session.RefreshToken = newRefreshToken
	return session, nil
}

func (s *SessionStore) DeleteSession(ctx context.Context, sessionID string) error {
	// Get session to find user ID for cleanup
	sessionData, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
	if err == redis.Nil {
		return nil // Session already doesn't exist
	}
	if err != nil {
		return fmt.Errorf("failed to get session for deletion: %w", err)
	}

	var session Session
	if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
		return fmt.Errorf("failed to unmarshal session: %w", err)
	}

	// Remove session from user's session list
	s.rdb.LRem(ctx, s.userKey(session.UserID), 0, sessionID)

	// Delete session data
	if err := s.rdb.Del(ctx, s.sessionKey(sessionID)).Err(); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	return nil
}

func (s *SessionStore) DeleteAllUserSessions(ctx context.Context, userID int64) error {
	// Get all user sessions
	sessionIDs, err := s.rdb.LRange(ctx, s.userKey(userID), 0, -1).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// Delete all session data
	for _, sessionID := range sessionIDs {
		s.rdb.Del(ctx, s.sessionKey(sessionID))
	}

	// Clear user session list
	if err := s.rdb.Del(ctx, s.userKey(userID)).Err(); err != nil {
		return fmt.Errorf("failed to clear user session list: %w", err)
	}

	return nil
}

func (s *SessionStore) GetUserSessions(ctx context.Context, userID int64) ([]*Session, error) {
	sessionIDs, err := s.rdb.LRange(ctx, s.userKey(userID), 0, -1).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get user sessions: %w", err)
	}

	var sessions []*Session
	for _, sessionID := range sessionIDs {
		sessionData, err := s.rdb.Get(ctx, s.sessionKey(sessionID)).Result()
		if err != nil && err != redis.Nil {
			continue // Skip invalid sessions
		}

		var session Session
		if err := json.Unmarshal([]byte(sessionData), &session); err != nil {
			continue // Skip corrupted sessions
		}

		// Don't include hashed token in response
		session.RefreshToken = ""
		sessions = append(sessions, &session)
	}

	return sessions, nil
}
