package interceptors

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// LoggingInterceptor логирует gRPC запросы и ответы
type LoggingInterceptor struct {
	logger Logger
}

// Logger интерфейс для логирования
type Logger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// NewLoggingInterceptor создает новый интерцептор для логирования
func NewLoggingInterceptor(logger Logger) *LoggingInterceptor {
	return &LoggingInterceptor{
		logger: logger,
	}
}

// UnaryInterceptor возвращает unary interceptor для логирования
func (li *LoggingInterceptor) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()

		// Получаем информацию о клиенте
		clientIP := getClientIP(ctx)
		method := info.FullMethod

		// Логируем начало запроса
		li.logger.Info("gRPC request started",
			"method", method,
			"client_ip", clientIP,
			"request_size", getMessageSize(req),
		)

		// Выполняем запрос
		resp, err := handler(ctx, req)

		// Измеряем длительность
		duration := time.Since(start)

		// Логируем результат
		if err != nil {
			grpcStatus, _ := status.FromError(err)
			li.logger.Error("gRPC request failed",
				"method", method,
				"client_ip", clientIP,
				"duration", duration,
				"error", err.Error(),
				"code", grpcStatus.Code(),
				"message", grpcStatus.Message(),
			)
		} else {
			li.logger.Info("gRPC request completed",
				"method", method,
				"client_ip", clientIP,
				"duration", duration,
				"response_size", getMessageSize(resp),
			)
		}

		return resp, err
	}
}

// StreamInterceptor возвращает stream interceptor для логирования
func (li *LoggingInterceptor) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()

		// Получаем информацию о клиенте
		clientIP := getClientIP(ss.Context())
		method := info.FullMethod

		// Логируем начало стрима
		li.logger.Info("gRPC stream started",
			"method", method,
			"client_ip", clientIP,
			"stream_type", getStreamType(info),
		)

		// Выполняем стрим
		err := handler(srv, ss)

		// Измеряем длительность
		duration := time.Since(start)

		// Логируем результат
		if err != nil {
			grpcStatus, _ := status.FromError(err)
			li.logger.Error("gRPC stream failed",
				"method", method,
				"client_ip", clientIP,
				"duration", duration,
				"error", err.Error(),
				"code", grpcStatus.Code(),
				"message", grpcStatus.Message(),
			)
		} else {
			li.logger.Info("gRPC stream completed",
				"method", method,
				"client_ip", clientIP,
				"duration", duration,
			)
		}

		return err
	}
}

// getClientIP извлекает IP адрес клиента из контекста
func getClientIP(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok {
		return p.Addr.String()
	}
	return "unknown"
}

// getMessageSize возвращает размер сообщения в байтах
func getMessageSize(msg interface{}) int {
	if pm, ok := msg.(proto.Message); ok {
		return proto.Size(pm)
	}
	return 0
}

// getStreamType определяет тип стрима
func getStreamType(info *grpc.StreamServerInfo) string {
	switch {
	case info.IsClientStream && info.IsServerStream:
		return "bidirectional"
	case info.IsClientStream:
		return "client_stream"
	case info.IsServerStream:
		return "server_stream"
	default:
		return "unary"
	}
}

// LogEntry представляет запись лога
type LogEntry struct {
	Timestamp    time.Time              `json:"timestamp"`
	Level        string                 `json:"level"`
	Method       string                 `json:"method"`
	ClientIP     string                 `json:"client_ip"`
	Duration     time.Duration          `json:"duration"`
	Error        string                 `json:"error,omitempty"`
	Code         codes.Code             `json:"code,omitempty"`
	Message      string                 `json:"message,omitempty"`
	RequestSize  int                    `json:"request_size,omitempty"`
	ResponseSize int                    `json:"response_size,omitempty"`
	StreamType   string                 `json:"stream_type,omitempty"`
	Fields       map[string]interface{} `json:"fields,omitempty"`
}
