package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler обрабатывает HTTP запросы аутентификации
type AuthHandler struct {
	jwtService  *JWTService
	userService UserService
}

// UserService определяет интерфейс для работы с пользователями
type UserService interface {
	GetUserByEmail(ctx context.Context, email string) (*UserInfo, error)
	VerifyPassword(hashedPassword, password string) bool
}

// NewAuthHandler создает новый обработчик аутентификации
func NewAuthHandler(jwtService *JWTService, userService UserService) *AuthHandler {
	return &AuthHandler{
		jwtService:  jwtService,
		userService: userService,
	}
}

// Login обрабатывает POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Валидация входных данных
	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Получаем пользователя из базы данных
	user, err := h.userService.GetUserByEmail(ctx, req.Email)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Проверяем пароль
	if !h.userService.VerifyPassword(user.PasswordHash, req.Password) {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Генерируем пару токенов
	tokenPair, err := h.jwtService.GenerateTokenPair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		http.Error(w, "Failed to generate tokens", http.StatusInternalServerError)
		return
	}

	// Формируем ответ
	response := AuthResponse{
		TokenPair: *tokenPair,
		User: UserInfo{
			ID:    user.ID,
			Email: user.Email,
			Role:  user.Role,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Refresh обрабатывает POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.RefreshToken == "" {
		http.Error(w, "Refresh token is required", http.StatusBadRequest)
		return
	}

	// Выполняем ротацию refresh токена
	newTokenPair, err := h.jwtService.RefreshTokens(ctx, req.RefreshToken)
	if err != nil {
		http.Error(w, "Failed to refresh token: "+err.Error(), http.StatusUnauthorized)
		return
	}

	// Формируем ответ
	response := map[string]interface{}{
		"tokens": newTokenPair,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Logout обрабатывает POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Получаем refresh token из запроса (опционально, можно из body или header)
	var req struct {
		RefreshToken string `json:"refresh_token,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Если тело запроса пустое, пробуем получить токен из заголовка
		req.RefreshToken = r.Header.Get("X-Refresh-Token")
	}

	// Если предоставлен refresh token - отзываем его
	if req.RefreshToken != "" {
		claims, err := h.jwtService.ValidateToken(req.RefreshToken, RefreshTokenType)
		if err == nil {
			h.jwtService.RevokeRefreshToken(ctx, claims.TokenID)
		}
	}

	// Получаем ID пользователя из context для полного logout
	if userID, ok := GetUserIDFromContext(ctx); ok {
		// Выполняем полный logout пользователя (отзыв всех refresh токенов)
		err := h.jwtService.Logout(ctx, userID)
		if err != nil {
			// Логируем ошибку, но не возвращаем ошибку клиенту
			// так как основная задача - очистить сессию на клиенте
			// в production здесь лучше добавить логирование
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Logged out successfully",
	})
}

// Me обрабатывает GET /api/v1/auth/me
// Возвращает информацию о текущем пользователе
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Получаем claims из context
	claims, ok := GetClaimsFromContext(ctx)
	if !ok {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Формируем ответ с информацией о пользователе
	response := UserInfo{
		ID:    claims.UserID,
		Email: claims.Email,
		Role:  claims.Role,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes регистрирует роуты аутентификации
func (h *AuthHandler) RegisterRoutes(router chi.Router) {
	router.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.With(h.jwtService.AuthMiddleware).Post("/logout", h.Logout)
		r.With(h.jwtService.AuthMiddleware).Get("/me", h.Me)
	})
}

// Вспомогательные функции

// HashPassword хеширует пароль с использованием bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword проверяет пароль
func CheckPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// ErrorResponse представляет структуру ошибки
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    int    `json:"code"`
}

// writeErrorResponse пишет ошибку в ответ
func writeErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error:   http.StatusText(statusCode),
		Message: message,
		Code:    statusCode,
	})
}
