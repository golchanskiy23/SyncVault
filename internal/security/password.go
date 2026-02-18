package security

import (
	"fmt"
	"regexp"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// PasswordHasher предоставляет безопасное хеширование паролей
type PasswordHasher struct {
	cost int
}

// NewPasswordHasher создает новый хешер паролей
func NewPasswordHasher() *PasswordHasher {
	return &PasswordHasher{
		cost: 12, // Рекомендуемый cost для bcrypt
	}
}

// HashPassword хеширует пароль с bcrypt
func (ph *PasswordHasher) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), ph.cost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}

// VerifyPassword проверяет пароль против хеша
func (ph *PasswordHasher) VerifyPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ValidatePassword проверяет сложность пароля
func (ph *PasswordHasher) ValidatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters long")
	}

	if len(password) > 128 {
		return fmt.Errorf("password must be no more than 128 characters long")
	}

	// Проверка на наличие хотя бы одной заглавной буквы
	hasUpper := false
	for _, r := range password {
		if unicode.IsUpper(r) {
			hasUpper = true
			break
		}
	}
	if !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}

	// Проверка на наличие хотя бы одной строчной буквы
	hasLower := false
	for _, r := range password {
		if unicode.IsLower(r) {
			hasLower = true
			break
		}
	}
	if !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}

	// Проверка на наличие хотя бы одной цифры
	hasDigit := false
	for _, r := range password {
		if unicode.IsDigit(r) {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	// Проверка на наличие хотя бы одного специального символа
	hasSpecial := false
	for _, r := range password {
		if regexp.MustCompile(`[!@#$%^&*()_+\-=\[\]{}|;:,.<>?]`).MatchString(string(r)) {
			hasSpecial = true
			break
		}
	}
	if !hasSpecial {
		return fmt.Errorf("password must contain at least one special character")
	}

	return nil
}
