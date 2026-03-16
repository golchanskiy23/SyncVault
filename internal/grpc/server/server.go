package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"syncvault/internal/grpc/interceptors"
	healthv1 "syncvault/internal/grpc/proto/health"
	notificationv1 "syncvault/internal/grpc/proto/notification"
	storagev1 "syncvault/internal/grpc/proto/storage"
	syncv1 "syncvault/internal/grpc/proto/sync"
)

// SimpleLogger простая реализация логгера для gRPC сервера
type SimpleLogger struct {
	level string
}

func (l *SimpleLogger) Info(msg string, fields ...interface{}) {
	log.Printf("[INFO] %s %v", msg, fields)
}

func (l *SimpleLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] %s %v", msg, fields)
}

func (l *SimpleLogger) Warn(msg string, fields ...interface{}) {
	log.Printf("[WARN] %s %v", msg, fields)
}

func (l *SimpleLogger) Debug(msg string, fields ...interface{}) {
	log.Printf("[DEBUG] %s %v", msg, fields)
}

// Server представляет gRPC сервер SyncVault
type Server struct {
	config *ServerConfig
	logger interceptors.Logger

	// gRPC сервисы
	storageService      *StorageService
	syncService         *SyncService
	notificationService *NotificationService
	healthService       *HealthService

	// Интерцепторы
	loggingInterceptor *interceptors.LoggingInterceptor
	authInterceptor    *interceptors.AuthInterceptor
	publicMethods      []string
}

// ServerConfig конфигурация gRPC сервера
type ServerConfig struct {
	Port             int           `yaml:"port"`
	Host             string        `yaml:"host"`
	Timeout          time.Duration `yaml:"timeout"`
	MaxRecvMsgSize   int           `yaml:"max_recv_msg_size"`
	MaxSendMsgSize   int           `yaml:"max_send_msg_size"`
	EnableReflection bool          `yaml:"enable_reflection"`
	EnableTLS        bool          `yaml:"enable_tls"`
	CertFile         string        `yaml:"cert_file"`
	KeyFile          string        `yaml:"key_file"`

	// JWT конфигурация
	JWTSecret     string        `yaml:"jwt_secret"`
	JWTIssuer     string        `yaml:"jwt_issuer"`
	TokenExpiry   time.Duration `yaml:"token_expiry"`
	RefreshExpiry time.Duration `yaml:"refresh_expiry"`

	// Логирование
	LogLevel      string `yaml:"log_level"`
	EnableMetrics bool   `yaml:"enable_metrics"`
}

// Option тип для функциональной конфигурации
type Option func(*ServerConfig)

// WithHost устанавливает хост
func WithHost(host string) Option {
	return func(c *ServerConfig) {
		c.Host = host
	}
}

// WithPort устанавливает порт
func WithPort(port int) Option {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

// WithJWTSecret устанавливает JWT секрет
func WithJWTSecret(secret string) Option {
	return func(c *ServerConfig) {
		c.JWTSecret = secret
	}
}

// WithEnableReflection включает рефлексию
func WithEnableReflection(enable bool) Option {
	return func(c *ServerConfig) {
		c.EnableReflection = enable
	}
}

// WithEnableTLS включает TLS
func WithEnableTLS(enable bool) Option {
	return func(c *ServerConfig) {
		c.EnableTLS = enable
	}
}

// NewServer создает новый gRPC сервер
func NewServer(opts ...Option) *Server {
	config := DefaultConfig()

	// Применяем опции
	for _, opt := range opts {
		opt(config)
	}

	// Создаем JWT валидатор
	jwtValidator := interceptors.NewJWTValidator(
		config.JWTSecret,
		config.JWTIssuer,
		config.TokenExpiry,
		config.RefreshExpiry,
	)

	// Создаем простой logger
	logger := &SimpleLogger{
		level: config.LogLevel,
	}

	// Публичные методы (без аутентификации)
	publicMethods := []string{
		"Check",
		"Watch",
		"HealthCheck",
		"Live",
		"Ready",
		"Startup",
	}

	// Создаем интерцепторы
	loggingInterceptor := interceptors.NewLoggingInterceptor(logger)
	authInterceptor := interceptors.NewAuthInterceptor(jwtValidator, publicMethods...)

	return &Server{
		config:              config,
		logger:              logger,
		storageService:      NewStorageService(),
		syncService:         NewSyncService(),
		notificationService: NewNotificationService(),
		healthService:       NewHealthService(),
		loggingInterceptor:  loggingInterceptor,
		authInterceptor:     authInterceptor,
		publicMethods:       publicMethods,
	}
}

// Start запускает gRPC сервер
func (s *Server) Start() error {
	// Создаем listener
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.config.Host, s.config.Port))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %w", s.config.Host, s.config.Port, err)
	}

	// Создаем gRPC сервер с интерцепторами
	grpcServer := s.createGRPCServer()

	s.logger.Info("Starting gRPC server",
		"host", s.config.Host,
		"port", s.config.Port,
		"tls", s.config.EnableTLS,
		"reflection", s.config.EnableReflection,
	)

	// Регистрируем сервисы
	s.registerServices(grpcServer)

	// Включаем reflection если нужно
	if s.config.EnableReflection {
		reflection.Register(grpcServer)
		s.logger.Info("gRPC reflection enabled")
	}

	// Запускаем сервер
	return grpcServer.Serve(lis)
}

// Stop останавливает gRPC сервер
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Stopping gRPC server")
	// Здесь будет логика graceful shutdown
	return nil
}

// createGRPCServer создает настроенный gRPC сервер
func (s *Server) createGRPCServer() *grpc.Server {
	opts := []grpc.ServerOption{
		// Unary интерцепторы
		grpc.ChainUnaryInterceptor(
			s.loggingInterceptor.UnaryInterceptor(),
			s.authInterceptor.UnaryInterceptor(),
		),
		// Stream интерцепторы
		grpc.ChainStreamInterceptor(
			s.loggingInterceptor.StreamInterceptor(),
			s.authInterceptor.StreamInterceptor(),
		),
		// Размеры сообщений
		grpc.MaxRecvMsgSize(s.config.MaxRecvMsgSize),
		grpc.MaxSendMsgSize(s.config.MaxSendMsgSize),
		// Таймауты
		grpc.ConnectionTimeout(s.config.Timeout),
	}

	// Добавляем TLS если нужно
	if s.config.EnableTLS {
		// Здесь будет логика TLS конфигурации
		s.logger.Info("TLS enabled (cert file: %s, key file: %s)", s.config.CertFile, s.config.KeyFile)
	}

	return grpc.NewServer(opts...)
}

// registerServices регистрирует gRPC сервисы
func (s *Server) registerServices(grpcServer *grpc.Server) {
	// Регистрируем Storage сервис
	storagev1.RegisterStorageServiceServer(grpcServer, s.storageService)
	s.logger.Info("Storage service registered")

	// Регистрируем Sync сервис
	syncv1.RegisterSyncServiceServer(grpcServer, s.syncService)
	s.logger.Info("Sync service registered")

	// Регистрируем Notification сервис
	notificationv1.RegisterNotificationServiceServer(grpcServer, s.notificationService)
	s.logger.Info("Notification service registered")

	// Регистрируем Health сервис
	healthv1.RegisterHealthServiceServer(grpcServer, s.healthService)
	s.logger.Info("Health service registered")
}

// DefaultConfig возвращает конфигурацию по умолчанию
func DefaultConfig() *ServerConfig {
	return &ServerConfig{
		Host:             "0.0.0.0",
		Port:             50051,
		Timeout:          30 * time.Second,
		MaxRecvMsgSize:   4 * 1024 * 1024, // 4MB
		MaxSendMsgSize:   4 * 1024 * 1024, // 4MB
		EnableReflection: true,
		EnableTLS:        false,

		JWTSecret:     "SyncVault@Dev#2024!xK9mZaaaaaaaa",
		JWTIssuer:     "syncvault",
		TokenExpiry:   1 * time.Hour,
		RefreshExpiry: 24 * time.Hour,

		LogLevel:      "info",
		EnableMetrics: true,
	}
}

// ConfigFromYAML создает конфигурацию из YAML файла
func ConfigFromYAML(filename string) (*ServerConfig, error) {
	// Здесь будет логика чтения YAML конфигурации
	config := DefaultConfig()

	// Временная реализация - в реальном приложении использовать yaml.Unmarshal
	log.Printf("Loading config from %s (using defaults for now)", filename)

	return config, nil
}

// ValidateConfig проверяет валидность конфигурации
func ValidateConfig(config *ServerConfig) error {
	if config.Port <= 0 || config.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", config.Port)
	}

	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	if config.MaxRecvMsgSize <= 0 {
		return fmt.Errorf("max_recv_msg_size must be positive")
	}

	if config.MaxSendMsgSize <= 0 {
		return fmt.Errorf("max_send_msg_size must be positive")
	}

	// Валидируем JWT секрет
	if err := interceptors.ValidateSecretKey(config.JWTSecret); err != nil {
		return fmt.Errorf("invalid JWT secret: %w", err)
	}

	return nil
}
