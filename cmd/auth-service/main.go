package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/types/known/timestamppb"

	authv1 "syncvault/internal/grpc/proto/auth"
)

// AuthService микросервис для аутентификации и авторизации
type AuthService struct {
	authv1.UnimplementedAuthServiceServer
}

func (s *AuthService) Login(ctx context.Context, req *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	log.Printf("Login attempt for user: %s", req.Email)

	// Имитация аутентификации
	if req.Email == "test@example.com" && req.Password == "password123" {
		token := fmt.Sprintf("jwt_token_%d", time.Now().UnixNano())
		refreshToken := fmt.Sprintf("refresh_token_%d", time.Now().UnixNano())

		return &authv1.LoginResponse{
			AccessToken:  token,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			User: &authv1.User{
				UserId:    "user_123",
				Email:     req.Email,
				Username:  "testuser",
				FirstName: "Test",
				LastName:  "User",
				IsActive:  true,
				CreatedAt: timestamppb.Now(),
				UpdatedAt: timestamppb.Now(),
			},
		}, nil
	}

	return nil, fmt.Errorf("invalid credentials")
}

func (s *AuthService) Register(ctx context.Context, req *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	log.Printf("Registration attempt for user: %s", req.Email)

	// Имитация регистрации
	userID := fmt.Sprintf("user_%d", time.Now().UnixNano())

	return &authv1.RegisterResponse{
		User: &authv1.User{
			UserId:    userID,
			Email:     req.Email,
			Username:  req.Username,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			IsActive:  true,
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Message: "User registered successfully",
	}, nil
}

func (s *AuthService) RefreshToken(ctx context.Context, req *authv1.RefreshTokenRequest) (*authv1.RefreshTokenResponse, error) {
	log.Printf("Token refresh request")

	// Имитация обновления токена
	newToken := fmt.Sprintf("new_jwt_token_%d", time.Now().UnixNano())

	return &authv1.RefreshTokenResponse{
		AccessToken: newToken,
		TokenType:   "Bearer",
		ExpiresIn:   3600,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, req *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	log.Printf("Logout request for user: %s", req.UserId)

	return &authv1.LogoutResponse{
		Success: true,
		Message: "Logged out successfully",
	}, nil
}

func (s *AuthService) ValidateToken(ctx context.Context, req *authv1.ValidateTokenRequest) (*authv1.ValidateTokenResponse, error) {
	log.Printf("Token validation request")

	// Имитация валидации токена
	if req.AccessToken == "jwt_token_valid" {
		return &authv1.ValidateTokenResponse{
			Valid: true,
			User: &authv1.User{
				UserId:    "user_123",
				Email:     "test@example.com",
				Username:  "testuser",
				FirstName: "Test",
				LastName:  "User",
				IsActive:  true,
				CreatedAt: timestamppb.Now(),
				UpdatedAt: timestamppb.Now(),
			},
			ExpiresAt: timestamppb.Now(),
		}, nil
	}

	return &authv1.ValidateTokenResponse{
		Valid:   false,
		Message: "Invalid token",
	}, nil
}

func (s *AuthService) GetUserProfile(ctx context.Context, req *authv1.GetUserProfileRequest) (*authv1.GetUserProfileResponse, error) {
	log.Printf("Getting user profile for: %s", req.UserId)

	// Имитация получения профиля
	return &authv1.GetUserProfileResponse{
		User: &authv1.User{
			UserId:    req.UserId,
			Email:     "test@example.com",
			Username:  "testuser",
			FirstName: "Test",
			LastName:  "User",
			IsActive:  true,
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
	}, nil
}

func (s *AuthService) UpdateUserProfile(ctx context.Context, req *authv1.UpdateUserProfileRequest) (*authv1.UpdateUserProfileResponse, error) {
	log.Printf("Updating user profile for: %s", req.UserId)

	return &authv1.UpdateUserProfileResponse{
		User: &authv1.User{
			UserId:    req.UserId,
			Email:     req.Email,
			Username:  req.Username,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			IsActive:  true,
			CreatedAt: timestamppb.Now(),
			UpdatedAt: timestamppb.Now(),
		},
		Updated: true,
	}, nil
}

func (s *AuthService) ChangePassword(ctx context.Context, req *authv1.ChangePasswordRequest) (*authv1.ChangePasswordResponse, error) {
	log.Printf("Password change request for user: %s", req.UserId)

	return &authv1.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully",
	}, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, req *authv1.ResetPasswordRequest) (*authv1.ResetPasswordResponse, error) {
	log.Printf("Password reset request for email: %s", req.Email)

	return &authv1.ResetPasswordResponse{
		Success:   true,
		Message:   "Password reset email sent",
		Token:     fmt.Sprintf("reset_token_%d", time.Now().UnixNano()),
		ExpiresAt: timestamppb.Now(),
	}, nil
}

func main() {
	log.Println("Starting Auth Service microservice...")

	// Создаем gRPC сервер
	grpcServer := grpc.NewServer()

	// Регистрируем сервисы
	authService := &AuthService{}
	authv1.RegisterAuthServiceServer(grpcServer, authService)

	// Включаем reflection для разработки
	reflection.Register(grpcServer)

	// Настраиваем порт
	port := "50056"
	if envPort := os.Getenv("AUTH_SERVICE_PORT"); envPort != "" {
		port = envPort
	}

	// Создаем listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	log.Printf("Auth Service listening on port %s", port)

	// Запускаем сервер в горутине
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Auth Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutdown timeout, forcing stop...")
		grpcServer.Stop()
	case <-stopped:
		log.Println("Auth Service stopped gracefully")
	}
}
