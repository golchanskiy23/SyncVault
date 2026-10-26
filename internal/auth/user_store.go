package auth

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"syncvault/internal/security"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user with this email or username already exists")
)

// PgUserStore — пользователи в таблице users (PostgreSQL).
// Реализует UserService.
type PgUserStore struct {
	db     *pgxpool.Pool
	hasher *security.PasswordHasher
}

func NewPgUserStore(db *pgxpool.Pool) *PgUserStore {
	return &PgUserStore{db: db, hasher: security.NewPasswordHasher()}
}

// CreateUser проверяет сложность пароля, хеширует его и создаёт пользователя
func (s *PgUserStore) CreateUser(ctx context.Context, email, username, password string) (*UserInfo, error) {
	if err := s.hasher.ValidatePassword(password); err != nil {
		return nil, err
	}
	hash, err := s.hasher.HashPassword(password)
	if err != nil {
		return nil, err
	}

	var id int64
	err = s.db.QueryRow(ctx, `
		INSERT INTO users (username, email, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id
	`, username, email, hash).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return &UserInfo{ID: strconv.FormatInt(id, 10), Email: email, Role: "user"}, nil
}

func (s *PgUserStore) GetUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	return s.getUser(ctx, `WHERE email = $1`, email)
}

func (s *PgUserStore) GetUserByID(ctx context.Context, id string) (*UserInfo, error) {
	return s.getUser(ctx, `WHERE id = $1`, id)
}

func (s *PgUserStore) getUser(ctx context.Context, where string, arg any) (*UserInfo, error) {
	var (
		id   int64
		user UserInfo
	)
	err := s.db.QueryRow(ctx, `
		SELECT id, email, password_hash FROM users `+where+` AND is_active = true
	`, arg).Scan(&id, &user.Email, &user.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	user.ID = strconv.FormatInt(id, 10)
	user.Role = "user"
	return &user, nil
}

func (s *PgUserStore) VerifyPassword(hashedPassword, password string) bool {
	return s.hasher.VerifyPassword(password, hashedPassword)
}

// Authenticate возвращает пользователя, если email и пароль верны
func (s *PgUserStore) Authenticate(ctx context.Context, email, password string) (*UserInfo, error) {
	user, err := s.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	if !s.VerifyPassword(user.PasswordHash, password) {
		return nil, ErrUserNotFound
	}
	return user, nil
}
