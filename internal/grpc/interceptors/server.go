package interceptors

import (
	"context"
	"time"

	"google.golang.org/grpc"
)

// ServerConfig представляет конфигурацию gRPC сервера
type ServerConfig struct {
	Port             int           `json:"port"`
	Timeout          time.Duration `json:"timeout"`
	MaxRecvMsgSize   int           `json:"max_recv_msg_size"`
	MaxSendMsgSize   int           `json:"max_send_msg_size"`
	EnableReflection bool          `json:"enable_reflection"`
	EnableTLS        bool          `json:"enable_tls"`
	CertFile         string        `json:"cert_file"`
	KeyFile          string        `json:"key_file"`
}

// Server представляет gRPC сервер с интерцепторами
type Server struct {
	config             *ServerConfig
	loggingInterceptor *LoggingInterceptor
	authInterceptor    *AuthInterceptor
}

// NewServer создает новый gRPC сервер
func NewServer(config *ServerConfig, logger Logger, jwtValidator JWTValidator, publicMethods ...string) *Server {
	return &Server{
		config:             config,
		loggingInterceptor: NewLoggingInterceptor(logger),
		authInterceptor:    NewAuthInterceptor(jwtValidator, publicMethods...),
	}
}

// NewGRPCServer создает настроенный gRPC сервер с интерцепторами
func (s *Server) NewGRPCServer() *grpc.Server {
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

	return grpc.NewServer(opts...)
}

// DefaultServerConfig возвращает конфигурацию по умолчанию
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Port:             50051,
		Timeout:          30 * time.Second,
		MaxRecvMsgSize:   4 * 1024 * 1024, // 4MB
		MaxSendMsgSize:   4 * 1024 * 1024, // 4MB
		EnableReflection: true,
		EnableTLS:        false,
	}
}

// HealthCheckService представляет сервис для health check'ов
type HealthCheckService struct{}

// Check реализация health check
func (h *HealthCheckService) Check(ctx context.Context, req *HealthCheckRequest) (*HealthCheckResponse, error) {
	return &HealthCheckResponse{
		Status: HealthCheckResponse_SERVING,
	}, nil
}

// Watch реализация streaming health check
func (h *HealthCheckService) Watch(ctx context.Context, req *HealthCheckRequest, stream Health_WatchServer) error {
	// Отправляем первоначальный статус
	if err := stream.Send(&HealthCheckResponse{
		Status: HealthCheckResponse_SERVING,
	}); err != nil {
		return err
	}

	// В реальном приложении здесь может быть логика для отслеживания состояния
	// и отправки обновлений при изменениях
	<-ctx.Done()
	return ctx.Err()
}

// HealthCheckRequest запрос для health check
type HealthCheckRequest struct {
	Service string `protobuf:"bytes,1,opt,name=service,proto3" json:"service,omitempty"`
}

// HealthCheckResponse ответ для health check
type HealthCheckResponse struct {
	Status HealthCheckResponse_ServingStatus `protobuf:"varint,1,opt,name=status,proto3,enum=grpc.health.v1.HealthCheckResponse.ServingStatus" json:"status,omitempty"`
}

// HealthCheckResponse_ServingStatus статус сервиса
type HealthCheckResponse_ServingStatus int32

const (
	HealthCheckResponse_UNKNOWN         HealthCheckResponse_ServingStatus = 0
	HealthCheckResponse_SERVING         HealthCheckResponse_ServingStatus = 1
	HealthCheckResponse_NOT_SERVING     HealthCheckResponse_ServingStatus = 2
	HealthCheckResponse_SERVICE_UNKNOWN HealthCheckResponse_ServingStatus = 3
)

// Health_WatchServer интерфейс для streaming health check
type Health_WatchServer interface {
	Send(*HealthCheckResponse) error
	grpc.ServerStream
}
